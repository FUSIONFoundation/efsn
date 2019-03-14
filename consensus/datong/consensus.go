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
	"github.com/davecgh/go-spew/spew"
)

const (
	wiggleTime = 500 * time.Millisecond // Random delay (per commit) to allow concurrent commits
	delayTimeModifier	= 30 // adjust factor
	adjustIntervalBlocks	= 10 // adjust delay time by blocks
)

var (
	errUnknownBlock = errors.New("unknown block")

	errCoinbase = errors.New("error coinbase")

	errMissingVanity = errors.New("extra-data 32 byte vanity prefix missing")

	errMissingSignature = errors.New("extra-data 65 byte suffix signature missing")

	errInvalidUncleHash = errors.New("non empty uncle hash")

	errUnauthorized = errors.New("unauthorized")

	IsPsnTestnet = true
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
	maxBlockTime       uint64 = 120 // 2 minutes
	ticketWeightStep          = 2   // 2%
	SelectedTicketTime        = &selectedTicketTime{info: make(map[common.Hash]*selectedInfo)}
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
		//return errInvalidUncleHash
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
	if len(block.Uncles()) > 0 {
		//return errors.New("uncles not allowed")
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
		return consensus.ErrUnknownAncestor
	}

	signature := header.Extra[len(header.Extra)-extraSeal:]
	pubkey, err := crypto.Ecrecover(sigHash(header).Bytes(), signature)
	if err != nil {
		return err
	}

	var signer common.Address
	copy(signer[:], crypto.Keccak256(pubkey[1:])[12:])

	parentTime := parent.Time.Uint64()
	time := header.Time.Uint64()
	if time-parentTime > maxBlockTime {
		if header.Coinbase != signer {
			return errors.New("Ticket owner not be the signer")
		}
		return nil
	}

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
	ticket := ticketMap[ticketID]

	// verify ticket with signer
	if ticket.Owner != signer {
		return errors.New("Ticket owner not be the signer")
	}

	tickets := make([]*common.Ticket, 0)
	i := 0
	for _, v := range ticketMap {
		if v.Height.Cmp(header.Number) < 0 {
			temp := v
			tickets = append(tickets, &temp)
			i++
		}
	}

	if i == 0 {
		return errors.New("verifySeal:  no tickets with correct header number, ticket not selected")
	}

	// verify
	statedb, errs := state.New(parent.Root, dt.stateCache)
	if errs != nil {
		return errs
	}
	diff, tid, errv := dt.calcTicketDifficulty(chain, header, statedb)
	if errv != nil {
		return errv
	}
	// verify ticket id
	if tid != ticketID {
		return errors.New("verifySeal ticketID mismatch")
	}
	// verify diffculty
	if diff.Cmp(header.Difficulty) != 0 {
		return errors.New("verifySeal difficulty mismatch")
	}

	return nil
}

// Prepare initializes the consensus fields of a block header according to the
// rules of a particular engine. The changes are executed inline.
func (dt *DaTong) Prepare(chain consensus.ChainReader, header *types.Header) error {
	if header.Coinbase == (common.Address{}) {
		log.Info("Prepare: Coinbase of header is empty")
		return errCoinbase
	}
	number := header.Number.Uint64()
	parent := chain.GetHeader(header.ParentHash, number-1)
	if parent == nil {
		log.Info("Prepare: unknown parent  (ErrUnknownAncestor)")
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
		//log.Info("Finalize", "total", total, "balance", balance, "t.Owner", t.Owner, "t.ID", t.ID)
	}
	return total
}

type htimeInfo struct {
	htime uint64
	res []*common.Ticket
}

type sortablehtimeSlice []*htimeInfo

func (s sortablehtimeSlice) Len() int {
	return len(s)
}

func (s sortablehtimeSlice) Less(i, j int) bool {
	return s[i].htime <= s[j].htime
}

func (s sortablehtimeSlice) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

