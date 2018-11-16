package datong

import (
	"errors"
	"math/big"

	"github.com/FusionFoundation/efsn/common"
	"github.com/FusionFoundation/efsn/consensus"
	"github.com/FusionFoundation/efsn/core/state"
	"github.com/FusionFoundation/efsn/core/types"
	"github.com/FusionFoundation/efsn/crypto/sha3"
	"github.com/FusionFoundation/efsn/ethdb"
	"github.com/FusionFoundation/efsn/log"
	"github.com/FusionFoundation/efsn/params"
	"github.com/FusionFoundation/efsn/rlp"
	"github.com/FusionFoundation/efsn/rpc"
)

// DaTong wacom
type DaTong struct {
	config     *params.DaTongConfig
	db         ethdb.Database
	stateCache state.Database
}

// New wacom
func New(config *params.DaTongConfig, db ethdb.Database) *DaTong {
	return &DaTong{
		config:     config,
		db:         db,
		stateCache: state.NewDatabase(db),
	}
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
	parent := chain.GetHeader(header.ParentHash, header.Number.Uint64()-1)
	if parent == nil {
		return consensus.ErrUnknownAncestor
	}
	point := dt.calcProofPoint(parent, header.Time)
	ticketID := common.BytesToHash(header.Extra)
	statedb, err := state.New(parent.Root, dt.stateCache)
	if err != nil {
		return err
	}
	tickets := statedb.AllTickets()
	if _, ok := tickets[ticketID]; !ok {
		return errors.New("Ticket not found")
	}
	ticket := tickets[ticketID]
	return dt.verifyTicket(point, header.Number, header.Difficulty, &ticket)
}

// VerifyHeaders is similar to VerifyHeader, but verifies a batch of headers
// concurrently. The method returns a quit channel to abort the operations and
// a results channel to retrieve the async verifications.
func (dt *DaTong) VerifyHeaders(chain consensus.ChainReader, headers []*types.Header, seals []bool) (chan<- struct{}, <-chan error) {
	abort := make(chan struct{})
	results := make(chan error, len(headers))
	go func() {
		for _, header := range headers {
			err := dt.VerifyHeader(chain, header, false)
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
		return errors.New("unknown block")
	}
	return nil
}

// Prepare initializes the consensus fields of a block header according to the
// rules of a particular engine. The changes are executed inline.
func (dt *DaTong) Prepare(chain consensus.ChainReader, header *types.Header) error {
	parent := chain.GetHeader(header.ParentHash, header.Number.Uint64()-1)
	if parent == nil {
		return consensus.ErrUnknownAncestor
	}
	header.Difficulty = dt.CalcDifficulty(chain, header.Time.Uint64(), parent)
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
	ticketID := common.BytesToHash(header.Extra)
	tickets := state.AllTickets()
	if _, ok := tickets[ticketID]; !ok {
		return nil, errors.New("Ticket not found")
	}
	ticket := tickets[ticketID]
	point := dt.calcProofPoint(parent, header.Time)
	if err := dt.verifyTicket(point, header.Number, header.Difficulty, &ticket); err != nil {
		return nil, err
	}
	checkTickets := make([]*common.Ticket, 0)
	for _, v := range tickets {
		if v.ExpireTime <= header.Time.Uint64() {
			state.RemoveTicket(v.ID)
		} else if v.ID != ticketID && v.Owner != header.Coinbase {
			checkTickets = append(checkTickets, &v)
		}
	}
	checkedTickets := dt.selectTickets(point, header.Number, header.Difficulty, checkTickets)
	for _, v := range checkedTickets {
		state.RemoveTicket(v.ID)
		value := common.NewTimeLock(&common.TimeLockItem{
			StartTime: header.Time.Uint64(),
			EndTime:   v.ExpireTime,
			Value:     v.Value,
		})
		state.AddTimeLockBalance(v.Owner, common.SystemAssetID, value)
	}

	state.RemoveTicket(ticket.ID)
	value := common.NewTimeLock(&common.TimeLockItem{
		StartTime: header.Time.Uint64(),
		EndTime:   ticket.ExpireTime,
		Value:     ticket.Value,
	})
	state.AddTimeLockBalance(ticket.Owner, common.SystemAssetID, value)

	state.AddBalance(header.Coinbase, common.SystemAssetID, dt.calcRewards(header.Number))
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
		return errors.New("unknown block")
	}

	parent := chain.GetHeader(header.ParentHash, header.Number.Uint64()-1)
	if parent == nil {
		return consensus.ErrUnknownAncestor
	}

	statedb, err := state.New(block.Root(), dt.stateCache)
	if err != nil {
		return err
	}
	allTickets := statedb.AllTickets()
	tickets := make([]*common.Ticket, 0)
	for _, v := range allTickets {
		if v.Owner == header.Coinbase {
			tickets = append(tickets, &v)
		}
	}

	found := make(chan *types.Block)
	go func() {
		dt.mine(block, parent, tickets, found)
	}()
	go func() {
		var result *types.Block
		select {
		case <-stop:
			return
		case result = <-found:
			select {
			case results <- result:
			default:
				log.Warn("Sealing result is not read by miner", "mode", "local", "sealhash", dt.SealHash(block.Header()))
			}
		}
	}()
	return nil
}

