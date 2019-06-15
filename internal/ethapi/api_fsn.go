package ethapi

import (
	"context"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/FusionFoundation/efsn/accounts"
	"github.com/FusionFoundation/efsn/common"
	"github.com/FusionFoundation/efsn/common/hexutil"
	"github.com/FusionFoundation/efsn/core/rawdb"
	"github.com/FusionFoundation/efsn/core/types"
	"github.com/FusionFoundation/efsn/log"
	"github.com/FusionFoundation/efsn/rlp"
	"github.com/FusionFoundation/efsn/rpc"
)

var lastBlockOfBuyTickets = int64(0)
var buyTicketOnBlockMap map[common.Address]bool
var buyTicketOnBlockMapMutex sync.Mutex

// FusionBaseArgs wacom
type FusionBaseArgs struct {
	From     common.Address  `json:"from"`
	Gas      *hexutil.Uint64 `json:"gas"`
	GasPrice *hexutil.Big    `json:"gasPrice"`
	Nonce    *hexutil.Uint64 `json:"nonce"`
}

// GenAssetArgs wacom
type GenAssetArgs struct {
	FusionBaseArgs
	Name        string       `json:"name"`
	Symbol      string       `json:"symbol"`
	Decimals    uint8        `json:"decimals"`
	Total       *hexutil.Big `json:"total"`
	CanChange   bool         `json:"canChange"`
	Description string       `json:"description"`
}

// SendAssetArgs wacom
type SendAssetArgs struct {
	FusionBaseArgs
	AssetID common.Hash    `json:"asset"`
	To      common.Address `json:"to"`
	Value   *hexutil.Big   `json:"value"`
}

// TimeLockArgs wacom
type TimeLockArgs struct {
	SendAssetArgs
	StartTime *hexutil.Uint64 `json:"start"`
	EndTime   *hexutil.Uint64 `json:"end"`
}

// BuyTicketArgs wacom
type BuyTicketArgs struct {
	FusionBaseArgs
	Start *hexutil.Uint64 `json:"start"`
	End   *hexutil.Uint64 `json:"end"`
}

// AssetValueChangeArgs wacom
type AssetValueChangeArgs struct {
	FusionBaseArgs
	AssetID common.Hash    `json:"asset"`
	To      common.Address `json:"to"`
	Value   *hexutil.Big   `json:"value"`
	IsInc   bool           `json:"isInc"`
}

type AssetValueChangeExArgs struct {
	FusionBaseArgs
	AssetID     common.Hash    `json:"asset"`
	To          common.Address `json:"to"`
	Value       *hexutil.Big   `json:"value"`
	IsInc       bool           `json:"isInc"`
	TransacData string         `json:"transacData"`
}

// MakeSwapArgs wacom
type MakeSwapArgs struct {
	FusionBaseArgs
	FromAssetID   common.Hash
	FromStartTime *hexutil.Uint64
	FromEndTime   *hexutil.Uint64
	MinFromAmount *hexutil.Big
	ToAssetID     common.Hash
	ToStartTime   *hexutil.Uint64
	ToEndTime     *hexutil.Uint64
	MinToAmount   *hexutil.Big
	SwapSize      *big.Int
	Targes        []common.Address
	Time          *big.Int
	Description   string
}

// RecallSwapArgs wacom
type RecallSwapArgs struct {
	FusionBaseArgs
	SwapID common.Hash
}

// TakeSwapArgs wacom
type TakeSwapArgs struct {
	FusionBaseArgs
	SwapID common.Hash
	Size   *big.Int
}

// MakeMultiSwapArgs wacom
type MakeMultiSwapArgs struct {
	FusionBaseArgs
	FromAssetID   []common.Hash
	FromStartTime []*hexutil.Uint64
	FromEndTime   []*hexutil.Uint64
	MinFromAmount []*hexutil.Big
	ToAssetID     []common.Hash
	ToStartTime   []*hexutil.Uint64
	ToEndTime     []*hexutil.Uint64
	MinToAmount   []*hexutil.Big
	SwapSize      *big.Int
	Targes        []common.Address
	Time          *big.Int
	Description   string
}

// RecallMultiSwapArgs wacom
type RecallMultiSwapArgs struct {
	FusionBaseArgs
	SwapID common.Hash
}

// TakeSwapArgs wacom
type TakeMultiSwapArgs struct {
	FusionBaseArgs
	SwapID common.Hash
	Size   *big.Int
}

func (args *FusionBaseArgs) toSendArgs() SendTxArgs {
	return SendTxArgs{
		From:     args.From,
		Gas:      args.Gas,
		GasPrice: args.GasPrice,
		Nonce:    args.Nonce,
	}
}

func (args *SendAssetArgs) toParam() *common.SendAssetParam {
	return &common.SendAssetParam{
		AssetID: args.AssetID,
		To:      args.To,
		Value:   args.Value.ToInt(),
	}
}

func (args *SendAssetArgs) toData() ([]byte, error) {
	return args.toParam().ToBytes()
}

func (args *TimeLockArgs) toParam(typ common.TimeLockType) *common.TimeLockParam {
	return &common.TimeLockParam{
		Type:      typ,
		AssetID:   args.AssetID,
		To:        args.To,
		StartTime: uint64(*args.StartTime),
		EndTime:   uint64(*args.EndTime),
		Value:     args.Value.ToInt(),
	}
}

func (args *TimeLockArgs) toData(typ common.TimeLockType) ([]byte, error) {
	return args.toParam(typ).ToBytes()
}

func (args *GenAssetArgs) toParam() *common.GenAssetParam {
	return &common.GenAssetParam{
		Name:        args.Name,
		Symbol:      args.Symbol,
		Decimals:    args.Decimals,
		Total:       args.Total.ToInt(),
		CanChange:   args.CanChange,
		Description: args.Description,
	}
}

func (args *GenAssetArgs) toData() ([]byte, error) {
	return args.toParam().ToBytes()
}

func (args *BuyTicketArgs) toParam() *common.BuyTicketParam {
	return &common.BuyTicketParam{
		Start: uint64(*args.Start),
		End:   uint64(*args.End),
	}
}

func (args *BuyTicketArgs) toData() ([]byte, error) {
	return args.toParam().ToBytes()
}

func (args *BuyTicketArgs) init(defStart uint64) {

	if args.Start == nil {
		args.Start = new(hexutil.Uint64)
		*(*uint64)(args.Start) = defStart
	}

	if args.End == nil {
		args.End = new(hexutil.Uint64)
		*(*uint64)(args.End) = uint64(*args.Start) + 30*24*3600
	}
}

func (args *AssetValueChangeArgs) toData() ([]byte, error) {
	param := common.AssetValueChangeParam{
		AssetID: args.AssetID,
		To:      args.To,
		Value:   args.Value.ToInt(),
		IsInc:   args.IsInc,
	}
	return param.ToBytes()
}

func (args *AssetValueChangeExArgs) toParam() *common.AssetValueChangeExParam {
	return &common.AssetValueChangeExParam{
		AssetID:     args.AssetID,
		To:          args.To,
		Value:       args.Value.ToInt(),
		IsInc:       args.IsInc,
		TransacData: args.TransacData,
	}
}

func (args *AssetValueChangeExArgs) toData() ([]byte, error) {
	return args.toParam().ToBytes()
}

func (args *MakeSwapArgs) init() {

	if args.FromStartTime == nil {
		args.FromStartTime = new(hexutil.Uint64)
		*(*uint64)(args.FromStartTime) = common.TimeLockNow
	}

	if args.FromEndTime == nil {
		args.FromEndTime = new(hexutil.Uint64)
		*(*uint64)(args.FromEndTime) = common.TimeLockForever
	}

	if args.ToStartTime == nil {
		args.ToStartTime = new(hexutil.Uint64)
		*(*uint64)(args.ToStartTime) = common.TimeLockNow
	}

	if args.ToEndTime == nil {
		args.ToEndTime = new(hexutil.Uint64)
		*(*uint64)(args.ToEndTime) = common.TimeLockForever
	}
}

func (args *MakeSwapArgs) toParam(time *big.Int) *common.MakeSwapParam {
	return &common.MakeSwapParam{
		FromAssetID:   args.FromAssetID,
		FromStartTime: uint64(*args.FromStartTime),
		FromEndTime:   uint64(*args.FromEndTime),
		MinFromAmount: args.MinFromAmount.ToInt(),
		ToAssetID:     args.ToAssetID,
		ToStartTime:   uint64(*args.ToStartTime),
		ToEndTime:     uint64(*args.ToEndTime),
		MinToAmount:   args.MinToAmount.ToInt(),
		SwapSize:      args.SwapSize,
		Targes:        args.Targes,
		Time:          time,
		Description:   args.Description,
	}
}

func (args *MakeSwapArgs) toData(time *big.Int) ([]byte, error) {
	return args.toParam(time).ToBytes()
}

func (args *RecallSwapArgs) toParam() *common.RecallSwapParam {
	return &common.RecallSwapParam{
		SwapID: args.SwapID,
	}
}

func (args *RecallSwapArgs) toData() ([]byte, error) {
	return args.toParam().ToBytes()
}

