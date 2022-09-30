// Copyright 2014 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

// Package state provides a caching layer atop the Ethereum state trie.
package state

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"github.com/FusionFoundation/efsn/v4/core/rawdb"
	"io"
	"math/big"
	"sort"
	"sync"

	"github.com/FusionFoundation/efsn/v4/common"
	"github.com/FusionFoundation/efsn/v4/core/types"
	"github.com/FusionFoundation/efsn/v4/crypto"
	"github.com/FusionFoundation/efsn/v4/log"
	"github.com/FusionFoundation/efsn/v4/rlp"
	"github.com/FusionFoundation/efsn/v4/trie"
)

type revision struct {
	id           int
	journalIndex int
}

var (
	// emptyRoot is the known root hash of an empty trie.
	emptyRoot = common.HexToHash("56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421")

	// emptyCode is the known hash of the empty EVM bytecode.
	emptyCode = crypto.Keccak256Hash(nil)
)

// StateDB structs within the ethereum protocol are used to store anything
// within the merkle trie. StateDBs take care of caching and storing
// nested states. It's the general query interface to retrieve:
// * Contracts
// * Accounts
type StateDB struct {
	db   Database
	trie Trie

	// This map holds 'live' objects, which will get modified while processing a state transition.
	stateObjects        map[common.Address]*stateObject
	stateObjectsPending map[common.Address]struct{} // State objects finalized but not yet written to the trie
	stateObjectsDirty   map[common.Address]struct{} // State objects modified in the current execution

	// DB error.
	// State objects are used by the consensus core and VM which are
	// unable to deal with database-level errors. Any error that occurs
	// during a database read is memoized here and will eventually be returned
	// by StateDB.Commit.
	dbErr error

	// The refund counter, also used by state transitioning.
	refund uint64

	thash   common.Hash
	txIndex int
	logs    map[common.Hash][]*types.Log
	logSize uint

	preimages map[common.Hash][]byte

	// Per-transaction access list
	accessList *accessList

	// Journal of state modifications. This is the backbone of
	// Snapshot and RevertToSnapshot.
	journal        *journal
	validRevisions []revision
	nextRevisionId int

	ticketsHash common.Hash
	tickets     common.TicketsDataSlice

	rwlock sync.RWMutex
}

type CachedTickets struct {
	hash    common.Hash
	tickets common.TicketsDataSlice
}

const maxCachedTicketsCount = 101

type CachedTicketSlice struct {
	tickets [maxCachedTicketsCount]CachedTickets
	start   int64
	end     int64
	rwlock  sync.RWMutex
}

var cachedTicketSlice = CachedTicketSlice{
	tickets: [maxCachedTicketsCount]CachedTickets{},
	start:   0,
	end:     0,
}

func (cts *CachedTicketSlice) Add(hash common.Hash, tickets common.TicketsDataSlice) {
	if cts.Get(hash) != nil {
		return
	}

	elem := CachedTickets{
		hash:    hash,
		tickets: tickets.DeepCopy(),
	}

	cts.rwlock.Lock()
	defer cts.rwlock.Unlock()

	cts.tickets[cts.end] = elem
	cts.end = (cts.end + 1) % maxCachedTicketsCount
	if cts.end == cts.start {
		cts.start = (cts.start + 1) % maxCachedTicketsCount
	}
}

func (cts *CachedTicketSlice) Get(hash common.Hash) common.TicketsDataSlice {
	if hash == (common.Hash{}) {
		return common.TicketsDataSlice{}
	}

	cts.rwlock.RLock()
	defer cts.rwlock.RUnlock()

	for i := cts.start; i != cts.end; i = (i + 1) % maxCachedTicketsCount {
		v := cts.tickets[i]
		if v.hash == hash {
			return v.tickets
		}
	}
	return nil
}

func GetCachedTickets(hash common.Hash) common.TicketsDataSlice {
	return cachedTicketSlice.Get(hash)
}

func calcTicketsStorageData(tickets common.TicketsDataSlice) ([]byte, error) {
	blob, err := rlp.EncodeToBytes(&tickets)
	if err != nil {
		return nil, fmt.Errorf("Unable to encode tickets. err: %v", err)
	}

	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(blob); err != nil {
		return nil, fmt.Errorf("Unable to zip tickets data")
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("Unable to zip tickets")
	}
	data := buf.Bytes()
	return data, nil
}

func AddCachedTickets(hash common.Hash, tickets common.TicketsDataSlice) error {
	data, err := calcTicketsStorageData(tickets)
	if err != nil {
		return fmt.Errorf("AddCachedTickets: %v", err)
	}
	if hash != crypto.Keccak256Hash(data) {
		return fmt.Errorf("AddCachedTickets: hash mismatch")
	}
	cachedTicketSlice.Add(hash, tickets)
	return nil
}

// Create a new state from a given trie.
func New(root common.Hash, mixDigest common.Hash, db Database) (*StateDB, error) {
	tr, err := db.OpenTrie(root)
	if err != nil {
		return nil, err
	}
	statedb := &StateDB{
		db:                  db,
		trie:                tr,
		stateObjects:        make(map[common.Address]*stateObject),
		stateObjectsPending: make(map[common.Address]struct{}),
		stateObjectsDirty:   make(map[common.Address]struct{}),
		logs:                make(map[common.Hash][]*types.Log),
		preimages:           make(map[common.Hash][]byte),
		journal:             newJournal(),
		accessList:          newAccessList(),
		ticketsHash:         mixDigest,
		tickets:             nil,
	}
	return statedb, nil
}

// setError remembers the first non-nil error it is called with.
func (s *StateDB) setError(err error) {
	if s.dbErr == nil {
		s.dbErr = err
	}
}

func (s *StateDB) Error() error {
	return s.dbErr
}

// Reset clears out all ephemeral state objects from the state db, but keeps
// the underlying state trie to avoid reloading data for the next operations.
func (s *StateDB) Reset(root common.Hash) error {
	tr, err := s.db.OpenTrie(root)
	if err != nil {
		return err
	}
	s.trie = tr
	s.stateObjects = make(map[common.Address]*stateObject)
	s.stateObjectsPending = make(map[common.Address]struct{})
	s.stateObjectsDirty = make(map[common.Address]struct{})
	s.thash = common.Hash{}
	s.txIndex = 0
	s.logs = make(map[common.Hash][]*types.Log)
	s.logSize = 0
	s.preimages = make(map[common.Hash][]byte)
	s.clearJournalAndRefund()
	s.accessList = newAccessList()
	s.tickets = nil
	return nil
}

