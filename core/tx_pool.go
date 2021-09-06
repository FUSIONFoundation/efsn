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

package core

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"math/big"
	"sort"
	"sync"
	"time"

	"github.com/FusionFoundation/efsn/common"
	"github.com/FusionFoundation/efsn/common/prque"
	"github.com/FusionFoundation/efsn/consensus/datong"
	"github.com/FusionFoundation/efsn/core/state"
	"github.com/FusionFoundation/efsn/core/types"
	"github.com/FusionFoundation/efsn/event"
	"github.com/FusionFoundation/efsn/log"
	"github.com/FusionFoundation/efsn/metrics"
	"github.com/FusionFoundation/efsn/params"
	"github.com/FusionFoundation/efsn/rlp"
)

const (
	// chainHeadChanSize is the size of channel listening to ChainHeadEvent.
	chainHeadChanSize = 10
)

var (
	// ErrInvalidSender is returned if the transaction contains an invalid signature.
	ErrInvalidSender = errors.New("invalid sender")

	// ErrUnderpriced is returned if a transaction's gas price is below the minimum
	// configured for the transaction pool.
	ErrUnderpriced = errors.New("transaction underpriced")

	// ErrReplaceUnderpriced is returned if a transaction is attempted to be replaced
	// with a different one without the required price bump.
	ErrReplaceUnderpriced = errors.New("replacement transaction underpriced")

	// ErrGasLimit is returned if a transaction's requested gas limit exceeds the
	// maximum allowance of the current block.
	ErrGasLimit = errors.New("exceeds block gas limit")

	// ErrNegativeValue is a sanity error to ensure noone is able to specify a
	// transaction with a negative value.
	ErrNegativeValue = errors.New("negative value")

	// ErrOversizedData is returned if the input data of a transaction is greater
	// than some meaningful limit a user might use. This is not a consensus error
	// making the transaction invalid, rather a DOS protection.
	ErrOversizedData = errors.New("oversized data")
)

var (
	evictionInterval    = time.Minute     // Time interval to check for evictable transactions
	statsReportInterval = 8 * time.Second // Time interval to report transaction pool stats
)

var (
	// Metrics for the pending pool
	pendingDiscardCounter   = metrics.NewRegisteredCounter("txpool/pending/discard", nil)
	pendingReplaceCounter   = metrics.NewRegisteredCounter("txpool/pending/replace", nil)
	pendingRateLimitCounter = metrics.NewRegisteredCounter("txpool/pending/ratelimit", nil) // Dropped due to rate limiting
	pendingNofundsCounter   = metrics.NewRegisteredCounter("txpool/pending/nofunds", nil)   // Dropped due to out-of-funds

	// Metrics for the queued pool
	queuedDiscardCounter   = metrics.NewRegisteredCounter("txpool/queued/discard", nil)
	queuedReplaceCounter   = metrics.NewRegisteredCounter("txpool/queued/replace", nil)
	queuedRateLimitCounter = metrics.NewRegisteredCounter("txpool/queued/ratelimit", nil) // Dropped due to rate limiting
	queuedNofundsCounter   = metrics.NewRegisteredCounter("txpool/queued/nofunds", nil)   // Dropped due to out-of-funds

	// General tx metrics
	invalidTxCounter     = metrics.NewRegisteredCounter("txpool/invalid", nil)
	underpricedTxCounter = metrics.NewRegisteredCounter("txpool/underpriced", nil)
)

// TxStatus is the current status of a transaction as seen by the pool.
type TxStatus uint

const (
	TxStatusUnknown TxStatus = iota
	TxStatusQueued
	TxStatusPending
	TxStatusIncluded
)

// blockChain provides the state of blockchain and current gas limit to do
// some pre checks in tx pool and event subscribers.
type blockChain interface {
	CurrentBlock() *types.Block
	GetBlock(hash common.Hash, number uint64) *types.Block
	StateAt(root common.Hash, mixDigest common.Hash) (*state.StateDB, error)

	SubscribeChainHeadEvent(ch chan<- ChainHeadEvent) event.Subscription
}

// TxPoolConfig are the configuration parameters of the transaction pool.
type TxPoolConfig struct {
	Locals    []common.Address // Addresses that should be treated by default as local
	NoLocals  bool             // Whether local transaction handling should be disabled
	Journal   string           // Journal of local transactions to survive node restarts
	Rejournal time.Duration    // Time interval to regenerate the local transaction journal

	PriceLimit uint64 // Minimum gas price to enforce for acceptance into the pool
	PriceBump  uint64 // Minimum price bump percentage to replace an already existing transaction (nonce)

	AccountSlots uint64 // Number of executable transaction slots guaranteed per account
	GlobalSlots  uint64 // Maximum number of executable transaction slots for all accounts
	AccountQueue uint64 // Maximum number of non-executable transaction slots permitted per account
	GlobalQueue  uint64 // Maximum number of non-executable transaction slots for all accounts

	Lifetime         time.Duration // Maximum amount of time non-executable transaction are queued
	TicketTxLifetime time.Duration // Maximum amount of time buy ticket transaction are queued
}

// DefaultTxPoolConfig contains the default configurations for the transaction
// pool.
var DefaultTxPoolConfig = TxPoolConfig{
	Journal:   "transactions.rlp",
	Rejournal: time.Hour,

	PriceLimit: 1,
	PriceBump:  10,

	AccountSlots: 16,
	GlobalSlots:  4096,
	AccountQueue: 64,
	GlobalQueue:  1024,

	Lifetime:         3 * time.Hour,
	TicketTxLifetime: 10 * time.Minute,
}

// sanitize checks the provided user configurations and changes anything that's
// unreasonable or unworkable.
func (config *TxPoolConfig) sanitize() TxPoolConfig {
	conf := *config
	if conf.Rejournal < time.Second {
		log.Warn("Sanitizing invalid txpool journal time", "provided", conf.Rejournal, "updated", time.Second)
		conf.Rejournal = time.Second
	}
	if conf.PriceLimit < 1 {
		log.Warn("Sanitizing invalid txpool price limit", "provided", conf.PriceLimit, "updated", DefaultTxPoolConfig.PriceLimit)
		conf.PriceLimit = DefaultTxPoolConfig.PriceLimit
	}
	if conf.PriceBump < 1 {
		log.Warn("Sanitizing invalid txpool price bump", "provided", conf.PriceBump, "updated", DefaultTxPoolConfig.PriceBump)
		conf.PriceBump = DefaultTxPoolConfig.PriceBump
	}
	return conf
}

// TxPool contains all currently known transactions. Transactions
// enter the pool when they are received from the network or submitted
// locally. They exit the pool when they are included in the blockchain.
//
// The pool separates processable transactions (which can be applied to the
// current state) and future transactions. Transactions move between those
// two states over time as they are received and processed.
type TxPool struct {
	config       TxPoolConfig
	chainconfig  *params.ChainConfig
	chain        blockChain
	gasPrice     *big.Int
	txFeed       event.Feed
	scope        event.SubscriptionScope
	chainHeadCh  chan ChainHeadEvent
	chainHeadSub event.Subscription
	signer       types.Signer
	mu           sync.RWMutex

	istanbul bool // Fork indicator whether we are in the istanbul stage.

	currentState  *state.StateDB      // Current state in the blockchain head
	pendingState  *state.ManagedState // Pending state tracking virtual nonces
	currentMaxGas uint64              // Current gas limit for transaction caps

	locals  *accountSet // Set of local transaction to exempt from eviction rules
	journal *txJournal  // Journal of local transaction to back up to disk

	pending map[common.Address]*txList   // All currently processable transactions
	queue   map[common.Address]*txList   // Queued but non-processable transactions
	beats   map[common.Address]time.Time // Last heartbeat from each known account
	all     *txLookup                    // All transactions to allow lookups
	priced  *txPricedList                // All transactions sorted by price

	wg sync.WaitGroup // for shutdown sync
}

// NewTxPool creates a new transaction pool to gather, sort and filter inbound
// transactions from the network.
func NewTxPool(config TxPoolConfig, chainconfig *params.ChainConfig, chain blockChain) *TxPool {
	// Sanitize the input to ensure no vulnerable gas prices are set
	config = (&config).sanitize()

	// Create the transaction pool with its initial settings
	pool := &TxPool{
		config:      config,
		chainconfig: chainconfig,
		chain:       chain,
		signer:      types.NewEIP155Signer(chainconfig.ChainID),
		pending:     make(map[common.Address]*txList),
		queue:       make(map[common.Address]*txList),
		beats:       make(map[common.Address]time.Time),
		all:         newTxLookup(),
		chainHeadCh: make(chan ChainHeadEvent, chainHeadChanSize),
		gasPrice:    new(big.Int).SetUint64(config.PriceLimit),
	}
	pool.locals = newAccountSet(pool.signer)
	for _, addr := range config.Locals {
		log.Info("Setting new local account", "address", addr)
		pool.locals.add(addr)
	}
	pool.priced = newTxPricedList(pool.all)
	pool.reset(nil, chain.CurrentBlock().Header())

	// If local transactions and journaling is enabled, load from disk
	if !config.NoLocals && config.Journal != "" {
		pool.journal = newTxJournal(config.Journal)

		if err := pool.journal.load(pool.AddLocals); err != nil {
			log.Warn("Failed to load transaction journal", "err", err)
		}
		if err := pool.journal.rotate(pool.local()); err != nil {
			log.Warn("Failed to rotate transaction journal", "err", err)
		}
	}
	// Subscribe events from blockchain
	pool.chainHeadSub = pool.chain.SubscribeChainHeadEvent(pool.chainHeadCh)

	// Start the event loop and return
	pool.wg.Add(1)
	go pool.loop()

	return pool
}

