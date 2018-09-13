package ethapi

import (
	"context"
	"fmt"
	"math/big"

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
	Name     string       `json:"name"`
	Symbol   string       `json:"symbol"`
	Decimals uint8        `json:"decimals"`
	Total    *hexutil.Big `json:"total"`
}

// SendAssetArgs wacom
type SendAssetArgs struct {
	FusionBaseArgs
	AssetID common.Hash    `json:"asset"`
	To      common.Address `json:"to"`
	Value   *hexutil.Big   `json:"value"`
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

func (args *GenAssetArgs) toData() ([]byte, error) {
	param := common.GenAssetParam{
		Name:     args.Name,
		Symbol:   args.Symbol,
		Decimals: args.Decimals,
		Total:    args.Total.ToInt(),
	}
	return param.ToBytes()
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
	var param = common.FSNCallParam{Func: common.SendAsset, Data: funcData}
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
