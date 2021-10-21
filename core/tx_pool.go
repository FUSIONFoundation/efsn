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
	"github.com/FusionFoundation/efsn/consensus/misc"
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

	// txSlotSize is used to calculate how many data slots a single transaction
	// takes up based on its size. The slots are used as DoS protection, ensuring
	// that validating a new transaction remains a constant operation (in reality
	// O(maxslots), where max slots are 4 currently).
	txSlotSize = 32 * 1024

	// txMaxSize is the maximum size a single transaction can have. This field has
	// non-trivial consequences: larger transactions are significantly harder and
	// more expensive to propagate; larger transactions also take more resources
	// to validate whether they fit into the pool or not.
	txMaxSize = 4 * txSlotSize // 128KB
)

var (
	// ErrAlreadyKnown is returned if the transactions is already contained
	// within the pool.
	ErrAlreadyKnown = errors.New("already known")

	// ErrInvalidSender is returned if the transaction contains an invalid signature.
	ErrInvalidSender = errors.New("invalid sender")

	// ErrUnderpriced is returned if a transaction's gas price is below the minimum
	// configured for the transaction pool.
	ErrUnderpriced = errors.New("transaction underpriced")

	// ErrTxPoolOverflow is returned if the transaction pool is full and can't accpet
	// another remote transaction.
	ErrTxPoolOverflow = errors.New("txpool is full")

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
	pendingDiscardMeter   = metrics.NewRegisteredMeter("txpool/pending/discard", nil)
	pendingReplaceMeter   = metrics.NewRegisteredMeter("txpool/pending/replace", nil)
	pendingRateLimitMeter = metrics.NewRegisteredMeter("txpool/pending/ratelimit", nil) // Dropped due to rate limiting
	pendingNofundsMeter   = metrics.NewRegisteredMeter("txpool/pending/nofunds", nil)   // Dropped due to out-of-funds

	// Metrics for the queued pool
	queuedDiscardMeter   = metrics.NewRegisteredMeter("txpool/queued/discard", nil)
	queuedReplaceMeter   = metrics.NewRegisteredMeter("txpool/queued/replace", nil)
	queuedRateLimitMeter = metrics.NewRegisteredMeter("txpool/queued/ratelimit", nil) // Dropped due to rate limiting
	queuedNofundsMeter   = metrics.NewRegisteredMeter("txpool/queued/nofunds", nil)   // Dropped due to out-of-funds
	queuedEvictionMeter  = metrics.NewRegisteredMeter("txpool/queued/eviction", nil)  // Dropped due to lifetime

	// General tx metrics
	knownTxMeter       = metrics.NewRegisteredMeter("txpool/known", nil)
	validTxMeter       = metrics.NewRegisteredMeter("txpool/valid", nil)
	invalidTxMeter     = metrics.NewRegisteredMeter("txpool/invalid", nil)
	underpricedTxMeter = metrics.NewRegisteredMeter("txpool/underpriced", nil)
	overflowedTxMeter  = metrics.NewRegisteredMeter("txpool/overflowed", nil)

	pendingGauge = metrics.NewRegisteredGauge("txpool/pending", nil)
	queuedGauge  = metrics.NewRegisteredGauge("txpool/queued", nil)
	localGauge   = metrics.NewRegisteredGauge("txpool/local", nil)
	slotsGauge   = metrics.NewRegisteredGauge("txpool/slots", nil)

	reheapTimer = metrics.NewRegisteredTimer("txpool/reheap", nil)
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
	GlobalSlots:  4096 + 1024, // urgent + floating queue capacity with 4:1 ratio
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
	if conf.AccountSlots < 1 {
		log.Warn("Sanitizing invalid txpool account slots", "provided", conf.AccountSlots, "updated", DefaultTxPoolConfig.AccountSlots)
		conf.AccountSlots = DefaultTxPoolConfig.AccountSlots
	}
	if conf.GlobalSlots < 1 {
		log.Warn("Sanitizing invalid txpool global slots", "provided", conf.GlobalSlots, "updated", DefaultTxPoolConfig.GlobalSlots)
		conf.GlobalSlots = DefaultTxPoolConfig.GlobalSlots
	}
	if conf.AccountQueue < 1 {
		log.Warn("Sanitizing invalid txpool account queue", "provided", conf.AccountQueue, "updated", DefaultTxPoolConfig.AccountQueue)
		conf.AccountQueue = DefaultTxPoolConfig.AccountQueue
	}
	if conf.GlobalQueue < 1 {
		log.Warn("Sanitizing invalid txpool global queue", "provided", conf.GlobalQueue, "updated", DefaultTxPoolConfig.GlobalQueue)
		conf.GlobalQueue = DefaultTxPoolConfig.GlobalQueue
	}
	if conf.Lifetime < 1 {
		log.Warn("Sanitizing invalid txpool lifetime", "provided", conf.Lifetime, "updated", DefaultTxPoolConfig.Lifetime)
		conf.Lifetime = DefaultTxPoolConfig.Lifetime
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
	config      TxPoolConfig
	chainconfig *params.ChainConfig
	chain       blockChain
	gasPrice    *big.Int
	txFeed      event.Feed
	scope       event.SubscriptionScope
	signer      types.Signer
	mu          sync.RWMutex

	istanbul bool // Fork indicator whether we are in the istanbul stage.
	eip2718  bool // Fork indicator whether we are using EIP-2718 type transactions.
	eip1559  bool // Fork indicator whether we are using EIP-1559 type transactions.

	currentState  *state.StateDB // Current state in the blockchain head
	pendingNonces *txNoncer      // Pending state tracking virtual nonces
	currentMaxGas uint64         // Current gas limit for transaction caps

	locals  *accountSet // Set of local transaction to exempt from eviction rules
	journal *txJournal  // Journal of local transaction to back up to disk

	pending map[common.Address]*txList   // All currently processable transactions
	queue   map[common.Address]*txList   // Queued but non-processable transactions
	beats   map[common.Address]time.Time // Last heartbeat from each known account
	all     *txLookup                    // All transactions to allow lookups
	priced  *txPricedList                // All transactions sorted by price

	chainHeadCh     chan ChainHeadEvent
	chainHeadSub    event.Subscription
	reqResetCh      chan *txpoolResetRequest
	reqPromoteCh    chan *accountSet
	queueTxEventCh  chan *types.Transaction
	reorgDoneCh     chan chan struct{}
	reorgShutdownCh chan struct{}  // requests shutdown of scheduleReorgLoop
	wg              sync.WaitGroup // tracks loop, scheduleReorgLoop
}

type txpoolResetRequest struct {
	oldHead, newHead *types.Header
}

// NewTxPool creates a new transaction pool to gather, sort and filter inbound
// transactions from the network.
func NewTxPool(config TxPoolConfig, chainconfig *params.ChainConfig, chain blockChain) *TxPool {
	// Sanitize the input to ensure no vulnerable gas prices are set
	config = (&config).sanitize()

	// Create the transaction pool with its initial settings
	pool := &TxPool{
		config:          config,
		chainconfig:     chainconfig,
		chain:           chain,
		signer:          types.LatestSigner(chainconfig),
		pending:         make(map[common.Address]*txList),
		queue:           make(map[common.Address]*txList),
		beats:           make(map[common.Address]time.Time),
		all:             newTxLookup(),
		chainHeadCh:     make(chan ChainHeadEvent, chainHeadChanSize),
		reqResetCh:      make(chan *txpoolResetRequest),
		reqPromoteCh:    make(chan *accountSet),
		queueTxEventCh:  make(chan *types.Transaction),
		reorgDoneCh:     make(chan chan struct{}),
		reorgShutdownCh: make(chan struct{}),
		gasPrice:        new(big.Int).SetUint64(config.PriceLimit),
	}
	pool.locals = newAccountSet(pool.signer)
	for _, addr := range config.Locals {
		log.Info("Setting new local account", "address", addr)
		pool.locals.add(addr)
	}
	pool.priced = newTxPricedList(pool.all)
	pool.reset(nil, chain.CurrentBlock().Header())

	// Start the reorg loop early so it can handle requests generated during journal loading.
	pool.wg.Add(1)
	go pool.scheduleReorgLoop()

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

	// Subscribe events from blockchain and start the main event loop.
	pool.chainHeadSub = pool.chain.SubscribeChainHeadEvent(pool.chainHeadCh)
	pool.wg.Add(1)
	go pool.loop()

	return pool
}

// loop is the transaction pool's main event loop, waiting for and reacting to
// outside blockchain events as well as for various reporting and transaction
// eviction events.
func (pool *TxPool) loop() {
	defer pool.wg.Done()

	var (
		prevPending, prevQueued, prevStales int
		// Start the stats reporting and transaction eviction tickers
		report  = time.NewTicker(statsReportInterval)
		evict   = time.NewTicker(evictionInterval)
		journal = time.NewTicker(pool.config.Rejournal)
		// Track the previous head headers for transaction reorgs
		head = pool.chain.CurrentBlock()
	)

	defer report.Stop()
	defer evict.Stop()
	defer journal.Stop()

	// Keep waiting for and reacting to the various events
	for {
		select {
		// Handle ChainHeadEvent
		case ev := <-pool.chainHeadCh:
			if ev.Block != nil {
				pool.requestReset(head.Header(), ev.Block.Header())
				head = ev.Block
			}
		// System shutdown.
		case <-pool.chainHeadSub.Err():
			close(pool.reorgShutdownCh)
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
					list := pool.queue[addr].Flatten()
					for _, tx := range list {
						pool.removeTx(tx.Hash(), true)
					}
					queuedEvictionMeter.Mark(int64(len(list)))
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

	old := pool.gasPrice
	pool.gasPrice = price
	// if the min miner fee increased, remove transactions below the new threshold
	if price.Cmp(old) > 0 {
		// pool.priced is sorted by GasFeeCap, so we have to iterate through pool.all instead
		drop := pool.all.RemotesBelowTip(price)
		for _, tx := range drop {
			pool.removeTx(tx.Hash(), false)
		}
		pool.priced.Removed(len(drop))
	}

	log.Info("Transaction pool price threshold updated", "price", price)
}

// Nonce returns the next nonce of an account, with all transactions executable
// by the pool already applied on top.
func (pool *TxPool) Nonce(addr common.Address) uint64 {
	pool.mu.RLock()
	defer pool.mu.RUnlock()

	return pool.pendingNonces.get(addr)
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
//
// The enforceTips parameter can be used to do an extra filtering on the pending
// transactions and only return those whose **effective** tip is large enough in
// the next pending execution environment.
func (pool *TxPool) Pending(enforceTips bool) map[common.Address]types.Transactions {
	pool.mu.Lock()
	defer pool.mu.Unlock()

	pending := make(map[common.Address]types.Transactions)
	for addr, list := range pool.pending {
		txs := list.Flatten()

		// If the miner requests tip enforcement, cap the lists now
		if enforceTips && !pool.locals.contains(addr) {
			for i, tx := range txs {
				// Fusion will give the base fee back to miner, so no need to minus the base fee when enforceTips is true
				if tx.EffectiveGasTipIntCmp(pool.gasPrice, nil) < 0 {
					txs = txs[:i]
					break
				}
			}
		}
		if len(txs) > 0 {
			pending[addr] = txs
		}
	}
	return pending
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
	// Accept only legacy transactions until EIP-2718/2930 activates.
	if !pool.eip2718 && tx.Type() != types.LegacyTxType {
		return ErrTxTypeNotSupported
	}
	// Reject dynamic fee transactions until EIP-1559 activates.
	if !pool.eip1559 && tx.Type() == types.DynamicFeeTxType {
		return ErrTxTypeNotSupported
	}
	// Reject transactions over defined size to prevent DOS attacks
	if uint64(tx.Size()) > txMaxSize {
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
	// Sanity check for extremely large numbers
	if tx.GasFeeCap().BitLen() > 256 {
		return ErrFeeCapVeryHigh
	}
	if tx.GasTipCap().BitLen() > 256 {
		return ErrTipVeryHigh
	}
	// Ensure gasFeeCap is greater than or equal to gasTipCap.
	if tx.GasFeeCapIntCmp(tx.GasTipCap()) < 0 {
		return ErrTipAboveFeeCap
	}
	// Make sure the transaction is signed properly
	from, err := types.Sender(pool.signer, tx)
	if err != nil {
		return ErrInvalidSender
	}
	// Drop non-local transactions under our own minimal accepted gas price
	if !local && tx.GasTipCapIntCmp(pool.gasPrice) < 0 {
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
	intrGas, err := IntrinsicGas(tx.Data(), tx.AccessList(), tx.To() == nil, true, pool.istanbul)
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
		knownTxMeter.Mark(1)
		return false, ErrAlreadyKnown
	}
	// Make the local flag. If it's from local source or it's from the network but
	// the sender is marked as local previously, treat it as the local transaction.
	isLocal := local || pool.locals.containsTx(tx)

	// If the transaction fails basic validation, discard it
	if err := pool.validateTx(tx, isLocal); err != nil {
		log.Trace("Discarding invalid transaction", "hash", hash, "err", err)
		invalidTxMeter.Mark(1)
		return false, err
	}
	// If the transaction is an invalid FsnCall tx, discard it
	if err := pool.validateAddFsnCallTx(tx); err != nil {
		invalidTxMeter.Mark(1)
		return false, err
	}
	// If the transaction pool is full, discard underpriced transactions
	if uint64(pool.all.Slots()+numSlots(tx)) >= pool.config.GlobalSlots+pool.config.GlobalQueue {
		// If the new transaction is underpriced, don't accept it
		if !local && pool.priced.Underpriced(tx) {
			log.Trace("Discarding underpriced transaction", "hash", hash, "gasTipCap", tx.GasTipCap(), "gasFeeCap", tx.GasFeeCap())
			underpricedTxMeter.Mark(1)
			return false, ErrUnderpriced
		}
		// New transaction is better than our worse ones, make room for it.
		// If it's a local transaction, forcibly discard all available transactions.
		// Otherwise if we can't make enough room for new one, abort the operation.
		drop, success := pool.priced.Discard(pool.all.Slots()-int(pool.config.GlobalSlots+pool.config.GlobalQueue)+numSlots(tx), isLocal)

		// Special case, we still can't make the room for the new remote one.
		if !isLocal && !success {
			log.Trace("Discarding overflown transaction", "hash", hash)
			overflowedTxMeter.Mark(1)
			return false, ErrTxPoolOverflow
		}
		// Kick out the underpriced remote transactions.
		for _, tx := range drop {
			log.Trace("Discarding freshly underpriced transaction", "hash", tx.Hash(), "gasTipCap", tx.GasTipCap(), "gasFeeCap", tx.GasFeeCap())
			underpricedTxMeter.Mark(1)
			pool.removeTx(tx.Hash(), false)
		}
	}
	// Try to replace an existing transaction in the pending pool
	from, _ := types.Sender(pool.signer, tx) // already validated
	if list := pool.pending[from]; list != nil && list.Overlaps(tx) {
		// Nonce already pending, check if required price bump is met
		inserted, old := list.Add(tx, pool.config.PriceBump)
		if !inserted {
			pendingDiscardMeter.Mark(1)
			return false, ErrReplaceUnderpriced
		}
		// New transaction is better, replace old one
		if old != nil {
			pool.all.Remove(old.Hash())
			pool.priced.Removed(1)
			pendingReplaceMeter.Mark(1)
		}
		pool.all.Add(tx, isLocal)
		pool.priced.Put(tx, isLocal)
		pool.journalTx(from, tx)
		pool.queueTxEvent(tx)
		log.Trace("Pooled new executable transaction", "hash", hash, "from", from, "to", tx.To())

		// Successful promotion, bump the heartbeat
		pool.beats[from] = time.Now()
		return old != nil, nil
	}
	// New transaction isn't replacing a pending one, push into queue
	replace, err := pool.enqueueTx(hash, tx, isLocal, true)
	if err != nil {
		return false, err
	}
	// Mark local addresses and journal local transactions
	if local && !pool.locals.contains(from) {
		log.Info("Setting new local account", "address", from)
		pool.locals.add(from)
		pool.priced.Removed(pool.all.RemoteToLocals(pool.locals)) // Migrate the remotes if it's marked as local first time.
	}
	if isLocal {
		localGauge.Inc(1)
	}
	pool.journalTx(from, tx)

	log.Trace("Pooled new future transaction", "hash", hash, "from", from, "to", tx.To())
	return replace, nil
}

// enqueueTx inserts a new transaction into the non-executable transaction queue.
//
// Note, this method assumes the pool lock is held!
func (pool *TxPool) enqueueTx(hash common.Hash, tx *types.Transaction, local bool, addAll bool) (bool, error) {
	// Try to insert the transaction into the future queue
	from, _ := types.Sender(pool.signer, tx) // already validated
	if pool.queue[from] == nil {
		pool.queue[from] = newTxList(false)
	}
	inserted, old := pool.queue[from].Add(tx, pool.config.PriceBump)
	if !inserted {
		// An older transaction was better, discard this
		queuedDiscardMeter.Mark(1)
		return false, ErrReplaceUnderpriced
	}
	// Discard any previous transaction and mark this
	if old != nil {
		pool.all.Remove(old.Hash())
		pool.priced.Removed(1)
		queuedReplaceMeter.Mark(1)
	} else {
		// Nothing was replaced, bump the queued counter
		queuedGauge.Inc(1)
	}
	// If the transaction isn't in lookup set but it's expected to be there,
	// show the error log.
	if pool.all.Get(hash) == nil && !addAll {
		log.Error("Missing transaction in lookup set, please report the issue", "hash", hash)
	}
	if addAll {
		pool.all.Add(tx, local)
		pool.priced.Put(tx, local)
	}
	// If we never record the heartbeat, do it right now.
	if _, exist := pool.beats[from]; !exist {
		pool.beats[from] = time.Now()
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
		pool.priced.Removed(1)
		pendingDiscardMeter.Mark(1)
		return false
	}
	// Otherwise discard any previous transaction and mark this
	if old != nil {
		pool.all.Remove(old.Hash())
		pool.priced.Removed(1)
		pendingReplaceMeter.Mark(1)
	} else {
		// Nothing was replaced, bump the pending counter
		pendingGauge.Inc(1)
	}
	// Set the potentially new pending nonce and notify any subsystems of the new tx
	pool.pendingNonces.set(addr, tx.Nonce()+1)

	// Successful promotion, bump the heartbeat
	pool.beats[addr] = time.Now()
	return true
}

// AddLocals enqueues a batch of transactions into the pool if they are valid, marking the
// senders as a local ones, ensuring they go around the local pricing constraints.
//
// This method is used to add transactions from the RPC API and performs synchronous pool
// reorganization and event propagation.
func (pool *TxPool) AddLocals(txs []*types.Transaction) []error {
	return pool.addTxs(txs, !pool.config.NoLocals, true)
}

// AddLocal enqueues a single local transaction into the pool if it is valid. This is
// a convenience wrapper aroundd AddLocals.
func (pool *TxPool) AddLocal(tx *types.Transaction) error {
	errs := pool.AddLocals([]*types.Transaction{tx})
	return errs[0]
}

// AddRemotes enqueues a batch of transactions into the pool if they are valid. If the
// senders are not among the locally tracked ones, full pricing constraints will apply.
//
// This method is used to add transactions from the p2p network and does not wait for pool
// reorganization and internal event propagation.
func (pool *TxPool) AddRemotes(txs []*types.Transaction) []error {
	return pool.addTxs(txs, false, false)
}

// AddRemote enqueues a single transaction into the pool if it is valid. This is a convenience
// wrapper around AddRemotes.
//
// Deprecated: use AddRemotes
func (pool *TxPool) AddRemote(tx *types.Transaction) error {
	errs := pool.AddRemotes([]*types.Transaction{tx})
	return errs[0]
}

// addTxs attempts to queue a batch of transactions if they are valid.
func (pool *TxPool) addTxs(txs []*types.Transaction, local, sync bool) []error {
	// Filter out known ones without obtaining the pool lock or recovering signatures
	var (
		errs = make([]error, len(txs))
		news = make([]*types.Transaction, 0, len(txs))
	)
	for i, tx := range txs {
		// If the transaction is known, pre-set the error slot
		if pool.all.Get(tx.Hash()) != nil {
			errs[i] = ErrAlreadyKnown
			knownTxMeter.Mark(1)
			continue
		}
		// Exclude transactions with invalid signatures as soon as
		// possible and cache senders in transactions before
		// obtaining lock
		_, err := types.Sender(pool.signer, tx)
		if err != nil {
			errs[i] = ErrInvalidSender
			invalidTxMeter.Mark(1)
			continue
		}
		// Accumulate all unknown transactions for deeper processing
		news = append(news, tx)
	}
	if len(news) == 0 {
		return errs
	}

	// Process all the new transaction and merge any errors into the original slice
	pool.mu.Lock()
	newErrs, dirtyAddrs := pool.addTxsLocked(news, local)
	pool.mu.Unlock()

	var nilSlot = 0
	for _, err := range newErrs {
		for errs[nilSlot] != nil {
			nilSlot++
		}
		errs[nilSlot] = err
		nilSlot++
	}
	// Reorg the pool internals if needed and return
	done := pool.requestPromoteExecutables(dirtyAddrs)
	if sync {
		<-done
	}
	return errs
}

// addTxsLocked attempts to queue a batch of transactions if they are valid.
// The transaction pool lock must be held.
func (pool *TxPool) addTxsLocked(txs []*types.Transaction, local bool) ([]error, *accountSet) {
	dirty := newAccountSet(pool.signer)
	errs := make([]error, len(txs))
	for i, tx := range txs {
		replaced, err := pool.add(tx, local)
		errs[i] = err
		if err == nil && !replaced {
			dirty.addTx(tx)
		}
	}
	validTxMeter.Mark(int64(len(dirty.accounts)))
	return errs, dirty
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

// Has returns an indicator whether txpool has a transaction cached with the
// given hash.
func (pool *TxPool) Has(hash common.Hash) bool {
	return pool.all.Get(hash) != nil
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
		pool.priced.Removed(1)
	}
	if pool.locals.contains(addr) {
		localGauge.Dec(1)
	}
	// Remove the transaction from the pending lists and reset the account nonce
	if pending := pool.pending[addr]; pending != nil {
		if removed, invalids := pending.Remove(tx); removed {
			// If no more pending transactions are left, remove the list
			if pending.Empty() {
				delete(pool.pending, addr)
			}
			// Postpone any invalidated transactions
			for _, tx := range invalids {
				// Internal shuffle shouldn't touch the lookup set.
				pool.enqueueTx(tx.Hash(), tx, false, false)
			}
			// Update the account nonce if needed
			pool.pendingNonces.setIfLower(addr, tx.Nonce())
			// Reduce the pending counter
			pendingGauge.Dec(int64(1 + len(invalids)))
			return
		}
	}
	// Transaction is in the future queue
	if future := pool.queue[addr]; future != nil {
		if removed, _ := future.Remove(tx); removed {
			// Reduce the queued counter
			queuedGauge.Dec(1)
		}
		if future.Empty() {
			delete(pool.queue, addr)
			delete(pool.beats, addr)
		}
	}
}

// requestReset requests a pool reset to the new head block.
// The returned channel is closed when the reset has occurred.
func (pool *TxPool) requestReset(oldHead *types.Header, newHead *types.Header) chan struct{} {
	select {
	case pool.reqResetCh <- &txpoolResetRequest{oldHead, newHead}:
		return <-pool.reorgDoneCh
	case <-pool.reorgShutdownCh:
		return pool.reorgShutdownCh
	}
}

// requestPromoteExecutables requests transaction promotion checks for the given addresses.
// The returned channel is closed when the promotion checks have occurred.
func (pool *TxPool) requestPromoteExecutables(set *accountSet) chan struct{} {
	select {
	case pool.reqPromoteCh <- set:
		return <-pool.reorgDoneCh
	case <-pool.reorgShutdownCh:
		return pool.reorgShutdownCh
	}
}

// queueTxEvent enqueues a transaction event to be sent in the next reorg run.
func (pool *TxPool) queueTxEvent(tx *types.Transaction) {
	select {
	case pool.queueTxEventCh <- tx:
	case <-pool.reorgShutdownCh:
	}
}

// scheduleReorgLoop schedules runs of reset and promoteExecutables. Code above should not
// call those methods directly, but request them being run using requestReset and
// requestPromoteExecutables instead.
func (pool *TxPool) scheduleReorgLoop() {
	defer pool.wg.Done()

	var (
		curDone       chan struct{} // non-nil while runReorg is active
		nextDone      = make(chan struct{})
		launchNextRun bool
		reset         *txpoolResetRequest
		dirtyAccounts *accountSet
		queuedEvents  = make(map[common.Address]*txSortedMap)
	)
	for {
		// Launch next background reorg if needed
		if curDone == nil && launchNextRun {
			// Run the background reorg and announcements
			go pool.runReorg(nextDone, reset, dirtyAccounts, queuedEvents)

			// Prepare everything for the next round of reorg
			curDone, nextDone = nextDone, make(chan struct{})
			launchNextRun = false

			reset, dirtyAccounts = nil, nil
			queuedEvents = make(map[common.Address]*txSortedMap)
		}

		select {
		case req := <-pool.reqResetCh:
			// Reset request: update head if request is already pending.
			if reset == nil {
				reset = req
			} else {
				reset.newHead = req.newHead
			}
			launchNextRun = true
			pool.reorgDoneCh <- nextDone

		case req := <-pool.reqPromoteCh:
			// Promote request: update address set if request is already pending.
			if dirtyAccounts == nil {
				dirtyAccounts = req
			} else {
				dirtyAccounts.merge(req)
			}
			launchNextRun = true
			pool.reorgDoneCh <- nextDone

		case tx := <-pool.queueTxEventCh:
			// Queue up the event, but don't schedule a reorg. It's up to the caller to
			// request one later if they want the events sent.
			addr, _ := types.Sender(pool.signer, tx)
			if _, ok := queuedEvents[addr]; !ok {
				queuedEvents[addr] = newTxSortedMap()
			}
			queuedEvents[addr].Put(tx)

		case <-curDone:
			curDone = nil

		case <-pool.reorgShutdownCh:
			// Wait for current run to finish.
			if curDone != nil {
				<-curDone
			}
			close(nextDone)
			return
		}
	}
}

// runReorg runs reset and promoteExecutables on behalf of scheduleReorgLoop.
func (pool *TxPool) runReorg(done chan struct{}, reset *txpoolResetRequest, dirtyAccounts *accountSet, events map[common.Address]*txSortedMap) {
	defer close(done)

	var promoteAddrs []common.Address
	if dirtyAccounts != nil && reset == nil {
		// Only dirty accounts need to be promoted, unless we're resetting.
		// For resets, all addresses in the tx queue will be promoted and
		// the flatten operation can be avoided.
		promoteAddrs = dirtyAccounts.flatten()
	}
	pool.mu.Lock()
	if reset != nil {
		// Reset from the old head to the new, rescheduling any reorged transactions
		pool.reset(reset.oldHead, reset.newHead)

		// Nonces were reset, discard any events that became stale
		for addr := range events {
			events[addr].Forward(pool.pendingNonces.get(addr))
			if events[addr].Len() == 0 {
				delete(events, addr)
			}
		}
		// Reset needs promote for all addresses
		promoteAddrs = make([]common.Address, 0, len(pool.queue))
		for addr := range pool.queue {
			promoteAddrs = append(promoteAddrs, addr)
		}
	}
	// Check for pending transactions for every account that sent new ones
	promoted := pool.promoteExecutables(promoteAddrs)

	// If a new block appeared, validate the pool of pending transactions. This will
	// remove any transaction that has been included in the block or was invalidated
	// because of another transaction (e.g. higher gas price).
	if reset != nil {
		pool.demoteUnexecutables()
		if reset.newHead != nil && pool.chainconfig.IsLondon(new(big.Int).Add(reset.newHead.Number, big.NewInt(1))) {
			pendingBaseFee := misc.CalcBaseFee(pool.chainconfig, reset.newHead)
			pool.priced.SetBaseFee(pendingBaseFee)
		}
	}
	// Ensure pool.queue and pool.pending sizes stay within the configured limits.
	pool.truncatePending()
	pool.truncateQueue()

	// Update all accounts to the latest known pending nonce
	for addr, list := range pool.pending {
		highestPending := list.LastElement()
		pool.pendingNonces.set(addr, highestPending.Nonce()+1)
	}
	pool.mu.Unlock()

	// Notify subsystems for newly added transactions
	for _, tx := range promoted {
		addr, _ := types.Sender(pool.signer, tx)
		if _, ok := events[addr]; !ok {
			events[addr] = newTxSortedMap()
		}
		events[addr].Put(tx)
	}
	if len(events) > 0 {
		var txs []*types.Transaction
		for _, set := range events {
			txs = append(txs, set.Flatten()...)
		}
		pool.txFeed.Send(NewTxsEvent{txs})
	}
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
			if rem == nil {
				// This can happen if a setHead is performed, where we simply discard the old
				// head from the chain.
				// If that is the case, we don't have the lost transactions any more, and
				// there's nothing to add
				if newNum >= oldNum {
					// If we reorged to a same or higher number, then it's not a case of setHead
					log.Warn("Transaction pool reset with missing oldhead",
						"old", oldHead.Hash(), "oldnum", oldNum, "new", newHead.Hash(), "newnum", newNum)
					return
				}
				// If the reorg ended up on a lower number, it's indicative of setHead being the cause
				log.Debug("Skipping transaction reset caused by setHead",
					"old", oldHead.Hash(), "oldnum", oldNum, "new", newHead.Hash(), "newnum", newNum)
				// We still need to update the current state s.th. the lost transactions can be readded by the user
			} else {
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
	pool.pendingNonces = newTxNoncer(statedb)
	pool.currentMaxGas = newHead.GasLimit

	// Inject any transactions discarded due to reorgs
	log.Debug("Reinjecting stale transactions", "count", len(reinject))
	senderCacher.recover(pool.signer, reinject)
	pool.addTxsLocked(reinject, false)

	// Update all fork indicator by next pending block number.
	next := new(big.Int).Add(newHead.Number, big.NewInt(1))
	pool.istanbul = pool.chainconfig.IsIstanbul(next)
	pool.eip2718 = pool.chainconfig.IsBerlin(next)
	pool.eip1559 = pool.chainconfig.IsLondon(next)
}

// promoteExecutables moves transactions that have become processable from the
// future queue to the set of pending transactions. During this process, all
// invalidated transactions (low nonce, low balance) are deleted.
func (pool *TxPool) promoteExecutables(accounts []common.Address) []*types.Transaction {
	// Track the promoted transactions to broadcast them at once
	var promoted []*types.Transaction

	// Iterate over all accounts and promote any executable transactions
	for _, addr := range accounts {
		list := pool.queue[addr]
		if list == nil {
			continue // Just in case someone calls with a non existing account
		}
		// Drop all transactions that are deemed too old (low nonce)
		forwards := list.Forward(pool.currentState.GetNonce(addr))
		for _, tx := range forwards {
			hash := tx.Hash()
			pool.all.Remove(hash)
		}
		log.Trace("Removed old queued transactions", "count", len(forwards))
		// Drop all transactions that are too costly (low balance or out of gas)
		drops, _ := list.Filter(pool.currentState.GetBalance(common.SystemAssetID, addr), pool.currentMaxGas)
		for _, tx := range drops {
			hash := tx.Hash()
			pool.all.Remove(hash)
		}
		log.Trace("Removed unpayable queued transactions", "count", len(drops))
		queuedNofundsMeter.Mark(int64(len(drops)))
		// Drop all FsnCall transactions that are invalid
		filter := func(tx *types.Transaction) bool {
			return pool.validateFsnCallTx(tx) != nil
		}
		removes, _ := list.FilterInvalid(filter)
		for _, tx := range removes {
			hash := tx.Hash()
			log.Trace("Removed invalid pending transaction", "hash", hash)
			pool.all.Remove(hash)
		}

		// Gather all executable transactions and promote them
		readies := list.Ready(pool.pendingNonces.get(addr))
		for _, tx := range readies {
			hash := tx.Hash()
			if pool.promoteTx(addr, hash, tx) {
				promoted = append(promoted, tx)
			}
		}
		log.Trace("Promoted queued transactions", "count", len(promoted))
		queuedGauge.Dec(int64(len(readies)))

		// Drop all transactions over the allowed limit
		var caps types.Transactions
		if !pool.locals.contains(addr) {
			caps = list.Cap(int(pool.config.AccountQueue))
			for _, tx := range caps {
				hash := tx.Hash()
				pool.all.Remove(hash)
				log.Trace("Removed cap-exceeding queued transaction", "hash", hash)
			}
			queuedRateLimitMeter.Mark(int64(len(caps)))
		}
		// Mark all the items dropped as removed
		pool.priced.Removed(len(forwards) + len(drops) + len(caps))
		queuedGauge.Dec(int64(len(forwards) + len(drops) + len(caps)))
		if pool.locals.contains(addr) {
			localGauge.Dec(int64(len(forwards) + len(drops) + len(caps)))
		}
		// Delete the entire queue entry if it became empty.
		if list.Empty() {
			delete(pool.queue, addr)
			delete(pool.beats, addr)
		}
	}
	return promoted
}

// truncatePending removes transactions from the pending queue if the pool is above the
// pending limit. The algorithm tries to reduce transaction counts by an approximately
// equal number for all for accounts with many pending transactions.
func (pool *TxPool) truncatePending() {
	pending := uint64(0)
	for _, list := range pool.pending {
		pending += uint64(list.Len())
	}
	if pending <= pool.config.GlobalSlots {
		return
	}

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

					caps := list.Cap(list.Len() - 1)
					for _, tx := range caps {
						// Drop the transaction from the global pools too
						hash := tx.Hash()
						pool.all.Remove(hash)

						// Update the account nonce to the dropped transaction
						pool.pendingNonces.setIfLower(offenders[i], tx.Nonce())
						log.Trace("Removed fairness-exceeding pending transaction", "hash", hash)
					}
					pool.priced.Removed(len(caps))
					pendingGauge.Dec(int64(len(caps)))
					if pool.locals.contains(offenders[i]) {
						localGauge.Dec(int64(len(caps)))
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

				caps := list.Cap(list.Len() - 1)
				for _, tx := range caps {
					// Drop the transaction from the global pools too
					hash := tx.Hash()
					pool.all.Remove(hash)

					// Update the account nonce to the dropped transaction
					pool.pendingNonces.setIfLower(addr, tx.Nonce())
					log.Trace("Removed fairness-exceeding pending transaction", "hash", hash)
				}
				pool.priced.Removed(len(caps))
				pendingGauge.Dec(int64(len(caps)))
				if pool.locals.contains(addr) {
					localGauge.Dec(int64(len(caps)))
				}
				pending--
			}
		}
	}
	pendingRateLimitMeter.Mark(int64(pendingBeforeCap - pending))
}

// truncateQueue drops the oldes transactions in the queue if the pool is above the global queue limit.
func (pool *TxPool) truncateQueue() {
	queued := uint64(0)
	for _, list := range pool.queue {
		queued += uint64(list.Len())
	}
	if queued <= pool.config.GlobalQueue {
		return
	}

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
			queuedRateLimitMeter.Mark(int64(size))
			continue
		}
		// Otherwise drop only last few transactions
		txs := list.Flatten()
		for i := len(txs) - 1; i >= 0 && drop > 0; i-- {
			pool.removeTx(txs[i].Hash(), true)
			drop--
			queuedRateLimitMeter.Mark(1)
		}
	}
}

// demoteUnexecutables removes invalid and processed transactions from the pools
// executable/pending queue and any subsequent transactions that become unexecutable
// are moved back into the future queue.
//
// Note: transactions are not marked as removed in the priced list because re-heaping
// is always explicitly triggered by SetBaseFee and it would be unnecessary and wasteful
// to trigger a re-heap is this function
func (pool *TxPool) demoteUnexecutables() {
	// Iterate over all accounts and demote any non-executable transactions
	for addr, list := range pool.pending {
		nonce := pool.currentState.GetNonce(addr)

		// Drop all transactions that are deemed too old (low nonce)
		olds := list.Forward(nonce)
		for _, tx := range olds {
			hash := tx.Hash()
			pool.all.Remove(hash)
			log.Trace("Removed old pending transaction", "hash", hash)
		}
		// Drop all FsnCall transactions that are invalid
		filter := func(tx *types.Transaction) bool {
			return pool.validateFsnCallTx(tx) != nil
		}
		removes, adjusts := list.FilterInvalid(filter)
		for _, tx := range removes {
			hash := tx.Hash()
			pool.all.Remove(hash)
			log.Trace("Removed invalid pending transaction", "hash", hash)
		}
		// Drop all transactions that are too costly (low balance or out of gas), and queue any invalids back for later
		drops, invalids := list.Filter(pool.currentState.GetBalance(common.SystemAssetID, addr), pool.currentMaxGas)
		for _, tx := range drops {
			hash := tx.Hash()
			log.Trace("Removed unpayable pending transaction", "hash", hash)
			pool.all.Remove(hash)
		}
		pendingNofundsMeter.Mark(int64(len(drops)))

		for _, tx := range invalids {
			hash := tx.Hash()
			log.Trace("Demoting pending transaction", "hash", hash)

			// Internal shuffle shouldn't touch the lookup set.
			pool.enqueueTx(hash, tx, false, false)
		}
		pendingGauge.Dec(int64(len(olds) + len(drops) + len(invalids)))
		if pool.locals.contains(addr) {
			localGauge.Dec(int64(len(olds) + len(drops) + len(invalids)))
		}
		if adjusts.Len() > 0 {
			adjusts = types.TxDifference(adjusts, drops)
			adjusts = types.TxDifference(adjusts, invalids)
			for _, tx := range adjusts {
				hash := tx.Hash()
				log.Trace("Demoting pending transaction", "hash", hash)
				pool.enqueueTx(hash, tx, false, false)
			}
		}
		// If there's a gap in front, alert (should never happen) and postpone all transactions
		if list.Len() > 0 && list.txs.Get(nonce) == nil {
			gapped := list.Cap(0)
			for _, tx := range gapped {
				hash := tx.Hash()
				log.Error("Demoting invalidated transaction", "hash", hash)

				// Internal shuffle shouldn't touch the lookup set.
				pool.enqueueTx(hash, tx, false, false)
			}
			pendingGauge.Dec(int64(len(gapped)))
			// This might happen in a reorg, so log it to the metering
			blockReorgInvalidatedTx.Mark(int64(len(gapped)))
		}
		// Delete the entire queue entry if it became empty.
		if list.Empty() {
			delete(pool.pending, addr)
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
func newAccountSet(signer types.Signer, addrs ...common.Address) *accountSet {
	as := &accountSet{
		accounts: make(map[common.Address]struct{}),
		signer:   signer,
	}
	for _, addr := range addrs {
		as.add(addr)
	}
	return as
}

// contains checks if a given address is contained within the set.
func (as *accountSet) contains(addr common.Address) bool {
	_, exist := as.accounts[addr]
	return exist
}

func (as *accountSet) empty() bool {
	return len(as.accounts) == 0
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

// addTx adds the sender of tx into the set.
func (as *accountSet) addTx(tx *types.Transaction) {
	if addr, err := types.Sender(as.signer, tx); err == nil {
		as.add(addr)
	}
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

// merge adds all addresses from the 'other' set into 'as'.
func (as *accountSet) merge(other *accountSet) {
	for addr := range other.accounts {
		as.accounts[addr] = struct{}{}
	}
	as.cache = nil
}

// txLookup is used internally by TxPool to track transactions while allowing
// lookup without mutex contention.
//
// Note, although this type is properly protected against concurrent access, it
// is **not** a type that should ever be mutated or even exposed outside of the
// transaction pool, since its internal state is tightly coupled with the pools
// internal mechanisms. The sole purpose of the type is to permit out-of-bound
// peeking into the pool in TxPool.Get without having to acquire the widely scoped
// TxPool.mu mutex.
//
// This lookup set combines the notion of "local transactions", which is useful
// to build upper-level structure.
type txLookup struct {
	slots   int
	lock    sync.RWMutex
	locals  map[common.Hash]*types.Transaction
	remotes map[common.Hash]*types.Transaction

	ticketTxBeats map[common.Hash]time.Time // heartbeat from each buy ticket transactions
}

// newTxLookup returns a new txLookup structure.
func newTxLookup() *txLookup {
	return &txLookup{
		locals:        make(map[common.Hash]*types.Transaction),
		remotes:       make(map[common.Hash]*types.Transaction),
		ticketTxBeats: make(map[common.Hash]time.Time),
	}
}

// Range calls f on each key and value present in the map. The callback passed
// should return the indicator whether the iteration needs to be continued.
// Callers need to specify which set (or both) to be iterated.
func (t *txLookup) Range(f func(hash common.Hash, tx *types.Transaction, local bool) bool, local bool, remote bool) {
	t.lock.RLock()
	defer t.lock.RUnlock()

	if local {
		for key, value := range t.locals {
			if !f(key, value, true) {
				return
			}
		}
	}
	if remote {
		for key, value := range t.remotes {
			if !f(key, value, false) {
				return
			}
		}
	}
}

// Get returns a transaction if it exists in the lookup, or nil if not found.
func (t *txLookup) Get(hash common.Hash) *types.Transaction {
	t.lock.RLock()
	defer t.lock.RUnlock()

	if tx := t.locals[hash]; tx != nil {
		return tx
	}
	return t.remotes[hash]
}

// GetLocal returns a transaction if it exists in the lookup, or nil if not found.
func (t *txLookup) GetLocal(hash common.Hash) *types.Transaction {
	t.lock.RLock()
	defer t.lock.RUnlock()

	return t.locals[hash]
}

// GetRemote returns a transaction if it exists in the lookup, or nil if not found.
func (t *txLookup) GetRemote(hash common.Hash) *types.Transaction {
	t.lock.RLock()
	defer t.lock.RUnlock()

	return t.remotes[hash]
}

func (t *txLookup) GetByPredicate(predicate func(*types.Transaction) bool) *types.Transaction {
	t.lock.RLock()
	defer t.lock.RUnlock()

	for _, tx := range t.locals {
		if predicate(tx) {
			return tx
		}
	}
	for _, tx := range t.remotes {
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

	return len(t.locals) + len(t.remotes)
}

// LocalCount returns the current number of local transactions in the lookup.
func (t *txLookup) LocalCount() int {
	t.lock.RLock()
	defer t.lock.RUnlock()

	return len(t.locals)
}

// RemoteCount returns the current number of remote transactions in the lookup.
func (t *txLookup) RemoteCount() int {
	t.lock.RLock()
	defer t.lock.RUnlock()

	return len(t.remotes)
}

// Slots returns the current number of slots used in the lookup.
func (t *txLookup) Slots() int {
	t.lock.RLock()
	defer t.lock.RUnlock()

	return t.slots
}

// Add adds a transaction to the lookup.
func (t *txLookup) Add(tx *types.Transaction, local bool) {
	t.lock.Lock()
	defer t.lock.Unlock()

	t.slots += numSlots(tx)
	slotsGauge.Update(int64(t.slots))

	if local {
		t.locals[tx.Hash()] = tx
	} else {
		t.remotes[tx.Hash()] = tx
	}

	if tx.IsBuyTicketTx() {
		t.ticketTxBeats[tx.Hash()] = time.Now()
	}
}

// Remove removes a transaction from the lookup.
func (t *txLookup) Remove(hash common.Hash) {
	t.lock.Lock()
	defer t.lock.Unlock()

	tx, ok := t.locals[hash]
	if !ok {
		tx, ok = t.remotes[hash]
	}
	if !ok {
		log.Error("No transaction found to be deleted", "hash", hash)
		return
	}
	t.slots -= numSlots(tx)
	slotsGauge.Update(int64(t.slots))

	delete(t.locals, hash)
	delete(t.remotes, hash)
}

// RemoteToLocals migrates the transactions belongs to the given locals to locals
// set. The assumption is held the locals set is thread-safe to be used.
func (t *txLookup) RemoteToLocals(locals *accountSet) int {
	t.lock.Lock()
	defer t.lock.Unlock()

	var migrated int
	for hash, tx := range t.remotes {
		if locals.containsTx(tx) {
			t.locals[hash] = tx
			delete(t.remotes, hash)
			migrated += 1
		}
	}
	return migrated
}

// RemotesBelowTip finds all remote transactions below the given tip threshold.
func (t *txLookup) RemotesBelowTip(threshold *big.Int) types.Transactions {
	found := make(types.Transactions, 0, 128)
	t.Range(func(hash common.Hash, tx *types.Transaction, local bool) bool {
		if tx.GasTipCapIntCmp(threshold) < 0 {
			found = append(found, tx)
		}
		return true
	}, false, true) // Only iterate remotes
	return found
}

// numSlots calculates the number of slots needed for a single transaction.
func numSlots(tx *types.Transaction) int {
	return int((tx.Size() + txSlotSize - 1) / txSlotSize)
}

func (pool *TxPool) validateAddFsnCallTx(tx *types.Transaction) error {
	if err := pool.validateFsnCallTx(tx); err != nil {
		return err
	}
	if tx.IsBuyTicketTx() {
		from, _ := types.Sender(pool.signer, tx) // already validated
		found := false
		var oldTxHash common.Hash
		pool.all.Range(func(hash common.Hash, tx1 *types.Transaction, local bool) bool {
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
		}, true, true)
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
	from, err := types.Sender(pool.signer, tx)
	if err != nil {
		return fmt.Errorf("validateFsnCallTx err:%v", err)
	}
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
		assetID := tx.GetAssetId()
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
		swapId := tx.GetAssetId()

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
		swapID := tx.GetAssetId()

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
