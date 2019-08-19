package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"

	"github.com/FusionFoundation/efsn/cmd/utils"
	"github.com/FusionFoundation/efsn/common"
	"github.com/FusionFoundation/efsn/common/hexutil"
	"github.com/FusionFoundation/efsn/consensus/datong"
	"github.com/FusionFoundation/efsn/core/types"
	"github.com/FusionFoundation/efsn/rlp"

	"gopkg.in/urfave/cli.v1"
)

var (
	rawTxCommand = cli.Command{
		Name:     "rawtx",
		Usage:    "Process raw transaction",
		Category: "RAWTX COMMANDS",
		Description: `

Process raw transaction.`,
		Subcommands: []cli.Command{
			{
				Name:      "decode",
				Usage:     "Decode transaction from raw hex data",
				Action:    utils.MigrateFlags(decodeRawTx),
				Flags:     []cli.Flag{},
				ArgsUsage: "<hexstr>",
				Description: `
rawtx decode <hexstr>`,
			},
			{
				Name:      "decodeInput",
				Usage:     "Decode param from tx's input hex data",
				Action:    utils.MigrateFlags(decodeTxInput),
				Flags:     []cli.Flag{},
				ArgsUsage: "<hexstr>",
				Description: `
rawtx decodeInput <hexstr>`,
			},
			{
				Name:      "sendAsset",
				Usage:     "Create a 'sendAsset' raw transaction",
				Action:    utils.MigrateFlags(createSendAssetRawTx),
				Flags:     []cli.Flag{},
				ArgsUsage: "<asset> <to> <value>",
				Description: `
rawtx sendAsset <asset> <to> <value>`,
			},
			{
				Name:      "assetToTimeLock",
				Usage:     "Create a 'assetToTimeLock' raw transaction",
				Action:    utils.MigrateFlags(createAssetToTimeLockRawTx),
				Flags:     []cli.Flag{},
				ArgsUsage: "<asset> <to> <start> <end> <value>",
				Description: `
rawtx assetToTimeLock <asset> <to> <start> <end> <value>`,
			},
			{
				Name:      "timeLockToTimeLock",
				Usage:     "Create a 'timeLockToTimeLock' raw transaction",
				Action:    utils.MigrateFlags(createTimeLockToTimeLockRawTx),
				Flags:     []cli.Flag{},
				ArgsUsage: "<asset> <to> <start> <end> <value>",
				Description: `
rawtx timeLockToTimeLock <asset> <to> <start> <end> <value>`,
			},
			{
				Name:      "timeLockToAsset",
				Usage:     "Create a 'timeLockToAsset' raw transaction",
				Action:    utils.MigrateFlags(createTimeLockToAssetRawTx),
				Flags:     []cli.Flag{},
				ArgsUsage: "<asset> <to> <value>",
				Description: `
rawtx timeLockToAsset <asset> <to> <value>`,
			},
			{
				Name:      "genNotation",
				Usage:     "Create a 'genNotation' raw transaction",
				Action:    utils.MigrateFlags(createGenNotationRawTx),
				Flags:     []cli.Flag{},
				ArgsUsage: "",
				Description: `
rawtx genNotation`,
			},
			{
				Name:      "buyTicket",
				Usage:     "Create a 'buyTicket' raw transaction",
				Action:    utils.MigrateFlags(createBuyTicketRawTx),
				Flags:     []cli.Flag{},
				ArgsUsage: "<start> <end>",
				Description: `
rawtx buyTicket <start> <end>`,
			},
		},
	}
)

func printTx(tx *types.Transaction) error {
	bs, err := tx.MarshalJSON()
	if err != nil {
		return fmt.Errorf("json marshal err %v", err)
	}
	fmt.Println(string(bs))
	return nil
}

func printRawTx(tx *types.Transaction) error {
	bs, err := json.Marshal(&struct {
		Recipient *common.Address `json:"to"       rlp:"nil"`
		Payload   hexutil.Bytes   `json:"input"    gencodec:"required"`
	}{
		Recipient: tx.To(),
		Payload:   tx.Data(),
	})
	if err != nil {
		return fmt.Errorf("json marshal err %v", err)
	}
	fmt.Println(string(bs))
	return nil
}

func decodeRawTx(ctx *cli.Context) error {
	args := ctx.Args()
	if len(args) != 1 {
		return fmt.Errorf("wrong number of arguments")
	}
	data, err := hexutil.Decode(args.First())
	if err != nil {
		return fmt.Errorf("wrong arguments %v", err)
	}
	var tx types.Transaction
	err = rlp.Decode(bytes.NewReader(data), &tx)
	if err != nil {
		return fmt.Errorf("decode rawTx err %v", err)
	}
	return printTx(&tx)
}