func (args *TakeSwapArgs) toParam() *common.TakeSwapParam {
	return &common.TakeSwapParam{
		SwapID: args.SwapID,
		Size:   args.Size,
	}
}

func (args *TakeSwapArgs) toData() ([]byte, error) {
	return args.toParam().ToBytes()
}

func (args *MakeMultiSwapArgs) init() {

	if args.FromStartTime == nil || len(args.FromStartTime) == 0 {
		args.FromStartTime = make([]*hexutil.Uint64, 1)
		*(*uint64)(args.FromStartTime[0]) = common.TimeLockNow
	}

	if args.FromEndTime == nil || len(args.FromEndTime) == 0 {
		args.FromEndTime = make([]*hexutil.Uint64, 1)
		*(*uint64)(args.FromEndTime[0]) = common.TimeLockForever
	}

	if args.ToStartTime == nil || len(args.ToStartTime) == 0 {
		args.ToStartTime = make([]*hexutil.Uint64, 1)
		*(*uint64)(args.ToStartTime[0]) = common.TimeLockNow
	}

	if args.ToEndTime == nil || len(args.ToEndTime) == 0 {
		args.ToEndTime = make([]*hexutil.Uint64, 1)
		*(*uint64)(args.ToEndTime[0]) = common.TimeLockForever
	}
}

func (args *MakeMultiSwapArgs) toParam(time *big.Int) *common.MakeMultiSwapParam {

	fromStartTime := make([]uint64, len(args.FromStartTime))
	for i := 0; i < len(args.FromStartTime); i++ {
		fromStartTime[i] = uint64(*args.FromStartTime[i])
	}
	fromEndTime := make([]uint64, len(args.FromEndTime))
	for i := 0; i < len(args.FromEndTime); i++ {
		fromEndTime[i] = uint64(*args.FromEndTime[i])
	}
	minFromAmount := make([]*big.Int, len(args.MinFromAmount))
	for i := 0; i < len(args.MinFromAmount); i++ {
		minFromAmount[i] = args.MinFromAmount[i].ToInt()
	}
	toStartTime := make([]uint64, len(args.ToStartTime))
	for i := 0; i < len(args.ToStartTime); i++ {
		toStartTime[i] = uint64(*args.ToStartTime[i])
	}
	toEndTime := make([]uint64, len(args.ToEndTime))
	for i := 0; i < len(args.ToEndTime); i++ {
		toEndTime[i] = uint64(*args.ToEndTime[i])
	}
	minToAmount := make([]*big.Int, len(args.MinToAmount))
	for i := 0; i < len(args.MinToAmount); i++ {
		minToAmount[i] = args.MinToAmount[i].ToInt()
	}
	return &common.MakeMultiSwapParam{
		FromAssetID:   args.FromAssetID,
		FromStartTime: fromStartTime,
		FromEndTime:   fromEndTime,
		MinFromAmount: minFromAmount,
		ToAssetID:     args.ToAssetID,
		ToStartTime:   toStartTime,
		ToEndTime:     toEndTime,
		MinToAmount:   minToAmount,
		SwapSize:      args.SwapSize,
		Targes:        args.Targes,
		Time:          time,
		Description:   args.Description,
	}
}

func (args *MakeMultiSwapArgs) toData(time *big.Int) ([]byte, error) {
	return args.toParam(time).ToBytes()
}

func (args *RecallMultiSwapArgs) toParam() *common.RecallMultiSwapParam {
	return &common.RecallMultiSwapParam{
		SwapID: args.SwapID,
	}
}

func (args *RecallMultiSwapArgs) toData() ([]byte, error) {
	return args.toParam().ToBytes()
}

func (args *TakeMultiSwapArgs) toParam() *common.TakeMultiSwapParam {
	return &common.TakeMultiSwapParam{
		SwapID: args.SwapID,
		Size:   args.Size,
	}
}

func (args *TakeMultiSwapArgs) toData() ([]byte, error) {
	return args.toParam().ToBytes()
}

func (args *TimeLockArgs) init() {

	if args.StartTime == nil {
		args.StartTime = new(hexutil.Uint64)
		*(*uint64)(args.StartTime) = common.TimeLockNow
	}

	if args.EndTime == nil {
		args.EndTime = new(hexutil.Uint64)
		*(*uint64)(args.EndTime) = common.TimeLockForever
	}
}

// PublicFusionAPI ss
type PublicFusionAPI struct {
	b Backend
}

// NewPublicFusionAPI ss
func NewPublicFusionAPI(b Backend) *PublicFusionAPI {
	return &PublicFusionAPI{
		b: b,
	}
}

// GetBalance wacom
func (s *PublicFusionAPI) GetBalance(ctx context.Context, assetID common.Hash, address common.Address, blockNr rpc.BlockNumber) (string, error) {
	state, _, err := s.b.StateAndHeaderByNumber(ctx, blockNr)
	if state == nil || err != nil {
		return "0", err
	}
	b := state.GetBalance(assetID, address)
	return b.String(), state.Error()
}

// GetAllBalances wacom
func (s *PublicFusionAPI) GetAllBalances(ctx context.Context, address common.Address, blockNr rpc.BlockNumber) (map[common.Hash]string, error) {
	state, _, err := s.b.StateAndHeaderByNumber(ctx, blockNr)
	if state == nil || err != nil {
		return make(map[common.Hash]string), err
	}
	b := state.GetAllBalances(address)
	return b, state.Error()
}

// GetTimeLockBalance wacom
func (s *PublicFusionAPI) GetTimeLockBalance(ctx context.Context, assetID common.Hash, address common.Address, blockNr rpc.BlockNumber) (*common.TimeLock, error) {
	state, _, err := s.b.StateAndHeaderByNumber(ctx, blockNr)
	if state == nil || err != nil {
		return new(common.TimeLock), err
	}
	b := state.GetTimeLockBalance(assetID, address)
	if state.Error() == nil {
		b = b.ToDisplay()
	}
	return b, state.Error()
}

// GetAllTimeLockBalances wacom
func (s *PublicFusionAPI) GetAllTimeLockBalances(ctx context.Context, address common.Address, blockNr rpc.BlockNumber) (map[common.Hash]*common.TimeLock, error) {
	state, _, err := s.b.StateAndHeaderByNumber(ctx, blockNr)
	if state == nil || err != nil {
		return make(map[common.Hash]*common.TimeLock), err
	}
	b := state.GetAllTimeLockBalances(address)
	if state.Error() == nil {
		for k, v := range b {
			b[k] = v.ToDisplay()
		}
	}
	return b, state.Error()
}

// GetNotation wacom
func (s *PublicFusionAPI) GetNotation(ctx context.Context, address common.Address, blockNr rpc.BlockNumber) (uint64, error) {
	state, _, err := s.b.StateAndHeaderByNumber(ctx, blockNr)
	if state == nil || err != nil {
		return 0, err
	}
	b := state.CalcNotationDisplay(state.GetNotation(address))
	return b, state.Error()
}

// GetAddressByNotation wacom
func (s *PublicFusionAPI) GetAddressByNotation(ctx context.Context, notation uint64, blockNr rpc.BlockNumber) (common.Address, error) {
	state, _, err := s.b.StateAndHeaderByNumber(ctx, blockNr)
	if state == nil || err != nil {
		return common.Address{}, err
	}
	address, err := state.GetAddressByNotation(notation)
	if err != nil {
		log.Error("GetAddressByNotation: error ", "err", err)
		return common.Address{}, err
	}
	return address, nil
}

// AllNotation wacom
func (s *PublicFusionAPI) AllNotation(ctx context.Context, blockNr rpc.BlockNumber) (map[common.Address]uint64, error) {
	return nil, fmt.Errorf("AllNotations has been depreciated please use api.fusionnetwork.io")
}

// GetAsset wacom
func (s *PublicFusionAPI) GetAsset(ctx context.Context, assetID common.Hash, blockNr rpc.BlockNumber) (*common.Asset, error) {
	state, _, err := s.b.StateAndHeaderByNumber(ctx, blockNr)
	if state == nil || err != nil {
		return nil, err
	}
	asset, assetErr := state.GetAsset(assetID)

	if assetErr != nil {
		return nil, fmt.Errorf("Asset not found")
	}
	return &asset, nil
}

// AllAssets wacom
func (s *PublicFusionAPI) AllAssets(ctx context.Context, blockNr rpc.BlockNumber) (map[common.Hash]common.Asset, error) {
	return nil, fmt.Errorf("AllAssets has been depreciated, use api.fusionnetwork.io")
}

// AllAssetsByAddress wacom
func (s *PublicFusionAPI) AllAssetsByAddress(ctx context.Context, address common.Address, blockNr rpc.BlockNumber) (map[common.Hash]common.Asset, error) {
	return nil, fmt.Errorf("AllAssetsByAddress has been depreciated, use api.fusionnetwork.io")
}

// AssetExistForAddress wacom
func (s *PublicFusionAPI) AssetExistForAddress(ctx context.Context, assetName string, address common.Address, blockNr rpc.BlockNumber) (common.Hash, error) {
	return common.Hash{}, fmt.Errorf("AllAssetsByAddress has been depreciated, use api.fusionnetwork.io")
}