func (s *StateDB) AddLog(log *types.Log) {
	s.journal.append(addLogChange{txhash: s.thash})

	log.TxHash = s.thash
	log.TxIndex = uint(s.txIndex)
	log.Index = s.logSize
	s.logs[s.thash] = append(s.logs[s.thash], log)
	s.logSize++
}

func (s *StateDB) GetLogs(hash common.Hash, blockHash common.Hash) []*types.Log {
	logs := s.logs[hash]
	for _, l := range logs {
		l.BlockHash = blockHash
	}
	return logs
}

func (s *StateDB) Logs() []*types.Log {
	var logs []*types.Log
	for _, lgs := range s.logs {
		logs = append(logs, lgs...)
	}
	return logs
}

// AddPreimage records a SHA3 preimage seen by the VM.
func (s *StateDB) AddPreimage(hash common.Hash, preimage []byte) {
	if _, ok := s.preimages[hash]; !ok {
		s.journal.append(addPreimageChange{hash: hash})
		pi := make([]byte, len(preimage))
		copy(pi, preimage)
		s.preimages[hash] = pi
	}
}

// Preimages returns a list of SHA3 preimages that have been submitted.
func (s *StateDB) Preimages() map[common.Hash][]byte {
	return s.preimages
}

// AddRefund adds gas to the refund counter
func (s *StateDB) AddRefund(gas uint64) {
	s.journal.append(refundChange{prev: s.refund})
	s.refund += gas
}

// SubRefund removes gas from the refund counter.
// This method will panic if the refund counter goes below zero
func (s *StateDB) SubRefund(gas uint64) {
	s.journal.append(refundChange{prev: s.refund})
	if gas > s.refund {
		panic("Refund counter below zero")
	}
	s.refund -= gas
}

// Exist reports whether the given account address exists in the state.
// Notably this also returns true for suicided accounts.
func (s *StateDB) Exist(addr common.Address) bool {
	return s.getStateObject(addr) != nil
}

// Empty returns whether the state object is either non-existent
// or empty according to the EIP161 specification (balance = nonce = code = 0)
func (s *StateDB) Empty(addr common.Address) bool {
	so := s.getStateObject(addr)
	return so == nil || so.empty()
}

func (s *StateDB) GetAllBalances(addr common.Address) map[common.Hash]string {
	stateObject := s.getStateObject(addr)
	if stateObject != nil {
		return stateObject.CopyBalances()
	}
	return make(map[common.Hash]string)
}

func (s *StateDB) GetBalance(assetID common.Hash, addr common.Address) *big.Int {
	stateObject := s.getStateObject(addr)
	if stateObject != nil {
		return stateObject.Balance(assetID)
	}
	return big.NewInt(0)
}

func (s *StateDB) GetAllTimeLockBalances(addr common.Address) map[common.Hash]*common.TimeLock {
	stateObject := s.getStateObject(addr)
	if stateObject != nil {
		return stateObject.CopyTimeLockBalances()
	}
	return make(map[common.Hash]*common.TimeLock)
}

func (s *StateDB) GetTimeLockBalance(assetID common.Hash, addr common.Address) *common.TimeLock {
	stateObject := s.getStateObject(addr)
	if stateObject != nil {
		return stateObject.TimeLockBalance(assetID)
	}
	return new(common.TimeLock)
}

func (s *StateDB) GetNonce(addr common.Address) uint64 {
	stateObject := s.getStateObject(addr)
	if stateObject != nil {
		return stateObject.Nonce()
	}

	return 0
}

// TxIndex returns the current transaction index set by Prepare.
func (s *StateDB) TxIndex() int {
	return s.txIndex
}

func (s *StateDB) GetCode(addr common.Address) []byte {
	stateObject := s.getStateObject(addr)
	if stateObject != nil {
		return stateObject.Code(s.db)
	}
	return nil
}

func (s *StateDB) GetCodeSize(addr common.Address) int {
	stateObject := s.getStateObject(addr)
	if stateObject != nil {
		return stateObject.CodeSize(s.db)
	}
	return 0
}

func (s *StateDB) GetCodeHash(addr common.Address) common.Hash {
	stateObject := s.getStateObject(addr)
	if stateObject == nil {
		return common.Hash{}
	}
	return common.BytesToHash(stateObject.CodeHash())
}

// GetState retrieves a value from the given account's storage trie.
func (s *StateDB) GetState(addr common.Address, hash common.Hash) common.Hash {
	stateObject := s.getStateObject(addr)
	if stateObject != nil {
		return stateObject.GetState(s.db, hash)
	}
	return common.Hash{}
}

// GetCommittedState retrieves a value from the given account's committed storage trie.
func (s *StateDB) GetCommittedState(addr common.Address, hash common.Hash) common.Hash {
	stateObject := s.getStateObject(addr)
	if stateObject != nil {
		return stateObject.GetCommittedState(s.db, hash)
	}
	return common.Hash{}
}

func (s *StateDB) GetData(addr common.Address) []byte {
	stateObject := s.getStateObject(addr)
	if stateObject != nil {
		return stateObject.Code(s.db)
	}
	return nil
}

func (s *StateDB) GetDataHash(addr common.Address) common.Hash {
	stateObject := s.getStateObject(addr)
	if stateObject != nil {
		return common.BytesToHash(stateObject.CodeHash())
	}
	return common.Hash{}
}

// Database retrieves the low level database supporting the lower level trie ops.
func (s *StateDB) Database() Database {
	return s.db
}

// StorageTrie returns the storage trie of an account.
// The return value is a copy and is nil for non-existent accounts.
func (s *StateDB) StorageTrie(addr common.Address) Trie {
	stateObject := s.getStateObject(addr)
	if stateObject == nil {
		return nil
	}
	cpy := stateObject.deepCopy(s)
	cpy.updateTrie(s.db)
	return cpy.getTrie(s.db)
}

func (s *StateDB) HasSuicided(addr common.Address) bool {
	stateObject := s.getStateObject(addr)
	if stateObject != nil {
		return stateObject.suicided
	}
	return false
}

/*
 * SETTERS
 */

// AddBalance adds amount to the account associated with addr.
func (s *StateDB) AddBalance(addr common.Address, assetID common.Hash, amount *big.Int) {
	stateObject := s.GetOrNewStateObject(addr)
	if stateObject != nil {
		stateObject.AddBalance(assetID, amount)
	}
}

// SubBalance subtracts amount from the account associated with addr.
func (s *StateDB) SubBalance(addr common.Address, assetID common.Hash, amount *big.Int) {
	stateObject := s.GetOrNewStateObject(addr)
	if stateObject != nil {
		stateObject.SubBalance(assetID, amount)
	}
}

