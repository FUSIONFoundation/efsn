package datong

import (
	"bytes"
	"errors"
	"math/big"
	"sync"
	"time"

	"github.com/FusionFoundation/efsn/accounts"
	"github.com/FusionFoundation/efsn/common"
	"github.com/FusionFoundation/efsn/consensus"
	"github.com/FusionFoundation/efsn/core/state"
	"github.com/FusionFoundation/efsn/core/types"
	"github.com/FusionFoundation/efsn/crypto"
	"github.com/FusionFoundation/efsn/crypto/sha3"
	"github.com/FusionFoundation/efsn/log"
	"github.com/FusionFoundation/efsn/params"
	"github.com/FusionFoundation/efsn/rlp"
	"github.com/FusionFoundation/efsn/rpc"
)

var (
	errUnknownBlock = errors.New("unknown block")

	errMissingVanity = errors.New("extra-data 32 byte vanity prefix missing")

	errMissingSignature = errors.New("extra-data 65 byte suffix signature missing")

	errInvalidUncleHash = errors.New("non empty uncle hash")

	errUnauthorized = errors.New("unauthorized")
)

// SignerFn is a signer callback function to request a hash to be signed by a
// backing account.
type SignerFn func(accounts.Account, []byte) ([]byte, error)

var (
	extraVanity = 32
	extraSeal   = 65
)

var (
	emptyUncleHash = types.CalcUncleHash(nil)
)

// DaTong wacom
type DaTong struct {
	config *params.DaTongConfig
	signer common.Address
	signFn SignerFn
	lock   sync.RWMutex
}

// New wacom
func New(config *params.DaTongConfig) *DaTong {
	return &DaTong{
		config: config,
	}
}

// Authorize wacom
func (dt *DaTong) Authorize(signer common.Address, signFn SignerFn) {
	dt.lock.Lock()
	defer dt.lock.Unlock()
	dt.signer = signer
	dt.signFn = signFn
}

// Author retrieves the Ethereum address of the account that minted the given
// block, which may be different from the header's coinbase if a consensus
// engine is based on signatures.
func (dt *DaTong) Author(header *types.Header) (common.Address, error) {
	return header.Coinbase, nil
}

// VerifyHeader checks whether a header conforms to the consensus rules of the
// stock Ethereum ethash engine.
func (dt *DaTong) VerifyHeader(chain consensus.ChainReader, header *types.Header, seal bool) error {
	if header.Number == nil {
		return errUnknownBlock
	}

	if len(header.Extra) < extraVanity {
		return errMissingVanity
	}
	if len(header.Extra) < extraVanity+extraSeal {
		return errMissingSignature
	}

	if _, err := newSnapshotWithData(getSnapDataByHeader(header)); err != nil {
		return err
	}

	if header.UncleHash != emptyUncleHash {
		return errInvalidUncleHash
	}

	if header.Time.Cmp(big.NewInt(time.Now().Unix())) > 0 {
		return consensus.ErrFutureBlock
	}
	return nil
}

// VerifyHeaders is similar to VerifyHeader, but verifies a batch of headers
// concurrently. The method returns a quit channel to abort the operations and
// a results channel to retrieve the async verifications.
func (dt *DaTong) VerifyHeaders(chain consensus.ChainReader, headers []*types.Header, seals []bool) (chan<- struct{}, <-chan error) {
	abort := make(chan struct{})
	results := make(chan error, len(headers))
	go func() {
		for i, header := range headers {
			err := dt.VerifyHeader(chain, header, seals[i])
			select {
			case <-abort:
				return
			case results <- err:
			}
		}
	}()
	return abort, results
}

// VerifyUncles verifies that the given block's uncles conform to the consensus
// rules of the stock Ethereum ethash engine.
func (dt *DaTong) VerifyUncles(chain consensus.ChainReader, block *types.Block) error {
	if len(block.Uncles()) > 0 {
		return errors.New("uncles not allowed")
	}
	return nil
}