func (s *PublicFusionAPI) getAllTickets(ctx context.Context, blockNr rpc.BlockNumber) (common.TicketsDataSlice, error) {
	state, _, err := s.b.StateAndHeaderByNumber(ctx, blockNr)
	if state == nil || err != nil {
		return nil, err
	}
	tickets, err := state.AllTickets()
	if err == nil {
		err = state.Error()
	}
	if err != nil {
		log.Debug("AllTickets:apifsn.go unable to retrieve previous tickets")
		return nil, fmt.Errorf("AllTickets:apifsn.go unable to retrieve previous tickets. error: %v", err)
	}
	return tickets, nil
}

// AllTickets wacom
func (s *PublicFusionAPI) AllTickets(ctx context.Context, blockNr rpc.BlockNumber) (map[common.Hash]common.TicketDisplay, error) {
	tickets, err := s.getAllTickets(ctx, blockNr)
	if err != nil {
		return nil, err
	}
	return tickets.ToMap(), nil
}

// TotalNumberOfTickets wacom
func (s *PublicFusionAPI) TotalNumberOfTickets(ctx context.Context, blockNr rpc.BlockNumber) (int, error) {
	tickets, err := s.getAllTickets(ctx, blockNr)
	if err != nil {
		return 0, err
	}
	return int(tickets.NumberOfTickets()), err
}

// TotalNumberOfTicketsByAddress wacom
func (s *PublicFusionAPI) TotalNumberOfTicketsByAddress(ctx context.Context, address common.Address, blockNr rpc.BlockNumber) (int, error) {
	tickets, err := s.getAllTickets(ctx, blockNr)
	if err != nil {
		return 0, err
	}
	return int(tickets.NumberOfTicketsByAddress(address)), err
}

// TicketPrice wacom
func (s *PublicFusionAPI) TicketPrice(ctx context.Context, blockNr rpc.BlockNumber) (string, error) {
	state, header, err := s.b.StateAndHeaderByNumber(ctx, blockNr)
	if state == nil || err != nil {
		return "", err
	}
	return common.TicketPrice(header.Number).String(), nil
}

// AllTicketsByAddress wacom
func (s *PublicFusionAPI) AllTicketsByAddress(ctx context.Context, address common.Address, blockNr rpc.BlockNumber) (map[common.Hash]common.TicketDisplay, error) {
	tickets, err := s.getAllTickets(ctx, blockNr)
	if err != nil {
		return nil, err
	}
	for _, v := range tickets {
		if v.Owner == address {
			return v.ToMap(), nil
		}
	}
	return nil, nil
}

func (s *PublicFusionAPI) getSwapIDByTxHash(hash common.Hash) common.Hash {
	var swapID common.Hash
	tx, _, _, _ := rawdb.ReadTransaction(s.b.ChainDb(), hash)
	if tx == nil {
		tx = s.b.GetPoolTransaction(hash)
	}
	if tx != nil {
		var signer types.Signer = types.FrontierSigner{}
		if tx.Protected() {
			signer = types.NewEIP155Signer(tx.ChainId())
		}
		if msg, err := tx.AsMessage(signer); err == nil {
			swapID = msg.AsTransaction().Hash()
		}
	}
	return swapID
}

// GetSwap wacom
func (s *PublicFusionAPI) GetSwap(ctx context.Context, swapID common.Hash, blockNr rpc.BlockNumber) (*common.Swap, error) {
	state, _, err := s.b.StateAndHeaderByNumber(ctx, blockNr)
	if state == nil || err != nil {
		return nil, err
	}
	if swap, err := state.GetSwap(swapID); err == nil {
		return &swap, nil
	}
	// treat swapId as tx hash, deduct swap id from the tx
	if id := s.getSwapIDByTxHash(swapID); id != (common.Hash{}) {
		if swap, err := state.GetSwap(id); err == nil {
			return &swap, nil
		}
	}
	return nil, fmt.Errorf("Swap not found")
}

// GetMultiSwap wacom
func (s *PublicFusionAPI) GetMultiSwap(ctx context.Context, swapID common.Hash, blockNr rpc.BlockNumber) (*common.MultiSwap, error) {
	state, _, err := s.b.StateAndHeaderByNumber(ctx, blockNr)
	if state == nil || err != nil {
		return nil, err
	}
	if swap, err := state.GetMultiSwap(swapID); err == nil {
		return &swap, nil
	}
	// treat swapId as tx hash, deduct swap id from the tx
	if id := s.getSwapIDByTxHash(swapID); id != (common.Hash{}) {
		if swap, err := state.GetMultiSwap(id); err == nil {
			return &swap, nil
		}
	}
	return nil, fmt.Errorf("MultiSwap not found")
}

// AllSwaps wacom
func (s *PublicFusionAPI) AllSwaps(ctx context.Context, blockNr rpc.BlockNumber) (map[common.Hash]common.Swap, error) {
	return nil, fmt.Errorf("AllSwaps has been depreciated please use api.fusionnetwork.io")
}

// AllSwapsByAddress wacom
func (s *PublicFusionAPI) AllSwapsByAddress(ctx context.Context, address common.Address, blockNr rpc.BlockNumber) (map[common.Hash]common.Swap, error) {
	return nil, fmt.Errorf("AllSwapsByAddress has been depreciated please use api.fusionnetwork.io")
}

// PrivateFusionAPI ss
type PrivateFusionAPI struct {
	am        *accounts.Manager
	nonceLock *AddrLocker
	b         Backend
	papi      *PrivateAccountAPI
}

// store privateFusionAPI
var privateFusionAPI = &PrivateFusionAPI{}

func AutoBuyTicket(account common.Address, passwd string) {
	for {
		select {
		case <-common.AutoBuyTicketChan:
			if privateFusionAPI.b.IsMining() {
				fbase := FusionBaseArgs{From: account}
				args := BuyTicketArgs{FusionBaseArgs: fbase}
				privateFusionAPI.BuyTicket(nil, args, passwd)
			}
		}
	}
}

// NewPrivateFusionAPI ss
func NewPrivateFusionAPI(b Backend, nonceLock *AddrLocker, papi *PrivateAccountAPI) *PrivateFusionAPI {
	privateFusionAPI = &PrivateFusionAPI{
		am:        b.AccountManager(),
		nonceLock: nonceLock,
		b:         b,
		papi:      papi,
	}
	return privateFusionAPI
}

// GenNotation ss
func (s *PrivateFusionAPI) GenNotation(ctx context.Context, args FusionBaseArgs, passwd string) (common.Hash, error) {

	state, _, err := s.b.StateAndHeaderByNumber(ctx, rpc.LatestBlockNumber)
	if state == nil || err != nil {
		return common.Hash{}, err
	}

	notation := state.GetNotation(args.From)

	if notation != 0 {
		return common.Hash{}, fmt.Errorf("An address can have only one notation, you already have a mapped notation:%d", state.CalcNotationDisplay(notation))
	}

	var param = common.FSNCallParam{Func: common.GenNotationFunc}
	data, err := param.ToBytes()
	if err != nil {
		return common.Hash{}, err
	}
	var argsData = hexutil.Bytes(data)
	sendArgs := args.toSendArgs()
	sendArgs.To = &common.FSNCallAddress
	sendArgs.Data = &argsData
	return s.papi.SendTransaction(ctx, sendArgs, passwd)
}

// GenAsset ss
func (s *PrivateFusionAPI) GenAsset(ctx context.Context, args GenAssetArgs, passwd string) (common.Hash, error) {
	if err := args.toParam().Check(common.BigMaxUint64); err != nil {
		return common.Hash{}, err
	}

	funcData, err := args.toData()
	if err != nil {
		return common.Hash{}, err
	}

	var param = common.FSNCallParam{Func: common.GenAssetFunc, Data: funcData}
	data, err := param.ToBytes()
	if err != nil {
		return common.Hash{}, err
	}
	var argsData = hexutil.Bytes(data)
	sendArgs := args.toSendArgs()
	sendArgs.To = &common.FSNCallAddress
	sendArgs.Data = &argsData
	return s.papi.SendTransaction(ctx, sendArgs, passwd)
}

// SendAsset ss
func (s *PrivateFusionAPI) SendAsset(ctx context.Context, args SendAssetArgs, passwd string) (common.Hash, error) {
	state, _, err := s.b.StateAndHeaderByNumber(ctx, rpc.LatestBlockNumber)
	if state == nil || err != nil {
		return common.Hash{}, err
	}
	if err := args.toParam().Check(common.BigMaxUint64); err != nil {
		return common.Hash{}, err
	}

	if state.GetBalance(args.AssetID, args.From).Cmp(args.Value.ToInt()) < 0 {
		return common.Hash{}, fmt.Errorf("not enough asset")
	}

	funcData, err := args.toData()
	if err != nil {
		return common.Hash{}, err
	}
	var param = common.FSNCallParam{Func: common.SendAssetFunc, Data: funcData}
	data, err := param.ToBytes()
	if err != nil {
		return common.Hash{}, err
	}
	var argsData = hexutil.Bytes(data)
	sendArgs := args.toSendArgs()
	sendArgs.To = &common.FSNCallAddress
	sendArgs.Data = &argsData
	sendArgs.Value = (*hexutil.Big)(big.NewInt(0))
	return s.papi.SendTransaction(ctx, sendArgs, passwd)
}