// loop is the transaction pool's main event loop, waiting for and reacting to
// outside blockchain events as well as for various reporting and transaction
// eviction events.
func (pool *TxPool) loop() {
	defer pool.wg.Done()

	// Start the stats reporting and transaction eviction tickers
	var prevPending, prevQueued, prevStales int

	report := time.NewTicker(statsReportInterval)
	defer report.Stop()

	evict := time.NewTicker(evictionInterval)
	defer evict.Stop()

	journal := time.NewTicker(pool.config.Rejournal)
	defer journal.Stop()

	// Track the previous head headers for transaction reorgs
	head := pool.chain.CurrentBlock()

	// Keep waiting for and reacting to the various events
	for {
		select {
		// Handle ChainHeadEvent
		case ev := <-pool.chainHeadCh:
			if ev.Block != nil {
				pool.mu.Lock()
				pool.reset(head.Header(), ev.Block.Header())
				head = ev.Block

				pool.mu.Unlock()
			}
		// Be unsubscribed due to system stopped
		case <-pool.chainHeadSub.Err():
			return

		// Handle stats reporting ticks
		case <-report.C:
			pool.mu.RLock()
			pending, queued := pool.stats()
			stales := pool.priced.stales
			pool.mu.RUnlock()

			if pending != prevPending || queued != prevQueued || stales != prevStales {
				log.Debug("Transaction pool status report", "executable", pending, "queued", queued, "stales", stales)
				prevPending, prevQueued, prevStales = pending, queued, stales
			}

		// Handle inactive account transaction eviction
		case <-evict.C:
			pool.mu.Lock()
			for addr := range pool.queue {
				// Skip local transactions from the eviction mechanism
				if pool.locals.contains(addr) {
					continue
				}
				// Any non-locals old enough should be removed
				if time.Since(pool.beats[addr]) > pool.config.Lifetime {
					for _, tx := range pool.queue[addr].Flatten() {
						pool.removeTx(tx.Hash(), true)
					}
				}
			}
			for hash, receiveTime := range pool.all.ticketTxBeats {
				tx := pool.all.Get(hash)
				if tx == nil {
					delete(pool.all.ticketTxBeats, hash)
					continue
				}
				// Any buy ticket tx old enough should be removed
				if time.Since(receiveTime) > pool.config.TicketTxLifetime {
					log.Info("remove buy ticket tx old enough",
						"hash", hash,
						"receive", receiveTime,
						"passed", time.Since(receiveTime))
					pool.removeTx(hash, true)
					delete(pool.all.ticketTxBeats, hash)
				}
			}
			pool.mu.Unlock()

		// Handle local transaction journal rotation
		case <-journal.C:
			if pool.journal != nil {
				pool.mu.Lock()
				if err := pool.journal.rotate(pool.local()); err != nil {
					log.Warn("Failed to rotate local tx journal", "err", err)
				}
				pool.mu.Unlock()
			}
		}
	}
}

// lockedReset is a wrapper around reset to allow calling it in a thread safe
// manner. This method is only ever used in the tester!
func (pool *TxPool) lockedReset(oldHead, newHead *types.Header) {
	pool.mu.Lock()
	defer pool.mu.Unlock()

	pool.reset(oldHead, newHead)
}

// reset retrieves the current state of the blockchain and ensures the content
// of the transaction pool is valid with regard to the chain state.
func (pool *TxPool) reset(oldHead, newHead *types.Header) {
	// If we're reorging an old state, reinject all dropped transactions
	var reinject types.Transactions

	if oldHead != nil && oldHead.Hash() != newHead.ParentHash {
		// If the reorg is too deep, avoid doing it (will happen during fast sync)
		oldNum := oldHead.Number.Uint64()
		newNum := newHead.Number.Uint64()

		if depth := uint64(math.Abs(float64(oldNum) - float64(newNum))); depth > 64 {
			log.Debug("Skipping deep transaction reorg", "depth", depth)
		} else {
			// Reorg seems shallow enough to pull in all transactions into memory
			var discarded, included types.Transactions

			var (
				rem = pool.chain.GetBlock(oldHead.Hash(), oldHead.Number.Uint64())
				add = pool.chain.GetBlock(newHead.Hash(), newHead.Number.Uint64())
			)
			for rem.NumberU64() > add.NumberU64() {
				discarded = append(discarded, rem.Transactions()...)
				if rem = pool.chain.GetBlock(rem.ParentHash(), rem.NumberU64()-1); rem == nil {
					log.Error("Unrooted old chain seen by tx pool", "block", oldHead.Number, "hash", oldHead.Hash())
					return
				}
			}
			for add.NumberU64() > rem.NumberU64() {
				included = append(included, add.Transactions()...)
				if add = pool.chain.GetBlock(add.ParentHash(), add.NumberU64()-1); add == nil {
					log.Error("Unrooted new chain seen by tx pool", "block", newHead.Number, "hash", newHead.Hash())
					return
				}
			}
			for rem.Hash() != add.Hash() {
				discarded = append(discarded, rem.Transactions()...)
				if rem = pool.chain.GetBlock(rem.ParentHash(), rem.NumberU64()-1); rem == nil {
					log.Error("Unrooted old chain seen by tx pool", "block", oldHead.Number, "hash", oldHead.Hash())
					return
				}
				included = append(included, add.Transactions()...)
				if add = pool.chain.GetBlock(add.ParentHash(), add.NumberU64()-1); add == nil {
					log.Error("Unrooted new chain seen by tx pool", "block", newHead.Number, "hash", newHead.Hash())
					return
				}
			}
			reinject = types.TxDifference(discarded, included)
		}
	}
	// Initialize the internal state to the current head
	if newHead == nil {
		newHead = pool.chain.CurrentBlock().Header() // Special case during testing
	}
	statedb, err := pool.chain.StateAt(newHead.Root, newHead.MixDigest)
	if err != nil {
		log.Error("Failed to reset txpool state", "err", err)
		return
	}
	pool.currentState = statedb
	pool.pendingState = state.ManageState(statedb)
	pool.currentMaxGas = newHead.GasLimit

	// Inject any transactions discarded due to reorgs
	log.Debug("Reinjecting stale transactions", "count", len(reinject))
	senderCacher.recover(pool.signer, reinject)
	pool.addTxsLocked(reinject, false)

	// Update all fork indicator by next pending block number.
	next := new(big.Int).Add(newHead.Number, big.NewInt(1))
	pool.istanbul = pool.chainconfig.IsIstanbul(next)

	// validate the pool of pending transactions, this will remove
	// any transactions that have been included in the block or
	// have been invalidated because of another transaction (e.g.
	// higher gas price)
	pool.demoteUnexecutables()

	// Update all accounts to the latest known pending nonce
	for addr, list := range pool.pending {
		txs := list.Flatten() // Heavy but will be cached and is needed by the miner anyway
		pool.pendingState.SetNonce(addr, txs[len(txs)-1].Nonce()+1)
	}
	// Check the queue and move transactions over to the pending if possible
	// or remove those that have become invalid
	pool.promoteExecutables(nil)
}

// Stop terminates the transaction pool.
func (pool *TxPool) Stop() {
	// Unsubscribe all subscriptions registered from txpool
	pool.scope.Close()

	// Unsubscribe subscriptions registered from blockchain
	pool.chainHeadSub.Unsubscribe()
	pool.wg.Wait()

	if pool.journal != nil {
		pool.journal.close()
	}
	log.Info("Transaction pool stopped")
}

// SubscribeNewTxsEvent registers a subscription of NewTxsEvent and
// starts sending event to the given channel.
func (pool *TxPool) SubscribeNewTxsEvent(ch chan<- NewTxsEvent) event.Subscription {
	return pool.scope.Track(pool.txFeed.Subscribe(ch))
}

// GasPrice returns the current gas price enforced by the transaction pool.
func (pool *TxPool) GasPrice() *big.Int {
	pool.mu.RLock()
	defer pool.mu.RUnlock()

	return new(big.Int).Set(pool.gasPrice)
}