//////////////
type DisInfo struct {
	owner *common.Ticket
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

func PrintTime(s string,t time.Time) {
	ss := s + " uses time"
	log.Info("=============",ss,common.PrettyDuration(time.Since(t)),"","===============")
}
//////////////
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
	parentState, errs := state.New(parent.Root, dt.stateCache)
	if errs != nil {
		return nil, errs
	}

	parentTicketMap, err := parentState.AllTickets()
	if err != nil {
		return nil, err
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
		return nil, errors.New("Miner doesn't have ticket")
	}
	// calc balance before selected ticket from stored tickets list
	ticketsTotalAmount := uint64(len(tickets))
	parentTime := parent.Time.Uint64()
	htime := parentTime
	var (
		selected             *common.Ticket
		retreat              []*common.Ticket
		selectedList         []*common.Ticket
		selectedNoSameTicket []*common.Ticket
	)
	deleteAll := false
	selectedTime := uint64(0)
	selectedList = make([]*common.Ticket, 0)
	for {
	        if IsPsnTestnet == true {
		    break
		}

		htime++
		retreat = make([]*common.Ticket, 0)
		selectedNoSameTicket = make([]*common.Ticket, 0) //TODO
		s := dt.selectTickets(tickets, parent, htime,header)
		//spew.Printf("Finalize, parent.Number: %+v, htime: %+v, selected ticket: %#v\n", *parent.Number, htime, s)
		for _, t := range s {
			if t.Owner == header.Coinbase {
				selected = t
				break
			} else {
				selectedTime++
				retreat = append(retreat, t)
				noSameTicket := true
				for _, nt := range selectedList {
					if t.ID == nt.ID {
						noSameTicket = false
						break
					}
				}
				if noSameTicket == true {
					selectedNoSameTicket = append(selectedNoSameTicket, t)
				}
			}
		}
		for _, mt := range selectedNoSameTicket {
			// store tickets the different
			selectedList = append(selectedList, mt)
		}
		if selected != nil {
			break
		}
		if (htime - parentTime) > maxBlockTime {
			deleteAll = true
			break
		}
	}
	// If selected not mine, sort all tickets by weight and ID
	if selected == nil && IsPsnTestnet != true {
		log.Info("Finalize time,", "all tickets not selected in maxBlockTime, header.Number", header.Number)

		sortTickets := dt.sortByWeightAndID(tickets, parent, parent.Time.Uint64())
		for _, t := range sortTickets {
			if t.Owner == header.Coinbase {
				selected = t
				deleteAll = false
				break
			} else {
				//ticket queue in selectedList
				selectedTime++
				retreat = append(retreat, t)
			}

		}
	}

	if IsPsnTestnet == true {
	    tmp := time.Now()
	    selectedTime = uint64(0)
	    parentHash := parent.Hash()
	    sel := make(chan *DisInfo, len(tickets))
	    for i := 0; i < len(tickets); i++ {
		ticket := tickets[i]
		w := new(big.Int).Sub(parent.Number, ticket.Height)
		w = new(big.Int).Add(w,common.Big1)
		w2 := new(big.Int).Mul(w,w)

		id := new(big.Int).SetBytes(crypto.Keccak256(parentHash[:], ticket.ID[:],[]byte(ticket.Owner.Hex())))
		id2 := new(big.Int).Mul(id,id)
		s := new(big.Int).Add(w2,id2)

		ht := &DisInfo{owner:ticket,res:s}
		sel <- ht
	    }
	    var list DistanceSlice
	    tt := len(sel)
	    for i:=0;i<tt;i++ {
		v := <- sel
		list = append(list, v)
	}
	    sort.Sort(list)
	    for _, t := range list {
		    if t.owner.Owner == header.Coinbase {
			    htime = parentTime
			    selected = t.owner
			    spew.Printf("selected ticket: %#v, coinbase: 0x%x\n", t, header.Coinbase)
			    break
		    } else {
			    selectedTime++//ticket queue in selectedList
			    retreat = append(retreat, t.owner)
		    }

	    }
	    PrintTime("total calc time",tmp)
	}
	if selected == nil {
		return nil, errors.New("myself tickets not selected in maxBlockTime")
	}
	log.Info("Finalize time", "htime", htime, "selectedRound", htime - parentTime)

	updateSelectedTicketTime(header, selected.ID, htime - parentTime, selectedTime)
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
	if deleteAll {
		snap.AddLog(&ticketLog{
			TicketID: common.BytesToHash(header.Coinbase[:]),
			Type:     ticketSelect,
		})

		for _, t := range ticketMap {
			if t.Height.Cmp(header.Number) < 0 {
				delete(ticketMap, t.ID)
				headerState.RemoveTicket(t.ID)
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
					headerState.AddTimeLockBalance(t.Owner, common.SystemAssetID, value)
				}
			}
		}
	} else {
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

		for _, t := range retreat {
			delete(ticketMap, t.ID)
			headerState.RemoveTicket(t.ID)
			if t.Height.Cmp(common.Big0) > 0 {
				value := common.NewTimeLock(&common.TimeLockItem{
					StartTime: t.StartTime,
					EndTime:   t.ExpireTime,
					Value:     t.Value,
				})
				headerState.AddTimeLockBalance(t.Owner, common.SystemAssetID, value)
			}
			snap.AddLog(&ticketLog{
				TicketID: t.ID,
				Type:     ticketRetreat,
			})
		}
	}

	remainingWeight := new(big.Int)
	//totalBalance := new(big.Int)
	//balanceTemp := make(map[common.Address]bool)
	ticketNumber := 0

	for _, t := range ticketMap {
		if t.ExpireTime <= htime {
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
			//weight = weight.Mul(weight, big.NewInt(int64(ticketWeightStep))) // one ticket every block add weight eq number * setp
			//weight = weight.Add(weight, common.Big100)                       // one ticket weight eq 100
			remainingWeight = remainingWeight.Add(remainingWeight, weight)

			//log.Info("Finalize", "t.Owner", t.Owner)
			//if _, exist := balanceTemp[t.Owner]; !exist {
			//	balanceTemp[t.Owner] = true
			//	balance := headerState.GetBalance(common.SystemAssetID, t.Owner)
			//	balance = new(big.Int).Div(balance, new(big.Int).SetUint64(uint64(1e+18)))
			//	totalBalance = totalBalance.Add(totalBalance, balance)
			//	//log.Info("Finalize", "totalBalance", totalBalance, "balance", balance, "t.Owner", t.Owner)
			//}
		}
	}

	if remainingWeight.Cmp(common.Big0) <= 0 {
		return nil, errors.New("Next block don't have ticket, wait buy ticket")
	}

	//log.Info("Finalize snap.Set", "totalBalance", totalBalance, "remainingWeight", remainingWeight, "ticketNumber", ticketNumber)
	//snap.SetWeight(new(big.Int).Add(totalBalance, remainingWeight))
	log.Info("Finalize snap.Set", "remainingWeight", remainingWeight, "ticketNumber", ticketNumber)
	snap.SetWeight(remainingWeight)
	snap.SetTicketWeight(remainingWeight)
	snap.SetTicketNumber(ticketNumber)

	// cacl difficulty
	ticketsTotal := ticketsTotalAmount - selectedTime
	header.Difficulty = new(big.Int).SetUint64(ticketsTotal)

	snapBytes := snap.Bytes()
	header.Extra = header.Extra[:extraVanity]
	header.Extra = append(header.Extra, snapBytes...)
	header.Extra = append(header.Extra, make([]byte, extraSeal)...)
	headerState.AddBalance(header.Coinbase, common.SystemAssetID, calcRewards(header.Number))
	header.Root = headerState.IntermediateRoot(chain.Config().IsEIP158(header.Number))

	return types.NewBlock(header, txs, uncles, receipts), nil
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

	delay, errc := dt.calcDelayTime(chain, header)
	log.Info("Seal()", "delay", delay)
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
func (dt *DaTong) selectTickets(tickets []*common.Ticket, parent *types.Header, time uint64,header *types.Header) []*common.Ticket {
	
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
			//times = times.Mul(times, big.NewInt(int64(ticketWeightStep)))
			//times = times.Add(times, common.Big100)

			if dt.validateTicket(tickets[i], point, length, times) {
				tickets[i].SetWeight(times)
				selectedTickets = append(selectedTickets, tickets[i])

				/////
				if tickets[i].Owner == header.Coinbase {
					break
				}
				/////
			}
		}
	}
	if len(selectedTickets) == 0 {
		log.Debug("=============selectTickets,select tickets len = 0=========================")
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
		ticketPoint := new(big.Int).SetBytes(crypto.Keccak256( tickBytes, pointBytes, times.Bytes()))
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
	round uint64// selected round
	list  uint64// tickets number before myself in selected list
	broad bool
}