// AssetToTimeLock ss
func (s *PrivateFusionAPI) AssetToTimeLock(ctx context.Context, args TimeLockArgs, passwd string) (common.Hash, error) {
	state, header, err := s.b.StateAndHeaderByNumber(ctx, rpc.LatestBlockNumber)
	if state == nil || err != nil {
		return common.Hash{}, err
	}
	args.init()
	if err := args.toParam(common.AssetToTimeLock).Check(common.BigMaxUint64, header.Time.Uint64()); err != nil {
		return common.Hash{}, err
	}
	needValue := common.NewTimeLock(&common.TimeLockItem{
		StartTime: common.MaxUint64(uint64(*args.StartTime), header.Time.Uint64()),
		EndTime:   uint64(*args.EndTime),
		Value:     args.Value.ToInt(),
	})
	if err := needValue.IsValid(); err != nil {
		return common.Hash{}, fmt.Errorf("AssetToTimeLock err:%v", err.Error())
	}
	if state.GetBalance(args.AssetID, args.From).Cmp(args.Value.ToInt()) < 0 {
		return common.Hash{}, fmt.Errorf("not enough asset")
	}
	funcData, err := args.toData(common.AssetToTimeLock)
	if err != nil {
		return common.Hash{}, err
	}
	var param = common.FSNCallParam{Func: common.TimeLockFunc, Data: funcData}
	data, err := param.ToBytes()
	if err != nil {
		return common.Hash{}, err
	}
	var argsData = hexutil.Bytes(data)
	sendArgs := args.toSendArgs()
	sendArgs.To = &common.FSNCallAddress
	sendArgs.Data = &argsData
	return s.papi.SendTransaction(ctx, sendArgs, passwd)
}

// TimeLockToTimeLock ss
func (s *PrivateFusionAPI) TimeLockToTimeLock(ctx context.Context, args TimeLockArgs, passwd string) (common.Hash, error) {
	state, header, err := s.b.StateAndHeaderByNumber(ctx, rpc.LatestBlockNumber)
	if state == nil || err != nil {
		return common.Hash{}, err
	}

	args.init()
	if err := args.toParam(common.TimeLockToTimeLock).Check(common.BigMaxUint64, header.Time.Uint64()); err != nil {
		return common.Hash{}, err
	}
	needValue := common.NewTimeLock(&common.TimeLockItem{
		StartTime: common.MaxUint64(uint64(*args.StartTime), header.Time.Uint64()),
		EndTime:   uint64(*args.EndTime),
		Value:     args.Value.ToInt(),
	})
	if err := needValue.IsValid(); err != nil {
		return common.Hash{}, fmt.Errorf("TimeLockToTimeLock err:%v", err.Error())
	}

	if state.GetTimeLockBalance(args.AssetID, args.From).Cmp(needValue) < 0 {
		return common.Hash{}, fmt.Errorf("not enough time lock balance")
	}

	funcData, err := args.toData(common.TimeLockToTimeLock)
	if err != nil {
		return common.Hash{}, err
	}
	var param = common.FSNCallParam{Func: common.TimeLockFunc, Data: funcData}
	data, err := param.ToBytes()
	if err != nil {
		return common.Hash{}, err
	}
	var argsData = hexutil.Bytes(data)
	sendArgs := args.toSendArgs()
	sendArgs.To = &common.FSNCallAddress
	sendArgs.Data = &argsData
	return s.papi.SendTransaction(ctx, sendArgs, passwd)
}

// TimeLockToAsset ss
func (s *PrivateFusionAPI) TimeLockToAsset(ctx context.Context, args TimeLockArgs, passwd string) (common.Hash, error) {
	state, header, err := s.b.StateAndHeaderByNumber(ctx, rpc.LatestBlockNumber)
	if state == nil || err != nil {
		return common.Hash{}, err
	}
	args.init()
	*(*uint64)(args.StartTime) = header.Time.Uint64()
	*(*uint64)(args.EndTime) = common.TimeLockForever
	if err := args.toParam(common.TimeLockToAsset).Check(common.BigMaxUint64, header.Time.Uint64()); err != nil {
		return common.Hash{}, err
	}
	needValue := common.NewTimeLock(&common.TimeLockItem{
		StartTime: uint64(*args.StartTime),
		EndTime:   uint64(*args.EndTime),
		Value:     args.Value.ToInt(),
	})
	if err := needValue.IsValid(); err != nil {
		return common.Hash{}, fmt.Errorf("TimeLockToAsset err:%v", err.Error())
	}
	if state.GetTimeLockBalance(args.AssetID, args.From).Cmp(needValue) < 0 {
		return common.Hash{}, fmt.Errorf("not enough time lock balance")
	}
	funcData, err := args.toData(common.TimeLockToAsset)
	if err != nil {
		return common.Hash{}, err
	}
	var param = common.FSNCallParam{Func: common.TimeLockFunc, Data: funcData}
	data, err := param.ToBytes()
	if err != nil {
		return common.Hash{}, err
	}
	var argsData = hexutil.Bytes(data)
	sendArgs := args.toSendArgs()
	sendArgs.To = &common.FSNCallAddress
	sendArgs.Data = &argsData
	return s.papi.SendTransaction(ctx, sendArgs, passwd)
}

/** on our public gateways too many buyTickets are past through
this cache of purchase on block will stop multiple purchase
attempt on a block (which state_transistion also flags).
the goals is to limit the number of buytickets being processed
if it is know that they will fail anyway
*/
func doesTicketPurchaseExistsForBlock(blockNbr int64, from common.Address) bool {
	buyTicketOnBlockMapMutex.Lock()
	defer buyTicketOnBlockMapMutex.Unlock()
	if lastBlockOfBuyTickets == 0 || lastBlockOfBuyTickets != blockNbr {
		lastBlockOfBuyTickets = blockNbr
		buyTicketOnBlockMap = make(map[common.Address]bool)
	}
	_, found := buyTicketOnBlockMap[from]
	return found
}

// only record on purchase ticket successfully
func addTicketPurchaseForBlock(from common.Address) {
	buyTicketOnBlockMapMutex.Lock()
	defer buyTicketOnBlockMapMutex.Unlock()
	buyTicketOnBlockMap[from] = true
}

// BuyTicket ss
func (s *PrivateFusionAPI) BuyTicket(ctx context.Context, args BuyTicketArgs, passwd string) (common.Hash, error) {
	state, header, err := s.b.StateAndHeaderByNumber(ctx, rpc.LatestBlockNumber)
	if state == nil || err != nil {
		return common.Hash{}, err
	}

	if doesTicketPurchaseExistsForBlock(header.Number.Int64(), args.From) {
		log.Info("Purchase of BuyTicket for this block already submitted")
		return common.Hash{}, fmt.Errorf("Purchase of BuyTicket for this block already submitted")
	}

	args.init(header.Time.Uint64())
	now := uint64(time.Now().Unix())
	if err := args.toParam().Check(common.BigMaxUint64, now, 600); err != nil {
		return common.Hash{}, err
	}

	start := uint64(*args.Start)
	end := uint64(*args.End)
	value := common.TicketPrice(header.Number)
	needValue := common.NewTimeLock(&common.TimeLockItem{
		StartTime: common.MaxUint64(start, header.Time.Uint64()),
		EndTime:   end,
		Value:     value,
	})
	if err := needValue.IsValid(); err != nil {
		return common.Hash{}, fmt.Errorf("BuyTicket err:%v", err.Error())
	}
	if state.GetTimeLockBalance(common.SystemAssetID, args.From).Cmp(needValue) < 0 {
		if state.GetBalance(common.SystemAssetID, args.From).Cmp(value) < 0 {
			return common.Hash{}, fmt.Errorf("not enough time lock or asset balance")
		}
	}
	data, err := args.toData()
	if err != nil {
		return common.Hash{}, err
	}
	var param = common.FSNCallParam{Func: common.BuyTicketFunc, Data: data}
	data, err = param.ToBytes()
	if err != nil {
		return common.Hash{}, err
	}
	var argsData = hexutil.Bytes(data)
	sendArgs := args.toSendArgs()
	sendArgs.To = &common.FSNCallAddress
	sendArgs.Data = &argsData
	hash, err := s.papi.SendTransaction(ctx, sendArgs, passwd)
	if err != nil {
		return common.Hash{}, err
	}
	addTicketPurchaseForBlock(args.From)
	return hash, err
}

// IncAsset ss
func (s *PrivateFusionAPI) IncAsset(ctx context.Context, args AssetValueChangeExArgs, passwd string) (common.Hash, error) {
	args.IsInc = true
	return s.checkAssetValueChange(ctx, args, passwd)
}

// DecAsset ss
func (s *PrivateFusionAPI) DecAsset(ctx context.Context, args AssetValueChangeExArgs, passwd string) (common.Hash, error) {
	args.IsInc = false
	return s.checkAssetValueChange(ctx, args, passwd)
}

