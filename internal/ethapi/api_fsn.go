package ethapi

import (
	"context"
	"fmt"
	"math/big"

	"github.com/FusionFoundation/efsn/accounts"
	"github.com/FusionFoundation/efsn/common"
	"github.com/FusionFoundation/efsn/common/hexutil"
	"github.com/FusionFoundation/efsn/core/types"
	"github.com/FusionFoundation/efsn/rlp"
	"github.com/FusionFoundation/efsn/rpc"
)

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
	Name      string       `json:"name"`
	Symbol    string       `json:"symbol"`
	Decimals  uint8        `json:"decimals"`
	Total     *hexutil.Big `json:"total"`
	CanChange bool         `json:"canChange"`
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

func (args *FusionBaseArgs) toSendArgs() SendTxArgs {
	return SendTxArgs{
		From:     args.From,
		Gas:      args.Gas,
		GasPrice: args.GasPrice,
		Nonce:    args.Nonce,
	}
}

func (args *SendAssetArgs) toData() ([]byte, error) {
	param := common.SendAssetParam{
		AssetID: args.AssetID,
		To:      args.To,
		Value:   args.Value.ToInt(),
	}
	return param.ToBytes()
}

func (args *TimeLockArgs) toData(typ common.TimeLockType) ([]byte, error) {
	param := common.TimeLockParam{
		Type:      typ,
		AssetID:   args.AssetID,
		To:        args.To,
		StartTime: uint64(*args.StartTime),
		EndTime:   uint64(*args.EndTime),
		Value:     args.Value.ToInt(),
	}
	return param.ToBytes()
}

func (args *GenAssetArgs) toData() ([]byte, error) {
	param := common.GenAssetParam{
		Name:      args.Name,
		Symbol:    args.Symbol,
		Decimals:  args.Decimals,
		Total:     args.Total.ToInt(),
		CanChange: args.CanChange,
	}
	return param.ToBytes()
}

