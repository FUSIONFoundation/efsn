package ethapi

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rpc"
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

// AssetValueChangeArgs wacom
type AssetValueChangeArgs struct {
	SendAssetArgs
	IsInc bool `json:"isInc"`
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

func (args *AssetValueChangeArgs) toData() ([]byte, error) {
	param := common.AssetValueChangeParam{
		AssetID: args.AssetID,
		To:      args.To,
		Value:   args.Value.ToInt(),
		IsInc:   args.IsInc,
	}
	return param.ToBytes()
}

func (args *TimeLockArgs) init() {

	if args.StartTime == nil {
		*(*uint64)(args.StartTime) = common.TimeLockNow
	}

	if args.EndTime == nil {
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
func (s *PublicFusionAPI) GetBalance(ctx context.Context, assetID common.Hash, address common.Address, blockNr rpc.BlockNumber) (*big.Int, error) {
	state, _, err := s.b.StateAndHeaderByNumber(ctx, blockNr)
	if state == nil || err != nil {
		return new(big.Int), err
	}
	b := state.GetBalance(assetID, address)
	return b, state.Error()
}

// GetAllBalances wacom
func (s *PublicFusionAPI) GetAllBalances(ctx context.Context, address common.Address, blockNr rpc.BlockNumber) (map[common.Hash]*big.Int, error) {
	state, _, err := s.b.StateAndHeaderByNumber(ctx, blockNr)
	if state == nil || err != nil {
		return make(map[common.Hash]*big.Int), err
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
		return common.Hash{}, fmt.Errorf("An address just can gen a notation, your have a notation:%d", calcNotationDisplay(notation))
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
	*(*uint64)(args.StartTime) = uint64(time.Now().Unix())
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
func (s *PrivateFusionAPI) BuyTicket(ctx context.Context, args FusionBaseArgs, passwd string) (common.Hash, error) {
	state, _, err := s.b.StateAndHeaderByNumber(ctx, rpc.LatestBlockNumber)
	if state == nil || err != nil {
		return common.Hash{}, err
	}

	block, err := s.b.BlockByNumber(ctx, rpc.LatestBlockNumber)
	if block == nil || err != nil {
		return common.Hash{}, err
	}

	start := block.Time().Uint64()
	value := big.NewInt(1)
	needValue := common.NewTimeLock(&common.TimeLockItem{
		StartTime: start,
		EndTime:   start + 40*24*3600,
		Value:     value,
	})
	if state.GetTimeLockBalance(common.SystemAssetID, args.From).Cmp(needValue) < 0 {
		return common.Hash{}, fmt.Errorf("not enough time lock balance")
	}

	var param = common.FSNCallParam{Func: common.BuyTicketFunc}
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

func calcNotationDisplay(notation uint64) uint64 {
	if notation == 0 {
		return notation
	}
	check := (notation ^ 8192 ^ 13 + 73/76798669*708583737978) % 100
	return (notation*100 + check)
}