// SetGasPrice updates the minimum price required by the transaction pool for a
// new transaction, and drops all transactions below this threshold.
func (pool *TxPool) SetGasPrice(price *big.Int) {
	pool.mu.Lock()
	defer pool.mu.Unlock()

	pool.gasPrice = price
	for _, tx := range pool.priced.Cap(price, pool.locals) {
		pool.removeTx(tx.Hash(), false)
	}
	log.Info("Transaction pool price threshold updated", "price", price)
}

// State returns the virtual managed state of the transaction pool.
func (pool *TxPool) State() *state.ManagedState {
	pool.mu.RLock()
	defer pool.mu.RUnlock()

	return pool.pendingState
}

// Stats retrieves the current pool stats, namely the number of pending and the
// number of queued (non-executable) transactions.
func (pool *TxPool) Stats() (int, int) {
	pool.mu.RLock()
	defer pool.mu.RUnlock()

	return pool.stats()
}

// stats retrieves the current pool stats, namely the number of pending and the
// number of queued (non-executable) transactions.
func (pool *TxPool) stats() (int, int) {
	pending := 0
	for _, list := range pool.pending {
		pending += list.Len()
	}
	queued := 0
	for _, list := range pool.queue {
		queued += list.Len()
	}
	return pending, queued
}

// Content retrieves the data content of the transaction pool, returning all the
// pending as well as queued transactions, grouped by account and sorted by nonce.
func (pool *TxPool) Content() (map[common.Address]types.Transactions, map[common.Address]types.Transactions) {
	pool.mu.Lock()
	defer pool.mu.Unlock()

	pending := make(map[common.Address]types.Transactions)
	for addr, list := range pool.pending {
		pending[addr] = list.Flatten()
	}
	queued := make(map[common.Address]types.Transactions)
	for addr, list := range pool.queue {
		queued[addr] = list.Flatten()
	}
	return pending, queued
}

// Pending retrieves all currently processable transactions, grouped by origin
// account and sorted by nonce. The returned transaction set is a copy and can be
// freely modified by calling code.
func (pool *TxPool) Pending() (map[common.Address]types.Transactions, error) {
	pool.mu.Lock()
	defer pool.mu.Unlock()

	pending := make(map[common.Address]types.Transactions)
	for addr, list := range pool.pending {
		pending[addr] = list.Flatten()
	}
	return pending, nil
}

// Locals retrieves the accounts currently considered local by the pool.
func (pool *TxPool) Locals() []common.Address {
	pool.mu.Lock()
	defer pool.mu.Unlock()

	return pool.locals.flatten()
}

// local retrieves all currently known local transactions, grouped by origin
// account and sorted by nonce. The returned transaction set is a copy and can be
// freely modified by calling code.
func (pool *TxPool) local() map[common.Address]types.Transactions {
	txs := make(map[common.Address]types.Transactions)
	for addr := range pool.locals.accounts {
		if pending := pool.pending[addr]; pending != nil {
			txs[addr] = append(txs[addr], pending.Flatten()...)
		}
		if queued := pool.queue[addr]; queued != nil {
			txs[addr] = append(txs[addr], queued.Flatten()...)
		}
	}
	return txs
}

// validateTx checks whether a transaction is valid according to the consensus
// rules and adheres to some heuristic limits of the local node (price and size).
func (pool *TxPool) validateTx(tx *types.Transaction, local bool) error {
	// Heuristic limit, reject transactions over 32KB to prevent DOS attacks
	if tx.Size() > 32*1024 {
		return ErrOversizedData
	}
	// Transactions can't be negative. This may never happen using RLP decoded
	// transactions but may occur if you create a transaction using the RPC.
	if tx.Value().Sign() < 0 {
		return ErrNegativeValue
	}
	// Ensure the transaction doesn't exceed the current block limit gas.
	if pool.currentMaxGas < tx.Gas() {
		return ErrGasLimit
	}
	// Make sure the transaction is signed properly
	from, err := types.Sender(pool.signer, tx)
	if err != nil {
		return ErrInvalidSender
	}
	// Drop non-local transactions under our own minimal accepted gas price
	local = local || pool.locals.contains(from) // account may be local even if the transaction arrived from the network
	if !local && pool.gasPrice.Cmp(tx.GasPrice()) > 0 {
		return ErrUnderpriced
	}
	// Ensure the transaction adheres to nonce ordering
	if pool.currentState.GetNonce(from) > tx.Nonce() {
		return ErrNonceTooLow
	}
	// Transactor should have enough funds to cover the costs
	// cost == V + GP * GL
	if pool.currentState.GetBalance(common.SystemAssetID, from).Cmp(tx.Cost()) < 0 {
		return ErrInsufficientFunds
	}
	intrGas, err := IntrinsicGas(tx.Data(), tx.To() == nil, true, pool.istanbul)
	if err != nil {
		return err
	}
	if tx.Gas() < intrGas {
		return ErrIntrinsicGas
	}
	return nil
}

// add validates a transaction and inserts it into the non-executable queue for
// later pending promotion and execution. If the transaction is a replacement for
// an already pending or queued one, it overwrites the previous and returns this
// so outer code doesn't uselessly call promote.
//
// If a newly added transaction is marked as local, its sending account will be
// whitelisted, preventing any associated transaction from being dropped out of
// the pool due to pricing constraints.
func (pool *TxPool) add(tx *types.Transaction, local bool) (bool, error) {
	// If the transaction is already known, discard it
	hash := tx.Hash()
	if pool.all.Get(hash) != nil {
		log.Trace("Discarding already known transaction", "hash", hash)
		return false, fmt.Errorf("known transaction: %x", hash)
	}
	// If the transaction fails basic validation, discard it
	if err := pool.validateTx(tx, local); err != nil {
		log.Trace("Discarding invalid transaction", "hash", hash, "err", err)
		invalidTxCounter.Inc(1)
		return false, err
	}
	// If the transaction is an invalid FsnCall tx, discard it
	if err := pool.validateAddFsnCallTx(tx); err != nil {
		invalidTxCounter.Inc(1)
		return false, err
	}
	// If the transaction pool is full, discard underpriced transactions
	if uint64(pool.all.Count()) >= pool.config.GlobalSlots+pool.config.GlobalQueue {
		// If the new transaction is underpriced, don't accept it
		if !local && pool.priced.Underpriced(tx, pool.locals) {
			log.Trace("Discarding underpriced transaction", "hash", hash, "price", tx.GasPrice())
			underpricedTxCounter.Inc(1)
			return false, ErrUnderpriced
		}
		// New transaction is better than our worse ones, make room for it
		drop := pool.priced.Discard(pool.all.Count()-int(pool.config.GlobalSlots+pool.config.GlobalQueue-1), pool.locals)
		for _, tx := range drop {
			log.Trace("Discarding freshly underpriced transaction", "hash", tx.Hash(), "price", tx.GasPrice())
			underpricedTxCounter.Inc(1)
			pool.removeTx(tx.Hash(), false)
		}
	}
	// If the transaction is replacing an already pending one, do directly
	from, _ := types.Sender(pool.signer, tx) // already validated
	if list := pool.pending[from]; list != nil && list.Overlaps(tx) {
		// Nonce already pending, check if required price bump is met
		inserted, old := list.Add(tx, pool.config.PriceBump)
		if !inserted {
			pendingDiscardCounter.Inc(1)
			return false, ErrReplaceUnderpriced
		}
		// New transaction is better, replace old one
		if old != nil {
			pool.all.Remove(old.Hash())
			pool.priced.Removed()
			pendingReplaceCounter.Inc(1)
		}
		pool.all.Add(tx)
		pool.priced.Put(tx)
		pool.journalTx(from, tx)

		log.Trace("Pooled new executable transaction", "hash", hash, "from", from, "to", tx.To())

		// We've directly injected a replacement transaction, notify subsystems
		go pool.txFeed.Send(NewTxsEvent{types.Transactions{tx}})

		return old != nil, nil
	}
	// New transaction isn't replacing a pending one, push into queue
	replace, err := pool.enqueueTx(hash, tx)
	if err != nil {
		return false, err
	}
	// Mark local addresses and journal local transactions
	if local {
		if !pool.locals.contains(from) {
			log.Info("Setting new local account", "address", from)
			pool.locals.add(from)
		}
	}
	pool.journalTx(from, tx)

	log.Trace("Pooled new future transaction", "hash", hash, "from", from, "to", tx.To())
	return replace, nil
}

