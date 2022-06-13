package ethclient

import (
	"context"
	ethereum "github.com/FusionFoundation/efsn/v4"
	"github.com/FusionFoundation/efsn/v4/common"
	"github.com/FusionFoundation/efsn/v4/common/hexutil"
	"github.com/FusionFoundation/efsn/v4/eth/tracers"
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