func (s *PrivateFusionAPI) checkAssetValueChange(ctx context.Context, args AssetValueChangeExArgs, passwd string) (common.Hash, error) {
	state, _, err := s.b.StateAndHeaderByNumber(ctx, rpc.LatestBlockNumber)
	if state == nil || err != nil {
		return common.Hash{}, err
	}

	if err := args.toParam().Check(common.BigMaxUint64); err != nil {
		return common.Hash{}, err
	}

	asset, assetError := state.GetAsset(args.AssetID)
	if assetError != nil {
		return common.Hash{}, fmt.Errorf("asset not found")
	}

	if !asset.CanChange {
		return common.Hash{}, fmt.Errorf("asset can't inc or dec")
	}

	if asset.Owner != args.From {
		return common.Hash{}, fmt.Errorf("must be change by onwer")
	}

	currentBalance := state.GetBalance(args.AssetID, args.To)
	val := args.Value.ToInt()
	if !args.IsInc {
		if currentBalance.Cmp(val) < 0 {
			return common.Hash{}, fmt.Errorf("not enough asset")
		}
	}

	funcData, err := args.toData()
	if err != nil {
		return common.Hash{}, err
	}
	var param = common.FSNCallParam{Func: common.AssetValueChangeFunc, Data: funcData}
	data, err := param.ToBytes()
	if err != nil {
		return common.Hash{}, err
	}
	var argsData = hexutil.Bytes(data)
	sendArgs := args.toSendArgs()
	sendArgs.To = &common.FSNCallAddress
	sendArgs.Data = &argsData
	return s.papi.SendTransaction(ctx, sendArgs, passwd)
}

// MakeSwap ss
func (s *PrivateFusionAPI) MakeSwap(ctx context.Context, args MakeSwapArgs, passwd string) (common.Hash, error) {
	state, header, err := s.b.StateAndHeaderByNumber(ctx, rpc.LatestBlockNumber)
	if state == nil || err != nil {
		return common.Hash{}, err
	}

	args.init()
	now := uint64(time.Now().Unix())
	if err := args.toParam(header.Time).Check(common.BigMaxUint64, now); err != nil {
		return common.Hash{}, err
	}

	total := new(big.Int).Mul(args.MinFromAmount.ToInt(), args.SwapSize)
	start := uint64(*args.FromStartTime)
	end := uint64(*args.FromEndTime)

	if args.FromAssetID == common.OwnerUSANAssetID {
		notation := state.GetNotation(args.From)
		if notation == 0 {
			return common.Hash{}, fmt.Errorf("from address does not have a notation")
		}
	} else if start == common.TimeLockNow && end == common.TimeLockForever {
		if state.GetBalance(args.FromAssetID, args.From).Cmp(total) < 0 {
			return common.Hash{}, fmt.Errorf("not enough from asset")
		}
	} else {
		needValue := common.NewTimeLock(&common.TimeLockItem{
			StartTime: common.MaxUint64(start, header.Time.Uint64()),
			EndTime:   end,
			Value:     total,
		})
		if err := needValue.IsValid(); err != nil {
			return common.Hash{}, fmt.Errorf("MakeSwap from err:%v", err.Error())
		}
		if state.GetTimeLockBalance(args.FromAssetID, args.From).Cmp(needValue) < 0 {
			if state.GetBalance(args.FromAssetID, args.From).Cmp(total) < 0 {
				return common.Hash{}, fmt.Errorf("not enough time lock or asset balance")
			}
		}
	}

	funcData, err := args.toData(header.Time)
	if err != nil {
		return common.Hash{}, err
	}
	var param = common.FSNCallParam{Func: common.MakeSwapFuncExt, Data: funcData}
	data, err := param.ToBytes()
	if err != nil {
		return common.Hash{}, err
	}
	var argsData = hexutil.Bytes(data)
	sendArgs := args.toSendArgs()
	sendArgs.To = &common.FSNCallAddress
	sendArgs.Data = &argsData
	return s.papi.SendTransaction(ctx, sendArgs, passwd)
}

// RecallSwap ss
func (s *PrivateFusionAPI) RecallSwap(ctx context.Context, args RecallSwapArgs, passwd string) (common.Hash, error) {
	state, _, err := s.b.StateAndHeaderByNumber(ctx, rpc.LatestBlockNumber)
	if state == nil || err != nil {
		return common.Hash{}, err
	}
	var swap common.Swap
	swap, err = state.GetSwap(args.SwapID)
	if err != nil {
		return common.Hash{}, err
	}
	if err := args.toParam().Check(common.BigMaxUint64, &swap); err != nil {
		return common.Hash{}, err
	}

	if swap.Owner != args.From {
		return common.Hash{}, fmt.Errorf("Must be swap owner can recall")
	}

	funcData, err := args.toData()
	if err != nil {
		return common.Hash{}, err
	}
	var param = common.FSNCallParam{Func: common.RecallSwapFunc, Data: funcData}
	data, err := param.ToBytes()
	if err != nil {
		return common.Hash{}, err
	}
	var argsData = hexutil.Bytes(data)
	sendArgs := args.toSendArgs()
	sendArgs.To = &common.FSNCallAddress
	sendArgs.Data = &argsData
	return s.papi.SendTransaction(ctx, sendArgs, passwd)
}

// TakeSwap ss
func (s *PrivateFusionAPI) TakeSwap(ctx context.Context, args TakeSwapArgs, passwd string) (common.Hash, error) {
	state, _, err := s.b.StateAndHeaderByNumber(ctx, rpc.LatestBlockNumber)
	if state == nil || err != nil {
		return common.Hash{}, err
	}

	swap, err := state.GetSwap(args.SwapID)
	if err != nil {
		return common.Hash{}, err
	}

	now := uint64(time.Now().Unix())
	if err := args.toParam().Check(common.BigMaxUint64, &swap, now); err != nil {
		return common.Hash{}, err
	}

	total := new(big.Int).Mul(swap.MinToAmount, args.Size)

	start := swap.ToStartTime
	end := swap.ToEndTime

	if start == common.TimeLockNow && end == common.TimeLockForever {
		if state.GetBalance(swap.ToAssetID, args.From).Cmp(total) < 0 {
			return common.Hash{}, fmt.Errorf("not enough from asset")
		}
	} else {
		needValue := common.NewTimeLock(&common.TimeLockItem{
			StartTime: start,
			EndTime:   end,
			Value:     total,
		})
		if err := needValue.IsValid(); err != nil {
			return common.Hash{}, fmt.Errorf("TakeSwap to err:%v", err.Error())
		}
		if state.GetTimeLockBalance(swap.ToAssetID, args.From).Cmp(needValue) < 0 {
			if state.GetBalance(swap.ToAssetID, args.From).Cmp(total) < 0 {
				return common.Hash{}, fmt.Errorf("not enough time lock or asset balance")
			}
		}
	}

	funcData, err := args.toData()
	if err != nil {
		return common.Hash{}, err
	}
	var param = common.FSNCallParam{Func: common.TakeSwapFuncExt, Data: funcData}
	data, err := param.ToBytes()
	if err != nil {
		return common.Hash{}, err
	}
	var argsData = hexutil.Bytes(data)
	sendArgs := args.toSendArgs()
	sendArgs.To = &common.FSNCallAddress
	sendArgs.Data = &argsData
	return s.papi.SendTransaction(ctx, sendArgs, passwd)
}

// FusionTransactionAPI ss
type FusionTransactionAPI struct {
	am        *accounts.Manager
	nonceLock *AddrLocker
	b         Backend
	txapi     *PublicTransactionPoolAPI
}

// NewFusionTransactionAPI ss
func NewFusionTransactionAPI(b Backend, nonceLock *AddrLocker, txapi *PublicTransactionPoolAPI) *FusionTransactionAPI {
	return &FusionTransactionAPI{
		am:        b.AccountManager(),
		nonceLock: nonceLock,
		b:         b,
		txapi:     txapi,
	}
}

func (s *FusionTransactionAPI) buildTransaction(ctx context.Context, args SendTxArgs) (*types.Transaction, error) {
	if args.Nonce == nil {
		s.nonceLock.LockAddr(args.From)
		defer s.nonceLock.UnlockAddr(args.From)
	}
	if err := args.setDefaults(ctx, s.b); err != nil {
		return nil, err
	}
	tx := args.toTransaction()
	return tx, nil
}

func (s *FusionTransactionAPI) sendTransaction(ctx context.Context, from common.Address, tx *types.Transaction) (common.Hash, error) {
	account := accounts.Account{Address: from}
	wallet, err := s.b.AccountManager().Find(account)
	if err != nil {
		return common.Hash{}, err
	}
	var chainID *big.Int
	if config := s.b.ChainConfig(); config.IsEIP155(s.b.CurrentBlock().Number()) {
		chainID = config.ChainID
	}
	signed, err := wallet.SignTx(account, tx, chainID)
	if err != nil {
		return common.Hash{}, err
	}
	return s.SendRawTransaction(ctx, signed)
}

// SendRawTransaction wacom
func (s *FusionTransactionAPI) SendRawTransaction(ctx context.Context, tx *types.Transaction) (common.Hash, error) {
	encodedTx, err := rlp.EncodeToBytes(tx)
	if err != nil {
		return common.Hash{}, err
	}
	return s.txapi.SendRawTransaction(ctx, encodedTx)
}

