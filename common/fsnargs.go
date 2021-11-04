package common

import (
	"math/big"

	"github.com/FusionFoundation/efsn/v4/common/hexutil"
)

type FSNBaseArgsInterface interface {
	BaseArgs() *FusionBaseArgs
	ToData() ([]byte, error)
}

func (args *FusionBaseArgs) BaseArgs() *FusionBaseArgs {
	return args
}

// FusionBaseArgs wacom
type FusionBaseArgs struct {
	From     Address         `json:"from"`
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
	AssetID Hash         `json:"asset"`
	To      Address      `json:"to"`
	ToUSAN  uint64       `json:"toUSAN"`
	Value   *hexutil.Big `json:"value"`
}

// TimeLockArgs wacom
type TimeLockArgs struct {
	SendAssetArgs
	StartTime    *hexutil.Uint64 `json:"start"`
	EndTime      *hexutil.Uint64 `json:"end"`
	TimeLockType TimeLockType    `json:"type"`
}

// BuyTicketArgs wacom
type BuyTicketArgs struct {
	FusionBaseArgs
	Start *hexutil.Uint64 `json:"start"`
	End   *hexutil.Uint64 `json:"end"`
}

type AssetValueChangeExArgs struct {
	FusionBaseArgs
	AssetID     Hash         `json:"asset"`
	To          Address      `json:"to"`
	Value       *hexutil.Big `json:"value"`
	IsInc       bool         `json:"isInc"`
	TransacData string       `json:"transacData"`
}

// MakeSwapArgs wacom
type MakeSwapArgs struct {
	FusionBaseArgs
	FromAssetID   Hash
	FromStartTime *hexutil.Uint64
	FromEndTime   *hexutil.Uint64
	MinFromAmount *hexutil.Big
	ToAssetID     Hash
	ToStartTime   *hexutil.Uint64
	ToEndTime     *hexutil.Uint64
	MinToAmount   *hexutil.Big
	SwapSize      *big.Int
	Targes        []Address
	Time          *big.Int
	Description   string
}

// RecallSwapArgs wacom
type RecallSwapArgs struct {
	FusionBaseArgs
	SwapID Hash
}

// TakeSwapArgs wacom
type TakeSwapArgs struct {
	FusionBaseArgs
	SwapID Hash
	Size   *big.Int
}

// MakeMultiSwapArgs wacom
type MakeMultiSwapArgs struct {
	FusionBaseArgs
	FromAssetID   []Hash
	FromStartTime []*hexutil.Uint64
	FromEndTime   []*hexutil.Uint64
	MinFromAmount []*hexutil.Big
	ToAssetID     []Hash
	ToStartTime   []*hexutil.Uint64
	ToEndTime     []*hexutil.Uint64
	MinToAmount   []*hexutil.Big
	SwapSize      *big.Int
	Targes        []Address
	Time          *big.Int
	Description   string
}

// RecallMultiSwapArgs wacom
type RecallMultiSwapArgs struct {
	FusionBaseArgs
	SwapID Hash
}

// TakeSwapArgs wacom
type TakeMultiSwapArgs struct {
	FusionBaseArgs
	SwapID Hash
	Size   *big.Int
}

//////////////////// args ToParam, ToData, Init ///////////////////////

func (args *FusionBaseArgs) ToData() ([]byte, error) {
	return nil, nil
}

func (args *SendAssetArgs) ToParam() *SendAssetParam {
	return &SendAssetParam{
		AssetID: args.AssetID,
		To:      args.To,
		Value:   args.Value.ToInt(),
	}
}

func (args *SendAssetArgs) ToData() ([]byte, error) {
	return args.ToParam().ToBytes()
}

func (args *TimeLockArgs) ToParam() *TimeLockParam {
	return &TimeLockParam{
		Type:      args.TimeLockType,
		AssetID:   args.AssetID,
		To:        args.To,
		StartTime: uint64(*args.StartTime),
		EndTime:   uint64(*args.EndTime),
		Value:     args.Value.ToInt(),
	}
}

func (args *TimeLockArgs) ToData() ([]byte, error) {
	return args.ToParam().ToBytes()
}

func (args *GenAssetArgs) ToParam() *GenAssetParam {
	return &GenAssetParam{
		Name:        args.Name,
		Symbol:      args.Symbol,
		Decimals:    args.Decimals,
		Total:       args.Total.ToInt(),
		CanChange:   args.CanChange,
		Description: args.Description,
	}
}

func (args *GenAssetArgs) ToData() ([]byte, error) {
	return args.ToParam().ToBytes()
}

func (args *BuyTicketArgs) ToParam() *BuyTicketParam {
	return &BuyTicketParam{
		Start: uint64(*args.Start),
		End:   uint64(*args.End),
	}
}

func (args *BuyTicketArgs) ToData() ([]byte, error) {
	return args.ToParam().ToBytes()
}

func (args *BuyTicketArgs) Init(defStart uint64) {

	if args.Start == nil {
		args.Start = new(hexutil.Uint64)
		*(*uint64)(args.Start) = defStart
	}

	if args.End == nil {
		args.End = new(hexutil.Uint64)
		*(*uint64)(args.End) = uint64(*args.Start) + 30*24*3600
	}
}