func (s *StateDB) SetBalance(addr common.Address, assetID common.Hash, amount *big.Int) {
	stateObject := s.GetOrNewStateObject(addr)
	if stateObject != nil {
		stateObject.SetBalance(assetID, amount)
	}
}

func (s *StateDB) AddTimeLockBalance(addr common.Address, assetID common.Hash, amount *common.TimeLock, timestamp uint64) {
	stateObject := s.GetOrNewStateObject(addr)
	if stateObject != nil {
		stateObject.AddTimeLockBalance(assetID, amount, timestamp)
	}
}

func (s *StateDB) SubTimeLockBalance(addr common.Address, assetID common.Hash, amount *common.TimeLock, timestamp uint64) {
	stateObject := s.GetOrNewStateObject(addr)
	if stateObject != nil {
		stateObject.SubTimeLockBalance(assetID, amount, timestamp)
	}
}

func (s *StateDB) SetTimeLockBalance(addr common.Address, assetID common.Hash, amount *common.TimeLock) {
	stateObject := s.GetOrNewStateObject(addr)
	if stateObject != nil {
		stateObject.SetTimeLockBalance(assetID, amount)
	}
}

func (s *StateDB) SetNonce(addr common.Address, nonce uint64) {
	stateObject := s.GetOrNewStateObject(addr)
	if stateObject != nil {
		stateObject.SetNonce(nonce)
	}
}

func (s *StateDB) SetCode(addr common.Address, code []byte) {
	stateObject := s.GetOrNewStateObject(addr)
	if stateObject != nil {
		stateObject.SetCode(crypto.Keccak256Hash(code), code)
	}
}

func (s *StateDB) SetState(addr common.Address, key, value common.Hash) {
	stateObject := s.GetOrNewStateObject(addr)
	if stateObject != nil {
		stateObject.SetState(s.db, key, value)
	}
}

// SetStorage replaces the entire storage for the specified account with given
// storage. This function should only be used for debugging.
func (s *StateDB) SetStorage(addr common.Address, storage map[common.Hash]common.Hash) {
	stateObject := s.GetOrNewStateObject(addr)
	if stateObject != nil {
		stateObject.SetStorage(storage)
	}
}

func (s *StateDB) SetData(addr common.Address, value []byte) common.Hash {
	stateObject := s.GetOrNewStateObject(addr)
	if stateObject != nil {
		hash := crypto.Keccak256Hash(value)
		stateObject.SetCode(hash, value)
		return hash
	}
	return common.Hash{}
}

// Suicide marks the given account as suicided.
// This clears the account balance.
//
// The account's state object is still available until the state is committed,
// getStateObject will return a non-nil account after Suicide.
func (s *StateDB) Suicide(addr common.Address) bool {
	stateObject := s.getStateObject(addr)
	if stateObject == nil {
		return false
	}
	for i, v := range stateObject.data.BalancesVal {
		k := stateObject.data.BalancesHash[i]
		s.journal.append(suicideChange{
			isTimeLock:  false,
			account:     &addr,
			prev:        stateObject.suicided,
			assetID:     k,
			prevbalance: new(big.Int).Set(v),
		})
		stateObject.suicided = true
		stateObject.setBalance(k, new(big.Int))
	}

	for i, v := range stateObject.data.TimeLockBalancesVal {
		k := stateObject.data.TimeLockBalancesHash[i]
		s.journal.append(suicideChange{
			account:             &addr,
			prev:                stateObject.suicided,
			assetID:             k,
			prevTimeLockBalance: new(common.TimeLock).Set(v),
		})
		stateObject.suicided = true
		stateObject.SetTimeLockBalance(k, new(common.TimeLock))
	}

	stateObject.markSuicided()
	return true
}

func (s *StateDB) TransferAll(from, to common.Address, timestamp uint64) {
	fromObject := s.getStateObject(from)
	if fromObject == nil {
		return
	}

	// remove tickets
	s.ClearTickets(from, to, timestamp)

	// burn notation
	s.BurnNotation(from)

	// transfer all balances
	for i, v := range fromObject.data.BalancesVal {
		k := fromObject.data.BalancesHash[i]
		fromObject.SetBalance(k, new(big.Int))
		s.AddBalance(to, k, v)
	}

	// transfer all timelock balances
	for i, v := range fromObject.data.TimeLockBalancesVal {
		k := fromObject.data.TimeLockBalancesHash[i]
		fromObject.SetTimeLockBalance(k, new(common.TimeLock))
		s.AddTimeLockBalance(to, k, v, timestamp)
	}
}

//
// Setting, updating & deleting state object methods.
//

// updateStateObject writes the given object to the trie.
func (s *StateDB) updateStateObject(obj *stateObject) {
	addr := obj.Address()
	data, err := rlp.EncodeToBytes(obj)
	if err != nil {
		panic(fmt.Errorf("can't encode object at %x: %v", addr[:], err))
	}
	if err = s.trie.TryUpdate(addr[:], data); err != nil {
		s.setError(fmt.Errorf("updateStateObject (%x) error: %v", addr[:], err))
	}
}

// deleteStateObject removes the given object from the state trie.
func (s *StateDB) deleteStateObject(obj *stateObject) {
	// Delete the account from the trie
	addr := obj.Address()
	if err := s.trie.TryDelete(addr[:]); err != nil {
		s.setError(fmt.Errorf("deleteStateObject (%x) error: %v", addr[:], err))
	}
}

// getStateObject retrieves a state object given by the address, returning nil if
// the object is not found or was deleted in this execution context. If you need
// to differentiate between non-existent/just-deleted, use getDeletedStateObject.
func (s *StateDB) getStateObject(addr common.Address) *stateObject {
	if obj := s.getDeletedStateObject(addr); obj != nil && !obj.deleted {
		return obj
	}
	return nil
}

// getDeletedStateObject is similar to getStateObject, but instead of returning
// nil for a deleted state object, it returns the actual object with the deleted
// flag set. This is needed by the state journal to revert to the correct s-
// destructed object instead of wiping all knowledge about the state object.
func (s *StateDB) getDeletedStateObject(addr common.Address) *stateObject {
	// Prefer live objects if any is available
	if obj := s.stateObjects[addr]; obj != nil {
		return obj
	}

	var (
		data *Account
		err  error
	)
	// Load the object from the database.
	enc, err := s.trie.TryGet(addr.Bytes())
	if err != nil {
		s.setError(fmt.Errorf("getDeleteStateObject (%x) error: %v", addr.Bytes(), err))
		return nil
	}
	if len(enc) == 0 {
		return nil
	}
	data = new(Account)
	if err := rlp.DecodeBytes(enc, data); err != nil {
		log.Error("Failed to decode state object", "addr", addr, "err", err)
		return nil
	}
	// Insert into the live set.
	obj := newObject(s, addr, *data)
	s.setStateObject(obj)
	return obj
}

