package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"

	"github.com/FusionFoundation/efsn/v4/common"
	"github.com/FusionFoundation/efsn/v4/common/hexutil"
	"github.com/FusionFoundation/efsn/v4/consensus/datong"
	"github.com/FusionFoundation/efsn/v4/core/types"
	"github.com/FusionFoundation/efsn/v4/rlp"

	"github.com/urfave/cli/v2"
)

var (
	rawTxCommand = &cli.Command{
		Name:  "rawtx",
		Usage: "Process raw transaction",
		Description: `
Process raw transaction.`,
		Subcommands: []*cli.Command{
			{
				Name:      "decodeRawTx",
				Usage:     "Decode transaction from rawtx hex data",
				Action:    decodeRawTx,
				Flags:     []cli.Flag{},
				ArgsUsage: "<hexstr> [decodeinput]",
				Description: `
rawtx decodeRawTx <hexstr> [decodeinput]
decodeinput defaults to true, you can specify false to ignore it.`,
			},
			{
				Name:      "decodeTxInput",
				Usage:     "Decode fsn call param from tx input hex data",
				Action:    decodeTxInput,
				Flags:     []cli.Flag{},
				ArgsUsage: "<hexstr>",
				Description: `
rawtx decodeTxInput <hexstr>`,
			},
			{
				Name:      "decodeLogData",
				Usage:     "Decode log data from tx receipt log hex data",
				Action:    decodeLogData,
				Flags:     []cli.Flag{},
				ArgsUsage: "<hexstr>",
				Description: `
rawtx decodeLogData <hexstr>`,
			},
			{
				Name:      "sendAsset",
				Usage:     "Create a 'sendAsset' raw transaction",
				Action:    createSendAssetRawTx,
				Flags:     []cli.Flag{},
				ArgsUsage: "<asset> <to> <value>",
				Description: `
rawtx sendAsset <asset> <to> <value>`,
			},
			{
				Name:      "assetToTimeLock",
				Usage:     "Create a 'assetToTimeLock' raw transaction",
				Action:    createAssetToTimeLockRawTx,
				Flags:     []cli.Flag{},
				ArgsUsage: "<asset> <to> <start> <end> <value>",
				Description: `
rawtx assetToTimeLock <asset> <to> <start> <end> <value>`,
			},
			{
				Name:      "timeLockToTimeLock",
				Usage:     "Create a 'timeLockToTimeLock' raw transaction",
				Action:    createTimeLockToTimeLockRawTx,
				Flags:     []cli.Flag{},
				ArgsUsage: "<asset> <to> <start> <end> <value>",
				Description: `
rawtx timeLockToTimeLock <asset> <to> <start> <end> <value>`,
			},
			{
				Name:      "timeLockToAsset",
				Usage:     "Create a 'timeLockToAsset' raw transaction",
				Action:    createTimeLockToAssetRawTx,
				Flags:     []cli.Flag{},
				ArgsUsage: "<asset> <to> <value>",
				Description: `
rawtx timeLockToAsset <asset> <to> <value>`,
			},
			{
				Name:      "genNotation",
				Usage:     "Create a 'genNotation' raw transaction",
				Action:    createGenNotationRawTx,
				Flags:     []cli.Flag{},
				ArgsUsage: "",
				Description: `
rawtx genNotation`,
			},
			{
				Name:      "buyTicket",
				Usage:     "Create a 'buyTicket' raw transaction",
				Action:    createBuyTicketRawTx,
				Flags:     []cli.Flag{},
				ArgsUsage: "<start> <end>",
				Description: `
rawtx buyTicket <start> <end>`,
			},
		},
	}
)

