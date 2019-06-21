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
	"io"
	"math/big"
	"sort"
	"sync"

	"github.com/FusionFoundation/efsn/common"
	"github.com/FusionFoundation/efsn/core/types"
	"github.com/FusionFoundation/efsn/crypto"
	"github.com/FusionFoundation/efsn/log"
	"github.com/FusionFoundation/efsn/rlp"
	"github.com/FusionFoundation/efsn/trie"
)

type revision struct {
	id           int
	journalIndex int
}

var (
	// emptyState is the known hash of an empty state trie entry.
	emptyState = crypto.Keccak256Hash(nil)

	// emptyCode is the known hash of the empty EVM bytecode.
	emptyCode = crypto.Keccak256Hash(nil)
)

// StateDBs within the ethereum protocol are used to store anything
// within the merkle trie. StateDBs take care of caching and storing
// nested states. It's the general query interface to retrieve:
// * Contracts
// * Accounts
type StateDB struct {
	db   Database
	trie Trie

	// This map holds 'live' objects, which will get modified while processing a state transition.
	stateObjects      map[common.Address]*stateObject
	stateObjectsDirty map[common.Address]struct{}

	// DB error.
	// State objects are used by the consensus core and VM which are
	// unable to deal with database-level errors. Any error that occurs
	// during a database read is memoized here and will eventually be returned
	// by StateDB.Commit.
	dbErr error

	// The refund counter, also used by state transitioning.
	refund uint64

	thash, bhash common.Hash
	txIndex      int
	logs         map[common.Hash][]*types.Log
	logSize      uint

	preimages map[common.Hash][]byte

	// Journal of state modifications. This is the backbone of
	// Snapshot and RevertToSnapshot.
	journal        *journal
	validRevisions []revision
	nextRevisionId int

	ticketsHash common.Hash
	tickets     common.TicketsDataSlice

	lock   sync.Mutex
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

func (cts CachedTicketSlice) Get(hash common.Hash) common.TicketsDataSlice {
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
		db:                db,
		trie:              tr,
		stateObjects:      make(map[common.Address]*stateObject),
		stateObjectsDirty: make(map[common.Address]struct{}),
		logs:              make(map[common.Hash][]*types.Log),
		preimages:         make(map[common.Hash][]byte),
		journal:           newJournal(),
		ticketsHash:       mixDigest,
		tickets:           nil,
	}
	return statedb, nil
}

// setError remembers the first non-nil error it is called with.
func (self *StateDB) setError(err error) {
	if self.dbErr == nil {
		self.dbErr = err
	}
}

func (self *StateDB) Error() error {
	return self.dbErr
}

// Reset clears out all ephemeral state objects from the state db, but keeps
// the underlying state trie to avoid reloading data for the next operations.
func (self *StateDB) Reset(root common.Hash) error {
	tr, err := self.db.OpenTrie(root)
	if err != nil {
		return err
	}
	self.trie = tr
	self.stateObjects = make(map[common.Address]*stateObject)
	self.stateObjectsDirty = make(map[common.Address]struct{})
	self.thash = common.Hash{}
	self.bhash = common.Hash{}
	self.txIndex = 0
	self.logs = make(map[common.Hash][]*types.Log)
	self.logSize = 0
	self.preimages = make(map[common.Hash][]byte)
	self.clearJournalAndRefund()
	self.tickets = nil
	return nil
}

func (self *StateDB) AddLog(log *types.Log) {
	self.journal.append(addLogChange{txhash: self.thash})

	log.TxHash = self.thash
	log.BlockHash = self.bhash
	log.TxIndex = uint(self.txIndex)
	log.Index = self.logSize
	self.logs[self.thash] = append(self.logs[self.thash], log)
	self.logSize++
}

func (self *StateDB) GetLogs(hash common.Hash) []*types.Log {
	return self.logs[hash]
}

func (self *StateDB) Logs() []*types.Log {
	var logs []*types.Log
	for _, lgs := range self.logs {
		logs = append(logs, lgs...)
	}
	return logs
}

// AddPreimage records a SHA3 preimage seen by the VM.
func (self *StateDB) AddPreimage(hash common.Hash, preimage []byte) {
	if _, ok := self.preimages[hash]; !ok {
		self.journal.append(addPreimageChange{hash: hash})
		pi := make([]byte, len(preimage))
		copy(pi, preimage)
		self.preimages[hash] = pi
	}
}

// Preimages returns a list of SHA3 preimages that have been submitted.
func (self *StateDB) Preimages() map[common.Hash][]byte {
	return self.preimages
}