// store selected-ticket spend time
type selectedTicketTime struct {
	info map[common.Hash]*selectedInfo
	sync.Mutex
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
	sl := &selectedInfo{round: round, list: list}
	SelectedTicketTime.info[common.BytesToHash(hash)] = sl
	log.Info("updateSelectedTicketTime", "header.Number", header.Number, "ticketID", ticketID, "round", round, "list", list)
}

func haveSelectedTicketTime(header *types.Header) (uint64, uint64, error) {
	SelectedTicketTime.Lock()
	defer SelectedTicketTime.Unlock()

	hash := calcHeaderHash(header)
	if hash == nil {
		return uint64(0), uint64(0), errors.New("Hash return nil")
	}
	ticketInfo := SelectedTicketTime.info[common.BytesToHash(hash)]
	if ticketInfo != nil {
		log.Info("haveSelectedTicketTime", "header.Number", header.Number, "header.Root", header.Root, "time", ticketInfo)
		return ticketInfo.round, ticketInfo.list, nil
	}
	log.Info("Error: not found ticketTime. haveSelectedTicketTime", "header.Number", header.Number, "header.Root", header.Root)
	return uint64(0), uint64(0), errors.New("Mismatched SealHash")
}

func calcHeaderHash(header *types.Header) []byte {
	snap, err := newSnapshotWithData(getSnapDataByHeader(header))
	if err != nil {
		return nil
	}
	ticketID := snap.GetVoteTicket()
	sum := header.Number.String() + ticketID.String() + header.Coinbase.String()
	return crypto.Keccak256([]byte(sum))
}