func (s *StateDB) setStateObject(object *stateObject) {
	s.stateObjects[object.Address()] = object
}

// GetOrNewStateObject retrieves a state object or create a new state object if nil.
func (s *StateDB) GetOrNewStateObject(addr common.Address) *stateObject {
	stateObject := s.getStateObject(addr)
	if stateObject == nil {
		stateObject, _ = s.createObject(addr)
	}
	return stateObject
}

// createObject creates a new state object. If there is an existing account with
// the given address, it is overwritten and returned as the second return value.
func (s *StateDB) createObject(addr common.Address) (newobj, prev *stateObject) {
	prev = s.getDeletedStateObject(addr) // Note, prev might have been deleted, we need that!

	newobj = newObject(s, addr, Account{})
	newobj.setNonce(0) // sets the object to dirty
	if prev == nil {
		s.journal.append(createObjectChange{account: &addr})
	} else {
		s.journal.append(resetObjectChange{prev: prev})
	}
	s.setStateObject(newobj)
	if prev != nil && !prev.deleted {
		return newobj, prev
	}
	return newobj, nil
}

// CreateAccount explicitly creates a state object. If a state object with the address
// already exists the balance is carried over to the new account.
//
// CreateAccount is called during the EVM CREATE operation. The situation might arise that
// a contract does the following:
//
//  1. sends funds to sha(account ++ (nonce + 1))
//  2. tx_create(sha(account ++ nonce)) (note that this gets the address of 1)
//
// Carrying over the balance ensures that Ether doesn't disappear.
func (s *StateDB) CreateAccount(addr common.Address) {
	newObj, prev := s.createObject(addr)
	if prev != nil {
		for i, v := range prev.data.BalancesVal {
			newObj.setBalance(prev.data.BalancesHash[i], v)
		}
		for i, v := range prev.data.TimeLockBalancesVal {
			newObj.setTimeLockBalance(prev.data.TimeLockBalancesHash[i], v)
		}
	}
}

func (s *StateDB) ForEachStorage(addr common.Address, cb func(key, value common.Hash) bool) {
	so := s.getStateObject(addr)
	if so == nil {
		return
	}
	it := trie.NewIterator(so.getTrie(s.db).NodeIterator(nil))
	for it.Next() {
		key := common.BytesToHash(s.trie.GetKey(it.Key))
		if value, dirty := so.dirtyStorage[key]; dirty {
			cb(key, value)
			continue
		}
		cb(key, common.BytesToHash(it.Value))
	}
}

// Copy creates a deep, independent copy of the state.
// Snapshots of the copied state cannot be applied to the copy.
func (s *StateDB) Copy() *StateDB {
	// Copy all the basic fields, initialize the memory ones
	state := &StateDB{
		db:                  s.db,
		trie:                s.db.CopyTrie(s.trie),
		stateObjects:        make(map[common.Address]*stateObject, len(s.journal.dirties)),
		stateObjectsPending: make(map[common.Address]struct{}, len(s.stateObjectsPending)),
		stateObjectsDirty:   make(map[common.Address]struct{}, len(s.journal.dirties)),
		refund:              s.refund,
		logs:                make(map[common.Hash][]*types.Log, len(s.logs)),
		logSize:             s.logSize,
		preimages:           make(map[common.Hash][]byte, len(s.preimages)),
		journal:             newJournal(),
		ticketsHash:         s.ticketsHash,
		tickets:             s.tickets.DeepCopy(),
	}
	// Copy the dirty states, logs, and preimages
	for addr := range s.journal.dirties {
		// As documented [here](https://github.com/FusionFoundation/efsn/pull/16485#issuecomment-380438527),
		// and in the Finalise-method, there is a case where an object is in the journal but not
		// in the stateObjects: OOG after touch on ripeMD prior to Byzantium. Thus, we need to check for
		// nil
		if object, exist := s.stateObjects[addr]; exist {
			// Even though the original object is dirty, we are not copying the journal,
			// so we need to make sure that anyside effect the journal would have caused
			// during a commit (or similar op) is already applied to the copy.
			state.stateObjects[addr] = object.deepCopy(state)

			state.stateObjectsDirty[addr] = struct{}{}   // Mark the copy dirty to force internal (code/state) commits
			state.stateObjectsPending[addr] = struct{}{} // Mark the copy pending to force external (account) commits
		}
	}
	// Above, we don't copy the actual journal. This means that if the copy is copied, the
	// loop above will be a no-op, since the copy's journal is empty.
	// Thus, here we iterate over stateObjects, to enable copies of copies
	for addr := range s.stateObjectsPending {
		if _, exist := state.stateObjects[addr]; !exist {
			state.stateObjects[addr] = s.stateObjects[addr].deepCopy(state)
		}
		state.stateObjectsPending[addr] = struct{}{}
	}
	for addr := range s.stateObjectsDirty {
		if _, exist := state.stateObjects[addr]; !exist {
			state.stateObjects[addr] = s.stateObjects[addr].deepCopy(state)
		}
		state.stateObjectsDirty[addr] = struct{}{}
	}
	for hash, logs := range s.logs {
		cpy := make([]*types.Log, len(logs))
		for i, l := range logs {
			cpy[i] = new(types.Log)
			*cpy[i] = *l
		}
		state.logs[hash] = cpy
	}
	for hash, preimage := range s.preimages {
		state.preimages[hash] = preimage
	}
	// Do we need to copy the access list? In practice: No. At the start of a
	// transaction, the access list is empty. In practice, we only ever copy state
	// _between_ transactions/blocks, never in the middle of a transaction.
	// However, it doesn't cost us much to copy an empty list, so we do it anyway
	// to not blow up if we ever decide copy it in the middle of a transaction
	state.accessList = s.accessList.Copy()

	return state
}

// Snapshot returns an identifier for the current revision of the state.
func (s *StateDB) Snapshot() int {
	id := s.nextRevisionId
	s.nextRevisionId++
	s.validRevisions = append(s.validRevisions, revision{id, s.journal.length()})
	return id
}

// RevertToSnapshot reverts all state changes made since the given revision.
func (s *StateDB) RevertToSnapshot(revid int) {
	// Find the snapshot in the stack of valid snapshots.
	idx := sort.Search(len(s.validRevisions), func(i int) bool {
		return s.validRevisions[i].id >= revid
	})
	if idx == len(s.validRevisions) || s.validRevisions[idx].id != revid {
		panic(fmt.Errorf("revision id %v cannot be reverted", revid))
	}
	snapshot := s.validRevisions[idx].journalIndex

	// Replay the journal to undo changes and remove invalidated snapshots
	s.journal.revert(s, snapshot)
	s.validRevisions = s.validRevisions[:idx]
}