// BuildGenNotationTx ss
func (s *FusionTransactionAPI) BuildGenNotationTx(ctx context.Context, args FusionBaseArgs) (*types.Transaction, error) {
	state, _, err := s.b.StateAndHeaderByNumber(ctx, rpc.LatestBlockNumber)
	if state == nil || err != nil {
		return nil, err
	}
	notation := state.GetNotation(args.From)
	if notation != 0 {
		return nil, fmt.Errorf("An address can have only one notation, you already have a mapped notation:%d", state.CalcNotationDisplay(notation))
	}
	var param = common.FSNCallParam{Func: common.GenNotationFunc}
	data, err := param.ToBytes()
	if err != nil {
		return nil, err
	}
	var argsData = hexutil.Bytes(data)
	sendArgs := args.toSendArgs()
	sendArgs.To = &common.FSNCallAddress
	sendArgs.Data = &argsData
	return s.buildTransaction(ctx, sendArgs)
}

// GenNotation ss
func (s *FusionTransactionAPI) GenNotation(ctx context.Context, args FusionBaseArgs) (common.Hash, error) {

	tx, err := s.BuildGenNotationTx(ctx, args)
	if err != nil {
		return common.Hash{}, err
	}
	return s.sendTransaction(ctx, args.From, tx)
}

// BuildGenAssetTx ss
func (s *FusionTransactionAPI) BuildGenAssetTx(ctx context.Context, args GenAssetArgs) (*types.Transaction, error) {
	if err := args.toParam().Check(common.BigMaxUint64); err != nil {
		return nil, err
	}

	funcData, err := args.toData()
	if err != nil {
		return nil, err
	}

	var param = common.FSNCallParam{Func: common.GenAssetFunc, Data: funcData}
	data, err := param.ToBytes()
	if err != nil {
		return nil, err
	}
	var argsData = hexutil.Bytes(data)
	sendArgs := args.toSendArgs()
	sendArgs.To = &common.FSNCallAddress
	sendArgs.Data = &argsData
	return s.buildTransaction(ctx, sendArgs)
}

// GenAsset ss
func (s *FusionTransactionAPI) GenAsset(ctx context.Context, args GenAssetArgs) (common.Hash, error) {
	tx, err := s.BuildGenAssetTx(ctx, args)
	if err != nil {
		return common.Hash{}, err
	}
	return s.sendTransaction(ctx, args.From, tx)
}

// BuildSendAssetTx ss
func (s *FusionTransactionAPI) BuildSendAssetTx(ctx context.Context, args SendAssetArgs) (*types.Transaction, error) {
	state, _, err := s.b.StateAndHeaderByNumber(ctx, rpc.LatestBlockNumber)
	if state == nil || err != nil {
		return nil, err
	}
	if err := args.toParam().Check(common.BigMaxUint64); err != nil {
		return nil, err
	}

	if state.GetBalance(args.AssetID, args.From).Cmp(args.Value.ToInt()) < 0 {
		return nil, fmt.Errorf("not enough asset")
	}

	funcData, err := args.toData()
	if err != nil {
		return nil, err
	}
	var param = common.FSNCallParam{Func: common.SendAssetFunc, Data: funcData}
	data, err := param.ToBytes()
	if err != nil {
		return nil, err
	}
	var argsData = hexutil.Bytes(data)
	sendArgs := args.toSendArgs()
	sendArgs.To = &common.FSNCallAddress
	sendArgs.Data = &argsData
	sendArgs.Value = (*hexutil.Big)(big.NewInt(0))
	return s.buildTransaction(ctx, sendArgs)
}

// SendAsset ss
func (s *FusionTransactionAPI) SendAsset(ctx context.Context, args SendAssetArgs) (common.Hash, error) {
	tx, err := s.BuildSendAssetTx(ctx, args)
	if err != nil {
		return common.Hash{}, err
	}
	return s.sendTransaction(ctx, args.From, tx)
}

// BuildAssetToTimeLockTx ss
func (s *FusionTransactionAPI) BuildAssetToTimeLockTx(ctx context.Context, args TimeLockArgs) (*types.Transaction, error) {
	state, header, err := s.b.StateAndHeaderByNumber(ctx, rpc.LatestBlockNumber)
	if state == nil || err != nil {
		return nil, err
	}
	args.init()
	if err := args.toParam(common.AssetToTimeLock).Check(common.BigMaxUint64, header.Time.Uint64()); err != nil {
		return nil, err
	}
	needValue := common.NewTimeLock(&common.TimeLockItem{
		StartTime: common.MaxUint64(uint64(*args.StartTime), header.Time.Uint64()),
		EndTime:   uint64(*args.EndTime),
		Value:     args.Value.ToInt(),
	})
	if err := needValue.IsValid(); err != nil {
		return nil, fmt.Errorf("BuildAssetToTimeLockTx err:%v", err.Error())
	}
	if state.GetBalance(args.AssetID, args.From).Cmp(args.Value.ToInt()) < 0 {
		return nil, fmt.Errorf("not enough asset")
	}
	funcData, err := args.toData(common.AssetToTimeLock)
	if err != nil {
		return nil, err
	}
	var param = common.FSNCallParam{Func: common.TimeLockFunc, Data: funcData}
	data, err := param.ToBytes()
	if err != nil {
		return nil, err
	}
	var argsData = hexutil.Bytes(data)
	sendArgs := args.toSendArgs()
	sendArgs.To = &common.FSNCallAddress
	sendArgs.Data = &argsData
	return s.buildTransaction(ctx, sendArgs)
}

// AssetToTimeLock ss
func (s *FusionTransactionAPI) AssetToTimeLock(ctx context.Context, args TimeLockArgs) (common.Hash, error) {
	tx, err := s.BuildAssetToTimeLockTx(ctx, args)
	if err != nil {
		return common.Hash{}, err
	}
	return s.sendTransaction(ctx, args.From, tx)
}

// BuildTimeLockToTimeLockTx ss
func (s *FusionTransactionAPI) BuildTimeLockToTimeLockTx(ctx context.Context, args TimeLockArgs) (*types.Transaction, error) {
	state, header, err := s.b.StateAndHeaderByNumber(ctx, rpc.LatestBlockNumber)
	if state == nil || err != nil {
		return nil, err
	}
	args.init()
	if err := args.toParam(common.TimeLockToTimeLock).Check(common.BigMaxUint64, header.Time.Uint64()); err != nil {
		return nil, err
	}
	needValue := common.NewTimeLock(&common.TimeLockItem{
		StartTime: common.MaxUint64(uint64(*args.StartTime), header.Time.Uint64()),
		EndTime:   uint64(*args.EndTime),
		Value:     args.Value.ToInt(),
	})
	if err := needValue.IsValid(); err != nil {
		return nil, fmt.Errorf("BuildTimeLockToTimeLockTx err:%v", err.Error())
	}

	if state.GetTimeLockBalance(args.AssetID, args.From).Cmp(needValue) < 0 {
		return nil, fmt.Errorf("not enough time lock balance")
	}

	funcData, err := args.toData(common.TimeLockToTimeLock)
	if err != nil {
		return nil, err
	}
	var param = common.FSNCallParam{Func: common.TimeLockFunc, Data: funcData}
	data, err := param.ToBytes()
	if err != nil {
		return nil, err
	}
	var argsData = hexutil.Bytes(data)
	sendArgs := args.toSendArgs()
	sendArgs.To = &common.FSNCallAddress
	sendArgs.Data = &argsData
	return s.buildTransaction(ctx, sendArgs)
}

// TimeLockToTimeLock ss
func (s *FusionTransactionAPI) TimeLockToTimeLock(ctx context.Context, args TimeLockArgs) (common.Hash, error) {
	tx, err := s.BuildTimeLockToTimeLockTx(ctx, args)
	if err != nil {
		return common.Hash{}, err
	}
	return s.sendTransaction(ctx, args.From, tx)
}

// BuildTimeLockToAssetTx ss
func (s *FusionTransactionAPI) BuildTimeLockToAssetTx(ctx context.Context, args TimeLockArgs) (*types.Transaction, error) {
	state, header, err := s.b.StateAndHeaderByNumber(ctx, rpc.LatestBlockNumber)
	if state == nil || err != nil {
		return nil, err
	}
	args.init()
	*(*uint64)(args.StartTime) = header.Time.Uint64()
	*(*uint64)(args.EndTime) = common.TimeLockForever
	if err := args.toParam(common.TimeLockToAsset).Check(common.BigMaxUint64, header.Time.Uint64()); err != nil {
		return nil, err
	}
	needValue := common.NewTimeLock(&common.TimeLockItem{
		StartTime: uint64(*args.StartTime),
		EndTime:   uint64(*args.EndTime),
		Value:     args.Value.ToInt(),
	})
	if err := needValue.IsValid(); err != nil {
		return nil, fmt.Errorf("BuildTimeLockToAssetTx err:%v", err.Error())
	}
	if state.GetTimeLockBalance(args.AssetID, args.From).Cmp(needValue) < 0 {
		return nil, fmt.Errorf("not enough time lock balance")
	}
	funcData, err := args.toData(common.TimeLockToAsset)
	if err != nil {
		return nil, err
	}
	var param = common.FSNCallParam{Func: common.TimeLockFunc, Data: funcData}
	data, err := param.ToBytes()
	if err != nil {
		return nil, err
	}
	var argsData = hexutil.Bytes(data)
	sendArgs := args.toSendArgs()
	sendArgs.To = &common.FSNCallAddress
	sendArgs.Data = &argsData
	return s.buildTransaction(ctx, sendArgs)
}

