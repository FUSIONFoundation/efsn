package datong

import (
	"bytes"
	"errors"
	"math"
	"math/big"
	"sort"
	"sync"
	"time"

	"github.com/FusionFoundation/efsn/accounts"
	"github.com/FusionFoundation/efsn/common"
	"github.com/FusionFoundation/efsn/consensus"
	"github.com/FusionFoundation/efsn/core/state"
	"github.com/FusionFoundation/efsn/core/types"
	"github.com/FusionFoundation/efsn/crypto"
	"github.com/FusionFoundation/efsn/crypto/sha3"
	"github.com/FusionFoundation/efsn/ethdb"
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
	maxBytes                = bytes.Repeat([]byte{0xff}, common.HashLength)
	maxDiff                 = new(big.Int).SetBytes(maxBytes)
	maxProb                 = new(big.Int)
	extraVanity             = 32
	extraSeal               = 65
	maxBlockTime     uint64 = 120 // 2 minutes
	ticketWeightStep        = 2   // 2%
)

var (
	emptyUncleHash = types.CalcUncleHash(nil)
)

// DaTong wacom
type DaTong struct {
	config     *params.DaTongConfig
	db         ethdb.Database
	stateCache state.Database

	signer            common.Address
	signFn            SignerFn
	lock              sync.RWMutex
	weight            *big.Int
	validTicketNumber *big.Int
}