// GetRefund returns the current value of the refund counter.
func (s *StateDB) GetRefund() uint64 {
	return s.refund
}

// Finalise finalises the state by removing the s destructed objects and clears
// the journal as well as the refunds. Finalise, however, will not push any updates
// into the tries just yet. Only IntermediateRoot or Commit will do that.
func (s *StateDB) Finalise(deleteEmptyObjects bool) {
	for addr := range s.journal.dirties {
		obj, exist := s.stateObjects[addr]
		if !exist {
			// ripeMD is 'touched' at block 1714175, in tx 0x1237f737031e40bcde4a8b7e717b2d15e3ecadfe49bb1bbc71ee9deb09c6fcf2
			// That tx goes out of gas, and although the notion of 'touched' does not exist there, the
			// touch-event will still be recorded in the journal. Since ripeMD is a special snowflake,
			// it will persist in the journal even though the journal is reverted. In this special circumstance,
			// it may exist in `s.journal.dirties` but not in `s.stateObjects`.
			// Thus, we can safely ignore it here
			continue
		}

		if obj.suicided || (deleteEmptyObjects && obj.empty()) {
			obj.deleted = true
		} else {
			obj.finalise()
		}
		s.stateObjectsPending[addr] = struct{}{}
		s.stateObjectsDirty[addr] = struct{}{}
	}
	// Invalidate journal because reverting across transactions is not allowed.
	s.clearJournalAndRefund()
}

// IntermediateRoot computes the current root hash of the state trie.
// It is called in between transactions to get the root hash that
// goes into transaction receipts.
func (s *StateDB) IntermediateRoot(deleteEmptyObjects bool) common.Hash {
	// Finalise all the dirty storage states and write them into the tries
	s.Finalise(deleteEmptyObjects)

	// Although naively it makes sense to retrieve the account trie and then do
	// the contract storage and account updates sequentially, that short circuits
	// the account prefetcher. Instead, let's process all the storage updates
	// first, giving the account prefeches just a few more milliseconds of time
	// to pull useful data from disk.
	for addr := range s.stateObjectsPending {
		if obj := s.stateObjects[addr]; !obj.deleted {
			obj.updateRoot(s.db)
		}
	}

	for addr := range s.stateObjectsPending {
		if obj := s.stateObjects[addr]; obj.deleted {
			s.deleteStateObject(obj)
		} else {
			s.updateStateObject(obj)
		}
	}
	if len(s.stateObjectsPending) > 0 {
		s.stateObjectsPending = make(map[common.Address]struct{})
	}

	return s.trie.Hash()
}

// Prepare sets the current transaction hash and index and block hash which is
// used when the EVM emits new state logs.
func (s *StateDB) Prepare(thash common.Hash, ti int) {
	s.thash = thash
	s.txIndex = ti
	s.accessList = newAccessList()
}

func (s *StateDB) clearJournalAndRefund() {
	if len(s.journal.entries) > 0 {
		s.journal = newJournal()
		s.refund = 0
	}
	s.validRevisions = s.validRevisions[:0] // Snapshots can be created without journal entires
}

// Commit writes the state to the underlying in-memory trie database.
func (s *StateDB) Commit(deleteEmptyObjects bool) (common.Hash, error) {
	if s.dbErr != nil {
		return common.Hash{}, fmt.Errorf("commit aborted due to earlier error: %v", s.dbErr)
	}
	// Finalize any pending changes and merge everything into the tries
	s.IntermediateRoot(deleteEmptyObjects)

	// Commit objects to the trie, measuring the elapsed time
	codeWriter := s.db.TrieDB().DiskDB().NewBatch()
	for addr := range s.stateObjectsDirty {
		if obj := s.stateObjects[addr]; !obj.deleted {
			// Write any contract code associated with the state object
			if obj.code != nil && obj.dirtyCode {
				// Fusion Special Logic, ticket data stored in Contract Code area
				if addr == common.TicketKeyAddress {
					s.db.TrieDB().InsertBlob(common.BytesToHash(obj.CodeHash()), obj.code)
				} else {
					rawdb.WriteCode(codeWriter, common.BytesToHash(obj.CodeHash()), obj.code)
				}
				obj.dirtyCode = false
			}
			// Write any storage changes in the state object to its storage trie
			if err := obj.CommitTrie(s.db); err != nil {
				return common.Hash{}, err
			}
		}
	}
	if len(s.stateObjectsDirty) > 0 {
		s.stateObjectsDirty = make(map[common.Address]struct{})
	}
	if codeWriter.ValueSize() > 0 {
		if err := codeWriter.Write(); err != nil {
			log.Crit("Failed to commit dirty codes", "error", err)
		}
	}
	// The onleaf func is called _serially_, so we can reuse the same account
	// for unmarshalling every time.
	var account Account
	root, err := s.trie.Commit(func(_ [][]byte, _ []byte, leaf []byte, parent common.Hash) error {
		if err := rlp.DecodeBytes(leaf, &account); err != nil {
			return nil
		}
		if account.Root != emptyRoot {
			s.db.TrieDB().Reference(account.Root, parent)
		}
		code := common.BytesToHash(account.CodeHash)
		if code != emptyCode {
			s.db.TrieDB().Reference(code, parent)
		}
		return nil
	})
	return root, err
}

// PrepareAccessList handles the preparatory steps for executing a state transition with
// regards to both EIP-2929 and EIP-2930:
//
// - Add sender to access list (2929)
// - Add destination to access list (2929)
// - Add precompiles to access list (2929)
// - Add the contents of the optional tx access list (2930)
//
// This method should only be called if Berlin/2929+2930 is applicable at the current number.
func (s *StateDB) PrepareAccessList(sender common.Address, dst *common.Address, precompiles []common.Address, list types.AccessList) {
	s.AddAddressToAccessList(sender)
	if dst != nil {
		s.AddAddressToAccessList(*dst)
		// If it's a create-tx, the destination will be added inside evm.create
	}
	for _, addr := range precompiles {
		s.AddAddressToAccessList(addr)
	}
	for _, el := range list {
		s.AddAddressToAccessList(el.Address)
		for _, key := range el.StorageKeys {
			s.AddSlotToAccessList(el.Address, key)
		}
	}
}

// AddAddressToAccessList adds the given address to the access list
func (s *StateDB) AddAddressToAccessList(addr common.Address) {
	if s.accessList.AddAddress(addr) {
		s.journal.append(accessListAddAccountChange{&addr})
	}
}