// TimeLockToAsset ss
func (s *FusionTransactionAPI) TimeLockToAsset(ctx context.Context, args TimeLockArgs) (common.Hash, error) {
	tx, err := s.BuildTimeLockToAssetTx(ctx, args)
	if err != nil {
		return common.Hash{}, err
	}
	return s.sendTransaction(ctx, args.From, tx)
}

// BuildBuyTicketTx ss
func (s *FusionTransactionAPI) BuildBuyTicketTx(ctx context.Context, args BuyTicketArgs) (*types.Transaction, error) {
	state, header, err := s.b.StateAndHeaderByNumber(ctx, rpc.LatestBlockNumber)
	if state == nil || err != nil {
		return nil, err
	}

	if doesTicketPurchaseExistsForBlock(header.Number.Int64(), args.From) {
		return nil, fmt.Errorf("Purchase of BuyTicket for this block already submitted")
	}

	args.init(header.Time.Uint64())
	now := uint64(time.Now().Unix())
	if err := args.toParam().Check(common.BigMaxUint64, now, 600); err != nil {
		return nil, err
	}

	start := uint64(*args.Start)
	end := uint64(*args.End)
	value := common.TicketPrice(header.Number)
	needValue := common.NewTimeLock(&common.TimeLockItem{
		StartTime: common.MaxUint64(start, header.Time.Uint64()),
		EndTime:   end,
		Value:     value,
	})
	if err := needValue.IsValid(); err != nil {
		return nil, fmt.Errorf("BuildBuyTicketTx err:%v", err.Error())
	}

	if state.GetTimeLockBalance(common.SystemAssetID, args.From).Cmp(needValue) < 0 {
		if state.GetBalance(common.SystemAssetID, args.From).Cmp(value) < 0 {
			return nil, fmt.Errorf("not enough time lock or asset balance")
		}
	}
	data, err := args.toData()
	if err != nil {
		return nil, err
	}
	var param = common.FSNCallParam{Func: common.BuyTicketFunc, Data: data}
	data, err = param.ToBytes()
	if err != nil {
		return nil, err
	}
	var argsData = hexutil.Bytes(data)
	sendArgs := args.toSendArgs()
	sendArgs.To = &common.FSNCallAddress
	sendArgs.Data = &argsData
	return s.buildTransaction(ctx, sendArgs)
}

// BuyTicket ss
func (s *FusionTransactionAPI) BuyTicket(ctx context.Context, args BuyTicketArgs) (common.Hash, error) {
	tx, err := s.BuildBuyTicketTx(ctx, args)
	if err != nil {
		return common.Hash{}, err
	}
	hash, err := s.sendTransaction(ctx, args.From, tx)
	if err != nil {
		return common.Hash{}, err
	}
	addTicketPurchaseForBlock(args.From)
	return hash, err
}

func (s *FusionTransactionAPI) buildAssetValueChangeTx(ctx context.Context, args AssetValueChangeExArgs) (*types.Transaction, error) {
	state, _, err := s.b.StateAndHeaderByNumber(ctx, rpc.LatestBlockNumber)
	if state == nil || err != nil {
		return nil, err
	}

	if err := args.toParam().Check(common.BigMaxUint64); err != nil {
		return nil, err
	}

	asset, assetError := state.GetAsset(args.AssetID)
	if assetError != nil {
		return nil, fmt.Errorf("asset not found")
	}

	if !asset.CanChange {
		return nil, fmt.Errorf("asset can't inc or dec")
	}

	if asset.Owner != args.From {
		return nil, fmt.Errorf("must be change by onwer")
	}

	currentBalance := state.GetBalance(args.AssetID, args.To)
	val := args.Value.ToInt()
	if !args.IsInc {
		if currentBalance.Cmp(val) < 0 {
			return nil, fmt.Errorf("not enough asset")
		}
	}

	funcData, err := args.toData()
	if err != nil {
		return nil, err
	}
	var param = common.FSNCallParam{Func: common.AssetValueChangeFunc, Data: funcData}
	data, err := param.ToBytes()
	if err != nil {
		return nil, err
	}
	var argsData = hexutil.Bytes(data)
	sendArgs := args.toSendArgs()
	sendArgs.To = &common.FSNCallAddress
	sendArgs.Data = &argsData
	return s.buildTransaction(ctx, sendArgs)
}

// BuildIncAssetTx ss
func (s *FusionTransactionAPI) BuildIncAssetTx(ctx context.Context, args AssetValueChangeExArgs) (*types.Transaction, error) {
	args.IsInc = true
	return s.buildAssetValueChangeTx(ctx, args)
}

// IncAsset ss
func (s *FusionTransactionAPI) IncAsset(ctx context.Context, args AssetValueChangeExArgs) (common.Hash, error) {
	tx, err := s.BuildIncAssetTx(ctx, args)
	if err != nil {
		return common.Hash{}, err
	}
	return s.sendTransaction(ctx, args.From, tx)
}

// BuildDecAssetTx ss
func (s *FusionTransactionAPI) BuildDecAssetTx(ctx context.Context, args AssetValueChangeExArgs) (*types.Transaction, error) {
	args.IsInc = false
	return s.buildAssetValueChangeTx(ctx, args)
}

// DecAsset ss
func (s *FusionTransactionAPI) DecAsset(ctx context.Context, args AssetValueChangeExArgs) (common.Hash, error) {
	tx, err := s.BuildDecAssetTx(ctx, args)
	if err != nil {
		return common.Hash{}, err
	}
	return s.sendTransaction(ctx, args.From, tx)
}

// BuildMakeSwapTx ss
func (s *FusionTransactionAPI) BuildMakeSwapTx(ctx context.Context, args MakeSwapArgs) (*types.Transaction, error) {
	state, header, err := s.b.StateAndHeaderByNumber(ctx, rpc.LatestBlockNumber)
	if state == nil || err != nil {
		return nil, err
	}

	args.init()
	now := uint64(time.Now().Unix())
	if err := args.toParam(header.Time).Check(common.BigMaxUint64, now); err != nil {
		return nil, err
	}

	total := new(big.Int).Mul(args.MinFromAmount.ToInt(), args.SwapSize)
	start := uint64(*args.FromStartTime)
	end := uint64(*args.FromEndTime)

	if args.FromAssetID == common.OwnerUSANAssetID {
		notation := state.GetNotation(args.From)
		if notation == 0 {
			return nil, fmt.Errorf("from address does not have a notation")
		}
	} else if start == common.TimeLockNow && end == common.TimeLockForever {
		if state.GetBalance(args.FromAssetID, args.From).Cmp(total) < 0 {
			return nil, fmt.Errorf("not enough from asset")
		}
	} else {
		needValue := common.NewTimeLock(&common.TimeLockItem{
			StartTime: common.MaxUint64(start, header.Time.Uint64()),
			EndTime:   end,
			Value:     total,
		})
		if err := needValue.IsValid(); err != nil {
			return nil, fmt.Errorf("BuildMakeSwapTx from err:%v", err.Error())
		}
		if state.GetTimeLockBalance(args.FromAssetID, args.From).Cmp(needValue) < 0 {
			if state.GetBalance(args.FromAssetID, args.From).Cmp(total) < 0 {
				return nil, fmt.Errorf("not enough time lock or asset balance")
			}
		}
	}

	funcData, err := args.toData(header.Time)
	if err != nil {
		return nil, err
	}
	var param = common.FSNCallParam{Func: common.MakeSwapFuncExt, Data: funcData}
	data, err := param.ToBytes()
	if err != nil {
		return nil, err
	}
	var argsData = hexutil.Bytes(data)
	sendArgs := args.toSendArgs()
	sendArgs.To = &common.FSNCallAddress
	sendArgs.Data = &argsData
	return s.buildTransaction(ctx, sendArgs)
}

// MakeSwap ss
func (s *FusionTransactionAPI) MakeSwap(ctx context.Context, args MakeSwapArgs) (common.Hash, error) {
	tx, err := s.BuildMakeSwapTx(ctx, args)
	if err != nil {
		return common.Hash{}, err
	}
	return s.sendTransaction(ctx, args.From, tx)
}