// SealHash returns the hash of a block prior to it being sealed.
func (dt *DaTong) SealHash(header *types.Header) (hash common.Hash) {
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
		header.Extra,
	})
	hasher.Sum(hash[:0])
	return hash
}

// CalcDifficulty is the difficulty adjustment algorithm. It returns the difficulty
// that a new block should have.
func (dt *DaTong) CalcDifficulty(chain consensus.ChainReader, time uint64, parent *types.Header) *big.Int {
	statedb, err := state.New(parent.Root, dt.stateCache)
	maxDiff := new(big.Int).SetUint64(dt.config.MaxDiff)
	if err != nil {
		return maxDiff
	}
	tickets := statedb.AllTickets()
	weight := big.NewInt(0)
	height := parent.Number
	for _, v := range tickets {
		weight = weight.Add(weight, height)
		weight = weight.Sub(weight, v.Height)
	}
	distance := new(big.Int).Sub(new(big.Int).SetUint64(time), parent.Time)
	// max / (weight * (period/distance))
	diff := new(big.Int).Mul(weight, new(big.Int).SetUint64(dt.config.Period))
	temp := new(big.Int).Mul(maxDiff, distance)
	diff = diff.Div(temp, diff)
	return diff
}

// APIs returns the RPC APIs this consensus engine provides.
func (dt *DaTong) APIs(chain consensus.ChainReader) []rpc.API {
	return []rpc.API{}
}

// Close terminates any background threads maintained by the consensus engine.
func (dt *DaTong) Close() error {
	return nil
}

func (dt *DaTong) calcStakeModifier() common.Hash {
	return common.Hash{}
}

func (dt *DaTong) calcProofPoint(parent *types.Header, time *big.Int) *big.Int {
	hasher := sha3.NewKeccak256()
	rlp.Encode(hasher, []interface{}{
		dt.calcStakeModifier(),
		parent.ParentHash,
		parent.UncleHash,
		parent.Coinbase,
		parent.Root,
		parent.TxHash,
		parent.ReceiptHash,
		parent.Bloom,
		parent.Difficulty,
		parent.Number,
		parent.GasLimit,
		parent.GasUsed,
		parent.Time,
		parent.Extra,
		time,
	})
	hash := common.Hash{}
	hasher.Sum(hash[:0])
	return new(big.Int).SetBytes(hash[:])
}

func (dt *DaTong) selectTickets(proofPoint, height, diff *big.Int, tickets []*common.Ticket) []*common.Ticket {
	results := make([]*common.Ticket, 0)
	for _, ticket := range tickets {
		if dt.verifyTicket(proofPoint, height, diff, ticket) == nil {
			results = append(results, ticket)
		}
	}
	return results
}

func (dt *DaTong) verifyTicket(proofPoint, height, diff *big.Int, ticket *common.Ticket) error {
	ticketPoint := new(big.Int).SetBytes(ticket.ID[:])
	length := new(big.Int).Div(new(big.Int).SetUint64(dt.config.MaxDiff), diff)
	weight := new(big.Int).Sub(height, ticket.Height)
	logicProofPoint := new(big.Int).Div(proofPoint, length)
	logicTicketPoint := new(big.Int).Div(ticketPoint, length)
	start := new(big.Int).Sub(logicTicketPoint, weight)
	end := new(big.Int).Add(logicTicketPoint, weight)
	if start.Cmp(logicProofPoint) <= 0 && end.Cmp(logicProofPoint) >= 0 {
		ticket.SetLength(weight)
		return nil
	}
	return errors.New("the point not in the ticket")
}

func (dt *DaTong) calcRewards(height *big.Int) *big.Int {
	return big.NewInt(1)
}

func (dt *DaTong) mine(block *types.Block, parent *types.Header, tickets []*common.Ticket, found chan *types.Block) {
	header := block.Header()
	point := dt.calcProofPoint(parent, header.Time)
	selectedTickets := dt.selectTickets(point, header.Number, header.Difficulty, tickets)
	if len(selectedTickets) > 0 {
		ticket := selectedTickets[0]
		for i := 1; i < len(selectedTickets); i++ {
			if selectedTickets[i].Length().Cmp(ticket.Length()) > 0 {
				ticket = selectedTickets[i]
			}
		}
		header.Extra = ticket.ID[:]
		found <- block.WithSeal(header)
	}
}