func (dt *DaTong) UpdateBlockBroadcast(header *types.Header) {
	SelectedTicketTime.Lock()
	defer SelectedTicketTime.Unlock()

	hash := calcHeaderHash(header)
	if hash == nil {
		return
	}
	ticketInfo := SelectedTicketTime.info[common.BytesToHash(hash)]
	if ticketInfo != nil {
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
	if ticketInfo != nil {
		return ticketInfo.broad
	}
	return false
}

func (dt *DaTong) calcTicketDifficulty(chain consensus.ChainReader, header *types.Header, statedb *state.StateDB) (*big.Int, common.Hash, error) {
	parent := chain.GetHeader(header.ParentHash, header.Number.Uint64()-1)
	if parent == nil {
		return nil, common.Hash{}, consensus.ErrUnknownAncestor
	}
	parentState, errs := state.New(parent.Root, dt.stateCache)
	if errs != nil {
		return nil, common.Hash{}, errs
	}

	parentTicketMap, err := parentState.AllTickets()
	if err != nil {
		return nil, common.Hash{}, err
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
		return nil, common.Hash{}, errors.New("Miner doesn't have ticket")
	}
	// calc balance before selected ticket from stored tickets list
	ticketsTotalAmount := uint64(len(tickets))
	parentTime := parent.Time.Uint64()
	htime := parentTime
	var (
		selected             *common.Ticket
		retreat              []*common.Ticket
		selectedList         []*common.Ticket
		selectedNoSameTicket []*common.Ticket
	)
	selectedTime := uint64(0)
	selectedList = make([]*common.Ticket, 0)
	for {
	        if IsPsnTestnet == true {
		    break
		}
		htime++
		retreat = make([]*common.Ticket, 0)
		selectedNoSameTicket = make([]*common.Ticket, 0)
		s := dt.selectTickets(tickets, parent, htime,header)
		for _, t := range s {
			if t.Owner == header.Coinbase {
				selected = t
				break
			} else {
				selectedTime++
				retreat = append(retreat, t)
				noSameTicket := true
				for _, nt := range selectedList {
					if t.ID == nt.ID {
						noSameTicket = false
						break
					}
				}
				if noSameTicket == true {
					selectedNoSameTicket = append(selectedNoSameTicket, t)
				}
			}
		}
		for _, mt := range selectedNoSameTicket {
			// store tickets the different
			selectedList = append(selectedList, mt)
		}
		if selected != nil {
			break
		}
		if (htime - parentTime) > maxBlockTime {
			break
		}
	}
	// If this, Datong consensus error
	if selected == nil && selectedTime == uint64(0) && IsPsnTestnet != true {

		sortTickets := dt.sortByWeightAndID(tickets, parent, parent.Time.Uint64())
		for _, t := range sortTickets {
			if t.Owner == header.Coinbase {
				selected = t
				break
			} else {
				selectedTime++ //ticket queue in selectedList
				retreat = append(retreat, t)
			}

		}
	}

	if IsPsnTestnet == true {
	    tmp := time.Now()
	    selectedTime = uint64(0)
	    parentHash := parent.Hash()
	    sel := make(chan *DisInfo, len(tickets))
	    for i := 0; i < len(tickets); i++ {
		ticket := tickets[i]
		w := new(big.Int).Sub(parent.Number, ticket.Height)
		w = new(big.Int).Add(w,common.Big1)
		w2 := new(big.Int).Mul(w,w)

		id := new(big.Int).SetBytes(crypto.Keccak256(parentHash[:], ticket.ID[:],[]byte(ticket.Owner.Hex())))
		id2 := new(big.Int).Mul(id,id)
		s := new(big.Int).Add(w2,id2)

		ht := &DisInfo{owner:ticket,res:s}
		sel <- ht
	    }
	    var list DistanceSlice
	    tt := len(sel)
	    for i:=0;i<tt;i++ {
		v := <- sel
		list = append(list, v)
	}
	    sort.Sort(list)
	    for _, t := range list {
		    if t.owner.Owner == header.Coinbase {
			    htime = parentTime
			    selected = t.owner
			    spew.Printf("selected ticket: %#v, coinbase: 0x%x\n", t, header.Coinbase)
			    break
		    } else {
			    selectedTime++//ticket queue in selectedList
			    retreat = append(retreat, t.owner)
		    }

	    }

	    //bug
	    norep := make(map[common.Address]bool)
	    for _,nr := range retreat {
		_,ok := norep[nr.Owner]
		if ok == false {
		    norep[nr.Owner]=true
		}
	    }
	    log.Info("===========datong.Finalize","include repeat selecteTime",selectedTime,"","=================")
	    selectedTime = uint64(len(norep))
	    log.Info("===========datong.Finalize","no repeat selecteTime",selectedTime,"","=================")
	    //
	    PrintTime("total calc time",tmp)
	}
	if selected == nil {
		return nil, common.Hash{}, errors.New("myself tickets not selected in maxBlockTime")
	}

	// cacl difficulty
	ticketsTotal := ticketsTotalAmount - selectedTime

	return new(big.Int).SetUint64(ticketsTotal), selected.ID, nil
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
			//times = times.Add(times, common.Big1)
			times = times.Mul(times, big.NewInt(int64(ticketWeightStep)))
			times = times.Add(times, common.Big100)

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
	// debug code to remove tickets on next block if needed
	//
	// snapData := getSnapDataByHeader(header)
	// snap, err := newSnapshotWithData(snapData)
	// if err != nil {
	// 	return err
	// }
	// deleteMap := make([]common.Hash, 0)
	// for _, tik := range snap.logs {
	// 	deleteMap = append(deleteMap, tik.TicketID)
	// }
	// statedb.RemoveTickets(deleteMap)
	return nil
}

func (dt *DaTong) calcDelayTime(chain consensus.ChainReader, header *types.Header) (time.Duration, error) {
	list := uint64(0)
	err := errors.New("")
	_, list, err = haveSelectedTicketTime(header)
	if err != nil {
		return time.Duration(int64(0)) * time.Millisecond, err
	}

	// delayTime = headerTime + (15 - 2) - time.Now
	newBlockTime := new(big.Int).Add(header.Time, new(big.Int).SetUint64(dt.config.Period - 2))
	delayTime := time.Unix(newBlockTime.Int64(), 0).Sub(time.Now())
	delayTime += time.Duration(list * uint64(delayTimeModifier)) * time.Second
	if header.Number.Uint64() < (adjustIntervalBlocks + 2) {
		return delayTime, nil
	}

	// adjust = ( ( parent - gparent ) / 2 - (dt.config.Period) ) / dt.config.Period
	parent := chain.GetHeaderByNumber(header.Number.Uint64() - 1)
	gparent := chain.GetHeaderByNumber(header.Number.Uint64() - 1 - adjustIntervalBlocks)
	adjust := ((time.Unix(parent.Time.Int64(), 0).Sub(time.Unix(gparent.Time.Int64(), 0)) / adjustIntervalBlocks) -
			time.Duration(int64(dt.config.Period)) * time.Second) /
			time.Duration(int64(adjustIntervalBlocks))
	delayTime -= adjust
	if delayTime < 0 {
		if list > 0 {
			delayTime = time.Duration(list * uint64(delayTimeModifier)) * time.Second
		}else {
			delayTime = time.Duration(1) * time.Second
		}
	}else if delayTime < (time.Duration(list * uint64(delayTimeModifier)) * time.Second) {
		if list > 0 {
			delayTime += time.Duration(list * uint64(delayTimeModifier)) * time.Second
		}
	}
	log.Info("calcDelayTime", "header.Number", header.Number, "delayTime", delayTime, "listOrder", list, "adjustTime", adjust, "header.Time", time.Unix(header.Time.Int64(), 0), "coinbase", header.Coinbase)
	return delayTime, nil
}