// BuildRecallSwapTx ss
func (s *FusionTransactionAPI) BuildRecallSwapTx(ctx context.Context, args RecallSwapArgs) (*types.Transaction, error) {
	state, _, err := s.b.StateAndHeaderByNumber(ctx, rpc.LatestBlockNumber)
	if state == nil || err != nil {
		return nil, err
	}

	var swap common.Swap
	swap, err = state.GetSwap(args.SwapID)
	if err != nil {
		return nil, err
	}

	if err := args.toParam().Check(common.BigMaxUint64, &swap); err != nil {
		return nil, err
	}

	if swap.Owner != args.From {
		return nil, fmt.Errorf("Must be swap onwer can recall")
	}

	funcData, err := args.toData()
	if err != nil {
		return nil, err
	}
	var param = common.FSNCallParam{Func: common.RecallSwapFunc, Data: funcData}
	data, err := param.ToBytes()
	if err != nil {
		return nil, err
	}
	var argsData = hexutil.Bytes(data)
	sendArgs := args.toSendArgs()
	sendArgs.To = &common.FSNCallAddress
	sendArgs.Data = &argsData
	return s.buildTransaction(ctx, sendArgs)
}

// RecallSwap ss
func (s *FusionTransactionAPI) RecallSwap(ctx context.Context, args RecallSwapArgs) (common.Hash, error) {
	tx, err := s.BuildRecallSwapTx(ctx, args)
	if err != nil {
		return common.Hash{}, err
	}
	return s.sendTransaction(ctx, args.From, tx)
}

// BuildTakeSwapTx ss
func (s *FusionTransactionAPI) BuildTakeSwapTx(ctx context.Context, args TakeSwapArgs) (*types.Transaction, error) {
	state, _, err := s.b.StateAndHeaderByNumber(ctx, rpc.LatestBlockNumber)
	if state == nil || err != nil {
		return nil, err
	}
	var swap common.Swap
	swap, err = state.GetSwap(args.SwapID)
	if err != nil {
		return nil, err
	}

	now := uint64(time.Now().Unix())
	if err := args.toParam().Check(common.BigMaxUint64, &swap, now); err != nil {
		return nil, err
	}

	total := new(big.Int).Mul(swap.MinToAmount, args.Size)
	start := swap.ToStartTime
	end := swap.ToEndTime

	if start == common.TimeLockNow && end == common.TimeLockForever {
		if state.GetBalance(swap.ToAssetID, args.From).Cmp(total) < 0 {
			return nil, fmt.Errorf("not enough from asset")
		}
	} else {
		needValue := common.NewTimeLock(&common.TimeLockItem{
			StartTime: start,
			EndTime:   end,
			Value:     total,
		})
		if err := needValue.IsValid(); err != nil {
			return nil, fmt.Errorf("BuildTakeSwapTx to err:%v", err.Error())
		}
		if state.GetTimeLockBalance(swap.ToAssetID, args.From).Cmp(needValue) < 0 {
			if state.GetBalance(swap.ToAssetID, args.From).Cmp(total) < 0 {
				return nil, fmt.Errorf("not enough time lock or asset balance")
			}
		}
	}
	funcData, err := args.toData()
	if err != nil {
		return nil, err
	}
	var param = common.FSNCallParam{Func: common.TakeSwapFuncExt, Data: funcData}
	data, err := param.ToBytes()
	if err != nil {
		return nil, err
	}
	var argsData = hexutil.Bytes(data)
	sendArgs := args.toSendArgs()
	sendArgs.To = &common.FSNCallAddress
	sendArgs.Data = &argsData
	return s.buildTransaction(ctx, sendArgs)
}

// TakeSwap ss
func (s *FusionTransactionAPI) TakeSwap(ctx context.Context, args TakeSwapArgs) (common.Hash, error) {
	tx, err := s.BuildTakeSwapTx(ctx, args)
	if err != nil {
		return common.Hash{}, err
	}
	return s.sendTransaction(ctx, args.From, tx)
}

// BuildMakeMultiSwapTx ss
func (s *FusionTransactionAPI) BuildMakeMultiSwapTx(ctx context.Context, args MakeMultiSwapArgs) (*types.Transaction, error) {
	state, header, err := s.b.StateAndHeaderByNumber(ctx, rpc.LatestBlockNumber)
	if state == nil || err != nil {
		return nil, err
	}

	args.init()
	now := uint64(time.Now().Unix())
	if err := args.toParam(header.Time).Check(common.BigMaxUint64, now); err != nil {
		return nil, err
	}

	ln := len(args.MinFromAmount)
	for i := 0; i < ln; i++ {
		total := new(big.Int).Mul(args.MinFromAmount[i].ToInt(), args.SwapSize)
		start := uint64(*args.FromStartTime[i])
		end := uint64(*args.FromEndTime[i])

		if args.FromAssetID[i] == common.OwnerUSANAssetID {
			return nil, fmt.Errorf("USANs cannot be multi-swapped")
		} else if start == common.TimeLockNow && end == common.TimeLockForever {
			if state.GetBalance(args.FromAssetID[i], args.From).Cmp(total) < 0 {
				return nil, fmt.Errorf("not enough from asset")
			}
		} else {
			needValue := common.NewTimeLock(&common.TimeLockItem{
				StartTime: common.MaxUint64(start, header.Time.Uint64()),
				EndTime:   end,
				Value:     total,
			})
			if err := needValue.IsValid(); err != nil {
				return nil, fmt.Errorf("BuildMakeSwapTx from err:%v", err.Error())
			}
			if state.GetTimeLockBalance(args.FromAssetID[i], args.From).Cmp(needValue) < 0 {
				if state.GetBalance(args.FromAssetID[i], args.From).Cmp(total) < 0 {
					return nil, fmt.Errorf("not enough time lock or asset balance")
				}
			}
		}
	}

	funcData, err := args.toData(header.Time)
	if err != nil {
		return nil, err
	}
	var param = common.FSNCallParam{Func: common.MakeMultiSwapFunc, Data: funcData}
	data, err := param.ToBytes()
	if err != nil {
		return nil, err
	}
	var argsData = hexutil.Bytes(data)
	sendArgs := args.toSendArgs()
	sendArgs.To = &common.FSNCallAddress
	sendArgs.Data = &argsData
	return s.buildTransaction(ctx, sendArgs)
}

// BuildRecallMultiSwapTx ss
func (s *FusionTransactionAPI) BuildMultiRecallSwapTx(ctx context.Context, args RecallMultiSwapArgs) (*types.Transaction, error) {
	state, _, err := s.b.StateAndHeaderByNumber(ctx, rpc.LatestBlockNumber)
	if state == nil || err != nil {
		return nil, err
	}

	var swap common.MultiSwap
	swap, err = state.GetMultiSwap(args.SwapID)
	if err != nil {
		return nil, err
	}

	if err := args.toParam().Check(common.BigMaxUint64, &swap); err != nil {
		return nil, err
	}

	if swap.Owner != args.From {
		return nil, fmt.Errorf("Must be swap onwer can recall")
	}

	funcData, err := args.toData()
	if err != nil {
		return nil, err
	}
	var param = common.FSNCallParam{Func: common.RecallMultiSwapFunc, Data: funcData}
	data, err := param.ToBytes()
	if err != nil {
		return nil, err
	}
	var argsData = hexutil.Bytes(data)
	sendArgs := args.toSendArgs()
	sendArgs.To = &common.FSNCallAddress
	sendArgs.Data = &argsData
	return s.buildTransaction(ctx, sendArgs)
}

// BuildTakeSwapTx ss
func (s *FusionTransactionAPI) BuildTakeMultiSwapTx(ctx context.Context, args TakeMultiSwapArgs) (*types.Transaction, error) {
	state, _, err := s.b.StateAndHeaderByNumber(ctx, rpc.LatestBlockNumber)
	if state == nil || err != nil {
		return nil, err
	}
	var swap common.MultiSwap
	swap, err = state.GetMultiSwap(args.SwapID)
	if err != nil {
		return nil, err
	}

	now := uint64(time.Now().Unix())
	if err := args.toParam().Check(common.BigMaxUint64, &swap, now); err != nil {
		return nil, err
	}

	ln := len(swap.MinToAmount)
	for i := 0; i < ln; i++ {
		total := new(big.Int).Mul(swap.MinToAmount[i], args.Size)
		start := swap.ToStartTime[i]
		end := swap.ToEndTime[i]

		if start == common.TimeLockNow && end == common.TimeLockForever {
			if state.GetBalance(swap.ToAssetID[i], args.From).Cmp(total) < 0 {
				return nil, fmt.Errorf("not enough from asset")
			}
		} else {
			needValue := common.NewTimeLock(&common.TimeLockItem{
				StartTime: start,
				EndTime:   end,
				Value:     total,
			})
			if err := needValue.IsValid(); err != nil {
				return nil, fmt.Errorf("BuildTakeSwapTx to err:%v", err.Error())
			}
			if state.GetTimeLockBalance(swap.ToAssetID[i], args.From).Cmp(needValue) < 0 {
				if state.GetBalance(swap.ToAssetID[i], args.From).Cmp(total) < 0 {
					return nil, fmt.Errorf("not enough time lock or asset balance")
				}
			}
		}
	}
	funcData, err := args.toData()
	if err != nil {
		return nil, err
	}
	var param = common.FSNCallParam{Func: common.TakeMultiSwapFunc, Data: funcData}
	data, err := param.ToBytes()
	if err != nil {
		return nil, err
	}
	var argsData = hexutil.Bytes(data)
	sendArgs := args.toSendArgs()
	sendArgs.To = &common.FSNCallAddress
	sendArgs.Data = &argsData
	return s.buildTransaction(ctx, sendArgs)
}