// New wacom
func New(config *params.DaTongConfig, db ethdb.Database) *DaTong {
	maxProb.SetUint64(uint64(math.Pow(2, float64(config.Period+1))))
	return &DaTong{
		config:     config,
		db:         db,
		stateCache: state.NewDatabase(db),

		weight:            new(big.Int),
		validTicketNumber: new(big.Int),
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
func (dt *DaTong) verifyHeader(chain consensus.ChainReader, header *types.Header, seal bool, parents []*types.Header) error {
	if header.Number == nil {
		log.Info("consensus.verifyheader eerror unknown block ")
		return errUnknownBlock
	}

	if len(header.Extra) < extraVanity {
		log.Info("consensus.verifyheadererr missing vanity ")
		return errMissingVanity
	}
	if len(header.Extra) < extraVanity+extraSeal {
		log.Info("consensus.verifyheadererr missing signature ")
		return errMissingSignature
	}

	if _, err := newSnapshotWithData(getSnapDataByHeader(header)); err != nil {
		log.Info("consensus.verifyheadererr snapshot with data ", err, err.Error())
		return err
	}

	if header.UncleHash != emptyUncleHash {
		log.Info("consensus.verifyheadererr invalid uncle hash ")
		return errInvalidUncleHash
	}

	if header.Time.Cmp(big.NewInt(time.Now().Unix())) > 0 {
		log.Info("consensus.verifyheadererr error future block ")
		return consensus.ErrFutureBlock
	}

	return dt.verifySeal(chain, header, parents)
}

// PreProcess update state if needed from various block info
// used with some PoS Systems
func (c *DaTong) PreProcess(chain consensus.ChainReader, header *types.Header, statedb *state.StateDB) error {
	snapData := getSnapDataByHeader(header)
	snap, err := newSnapshotWithData(snapData)
	if err != nil {
		return err
	}
	deleteMap := make([]common.Hash, 0)
	for _, tik := range snap.logs {
		deleteMap = append(deleteMap, tik.TicketID)
	}
	statedb.RemoveTickets(deleteMap)
	return nil
}

// VerifyHeader checks whether a header conforms to the consensus rules of the
// stock Ethereum ethash engine.
func (dt *DaTong) VerifyHeader(chain consensus.ChainReader, header *types.Header, seal bool) error {
	return dt.verifyHeader(chain, header, seal, glb_parents)
}

// VerifyHeaders is similar to VerifyHeader, but verifies a batch of headers
// concurrently. The method returns a quit channel to abort the operations and
// a results channel to retrieve the async verifications.
func (dt *DaTong) VerifyHeaders(chain consensus.ChainReader, headers []*types.Header, seals []bool) (chan<- struct{}, <-chan error) {
	abort := make(chan struct{})
	results := make(chan error, len(headers))
	go func() {
		for i, header := range headers {
			err := dt.verifyHeader(chain, header, seals[i], headers[:i])
			select {
			case <-abort:
				log.Info("Verify Headers", "ABORT BLOCK", header.Number)
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

// VerifySeal implements consensus.Engine, checking whether the signature contained
// in the header satisfies the consensus protocol requirements.
func (c *DaTong) VerifySeal(chain consensus.ChainReader, header *types.Header) error {
	return c.verifySeal(chain, header, nil)
}

var glb_parents []*types.Header

func SetHeaders(parents []*types.Header) {
	glb_parents = parents
}

// VerifySeal checks whether the crypto seal on a header is valid according to
// the consensus rules of the given engine.
func (dt *DaTong) verifySeal(chain consensus.ChainReader, header *types.Header, parents []*types.Header) error {
	number := header.Number.Uint64()
	if number == 0 {
		return errUnknownBlock
	}

	var parent *types.Header
	if len(parents) > 0 {
		parent = parents[len(parents)-1]
	} else {
		parent = chain.GetHeader(header.ParentHash, number-1)
	}
	if parent == nil {
		log.Info("consensus.verifySeal ErrUnknownAncestor", "number", number-1)
		return consensus.ErrUnknownAncestor
	}

	snap, err := newSnapshotWithData(getSnapDataByHeader(header))
	if err != nil {
		log.Info("consensus.verifySeal Err newSnapshot ", "err", err.Error())
		return err
	}

	signature := header.Extra[len(header.Extra)-extraSeal:]
	pubkey, err := crypto.Ecrecover(sigHash(header).Bytes(), signature)
	if err != nil {
		log.Info("consensus.verifySeal Ecrecover ", "err", err.Error())
		return err
	}

	var signer common.Address
	copy(signer[:], crypto.Keccak256(pubkey[1:])[12:])

	parentTime := parent.Time.Uint64()
	time := header.Time.Uint64()
	if time-parentTime > maxBlockTime {
		if header.Coinbase != signer {
			log.Info("consensus.verifySeal Ticket owner not be the signer")
			return errors.New("Ticket owner not be the signer")
		}
		return nil
	}
	ticketID := snap.GetVoteTicket()
	ticketMap, err := dt.getAllTickets(chain, header, parents)

	if err != nil {
		log.Info("consensus.verifySeal getAllTickets", "err", err.Error())
		return err
	}

	if _, ok := ticketMap[ticketID]; !ok {
		log.Info("consensus.verifySeal ticketNotFound", "ticketID", ticketID)
		return errors.New("Ticket not found")
	}
	ticket := ticketMap[ticketID]

	if ticket.Owner != signer {
		log.Info("consensus.verifySeal ticket owner not signer", "ticOwner", ticket.Owner, "signer", signer)
		return errors.New("Ticket owner not be the signer")
	}

	tickets := make([]*common.Ticket, 0)
	selected := false
	i := 0
	for _, v := range ticketMap {
		if v.Height.Cmp(header.Number) < 0 {
			temp := v
			tickets = append(tickets, &temp)
			i++
		}
	}

	//log.Info("Number of tickets to try and select ", "#", i, "ticketsToSelectFrom_ticketMap", len(ticketMap))

	if i == 0 {
		log.Info("consensus.verifySeal no tickets with correct header number, ticket not selected")
		return errors.New("verifySeal:  no tickets with correct header number, ticket not selected")
	}

	selectedTickets := dt.selectTickets(tickets, parent, header.Time.Uint64())
	for _, v := range selectedTickets {
		if v.ID == ticketID {
			selected = true
			break
		}
	}

	if !selected {
		log.Info("consensus.verifySeal ticket not selected")
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
	header.Extra = append(header.Extra, make([]byte, extraSeal)...)
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

	ticketMap, err := state.AllTickets()
	if err != nil {
		log.Error("unable to retrieve tickets in Finalize:Consensus.go")
		return nil, err
	}

	if len(ticketMap) == 1 {
		log.Error("Next block doesn't have ticket, wait buy ticket")
		return nil, errors.New("Next block doesn't have ticket, wait buy ticket")
	}

	tickets := make([]*common.Ticket, 0)
	haveTicket := false

	var weight, number uint64

	for _, v := range ticketMap {
		if v.Height.Cmp(header.Number) < 0 {
			if v.Owner == header.Coinbase {
				number++
				weight += header.Number.Uint64() - v.Height.Uint64() + 1
				haveTicket = true
			}
			temp := v
			tickets = append(tickets, &temp)
		}
	}

	dt.weight.SetUint64(weight)
	dt.validTicketNumber.SetUint64(number)

	if !haveTicket {
		log.Error("Miner doesn't have ticket")
		return nil, errors.New("Miner doesn't have ticket")
	}
	parentTime := parent.Time.Uint64()
	time := header.Time.Uint64()
	var (
		selected *common.Ticket
		retreat  []*common.Ticket
	)
	deleteAll := false
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
		if (time - parentTime) > maxBlockTime {
			deleteAll = true
			break
		}
	}
	header.Time = new(big.Int).SetUint64(time)
	snap := newSnapshot()

	deleteMap := make([]common.Hash, 0)

	if deleteAll {
		snap.AddLog(&ticketLog{
			TicketID: common.BytesToHash(header.Coinbase[:]),
			Type:     ticketSelect,
		})

		for _, t := range ticketMap {
			if t.Height.Cmp(header.Number) < 0 {
				delete(ticketMap, t.ID)
				deleteMap = append(deleteMap, t.ID)
				snap.AddLog(&ticketLog{
					TicketID: t.ID,
					Type:     ticketDelete,
				})
				if t.Height.Cmp(common.Big0) > 0 {
					value := common.NewTimeLock(&common.TimeLockItem{
						StartTime: t.StartTime,
						EndTime:   t.ExpireTime,
						Value:     t.Value,
					})
					state.AddTimeLockBalance(t.Owner, common.SystemAssetID, value)
				}
			}
		}
	} else {
		delete(ticketMap, selected.ID)
		deleteMap = append(deleteMap, selected.ID)
		if selected.Height.Cmp(common.Big0) > 0 {
			value := common.NewTimeLock(&common.TimeLockItem{
				StartTime: selected.StartTime,
				EndTime:   selected.ExpireTime,
				Value:     selected.Value,
			})
			state.AddTimeLockBalance(selected.Owner, common.SystemAssetID, value)
		}
		snap.AddLog(&ticketLog{
			TicketID: selected.ID,
			Type:     ticketSelect,
		})

		for _, t := range retreat {
			delete(ticketMap, t.ID)
			deleteMap = append(deleteMap, t.ID)
			if t.Height.Cmp(common.Big0) > 0 {
				value := common.NewTimeLock(&common.TimeLockItem{
					StartTime: t.StartTime,
					EndTime:   t.ExpireTime,
					Value:     t.Value,
				})
				state.AddTimeLockBalance(t.Owner, common.SystemAssetID, value)
			}
			snap.AddLog(&ticketLog{
				TicketID: t.ID,
				Type:     ticketRetreat,
			})
		}
	}

	remainingWeight := new(big.Int)
	// totalBalance := new(big.Int)
	// balanceTemp := make(map[common.Address]bool)

	ticketNumber := 0

	for _, t := range ticketMap {
		if t.ExpireTime <= time {
			delete(ticketMap, t.ID)
			deleteMap = append(deleteMap, t.ID)
			snap.AddLog(&ticketLog{
				TicketID: t.ID,
				Type:     ticketExpired,
			})
			if t.Height.Cmp(common.Big0) > 0 {
				value := common.NewTimeLock(&common.TimeLockItem{
					StartTime: t.StartTime,
					EndTime:   t.ExpireTime,
					Value:     t.Value,
				})
				state.AddTimeLockBalance(t.Owner, common.SystemAssetID, value)
			}
		} else {
			ticketNumber++
			weight := new(big.Int).Sub(header.Number, t.Height)
			remainingWeight.Add(remainingWeight, weight.Add(weight, common.Big1))
			// weight = weight.Mul(weight, big.NewInt(int64(ticketWeightStep))) // one ticket every block add weight eq number * setp
			// weight = weight.Add(weight, common.Big100)                       // one ticket weight eq 100
			// remainingWeight = remainingWeight.Add(remainingWeight, weight)

			// if _, exist := balanceTemp[t.Owner]; !exist {
			// 	balanceTemp[t.Owner] = true
			// 	balance := state.GetBalance(common.SystemAssetID, t.Owner)
			// 	totalBalance = totalBalance.Add(totalBalance, balance)
			// }

		}
	}

	// this is done in preprocess of next block state.RemoveTickets( deleteMap )

	if remainingWeight.Cmp(common.Big0) <= 0 {
		return nil, errors.New("Next block don't have ticket, wait buy ticket")
	}

	snap.SetWeight(remainingWeight)
	// snap.SetWeight(new(big.Int).Add(totalBalance, remainingWeight))
	snap.SetTicketWeight(remainingWeight) // leaving this here it just fills out ticket weight does nothing
	snap.SetTicketNumber(ticketNumber)

	snapBytes := snap.Bytes()
	header.Extra = header.Extra[:extraVanity]
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

// ConsensusData wacom
func (dt *DaTong) ConsensusData() []*big.Int {
	return []*big.Int{dt.weight, dt.validTicketNumber}
}

// APIs returns the RPC APIs this consensus engine provides.
func (dt *DaTong) APIs(chain consensus.ChainReader) []rpc.API {
	return []rpc.API{{
		Namespace: "fsn",
		Version:   "1.0",
		Service:   &API{chain: chain},
		Public:    false,
	}}
}

// Close terminates any background threads maintained by the consensus engine.
func (dt *DaTong) Close() error {
	return nil
}

func (dt *DaTong) getAllTickets(chain consensus.ChainReader, header *types.Header, parents []*types.Header) (map[common.Hash]common.Ticket, error) {
	number := header.Number.Uint64()
	if number == 0 {
		return nil, errUnknownBlock
	}
	var parent *types.Header
	if len(parents) > 0 {
		// log.Info("  getAllTickets use past headers ")
		parent = parents[len(parents)-1]
	} else {
		// log.Info("  getAllTickets get header with parent hash ")
		parent = chain.GetHeader(header.ParentHash, number-1)
	}

	if parent == nil {
		return nil, consensus.ErrUnknownAncestor
	}

	var statedb *state.StateDB
	var err error

	statedb, err = state.New(parent.Root, dt.stateCache)

	if err != nil {
		log.Info("  consensus.go getAllTickets state not found for parent root ", "err", err.Error())
		return nil, err
	}

	allTickets, err := statedb.AllTickets()
	return allTickets, err
}

type ticketSlice struct {
	data         []*common.Ticket
	isSortWeight bool
}

func (c ticketSlice) Len() int {
	return len(c.data)
}
func (c ticketSlice) Swap(i, j int) {
	c.data[i], c.data[j] = c.data[j], c.data[i]
}

func (c ticketSlice) Less(i, j int) bool {
	if c.isSortWeight {
		return c.data[i].Weight().Cmp(c.data[j].Weight()) < 0
	}
	return new(big.Int).SetBytes(c.data[i].ID[:]).Cmp(new(big.Int).SetBytes(c.data[j].ID[:])) < 0

}

func (dt *DaTong) selectTickets(tickets []*common.Ticket, parent *types.Header, time uint64) []*common.Ticket {
	sort.Sort(ticketSlice{
		data:         tickets,
		isSortWeight: false,
	})
	selectedTickets := make([]*common.Ticket, 0)
	sanp, err := newSnapshotWithData(getSnapDataByHeader(parent))
	if err != nil {
		return selectedTickets
	}
	weight := sanp.Weight()
	//weight := sanp.TicketWeight()
	distance := new(big.Int).Sub(new(big.Int).SetUint64(time), parent.Time)
	prob := dt.getProbability(distance)
	length := new(big.Int).Div(maxDiff, weight)
	length = length.Mul(length, prob)
	length = length.Div(length, maxProb)
	length = length.Div(length, common.Big2)
	parentHash := parent.Hash()
	point := new(big.Int).SetBytes(crypto.Keccak256(parentHash[:], prob.Bytes()))
	expireTime := parent.Time.Uint64()
	for i := 0; i < len(tickets); i++ {
		if time >= tickets[i].StartTime && tickets[i].ExpireTime > expireTime {

			times := new(big.Int).Sub(parent.Number, tickets[i].Height)
			times = times.Add(times, common.Big1)
			// times = times.Mul(times, big.NewInt(int64(ticketWeightStep)))
			// times = times.Add(times, common.Big100)

			if dt.validateTicket(tickets[i], point, length, times) {
				tickets[i].SetWeight(times)
				selectedTickets = append(selectedTickets, tickets[i])
			}
		}
	}
	sort.Sort(ticketSlice{
		data:         selectedTickets,
		isSortWeight: true,
	})
	return selectedTickets
}

func (dt *DaTong) getProbability(distance *big.Int) *big.Int {
	d := distance.Uint64()
	return new(big.Int).SetUint64(uint64(math.Pow(2, float64(d))))
}

func (dt *DaTong) validateTicket(ticket *common.Ticket, point, length, times *big.Int) bool {

	if length.Cmp(common.Big0) <= 0 {
		return true
	}

	for times.Cmp(common.Big0) > 0 {
		ticketPoint := new(big.Int).SetBytes(crypto.Keccak256(ticket.ID[:], point.Bytes(), times.Bytes()))
		ticketPoint = ticketPoint.Div(ticketPoint, length)
		tempPoint := new(big.Int).Div(point, length)
		if ticketPoint.Cmp(tempPoint) == 0 {
			return true
		}
		times = times.Sub(times, common.Big1)
	}
	return false
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
	var reward = new(big.Int).Mul(big.NewInt(25), big.NewInt(100000000000000000))
	// every 4915200 blocks divide reward by 2
	segment := new(big.Int).Div(height, new(big.Int).SetUint64(4915200))
	for i = 0; i < segment.Int64(); i++ {
		reward = new(big.Int).Div(reward, div2)
	}
	return reward
}

// GenGenesisExtraData wacom
func GenGenesisExtraData(number *big.Int) []byte {
	data := make([]byte, extraVanity)
	snap := newSnapshot()
	snap.SetWeight(number)
	snap.SetTicketWeight(number)
	data = append(data, snap.Bytes()...)
	data = append(data, bytes.Repeat([]byte{0x00}, extraSeal)...)
	return data
}