// VerifySeal checks whether the crypto seal on a header is valid according to
// the consensus rules of the given engine.
func (dt *DaTong) VerifySeal(chain consensus.ChainReader, header *types.Header) error {
	number := header.Number.Uint64()
	if number == 0 {
		return errUnknownBlock
	}
	parent := chain.GetHeader(header.ParentHash, number-1)
	if parent == nil {
		return consensus.ErrUnknownAncestor
	}
	snap, err := newSnapshotWithData(getSnapDataByHeader(header))
	if err != nil {
		return err
	}
	signature := header.Extra[len(header.Extra)-extraSeal:]
	pubkey, err := crypto.Ecrecover(sigHash(header).Bytes(), signature)
	if err != nil {
		return err
	}
	var signer common.Address
	copy(signer[:], crypto.Keccak256(pubkey[1:])[12:])
	ticketID := snap.GetVoteTicket()
	ticketMap := dt.getAllTickets(chain, header)

	if _, ok := ticketMap[ticketID]; !ok {
		return errors.New("Ticket not found")
	}
	ticket := ticketMap[ticketID]

	if ticket.Owner != signer {
		return errors.New("Ticket owner not be the signer")
	}

	tickets := make([]*common.Ticket, len(ticketMap))
	selected := false
	for _, v := range ticketMap {
		tickets = append(tickets, &v)
	}

	selectedTickets := dt.selectTickets(tickets, parent, header.Time.Uint64())
	for _, v := range selectedTickets {
		if v.ID == ticketID {
			selected = true
			break
		}
	}

	if !selected {
		return errors.New("the ticket not selected")
	}

	return nil
}

// Prepare initializes the consensus fields of a block header according to the
// rules of a particular engine. The changes are executed inline.
func (dt *DaTong) Prepare(chain consensus.ChainReader, header *types.Header) error {
	header.Coinbase = common.BytesToAddress(dt.signer[:])
	number := header.Number.Uint64()
	parent := chain.GetHeader(header.ParentHash, number-1)
	if parent == nil {
		return consensus.ErrUnknownAncestor
	}
	if len(header.Extra) < extraVanity {
		header.Extra = append(header.Extra, bytes.Repeat([]byte{0x00}, extraVanity-len(header.Extra))...)
	}
	header.Extra = header.Extra[:extraVanity]
	header.Difficulty = dt.CalcDifficulty(chain, parent.Time.Uint64(), parent)
	return nil
}

// Finalize runs any post-transaction state modifications (e.g. block rewards)
// and assembles the final block.
// Note: The block header and state database might be updated to reflect any
// consensus rules that happen at finalization (e.g. block rewards).
func (dt *DaTong) Finalize(chain consensus.ChainReader, header *types.Header, state *state.StateDB, txs []*types.Transaction,
	uncles []*types.Header, receipts []*types.Receipt) (*types.Block, error) {
	parent := chain.GetHeader(header.ParentHash, header.Number.Uint64()-1)
	if parent == nil {
		return nil, consensus.ErrUnknownAncestor
	}
	ticketMap := state.AllTickets()
	tickets := make([]*common.Ticket, len(ticketMap))
	haveTicket := false
	for _, v := range ticketMap {
		if v.Owner == header.Coinbase {
			haveTicket = true
		}
		tickets = append(tickets, &v)
	}

	if !haveTicket {
		return nil, errors.New("Miner don't have ticket")
	}
	time := uint64(time.Now().Unix())
	var (
		selected *common.Ticket
		retreat  []*common.Ticket
	)
	for {
		retreat = make([]*common.Ticket, 0)
		s := dt.selectTickets(tickets, parent, time)
		for _, t := range s {
			if t.Owner == header.Coinbase {
				selected = t
				break
			} else {
				retreat = append(retreat, t)
			}
		}
		if selected != nil {
			break
		}
		time++
	}
	header.Time = new(big.Int).SetUint64(time)

	snap := newSnapshot()

	delete(ticketMap, selected.ID)
	state.RemoveTicket(selected.ID)
	value := common.NewTimeLock(&common.TimeLockItem{
		StartTime: time,
		EndTime:   selected.ExpireTime,
		Value:     selected.Value,
	})
	state.AddTimeLockBalance(header.Coinbase, common.SystemAssetID, value)
	snap.AddLog(&ticketLog{
		TicketID: selected.ID,
		Type:     ticketSelect,
	})

	for _, t := range retreat {
		delete(ticketMap, t.ID)
		state.RemoveTicket(t.ID)
		value := common.NewTimeLock(&common.TimeLockItem{
			StartTime: time,
			EndTime:   t.ExpireTime,
			Value:     t.Value,
		})
		state.AddTimeLockBalance(t.Owner, common.SystemAssetID, value)
		snap.AddLog(&ticketLog{
			TicketID: t.ID,
			Type:     ticketRetreat,
		})
	}

	var remaining int64
	for _, t := range ticketMap {
		if t.ExpireTime <= time {
			delete(ticketMap, t.ID)
			state.RemoveTicket(t.ID)
			value := common.NewTimeLock(&common.TimeLockItem{
				StartTime: time,
				EndTime:   t.ExpireTime,
				Value:     t.Value,
			})
			state.AddTimeLockBalance(t.Owner, common.SystemAssetID, value)
			snap.AddLog(&ticketLog{
				TicketID: t.ID,
				Type:     ticketExpired,
			})
		} else {
			remaining++
		}
	}
	weight := dt.calcGenTicketNumber(state, txs)

	weight = weight.Add(weight, big.NewInt(remaining))

	if weight.Cmp(common.Big0) <= 0 {
		return nil, errors.New("Next block don't have ticket, wait buy ticket")
	}
	snap.SetWeight(weight)
	snapBytes := snap.Bytes()
	header.Extra = append(header.Extra, snapBytes...)
	header.Extra = append(header.Extra, make([]byte, extraSeal)...)
	state.AddBalance(header.Coinbase, common.SystemAssetID, calcRewards(header.Number))

	header.Root = state.IntermediateRoot(chain.Config().IsEIP158(header.Number))
	header.UncleHash = types.CalcUncleHash(nil)
	return types.NewBlock(header, txs, nil, receipts), nil
}