// AddSlotToAccessList adds the given (address, slot)-tuple to the access list
func (s *StateDB) AddSlotToAccessList(addr common.Address, slot common.Hash) {
	addrMod, slotMod := s.accessList.AddSlot(addr, slot)
	if addrMod {
		// In practice, this should not happen, since there is no way to enter the
		// scope of 'address' without having the 'address' become already added
		// to the access list (via call-variant, create, etc).
		// Better safe than sorry, though
		s.journal.append(accessListAddAccountChange{&addr})
	}
	if slotMod {
		s.journal.append(accessListAddSlotChange{
			address: &addr,
			slot:    &slot,
		})
	}
}

// AddressInAccessList returns true if the given address is in the access list.
func (s *StateDB) AddressInAccessList(addr common.Address) bool {
	return s.accessList.ContainsAddress(addr)
}

// SlotInAccessList returns true if the given (address, slot)-tuple is in the access list.
func (s *StateDB) SlotInAccessList(addr common.Address, slot common.Hash) (addressPresent bool, slotPresent bool) {
	return s.accessList.Contains(addr, slot)
}

// GetNotation wacom
func (s *StateDB) GetNotation(addr common.Address) uint64 {
	stateObject := s.getStateObject(addr)
	if stateObject != nil {
		return stateObject.Notation()
	}
	return 0
}

// AllNotation wacom
func (s *StateDB) AllNotation() ([]common.Address, error) {
	return nil, fmt.Errorf("AllNotations has been depreciated please use api.fusionnetwork.io")
}

// GenNotation wacom
func (s *StateDB) GenNotation(addr common.Address) error {
	stateObject := s.GetOrNewStateObject(addr)
	if stateObject != nil {
		if n := s.GetNotation(addr); n != 0 {
			return fmt.Errorf("Account %s has a notation:%d", addr.String(), n)
		}
		// get last notation value
		nextNotation, err := s.GetNotationCount()
		nextNotation++
		if err != nil {
			log.Error("GenNotation: Unable to get next notation value")
			return err
		}
		newNotation := s.CalcNotationDisplay(nextNotation)
		s.setNotationCount(nextNotation)
		s.setNotationToAddressLookup(newNotation, addr)
		stateObject.SetNotation(newNotation)
		return nil
	}
	return nil
}

func (s *StateDB) BurnNotation(addr common.Address) {
	stateObject := s.getStateObject(addr)
	if stateObject != nil {
		notation := stateObject.Notation()
		if notation != 0 {
			s.setNotationToAddressLookup(notation, common.Address{})
			stateObject.SetNotation(0)
		}
	}
}

type notationPersist struct {
	Deleted bool
	Count   uint64
	Address common.Address
}

func (s *StateDB) GetNotationCount() (uint64, error) {
	data := s.GetStructData(common.NotationKeyAddress, common.NotationKeyAddress.Bytes())
	if len(data) == 0 || data == nil {
		return 0, nil // not created yet
	}
	var np notationPersist
	rlp.DecodeBytes(data, &np)
	return np.Count, nil
}

func (s *StateDB) setNotationCount(newCount uint64) error {
	np := notationPersist{
		Count: newCount,
	}
	data, err := rlp.EncodeToBytes(&np)
	if err != nil {
		return err
	}
	s.SetStructData(common.NotationKeyAddress, common.NotationKeyAddress.Bytes(), data)
	return nil
}

func (s *StateDB) setNotationToAddressLookup(notation uint64, address common.Address) error {
	np := notationPersist{
		Count:   notation,
		Address: address,
	}
	data, err := rlp.EncodeToBytes(&np)
	if err != nil {
		return err
	}
	buf := make([]byte, binary.MaxVarintLen64)
	binary.PutUvarint(buf, notation)
	s.SetStructData(common.NotationKeyAddress, buf, data)
	return nil
}

// GetAddressByNotation wacom
func (s *StateDB) GetAddressByNotation(notation uint64) (common.Address, error) {
	buf := make([]byte, binary.MaxVarintLen64)
	binary.PutUvarint(buf, notation)
	data := s.GetStructData(common.NotationKeyAddress, buf)
	if len(data) == 0 || data == nil {
		return common.Address{}, fmt.Errorf("notation %v does not exist", notation)
	}
	var np notationPersist
	err := rlp.DecodeBytes(data, &np)
	if err != nil {
		return common.Address{}, err
	}
	if np.Deleted || np.Address == (common.Address{}) {
		return common.Address{}, fmt.Errorf("notation was deleted")
	}
	return np.Address, nil
}

// TransferNotation wacom
func (s *StateDB) TransferNotation(notation uint64, from common.Address, to common.Address) error {
	stateObjectFrom := s.GetOrNewStateObject(from)
	if stateObjectFrom == nil {
		return fmt.Errorf("Unable to get from address")
	}
	stateObjectTo := s.GetOrNewStateObject(to)
	if stateObjectTo == nil {
		return fmt.Errorf("Unable to get to address")
	}
	address, err := s.GetAddressByNotation(notation)
	if err != nil {
		return err
	}
	if address != from {
		return fmt.Errorf("This notation is not the from address")
	}
	// reset the notation
	oldNotationTo := stateObjectTo.Notation()
	if oldNotationTo != 0 {
		// need to clear notation to address
		// user should transfer an old notation or can burn it like this
		s.setNotationToAddressLookup(oldNotationTo, common.Address{})
	}
	s.setNotationToAddressLookup(notation, to)
	stateObjectTo.SetNotation(notation)
	stateObjectFrom.SetNotation(0)
	return nil
}

// CalcNotationDisplay wacom
func (s *StateDB) CalcNotationDisplay(notation uint64) uint64 {
	if notation == 0 {
		return notation
	}
	check := (notation ^ 8192 ^ 13 + 73/76798669*708583737978) % 100
	return (notation*100 + check)
}

// AllAssets wacom
func (s *StateDB) AllAssets() (map[common.Hash]common.Asset, error) {
	return nil, fmt.Errorf("All assets has been depreciated, use api.fusionnetwork.io")
}

type assetPersist struct {
	Deleted bool // if true swap was recalled and should not be returned
	Asset   common.Asset
}

// GetAsset wacom
func (s *StateDB) GetAsset(assetID common.Hash) (common.Asset, error) {
	data := s.GetStructData(common.AssetKeyAddress, assetID.Bytes())
	var asset assetPersist
	if len(data) == 0 || data == nil {
		return common.Asset{}, fmt.Errorf("asset not found")
	}
	rlp.DecodeBytes(data, &asset)
	if asset.Deleted {
		return common.Asset{}, fmt.Errorf("asset deleted")
	}
	return asset.Asset, nil
}

