package ethapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/FusionFoundation/efsn/accounts"
	"github.com/FusionFoundation/efsn/common"
	"github.com/FusionFoundation/efsn/common/hexutil"
	"github.com/FusionFoundation/efsn/core/rawdb"
	"github.com/FusionFoundation/efsn/core/state"
	"github.com/FusionFoundation/efsn/core/types"
	"github.com/FusionFoundation/efsn/log"
	"github.com/FusionFoundation/efsn/rlp"
	"github.com/FusionFoundation/efsn/rpc"
)

var lastBlockOfBuyTickets = int64(0)
var buyTicketOnBlockMap map[common.Address]bool
var buyTicketOnBlockMapMutex sync.Mutex

//--------------------------------------------- PublicFusionAPI -------------------------------------

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

// IsAutoBuyTicket wacom
func (s *PublicFusionAPI) IsAutoBuyTicket(ctx context.Context) bool {
	return common.AutoBuyTicket
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

// GetRawTimeLockBalance wacom
func (s *PublicFusionAPI) GetRawTimeLockBalance(ctx context.Context, assetID common.Hash, address common.Address, blockNr rpc.BlockNumber) (*common.TimeLock, error) {
	state, _, err := s.b.StateAndHeaderByNumber(ctx, blockNr)
	if state == nil || err != nil {
		return new(common.TimeLock), err
	}
	b := state.GetTimeLockBalance(assetID, address)
	return b, state.Error()
}

// GetAllRawTimeLockBalances wacom
func (s *PublicFusionAPI) GetAllRawTimeLockBalances(ctx context.Context, address common.Address, blockNr rpc.BlockNumber) (map[common.Hash]*common.TimeLock, error) {
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
	b := state.GetNotation(address)
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
	if asset, err := state.GetAsset(assetID); err == nil {
		return &asset, nil
	}

	// treat assetID as tx hash, deduct asset id from the tx
	if id := s.getIDByTxHash(ctx, assetID, "AssetID"); id != (common.Hash{}) {
		if asset, err := state.GetAsset(id); err == nil {
			return &asset, nil
		}
	}
	return nil, fmt.Errorf("Asset not found")
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

// TxAndReceipt wacom
type TxAndReceipt struct {
	Tx           *RPCTransaction        `json:"tx"`
	Receipt      map[string]interface{} `json:"receipt"`
	ReceiptFound bool                   `json:"receiptFound"`
}

// GetTransactionAndReceipt returns the transaction receipt for the given transaction hash.
func (s *PublicFusionAPI) GetTransactionAndReceipt(ctx context.Context, hash common.Hash) (TxAndReceipt, error) {
	// Try to return an already finalized transaction
	var orgTx *RPCTransaction
	if tx, blockHash, blockNumber, index := rawdb.ReadTransaction(s.b.ChainDb(), hash); tx != nil {
		orgTx = newRPCTransaction(tx, blockHash, blockNumber, index)
	} else if tx := s.b.GetPoolTransaction(hash); tx != nil {
		// No finalized transaction, try to retrieve it from the pool
		orgTx = newRPCPendingTransaction(tx)
	} else {
		return TxAndReceipt{}, fmt.Errorf("Tx not found")
	}

	tx, blockHash, blockNumber, index := rawdb.ReadTransaction(s.b.ChainDb(), hash)
	if tx == nil {
		return TxAndReceipt{
			Tx:           orgTx,
			Receipt:      nil,
			ReceiptFound: false,
		}, nil
	}
	receipts, err := s.b.GetReceipts(ctx, blockHash)
	if err != nil {
		return TxAndReceipt{
			Tx:           orgTx,
			Receipt:      nil,
			ReceiptFound: false,
		}, nil
	}
	if len(receipts) <= int(index) {
		return TxAndReceipt{
			Tx:           orgTx,
			Receipt:      nil,
			ReceiptFound: false,
		}, nil
	}
	receipt := receipts[index]

	var signer types.Signer = types.FrontierSigner{}
	if tx.Protected() {
		signer = types.NewEIP155Signer(tx.ChainId())
	}
	from, _ := types.Sender(signer, tx)

	fields := map[string]interface{}{
		"blockHash":         blockHash,
		"blockNumber":       hexutil.Uint64(blockNumber),
		"transactionHash":   hash,
		"transactionIndex":  hexutil.Uint64(index),
		"from":              from,
		"to":                tx.To(),
		"gasUsed":           hexutil.Uint64(receipt.GasUsed),
		"cumulativeGasUsed": hexutil.Uint64(receipt.CumulativeGasUsed),
		"contractAddress":   nil,
		"logs":              receipt.Logs,
		"logsBloom":         receipt.Bloom,
	}

	// Assign receipt status or post state.
	if len(receipt.PostState) > 0 {
		fields["root"] = hexutil.Bytes(receipt.PostState)
	} else {
		fields["status"] = hexutil.Uint(receipt.Status)
	}
	if receipt.Logs == nil {
		fields["logs"] = [][]*types.Log{}
	}
	// If the ContractAddress is 20 0x0 bytes, assume it is not a contract creation
	if receipt.ContractAddress != (common.Address{}) {
		fields["contractAddress"] = receipt.ContractAddress
	}
	return TxAndReceipt{
		Tx:           orgTx,
		Receipt:      fields,
		ReceiptFound: true,
	}, nil
}

// AllInfoForAddress wacom
type AllInfoForAddress struct {
	Tickets   map[common.Hash]common.TicketDisplay `json:"tickets"`
	Balances  map[common.Hash]string               `json:"balances"`
	Timelocks map[common.Hash]*common.TimeLock     `json:"timeLockBalances"`
	Notation  uint64                               `json:"notation"`
}

// AllInfoByAddress wacom
func (s *PublicFusionAPI) AllInfoByAddress(ctx context.Context, address common.Address, blockNr rpc.BlockNumber) (AllInfoForAddress, error) {
	allTickets, err := s.AllTicketsByAddress(ctx, address, blockNr)
	if err != nil {
		return AllInfoForAddress{}, err
	}
	allBalances, err := s.GetAllBalances(ctx, address, blockNr)
	if err != nil {
		return AllInfoForAddress{}, err
	}
	allTimeLockBalances, err := s.GetAllTimeLockBalances(ctx, address, blockNr)
	if err != nil {
		return AllInfoForAddress{}, err
	}
	notation, _ := s.GetNotation(ctx, address, blockNr)

	return AllInfoForAddress{
		Tickets:   allTickets,
		Balances:  allBalances,
		Timelocks: allTimeLockBalances,
		Notation:  notation,
	}, nil
}

func (s *PublicFusionAPI) getIDByTxHash(ctx context.Context, hash common.Hash, logKey string) common.Hash {
	var id common.Hash
	tx, blockHash, _, index := rawdb.ReadTransaction(s.b.ChainDb(), hash)
	if tx == nil {
		return id
	}
	// get from receipt's log
	receipts, err := s.b.GetReceipts(ctx, blockHash)
	if err == nil && len(receipts) > int(index) {
		receipt := receipts[index]

		for _, log := range receipt.Logs {
			if log.Address != common.FSNCallAddress {
				continue
			}
			maps := make(map[string]interface{})
			err := json.Unmarshal(log.Data, &maps)
			if err != nil {
				continue
			}

			if _, hasError := maps["Error"]; hasError {
				continue
			}

			idstr, idok := maps[logKey].(string)
			if idok {
				id = common.HexToHash(idstr)
				return id
			}

		}
	}
	return id
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
	if id := s.getIDByTxHash(ctx, swapID, "SwapID"); id != (common.Hash{}) {
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
	if id := s.getIDByTxHash(ctx, swapID, "SwapID"); id != (common.Hash{}) {
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

type Summary struct {
	TotalMiners  uint64 `json:"totalMiners"`
	TotalTickets uint64 `json:"totalTickets"`
}
type StakeInfo struct {
	StakeInfo map[common.Address]uint64 `json:"stakeInfo"`
	Summary   Summary                   `json:"summary"`
}

// GetMinerInfo wacom
func (s *PublicFusionAPI) GetStakeInfo(ctx context.Context, blockNr rpc.BlockNumber) (StakeInfo, error) {
	stakeInfo := StakeInfo{
		StakeInfo: make(map[common.Address]uint64),
	}
	state, _, err := s.b.StateAndHeaderByNumber(ctx, blockNr)
	if state == nil || err != nil {
		return stakeInfo, fmt.Errorf("Only node using `archive' mode can get history states. error: %v", err)
	}
	tickets, err := state.AllTickets()
	if err == nil {
		err = state.Error()
	}
	if err != nil {
		return stakeInfo, fmt.Errorf("Unable to retrieve all tickets. error: %v", err)
	}
	stakeInfo.Summary.TotalTickets, stakeInfo.Summary.TotalMiners = tickets.NumberOfTicketsAndOwners()
	for _, v := range tickets {
		stakeInfo.StakeInfo[v.Owner] = uint64(len(v.Tickets))
	}
	return stakeInfo, nil
}

//--------------------------------------------- PublicFusionAPI buile send tx args-------------------------------------
func FSNCallArgsToSendTxArgs(args common.FSNBaseArgsInterface, funcType common.FSNCallFunc, funcData []byte) (*SendTxArgs, error) {
	var param = common.FSNCallParam{Func: funcType, Data: funcData}
	data, err := param.ToBytes()
	if err != nil {
		return nil, err
	}
	var argsData = hexutil.Bytes(data)
	baseArgs := args.BaseArgs()
	return &SendTxArgs{
		From:     baseArgs.From,
		To:       &common.FSNCallAddress,
		Gas:      baseArgs.Gas,
		GasPrice: baseArgs.GasPrice,
		Value:    (*hexutil.Big)(big.NewInt(0)),
		Nonce:    baseArgs.Nonce,
		Data:     &argsData,
		Input:    nil,
	}, nil
}

func (s *PublicFusionAPI) BuildGenNotationSendTxArgs(ctx context.Context, args common.FusionBaseArgs) (*SendTxArgs, error) {
	state, _, err := s.b.StateAndHeaderByNumber(ctx, rpc.LatestBlockNumber)
	if state == nil || err != nil {
		return nil, err
	}
	notation := state.GetNotation(args.From)
	if notation != 0 {
		return nil, fmt.Errorf("An address can have only one notation, you already have a mapped notation:%d", notation)
	}

	return FSNCallArgsToSendTxArgs(&args, common.GenNotationFunc, nil)
}

func (s *PublicFusionAPI) BuildGenAssetSendTxArgs(ctx context.Context, args common.GenAssetArgs) (*SendTxArgs, error) {
	if err := args.ToParam().Check(common.BigMaxUint64); err != nil {
		return nil, err
	}

	funcData, err := args.ToData()
	if err != nil {
		return nil, err
	}
	return FSNCallArgsToSendTxArgs(&args, common.GenAssetFunc, funcData)
}

func CheckAndSetToAddress(args *common.SendAssetArgs, state *state.StateDB) error {
	if args.ToUSAN == 0 {
		return nil
	}
	address, err := state.GetAddressByNotation(args.ToUSAN)
	if err != nil {
		return err
	}
	if args.To == (common.Address{}) {
		args.To = address
	} else if args.To != address {
		return fmt.Errorf("'to' and 'toUSAN' conflicts")
	}
	return nil
}

func (s *PublicFusionAPI) BuildSendAssetSendTxArgs(ctx context.Context, args common.SendAssetArgs) (*SendTxArgs, error) {
	state, _, err := s.b.StateAndHeaderByNumber(ctx, rpc.LatestBlockNumber)
	if state == nil || err != nil {
		return nil, err
	}
	if err = CheckAndSetToAddress(&args, state); err != nil {
		return nil, err
	}
	if err := args.ToParam().Check(common.BigMaxUint64); err != nil {
		return nil, err
	}

	if state.GetBalance(args.AssetID, args.From).Cmp(args.Value.ToInt()) < 0 {
		return nil, fmt.Errorf("not enough asset")
	}

	funcData, err := args.ToData()
	if err != nil {
		return nil, err
	}
	return FSNCallArgsToSendTxArgs(&args, common.SendAssetFunc, funcData)
}

func (s *PublicFusionAPI) BuildAssetToTimeLockSendTxArgs(ctx context.Context, args common.TimeLockArgs) (*SendTxArgs, error) {
	state, header, err := s.b.StateAndHeaderByNumber(ctx, rpc.LatestBlockNumber)
	if state == nil || err != nil {
		return nil, err
	}
	if err = CheckAndSetToAddress(&args.SendAssetArgs, state); err != nil {
		return nil, err
	}
	args.Init()
	if err := args.ToParam(common.AssetToTimeLock).Check(common.BigMaxUint64, header.Time.Uint64()); err != nil {
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

	funcData, err := args.ToData(common.AssetToTimeLock)
	if err != nil {
		return nil, err
	}
	return FSNCallArgsToSendTxArgs(&args, common.TimeLockFunc, funcData)
}

func (s *PublicFusionAPI) BuildTimeLockToTimeLockSendTxArgs(ctx context.Context, args common.TimeLockArgs) (*SendTxArgs, error) {
	state, header, err := s.b.StateAndHeaderByNumber(ctx, rpc.LatestBlockNumber)
	if state == nil || err != nil {
		return nil, err
	}
	if err = CheckAndSetToAddress(&args.SendAssetArgs, state); err != nil {
		return nil, err
	}
	args.Init()
	if err := args.ToParam(common.TimeLockToTimeLock).Check(common.BigMaxUint64, header.Time.Uint64()); err != nil {
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

	funcData, err := args.ToData(common.TimeLockToTimeLock)
	if err != nil {
		return nil, err
	}
	return FSNCallArgsToSendTxArgs(&args, common.TimeLockFunc, funcData)
}

func (s *PublicFusionAPI) BuildTimeLockToAssetSendTxArgs(ctx context.Context, args common.TimeLockArgs) (*SendTxArgs, error) {
	state, header, err := s.b.StateAndHeaderByNumber(ctx, rpc.LatestBlockNumber)
	if state == nil || err != nil {
		return nil, err
	}
	if err = CheckAndSetToAddress(&args.SendAssetArgs, state); err != nil {
		return nil, err
	}
	args.Init()
	*(*uint64)(args.StartTime) = header.Time.Uint64()
	*(*uint64)(args.EndTime) = common.TimeLockForever
	if err := args.ToParam(common.TimeLockToAsset).Check(common.BigMaxUint64, header.Time.Uint64()); err != nil {
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

	funcData, err := args.ToData(common.TimeLockToAsset)
	if err != nil {
		return nil, err
	}
	return FSNCallArgsToSendTxArgs(&args, common.TimeLockFunc, funcData)
}

func (s *PublicFusionAPI) BuildBuyTicketSendTxArgs(ctx context.Context, args common.BuyTicketArgs) (*SendTxArgs, error) {
	state, header, err := s.b.StateAndHeaderByNumber(ctx, rpc.LatestBlockNumber)
	if state == nil || err != nil {
		return nil, err
	}

	if doesTicketPurchaseExistsForBlock(header.Number.Int64(), args.From) {
		return nil, fmt.Errorf("Purchase of BuyTicket for this block already submitted")
	}

	now := uint64(time.Now().Unix())
	defStart := header.Time.Uint64()
	if defStart+3600 < now {
		defStart = now
	}
	args.Init(defStart)
	if err := args.ToParam().Check(common.BigMaxUint64, now, 600); err != nil {
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

	funcData, err := args.ToData()
	if err != nil {
		return nil, err
	}
	return FSNCallArgsToSendTxArgs(&args, common.BuyTicketFunc, funcData)
}

func (s *PublicFusionAPI) BuildAssetValueChangeSendTxArgs(ctx context.Context, args common.AssetValueChangeExArgs) (*SendTxArgs, error) {
	state, _, err := s.b.StateAndHeaderByNumber(ctx, rpc.LatestBlockNumber)
	if state == nil || err != nil {
		return nil, err
	}

	if err := args.ToParam().Check(common.BigMaxUint64); err != nil {
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
		return nil, fmt.Errorf("can only be changed by onwer")
	}

	if asset.Owner != args.To && !args.IsInc {
		return nil, fmt.Errorf("decrement can only happen to asset's own account")
	}

	currentBalance := state.GetBalance(args.AssetID, args.To)
	val := args.Value.ToInt()
	if !args.IsInc {
		if currentBalance.Cmp(val) < 0 {
			return nil, fmt.Errorf("not enough asset")
		}
	}

	funcData, err := args.ToData()
	if err != nil {
		return nil, err
	}
	return FSNCallArgsToSendTxArgs(&args, common.AssetValueChangeFunc, funcData)
}

func (s *PublicFusionAPI) BuildMakeSwapSendTxArgs(ctx context.Context, args common.MakeSwapArgs) (*SendTxArgs, error) {
	state, header, err := s.b.StateAndHeaderByNumber(ctx, rpc.LatestBlockNumber)
	if state == nil || err != nil {
		return nil, err
	}

	args.Init()
	now := uint64(time.Now().Unix())
	if err := args.ToParam(header.Time).Check(common.BigMaxUint64, now); err != nil {
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

	funcData, err := args.ToData(header.Time)
	if err != nil {
		return nil, err
	}
	return FSNCallArgsToSendTxArgs(&args, common.MakeSwapFuncExt, funcData)
}

func (s *PublicFusionAPI) BuildRecallSwapSendTxArgs(ctx context.Context, args common.RecallSwapArgs) (*SendTxArgs, error) {
	state, _, err := s.b.StateAndHeaderByNumber(ctx, rpc.LatestBlockNumber)
	if state == nil || err != nil {
		return nil, err
	}

	var swap common.Swap
	swap, err = state.GetSwap(args.SwapID)
	if err != nil {
		return nil, err
	}

	if err := args.ToParam().Check(common.BigMaxUint64, &swap); err != nil {
		return nil, err
	}

	if swap.Owner != args.From {
		return nil, fmt.Errorf("Must be swap onwer can recall")
	}

	funcData, err := args.ToData()
	if err != nil {
		return nil, err
	}
	return FSNCallArgsToSendTxArgs(&args, common.RecallSwapFunc, funcData)
}

func (s *PublicFusionAPI) BuildTakeSwapSendTxArgs(ctx context.Context, args common.TakeSwapArgs) (*SendTxArgs, error) {
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
	if err := args.ToParam().Check(common.BigMaxUint64, &swap, now); err != nil {
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

	funcData, err := args.ToData()
	if err != nil {
		return nil, err
	}
	return FSNCallArgsToSendTxArgs(&args, common.TakeSwapFuncExt, funcData)
}

func (s *PublicFusionAPI) BuildMakeMultiSwapSendTxArgs(ctx context.Context, args common.MakeMultiSwapArgs) (*SendTxArgs, error) {
	state, header, err := s.b.StateAndHeaderByNumber(ctx, rpc.LatestBlockNumber)
	if state == nil || err != nil {
		return nil, err
	}

	args.Init()
	now := uint64(time.Now().Unix())
	if err := args.ToParam(header.Time).Check(common.BigMaxUint64, now); err != nil {
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

	funcData, err := args.ToData(header.Time)
	if err != nil {
		return nil, err
	}
	return FSNCallArgsToSendTxArgs(&args, common.MakeMultiSwapFunc, funcData)
}

func (s *PublicFusionAPI) BuildRecallMultiSwapSendTxArgs(ctx context.Context, args common.RecallMultiSwapArgs) (*SendTxArgs, error) {
	state, _, err := s.b.StateAndHeaderByNumber(ctx, rpc.LatestBlockNumber)
	if state == nil || err != nil {
		return nil, err
	}

	var swap common.MultiSwap
	swap, err = state.GetMultiSwap(args.SwapID)
	if err != nil {
		return nil, err
	}

	if err := args.ToParam().Check(common.BigMaxUint64, &swap); err != nil {
		return nil, err
	}

	if swap.Owner != args.From {
		return nil, fmt.Errorf("Must be swap onwer can recall")
	}

	funcData, err := args.ToData()
	if err != nil {
		return nil, err
	}
	return FSNCallArgsToSendTxArgs(&args, common.RecallMultiSwapFunc, funcData)
}

func (s *PublicFusionAPI) BuildTakeMultiSwapSendTxArgs(ctx context.Context, args common.TakeMultiSwapArgs) (*SendTxArgs, error) {
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
	if err := args.ToParam().Check(common.BigMaxUint64, &swap, now); err != nil {
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

	funcData, err := args.ToData()
	if err != nil {
		return nil, err
	}
	return FSNCallArgsToSendTxArgs(&args, common.TakeMultiSwapFunc, funcData)
}

//--------------------------------------------- PrivateFusionAPI -------------------------------------

// PrivateFusionAPI ss
type PrivateFusionAPI struct {
	PublicFusionAPI
	nonceLock *AddrLocker
	papi      *PrivateAccountAPI
}

// NewPrivateFusionAPI ss
func NewPrivateFusionAPI(b Backend, nonceLock *AddrLocker, papi *PrivateAccountAPI) *PrivateFusionAPI {
	return &PrivateFusionAPI{
		PublicFusionAPI: *NewPublicFusionAPI(b),
		nonceLock:       nonceLock,
		papi:            papi,
	}
}

// GenNotation ss
func (s *PrivateFusionAPI) GenNotation(ctx context.Context, args common.FusionBaseArgs, passwd string) (common.Hash, error) {
	sendArgs, err := s.BuildGenNotationSendTxArgs(ctx, args)
	if err != nil {
		return common.Hash{}, err
	}
	return s.papi.SendTransaction(ctx, *sendArgs, passwd)
}

// GenAsset ss
func (s *PrivateFusionAPI) GenAsset(ctx context.Context, args common.GenAssetArgs, passwd string) (common.Hash, error) {
	sendArgs, err := s.BuildGenAssetSendTxArgs(ctx, args)
	if err != nil {
		return common.Hash{}, err
	}
	return s.papi.SendTransaction(ctx, *sendArgs, passwd)
}

// SendAsset ss
func (s *PrivateFusionAPI) SendAsset(ctx context.Context, args common.SendAssetArgs, passwd string) (common.Hash, error) {
	sendArgs, err := s.BuildSendAssetSendTxArgs(ctx, args)
	if err != nil {
		return common.Hash{}, err
	}
	return s.papi.SendTransaction(ctx, *sendArgs, passwd)
}

// AssetToTimeLock ss
func (s *PrivateFusionAPI) AssetToTimeLock(ctx context.Context, args common.TimeLockArgs, passwd string) (common.Hash, error) {
	sendArgs, err := s.BuildAssetToTimeLockSendTxArgs(ctx, args)
	if err != nil {
		return common.Hash{}, err
	}
	return s.papi.SendTransaction(ctx, *sendArgs, passwd)
}

// TimeLockToTimeLock ss
func (s *PrivateFusionAPI) TimeLockToTimeLock(ctx context.Context, args common.TimeLockArgs, passwd string) (common.Hash, error) {
	sendArgs, err := s.BuildTimeLockToTimeLockSendTxArgs(ctx, args)
	if err != nil {
		return common.Hash{}, err
	}
	return s.papi.SendTransaction(ctx, *sendArgs, passwd)
}

// TimeLockToAsset ss
func (s *PrivateFusionAPI) TimeLockToAsset(ctx context.Context, args common.TimeLockArgs, passwd string) (common.Hash, error) {
	sendArgs, err := s.BuildTimeLockToAssetSendTxArgs(ctx, args)
	if err != nil {
		return common.Hash{}, err
	}
	return s.papi.SendTransaction(ctx, *sendArgs, passwd)
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
func (s *PrivateFusionAPI) BuyTicket(ctx context.Context, args common.BuyTicketArgs, passwd string) (common.Hash, error) {
	sendArgs, err := s.BuildBuyTicketSendTxArgs(ctx, args)
	if err != nil {
		return common.Hash{}, err
	}
	hash, err := s.papi.SendTransaction(ctx, *sendArgs, passwd)
	if err != nil {
		return common.Hash{}, err
	}
	addTicketPurchaseForBlock(args.From)
	return hash, err
}

// IncAsset ss
func (s *PrivateFusionAPI) IncAsset(ctx context.Context, args common.AssetValueChangeExArgs, passwd string) (common.Hash, error) {
	args.IsInc = true
	sendArgs, err := s.BuildAssetValueChangeSendTxArgs(ctx, args)
	if err != nil {
		return common.Hash{}, err
	}
	return s.papi.SendTransaction(ctx, *sendArgs, passwd)
}

// DecAsset ss
func (s *PrivateFusionAPI) DecAsset(ctx context.Context, args common.AssetValueChangeExArgs, passwd string) (common.Hash, error) {
	args.IsInc = false
	sendArgs, err := s.BuildAssetValueChangeSendTxArgs(ctx, args)
	if err != nil {
		return common.Hash{}, err
	}
	return s.papi.SendTransaction(ctx, *sendArgs, passwd)
}

// MakeSwap ss
func (s *PrivateFusionAPI) MakeSwap(ctx context.Context, args common.MakeSwapArgs, passwd string) (common.Hash, error) {
	sendArgs, err := s.BuildMakeSwapSendTxArgs(ctx, args)
	if err != nil {
		return common.Hash{}, err
	}
	return s.papi.SendTransaction(ctx, *sendArgs, passwd)
}

// RecallSwap ss
func (s *PrivateFusionAPI) RecallSwap(ctx context.Context, args common.RecallSwapArgs, passwd string) (common.Hash, error) {
	sendArgs, err := s.BuildRecallSwapSendTxArgs(ctx, args)
	if err != nil {
		return common.Hash{}, err
	}
	return s.papi.SendTransaction(ctx, *sendArgs, passwd)
}

// TakeSwap ss
func (s *PrivateFusionAPI) TakeSwap(ctx context.Context, args common.TakeSwapArgs, passwd string) (common.Hash, error) {
	sendArgs, err := s.BuildTakeSwapSendTxArgs(ctx, args)
	if err != nil {
		return common.Hash{}, err
	}
	return s.papi.SendTransaction(ctx, *sendArgs, passwd)
}

// MakeMultiSwap ss
func (s *PrivateFusionAPI) MakeMultiSwap(ctx context.Context, args common.MakeMultiSwapArgs, passwd string) (common.Hash, error) {
	sendArgs, err := s.BuildMakeMultiSwapSendTxArgs(ctx, args)
	if err != nil {
		return common.Hash{}, err
	}
	return s.papi.SendTransaction(ctx, *sendArgs, passwd)
}

// RecallMultiSwap ss
func (s *PrivateFusionAPI) RecallMultiSwap(ctx context.Context, args common.RecallMultiSwapArgs, passwd string) (common.Hash, error) {
	sendArgs, err := s.BuildRecallMultiSwapSendTxArgs(ctx, args)
	if err != nil {
		return common.Hash{}, err
	}
	return s.papi.SendTransaction(ctx, *sendArgs, passwd)
}

// TakeMultiSwap ss
func (s *PrivateFusionAPI) TakeMultiSwap(ctx context.Context, args common.TakeMultiSwapArgs, passwd string) (common.Hash, error) {
	sendArgs, err := s.BuildTakeMultiSwapSendTxArgs(ctx, args)
	if err != nil {
		return common.Hash{}, err
	}
	return s.papi.SendTransaction(ctx, *sendArgs, passwd)
}

//--------------------------------------------- FusionTransactionAPI -------------------------------------

// FusionTransactionAPI ss
type FusionTransactionAPI struct {
	PublicFusionAPI
	nonceLock *AddrLocker
	txapi     *PublicTransactionPoolAPI
}

var fusionTransactionAPI *FusionTransactionAPI

// NewFusionTransactionAPI ss
func NewFusionTransactionAPI(b Backend, nonceLock *AddrLocker, txapi *PublicTransactionPoolAPI) *FusionTransactionAPI {
	fusionTransactionAPI = &FusionTransactionAPI{
		PublicFusionAPI: *NewPublicFusionAPI(b),
		nonceLock:       nonceLock,
		txapi:           txapi,
	}
	return fusionTransactionAPI
}

// auto buy ticket
func AutoBuyTicket(enable bool) {
	if enable {
		_, err := fusionTransactionAPI.b.Coinbase()
		if err != nil {
			log.Warn("AutoBuyTicket not enabled as no coinbase account exist")
			enable = false
		}
	}
	common.AutoBuyTicket = enable

	for {
		<-common.AutoBuyTicketChan
	COMSUMEALL:
		for {
			select {
			case <-common.AutoBuyTicketChan:
			default:
				break COMSUMEALL
			}
		}

		// prevent auto buy ticket in syncing
		if !fusionTransactionAPI.b.IsMining() {
			common.DebugInfo("ignore AutoBuyTicket as isMining is false")
			continue
		}

		coinbase, err := fusionTransactionAPI.b.Coinbase()
		if err == nil {
			fbase := common.FusionBaseArgs{From: coinbase}
			args := common.BuyTicketArgs{FusionBaseArgs: fbase}
			fusionTransactionAPI.BuyTicket(context.TODO(), args)
		}
	}
}

// StartAutoBuyTicket ss
func (s *FusionTransactionAPI) StartAutoBuyTicket() error {
	if _, err := fusionTransactionAPI.b.Coinbase(); err != nil {
		return fmt.Errorf("StartAutoBuyTicket Error: coinbase not exist")
	}
	common.AutoBuyTicket = true
	return nil
}

// StopAutoBuyTicket ss
func (s *FusionTransactionAPI) StopAutoBuyTicket() {
	common.AutoBuyTicket = false
}

// report illegal
func ReportIllegal() {
	for {
		select {
		case content := <-common.ReportIllegalChan:
			coinbase, err := fusionTransactionAPI.b.Coinbase()
			if err == nil {
				args := common.FusionBaseArgs{From: coinbase}
				fusionTransactionAPI.ReportIllegal(context.TODO(), args, content)
			}
		}
	}
}

func (s *FusionTransactionAPI) ReportIllegal(ctx context.Context, args common.FusionBaseArgs, content []byte) (common.Hash, error) {
	oldtx := s.b.GetPoolTransactionByPredicate(func(tx *types.Transaction) bool {
		param := common.FSNCallParam{}
		rlp.DecodeBytes(tx.Data(), &param)
		return param.Func == common.ReportIllegalFunc && bytes.Equal(param.Data, content)
	})
	if oldtx != nil {
		return common.Hash{}, fmt.Errorf("ReportIllegal: already reported in txpool")
	}
	sendArgs, err := FSNCallArgsToSendTxArgs(&args, common.ReportIllegalFunc, content)
	if err != nil {
		return common.Hash{}, err
	}
	tx, err := s.buildTransaction(ctx, *sendArgs)
	if err != nil {
		return common.Hash{}, err
	}
	return s.sendTransaction(ctx, args.From, tx)
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
func (s *FusionTransactionAPI) BuildGenNotationTx(ctx context.Context, args common.FusionBaseArgs) (*types.Transaction, error) {
	sendArgs, err := s.BuildGenNotationSendTxArgs(ctx, args)
	if err != nil {
		return nil, err
	}
	return s.buildTransaction(ctx, *sendArgs)
}

// GenNotation ss
func (s *FusionTransactionAPI) GenNotation(ctx context.Context, args common.FusionBaseArgs) (common.Hash, error) {
	tx, err := s.BuildGenNotationTx(ctx, args)
	if err != nil {
		return common.Hash{}, err
	}
	return s.sendTransaction(ctx, args.From, tx)
}

// BuildGenAssetTx ss
func (s *FusionTransactionAPI) BuildGenAssetTx(ctx context.Context, args common.GenAssetArgs) (*types.Transaction, error) {
	sendArgs, err := s.BuildGenAssetSendTxArgs(ctx, args)
	if err != nil {
		return nil, err
	}
	return s.buildTransaction(ctx, *sendArgs)
}

// GenAsset ss
func (s *FusionTransactionAPI) GenAsset(ctx context.Context, args common.GenAssetArgs) (common.Hash, error) {
	tx, err := s.BuildGenAssetTx(ctx, args)
	if err != nil {
		return common.Hash{}, err
	}
	return s.sendTransaction(ctx, args.From, tx)
}

// BuildSendAssetTx ss
func (s *FusionTransactionAPI) BuildSendAssetTx(ctx context.Context, args common.SendAssetArgs) (*types.Transaction, error) {
	sendArgs, err := s.BuildSendAssetSendTxArgs(ctx, args)
	if err != nil {
		return nil, err
	}
	return s.buildTransaction(ctx, *sendArgs)
}

// SendAsset ss
func (s *FusionTransactionAPI) SendAsset(ctx context.Context, args common.SendAssetArgs) (common.Hash, error) {
	tx, err := s.BuildSendAssetTx(ctx, args)
	if err != nil {
		return common.Hash{}, err
	}
	return s.sendTransaction(ctx, args.From, tx)
}

// BuildAssetToTimeLockTx ss
func (s *FusionTransactionAPI) BuildAssetToTimeLockTx(ctx context.Context, args common.TimeLockArgs) (*types.Transaction, error) {
	sendArgs, err := s.BuildAssetToTimeLockSendTxArgs(ctx, args)
	if err != nil {
		return nil, err
	}
	return s.buildTransaction(ctx, *sendArgs)
}

// AssetToTimeLock ss
func (s *FusionTransactionAPI) AssetToTimeLock(ctx context.Context, args common.TimeLockArgs) (common.Hash, error) {
	tx, err := s.BuildAssetToTimeLockTx(ctx, args)
	if err != nil {
		return common.Hash{}, err
	}
	return s.sendTransaction(ctx, args.From, tx)
}

// BuildTimeLockToTimeLockTx ss
func (s *FusionTransactionAPI) BuildTimeLockToTimeLockTx(ctx context.Context, args common.TimeLockArgs) (*types.Transaction, error) {
	sendArgs, err := s.BuildTimeLockToTimeLockSendTxArgs(ctx, args)
	if err != nil {
		return nil, err
	}
	return s.buildTransaction(ctx, *sendArgs)
}

// TimeLockToTimeLock ss
func (s *FusionTransactionAPI) TimeLockToTimeLock(ctx context.Context, args common.TimeLockArgs) (common.Hash, error) {
	tx, err := s.BuildTimeLockToTimeLockTx(ctx, args)
	if err != nil {
		return common.Hash{}, err
	}
	return s.sendTransaction(ctx, args.From, tx)
}

// BuildTimeLockToAssetTx ss
func (s *FusionTransactionAPI) BuildTimeLockToAssetTx(ctx context.Context, args common.TimeLockArgs) (*types.Transaction, error) {
	sendArgs, err := s.BuildTimeLockToAssetSendTxArgs(ctx, args)
	if err != nil {
		return nil, err
	}
	return s.buildTransaction(ctx, *sendArgs)
}

// TimeLockToAsset ss
func (s *FusionTransactionAPI) TimeLockToAsset(ctx context.Context, args common.TimeLockArgs) (common.Hash, error) {
	tx, err := s.BuildTimeLockToAssetTx(ctx, args)
	if err != nil {
		return common.Hash{}, err
	}
	return s.sendTransaction(ctx, args.From, tx)
}

// BuildBuyTicketTx ss
func (s *FusionTransactionAPI) BuildBuyTicketTx(ctx context.Context, args common.BuyTicketArgs) (*types.Transaction, error) {
	sendArgs, err := s.BuildBuyTicketSendTxArgs(ctx, args)
	if err != nil {
		return nil, err
	}
	return s.buildTransaction(ctx, *sendArgs)
}

// BuyTicket ss
func (s *FusionTransactionAPI) BuyTicket(ctx context.Context, args common.BuyTicketArgs) (common.Hash, error) {
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

// BuildIncAssetTx ss
func (s *FusionTransactionAPI) BuildIncAssetTx(ctx context.Context, args common.AssetValueChangeExArgs) (*types.Transaction, error) {
	args.IsInc = true
	sendArgs, err := s.BuildAssetValueChangeSendTxArgs(ctx, args)
	if err != nil {
		return nil, err
	}
	return s.buildTransaction(ctx, *sendArgs)
}

// IncAsset ss
func (s *FusionTransactionAPI) IncAsset(ctx context.Context, args common.AssetValueChangeExArgs) (common.Hash, error) {
	tx, err := s.BuildIncAssetTx(ctx, args)
	if err != nil {
		return common.Hash{}, err
	}
	return s.sendTransaction(ctx, args.From, tx)
}

// BuildDecAssetTx ss
func (s *FusionTransactionAPI) BuildDecAssetTx(ctx context.Context, args common.AssetValueChangeExArgs) (*types.Transaction, error) {
	args.IsInc = false
	sendArgs, err := s.BuildAssetValueChangeSendTxArgs(ctx, args)
	if err != nil {
		return nil, err
	}
	return s.buildTransaction(ctx, *sendArgs)
}

// DecAsset ss
func (s *FusionTransactionAPI) DecAsset(ctx context.Context, args common.AssetValueChangeExArgs) (common.Hash, error) {
	tx, err := s.BuildDecAssetTx(ctx, args)
	if err != nil {
		return common.Hash{}, err
	}
	return s.sendTransaction(ctx, args.From, tx)
}

// BuildMakeSwapTx ss
func (s *FusionTransactionAPI) BuildMakeSwapTx(ctx context.Context, args common.MakeSwapArgs) (*types.Transaction, error) {
	sendArgs, err := s.BuildMakeSwapSendTxArgs(ctx, args)
	if err != nil {
		return nil, err
	}
	return s.buildTransaction(ctx, *sendArgs)
}

// MakeSwap ss
func (s *FusionTransactionAPI) MakeSwap(ctx context.Context, args common.MakeSwapArgs) (common.Hash, error) {
	tx, err := s.BuildMakeSwapTx(ctx, args)
	if err != nil {
		return common.Hash{}, err
	}
	return s.sendTransaction(ctx, args.From, tx)
}

// BuildRecallSwapTx ss
func (s *FusionTransactionAPI) BuildRecallSwapTx(ctx context.Context, args common.RecallSwapArgs) (*types.Transaction, error) {
	sendArgs, err := s.BuildRecallSwapSendTxArgs(ctx, args)
	if err != nil {
		return nil, err
	}
	return s.buildTransaction(ctx, *sendArgs)
}

// RecallSwap ss
func (s *FusionTransactionAPI) RecallSwap(ctx context.Context, args common.RecallSwapArgs) (common.Hash, error) {
	tx, err := s.BuildRecallSwapTx(ctx, args)
	if err != nil {
		return common.Hash{}, err
	}
	return s.sendTransaction(ctx, args.From, tx)
}

// BuildTakeSwapTx ss
func (s *FusionTransactionAPI) BuildTakeSwapTx(ctx context.Context, args common.TakeSwapArgs) (*types.Transaction, error) {
	sendArgs, err := s.BuildTakeSwapSendTxArgs(ctx, args)
	if err != nil {
		return nil, err
	}
	return s.buildTransaction(ctx, *sendArgs)
}

// TakeSwap ss
func (s *FusionTransactionAPI) TakeSwap(ctx context.Context, args common.TakeSwapArgs) (common.Hash, error) {
	tx, err := s.BuildTakeSwapTx(ctx, args)
	if err != nil {
		return common.Hash{}, err
	}
	return s.sendTransaction(ctx, args.From, tx)
}

// MakeMultiSwap wacom
func (s *FusionTransactionAPI) MakeMultiSwap(ctx context.Context, args common.MakeMultiSwapArgs) (common.Hash, error) {
	tx, err := s.BuildMakeMultiSwapTx(ctx, args)
	if err != nil {
		return common.Hash{}, err
	}
	return s.sendTransaction(ctx, args.From, tx)
}

// BuildMakeMultiSwapTx ss
func (s *FusionTransactionAPI) BuildMakeMultiSwapTx(ctx context.Context, args common.MakeMultiSwapArgs) (*types.Transaction, error) {
	sendArgs, err := s.BuildMakeMultiSwapSendTxArgs(ctx, args)
	if err != nil {
		return nil, err
	}
	return s.buildTransaction(ctx, *sendArgs)
}

// RecallMultiSwap wacom
func (s *FusionTransactionAPI) RecallMultiSwap(ctx context.Context, args common.RecallMultiSwapArgs) (common.Hash, error) {
	tx, err := s.BuildRecallMultiSwapTx(ctx, args)
	if err != nil {
		return common.Hash{}, err
	}
	return s.sendTransaction(ctx, args.From, tx)
}

// BuildRecallMultiSwapTx ss
func (s *FusionTransactionAPI) BuildRecallMultiSwapTx(ctx context.Context, args common.RecallMultiSwapArgs) (*types.Transaction, error) {
	sendArgs, err := s.BuildRecallMultiSwapSendTxArgs(ctx, args)
	if err != nil {
		return nil, err
	}
	return s.buildTransaction(ctx, *sendArgs)
}

// TakeMultiSwap wacom
func (s *FusionTransactionAPI) TakeMultiSwap(ctx context.Context, args common.TakeMultiSwapArgs) (common.Hash, error) {
	tx, err := s.BuildTakeMultiSwapTx(ctx, args)
	if err != nil {
		return common.Hash{}, err
	}
	return s.sendTransaction(ctx, args.From, tx)
}

// BuildTakeSwapTx ss
func (s *FusionTransactionAPI) BuildTakeMultiSwapTx(ctx context.Context, args common.TakeMultiSwapArgs) (*types.Transaction, error) {
	sendArgs, err := s.BuildTakeMultiSwapSendTxArgs(ctx, args)
	if err != nil {
		return nil, err
	}
	return s.buildTransaction(ctx, *sendArgs)
}