// Seal generates a new sealing request for the given input block and pushes
// the result into the given channel.
//
// Note, the method returns immediately and will send the result async. More
// than one result may also be returned depending on the consensus algorothm.
func (dt *DaTong) Seal(chain consensus.ChainReader, block *types.Block, results chan<- *types.Block, stop <-chan struct{}) error {
	header := block.Header()

	number := header.Number.Uint64()
	if number == 0 {
		return errUnknownBlock
	}
	dt.lock.RLock()
	signer, signFn := dt.signer, dt.signFn
	dt.lock.RUnlock()

	if signer != header.Coinbase {
		return errors.New("Mismatched Signer and Coinbase")
	}

	sighash, err := signFn(accounts.Account{Address: header.Coinbase}, sigHash(header).Bytes())
	if err != nil {
		return err
	}
	copy(header.Extra[len(header.Extra)-extraSeal:], sighash)

	delay := time.Unix(header.Time.Int64(), 0).Sub(time.Now())
	go func() {
		select {
		case <-stop:
			return
		case <-time.After(delay):
		}

		select {
		case results <- block.WithSeal(header):
		default:
			log.Warn("Sealing result is not read by miner", "sealhash", dt.SealHash(header))
		}
	}()

	return nil
}

// SealHash returns the hash of a block prior to it being sealed.
func (dt *DaTong) SealHash(header *types.Header) (hash common.Hash) {
	return sigHash(header)
}

// CalcDifficulty is the difficulty adjustment algorithm. It returns the difficulty
// that a new block should have.
func (dt *DaTong) CalcDifficulty(chain consensus.ChainReader, time uint64, parent *types.Header) *big.Int {
	snapData := getSnapDataByHeader(parent)
	snap, err := newSnapshotWithData(snapData)
	if err != nil {
		return nil
	}
	return calcDifficulty(snap)
}

// APIs returns the RPC APIs this consensus engine provides.
func (dt *DaTong) APIs(chain consensus.ChainReader) []rpc.API {
	return []rpc.API{}
}

// Close terminates any background threads maintained by the consensus engine.
func (dt *DaTong) Close() error {
	return nil
}

func (dt *DaTong) getAllTickets(chain consensus.ChainReader, header *types.Header) map[common.Hash]common.Ticket {
	return nil
}

func (dt *DaTong) selectTickets(tickets []*common.Ticket, parent *types.Header, time uint64) []*common.Ticket {
	return nil
}

func (dt *DaTong) calcGenTicketNumber(state *state.StateDB, txs []*types.Transaction) *big.Int {
	return big.NewInt(0)
}

func sigHash(header *types.Header) (hash common.Hash) {
	hasher := sha3.NewKeccak256()
	rlp.Encode(hasher, []interface{}{
		header.ParentHash,
		header.UncleHash,
		header.Coinbase,
		header.Root,
		header.TxHash,
		header.ReceiptHash,
		header.Bloom,
		header.Difficulty,
		header.Number,
		header.GasLimit,
		header.GasUsed,
		header.Time,
		header.Extra[:len(header.Extra)-extraSeal],
		header.MixDigest,
		header.Nonce,
	})
	hasher.Sum(hash[:0])
	return hash
}

func getSnapDataByHeader(header *types.Header) []byte {
	return getSnapData(header.Extra)
}

func getSnapData(data []byte) []byte {
	extraSuffix := len(data) - extraSeal
	return data[extraVanity:extraSuffix]
}

func calcDifficulty(snap *snapshot) *big.Int {
	return snap.Weight()
}

func calcRewards(height *big.Int) *big.Int {
	var i int64
	div2 := big.NewInt(2)
	// initial reward 2.5
	var reward = new(big.Int).Mul(big.NewInt(25), big.NewInt(10000000000000000))
	// every 4915200 blocks divide reward by 2
	segment := new(big.Int).Div(height, new(big.Int).SetUint64(4915200))
	for i = 0; i < segment.Int64(); i++ {
		reward = new(big.Int).Div(reward, div2)
	}
	return reward
}