func decodeFsnCallParam(fsnCall *common.FSNCallParam, funcParam interface{}) error {
	err := rlp.DecodeBytes(fsnCall.Data, funcParam)
	if err != nil {
		return fmt.Errorf("decode FSNCallParam err %v", err)
	}
	bs, err := json.Marshal(&struct {
		FuncType  string
		FuncParam interface{}
	}{
		FuncType:  fsnCall.Func.Name(),
		FuncParam: funcParam,
	})
	if err != nil {
		return fmt.Errorf("json marshal err %v", err)
	}
	fmt.Println(string(bs))
	return nil
}

func decodeTxInput(ctx *cli.Context) error {
	args := ctx.Args()
	if len(args) != 1 {
		return fmt.Errorf("wrong number of arguments")
	}
	data, err := hexutil.Decode(args.First())
	if err != nil {
		return fmt.Errorf("wrong arguments %v", err)
	}
	var fsnCall common.FSNCallParam
	rlp.DecodeBytes(data, &fsnCall)
	if err != nil {
		return fmt.Errorf("decode FSNCallParam err %v", err)
	}

	switch fsnCall.Func {
	case common.GenNotationFunc:
		return decodeFsnCallParam(&fsnCall, &GenNotationParam{})
	case common.GenAssetFunc:
		return decodeFsnCallParam(&fsnCall, &common.GenAssetParam{})
	case common.SendAssetFunc:
		return decodeFsnCallParam(&fsnCall, &common.SendAssetParam{})
	case common.TimeLockFunc:
		return decodeFsnCallParam(&fsnCall, &common.TimeLockParam{})
	case common.BuyTicketFunc:
		return decodeFsnCallParam(&fsnCall, &common.BuyTicketParam{})
	case common.AssetValueChangeFunc:
		return decodeFsnCallParam(&fsnCall, &common.AssetValueChangeExParam{})
	case common.EmptyFunc:
	case common.MakeSwapFunc, common.MakeSwapFuncExt:
		return decodeFsnCallParam(&fsnCall, &common.MakeSwapParam{})
	case common.RecallSwapFunc:
		return decodeFsnCallParam(&fsnCall, &common.RecallSwapParam{})
	case common.TakeSwapFunc, common.TakeSwapFuncExt:
		return decodeFsnCallParam(&fsnCall, &common.TakeSwapParam{})
	case common.RecallMultiSwapFunc:
		return decodeFsnCallParam(&fsnCall, &common.RecallMultiSwapParam{})
	case common.MakeMultiSwapFunc:
		return decodeFsnCallParam(&fsnCall, &common.MakeMultiSwapParam{})
	case common.TakeMultiSwapFunc:
		return decodeFsnCallParam(&fsnCall, &common.TakeMultiSwapParam{})
	case common.ReportIllegalFunc:
		h1, h2, err := datong.DecodeReport(fsnCall.Data)
		if err != nil {
			return fmt.Errorf("DecodeReport err %v", err)
		}
		reportContent := &struct {
			Header1 *types.Header
			Header2 *types.Header
		}{
			Header1: h1,
			Header2: h2,
		}
		bs, err := json.Marshal(&struct {
			FuncType  string
			FuncParam interface{}
		}{
			FuncType:  fsnCall.Func.Name(),
			FuncParam: reportContent,
		})
		if err != nil {
			return fmt.Errorf("json marshal err %v", err)
		}
		fmt.Println(string(bs))
	default:
		return fmt.Errorf("Unknown FuncType %v", fsnCall.Func)
	}
	return nil
}

type ParamInterface interface {
	ToBytes() ([]byte, error)
}

func toRawTx(funcType common.FSNCallFunc, funcParam ParamInterface) (*types.Transaction, error) {
	funcData, err := funcParam.ToBytes()
	if err != nil {
		return nil, err
	}
	var param = common.FSNCallParam{Func: funcType, Data: funcData}
	input, err := param.ToBytes()
	if err != nil {
		return nil, err
	}
	tx := types.NewTransaction(
		0,
		common.FSNCallAddress,
		big.NewInt(0),
		0,
		big.NewInt(0),
		input,
	)
	return tx, nil
}