// GenAsset wacom
func (s *StateDB) GenAsset(asset common.Asset) error {
	_, err := s.GetAsset(asset.ID)
	if err == nil {
		return fmt.Errorf("%s asset exists", asset.ID.String())
	}
	assetToSave := assetPersist{
		Deleted: false,
		Asset:   asset,
	}
	data, err := rlp.EncodeToBytes(&assetToSave)
	if err != nil {
		return err
	}
	s.SetStructData(common.AssetKeyAddress, asset.ID.Bytes(), data)
	return nil
}

// UpdateAsset wacom
func (s *StateDB) UpdateAsset(asset common.Asset) error {
	/** to update a asset we just overwrite it
	 */
	assetToSave := assetPersist{
		Deleted: false,
		Asset:   asset,
	}
	data, err := rlp.EncodeToBytes(&assetToSave)
	if err != nil {
		return err
	}
	s.SetStructData(common.AssetKeyAddress, asset.ID.Bytes(), data)
	return nil
}

// IsTicketExist wacom
func (s *StateDB) IsTicketExist(id common.Hash) bool {
	tickets, err := s.AllTickets()
	if err != nil {
		log.Error("IsTicketExist unable to retrieve all tickets")
		return false
	}

	_, err = tickets.Get(id)
	return err == nil
}

// GetTicket wacom
func (s *StateDB) GetTicket(id common.Hash) (*common.Ticket, error) {
	tickets, err := s.AllTickets()
	if err != nil {
		log.Error("GetTicket unable to retrieve all tickets")
		return nil, fmt.Errorf("GetTicket error: %v", err)
	}
	return tickets.Get(id)
}

// AllTickets wacom
func (s *StateDB) AllTickets() (common.TicketsDataSlice, error) {
	if len(s.tickets) != 0 {
		return s.tickets, nil
	}

	key := s.ticketsHash
	ts := cachedTicketSlice.Get(key)
	if ts != nil {
		s.tickets = ts.DeepCopy()
		return s.tickets, nil
	}

	s.rwlock.RLock()
	defer s.rwlock.RUnlock()

	blob := s.GetData(common.TicketKeyAddress)
	if len(blob) == 0 {
		return common.TicketsDataSlice{}, s.Error()
	}

	gz, err := gzip.NewReader(bytes.NewBuffer(blob))
	if err != nil {
		return nil, fmt.Errorf("Read tickets zip data: %v", err)
	}
	var buf bytes.Buffer
	if _, err = io.Copy(&buf, gz); err != nil {
		return nil, fmt.Errorf("Copy tickets zip data: %v", err)
	}
	if err := gz.Close(); err != nil {
		return nil, fmt.Errorf("Close read zip tickets: %v", err)
	}
	data := buf.Bytes()

	var tickets common.TicketsDataSlice
	if err := rlp.DecodeBytes(data, &tickets); err != nil {
		log.Error("Unable to decode tickets")
		return nil, fmt.Errorf("Unable to decode tickets, err: %v", err)
	}
	s.tickets = tickets
	cachedTicketSlice.Add(key, s.tickets)
	return s.tickets, nil
}

// AddTicket wacom
func (s *StateDB) AddTicket(ticket common.Ticket) error {
	tickets, err := s.AllTickets()
	if err != nil {
		return fmt.Errorf("AddTicket error: %v", err)
	}
	tickets, err = tickets.AddTicket(&ticket)
	if err != nil {
		return fmt.Errorf("AddTicket error: %v", err)
	}
	s.tickets = tickets
	return nil
}

// RemoveTicket wacom
func (s *StateDB) RemoveTicket(id common.Hash) error {
	tickets, err := s.AllTickets()
	if err != nil {
		return fmt.Errorf("RemoveTicket error: %v", err)
	}
	tickets, err = tickets.RemoveTicket(id)
	if err != nil {
		return fmt.Errorf("RemoveTicket error: %v", err)
	}
	s.tickets = tickets
	return nil
}

func (s *StateDB) TotalNumberOfTickets() uint64 {
	s.rwlock.RLock()
	defer s.rwlock.RUnlock()

	return s.tickets.NumberOfTickets()
}

func (s *StateDB) UpdateTickets(blockNumber *big.Int, timestamp uint64) (common.Hash, error) {
	s.rwlock.Lock()
	defer s.rwlock.Unlock()

	tickets := s.tickets
	tickets, err := tickets.ClearExpiredTickets(timestamp)
	if err != nil {
		return common.Hash{}, fmt.Errorf("UpdateTickets: %v", err)
	}
	s.tickets = tickets

	data, err := calcTicketsStorageData(s.tickets)
	if err != nil {
		return common.Hash{}, fmt.Errorf("UpdateTickets: %v", err)
	}

	hash := s.SetData(common.TicketKeyAddress, data)
	cachedTicketSlice.Add(hash, s.tickets)
	return hash, nil
}

func (s *StateDB) ClearTickets(from, to common.Address, timestamp uint64) {
	tickets, err := s.AllTickets()
	if err != nil {
		return
	}
	for i, v := range tickets {
		if v.Owner != from {
			continue
		}
		for _, ticket := range v.Tickets {
			if ticket.ExpireTime <= timestamp {
				continue
			}
			value := common.NewTimeLock(&common.TimeLockItem{
				StartTime: ticket.StartTime,
				EndTime:   ticket.ExpireTime,
				Value:     ticket.Value(),
			})
			s.AddTimeLockBalance(to, common.SystemAssetID, value, timestamp)
		}
		tickets = append(tickets[:i], tickets[i+1:]...)
		s.tickets = tickets
		break
	}
}

// AllSwaps wacom
func (s *StateDB) AllSwaps() (map[common.Hash]common.Swap, error) {
	return nil, fmt.Errorf("AllSwaps has been depreciated please use api.fusionnetwork.io")
}

/** swaps
*
 */
type swapPersist struct {
	Deleted bool // if true swap was recalled and should not be returned
	Swap    common.Swap
}

// GetSwap wacom
func (s *StateDB) GetSwap(swapID common.Hash) (common.Swap, error) {
	data := s.GetStructData(common.SwapKeyAddress, swapID.Bytes())
	var swap swapPersist
	if len(data) == 0 || data == nil {
		return common.Swap{}, fmt.Errorf("swap not found")
	}
	rlp.DecodeBytes(data, &swap)
	if swap.Deleted {
		return common.Swap{}, fmt.Errorf("swap deleted")
	}
	return swap.Swap, nil
}