func printTx(tx *types.Transaction, decodeInput bool) error {
	var bs []byte
	var err error
	if decodeInput {
		fsnTxInput, err := datong.DecodeTxInput(tx.Data())
		if err != nil {
			return fmt.Errorf("decode FSNCallParam err %v", err)
		}
		txExt := &struct {
			Tx         *types.Transaction `json:"tx"`
			FsnTxInput interface{}        `json:"fsnTxInput,omitempty"`
		}{
			Tx:         tx,
			FsnTxInput: fsnTxInput,
		}
		bs, err = json.Marshal(txExt)
		if err != nil {
			return fmt.Errorf("json marshal err %v", err)
		}
	} else {
		bs, err = tx.MarshalJSON()
		if err != nil {
			return fmt.Errorf("json marshal err %v", err)
		}
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
	if args.Len() < 1 || args.Len() > 2 {
		return fmt.Errorf("wrong number of arguments")
	}
	decodeInput := true
	if args.Len() > 1 && args.Get(1) == "false" {
		decodeInput = false
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
	return printTx(&tx, decodeInput)
}

func decodeTxInput(ctx *cli.Context) error {
	args := ctx.Args()
	if args.Len() != 1 {
		return fmt.Errorf("wrong number of arguments")
	}
	data, err := hexutil.Decode(args.First())
	if err != nil {
		return fmt.Errorf("wrong arguments %v", err)
	}
	res, err := datong.DecodeTxInput(data)
	if err != nil {
		return fmt.Errorf("decode FSNCallParam err %v", err)
	}
	bs, err := json.Marshal(res)
	if err != nil {
		return fmt.Errorf("json marshal err %v", err)
	}
	fmt.Println(string(bs))
	return nil
}

func decodeLogData(ctx *cli.Context) error {
	args := ctx.Args()
	if args.Len() != 1 {
		return fmt.Errorf("wrong number of arguments")
	}
	data, err := hexutil.Decode(args.First())
	if err != nil {
		return fmt.Errorf("wrong arguments %v", err)
	}
	res, err := datong.DecodeLogData(data)
	if err != nil {
		return fmt.Errorf("decode log data err %v", err)
	}
	bs, err := json.Marshal(res)
	if err != nil {
		return fmt.Errorf("json marshal err %v", err)
	}
	fmt.Println(string(bs))
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
	if args.Len() != 3 {
		return fmt.Errorf("wrong number of arguments")
	}
	asset := common.HexToHash(args.First())
	to := common.HexToAddress(args.Get(1))
	value, ok := new(big.Int).SetString(args.Get(2), 0)
	if !ok {
		return fmt.Errorf(args.Get(2) + " is not a right big number")
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
	if args.Len() != 5 {
		return fmt.Errorf("wrong number of arguments")
	}
	asset := common.HexToHash(args.First())
	to := common.HexToAddress(args.Get(1))
	start, err := strconv.ParseUint(args.Get(2), 0, 64)
	if err != nil {
		return fmt.Errorf("Invalid start number: %v", err)
	}
	end, err := strconv.ParseUint(args.Get(3), 0, 64)
	if err != nil {
		return fmt.Errorf("Invalid end number: %v", err)
	}
	value, ok := new(big.Int).SetString(args.Get(4), 0)
	if !ok {
		return fmt.Errorf(args.Get(4) + " is not a right big number")
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
	if args.Len() != 5 {
		return fmt.Errorf("wrong number of arguments")
	}
	asset := common.HexToHash(args.First())
	to := common.HexToAddress(args.Get(1))
	start, err := strconv.ParseUint(args.Get(2), 0, 64)
	if err != nil {
		return fmt.Errorf("Invalid start number: %v", err)
	}
	end, err := strconv.ParseUint(args.Get(3), 0, 64)
	if err != nil {
		return fmt.Errorf("Invalid end number: %v", err)
	}
	value, ok := new(big.Int).SetString(args.Get(4), 0)
	if !ok {
		return fmt.Errorf(args.Get(4) + " is not a right big number")
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
	if args.Len() != 3 {
		return fmt.Errorf("wrong number of arguments")
	}
	asset := common.HexToHash(args.First())
	to := common.HexToAddress(args.Get(1))
	value, ok := new(big.Int).SetString(args.Get(2), 0)
	if !ok {
		return fmt.Errorf(args.Get(2) + " is not a right big number")
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

func createGenNotationRawTx(ctx *cli.Context) error {
	tx, err := toRawTx(common.GenNotationFunc, &common.EmptyParam{})
	if err != nil {
		return err
	}
	return printRawTx(tx)
}

func createBuyTicketRawTx(ctx *cli.Context) error {
	args := ctx.Args()
	if args.Len() != 2 {
		return fmt.Errorf("wrong number of arguments")
	}
	start, err := strconv.ParseUint(args.First(), 0, 64)
	if err != nil {
		return fmt.Errorf("Invalid start number: %v", err)
	}
	end, err := strconv.ParseUint(args.Get(1), 0, 64)
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