func createSendAssetRawTx(ctx *cli.Context) error {
	args := ctx.Args()
	if len(args) != 3 {
		return fmt.Errorf("wrong number of arguments")
	}
	asset := common.HexToHash(args[0])
	to := common.HexToAddress(args[1])
	value, ok := new(big.Int).SetString(args[2], 0)
	if !ok {
		return fmt.Errorf(args[2] + " is not a right big number")
	}

	param := common.SendAssetParam{
		AssetID: asset,
		To:      to,
		Value:   value,
	}
	tx, err := toRawTx(common.SendAssetFunc, &param)
	if err != nil {
		return err
	}
	return printRawTx(tx)
}

func createAssetToTimeLockRawTx(ctx *cli.Context) error {
	args := ctx.Args()
	if len(args) != 5 {
		return fmt.Errorf("wrong number of arguments")
	}
	asset := common.HexToHash(args[0])
	to := common.HexToAddress(args[1])
	start, err := strconv.ParseUint(args[2], 0, 64)
	if err != nil {
		return fmt.Errorf("Invalid start number: %v", err)
	}
	end, err := strconv.ParseUint(args[3], 0, 64)
	if err != nil {
		return fmt.Errorf("Invalid end number: %v", err)
	}
	value, ok := new(big.Int).SetString(args[4], 0)
	if !ok {
		return fmt.Errorf(args[4] + " is not a right big number")
	}

	param := common.TimeLockParam{
		Type:      common.AssetToTimeLock,
		AssetID:   asset,
		To:        to,
		StartTime: start,
		EndTime:   end,
		Value:     value,
	}
	tx, err := toRawTx(common.TimeLockFunc, &param)
	if err != nil {
		return err
	}
	return printRawTx(tx)
}

func createTimeLockToTimeLockRawTx(ctx *cli.Context) error {
	args := ctx.Args()
	if len(args) != 5 {
		return fmt.Errorf("wrong number of arguments")
	}
	asset := common.HexToHash(args[0])
	to := common.HexToAddress(args[1])
	start, err := strconv.ParseUint(args[2], 0, 64)
	if err != nil {
		return fmt.Errorf("Invalid start number: %v", err)
	}
	end, err := strconv.ParseUint(args[3], 0, 64)
	if err != nil {
		return fmt.Errorf("Invalid end number: %v", err)
	}
	value, ok := new(big.Int).SetString(args[4], 0)
	if !ok {
		return fmt.Errorf(args[4] + " is not a right big number")
	}

	param := common.TimeLockParam{
		Type:      common.TimeLockToTimeLock,
		AssetID:   asset,
		To:        to,
		StartTime: start,
		EndTime:   end,
		Value:     value,
	}
	tx, err := toRawTx(common.TimeLockFunc, &param)
	if err != nil {
		return err
	}
	return printRawTx(tx)
}

func createTimeLockToAssetRawTx(ctx *cli.Context) error {
	args := ctx.Args()
	if len(args) != 3 {
		return fmt.Errorf("wrong number of arguments")
	}
	asset := common.HexToHash(args[0])
	to := common.HexToAddress(args[1])
	value, ok := new(big.Int).SetString(args[2], 0)
	if !ok {
		return fmt.Errorf(args[2] + " is not a right big number")
	}

	param := common.TimeLockParam{
		Type:      common.TimeLockToAsset,
		AssetID:   asset,
		To:        to,
		StartTime: common.TimeLockNow,
		EndTime:   common.TimeLockForever,
		Value:     value,
	}
	tx, err := toRawTx(common.TimeLockFunc, &param)
	if err != nil {
		return err
	}
	return printRawTx(tx)
}

type GenNotationParam struct{}

func (p *GenNotationParam) ToBytes() ([]byte, error) {
	return nil, nil
}

func createGenNotationRawTx(ctx *cli.Context) error {
	tx, err := toRawTx(common.GenNotationFunc, &GenNotationParam{})
	if err != nil {
		return err
	}
	return printRawTx(tx)
}

func createBuyTicketRawTx(ctx *cli.Context) error {
	args := ctx.Args()
	if len(args) != 2 {
		return fmt.Errorf("wrong number of arguments")
	}
	start, err := strconv.ParseUint(args[0], 0, 64)
	if err != nil {
		return fmt.Errorf("Invalid start number: %v", err)
	}
	end, err := strconv.ParseUint(args[1], 0, 64)
	if err != nil {
		return fmt.Errorf("Invalid end number: %v", err)
	}

	param := common.BuyTicketParam{
		Start: start,
		End:   end,
	}
	tx, err := toRawTx(common.BuyTicketFunc, &param)
	if err != nil {
		return err
	}
	return printRawTx(tx)
}