func (args *AssetValueChangeExArgs) ToParam() *AssetValueChangeExParam {
	return &AssetValueChangeExParam{
		AssetID:     args.AssetID,
		To:          args.To,
		Value:       args.Value.ToInt(),
		IsInc:       args.IsInc,
		TransacData: args.TransacData,
	}
}

func (args *AssetValueChangeExArgs) ToData() ([]byte, error) {
	return args.ToParam().ToBytes()
}

func (args *MakeSwapArgs) Init(time *big.Int) {
	args.Time = time

	if args.FromStartTime == nil {
		args.FromStartTime = new(hexutil.Uint64)
		*(*uint64)(args.FromStartTime) = TimeLockNow
	}

	if args.FromEndTime == nil {
		args.FromEndTime = new(hexutil.Uint64)
		*(*uint64)(args.FromEndTime) = TimeLockForever
	}

	if args.ToStartTime == nil {
		args.ToStartTime = new(hexutil.Uint64)
		*(*uint64)(args.ToStartTime) = TimeLockNow
	}

	if args.ToEndTime == nil {
		args.ToEndTime = new(hexutil.Uint64)
		*(*uint64)(args.ToEndTime) = TimeLockForever
	}
}

func (args *MakeSwapArgs) ToParam() *MakeSwapParam {
	return &MakeSwapParam{
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
		Time:          args.Time,
		Description:   args.Description,
	}
}

func (args *MakeSwapArgs) ToData() ([]byte, error) {
	return args.ToParam().ToBytes()
}

func (args *RecallSwapArgs) ToParam() *RecallSwapParam {
	return &RecallSwapParam{
		SwapID: args.SwapID,
	}
}

func (args *RecallSwapArgs) ToData() ([]byte, error) {
	return args.ToParam().ToBytes()
}

func (args *TakeSwapArgs) ToParam() *TakeSwapParam {
	return &TakeSwapParam{
		SwapID: args.SwapID,
		Size:   args.Size,
	}
}

func (args *TakeSwapArgs) ToData() ([]byte, error) {
	return args.ToParam().ToBytes()
}

func (args *MakeMultiSwapArgs) Init(time *big.Int) {
	args.Time = time

	ln := len(args.MinFromAmount)

	l := len(args.FromStartTime)
	if l < ln {
		temp := make([]*hexutil.Uint64, ln)
		copy(temp[:l], args.FromStartTime)
		for i := l; i < ln; i++ {
			temp[i] = new(hexutil.Uint64)
			*(*uint64)(temp[i]) = TimeLockNow
		}
		args.FromStartTime = temp
	}

	l = len(args.FromEndTime)
	if l < ln {
		temp := make([]*hexutil.Uint64, ln)
		copy(temp[:l], args.FromEndTime)
		for i := l; i < ln; i++ {
			temp[i] = new(hexutil.Uint64)
			*(*uint64)(temp[i]) = TimeLockForever
		}
		args.FromEndTime = temp
	}

	ln = len(args.MinToAmount)

	l = len(args.ToStartTime)
	if l < ln {
		temp := make([]*hexutil.Uint64, ln)
		copy(temp[:l], args.ToStartTime)
		for i := l; i < ln; i++ {
			temp[i] = new(hexutil.Uint64)
			*(*uint64)(temp[i]) = TimeLockNow
		}
		args.ToStartTime = temp
	}

	l = len(args.ToEndTime)
	if l < ln {
		temp := make([]*hexutil.Uint64, ln)
		copy(temp[:l], args.ToEndTime)
		for i := l; i < ln; i++ {
			temp[i] = new(hexutil.Uint64)
			*(*uint64)(temp[i]) = TimeLockForever
		}
		args.ToEndTime = temp
	}
}

func (args *MakeMultiSwapArgs) ToParam() *MakeMultiSwapParam {
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
	return &MakeMultiSwapParam{
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
		Time:          args.Time,
		Description:   args.Description,
	}
}

func (args *MakeMultiSwapArgs) ToData() ([]byte, error) {
	return args.ToParam().ToBytes()
}

func (args *RecallMultiSwapArgs) ToParam() *RecallMultiSwapParam {
	return &RecallMultiSwapParam{
		SwapID: args.SwapID,
	}
}

func (args *RecallMultiSwapArgs) ToData() ([]byte, error) {
	return args.ToParam().ToBytes()
}

func (args *TakeMultiSwapArgs) ToParam() *TakeMultiSwapParam {
	return &TakeMultiSwapParam{
		SwapID: args.SwapID,
		Size:   args.Size,
	}
}

func (args *TakeMultiSwapArgs) ToData() ([]byte, error) {
	return args.ToParam().ToBytes()
}

func (args *TimeLockArgs) Init(timeLockType TimeLockType) {
	args.TimeLockType = timeLockType

	if args.StartTime == nil {
		args.StartTime = new(hexutil.Uint64)
		*(*uint64)(args.StartTime) = TimeLockNow
	}

	if args.EndTime == nil {
		args.EndTime = new(hexutil.Uint64)
		*(*uint64)(args.EndTime) = TimeLockForever
	}
}
