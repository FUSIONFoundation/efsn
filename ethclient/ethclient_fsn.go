package ethclient

import (
	"context"
	ethereum "github.com/FusionFoundation/efsn/v5"
	"github.com/FusionFoundation/efsn/v5/common"
	"github.com/FusionFoundation/efsn/v5/common/hexutil"
	"github.com/FusionFoundation/efsn/v5/eth/tracers"
	"math/big"
)

// GetBlockReward get the Total Reward from Block
func (ec *Client) GetBlockReward(ctx context.Context, blockNumber *big.Int) (string, error) {
	var reward string
	if err := ec.c.CallContext(ctx, &reward, "fsn_getBlockReward", toBlockNumArg(blockNumber)); err != nil {
		return "", err
	}
	return reward, nil
}

// AssetBalanceAt returns the wei balance of the given account.
// The block number can be nil, in which case the balance is taken from the latest known block.
func (ec *Client) AssetBalanceAt(ctx context.Context, assetId common.Hash, account common.Address, blockNumber *big.Int) (string, error) {
	var result string
	err := ec.c.CallContext(ctx, &result, "fsn_getBalance", assetId, account, toBlockNumArg(blockNumber))
	return result, err
}

// AssetTimeLockBalanceAt returns the wei timelock balance of the given account.
// The block number can be nil, in which case the balance is taken from the latest known block.
func (ec *Client) AssetTimeLockBalanceAt(ctx context.Context, assetId common.Hash, account common.Address, blockNumber *big.Int) (*common.TimeLock, error) {
	var result common.TimeLock
	err := ec.c.CallContext(ctx, &result, "fsn_getTimeLockBalance", assetId, account, toBlockNumArg(blockNumber))
	return &result, err
}

// CallTrace is the result of a callTracer run.
type CallTrace struct {
	Type    string          `json:"type"`
	From    common.Address  `json:"from"`
	To      common.Address  `json:"to"`
	Input   hexutil.Bytes   `json:"input"`
	Output  hexutil.Bytes   `json:"output"`
	Gas     *hexutil.Uint64 `json:"gas,omitempty"`
	GasUsed *hexutil.Uint64 `json:"gasUsed,omitempty"`
	Value   *hexutil.Big    `json:"value,omitempty"`
	Error   string          `json:"error,omitempty"`
	Calls   []CallTrace     `json:"calls,omitempty"`
}

var callTracer = "callTracer"
var callTracerTimeout = "10s"

func (ec *Client) TraceInternalTx(ctx context.Context, tx common.Hash) (*CallTrace, error) {
	var result CallTrace
	err := ec.c.CallContext(ctx, &result, "debug_traceTransaction", tx, &tracers.TraceConfig{Tracer: &callTracer, Timeout: &callTracerTimeout})
	return &result, err
}

var returnMsgTracer = "returnMsgTracer"

func (ec *Client) TraceTxErrMsg(ctx context.Context, tx common.Hash) (string, error) {
	var result string
	err := ec.c.CallContext(ctx, &result, "debug_traceTransaction", tx, &tracers.TraceConfig{Tracer: &returnMsgTracer})
	return result, err
}

func (ec *Client) GetSwap(ctx context.Context, swapID common.Hash, blockNumber *big.Int) (*common.Swap, error) {
	var result *common.Swap
	err := ec.c.CallContext(ctx, &result, "fsn_getSwap", swapID, toBlockNumArg(blockNumber))
	if err == nil && result == nil {
		return nil, ethereum.NotFound
	}
	return result, err
}

func (ec *Client) GetMultiSwap(ctx context.Context, swapID common.Hash, blockNumber *big.Int) (*common.MultiSwap, error) {
	var result *common.MultiSwap
	err := ec.c.CallContext(ctx, &result, "fsn_getMultiSwap", swapID, toBlockNumArg(blockNumber))
	if err == nil && result == nil {
		return nil, ethereum.NotFound
	}
	return result, err
}

type Asset struct {
	ID          common.Hash
	Owner       common.Address
	Name        string
	Symbol      string
	Decimals    uint8
	Total       string
	CanChange   bool
	Description string
}

func (ec *Client) GetAsset(ctx context.Context, assetId common.Hash, blockNumber *big.Int) (*Asset, error) {
	var result *Asset
	err := ec.c.CallContext(ctx, &result, "fsn_getAsset", assetId, toBlockNumArg(blockNumber))
	if err == nil && result == nil {
		return nil, ethereum.NotFound
	}
	return result, err
}

func (ec *Client) NotationAt(ctx context.Context, account common.Address, blockNumber *big.Int) (uint64, error) {
	var result uint64
	err := ec.c.CallContext(ctx, &result, "fsn_getNotation", account, toBlockNumArg(blockNumber))
	return result, err
}