// enqueueTx inserts a new transaction into the non-executable transaction queue.
//
// Note, this method assumes the pool lock is held!
func (pool *TxPool) enqueueTx(hash common.Hash, tx *types.Transaction) (bool, error) {
	// Try to insert the transaction into the future queue
	from, _ := types.Sender(pool.signer, tx) // already validated
	if pool.queue[from] == nil {
		pool.queue[from] = newTxList(false)
	}
	inserted, old := pool.queue[from].Add(tx, pool.config.PriceBump)
	if !inserted {
		// An older transaction was better, discard this
		queuedDiscardCounter.Inc(1)
		return false, ErrReplaceUnderpriced
	}
	// Discard any previous transaction and mark this
	if old != nil {
		pool.all.Remove(old.Hash())
		pool.priced.Removed()
		queuedReplaceCounter.Inc(1)
	}
	if pool.all.Get(hash) == nil {
		pool.all.Add(tx)
		pool.priced.Put(tx)
	}
	return old != nil, nil
}

// journalTx adds the specified transaction to the local disk journal if it is
// deemed to have been sent from a local account.
func (pool *TxPool) journalTx(from common.Address, tx *types.Transaction) {
	// Only journal if it's enabled and the transaction is local
	if pool.journal == nil || !pool.locals.contains(from) {
		return
	}
	if err := pool.journal.insert(tx); err != nil {
		log.Warn("Failed to journal local transaction", "err", err)
	}
}

// promoteTx adds a transaction to the pending (processable) list of transactions
// and returns whether it was inserted or an older was better.
//
// Note, this method assumes the pool lock is held!
func (pool *TxPool) promoteTx(addr common.Address, hash common.Hash, tx *types.Transaction) bool {
	// Try to insert the transaction into the pending queue
	if pool.pending[addr] == nil {
		pool.pending[addr] = newTxList(true)
	}
	list := pool.pending[addr]

	inserted, old := list.Add(tx, pool.config.PriceBump)
	if !inserted {
		// An older transaction was better, discard this
		pool.all.Remove(hash)
		pool.priced.Removed()

		pendingDiscardCounter.Inc(1)
		return false
	}
	// Otherwise discard any previous transaction and mark this
	if old != nil {
		pool.all.Remove(old.Hash())
		pool.priced.Removed()

		pendingReplaceCounter.Inc(1)
	}
	// Failsafe to work around direct pending inserts (tests)
	if pool.all.Get(hash) == nil {
		pool.all.Add(tx)
		pool.priced.Put(tx)
	}
	// Set the potentially new pending nonce and notify any subsystems of the new tx
	pool.beats[addr] = time.Now()
	pool.pendingState.SetNonce(addr, tx.Nonce()+1)

	return true
}

// AddLocal enqueues a single transaction into the pool if it is valid, marking
// the sender as a local one in the mean time, ensuring it goes around the local
// pricing constraints.
func (pool *TxPool) AddLocal(tx *types.Transaction) error {
	return pool.addTx(tx, !pool.config.NoLocals)
}

// AddRemote enqueues a single transaction into the pool if it is valid. If the
// sender is not among the locally tracked ones, full pricing constraints will
// apply.
func (pool *TxPool) AddRemote(tx *types.Transaction) error {
	return pool.addTx(tx, false)
}

// AddLocals enqueues a batch of transactions into the pool if they are valid,
// marking the senders as a local ones in the mean time, ensuring they go around
// the local pricing constraints.
func (pool *TxPool) AddLocals(txs []*types.Transaction) []error {
	return pool.addTxs(txs, !pool.config.NoLocals)
}

// AddRemotes enqueues a batch of transactions into the pool if they are valid.
// If the senders are not among the locally tracked ones, full pricing constraints
// will apply.
func (pool *TxPool) AddRemotes(txs []*types.Transaction) []error {
	return pool.addTxs(txs, false)
}

// addTx enqueues a single transaction into the pool if it is valid.
func (pool *TxPool) addTx(tx *types.Transaction, local bool) error {
	pool.mu.Lock()
	defer pool.mu.Unlock()

	// Try to inject the transaction and update any state
	replace, err := pool.add(tx, local)
	if err != nil {
		return err
	}
	// If we added a new transaction, run promotion checks and return
	if !replace {
		from, _ := types.Sender(pool.signer, tx) // already validated
		pool.promoteExecutables([]common.Address{from})
	}
	return nil
}

// addTxs attempts to queue a batch of transactions if they are valid.
func (pool *TxPool) addTxs(txs []*types.Transaction, local bool) []error {
	pool.mu.Lock()
	defer pool.mu.Unlock()

	return pool.addTxsLocked(txs, local)
}

// addTxsLocked attempts to queue a batch of transactions if they are valid,
// whilst assuming the transaction pool lock is already held.
func (pool *TxPool) addTxsLocked(txs []*types.Transaction, local bool) []error {
	// Add the batch of transaction, tracking the accepted ones
	dirty := make(map[common.Address]struct{})
	errs := make([]error, len(txs))

	for i, tx := range txs {
		var replace bool
		if replace, errs[i] = pool.add(tx, local); errs[i] == nil && !replace {
			from, _ := types.Sender(pool.signer, tx) // already validated
			dirty[from] = struct{}{}
		}
	}
	// Only reprocess the internal state if something was actually added
	if len(dirty) > 0 {
		addrs := make([]common.Address, 0, len(dirty))
		for addr := range dirty {
			addrs = append(addrs, addr)
		}
		pool.promoteExecutables(addrs)
	}
	return errs
}

// Status returns the status (unknown/pending/queued) of a batch of transactions
// identified by their hashes.
func (pool *TxPool) Status(hashes []common.Hash) []TxStatus {
	status := make([]TxStatus, len(hashes))
	for i, hash := range hashes {
		tx := pool.Get(hash)
		if tx == nil {
			continue
		}
		from, _ := types.Sender(pool.signer, tx) // already validated
		pool.mu.RLock()
		if txList := pool.pending[from]; txList != nil && txList.txs.items[tx.Nonce()] != nil {
			status[i] = TxStatusPending
		} else if txList := pool.queue[from]; txList != nil && txList.txs.items[tx.Nonce()] != nil {
			status[i] = TxStatusQueued
		}
		// implicit else: the tx may have been included into a block between
		// checking pool.Get and obtaining the lock. In that case, TxStatusUnknown is correct
		pool.mu.RUnlock()
	}
	return status
}

// Get returns a transaction if it is contained in the pool
// and nil otherwise.
func (pool *TxPool) Get(hash common.Hash) *types.Transaction {
	return pool.all.Get(hash)
}

func (pool *TxPool) GetByPredicate(predicate func(*types.Transaction) bool) *types.Transaction {
	return pool.all.GetByPredicate(predicate)
}

// removeTx removes a single transaction from the queue, moving all subsequent
// transactions back to the future queue.
func (pool *TxPool) removeTx(hash common.Hash, outofbound bool) {
	// Fetch the transaction we wish to delete
	tx := pool.all.Get(hash)
	if tx == nil {
		return
	}
	addr, _ := types.Sender(pool.signer, tx) // already validated during insertion

	// Remove it from the list of known transactions
	pool.all.Remove(hash)
	if outofbound {
		pool.priced.Removed()
	}
	// Remove the transaction from the pending lists and reset the account nonce
	if pending := pool.pending[addr]; pending != nil {
		if removed, invalids := pending.Remove(tx); removed {
			// If no more pending transactions are left, remove the list
			if pending.Empty() {
				delete(pool.pending, addr)
				delete(pool.beats, addr)
			}
			// Postpone any invalidated transactions
			for _, tx := range invalids {
				pool.enqueueTx(tx.Hash(), tx)
			}
			// Update the account nonce if needed
			if nonce := tx.Nonce(); pool.pendingState.GetNonce(addr) > nonce {
				pool.pendingState.SetNonce(addr, nonce)
			}
			return
		}
	}
	// Transaction is in the future queue
	if future := pool.queue[addr]; future != nil {
		future.Remove(tx)
		if future.Empty() {
			delete(pool.queue, addr)
		}
	}
}