// AddSwap wacom
func (s *StateDB) AddSwap(swap common.Swap) error {
	_, err := s.GetSwap(swap.ID)
	if err == nil {
		return fmt.Errorf("%s Swap exists", swap.ID.String())
	}
	swapToSave := swapPersist{
		Deleted: false,
		Swap:    swap,
	}
	data, err := rlp.EncodeToBytes(&swapToSave)
	if err != nil {
		return err
	}
	s.SetStructData(common.SwapKeyAddress, swap.ID.Bytes(), data)
	return nil
}

// UpdateSwap wacom
func (s *StateDB) UpdateSwap(swap common.Swap) error {
	/** to update a swap we just overwrite it
	 */
	swapToSave := swapPersist{
		Deleted: false,
		Swap:    swap,
	}
	data, err := rlp.EncodeToBytes(&swapToSave)
	if err != nil {
		return err
	}
	s.SetStructData(common.SwapKeyAddress, swap.ID.Bytes(), data)
	return nil
}

// RemoveSwap wacom
func (s *StateDB) RemoveSwap(id common.Hash) error {
	swapFound, err := s.GetSwap(id)
	if err != nil {
		return fmt.Errorf("%s Swap not found ", id.String())
	}

	swapToSave := swapPersist{
		Deleted: true,
		Swap:    swapFound,
	}
	data, err := rlp.EncodeToBytes(&swapToSave)
	if err != nil {
		return err
	}
	s.SetStructData(common.SwapKeyAddress, id.Bytes(), data)
	return nil
}

/** swaps
*
 */
type multiSwapPersist struct {
	Deleted bool // if true swap was recalled and should not be returned
	Swap    common.MultiSwap
}

// GetMultiSwap wacom
func (s *StateDB) GetMultiSwap(swapID common.Hash) (common.MultiSwap, error) {
	data := s.GetStructData(common.MultiSwapKeyAddress, swapID.Bytes())
	var swap multiSwapPersist
	if len(data) == 0 || data == nil {
		return common.MultiSwap{}, fmt.Errorf("multi swap not found")
	}
	rlp.DecodeBytes(data, &swap)
	if swap.Deleted {
		return common.MultiSwap{}, fmt.Errorf("multi swap deleted")
	}
	return swap.Swap, nil
}

// AddMultiSwap wacom
func (s *StateDB) AddMultiSwap(swap common.MultiSwap) error {
	_, err := s.GetMultiSwap(swap.ID)
	if err == nil {
		return fmt.Errorf("%s Multi Swap exists", swap.ID.String())
	}
	swapToSave := multiSwapPersist{
		Deleted: false,
		Swap:    swap,
	}
	data, err := rlp.EncodeToBytes(&swapToSave)
	if err != nil {
		return err
	}
	s.SetStructData(common.MultiSwapKeyAddress, swap.ID.Bytes(), data)
	return nil
}

// UpdateMultiSwap wacom
func (s *StateDB) UpdateMultiSwap(swap common.MultiSwap) error {
	/** to update a swap we just overwrite it
	 */
	swapToSave := multiSwapPersist{
		Deleted: false,
		Swap:    swap,
	}
	data, err := rlp.EncodeToBytes(&swapToSave)
	if err != nil {
		return err
	}
	s.SetStructData(common.MultiSwapKeyAddress, swap.ID.Bytes(), data)
	return nil
}

// RemoveSwap wacom
func (s *StateDB) RemoveMultiSwap(id common.Hash) error {
	swapFound, err := s.GetMultiSwap(id)
	if err != nil {
		return fmt.Errorf("%s Multi Swap not found ", id.String())
	}

	swapToSave := multiSwapPersist{
		Deleted: true,
		Swap:    swapFound,
	}
	data, err := rlp.EncodeToBytes(&swapToSave)
	if err != nil {
		return err
	}
	s.SetStructData(common.MultiSwapKeyAddress, id.Bytes(), data)
	return nil
}

/** ReportIllegal
 */

// GetReport wacom
func (s *StateDB) IsReportExist(report []byte) bool {
	hash := crypto.Keccak256Hash(report)
	data := s.GetStructData(common.ReportKeyAddress, hash.Bytes())
	return len(data) > 0
}

// AddReport wacom
func (s *StateDB) AddReport(report []byte) error {
	if s.IsReportExist(report) {
		return fmt.Errorf("AddReport error: report exists")
	}
	hash := crypto.Keccak256Hash(report)
	s.SetStructData(common.ReportKeyAddress, hash.Bytes(), report)
	return nil
}

// GetStructData wacom
func (s *StateDB) GetStructData(addr common.Address, key []byte) []byte {
	if key == nil {
		return nil
	}
	stateObject := s.GetOrNewStateObject(addr)
	if stateObject != nil {
		keyHash := crypto.Keccak256Hash(key)
		keyIndex := new(big.Int)
		keyIndex.SetBytes(keyHash[:])
		info := stateObject.GetState(s.db, keyHash)
		size := common.BytesToInt(info[0:4])
		length := common.BytesToInt(info[common.HashLength/2 : common.HashLength/2+4])
		data := make([]byte, size)
		for i := 0; i < length; i++ {
			tempIndex := big.NewInt(int64(i))
			tempKey := crypto.Keccak256Hash(tempIndex.Bytes(), keyIndex.Bytes())
			tempData := stateObject.GetState(s.db, tempKey)
			start := i * common.HashLength
			end := start + common.HashLength
			if end > size {
				end = size
			}
			copy(data[start:end], tempData[common.HashLength-end+start:])
		}
		return data
	}

	return nil
}

// SetStructData wacom
func (s *StateDB) SetStructData(addr common.Address, key, value []byte) {
	if key == nil || value == nil {
		return
	}
	stateObject := s.GetOrNewStateObject(addr)
	if stateObject != nil {
		size := len(value)
		length := size / common.HashLength
		if size%common.HashLength != 0 {
			length++
		}
		info := common.Hash{}
		copy(info[0:], common.IntToBytes(size))
		copy(info[common.HashLength/2:], common.IntToBytes(length))
		keyHash := crypto.Keccak256Hash(key)
		keyIndex := new(big.Int)
		keyIndex.SetBytes(keyHash[:])
		stateObject.SetState(s.db, keyHash, info)
		for i := 0; i < length; i++ {
			tempIndex := big.NewInt(int64(i))
			tempKey := crypto.Keccak256Hash(tempIndex.Bytes(), keyIndex.Bytes())
			tempData := common.Hash{}
			start := i * common.HashLength
			end := start + common.HashLength
			if end > size {
				end = size
			}
			tempData.SetBytes(value[start:end])
			stateObject.SetState(s.db, tempKey, tempData)
		}
		stateObject.SetNonce(stateObject.Nonce() + 1)
	}
}