// AddRefund adds gas to the refund counter
func (self *StateDB) AddRefund(gas uint64) {
	self.journal.append(refundChange{prev: self.refund})
	self.refund += gas
}

// SubRefund removes gas from the refund counter.
// This method will panic if the refund counter goes below zero
func (self *StateDB) SubRefund(gas uint64) {
	self.journal.append(refundChange{prev: self.refund})
	if gas > self.refund {
		panic("Refund counter below zero")
	}
	self.refund -= gas
}

// Exist reports whether the given account address exists in the state.
// Notably this also returns true for suicided accounts.
func (self *StateDB) Exist(addr common.Address) bool {
	return self.getStateObject(addr) != nil
}

// Empty returns whether the state object is either non-existent
// or empty according to the EIP161 specification (balance = nonce = code = 0)
func (self *StateDB) Empty(addr common.Address) bool {
	so := self.getStateObject(addr)
	return so == nil || so.empty()
}

func (self *StateDB) GetAllBalances(addr common.Address) map[common.Hash]string {
	stateObject := self.getStateObject(addr)
	if stateObject != nil {
		return stateObject.CopyBalances()
	}
	return make(map[common.Hash]string)
}

func (self *StateDB) GetBalance(assetID common.Hash, addr common.Address) *big.Int {
	stateObject := self.getStateObject(addr)
	if stateObject != nil {
		return stateObject.Balance(assetID)
	}
	return big.NewInt(0)
}

func (self *StateDB) GetAllTimeLockBalances(addr common.Address) map[common.Hash]*common.TimeLock {
	stateObject := self.getStateObject(addr)
	if stateObject != nil {
		return stateObject.CopyTimeLockBalances()
	}
	return make(map[common.Hash]*common.TimeLock)
}

func (self *StateDB) GetTimeLockBalance(assetID common.Hash, addr common.Address) *common.TimeLock {
	stateObject := self.getStateObject(addr)
	if stateObject != nil {
		return stateObject.TimeLockBalance(assetID)
	}
	return new(common.TimeLock)
}

func (self *StateDB) GetNonce(addr common.Address) uint64 {
	stateObject := self.getStateObject(addr)
	if stateObject != nil {
		return stateObject.Nonce()
	}

	return 0
}

func (self *StateDB) GetCode(addr common.Address) []byte {
	stateObject := self.getStateObject(addr)
	if stateObject != nil {
		return stateObject.Code(self.db)
	}
	return nil
}

func (self *StateDB) GetCodeSize(addr common.Address) int {
	stateObject := self.getStateObject(addr)
	if stateObject == nil {
		return 0
	}
	if stateObject.code != nil {
		return len(stateObject.code)
	}
	size, err := self.db.ContractCodeSize(stateObject.addrHash, common.BytesToHash(stateObject.CodeHash()))
	if err != nil {
		self.setError(err)
	}
	return size
}

func (self *StateDB) GetCodeHash(addr common.Address) common.Hash {
	stateObject := self.getStateObject(addr)
	if stateObject == nil {
		return common.Hash{}
	}
	return common.BytesToHash(stateObject.CodeHash())
}

// GetState retrieves a value from the given account's storage trie.
func (self *StateDB) GetState(addr common.Address, hash common.Hash) common.Hash {
	stateObject := self.getStateObject(addr)
	if stateObject != nil {
		return stateObject.GetState(self.db, hash)
	}
	return common.Hash{}
}

// GetCommittedState retrieves a value from the given account's committed storage trie.
func (self *StateDB) GetCommittedState(addr common.Address, hash common.Hash) common.Hash {
	stateObject := self.getStateObject(addr)
	if stateObject != nil {
		return stateObject.GetCommittedState(self.db, hash)
	}
	return common.Hash{}
}

func (self *StateDB) GetData(addr common.Address) []byte {
	stateObject := self.getStateObject(addr)
	if stateObject != nil {
		return stateObject.Code(self.db)
	}
	return nil
}

func (self *StateDB) GetDataHash(addr common.Address) common.Hash {
	stateObject := self.getStateObject(addr)
	if stateObject != nil {
		return common.BytesToHash(stateObject.CodeHash())
	}
	return common.Hash{}
}

// Database retrieves the low level database supporting the lower level trie ops.
func (self *StateDB) Database() Database {
	return self.db
}