// promoteExecutables moves transactions that have become processable from the
// future queue to the set of pending transactions. During this process, all
// invalidated transactions (low nonce, low balance) are deleted.
func (pool *TxPool) promoteExecutables(accounts []common.Address) {
	// Track the promoted transactions to broadcast them at once
	var promoted []*types.Transaction

	// Gather all the accounts potentially needing updates
	if accounts == nil {
		accounts = make([]common.Address, 0, len(pool.queue))
		for addr := range pool.queue {
			accounts = append(accounts, addr)
		}
	}
	// Iterate over all accounts and promote any executable transactions
	for _, addr := range accounts {
		list := pool.queue[addr]
		if list == nil {
			continue // Just in case someone calls with a non existing account
		}
		// Drop all transactions that are deemed too old (low nonce)
		for _, tx := range list.Forward(pool.currentState.GetNonce(addr)) {
			hash := tx.Hash()
			log.Trace("Removed old queued transaction", "hash", hash)
			pool.all.Remove(hash)
			pool.priced.Removed()
		}
		// Drop all transactions that are too costly (low balance or out of gas)
		drops, _ := list.Filter(pool.currentState.GetBalance(common.SystemAssetID, addr), pool.currentMaxGas)
		for _, tx := range drops {
			hash := tx.Hash()
			log.Trace("Removed unpayable queued transaction", "hash", hash)
			pool.all.Remove(hash)
			pool.priced.Removed()
			queuedNofundsCounter.Inc(1)
		}
		// Drop all FsnCall transactions that are invalid
		filter := func(tx *types.Transaction) bool {
			return pool.validateFsnCallTx(tx) != nil
		}
		removes, _ := list.FilterInvalid(filter)
		for _, tx := range removes {
			hash := tx.Hash()
			log.Trace("Removed invalid pending transaction", "hash", hash)
			pool.all.Remove(hash)
			pool.priced.Removed()
		}
		// Gather all executable transactions and promote them
		for _, tx := range list.Ready(pool.pendingState.GetNonce(addr)) {
			hash := tx.Hash()
			if pool.promoteTx(addr, hash, tx) {
				log.Trace("Promoting queued transaction", "hash", hash)
				promoted = append(promoted, tx)
			}
		}
		// Drop all transactions over the allowed limit
		if !pool.locals.contains(addr) {
			for _, tx := range list.Cap(int(pool.config.AccountQueue)) {
				hash := tx.Hash()
				pool.all.Remove(hash)
				pool.priced.Removed()
				queuedRateLimitCounter.Inc(1)
				log.Trace("Removed cap-exceeding queued transaction", "hash", hash)
			}
		}
		// Delete the entire queue entry if it became empty.
		if list.Empty() {
			delete(pool.queue, addr)
		}
	}
	// Notify subsystem for new promoted transactions.
	if len(promoted) > 0 {
		go pool.txFeed.Send(NewTxsEvent{promoted})
	}
	// If the pending limit is overflown, start equalizing allowances
	pending := uint64(0)
	for _, list := range pool.pending {
		pending += uint64(list.Len())
	}
	if pending > pool.config.GlobalSlots {
		pendingBeforeCap := pending
		// Assemble a spam order to penalize large transactors first
		spammers := prque.New(nil)
		for addr, list := range pool.pending {
			// Only evict transactions from high rollers
			if !pool.locals.contains(addr) && uint64(list.Len()) > pool.config.AccountSlots {
				spammers.Push(addr, int64(list.Len()))
			}
		}
		// Gradually drop transactions from offenders
		offenders := []common.Address{}
		for pending > pool.config.GlobalSlots && !spammers.Empty() {
			// Retrieve the next offender if not local address
			offender, _ := spammers.Pop()
			offenders = append(offenders, offender.(common.Address))

			// Equalize balances until all the same or below threshold
			if len(offenders) > 1 {
				// Calculate the equalization threshold for all current offenders
				threshold := pool.pending[offender.(common.Address)].Len()

				// Iteratively reduce all offenders until below limit or threshold reached
				for pending > pool.config.GlobalSlots && pool.pending[offenders[len(offenders)-2]].Len() > threshold {
					for i := 0; i < len(offenders)-1; i++ {
						list := pool.pending[offenders[i]]
						for _, tx := range list.Cap(list.Len() - 1) {
							// Drop the transaction from the global pools too
							hash := tx.Hash()
							pool.all.Remove(hash)
							pool.priced.Removed()

							// Update the account nonce to the dropped transaction
							if nonce := tx.Nonce(); pool.pendingState.GetNonce(offenders[i]) > nonce {
								pool.pendingState.SetNonce(offenders[i], nonce)
							}
							log.Trace("Removed fairness-exceeding pending transaction", "hash", hash)
						}
						pending--
					}
				}
			}
		}
		// If still above threshold, reduce to limit or min allowance
		if pending > pool.config.GlobalSlots && len(offenders) > 0 {
			for pending > pool.config.GlobalSlots && uint64(pool.pending[offenders[len(offenders)-1]].Len()) > pool.config.AccountSlots {
				for _, addr := range offenders {
					list := pool.pending[addr]
					for _, tx := range list.Cap(list.Len() - 1) {
						// Drop the transaction from the global pools too
						hash := tx.Hash()
						pool.all.Remove(hash)
						pool.priced.Removed()

						// Update the account nonce to the dropped transaction
						if nonce := tx.Nonce(); pool.pendingState.GetNonce(addr) > nonce {
							pool.pendingState.SetNonce(addr, nonce)
						}
						log.Trace("Removed fairness-exceeding pending transaction", "hash", hash)
					}
					pending--
				}
			}
		}
		pendingRateLimitCounter.Inc(int64(pendingBeforeCap - pending))
	}
	// If we've queued more transactions than the hard limit, drop oldest ones
	queued := uint64(0)
	for _, list := range pool.queue {
		queued += uint64(list.Len())
	}
	if queued > pool.config.GlobalQueue {
		// Sort all accounts with queued transactions by heartbeat
		addresses := make(addressesByHeartbeat, 0, len(pool.queue))
		for addr := range pool.queue {
			if !pool.locals.contains(addr) { // don't drop locals
				addresses = append(addresses, addressByHeartbeat{addr, pool.beats[addr]})
			}
		}
		sort.Sort(addresses)

		// Drop transactions until the total is below the limit or only locals remain
		for drop := queued - pool.config.GlobalQueue; drop > 0 && len(addresses) > 0; {
			addr := addresses[len(addresses)-1]
			list := pool.queue[addr.address]

			addresses = addresses[:len(addresses)-1]

			// Drop all transactions if they are less than the overflow
			if size := uint64(list.Len()); size <= drop {
				for _, tx := range list.Flatten() {
					pool.removeTx(tx.Hash(), true)
				}
				drop -= size
				queuedRateLimitCounter.Inc(int64(size))
				continue
			}
			// Otherwise drop only last few transactions
			txs := list.Flatten()
			for i := len(txs) - 1; i >= 0 && drop > 0; i-- {
				pool.removeTx(txs[i].Hash(), true)
				drop--
				queuedRateLimitCounter.Inc(1)
			}
		}
	}
}

// demoteUnexecutables removes invalid and processed transactions from the pools
// executable/pending queue and any subsequent transactions that become unexecutable
// are moved back into the future queue.
func (pool *TxPool) demoteUnexecutables() {
	// Iterate over all accounts and demote any non-executable transactions
	for addr, list := range pool.pending {
		nonce := pool.currentState.GetNonce(addr)

		// Drop all transactions that are deemed too old (low nonce)
		for _, tx := range list.Forward(nonce) {
			hash := tx.Hash()
			log.Trace("Removed old pending transaction", "hash", hash)
			pool.all.Remove(hash)
			pool.priced.Removed()
		}
		// Drop all FsnCall transactions that are invalid
		filter := func(tx *types.Transaction) bool {
			return pool.validateFsnCallTx(tx) != nil
		}
		removes, adjusts := list.FilterInvalid(filter)
		for _, tx := range removes {
			hash := tx.Hash()
			log.Trace("Removed invalid pending transaction", "hash", hash)
			pool.all.Remove(hash)
			pool.priced.Removed()
		}
		// Drop all transactions that are too costly (low balance or out of gas), and queue any invalids back for later
		drops, invalids := list.Filter(pool.currentState.GetBalance(common.SystemAssetID, addr), pool.currentMaxGas)
		for _, tx := range drops {
			hash := tx.Hash()
			log.Trace("Removed unpayable pending transaction", "hash", hash)
			pool.all.Remove(hash)
			pool.priced.Removed()
			pendingNofundsCounter.Inc(1)
		}
		for _, tx := range invalids {
			hash := tx.Hash()
			log.Trace("Demoting pending transaction", "hash", hash)
			pool.enqueueTx(hash, tx)
		}
		if adjusts.Len() > 0 {
			adjusts = types.TxDifference(adjusts, drops)
			adjusts = types.TxDifference(adjusts, invalids)
			for _, tx := range adjusts {
				hash := tx.Hash()
				log.Trace("Demoting pending transaction", "hash", hash)
				pool.enqueueTx(hash, tx)
			}
		}
		// If there's a gap in front, alert (should never happen) and postpone all transactions
		if list.Len() > 0 && list.txs.Get(nonce) == nil {
			for _, tx := range list.Cap(0) {
				hash := tx.Hash()
				log.Error("Demoting invalidated transaction", "hash", hash)
				pool.enqueueTx(hash, tx)
			}
		}
		// Delete the entire queue entry if it became empty.
		if list.Empty() {
			delete(pool.pending, addr)
			delete(pool.beats, addr)
		}
	}
}

// addressByHeartbeat is an account address tagged with its last activity timestamp.
type addressByHeartbeat struct {
	address   common.Address
	heartbeat time.Time
}

