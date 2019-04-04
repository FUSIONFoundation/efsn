package datong

import (
	"bytes"
	"errors"
	"fmt"
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

const (
	wiggleTime           = 500 * time.Millisecond // Random delay (per commit) to allow concurrent commits
	delayTimeModifier    = 20                     // adjust factor
	adjustIntervalBlocks = 10                     // adjust delay time by blocks

	PSN20CheckAttackEnableHeight = 80000 // check attack after this height
)

var (
	errUnknownBlock = errors.New("unknown block")

	errCoinbase = errors.New("error coinbase")

	errMissingVanity = errors.New("extra-data 32 byte vanity prefix missing")

	errMissingSignature = errors.New("extra-data 65 byte suffix signature missing")

	errInvalidUncleHash = errors.New("non empty uncle hash")

	errUnauthorized = errors.New("unauthorized")
)

// SignerFn is a signer callback function to request a hash to be signed by a
// backing account.
type SignerFn func(accounts.Account, []byte) ([]byte, error)

var (
	maxBytes                  = bytes.Repeat([]byte{0xff}, common.HashLength)
	maxDiff                   = new(big.Int).SetBytes(maxBytes)
	maxProb                   = new(big.Int)
	extraVanity               = 32
	extraSeal                 = 65
	MinBlockTime       int64  = 7   // 7 seconds
	maxBlockTime       uint64 = 120 // 2 minutes
	ticketWeightStep          = 2   // 2%
	SelectedTicketTime        = &selectedTicketTime{info: make(map[common.Hash]*selectedInfo)}
	maxTickets                = new(big.Int).SetBytes(maxBytes)
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
		return errUnknownBlock
	}
	// Checkpoint blocks need to enforce zero beneficiary
	if header.Coinbase == (common.Address{}) {
		return errCoinbase
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

	return dt.verifySeal(chain, header, parents)
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
	// verify Ancestor
	var parent *types.Header
	if len(parents) > 0 {
		parent = parents[len(parents)-1]
	} else {
		parent = chain.GetHeader(header.ParentHash, number-1)
	}
	if parent == nil || parent.Number.Uint64() != number-1 || parent.Hash() != header.ParentHash {
		return consensus.ErrUnknownAncestor
	}
	if header.Time.Int64()-parent.Time.Int64() < MinBlockTime && number >= PSN20CheckAttackEnableHeight {
		return fmt.Errorf("block %v header.Time:%v < parent.Time:%v + %v Second",
			number, header.Time.Int64(), parent.Time.Int64(), MinBlockTime)

	}
	// verify signature
	signature := header.Extra[len(header.Extra)-extraSeal:]
	pubkey, err := crypto.Ecrecover(sigHash(header).Bytes(), signature)
	if err != nil {
		return err
	}
	var signer common.Address
	copy(signer[:], crypto.Keccak256(pubkey[1:])[12:])
	if header.Coinbase != signer {
		return errors.New("Ticket owner not be the signer")
	}
	// verify ticket
	snap, err := newSnapshotWithData(getSnapDataByHeader(header))
	if err != nil {
		return err
	}
	ticketID := snap.GetVoteTicket()
	ticketMap, err := dt.getAllTickets(chain, header, parents)
	if err != nil {
		return err
	}
	if _, ok := ticketMap[ticketID]; !ok {
		return errors.New("Ticket not found")
	}
	// verify ticket with signer
	ticket := ticketMap[ticketID]
	if ticket.Owner != signer {
		return errors.New("Ticket owner not be the signer")
	}
	// verify tickets pool
	i := 0
	for _, v := range ticketMap {
		if v.Height.Cmp(header.Number) < 0 {
			i++
			break
		}
	}
	if i == 0 {
		return errors.New("verifySeal:  no tickets with correct header number, ticket not selected")
	}
	// verify ticket: list squence, ID , ticket Info, difficulty
	statedb, errs := state.New(parent.Root, dt.stateCache)
	if errs != nil {
		return errs
	}
	diff, tk, listSq, _, errv := dt.calcTicketDifficulty(chain, header, statedb)
	if errv != nil {
		return errv
	}
	// check ticket ID
	if tk.ID != ticketID {
		return errors.New("verifySeal ticketID mismatch")
	}
	// check ticket info
	errt := dt.checkTicketInfo(header, tk)
	if errt != nil {
		return errt
	}
	// check difficulty
	if diff.Cmp(header.Difficulty) != 0 {
		return errors.New("verifySeal difficulty mismatch")
	}
	// check block time
	errc := dt.checkBlockTime(chain, header, parent, listSq)
	if errc != nil {
		return errc
	}

	return nil
}

// Prepare initializes the consensus fields of a block header according to the
// rules of a particular engine. The changes are executed inline.
func (dt *DaTong) Prepare(chain consensus.ChainReader, header *types.Header) error {
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

// calc tickets total balance
func calcTotalBalance(tickets []*common.Ticket, state *state.StateDB) *big.Int {
	total := new(big.Int).SetUint64(uint64(0))
	for _, t := range tickets {
		balance := state.GetBalance(common.SystemAssetID, t.Owner)
		balance = new(big.Int).Div(balance, new(big.Int).SetUint64(uint64(1e+18)))
		total = total.Add(total, balance)
	}
	return total
}

type DisInfo struct {
	tk  *common.Ticket
	res *big.Int
}
type DistanceSlice []*DisInfo

func (s DistanceSlice) Len() int {
	return len(s)
}

func (s DistanceSlice) Less(i, j int) bool {
	return s[i].res.Cmp(s[j].res) <= 0
}

func (s DistanceSlice) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

// Finalize runs any post-transaction state modifications (e.g. block rewards)
// and assembles the final block.
// Note: The block header and state database might be updated to reflect any
// consensus rules that happen at finalization (e.g. block rewards).
func (dt *DaTong) Finalize(chain consensus.ChainReader, header *types.Header, statedb *state.StateDB, txs []*types.Transaction,
	uncles []*types.Header, receipts []*types.Receipt) (*types.Block, error) {
	parent := chain.GetHeader(header.ParentHash, header.Number.Uint64()-1)
	if parent == nil {
		return nil, consensus.ErrUnknownAncestor
	}
	parentTime := parent.Time.Uint64()
	ticketsTotal, selected, selectedTime, retreat, errv := dt.calcTicketDifficulty(chain, header, statedb)
	if errv != nil {
		return nil, errv
	}

	updateSelectedTicketTime(header, selected.ID, 0, selectedTime)
	snap := newSnapshot()

	//update tickets
	tmpState := *statedb
	headerState := &tmpState
	ticketMap, err := headerState.AllTickets()
	if err != nil {
		return nil, err
	}
	if len(ticketMap) == 1 {
		return nil, errors.New("Next block doesn't have ticket, wait buy ticket")
	}
	delete(ticketMap, selected.ID)
	headerState.RemoveTicket(selected.ID)
	if selected.Height.Cmp(common.Big0) > 0 {
		value := common.NewTimeLock(&common.TimeLockItem{
			StartTime: selected.StartTime,
			EndTime:   selected.ExpireTime,
			Value:     selected.Value,
		})
		headerState.AddTimeLockBalance(selected.Owner, common.SystemAssetID, value)
	}
	snap.AddLog(&ticketLog{
		TicketID: selected.ID,
		Type:     ticketSelect,
	})
	//delete tickets before coinbase if selected miner did not Seal
	for _, t := range retreat {
		delete(ticketMap, t.ID)
		headerState.RemoveTicket(t.ID)
		snap.AddLog(&ticketLog{
			TicketID: t.ID,
			Type:     ticketRetreat,
		})
	}

	remainingWeight := new(big.Int)
	ticketNumber := 0
	for _, t := range ticketMap {
		if t.ExpireTime <= parentTime {
			delete(ticketMap, t.ID)
			headerState.RemoveTicket(t.ID)
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
				headerState.AddTimeLockBalance(t.Owner, common.SystemAssetID, value)
			}
		} else {
			ticketNumber++
			weight := new(big.Int).Sub(header.Number, t.Height)
			remainingWeight = remainingWeight.Add(remainingWeight, weight)
		}
	}
	if remainingWeight.Cmp(common.Big0) <= 0 {
		log.Warn("Next block don't have ticket, wait buy ticket", "remainingWeight", remainingWeight)
	}

	snap.SetWeight(remainingWeight)
	snap.SetTicketWeight(remainingWeight)
	snap.SetTicketNumber(ticketNumber)
	header.Difficulty = ticketsTotal
	snapBytes := snap.Bytes()
	header.Extra = header.Extra[:extraVanity]
	header.Extra = append(header.Extra, snapBytes...)
	header.Extra = append(header.Extra, make([]byte, extraSeal)...)
	headerState.AddBalance(header.Coinbase, common.SystemAssetID, calcRewards(header.Number))
	header.Root = headerState.IntermediateRoot(chain.Config().IsEIP158(header.Number))

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

	// delay time decide block time
	delay, errc := dt.calcDelayTime(chain, header)
	if errc != nil {
		return errc
	}
	go func() {
		select {
		case <-stop:
			return
		case <-time.After(delay):
		}

		select {
		case results <- block.WithSeal(header):
			// One of the threads found a block, abort all others
			stop = make(chan struct{})
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
		parent = parents[len(parents)-1]
	} else {
		parent = chain.GetHeader(header.ParentHash, number-1)
	}
	if parent == nil {
		return nil, consensus.ErrUnknownAncestor
	}

	var statedb *state.StateDB
	var err error
	statedb, err = state.New(parent.Root, dt.stateCache)
	if err != nil {
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
		if c.data[i].Weight().Cmp(c.data[j].Weight()) == 0 {
			//sort by ticketID
			return c.data[i].ID.String() < c.data[j].ID.String()
		}
		return c.data[i].Weight().Cmp(c.data[j].Weight()) < 0
	}
	return new(big.Int).SetBytes(c.data[i].ID[:]).Cmp(new(big.Int).SetBytes(c.data[j].ID[:])) < 0

}

//func (dt *DaTong) selectTickets(tickets []*common.Ticket, parent *types.Header, time uint64,header *types.Header,ch chan []*common.Ticket) []*common.Ticket {
func (dt *DaTong) selectTickets(tickets []*common.Ticket, parent *types.Header, time uint64, header *types.Header) []*common.Ticket {
	sort.Sort(ticketSlice{
		data:         tickets,
		isSortWeight: false,
	})
	selectedTickets := make([]*common.Ticket, 0)
	sanp, err := newSnapshotWithData(getSnapDataByHeader(parent))
	if err != nil {
		return selectedTickets
	}
	weight := sanp.TicketWeight()
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
		tik := tickets[i]
		if time >= tik.StartTime && tik.ExpireTime > expireTime {
			times := new(big.Int).Sub(parent.Number, tickets[i].Height)
			times = times.Add(times, common.Big1)
			if dt.validateTicket(tickets[i], point, length, times) {
				tickets[i].SetWeight(times)
				selectedTickets = append(selectedTickets, tickets[i])
				if tickets[i].Owner == header.Coinbase {
					break
				}
			}
		}
	}
	if len(selectedTickets) == 0 {
		return selectedTickets
	}
	sort.Sort(sort.Reverse(ticketSlice{
		data:         selectedTickets,
		isSortWeight: true,
	}))
	return selectedTickets
}

func (dt *DaTong) getProbability(distance *big.Int) *big.Int {
	d := distance.Uint64()
	if d > dt.config.Period {
		d = d - dt.config.Period + 1
		max := maxProb.Uint64()
		temp := uint64(math.Pow(2, float64(d)))
		value := max - max/temp
		return new(big.Int).SetUint64(value)
	}
	return new(big.Int).SetUint64(uint64(math.Pow(2, float64(d))))
}

func (dt *DaTong) validateTicket(ticket *common.Ticket, point, length, times *big.Int) bool {
	if length.Cmp(common.Big0) <= 0 {
		return true
	}

	tickBytes := ticket.ID[:]
	pointBytes := point.Bytes()
	tempPoint := new(big.Int).Div(point, length)
	for times.Cmp(common.Big0) > 0 {
		ticketPoint := new(big.Int).SetBytes(crypto.Keccak256(tickBytes, pointBytes, times.Bytes()))
		ticketPoint = ticketPoint.Div(ticketPoint, length)
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

type selectedInfo struct {
	round uint64 // selected round
	list  uint64 // tickets number before myself in selected list
	broad bool
}

// store selected-ticket spend time
type selectedTicketTime struct {
	info map[common.Hash]*selectedInfo
	sync.Mutex
}

func calcHeaderHash(header *types.Header) []byte {
	snap, err := newSnapshotWithData(getSnapDataByHeader(header))
	if err != nil {
		return nil
	}
	ticketID := snap.GetVoteTicket()
	// hash (header.Number + ticketID + header.Coinbase)
	sum := header.Number.String() + ticketID.String() + header.Coinbase.String()
	return crypto.Keccak256([]byte(sum))
}

func updateSelectedTicketTime(header *types.Header, ticketID common.Hash, round uint64, list uint64) {
	if (header == nil || ticketID == common.Hash{}) {
		log.Warn("updateSelectedTicketTime", "input error", "")
		return
	}
	SelectedTicketTime.Lock()
	defer SelectedTicketTime.Unlock()

	// hash (header.Number + ticketID + header.Coinbase)
	sum := header.Number.String() + ticketID.String() + header.Coinbase.String()
	hash := crypto.Keccak256([]byte(sum))
	ticketInfo := SelectedTicketTime.info[common.BytesToHash(hash)]
	if ticketInfo == nil {
		sl := &selectedInfo{round: round, list: list, broad: false}
		SelectedTicketTime.info[common.BytesToHash(hash)] = sl
		ticketInfo = SelectedTicketTime.info[common.BytesToHash(hash)]
	} else {
		ticketInfo.round = round
		ticketInfo.list = list
	}
}

func haveSelectedTicketTime(header *types.Header) (uint64, uint64, error) {
	SelectedTicketTime.Lock()
	defer SelectedTicketTime.Unlock()

	hash := calcHeaderHash(header)
	if hash == nil {
		return uint64(0), uint64(0), errors.New("Hash return nil")
	}
	ticketInfo := SelectedTicketTime.info[common.BytesToHash(hash)]
	if ticketInfo == nil {
		log.Warn("Error: not found ticketInfo. SelectedTicketTime", "header.Number", header.Number)
		return uint64(0), uint64(0), errors.New("not found ticketInfo")
	}
	return ticketInfo.round, ticketInfo.list, nil
}

func (dt *DaTong) UpdateBlockBroadcast(header *types.Header) {
	SelectedTicketTime.Lock()
	defer SelectedTicketTime.Unlock()

	hash := calcHeaderHash(header)
	if hash == nil {
		return
	}
	ticketInfo := SelectedTicketTime.info[common.BytesToHash(hash)]
	if ticketInfo == nil {
		sl := &selectedInfo{round: maxTickets.Uint64(), list: maxTickets.Uint64(), broad: true}
		SelectedTicketTime.info[common.BytesToHash(hash)] = sl
		ticketInfo = SelectedTicketTime.info[common.BytesToHash(hash)]
	} else {
		ticketInfo.broad = true
	}
}

func (dt *DaTong) HaveBlockBroaded(header *types.Header) bool {
	SelectedTicketTime.Lock()
	defer SelectedTicketTime.Unlock()

	hash := calcHeaderHash(header)
	if hash == nil {
		return false
	}
	ticketInfo := SelectedTicketTime.info[common.BytesToHash(hash)]
	if ticketInfo == nil {
		return false
	}
	return ticketInfo.broad
}

func (dt *DaTong) calcTicketDifficulty(chain consensus.ChainReader, header *types.Header, statedb *state.StateDB) (*big.Int, *common.Ticket, uint64, []*common.Ticket, error) {
	parent := chain.GetHeader(header.ParentHash, header.Number.Uint64()-1)
	if parent == nil {
		return nil, nil, 0, nil, consensus.ErrUnknownAncestor
	}
	parentState, errs := state.New(parent.Root, dt.stateCache)
	if errs != nil {
		return nil, nil, 0, nil, errs
	}
	parentTicketMap, err := parentState.AllTickets()
	if err != nil {
		return nil, nil, 0, nil, err
	}
	tickets := make([]*common.Ticket, 0)
	haveTicket := false
	var weight, number uint64
	for _, v := range parentTicketMap {
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
		return nil, nil, 0, nil, errors.New("Miner doesn't have ticket")
	}

	// calc balance before selected ticket from stored tickets list
	ticketsTotalAmount := uint64(len(tickets))
	var (
		selected             *common.Ticket
		retreat              []*common.Ticket
		selectedNoSameTicket []*common.Ticket
	)
	selectedTime := uint64(0)

	// make consensus by tickets sequence(selectedTime) with: parentHash, weigth, ticketID, coinbase
	selectedTime = uint64(0)
	parentHash := parent.Hash()
	sel := make(chan *DisInfo, len(tickets))
	for i := 0; i < len(tickets); i++ {
		ticket := tickets[i]
		w := new(big.Int).Sub(parent.Number, ticket.Height)
		w = new(big.Int).Add(w, common.Big1)
		w2 := new(big.Int).Mul(w, w)

		id := new(big.Int).SetBytes(crypto.Keccak256(parentHash[:], ticket.ID[:], []byte(ticket.Owner.Hex())))
		id2 := new(big.Int).Mul(id, id)
		s := new(big.Int).Add(w2, id2)

		ht := &DisInfo{tk: ticket, res: s}
		sel <- ht
	}
	var list DistanceSlice
	tt := len(sel)
	for i := 0; i < tt; i++ {
		v := <-sel
		list = append(list, v)
	}
	sort.Sort(list)
	selectedNoSameTicket = make([]*common.Ticket, 0)
	retreat = make([]*common.Ticket, 0)
	for _, t := range list {
		if t.tk.Owner == header.Coinbase {
			selected = t.tk
			break
		} else {
			selectedTime++                                            //ticket queue in selectedList
			selectedNoSameTicket = append(selectedNoSameTicket, t.tk) // temp store tickets
		}
	}
	// selectedTime: remove repeat tickets with one miner
	norep := make(map[common.Address]bool)
	for _, nr := range selectedNoSameTicket {
		_, exist := norep[nr.Owner]
		if exist == false {
			norep[nr.Owner] = true
			retreat = append(retreat, nr) // one miner one selected ticket
		}
	}
	selectedTime = uint64(len(norep))
	if selected == nil {
		return nil, nil, 0, nil, errors.New("myself tickets not selected in maxBlockTime")
	}

	// cacl difficulty
	ticketsTotal := ticketsTotalAmount - selectedTime
	return new(big.Int).SetUint64(ticketsTotal), selected, selectedTime, retreat, nil
}

func (dt *DaTong) sortByWeightAndID(tickets []*common.Ticket, parent *types.Header, time uint64) []*common.Ticket {
	sort.Sort(ticketSlice{
		data:         tickets,
		isSortWeight: false,
	})
	selectedTickets := make([]*common.Ticket, 0)

	expireTime := parent.Time.Uint64()
	for i := 0; i < len(tickets); i++ {
		if time >= tickets[i].StartTime && tickets[i].ExpireTime > expireTime {
			times := new(big.Int).Sub(parent.Number, tickets[i].Height)
			times = times.Add(times, common.Big1)
			tickets[i].SetWeight(times)
			selectedTickets = append(selectedTickets, tickets[i])
		}
	}
	sort.Sort(ticketSlice{
		data:         selectedTickets,
		isSortWeight: true,
	})
	return selectedTickets
}

// PreProcess update state if needed from various block info
// used with some PoS Systems
func (c *DaTong) PreProcess(chain consensus.ChainReader, header *types.Header, statedb *state.StateDB) error {
	return nil
}

func (dt *DaTong) calcDelayTime(chain consensus.ChainReader, header *types.Header) (time.Duration, error) {
	list := uint64(0)
	err := errors.New("")
	_, list, err = haveSelectedTicketTime(header)
	if err != nil {
		return time.Duration(int64(0)) * time.Millisecond, err
	}

	// delayTime = ParentTime + (15 - 2) - time.Now
	parent := chain.GetHeaderByNumber(header.Number.Uint64() - 1)
	endTime := new(big.Int).Add(header.Time, new(big.Int).SetUint64(list*uint64(delayTimeModifier)+dt.config.Period-2))
	delayTime := time.Unix(endTime.Int64(), 0).Sub(time.Now())

	// delay maximum is 2 minuts

	if (new(big.Int).Sub(endTime, header.Time)).Uint64() > maxBlockTime {
		endTime = new(big.Int).Add(header.Time, new(big.Int).SetUint64(maxBlockTime+dt.config.Period-2+list))
		delayTime = time.Unix(endTime.Int64(), 0).Sub(time.Now())
	}
	if header.Number.Uint64() > (adjustIntervalBlocks + 1) {
		// adjust = ( ( parent - gparent ) / 2 - (dt.config.Period) ) / dt.config.Period
		gparent := chain.GetHeaderByNumber(header.Number.Uint64() - 1 - adjustIntervalBlocks)
		adjust := ((time.Unix(parent.Time.Int64(), 0).Sub(time.Unix(gparent.Time.Int64(), 0)) / adjustIntervalBlocks) -
			time.Duration(int64(dt.config.Period))*time.Second) /
			time.Duration(int64(adjustIntervalBlocks))

		stampSecond := time.Duration(2) * time.Second
		if adjust > stampSecond {
			adjust = stampSecond
		} else if adjust < -stampSecond {
			adjust = -stampSecond
		}
		delayTime -= adjust
	}

	return delayTime, nil
}

// check ticket info
func (dt *DaTong) checkTicketInfo(header *types.Header, ticket *common.Ticket) error {
	// check height
	if ticket.Height.Cmp(header.Number) >= 0 {
		return errors.New("checkTicketInfo ticket height mismatch")
	}
	// check start and expire time
	if ticket.ExpireTime <= ticket.StartTime ||
		ticket.ExpireTime < (ticket.StartTime+30*24*3600) ||
		ticket.ExpireTime < header.Time.Uint64() {
		return errors.New("checkTicketInfo ticket ExpireTime mismatch")
	}
	// check value
	if ticket.Value.Cmp(common.TicketPrice()) < 0 {
		return errors.New("checkTicketInfo ticket Value mismatch")
	}
	return nil
}

// check block time
func (dt *DaTong) checkBlockTime(chain consensus.ChainReader, header *types.Header, parent *types.Header, list uint64) error {
	if list <= 0 { // No.1 pass, check others
		return nil
	}
	recvTime := time.Now().Sub(time.Unix(parent.Time.Int64(), 0))
	if recvTime < (time.Duration(int64(maxBlockTime+dt.config.Period)) * time.Second) { // < 120 s
		expectTime := time.Duration(dt.config.Period)*time.Second + time.Duration(list*uint64(delayTimeModifier))*time.Second
		if recvTime < expectTime {
			return fmt.Errorf("block time mismatch: order: %v, receive: %v, expect: %v.", list, recvTime, expectTime)
		}
	}
	return nil
}