// StorageTrie returns the storage trie of an account.
// The return value is a copy and is nil for non-existent accounts.
func (self *StateDB) StorageTrie(addr common.Address) Trie {
	stateObject := self.getStateObject(addr)
	if stateObject == nil {
		return nil
	}
	cpy := stateObject.deepCopy(self)
	return cpy.updateTrie(self.db)
}

func (self *StateDB) HasSuicided(addr common.Address) bool {
	stateObject := self.getStateObject(addr)
	if stateObject != nil {
		return stateObject.suicided
	}
	return false
}

/*
 * SETTERS
 */

// AddBalance adds amount to the account associated with addr.
func (self *StateDB) AddBalance(addr common.Address, assetID common.Hash, amount *big.Int) {
	stateObject := self.GetOrNewStateObject(addr)
	if stateObject != nil {
		stateObject.AddBalance(assetID, amount)
	}
}

// SubBalance subtracts amount from the account associated with addr.
func (self *StateDB) SubBalance(addr common.Address, assetID common.Hash, amount *big.Int) {
	stateObject := self.GetOrNewStateObject(addr)
	if stateObject != nil {
		stateObject.SubBalance(assetID, amount)
	}
}

func (self *StateDB) SetBalance(addr common.Address, assetID common.Hash, amount *big.Int) {
	stateObject := self.GetOrNewStateObject(addr)
	if stateObject != nil {
		stateObject.SetBalance(assetID, amount)
	}
}

func (self *StateDB) AddTimeLockBalance(addr common.Address, assetID common.Hash, amount *common.TimeLock, blockNumber *big.Int, timestamp uint64) {
	stateObject := self.GetOrNewStateObject(addr)
	if stateObject != nil {
		stateObject.AddTimeLockBalance(assetID, amount, blockNumber, timestamp)
	}
}

func (self *StateDB) SubTimeLockBalance(addr common.Address, assetID common.Hash, amount *common.TimeLock, blockNumber *big.Int, timestamp uint64) {
	stateObject := self.GetOrNewStateObject(addr)
	if stateObject != nil {
		stateObject.SubTimeLockBalance(assetID, amount, blockNumber, timestamp)
	}
}

func (self *StateDB) SetTimeLockBalance(addr common.Address, assetID common.Hash, amount *common.TimeLock) {
	stateObject := self.GetOrNewStateObject(addr)
	if stateObject != nil {
		stateObject.SetTimeLockBalance(assetID, amount)
	}
}

func (self *StateDB) SetNonce(addr common.Address, nonce uint64) {
	stateObject := self.GetOrNewStateObject(addr)
	if stateObject != nil {
		stateObject.SetNonce(nonce)
	}
}

func (self *StateDB) SetCode(addr common.Address, code []byte) {
	stateObject := self.GetOrNewStateObject(addr)
	if stateObject != nil {
		stateObject.SetCode(crypto.Keccak256Hash(code), code)
	}
}

func (self *StateDB) SetState(addr common.Address, key, value common.Hash) {
	stateObject := self.GetOrNewStateObject(addr)
	if stateObject != nil {
		stateObject.SetState(self.db, key, value)
	}
}