type addressesByHeartbeat []addressByHeartbeat

func (a addressesByHeartbeat) Len() int           { return len(a) }
func (a addressesByHeartbeat) Less(i, j int) bool { return a[i].heartbeat.Before(a[j].heartbeat) }
func (a addressesByHeartbeat) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }

// accountSet is simply a set of addresses to check for existence, and a signer
// capable of deriving addresses from transactions.
type accountSet struct {
	accounts map[common.Address]struct{}
	signer   types.Signer
	cache    *[]common.Address
}

// newAccountSet creates a new address set with an associated signer for sender
// derivations.
func newAccountSet(signer types.Signer) *accountSet {
	return &accountSet{
		accounts: make(map[common.Address]struct{}),
		signer:   signer,
	}
}

// contains checks if a given address is contained within the set.
func (as *accountSet) contains(addr common.Address) bool {
	_, exist := as.accounts[addr]
	return exist
}

// containsTx checks if the sender of a given tx is within the set. If the sender
// cannot be derived, this method returns false.
func (as *accountSet) containsTx(tx *types.Transaction) bool {
	if addr, err := types.Sender(as.signer, tx); err == nil {
		return as.contains(addr)
	}
	return false
}

// add inserts a new address into the set to track.
func (as *accountSet) add(addr common.Address) {
	as.accounts[addr] = struct{}{}
	as.cache = nil
}

// flatten returns the list of addresses within this set, also caching it for later
// reuse. The returned slice should not be changed!
func (as *accountSet) flatten() []common.Address {
	if as.cache == nil {
		accounts := make([]common.Address, 0, len(as.accounts))
		for account := range as.accounts {
			accounts = append(accounts, account)
		}
		as.cache = &accounts
	}
	return *as.cache
}

// txLookup is used internally by TxPool to track transactions while allowing lookup without
// mutex contention.
//
// Note, although this type is properly protected against concurrent access, it
// is **not** a type that should ever be mutated or even exposed outside of the
// transaction pool, since its internal state is tightly coupled with the pools
// internal mechanisms. The sole purpose of the type is to permit out-of-bound
// peeking into the pool in TxPool.Get without having to acquire the widely scoped
// TxPool.mu mutex.
type txLookup struct {
	all           map[common.Hash]*types.Transaction
	ticketTxBeats map[common.Hash]time.Time // heartbeat from each buy ticket transactions
	lock          sync.RWMutex
}

// newTxLookup returns a new txLookup structure.
func newTxLookup() *txLookup {
	return &txLookup{
		all:           make(map[common.Hash]*types.Transaction),
		ticketTxBeats: make(map[common.Hash]time.Time),
	}
}

// Range calls f on each key and value present in the map.
func (t *txLookup) Range(f func(hash common.Hash, tx *types.Transaction) bool) {
	t.lock.RLock()
	defer t.lock.RUnlock()

	for key, value := range t.all {
		if !f(key, value) {
			break
		}
	}
}

// Get returns a transaction if it exists in the lookup, or nil if not found.
func (t *txLookup) Get(hash common.Hash) *types.Transaction {
	t.lock.RLock()
	defer t.lock.RUnlock()

	return t.all[hash]
}

func (t *txLookup) GetByPredicate(predicate func(*types.Transaction) bool) *types.Transaction {
	t.lock.RLock()
	defer t.lock.RUnlock()

	for _, tx := range t.all {
		if predicate(tx) {
			return tx
		}
	}
	return nil
}

// Count returns the current number of items in the lookup.
func (t *txLookup) Count() int {
	t.lock.RLock()
	defer t.lock.RUnlock()

	return len(t.all)
}

// Add adds a transaction to the lookup.
func (t *txLookup) Add(tx *types.Transaction) {
	t.lock.Lock()
	defer t.lock.Unlock()

	t.all[tx.Hash()] = tx
	if tx.IsBuyTicketTx() {
		t.ticketTxBeats[tx.Hash()] = time.Now()
	}
}

// Remove removes a transaction from the lookup.
func (t *txLookup) Remove(hash common.Hash) {
	t.lock.Lock()
	defer t.lock.Unlock()

	delete(t.all, hash)
}

func (pool *TxPool) validateAddFsnCallTx(tx *types.Transaction) error {
	if err := pool.validateFsnCallTx(tx); err != nil {
		return err
	}
	if tx.IsBuyTicketTx() {
		from, _ := types.Sender(pool.signer, tx) // already validated
		found := false
		var oldTxHash common.Hash
		pool.all.Range(func(hash common.Hash, tx1 *types.Transaction) bool {
			if hash == tx.Hash() {
				found = true
				return false
			} else if tx1.IsBuyTicketTx() {
				sender, _ := types.Sender(pool.signer, tx1)
				if from == sender {
					// always choose latest buy ticket tx
					oldTxHash = hash
					return false
				}
			}
			return true
		})
		if found == true {
			return fmt.Errorf("%v has already bought a ticket in txpool", from.String())
		}
		if oldTxHash != (common.Hash{}) {
			pool.removeTx(oldTxHash, true)
		}
	}
	return nil
}

func (pool *TxPool) validateReceiveAssetPayableTx(tx *types.Transaction, from common.Address) error {
	header := pool.chain.CurrentBlock().Header()
	height := new(big.Int).Add(header.Number, big.NewInt(1))
	input := tx.Data()
	if !common.IsReceiveAssetPayableTx(height, input) {
		return nil
	}
	if pool.currentState.GetCodeSize(*tx.To()) == 0 {
		return fmt.Errorf("receiveAsset tx receiver must be contract")
	}
	timestamp := uint64(time.Now().Unix())
	p := &common.TransferTimeLockParam{}
	// use `timestamp+600` here to ensure timelock tx with minimum lifetime of 10 minutes,
	// that is endtime of timelock must be greater than or equal to `now + 600 seconds`.
	if err := common.ParseReceiveAssetPayableTxInput(p, input, timestamp+600); err != nil {
		return err
	}
	p.Value = tx.Value()
	p.GasValue = new(big.Int).Mul(new(big.Int).SetUint64(tx.Gas()), tx.GasPrice())
	if !CanTransferTimeLock(pool.currentState, from, p) {
		return ErrInsufficientFunds
	}
	return nil
}