func (args *BuyTicketArgs) toData() ([]byte, error) {
	param := common.BuyTicketParam{
		Start: uint64(*args.Start),
		End:   uint64(*args.End),
	}
	return param.ToBytes()
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

func (args *MakeSwapArgs) toData() ([]byte, error) {
	param := common.MakeSwapParam{
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
	}
	return param.ToBytes()
}

func (args *RecallSwapArgs) toData() ([]byte, error) {
	param := common.RecallSwapParam{
		SwapID: args.SwapID,
	}
	return param.ToBytes()
}

func (args *TakeSwapArgs) toData() ([]byte, error) {
	param := common.TakeSwapParam{
		SwapID: args.SwapID,
		Size:   args.Size,
	}
	return param.ToBytes()
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
	return b, state.Error()
}

// GetAllTimeLockBalances wacom
func (s *PublicFusionAPI) GetAllTimeLockBalances(ctx context.Context, address common.Address, blockNr rpc.BlockNumber) (map[common.Hash]*common.TimeLock, error) {
	state, _, err := s.b.StateAndHeaderByNumber(ctx, blockNr)
	if state == nil || err != nil {
		return make(map[common.Hash]*common.TimeLock), err
	}
	b := state.GetAllTimeLockBalances(address)
	return b, state.Error()
}

// GetNotation wacom
func (s *PublicFusionAPI) GetNotation(ctx context.Context, address common.Address, blockNr rpc.BlockNumber) (uint64, error) {
	state, _, err := s.b.StateAndHeaderByNumber(ctx, blockNr)
	if state == nil || err != nil {
		return 0, err
	}
	b := calcNotationDisplay(state.GetNotation(address))
	return b, state.Error()
}

// GetAddressByNotation wacom
func (s *PublicFusionAPI) GetAddressByNotation(ctx context.Context, notation uint64, blockNr rpc.BlockNumber) (common.Address, error) {
	state, _, err := s.b.StateAndHeaderByNumber(ctx, blockNr)
	if state == nil || err != nil {
		return common.Address{}, err
	}
	temp := notation / 100
	notations := state.AllNotation()
	if temp <= 0 || temp > uint64(len(notations)) {
		return common.Address{}, fmt.Errorf("Notation Not Found")
	}
	if calcNotationDisplay(temp) != notation {
		return common.Address{}, fmt.Errorf("Notation Check Error")
	}
	return notations[int(temp-1)], state.Error()
}

// AllNotation wacom
func (s *PublicFusionAPI) AllNotation(ctx context.Context, blockNr rpc.BlockNumber) (map[common.Address]uint64, error) {
	state, _, err := s.b.StateAndHeaderByNumber(ctx, blockNr)
	if state == nil || err != nil {
		return nil, err
	}
	notations := state.AllNotation()
	b := make(map[common.Address]uint64, len(notations))
	for i := 0; i < len(notations); i++ {
		b[notations[i]] = calcNotationDisplay(uint64(i + 1))
	}
	return b, state.Error()
}

// GetAsset wacom
func (s *PublicFusionAPI) GetAsset(ctx context.Context, assetID common.Hash, blockNr rpc.BlockNumber) (*common.Asset, error) {
	state, _, err := s.b.StateAndHeaderByNumber(ctx, blockNr)
	if state == nil || err != nil {
		return nil, err
	}
	assets := state.AllAssets()
	asset, ok := assets[assetID]

	if !ok {
		return nil, fmt.Errorf("Asset not found")
	}
	return &asset, nil
}

// AllAssets wacom
func (s *PublicFusionAPI) AllAssets(ctx context.Context, blockNr rpc.BlockNumber) (map[common.Hash]common.Asset, error) {
	state, _, err := s.b.StateAndHeaderByNumber(ctx, blockNr)
	if state == nil || err != nil {
		return nil, err
	}
	assets := state.AllAssets()
	return assets, state.Error()
}

// AllTickets wacom
func (s *PublicFusionAPI) AllTickets(ctx context.Context, blockNr rpc.BlockNumber) (map[common.Hash]common.Ticket, error) {
	state, _, err := s.b.StateAndHeaderByNumber(ctx, blockNr)
	if state == nil || err != nil {
		return nil, err
	}
	tickets := state.AllTickets()
	return tickets, state.Error()
}

// TotalNumberOfTickets wacom
func (s *PublicFusionAPI) TotalNumberOfTickets(ctx context.Context, blockNr rpc.BlockNumber) (int, error) {
	tickets, err := s.AllTickets(ctx, blockNr)
	return len(tickets), err
}

// TotalNumberOfTicketsByAddress wacom
func (s *PublicFusionAPI) TotalNumberOfTicketsByAddress(ctx context.Context, address common.Address, blockNr rpc.BlockNumber) (int, error) {
	tickets, err := s.AllTicketsByAddress(ctx, address, blockNr)
	return len(tickets), err
}

// TicketPrice wacom
func (s *PublicFusionAPI) TicketPrice(ctx context.Context) (string, error) {
	return common.TicketPrice().String(), nil
}

// AllTicketsByAddress wacom
func (s *PublicFusionAPI) AllTicketsByAddress(ctx context.Context, address common.Address, blockNr rpc.BlockNumber) (map[common.Hash]common.Ticket, error) {
	state, _, err := s.b.StateAndHeaderByNumber(ctx, blockNr)
	var ret = make(map[common.Hash]common.Ticket)
	if state == nil || err != nil {
		return nil, err
	}
	tickets := state.AllTickets()
	for k, v := range tickets {
		if v.Owner == address {
			ret[k] = v
		}
	}
	return ret, state.Error()
}

// AllSwaps wacom
func (s *PublicFusionAPI) AllSwaps(ctx context.Context, blockNr rpc.BlockNumber) (map[common.Hash]common.Swap, error) {
	state, _, err := s.b.StateAndHeaderByNumber(ctx, blockNr)
	if state == nil || err != nil {
		return nil, err
	}
	swaps := state.AllSwaps()
	return swaps, state.Error()
}

// AllSwapsByAddress wacom
func (s *PublicFusionAPI) AllSwapsByAddress(ctx context.Context, address common.Address, blockNr rpc.BlockNumber) (map[common.Hash]common.Swap, error) {
	state, _, err := s.b.StateAndHeaderByNumber(ctx, blockNr)
	if state == nil || err != nil {
		return nil, err
	}
	swaps := state.AllSwaps()
	var ret = make(map[common.Hash]common.Swap)
	for k, v := range swaps {
		if v.Owner == address {
			ret[k] = v
		}
	}
	return ret, state.Error()
}

// PrivateFusionAPI ss
type PrivateFusionAPI struct {
	am        *accounts.Manager
	nonceLock *AddrLocker
	b         Backend
	papi      *PrivateAccountAPI
}

// NewPrivateFusionAPI ss
func NewPrivateFusionAPI(b Backend, nonceLock *AddrLocker, papi *PrivateAccountAPI) *PrivateFusionAPI {
	return &PrivateFusionAPI{
		am:        b.AccountManager(),
		nonceLock: nonceLock,
		b:         b,
		papi:      papi,
	}
}

// GenNotation ss
func (s *PrivateFusionAPI) GenNotation(ctx context.Context, args FusionBaseArgs, passwd string) (common.Hash, error) {

	state, _, err := s.b.StateAndHeaderByNumber(ctx, rpc.LatestBlockNumber)
	if state == nil || err != nil {
		return common.Hash{}, err
	}

	notation := state.GetNotation(args.From)

	if notation != 0 {
		return common.Hash{}, fmt.Errorf("An address can have only one notation, you already have a mapped notation:%d", calcNotationDisplay(notation))
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

	args.init()

	state, _, err := s.b.StateAndHeaderByNumber(ctx, rpc.LatestBlockNumber)
	if state == nil || err != nil {
		return common.Hash{}, err
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
	args.init()

	state, _, err := s.b.StateAndHeaderByNumber(ctx, rpc.LatestBlockNumber)
	if state == nil || err != nil {
		return common.Hash{}, err
	}

	needValue := common.NewTimeLock(&common.TimeLockItem{
		StartTime: uint64(*args.StartTime),
		EndTime:   uint64(*args.EndTime),
		Value:     args.Value.ToInt(),
	})

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
	state, _, err := s.b.StateAndHeaderByNumber(ctx, rpc.LatestBlockNumber)
	if state == nil || err != nil {
		return common.Hash{}, err
	}
	block, err := s.b.BlockByNumber(ctx, rpc.LatestBlockNumber)
	if block == nil || err != nil {
		return common.Hash{}, err
	}
	args.init()
	*(*uint64)(args.StartTime) = block.Time().Uint64()
	*(*uint64)(args.EndTime) = common.TimeLockForever
	needValue := common.NewTimeLock(&common.TimeLockItem{
		StartTime: uint64(*args.StartTime),
		EndTime:   uint64(*args.EndTime),
		Value:     args.Value.ToInt(),
	})
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

// BuyTicket ss
func (s *PrivateFusionAPI) BuyTicket(ctx context.Context, args BuyTicketArgs, passwd string) (common.Hash, error) {
	state, _, err := s.b.StateAndHeaderByNumber(ctx, rpc.LatestBlockNumber)
	if state == nil || err != nil {
		return common.Hash{}, err
	}

	block, err := s.b.BlockByNumber(ctx, rpc.LatestBlockNumber)
	if block == nil || err != nil {
		return common.Hash{}, err
	}

	if args.Start == nil {
		args.Start = new(hexutil.Uint64)
		*(*uint64)(args.Start) = block.Time().Uint64()
	}

	if args.End == nil {
		args.End = new(hexutil.Uint64)
		*(*uint64)(args.End) = uint64(*args.Start) + 30*24*3600
	}

	start := uint64(*args.Start)
	end := uint64(*args.End)
	value := common.TicketPrice()
	needValue := common.NewTimeLock(&common.TimeLockItem{
		StartTime: start,
		EndTime:   end,
		Value:     value,
	})
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
	return s.papi.SendTransaction(ctx, sendArgs, passwd)
}

// IncAsset ss
func (s *PrivateFusionAPI) IncAsset(ctx context.Context, args AssetValueChangeArgs, passwd string) (common.Hash, error) {
	args.IsInc = true
	return s.checkAssetValueChange(ctx, args, passwd)
}

// DecAsset ss
func (s *PrivateFusionAPI) DecAsset(ctx context.Context, args AssetValueChangeArgs, passwd string) (common.Hash, error) {
	args.IsInc = false
	return s.checkAssetValueChange(ctx, args, passwd)
}

func (s *PrivateFusionAPI) checkAssetValueChange(ctx context.Context, args AssetValueChangeArgs, passwd string) (common.Hash, error) {

	big0 := big.NewInt(0)

	if args.Value.ToInt().Cmp(big0) <= 0 {
		return common.Hash{}, fmt.Errorf("illegal operation")
	}

	state, _, err := s.b.StateAndHeaderByNumber(ctx, rpc.LatestBlockNumber)
	if state == nil || err != nil {
		return common.Hash{}, err
	}

	assets := state.AllAssets()

	asset, ok := assets[args.AssetID]

	if !ok {
		return common.Hash{}, fmt.Errorf("asset not found")
	}

	if !asset.CanChange {
		return common.Hash{}, fmt.Errorf("asset can't inc or dec")
	}

	if asset.Owner != args.From {
		return common.Hash{}, fmt.Errorf("must be change by onwer")
	}

	if !args.IsInc {
		if state.GetBalance(args.AssetID, args.To).Cmp(args.Value.ToInt()) < 0 {
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

	args.init()

	big0 := big.NewInt(0)
	if args.MinFromAmount.ToInt().Cmp(big0) <= 0 || args.MinToAmount.ToInt().Cmp(big0) <= 0 || args.SwapSize.Cmp(big0) <= 0 {
		return common.Hash{}, fmt.Errorf("MinFromAmount,MinToAmount and SwapSize must be ge 1")
	}

	state, _, err := s.b.StateAndHeaderByNumber(ctx, rpc.LatestBlockNumber)
	if state == nil || err != nil {
		return common.Hash{}, err
	}

	total := new(big.Int).Mul(args.MinFromAmount.ToInt(), args.SwapSize)

	start := uint64(*args.FromStartTime)
	end := uint64(*args.FromEndTime)

	if start == common.TimeLockNow && end == common.TimeLockForever {
		if state.GetBalance(args.FromAssetID, args.From).Cmp(total) < 0 {
			return common.Hash{}, fmt.Errorf("not enough from asset")
		}
	} else {
		needValue := common.NewTimeLock(&common.TimeLockItem{
			StartTime: start,
			EndTime:   end,
			Value:     total,
		})
		if state.GetTimeLockBalance(args.FromAssetID, args.From).Cmp(needValue) < 0 {
			return common.Hash{}, fmt.Errorf("not enough time lock balance")
		}
	}

	funcData, err := args.toData()
	if err != nil {
		return common.Hash{}, err
	}
	var param = common.FSNCallParam{Func: common.MakeSwapFunc, Data: funcData}
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
	swaps := state.AllSwaps()
	swap, ok := swaps[args.SwapID]
	if !ok {
		return common.Hash{}, fmt.Errorf("Swap not found")
	}

	if swap.Owner != args.From {
		return common.Hash{}, fmt.Errorf("Must be swap onwer can recall")
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
	swaps := state.AllSwaps()
	swap, ok := swaps[args.SwapID]
	if !ok {
		return common.Hash{}, fmt.Errorf("Swap not found")
	}
	big0 := big.NewInt(0)
	if swap.SwapSize.Cmp(args.Size) < 0 || args.Size.Cmp(big0) <= 0 {
		return common.Hash{}, fmt.Errorf("SwapSize must le and Size must be ge 1")
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
		if state.GetTimeLockBalance(swap.ToAssetID, args.From).Cmp(needValue) < 0 {
			return common.Hash{}, fmt.Errorf("not enough time lock balance")
		}
	}

	funcData, err := args.toData()
	if err != nil {
		return common.Hash{}, err
	}
	var param = common.FSNCallParam{Func: common.TakeSwapFunc, Data: funcData}
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

func calcNotationDisplay(notation uint64) uint64 {
	if notation == 0 {
		return notation
	}
	check := (notation ^ 8192 ^ 13 + 73/76798669*708583737978) % 100
	return (notation*100 + check)
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
		return nil, fmt.Errorf("An address can have only one notation, you already have a mapped notation:%d", calcNotationDisplay(notation))
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

	args.init()

	state, _, err := s.b.StateAndHeaderByNumber(ctx, rpc.LatestBlockNumber)
	if state == nil || err != nil {
		return nil, err
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
	args.init()

	state, _, err := s.b.StateAndHeaderByNumber(ctx, rpc.LatestBlockNumber)
	if state == nil || err != nil {
		return nil, err
	}

	needValue := common.NewTimeLock(&common.TimeLockItem{
		StartTime: uint64(*args.StartTime),
		EndTime:   uint64(*args.EndTime),
		Value:     args.Value.ToInt(),
	})

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
	state, _, err := s.b.StateAndHeaderByNumber(ctx, rpc.LatestBlockNumber)
	if state == nil || err != nil {
		return nil, err
	}
	block, err := s.b.BlockByNumber(ctx, rpc.LatestBlockNumber)
	if block == nil || err != nil {
		return nil, err
	}
	args.init()
	*(*uint64)(args.StartTime) = block.Time().Uint64()
	*(*uint64)(args.EndTime) = common.TimeLockForever
	needValue := common.NewTimeLock(&common.TimeLockItem{
		StartTime: uint64(*args.StartTime),
		EndTime:   uint64(*args.EndTime),
		Value:     args.Value.ToInt(),
	})
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
	state, _, err := s.b.StateAndHeaderByNumber(ctx, rpc.LatestBlockNumber)
	if state == nil || err != nil {
		return nil, err
	}

	block, err := s.b.BlockByNumber(ctx, rpc.LatestBlockNumber)
	if block == nil || err != nil {
		return nil, err
	}

	if args.Start == nil {
		args.Start = new(hexutil.Uint64)
		*(*uint64)(args.Start) = block.Time().Uint64()
	}

	if args.End == nil {
		args.End = new(hexutil.Uint64)
		*(*uint64)(args.End) = uint64(*args.Start) + 30*24*3600
	}

	start := uint64(*args.Start)
	end := uint64(*args.End)

	value := common.TicketPrice()
	needValue := common.NewTimeLock(&common.TimeLockItem{
		StartTime: start,
		EndTime:   end,
		Value:     value,
	})

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
	return s.sendTransaction(ctx, args.From, tx)
}

func (s *FusionTransactionAPI) buildAssetValueChangeTx(ctx context.Context, args AssetValueChangeArgs) (*types.Transaction, error) {

	big0 := big.NewInt(0)

	if (args.IsInc && args.Value.ToInt().Cmp(big0) <= 0) || (!args.IsInc && args.Value.ToInt().Cmp(big0) >= 0) {
		return nil, fmt.Errorf("illegal operation")
	}

	state, _, err := s.b.StateAndHeaderByNumber(ctx, rpc.LatestBlockNumber)
	if state == nil || err != nil {
		return nil, err
	}

	assets := state.AllAssets()

	asset, ok := assets[args.AssetID]

	if !ok {
		return nil, fmt.Errorf("asset not found")
	}

	if !asset.CanChange {
		return nil, fmt.Errorf("asset can't inc or dec")
	}

	if asset.Owner != args.From {
		return nil, fmt.Errorf("must be change by onwer")
	}

	if !args.IsInc {
		if state.GetBalance(args.AssetID, args.To).Cmp(args.Value.ToInt()) < 0 {
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
func (s *FusionTransactionAPI) BuildIncAssetTx(ctx context.Context, args AssetValueChangeArgs) (*types.Transaction, error) {
	args.IsInc = true
	return s.buildAssetValueChangeTx(ctx, args)
}

// IncAsset ss
func (s *FusionTransactionAPI) IncAsset(ctx context.Context, args AssetValueChangeArgs) (common.Hash, error) {
	tx, err := s.BuildIncAssetTx(ctx, args)
	if err != nil {
		return common.Hash{}, err
	}
	return s.sendTransaction(ctx, args.From, tx)
}

// BuildDecAssetTx ss
func (s *FusionTransactionAPI) BuildDecAssetTx(ctx context.Context, args AssetValueChangeArgs) (*types.Transaction, error) {
	args.IsInc = false
	return s.buildAssetValueChangeTx(ctx, args)
}

// DecAsset ss
func (s *FusionTransactionAPI) DecAsset(ctx context.Context, args AssetValueChangeArgs) (common.Hash, error) {
	tx, err := s.BuildDecAssetTx(ctx, args)
	if err != nil {
		return common.Hash{}, err
	}
	return s.sendTransaction(ctx, args.From, tx)
}

// BuildMakeSwapTx ss
func (s *FusionTransactionAPI) BuildMakeSwapTx(ctx context.Context, args MakeSwapArgs) (*types.Transaction, error) {

	args.init()

	big0 := big.NewInt(0)
	if args.MinFromAmount.ToInt().Cmp(big0) <= 0 || args.MinToAmount.ToInt().Cmp(big0) <= 0 || args.SwapSize.Cmp(big0) <= 0 {
		return nil, fmt.Errorf("MinFromAmount,MinToAmount and SwapSize must be ge 1")
	}

	state, _, err := s.b.StateAndHeaderByNumber(ctx, rpc.LatestBlockNumber)
	if state == nil || err != nil {
		return nil, err
	}

	total := new(big.Int).Mul(args.MinFromAmount.ToInt(), args.SwapSize)

	start := uint64(*args.FromStartTime)
	end := uint64(*args.FromEndTime)

	if start == common.TimeLockNow && end == common.TimeLockForever {
		if state.GetBalance(args.FromAssetID, args.From).Cmp(total) < 0 {
			return nil, fmt.Errorf("not enough from asset")
		}
	} else {
		needValue := common.NewTimeLock(&common.TimeLockItem{
			StartTime: start,
			EndTime:   end,
			Value:     total,
		})
		if state.GetTimeLockBalance(args.FromAssetID, args.From).Cmp(needValue) < 0 {
			return nil, fmt.Errorf("not enough time lock balance")
		}
	}

	funcData, err := args.toData()
	if err != nil {
		return nil, err
	}
	var param = common.FSNCallParam{Func: common.MakeSwapFunc, Data: funcData}
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
	swaps := state.AllSwaps()
	swap, ok := swaps[args.SwapID]
	if !ok {
		return nil, fmt.Errorf("Swap not found")
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
	swaps := state.AllSwaps()
	swap, ok := swaps[args.SwapID]
	if !ok {
		return nil, fmt.Errorf("Swap not found")
	}
	big0 := big.NewInt(0)
	if swap.SwapSize.Cmp(args.Size) < 0 || args.Size.Cmp(big0) <= 0 {
		return nil, fmt.Errorf("SwapSize must le and Size must be ge 1")
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
		if state.GetTimeLockBalance(swap.ToAssetID, args.From).Cmp(needValue) < 0 {
			return nil, fmt.Errorf("not enough time lock balance")
		}
	}
	funcData, err := args.toData()
	if err != nil {
		return nil, err
	}
	var param = common.FSNCallParam{Func: common.TakeSwapFunc, Data: funcData}
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