func (self *StateDB) SetData(addr common.Address, value []byte) common.Hash {
	stateObject := self.GetOrNewStateObject(addr)
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
func (self *StateDB) Suicide(addr common.Address) bool {
	stateObject := self.getStateObject(addr)
	if stateObject == nil {
		return false
	}
	for i, v := range stateObject.data.BalancesVal {
		k := stateObject.data.BalancesHash[i]
		self.journal.append(suicideChange{
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
		self.journal.append(suicideChange{
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

//
// Setting, updating & deleting state object methods.
//

// updateStateObject writes the given object to the trie.
func (self *StateDB) updateStateObject(stateObject *stateObject) {
	addr := stateObject.Address()
	data, err := rlp.EncodeToBytes(stateObject)
	if err != nil {
		panic(fmt.Errorf("can't encode object at %x: %v", addr[:], err))
	}
	self.setError(self.trie.TryUpdate(addr[:], data))
}

// deleteStateObject removes the given object from the state trie.
func (self *StateDB) deleteStateObject(stateObject *stateObject) {
	stateObject.deleted = true
	addr := stateObject.Address()
	self.setError(self.trie.TryDelete(addr[:]))
}

// Retrieve a state object given by the address. Returns nil if not found.
func (self *StateDB) getStateObject(addr common.Address) (stateObject *stateObject) {
	// Prefer 'live' objects.
	if obj := self.stateObjects[addr]; obj != nil {
		if obj.deleted {
			return nil
		}
		return obj
	}

	// Load the object from the database.
	enc, err := self.trie.TryGet(addr[:])
	if len(enc) == 0 {
		self.setError(err)
		return nil
	}
	var data Account
	if err := rlp.DecodeBytes(enc, &data); err != nil {
		log.Error("Failed to decode state object", "addr", addr, "err", err)
		return nil
	}
	// Insert into the live set.
	obj := newObject(self, addr, data)
	self.setStateObject(obj)
	return obj
}

func (self *StateDB) setStateObject(object *stateObject) {
	self.stateObjects[object.Address()] = object
}

// Retrieve a state object or create a new state object if nil.
func (self *StateDB) GetOrNewStateObject(addr common.Address) *stateObject {
	stateObject := self.getStateObject(addr)
	if stateObject == nil || stateObject.deleted {
		stateObject, _ = self.createObject(addr)
	}
	return stateObject
}

// createObject creates a new state object. If there is an existing account with
// the given address, it is overwritten and returned as the second return value.
func (self *StateDB) createObject(addr common.Address) (newobj, prev *stateObject) {
	prev = self.getStateObject(addr)
	newobj = newObject(self, addr, Account{})
	newobj.setNonce(0) // sets the object to dirty
	if prev == nil {
		self.journal.append(createObjectChange{account: &addr})
	} else {
		self.journal.append(resetObjectChange{prev: prev})
	}
	self.setStateObject(newobj)
	return newobj, prev
}

// CreateAccount explicitly creates a state object. If a state object with the address
// already exists the balance is carried over to the new account.
//
// CreateAccount is called during the EVM CREATE operation. The situation might arise that
// a contract does the following:
//
//   1. sends funds to sha(account ++ (nonce + 1))
//   2. tx_create(sha(account ++ nonce)) (note that this gets the address of 1)
//
// Carrying over the balance ensures that Ether doesn't disappear.
func (self *StateDB) CreateAccount(addr common.Address) {
	new, prev := self.createObject(addr)
	if prev != nil {
		for i, v := range prev.data.BalancesVal {
			new.setBalance(prev.data.BalancesHash[i], v)
		}
		for i, v := range prev.data.TimeLockBalancesVal {
			new.setTimeLockBalance(prev.data.TimeLockBalancesHash[i], v)
		}
	}
}

func (db *StateDB) ForEachStorage(addr common.Address, cb func(key, value common.Hash) bool) {
	so := db.getStateObject(addr)
	if so == nil {
		return
	}
	it := trie.NewIterator(so.getTrie(db.db).NodeIterator(nil))
	for it.Next() {
		key := common.BytesToHash(db.trie.GetKey(it.Key))
		if value, dirty := so.dirtyStorage[key]; dirty {
			cb(key, value)
			continue
		}
		cb(key, common.BytesToHash(it.Value))
	}
}

// Copy creates a deep, independent copy of the state.
// Snapshots of the copied state cannot be applied to the copy.
func (self *StateDB) Copy() *StateDB {
	self.lock.Lock()
	defer self.lock.Unlock()

	// Copy all the basic fields, initialize the memory ones
	state := &StateDB{
		db:                self.db,
		trie:              self.db.CopyTrie(self.trie),
		stateObjects:      make(map[common.Address]*stateObject, len(self.journal.dirties)),
		stateObjectsDirty: make(map[common.Address]struct{}, len(self.journal.dirties)),
		refund:            self.refund,
		logs:              make(map[common.Hash][]*types.Log, len(self.logs)),
		logSize:           self.logSize,
		preimages:         make(map[common.Hash][]byte),
		journal:           newJournal(),
		ticketsHash:       self.ticketsHash,
		tickets:           self.tickets.DeepCopy(),
	}
	// Copy the dirty states, logs, and preimages
	for addr := range self.journal.dirties {
		// As documented [here](https://github.com/FusionFoundation/efsn/pull/16485#issuecomment-380438527),
		// and in the Finalise-method, there is a case where an object is in the journal but not
		// in the stateObjects: OOG after touch on ripeMD prior to Byzantium. Thus, we need to check for
		// nil
		if object, exist := self.stateObjects[addr]; exist {
			state.stateObjects[addr] = object.deepCopy(state)
			state.stateObjectsDirty[addr] = struct{}{}
		}
	}
	// Above, we don't copy the actual journal. This means that if the copy is copied, the
	// loop above will be a no-op, since the copy's journal is empty.
	// Thus, here we iterate over stateObjects, to enable copies of copies
	for addr := range self.stateObjectsDirty {
		if _, exist := state.stateObjects[addr]; !exist {
			state.stateObjects[addr] = self.stateObjects[addr].deepCopy(state)
			state.stateObjectsDirty[addr] = struct{}{}
		}
	}
	for hash, logs := range self.logs {
		cpy := make([]*types.Log, len(logs))
		for i, l := range logs {
			cpy[i] = new(types.Log)
			*cpy[i] = *l
		}
		state.logs[hash] = cpy
	}
	for hash, preimage := range self.preimages {
		state.preimages[hash] = preimage
	}
	return state
}

// Snapshot returns an identifier for the current revision of the state.
func (self *StateDB) Snapshot() int {
	id := self.nextRevisionId
	self.nextRevisionId++
	self.validRevisions = append(self.validRevisions, revision{id, self.journal.length()})
	return id
}

// RevertToSnapshot reverts all state changes made since the given revision.
func (self *StateDB) RevertToSnapshot(revid int) {
	// Find the snapshot in the stack of valid snapshots.
	idx := sort.Search(len(self.validRevisions), func(i int) bool {
		return self.validRevisions[i].id >= revid
	})
	if idx == len(self.validRevisions) || self.validRevisions[idx].id != revid {
		panic(fmt.Errorf("revision id %v cannot be reverted", revid))
	}
	snapshot := self.validRevisions[idx].journalIndex

	// Replay the journal to undo changes and remove invalidated snapshots
	self.journal.revert(self, snapshot)
	self.validRevisions = self.validRevisions[:idx]
}

// GetRefund returns the current value of the refund counter.
func (self *StateDB) GetRefund() uint64 {
	return self.refund
}

// Finalise finalises the state by removing the self destructed objects
// and clears the journal as well as the refunds.
func (s *StateDB) Finalise(deleteEmptyObjects bool) {
	for addr := range s.journal.dirties {
		stateObject, exist := s.stateObjects[addr]
		if !exist {
			// ripeMD is 'touched' at block 1714175, in tx 0x1237f737031e40bcde4a8b7e717b2d15e3ecadfe49bb1bbc71ee9deb09c6fcf2
			// That tx goes out of gas, and although the notion of 'touched' does not exist there, the
			// touch-event will still be recorded in the journal. Since ripeMD is a special snowflake,
			// it will persist in the journal even though the journal is reverted. In this special circumstance,
			// it may exist in `s.journal.dirties` but not in `s.stateObjects`.
			// Thus, we can safely ignore it here
			continue
		}

		if stateObject.suicided || (deleteEmptyObjects && stateObject.empty()) {
			s.deleteStateObject(stateObject)
		} else {
			stateObject.updateRoot(s.db)
			s.updateStateObject(stateObject)
		}
		s.stateObjectsDirty[addr] = struct{}{}
	}
	// Invalidate journal because reverting across transactions is not allowed.
	s.clearJournalAndRefund()
}

// IntermediateRoot computes the current root hash of the state trie.
// It is called in between transactions to get the root hash that
// goes into transaction receipts.
func (s *StateDB) IntermediateRoot(deleteEmptyObjects bool) common.Hash {
	s.Finalise(deleteEmptyObjects)
	return s.trie.Hash()
}

// Prepare sets the current transaction hash and index and block hash which is
// used when the EVM emits new state logs.
func (self *StateDB) Prepare(thash, bhash common.Hash, ti int) {
	self.thash = thash
	self.bhash = bhash
	self.txIndex = ti
}

func (s *StateDB) clearJournalAndRefund() {
	s.journal = newJournal()
	s.validRevisions = s.validRevisions[:0]
	s.refund = 0
}

// Commit writes the state to the underlying in-memory trie database.
func (s *StateDB) Commit(deleteEmptyObjects bool) (root common.Hash, err error) {
	defer s.clearJournalAndRefund()

	for addr := range s.journal.dirties {
		s.stateObjectsDirty[addr] = struct{}{}
	}
	// Commit objects to the trie.
	for addr, stateObject := range s.stateObjects {

		_, isDirty := s.stateObjectsDirty[addr]
		switch {
		case stateObject.suicided || (isDirty && deleteEmptyObjects && stateObject.empty()):
			// If the object has been removed, don't bother syncing it
			// and just mark it for deletion in the trie.
			s.deleteStateObject(stateObject)
		case isDirty:
			// Write any contract code associated with the state object
			if stateObject.code != nil && stateObject.dirtyCode {
				// log.Info( "SAVING code object " , "addr", addr)
				s.db.TrieDB().InsertBlob(common.BytesToHash(stateObject.CodeHash()), stateObject.code)
				stateObject.dirtyCode = false
			}
			// Write any storage changes in the state object to its storage trie.
			if err := stateObject.CommitTrie(s.db); err != nil {
				return common.Hash{}, err
			}
			// Update the object in the main account trie.
			s.updateStateObject(stateObject)
		}
		delete(s.stateObjectsDirty, addr)
	}
	// Write trie changes.
	root, err = s.trie.Commit(func(leaf []byte, parent common.Hash) error {
		var account Account
		if err := rlp.DecodeBytes(leaf, &account); err != nil {
			return nil
		}
		if account.Root != emptyState {
			s.db.TrieDB().Reference(account.Root, parent)
		}
		code := common.BytesToHash(account.CodeHash)
		if code != emptyCode {
			s.db.TrieDB().Reference(code, parent)
		}
		return nil
	})
	log.Debug("Trie cache stats after commit", "misses", trie.CacheMisses(), "unloads", trie.CacheUnloads())
	return root, err
}

// GetNotation wacom
func (db *StateDB) GetNotation(addr common.Address) uint64 {
	stateObject := db.getStateObject(addr)
	if stateObject != nil {
		return stateObject.Notation()
	}
	return 0
}

// AllNotation wacom
func (db *StateDB) AllNotation() ([]common.Address, error) {
	return nil, fmt.Errorf("AllNotations has been depreciated please use api.fusionnetwork.io")
}

// GenNotation wacom
func (db *StateDB) GenNotation(addr common.Address) error {
	stateObject := db.GetOrNewStateObject(addr)
	if stateObject != nil {
		if n := db.GetNotation(addr); n != 0 {
			return fmt.Errorf("Account %s has a notation:%d", addr.String(), n)
		}
		// get last notation value
		nextNotation, err := db.getNotationCount()
		nextNotation++
		if err != nil {
			log.Error("GenNotation: Unable to get next notation value")
			return err
		}
		newNotation := db.calcNotationDisplay(nextNotation)
		db.setNotationCount(nextNotation)
		db.setNotationToAddressLookup(newNotation, addr)
		stateObject.SetNotation(newNotation)
		return nil
	}
	return nil
}

type notationPersist struct {
	Deleted bool
	Count   uint64
	Address common.Address
}

func (db *StateDB) getNotationCount() (uint64, error) {
	data := db.GetStructData(common.NotationKeyAddress, common.NotationKeyAddress.Bytes())
	if len(data) == 0 || data == nil {
		return 0, nil // not created yet
	}
	var np notationPersist
	rlp.DecodeBytes(data, &np)
	return np.Count, nil
}

func (db *StateDB) setNotationCount(newCount uint64) error {
	np := notationPersist{
		Count: newCount,
	}
	data, err := rlp.EncodeToBytes(&np)
	if err != nil {
		return err
	}
	db.SetStructData(common.NotationKeyAddress, common.NotationKeyAddress.Bytes(), data)
	return nil
}

func (db *StateDB) setNotationToAddressLookup(notation uint64, address common.Address) error {
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
	db.SetStructData(common.NotationKeyAddress, buf, data)
	return nil
}

// GetAddressByNotation wacom
func (db *StateDB) GetAddressByNotation(notation uint64) (common.Address, error) {
	buf := make([]byte, binary.MaxVarintLen64)
	binary.PutUvarint(buf, notation)
	data := db.GetStructData(common.NotationKeyAddress, buf)
	if len(data) == 0 || data == nil {
		return common.Address{}, fmt.Errorf("notation %v does not exist", notation)
	}
	var np notationPersist
	err := rlp.DecodeBytes(data, &np)
	if err != nil {
		return common.Address{}, err
	}
	if np.Deleted {
		return common.Address{}, fmt.Errorf("notation was deleted")
	}
	return np.Address, nil
}

// TransferNotation wacom
func (db *StateDB) TransferNotation(notation uint64, from common.Address, to common.Address) error {
	stateObjectFrom := db.GetOrNewStateObject(from)
	if stateObjectFrom == nil {
		return fmt.Errorf("Unable to get from address")
	}
	stateObjectTo := db.GetOrNewStateObject(to)
	if stateObjectTo == nil {
		return fmt.Errorf("Unable to get to address")
	}
	displayNotation := notation
	address, err := db.GetAddressByNotation(displayNotation)
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
		db.setNotationToAddressLookup(oldNotationTo, common.Address{})
	}
	db.setNotationToAddressLookup(displayNotation, to)
	stateObjectTo.SetNotation(notation)
	stateObjectFrom.SetNotation(0)
	return nil
}

// CalcNotationDisplay wacom
func (db *StateDB) calcNotationDisplay(notation uint64) uint64 {
	if notation == 0 {
		return notation
	}
	check := (notation ^ 8192 ^ 13 + 73/76798669*708583737978) % 100
	return (notation*100 + check)
}

// // GenNotation wacom
// func (db *StateDB) GenNotation(addr common.Address) error {
// 	stateObject := db.GetOrNewStateObject(addr)
// 	if stateObject != nil {
// 		if n := db.GetNotation(addr); n != 0 {
// 			return fmt.Errorf("Account %s has a notation:%d", addr.String(), n)
// 		}
// 		notations, err := db.AllNotation()
// 		if err != nil {
// 			log.Error("GenNotation: Unable to decode bytes in AllNotation")
// 			return err
// 		}
// 		notations = append(notations, addr)
// 		stateObject.SetNotation(uint64(len(notations)))
// 		db.notations = notations
// 		return db.updateNotations()
// 	}
// 	return nil
// }

// AllAssets wacom
func (db *StateDB) AllAssets() (map[common.Hash]common.Asset, error) {
	return nil, fmt.Errorf("All assets has been depreciated, use api.fusionnetwork.io")
}

type assetPersist struct {
	Deleted bool // if true swap was recalled and should not be returned
	Asset   common.Asset
}

// GetAsset wacom
func (db *StateDB) GetAsset(assetID common.Hash) (common.Asset, error) {
	data := db.GetStructData(common.AssetKeyAddress, assetID.Bytes())
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
func (db *StateDB) GenAsset(asset common.Asset) error {
	_, err := db.GetAsset(asset.ID)
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
	db.SetStructData(common.AssetKeyAddress, asset.ID.Bytes(), data)
	return nil
}

// UpdateAsset wacom
func (db *StateDB) UpdateAsset(asset common.Asset) error {
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
	db.SetStructData(common.AssetKeyAddress, asset.ID.Bytes(), data)
	return nil
}

// IsTicketExist wacom
func (db *StateDB) IsTicketExist(id common.Hash) bool {
	tickets, err := db.AllTickets()
	if err != nil {
		log.Error("IsTicketExist unable to retrieve all tickets")
		return false
	}

	_, err = tickets.Get(id)
	return err == nil
}

// GetTicket wacom
func (db *StateDB) GetTicket(id common.Hash) (*common.Ticket, error) {
	tickets, err := db.AllTickets()
	if err != nil {
		log.Error("GetTicket unable to retrieve all tickets")
		return nil, fmt.Errorf("GetTicket error: %v", err)
	}
	return tickets.Get(id)
}

// AllTickets wacom
func (db *StateDB) AllTickets() (common.TicketsDataSlice, error) {
	if len(db.tickets) != 0 {
		return db.tickets, nil
	}

	key := db.ticketsHash
	ts := cachedTicketSlice.Get(key)
	if ts != nil {
		db.tickets = ts.DeepCopy()
		return db.tickets, nil
	}

	db.rwlock.RLock()
	defer db.rwlock.RUnlock()

	blob := db.GetData(common.TicketKeyAddress)
	if len(blob) == 0 {
		return common.TicketsDataSlice{}, db.Error()
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
	db.tickets = tickets
	cachedTicketSlice.Add(key, db.tickets)
	return db.tickets, nil
}

// AddTicket wacom
func (db *StateDB) AddTicket(ticket common.Ticket) error {
	tickets, err := db.AllTickets()
	if err != nil {
		return fmt.Errorf("AddTicket error: %v", err)
	}
	tickets, err = tickets.AddTicket(&ticket)
	if err != nil {
		return fmt.Errorf("AddTicket error: %v", err)
	}
	db.tickets = tickets
	return nil
}

// RemoveTicket wacom
func (db *StateDB) RemoveTicket(id common.Hash) error {
	tickets, err := db.AllTickets()
	if err != nil {
		return fmt.Errorf("RemoveTicket error: %v", err)
	}
	tickets, err = tickets.RemoveTicket(id)
	if err != nil {
		return fmt.Errorf("RemoveTicket error: %v", err)
	}
	db.tickets = tickets
	return nil
}

func (db *StateDB) TotalNumberOfTickets() uint64 {
	db.rwlock.RLock()
	defer db.rwlock.RUnlock()

	return db.tickets.NumberOfTickets()
}

func (db *StateDB) UpdateTickets(blockNumber *big.Int, timestamp uint64) (common.Hash, error) {
	db.rwlock.Lock()
	defer db.rwlock.Unlock()

	tickets := db.tickets
	tickets, err := tickets.ClearExpiredTickets(timestamp)
	if err != nil {
		return common.Hash{}, fmt.Errorf("UpdateTickets: %v", err)
	}
	db.tickets = tickets

	data, err := calcTicketsStorageData(db.tickets)
	if err != nil {
		return common.Hash{}, fmt.Errorf("UpdateTickets: %v", err)
	}

	hash := db.SetData(common.TicketKeyAddress, data)
	cachedTicketSlice.Add(hash, db.tickets)
	return hash, nil
}

// AllSwaps wacom
func (db *StateDB) AllSwaps() (map[common.Hash]common.Swap, error) {
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
func (db *StateDB) GetSwap(swapID common.Hash) (common.Swap, error) {
	data := db.GetStructData(common.SwapKeyAddress, swapID.Bytes())
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
func (db *StateDB) AddSwap(swap common.Swap) error {
	_, err := db.GetSwap(swap.ID)
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
	db.SetStructData(common.SwapKeyAddress, swap.ID.Bytes(), data)
	return nil
}

// UpdateSwap wacom
func (db *StateDB) UpdateSwap(swap common.Swap) error {
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
	db.SetStructData(common.SwapKeyAddress, swap.ID.Bytes(), data)
	return nil
}

// RemoveSwap wacom
func (db *StateDB) RemoveSwap(id common.Hash) error {
	swapFound, err := db.GetSwap(id)
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
	db.SetStructData(common.SwapKeyAddress, id.Bytes(), data)
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
func (db *StateDB) GetMultiSwap(swapID common.Hash) (common.MultiSwap, error) {
	data := db.GetStructData(common.MultiSwapKeyAddress, swapID.Bytes())
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
func (db *StateDB) AddMultiSwap(swap common.MultiSwap) error {
	_, err := db.GetMultiSwap(swap.ID)
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
	db.SetStructData(common.MultiSwapKeyAddress, swap.ID.Bytes(), data)
	return nil
}

// UpdateMultiSwap wacom
func (db *StateDB) UpdateMultiSwap(swap common.MultiSwap) error {
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
	db.SetStructData(common.MultiSwapKeyAddress, swap.ID.Bytes(), data)
	return nil
}

// RemoveSwap wacom
func (db *StateDB) RemoveMultiSwap(id common.Hash) error {
	swapFound, err := db.GetMultiSwap(id)
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
	db.SetStructData(common.MultiSwapKeyAddress, id.Bytes(), data)
	return nil
}

type assetsStruct struct {
	HASH  common.Hash
	ASSET common.Asset
}

type sortableAssetLURSlice []assetsStruct

func (s sortableAssetLURSlice) Len() int {
	return len(s)
}

func (s sortableAssetLURSlice) Less(i, j int) bool {
	a, _ := new(big.Int).SetString(s[i].HASH.Hex(), 0)
	b, _ := new(big.Int).SetString(s[j].HASH.Hex(), 0)
	return a.Cmp(b) < 0
}

func (s sortableAssetLURSlice) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

// GetStructData wacom
func (db *StateDB) GetStructData(addr common.Address, key []byte) []byte {
	if key == nil {
		return nil
	}
	stateObject := db.GetOrNewStateObject(addr)
	if stateObject != nil {
		keyHash := crypto.Keccak256Hash(key)
		keyIndex := new(big.Int)
		keyIndex.SetBytes(keyHash[:])
		info := stateObject.GetState(db.db, keyHash)
		size := common.BytesToInt(info[0:4])
		length := common.BytesToInt(info[common.HashLength/2 : common.HashLength/2+4])
		data := make([]byte, size)
		for i := 0; i < length; i++ {
			tempIndex := big.NewInt(int64(i))
			tempKey := crypto.Keccak256Hash(tempIndex.Bytes(), keyIndex.Bytes())
			tempData := stateObject.GetState(db.db, tempKey)
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
func (db *StateDB) SetStructData(addr common.Address, key, value []byte) {
	if key == nil || value == nil {
		return
	}
	stateObject := db.GetOrNewStateObject(addr)
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
		stateObject.SetState(db.db, keyHash, info)
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
			stateObject.SetState(db.db, tempKey, tempData)
		}
		stateObject.SetNonce(stateObject.Nonce() + 1)
	}
}