func (pool *TxPool) validateFsnCallTx(tx *types.Transaction) error {
	msg, err := tx.AsMessage(pool.signer)
	if err != nil {
		return fmt.Errorf("validateFsnCallTx err:%v", err)
	}
	from := msg.From()
	to := tx.To()

	if !common.IsFsnCall(to) {
		if to == nil {
			return nil
		}
		if err := pool.validateReceiveAssetPayableTx(tx, from); err != nil {
			return err
		}
		return nil
	}

	currBlockHeader := pool.chain.CurrentBlock().Header()
	nextBlockNumber := new(big.Int).Add(currBlockHeader.Number, big.NewInt(1))

	state := pool.currentState
	height := common.BigMaxUint64
	timestamp := uint64(time.Now().Unix())

	param := common.FSNCallParam{}
	if err := rlp.DecodeBytes(tx.Data(), &param); err != nil {
		return fmt.Errorf("decode FSNCallParam error")
	}

	fee := common.GetFsnCallFee(to, param.Func)
	fsnValue := big.NewInt(0)

	switch param.Func {
	case common.GenNotationFunc:
		if n := state.GetNotation(from); n != 0 {
			return fmt.Errorf("Account %s has a notation:%d", from.String(), n)
		}

	case common.GenAssetFunc:
		genAssetParam := common.GenAssetParam{}
		rlp.DecodeBytes(param.Data, &genAssetParam)
		if err := genAssetParam.Check(height); err != nil {
			return err
		}
		assetID := msg.AsTransaction().Hash()
		if _, err := state.GetAsset(assetID); err == nil {
			return fmt.Errorf("%s asset exists", assetID.String())
		}

	case common.SendAssetFunc:
		sendAssetParam := common.SendAssetParam{}
		rlp.DecodeBytes(param.Data, &sendAssetParam)
		if err := sendAssetParam.Check(height); err != nil {
			return err
		}
		if sendAssetParam.AssetID == common.SystemAssetID {
			fsnValue = sendAssetParam.Value
		} else if state.GetBalance(sendAssetParam.AssetID, from).Cmp(sendAssetParam.Value) < 0 {
			return fmt.Errorf("not enough asset")
		}

	case common.TimeLockFunc:
		timeLockParam := common.TimeLockParam{}
		rlp.DecodeBytes(param.Data, &timeLockParam)
		if timeLockParam.Type == common.TimeLockToAsset {
			if timeLockParam.StartTime > timestamp {
				return fmt.Errorf("TimeLockToAsset: Start time must be less than now")
			}
			timeLockParam.EndTime = common.TimeLockForever
		}
		if timeLockParam.To == (common.Address{}) {
			return fmt.Errorf("receiver address must be set and not zero address")
		}
		if err := timeLockParam.Check(height, timestamp); err != nil {
			return err
		}

		start := timeLockParam.StartTime
		end := timeLockParam.EndTime
		if start < timestamp {
			start = timestamp
		}
		needValue := common.NewTimeLock(&common.TimeLockItem{
			StartTime: start,
			EndTime:   end,
			Value:     new(big.Int).SetBytes(timeLockParam.Value.Bytes()),
		})
		if err := needValue.IsValid(); err != nil {
			return err
		}
		switch timeLockParam.Type {
		case common.AssetToTimeLock:
			if timeLockParam.AssetID == common.SystemAssetID {
				fsnValue = timeLockParam.Value
			} else if state.GetBalance(timeLockParam.AssetID, from).Cmp(timeLockParam.Value) < 0 {
				return fmt.Errorf("AssetToTimeLock: not enough asset")
			}
		case common.TimeLockToTimeLock:
			if state.GetTimeLockBalance(timeLockParam.AssetID, from).Cmp(needValue) < 0 {
				return fmt.Errorf("TimeLockToTimeLock: not enough time lock balance")
			}
		case common.TimeLockToAsset:
			if state.GetTimeLockBalance(timeLockParam.AssetID, from).Cmp(needValue) < 0 {
				return fmt.Errorf("TimeLockToAsset: not enough time lock balance")
			}
		case common.SmartTransfer:
			if !common.IsSmartTransferEnabled(nextBlockNumber) {
				return fmt.Errorf("SendTimeLock not enabled")
			}
			timeLockBalance := state.GetTimeLockBalance(timeLockParam.AssetID, from)
			if timeLockBalance.Cmp(needValue) < 0 {
				timeLockValue := timeLockBalance.GetSpendableValue(start, end)
				assetBalance := state.GetBalance(timeLockParam.AssetID, from)
				if new(big.Int).Add(timeLockValue, assetBalance).Cmp(timeLockParam.Value) < 0 {
					return fmt.Errorf("SendTimeLock: not enough balance")
				}
				fsnValue = new(big.Int).Sub(timeLockParam.Value, timeLockValue)
			}
		}

	case common.BuyTicketFunc:
		buyTicketParam := common.BuyTicketParam{}
		rlp.DecodeBytes(param.Data, &buyTicketParam)
		if err := buyTicketParam.Check(height, currBlockHeader.Time); err != nil {
			return err
		}

		start := buyTicketParam.Start
		end := buyTicketParam.End
		value := common.TicketPrice(height)
		needValue := common.NewTimeLock(&common.TimeLockItem{
			StartTime: common.MaxUint64(start, timestamp),
			EndTime:   end,
			Value:     value,
		})
		if err := needValue.IsValid(); err != nil {
			return err
		}

		if state.GetTimeLockBalance(common.SystemAssetID, from).Cmp(needValue) < 0 {
			fsnValue = value
		}

	case common.AssetValueChangeFunc:
		assetValueChangeParamEx := common.AssetValueChangeExParam{}
		rlp.DecodeBytes(param.Data, &assetValueChangeParamEx)

		if err := assetValueChangeParamEx.Check(height); err != nil {
			return err
		}

		asset, err := state.GetAsset(assetValueChangeParamEx.AssetID)
		if err != nil {
			return fmt.Errorf("asset not found")
		}

		if !asset.CanChange {
			return fmt.Errorf("asset can't inc or dec")
		}

		if asset.Owner != from {
			return fmt.Errorf("can only be changed by owner")
		}

		if asset.Owner != assetValueChangeParamEx.To && !assetValueChangeParamEx.IsInc {
			err := fmt.Errorf("decrement can only happen to asset's own account")
			return err
		}

		if !assetValueChangeParamEx.IsInc {
			if state.GetBalance(assetValueChangeParamEx.AssetID, assetValueChangeParamEx.To).Cmp(assetValueChangeParamEx.Value) < 0 {
				return fmt.Errorf("not enough asset")
			}
		}

	case common.EmptyFunc:

	case common.MakeSwapFunc, common.MakeSwapFuncExt:
		makeSwapParam := common.MakeSwapParam{}
		rlp.DecodeBytes(param.Data, &makeSwapParam)
		swapId := msg.AsTransaction().Hash()

		if _, err := state.GetSwap(swapId); err == nil {
			return fmt.Errorf("MakeSwap: %v Swap already exist", swapId.String())
		}

		if err := makeSwapParam.Check(height, timestamp); err != nil {
			return err
		}

		if _, err := state.GetAsset(makeSwapParam.ToAssetID); err != nil {
			return fmt.Errorf("ToAssetID asset %v not found", makeSwapParam.ToAssetID.String())
		}

		if makeSwapParam.FromAssetID == common.OwnerUSANAssetID {
			notation := state.GetNotation(from)
			if notation == 0 {
				return fmt.Errorf("the from address does not have a notation")
			}
		} else {
			total := new(big.Int).Mul(makeSwapParam.MinFromAmount, makeSwapParam.SwapSize)
			start := makeSwapParam.FromStartTime
			end := makeSwapParam.FromEndTime
			useAsset := start == common.TimeLockNow && end == common.TimeLockForever

			if useAsset == true {
				if makeSwapParam.FromAssetID == common.SystemAssetID {
					fsnValue = total
				} else if state.GetBalance(makeSwapParam.FromAssetID, from).Cmp(total) < 0 {
					return fmt.Errorf("not enough from asset")
				}
			} else {
				needValue := common.NewTimeLock(&common.TimeLockItem{
					StartTime: common.MaxUint64(start, timestamp),
					EndTime:   end,
					Value:     total,
				})
				if err := needValue.IsValid(); err != nil {
					return err
				}
				available := state.GetTimeLockBalance(makeSwapParam.FromAssetID, from)
				if available.Cmp(needValue) < 0 {
					if param.Func == common.MakeSwapFunc {
						// this was the legacy swap do not do
						// time lock and just return an error
						return fmt.Errorf("not enough time lock balance")
					}

					if makeSwapParam.FromAssetID == common.SystemAssetID {
						fsnValue = total
					} else if state.GetBalance(makeSwapParam.FromAssetID, from).Cmp(total) < 0 {
						return fmt.Errorf("not enough time lock or asset balance")
					}
				}
			}
		}

	case common.RecallSwapFunc:
		recallSwapParam := common.RecallSwapParam{}
		rlp.DecodeBytes(param.Data, &recallSwapParam)

		swap, err := state.GetSwap(recallSwapParam.SwapID)
		if err != nil {
			return fmt.Errorf("RecallSwap: %v Swap not found", recallSwapParam.SwapID.String())
		}

		if swap.Owner != from {
			return fmt.Errorf("Must be swap onwer can recall")
		}

		if err := recallSwapParam.Check(height, &swap); err != nil {
			return err
		}

	case common.TakeSwapFunc, common.TakeSwapFuncExt:
		takeSwapParam := common.TakeSwapParam{}
		rlp.DecodeBytes(param.Data, &takeSwapParam)

		swap, err := state.GetSwap(takeSwapParam.SwapID)
		if err != nil {
			return fmt.Errorf("TakeSwap: %v Swap not found", takeSwapParam.SwapID.String())
		}

		if err := takeSwapParam.Check(height, &swap, timestamp); err != nil {
			return err
		}

		if err := common.CheckSwapTargets(swap.Targes, from); err != nil {
			return err
		}

		if swap.FromAssetID == common.OwnerUSANAssetID {
			notation := state.GetNotation(swap.Owner)
			if notation == 0 || notation != swap.Notation {
				return fmt.Errorf("notation in swap is no longer valid")
			}
		}

		toTotal := new(big.Int).Mul(swap.MinToAmount, takeSwapParam.Size)
		toStart := swap.ToStartTime
		toEnd := swap.ToEndTime
		toUseAsset := toStart == common.TimeLockNow && toEnd == common.TimeLockForever

		if toUseAsset == true {
			if swap.ToAssetID == common.SystemAssetID {
				fsnValue = toTotal
			} else if state.GetBalance(swap.ToAssetID, from).Cmp(toTotal) < 0 {
				return fmt.Errorf("not enough from asset")
			}
		} else {
			toNeedValue := common.NewTimeLock(&common.TimeLockItem{
				StartTime: common.MaxUint64(toStart, timestamp),
				EndTime:   toEnd,
				Value:     toTotal,
			})
			isValid := true
			if err := toNeedValue.IsValid(); err != nil {
				isValid = false
			}
			if isValid && state.GetTimeLockBalance(swap.ToAssetID, from).Cmp(toNeedValue) < 0 {
				if param.Func == common.TakeSwapFunc {
					// this was the legacy swap do not do
					// time lock and just return an error
					return fmt.Errorf("not enough time lock balance")
				}

				if swap.ToAssetID == common.SystemAssetID {
					fsnValue = toTotal
				} else if state.GetBalance(swap.ToAssetID, from).Cmp(toTotal) < 0 {
					return fmt.Errorf("not enough time lock or asset balance")
				}
			}
		}

	case common.RecallMultiSwapFunc:
		recallSwapParam := common.RecallMultiSwapParam{}
		rlp.DecodeBytes(param.Data, &recallSwapParam)

		swap, err := state.GetMultiSwap(recallSwapParam.SwapID)
		if err != nil {
			return fmt.Errorf("Swap not found")
		}

		if swap.Owner != from {
			return fmt.Errorf("Must be swap onwer can recall")
		}

		if err := recallSwapParam.Check(height, &swap); err != nil {
			return err
		}

	case common.MakeMultiSwapFunc:
		makeSwapParam := common.MakeMultiSwapParam{}
		rlp.DecodeBytes(param.Data, &makeSwapParam)
		swapID := msg.AsTransaction().Hash()

		_, err := state.GetSwap(swapID)
		if err == nil {
			return fmt.Errorf("Swap already exist")
		}

		if err := makeSwapParam.Check(height, timestamp); err != nil {
			return err
		}

		for _, toAssetID := range makeSwapParam.ToAssetID {
			if _, err := state.GetAsset(toAssetID); err != nil {
				return fmt.Errorf("ToAssetID asset %v not found", toAssetID.String())
			}
		}

		ln := len(makeSwapParam.FromAssetID)

		useAsset := make([]bool, ln)
		total := make([]*big.Int, ln)
		needValue := make([]*common.TimeLock, ln)

		accountBalances := make(map[common.Hash]*big.Int)
		accountTimeLockBalances := make(map[common.Hash]*common.TimeLock)

		for i := 0; i < ln; i++ {
			if _, exist := accountBalances[makeSwapParam.FromAssetID[i]]; !exist {
				balance := state.GetBalance(makeSwapParam.FromAssetID[i], from)
				timelock := state.GetTimeLockBalance(makeSwapParam.FromAssetID[i], from)
				accountBalances[makeSwapParam.FromAssetID[i]] = new(big.Int).Set(balance)
				accountTimeLockBalances[makeSwapParam.FromAssetID[i]] = timelock.Clone()
			}

			total[i] = new(big.Int).Mul(makeSwapParam.MinFromAmount[i], makeSwapParam.SwapSize)
			start := makeSwapParam.FromStartTime[i]
			end := makeSwapParam.FromEndTime[i]
			useAsset[i] = start == common.TimeLockNow && end == common.TimeLockForever
			if useAsset[i] == false {
				needValue[i] = common.NewTimeLock(&common.TimeLockItem{
					StartTime: common.MaxUint64(start, timestamp),
					EndTime:   end,
					Value:     total[i],
				})
				if err := needValue[i].IsValid(); err != nil {
					return err
				}
			}
		}

		ln = len(makeSwapParam.FromAssetID)
		// check balances first
		for i := 0; i < ln; i++ {
			balance := accountBalances[makeSwapParam.FromAssetID[i]]
			timeLockBalance := accountTimeLockBalances[makeSwapParam.FromAssetID[i]]
			if useAsset[i] == true {
				if balance.Cmp(total[i]) < 0 {
					return fmt.Errorf("not enough from asset")
				}
				balance.Sub(balance, total[i])
				if makeSwapParam.FromAssetID[i] == common.SystemAssetID {
					fsnValue.Add(fsnValue, total[i])
				}
			} else {
				if timeLockBalance.Cmp(needValue[i]) < 0 {
					if balance.Cmp(total[i]) < 0 {
						return fmt.Errorf("not enough time lock or asset balance")
					}

					balance.Sub(balance, total[i])
					if makeSwapParam.FromAssetID[i] == common.SystemAssetID {
						fsnValue.Add(fsnValue, total[i])
					}
					totalValue := common.NewTimeLock(&common.TimeLockItem{
						StartTime: timestamp,
						EndTime:   common.TimeLockForever,
						Value:     total[i],
					})
					timeLockBalance.Add(timeLockBalance, totalValue)
				}
				timeLockBalance.Sub(timeLockBalance, needValue[i])
			}
		}

	case common.TakeMultiSwapFunc:
		takeSwapParam := common.TakeMultiSwapParam{}
		rlp.DecodeBytes(param.Data, &takeSwapParam)

		swap, err := state.GetMultiSwap(takeSwapParam.SwapID)
		if err != nil {
			return fmt.Errorf("Swap not found")
		}

		if err := takeSwapParam.Check(height, &swap, timestamp); err != nil {
			return err
		}

		if err := common.CheckSwapTargets(swap.Targes, from); err != nil {
			return err
		}

		lnTo := len(swap.ToAssetID)

		toUseAsset := make([]bool, lnTo)
		toTotal := make([]*big.Int, lnTo)
		toStart := make([]uint64, lnTo)
		toEnd := make([]uint64, lnTo)
		toNeedValue := make([]*common.TimeLock, lnTo)

		accountBalances := make(map[common.Hash]*big.Int)
		accountTimeLockBalances := make(map[common.Hash]*common.TimeLock)

		for i := 0; i < lnTo; i++ {
			if _, exist := accountBalances[swap.ToAssetID[i]]; !exist {
				balance := state.GetBalance(swap.ToAssetID[i], from)
				timelock := state.GetTimeLockBalance(swap.ToAssetID[i], from)
				accountBalances[swap.ToAssetID[i]] = new(big.Int).Set(balance)
				accountTimeLockBalances[swap.ToAssetID[i]] = timelock.Clone()
			}

			toTotal[i] = new(big.Int).Mul(swap.MinToAmount[i], takeSwapParam.Size)
			toStart[i] = swap.ToStartTime[i]
			toEnd[i] = swap.ToEndTime[i]
			toUseAsset[i] = toStart[i] == common.TimeLockNow && toEnd[i] == common.TimeLockForever
			if toUseAsset[i] == false {
				toNeedValue[i] = common.NewTimeLock(&common.TimeLockItem{
					StartTime: common.MaxUint64(toStart[i], timestamp),
					EndTime:   toEnd[i],
					Value:     toTotal[i],
				})
			}
		}

		// check to account balances
		for i := 0; i < lnTo; i++ {
			balance := accountBalances[swap.ToAssetID[i]]
			timeLockBalance := accountTimeLockBalances[swap.ToAssetID[i]]
			if toUseAsset[i] == true {
				if balance.Cmp(toTotal[i]) < 0 {
					return fmt.Errorf("not enough from asset")
				}
				balance.Sub(balance, toTotal[i])
				if swap.ToAssetID[i] == common.SystemAssetID {
					fsnValue.Add(fsnValue, toTotal[i])
				}
			} else {
				if err := toNeedValue[i].IsValid(); err != nil {
					continue
				}
				if timeLockBalance.Cmp(toNeedValue[i]) < 0 {
					if balance.Cmp(toTotal[i]) < 0 {
						return fmt.Errorf("not enough time lock or asset balance")
					}

					balance.Sub(balance, toTotal[i])
					if swap.ToAssetID[i] == common.SystemAssetID {
						fsnValue.Add(fsnValue, toTotal[i])
					}
					totalValue := common.NewTimeLock(&common.TimeLockItem{
						StartTime: timestamp,
						EndTime:   common.TimeLockForever,
						Value:     toTotal[i],
					})
					timeLockBalance.Add(timeLockBalance, totalValue)
				}
				timeLockBalance.Sub(timeLockBalance, toNeedValue[i])
			}
		}

	case common.ReportIllegalFunc:
		if _, _, err := datong.CheckAddingReport(state, param.Data, nil); err != nil {
			return err
		}
		oldtx := pool.GetByPredicate(func(trx *types.Transaction) bool {
			if trx == tx {
				return false
			}
			p := common.FSNCallParam{}
			rlp.DecodeBytes(trx.Data(), &p)
			return param.Func == common.ReportIllegalFunc && bytes.Equal(p.Data, param.Data)
		})
		if oldtx != nil {
			return fmt.Errorf("already reported in pool")
		}

	default:
		return fmt.Errorf("Unsupported FsnCall func '%v'", param.Func.Name())
	}
	// check gas, fee and value
	mgval := new(big.Int).Mul(new(big.Int).SetUint64(tx.Gas()), tx.GasPrice())
	mgval.Add(mgval, fee)
	mgval.Add(mgval, fsnValue)
	if balance := state.GetBalance(common.SystemAssetID, from); balance.Cmp(mgval) < 0 {
		return fmt.Errorf("insufficient balance(%v), need %v = (gas:%v * price:%v + value:%v + fee:%v)", balance, mgval, tx.Gas(), tx.GasPrice(), fsnValue, fee)
	}
	return nil
}
