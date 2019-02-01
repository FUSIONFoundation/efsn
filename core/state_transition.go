// Copyright 2014 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"reflect"
	"time"

	"github.com/FusionFoundation/efsn/common"
	"github.com/FusionFoundation/efsn/core/types"
	"github.com/FusionFoundation/efsn/core/vm"
	"github.com/FusionFoundation/efsn/crypto"
	"github.com/FusionFoundation/efsn/log"
	"github.com/FusionFoundation/efsn/params"
	"github.com/FusionFoundation/efsn/rlp"
)

var (
	errInsufficientBalanceForGas = errors.New("insufficient balance to pay for gas")
)

/*
The State Transitioning Model

A state transition is a change made when a transaction is applied to the current world state
The state transitioning model does all the necessary work to work out a valid new state root.

1) Nonce handling
2) Pre pay gas
3) Create a new state object if the recipient is \0*32
4) Value transfer
== If contract creation ==
  4a) Attempt to run transaction data
  4b) If valid, use result as code for the new state object
== end ==
5) Run Script section
6) Derive new state root
*/
type StateTransition struct {
	gp         *GasPool
	msg        Message
	gas        uint64
	gasPrice   *big.Int
	initialGas uint64
	value      *big.Int
	data       []byte
	state      vm.StateDB
	evm        *vm.EVM
}

// Message represents a message sent to a contract.
type Message interface {
	From() common.Address
	//FromFrontier() (common.Address, error)
	To() *common.Address

	GasPrice() *big.Int
	Gas() uint64
	Value() *big.Int

	Nonce() uint64
	CheckNonce() bool
	Data() []byte

	AsTransaction() *types.Transaction
}

// IntrinsicGas computes the 'intrinsic gas' for a message with the given data.
func IntrinsicGas(data []byte, contractCreation, homestead bool) (uint64, error) {
	// Set the starting gas for the raw transaction
	var gas uint64
	if contractCreation && homestead {
		gas = params.TxGasContractCreation
	} else {
		gas = params.TxGas
	}
	// Bump the required gas by the amount of transactional data
	if len(data) > 0 {
		// Zero and non-zero bytes are priced differently
		var nz uint64
		for _, byt := range data {
			if byt != 0 {
				nz++
			}
		}
		// Make sure we don't exceed uint64 for all data combinations
		if (math.MaxUint64-gas)/params.TxDataNonZeroGas < nz {
			return 0, vm.ErrOutOfGas
		}
		gas += nz * params.TxDataNonZeroGas

		z := uint64(len(data)) - nz
		if (math.MaxUint64-gas)/params.TxDataZeroGas < z {
			return 0, vm.ErrOutOfGas
		}
		gas += z * params.TxDataZeroGas
	}
	return gas, nil
}

// NewStateTransition initialises and returns a new state transition object.
func NewStateTransition(evm *vm.EVM, msg Message, gp *GasPool) *StateTransition {
	return &StateTransition{
		gp:       gp,
		evm:      evm,
		msg:      msg,
		gasPrice: msg.GasPrice(),
		value:    msg.Value(),
		data:     msg.Data(),
		state:    evm.StateDB,
	}
}

// ApplyMessage computes the new state by applying the given message
// against the old state within the environment.
//
// ApplyMessage returns the bytes returned by any EVM execution (if it took place),
// the gas used (which includes gas refunds) and an error if it failed. An error always
// indicates a core error meaning that the message would always fail for that particular
// state and would never be accepted within a block.
func ApplyMessage(evm *vm.EVM, msg Message, gp *GasPool) ([]byte, uint64, bool, error) {
	return NewStateTransition(evm, msg, gp).TransitionDb()
}

// to returns the recipient of the message.
func (st *StateTransition) to() common.Address {
	if st.msg == nil || st.msg.To() == nil /* contract creation */ {
		return common.Address{}
	}
	return *st.msg.To()
}

func (st *StateTransition) useGas(amount uint64) error {
	if st.gas < amount {
		return vm.ErrOutOfGas
	}
	st.gas -= amount

	return nil
}

func (st *StateTransition) buyGas() error {
	mgval := new(big.Int).Mul(new(big.Int).SetUint64(st.msg.Gas()), st.gasPrice)
	if st.state.GetBalance(common.SystemAssetID, st.msg.From()).Cmp(mgval) < 0 {
		return errInsufficientBalanceForGas
	}
	if err := st.gp.SubGas(st.msg.Gas()); err != nil {
		return err
	}
	st.gas += st.msg.Gas()

	st.initialGas = st.msg.Gas()
	st.state.SubBalance(st.msg.From(), common.SystemAssetID, mgval)
	return nil
}

func (st *StateTransition) preCheck() error {
	// Make sure this transaction's nonce is correct.
	if st.msg.CheckNonce() {
		nonce := st.state.GetNonce(st.msg.From())
		if nonce < st.msg.Nonce() {
			return ErrNonceTooHigh
		} else if nonce > st.msg.Nonce() {
			return ErrNonceTooLow
		}
	}
	return st.buyGas()
}

// TransitionDb will transition the state by applying the current message and
// returning the result including the used gas. It returns an error if failed.
// An error indicates a consensus issue.
func (st *StateTransition) TransitionDb() (ret []byte, usedGas uint64, failed bool, err error) {
	if err = st.preCheck(); err != nil {
		return
	}
	msg := st.msg
	sender := vm.AccountRef(msg.From())
	homestead := st.evm.ChainConfig().IsHomestead(st.evm.BlockNumber)
	contractCreation := msg.To() == nil

	// Pay intrinsic gas
	gas, err := IntrinsicGas(st.data, contractCreation, homestead)
	if err != nil {
		return nil, 0, false, err
	}
	if err = st.useGas(gas); err != nil {
		return nil, 0, false, err
	}

	var (
		evm = st.evm
		// vm errors do not effect consensus and are therefor
		// not assigned to err, except for insufficient balance
		// error.
		vmerr error
	)
	if contractCreation {
		ret, _, st.gas, vmerr = evm.Create(sender, st.data, st.gas, st.value)
	} else {
		// Increment the nonce for the next transaction
		st.state.SetNonce(msg.From(), st.state.GetNonce(sender.Address())+1)

		if st.msg.To() != nil && *st.msg.To() == common.FSNCallAddress {
			st.handleFsnCall()
		}

		ret, st.gas, vmerr = evm.Call(sender, st.to(), st.data, st.gas, st.value)
	}
	if vmerr != nil {
		log.Debug("VM returned with error", "err", vmerr)
		// The only possible consensus-error would be if there wasn't
		// sufficient balance to make the transfer happen. The first
		// balance transfer may never fail.
		if vmerr == vm.ErrInsufficientBalance {
			return nil, 0, false, vmerr
		}
	}
	st.refundGas()
	st.state.AddBalance(st.evm.Coinbase, common.SystemAssetID, new(big.Int).Mul(new(big.Int).SetUint64(st.gasUsed()), st.gasPrice))

	return ret, st.gasUsed(), vmerr != nil, err
}

func (st *StateTransition) refundGas() {
	// Apply refund counter, capped to half of the used gas.
	refund := st.gasUsed() / 2
	if refund > st.state.GetRefund() {
		refund = st.state.GetRefund()
	}
	st.gas += refund

	// Return ETH for remaining gas, exchanged at the original rate.
	remaining := new(big.Int).Mul(new(big.Int).SetUint64(st.gas), st.gasPrice)
	st.state.AddBalance(st.msg.From(), common.SystemAssetID, remaining)

	// Also return remaining gas to the block gas counter so it is
	// available for the next transaction.
	st.gp.AddGas(st.gas)
}

// gasUsed returns the amount of gas used up by the state transition.
func (st *StateTransition) gasUsed() uint64 {
	return st.initialGas - st.gas
}

var outputCommands = true

func outputCommandInfo(param1 string, param2 string, param3 interface{}) {
	if outputCommands {
		log.Info(param1, param2, param3)
	}
}

func (st *StateTransition) handleFsnCall() error {
	param := common.FSNCallParam{}
	rlp.DecodeBytes(st.msg.Data(), &param)
	switch param.Func {
	case common.GenNotationFunc:
		outputCommandInfo("GenNotationFunc", "from", st.msg.From())
		err := st.state.GenNotation(st.msg.From())
		if err == nil {
			st.addLog(common.GenNotationFunc, []byte{})
		}
		return err
	case common.GenAssetFunc:
		outputCommandInfo("GenAssetFunc", "from", st.msg.From())
		genAssetParam := common.GenAssetParam{}
		rlp.DecodeBytes(param.Data, &genAssetParam)
		asset := genAssetParam.ToAsset()
		asset.ID = st.msg.AsTransaction().Hash()
		asset.Owner = st.msg.From()
		big0 := big.NewInt(0)

		if len(genAssetParam.Name) == 0 || len(genAssetParam.Symbol) == 0 || genAssetParam.Total == nil || genAssetParam.Total.Cmp(big0) <= 0 {
			log.Info("GenAssetFunc name, symbol and total must be set")
			st.addLog(common.GenAssetFunc, genAssetParam, common.NewKeyValue("Error", "GenAssetFunc name, symbol and total must be set"))
			return fmt.Errorf("BuildGenAsset name, symbol and total must be set or greater than 0")
		}

		if genAssetParam.Decimals > 18 || int(genAssetParam.Decimals) < 0 {
			st.addLog(common.GenAssetFunc, genAssetParam, common.NewKeyValue("Error", "GenAssetFunc decimals must be between 0 and 18"))
			return fmt.Errorf("GenAssetFunc decimals must be between 0 and 18")
		}
		if err := st.state.GenAsset(asset); err != nil {
			st.addLog(common.GenAssetFunc, genAssetParam, common.NewKeyValue("Error", "unable to gen asset"))
			return err
		}
		st.state.AddBalance(st.msg.From(), asset.ID, asset.Total)
		st.addLog(common.GenAssetFunc, genAssetParam, common.NewKeyValue("AssetID", asset.ID))
		return nil
	case common.SendAssetFunc:
		outputCommandInfo("SendAssetFunc", "from", st.msg.From())
		sendAssetParam := common.SendAssetParam{}
		rlp.DecodeBytes(param.Data, &sendAssetParam)
		if st.state.GetBalance(sendAssetParam.AssetID, st.msg.From()).Cmp(sendAssetParam.Value) < 0 {
			st.addLog(common.SendAssetFunc, sendAssetParam, common.NewKeyValue("Error", "not enough asset"))
			return fmt.Errorf("not enough asset")
		}
		st.state.SubBalance(st.msg.From(), sendAssetParam.AssetID, sendAssetParam.Value)
		st.state.AddBalance(sendAssetParam.To, sendAssetParam.AssetID, sendAssetParam.Value)
		st.addLog(common.SendAssetFunc, sendAssetParam, common.NewKeyValue("AssetID", sendAssetParam.AssetID))
		return nil
	case common.TimeLockFunc:
		outputCommandInfo("TimeLockFunc", "from", st.msg.From())
		timeLockParam := common.TimeLockParam{}
		rlp.DecodeBytes(param.Data, &timeLockParam)
		if timeLockParam.Type == common.TimeLockToAsset {
			if timeLockParam.StartTime > uint64(time.Now().Unix()) {
				st.addLog(common.TimeLockFunc, timeLockParam, common.NewKeyValue("LockType", "TimeLockToAsset"), common.NewKeyValue("Error", "Start time must be more than now"))
				return fmt.Errorf("Start time must be more than now")
			}
			timeLockParam.EndTime = common.TimeLockForever
		}
		needValue := common.NewTimeLock(&common.TimeLockItem{
			StartTime: timeLockParam.StartTime,
			EndTime:   timeLockParam.EndTime,
			Value:     new(big.Int).SetBytes(timeLockParam.Value.Bytes()),
		})
		switch timeLockParam.Type {
		case common.AssetToTimeLock:
			if st.state.GetBalance(timeLockParam.AssetID, st.msg.From()).Cmp(timeLockParam.Value) < 0 {
				st.addLog(common.TimeLockFunc, timeLockParam, common.NewKeyValue("LockType", "AssetToTimeLock"), common.NewKeyValue("Error", "not enough asset"))
				return fmt.Errorf("not enough asset")
			}
			st.state.SubBalance(st.msg.From(), timeLockParam.AssetID, timeLockParam.Value)
			totalValue := common.NewTimeLock(&common.TimeLockItem{
				StartTime: common.TimeLockNow,
				EndTime:   common.TimeLockForever,
				Value:     new(big.Int).SetBytes(timeLockParam.Value.Bytes()),
			})
			surplusValue := new(common.TimeLock).Sub(totalValue, needValue)
			if !surplusValue.IsEmpty() {
				st.state.AddTimeLockBalance(st.msg.From(), timeLockParam.AssetID, surplusValue)
			}
			st.state.AddTimeLockBalance(timeLockParam.To, timeLockParam.AssetID, needValue)
			st.addLog(common.TimeLockFunc, timeLockParam, common.NewKeyValue("LockType", "AssetToTimeLock"), common.NewKeyValue("AssetID", timeLockParam.AssetID))
			return nil
		case common.TimeLockToTimeLock:
			if st.state.GetTimeLockBalance(timeLockParam.AssetID, st.msg.From()).Cmp(needValue) < 0 {
				st.addLog(common.TimeLockFunc, timeLockParam, common.NewKeyValue("LockType", "TimeLockToTimeLock"), common.NewKeyValue("Error", "not enough time lock balance"))
				return fmt.Errorf("not enough time lock balance")
			}
			st.state.SubTimeLockBalance(st.msg.From(), timeLockParam.AssetID, needValue)
			st.state.AddTimeLockBalance(timeLockParam.To, timeLockParam.AssetID, needValue)
			st.addLog(common.TimeLockFunc, timeLockParam, common.NewKeyValue("LockType", "TimeLockToTimeLock"), common.NewKeyValue("AssetID", timeLockParam.AssetID))
			return nil
		case common.TimeLockToAsset:
			if st.state.GetTimeLockBalance(timeLockParam.AssetID, st.msg.From()).Cmp(needValue) < 0 {
				st.addLog(common.TimeLockFunc, timeLockParam, common.NewKeyValue("LockType", "TimeLockToAsset"), common.NewKeyValue("Error", "not enough time lock balance"))
				return fmt.Errorf("not enough time lock balance")
			}
			st.state.SubTimeLockBalance(st.msg.From(), timeLockParam.AssetID, needValue)
			st.state.AddBalance(timeLockParam.To, timeLockParam.AssetID, timeLockParam.Value)
			st.addLog(common.TimeLockFunc, timeLockParam, common.NewKeyValue("LockType", "TimeLockToAsset"), common.NewKeyValue("AssetID", timeLockParam.AssetID))
			return nil
		}
	case common.BuyTicketFunc:
		outputCommandInfo("BuyTicketFunc", "from", st.msg.From())
		// log.Info( "Buy a ticket func called")
		from := st.msg.From()
		height := st.evm.Context.BlockNumber
		hash := st.evm.GetHash(height.Uint64() - 1)
		id := crypto.Keccak256Hash(from[:], hash[:])

		tickets, err := st.state.AllTickets()

		if err != nil {
			st.addLog(common.BuyTicketFunc, param.Data, common.NewKeyValue("Error", "unable to retrieve previous tickets"))
			log.Debug("BuyTicketFunc unable to retrieve previous tickets")
			return err
		}

		if _, ok := tickets[id]; ok {
			st.addLog(common.BuyTicketFunc, param.Data, common.NewKeyValue("Error", "one block just can buy one ticket"))
			return fmt.Errorf("one block just can buy one ticket")
		}

		buyTicketParam := common.BuyTicketParam{}
		rlp.DecodeBytes(param.Data, &buyTicketParam)

		start := buyTicketParam.Start
		end := buyTicketParam.End
		value := common.TicketPrice()
		needValue := common.NewTimeLock(&common.TimeLockItem{
			StartTime: start,
			EndTime:   end,
			Value:     value,
		})
		useAsset := false
		if st.state.GetTimeLockBalance(common.SystemAssetID, from).Cmp(needValue) < 0 {
			if st.state.GetBalance(common.SystemAssetID, from).Cmp(value) < 0 {
				st.addLog(common.BuyTicketFunc, param.Data, common.NewKeyValue("Error", "not enough time lock or asset balance"))
				return fmt.Errorf("not enough time lock or asset balance")
			}
			useAsset = true
		}

		if useAsset {
			st.state.SubBalance(from, common.SystemAssetID, value)
			totalValue := common.NewTimeLock(&common.TimeLockItem{
				StartTime: common.TimeLockNow,
				EndTime:   common.TimeLockForever,
				Value:     value,
			})
			surplusValue := new(common.TimeLock).Sub(totalValue, needValue)
			if !surplusValue.IsEmpty() {
				st.state.AddTimeLockBalance(from, common.SystemAssetID, surplusValue)
			}
		} else {
			st.state.SubTimeLockBalance(from, common.SystemAssetID, needValue)
		}

		ticket := common.Ticket{
			ID:         id,
			Owner:      from,
			Height:     height,
			StartTime:  buyTicketParam.Start,
			ExpireTime: end,
			Value:      value,
		}
		if err := st.state.AddTicket(ticket); err != nil {
			st.addLog(common.BuyTicketFunc, param.Data, common.NewKeyValue("Error", "unable to add ticket"))
			return err
		}
		st.addLog(common.BuyTicketFunc, param.Data, common.NewKeyValue("Ticket", ticket.ID))
		return nil
	case common.AssetValueChangeFunc, common.OldAssetValueChangeFunc:
		outputCommandInfo("AssetValueChangeFunc", "from", st.msg.From())
		var assetValueChangeParamEx common.AssetValueChangeExParam
		if param.Func == common.OldAssetValueChangeFunc {
			// convert old data to new format
			assetValueChangeParam := common.AssetValueChangeParam{}
			rlp.DecodeBytes(param.Data, &assetValueChangeParam)
			assetValueChangeParamEx = common.AssetValueChangeExParam{
				AssetID:     assetValueChangeParam.AssetID,
				To:          assetValueChangeParam.To,
				Value:       assetValueChangeParam.Value,
				IsInc:       assetValueChangeParam.IsInc,
				TransacData: "",
			}
		} else {
			assetValueChangeParamEx = common.AssetValueChangeExParam{}
			rlp.DecodeBytes(param.Data, &assetValueChangeParamEx)
		}
		big0 := big.NewInt(0)
		if (assetValueChangeParamEx.IsInc && assetValueChangeParamEx.Value.Cmp(big0) <= 0) || (!assetValueChangeParamEx.IsInc && assetValueChangeParamEx.Value.Cmp(big0) >= 0) {
			st.addLog(common.AssetValueChangeFunc, assetValueChangeParamEx, common.NewKeyValue("Error", "illegal operation"))
			return fmt.Errorf("illegal operation")
		}
		assets, err := st.state.AllAssets()
		if err != nil {
			log.Debug("AssetValueChange unable to retrieve previous assets")
			st.addLog(common.AssetValueChangeFunc, assetValueChangeParamEx, common.NewKeyValue("Error", "unable to retrieve previous assets"))
			return err
		}

		asset, ok := assets[assetValueChangeParamEx.AssetID]
		if !ok {
			st.addLog(common.AssetValueChangeFunc, assetValueChangeParamEx, common.NewKeyValue("Error", "asset not found"))
			return fmt.Errorf("asset not found")
		}

		if !asset.CanChange {
			st.addLog(common.AssetValueChangeFunc, assetValueChangeParamEx, common.NewKeyValue("Error", "asset can't inc or dec"))
			return fmt.Errorf("asset can't inc or dec")
		}

		if asset.Owner != st.msg.From() {
			st.addLog(common.AssetValueChangeFunc, assetValueChangeParamEx, common.NewKeyValue("Error", "must be change by owner"))
			return fmt.Errorf("must be change by owner")
		}

		currentBalance := st.state.GetBalance(assetValueChangeParamEx.AssetID, assetValueChangeParamEx.To)
		val := assetValueChangeParamEx.Value
		if !assetValueChangeParamEx.IsInc {
			if currentBalance.Cmp(val) < 0 {
				st.addLog(common.AssetValueChangeFunc, assetValueChangeParamEx, common.NewKeyValue("Error", "not enough asset"))
				return fmt.Errorf("not enough asset")
			}
		}

		if assetValueChangeParamEx.IsInc {
			st.state.AddBalance(assetValueChangeParamEx.To, assetValueChangeParamEx.AssetID, assetValueChangeParamEx.Value)
			asset.Total = asset.Total.Add(asset.Total, assetValueChangeParamEx.Value)
		} else {
			if st.state.GetBalance(assetValueChangeParamEx.AssetID, assetValueChangeParamEx.To).Cmp(assetValueChangeParamEx.Value) < 0 {
				st.addLog(common.AssetValueChangeFunc, assetValueChangeParamEx, common.NewKeyValue("Error", "not enough asset"))
				return fmt.Errorf("not enough asset")
			}
			st.state.SubBalance(assetValueChangeParamEx.To, assetValueChangeParamEx.AssetID, assetValueChangeParamEx.Value)
			asset.Total = asset.Total.Sub(asset.Total, assetValueChangeParamEx.Value)
		}
		err = st.state.UpdateAsset(asset)
		if err == nil {
			st.addLog(common.AssetValueChangeFunc, assetValueChangeParamEx, common.NewKeyValue("AssetID", assetValueChangeParamEx.AssetID))
		} else {
			st.addLog(common.AssetValueChangeFunc, assetValueChangeParamEx, common.NewKeyValue("Error", "error update asset"))
		}
		return err
	case common.EmptyFunc:
	case common.MakeSwapFunc, common.MakeSwapFuncExt:
		outputCommandInfo("MakeSwapFunc", "from", st.msg.From())
		makeSwapParam := common.MakeSwapParam{}
		rlp.DecodeBytes(param.Data, &makeSwapParam)
		big0 := big.NewInt(0)
		if makeSwapParam.MinFromAmount.Cmp(big0) <= 0 || makeSwapParam.MinToAmount.Cmp(big0) <= 0 || makeSwapParam.SwapSize.Cmp(big0) <= 0 {
			st.addLog(common.MakeSwapFunc, makeSwapParam, common.NewKeyValue("Error", "MinFromAmount,MinToAmount and SwapSize must be ge 1"))
			return fmt.Errorf("MinFromAmount,MinToAmount and SwapSize must be ge 1")
		}

		total := new(big.Int).Mul(makeSwapParam.MinFromAmount, makeSwapParam.SwapSize)
		if total.Cmp(big0) <= 0 {
			log.Info("make swap overflow 1", "MinFromAmount", makeSwapParam.MinFromAmount, "Swap", makeSwapParam.SwapSize)
			st.addLog(common.MakeSwapFunc, makeSwapParam, common.NewKeyValue("Error", "size * minToAmount too large"))
			return fmt.Errorf("Error", "size * minToAmount too large")
		}
		start := makeSwapParam.FromStartTime
		end := makeSwapParam.FromEndTime
		needValue := common.NewTimeLock(&common.TimeLockItem{
			StartTime: start,
			EndTime:   end,
			Value:     total,
		})

		if start == common.TimeLockNow && end == common.TimeLockForever {
			if st.state.GetBalance(makeSwapParam.FromAssetID, st.msg.From()).Cmp(total) < 0 {
				st.addLog(common.MakeSwapFunc, makeSwapParam, common.NewKeyValue("Error", "not enough from asset"))
				return fmt.Errorf("not enough from asset")
			}
		} else {
			available := st.state.GetTimeLockBalance(makeSwapParam.FromAssetID, st.msg.From())
			if available.Cmp(needValue) < 0 {
				if param.Func == common.MakeSwapFunc {
					// this was the legacy swap do not do
					// time lock and just return an error
					st.addLog(common.MakeSwapFunc, makeSwapParam, common.NewKeyValue("Error", "not enough time lock or asset balance"))
					return fmt.Errorf("not enough time lock balance")
				}

				if st.state.GetBalance(makeSwapParam.FromAssetID, st.msg.From()).Cmp(total) < 0 {
					st.addLog(common.MakeSwapFunc, makeSwapParam, common.NewKeyValue("Error", "not enough time lock or asset balance"))
					return fmt.Errorf("not enough time lock or asset balance")
				}

				// subtract the asset from the balance
				st.state.SubBalance(st.msg.From(), makeSwapParam.FromAssetID, total)
				// see if we need timelock from to start
				if start != common.TimeLockNow {
					todayValue := common.NewTimeLock(&common.TimeLockItem{
						StartTime: common.TimeLockNow,
						EndTime:   start - 1,
						Value:     total,
					})
					st.state.AddTimeLockBalance(st.msg.From(), makeSwapParam.FromAssetID, todayValue)
				}
				//lock needed portion
				st.state.AddTimeLockBalance(st.msg.From(), makeSwapParam.FromAssetID, needValue)
				// then make sure from end to forever is set
				if end != common.TimeLockForever {
					totalValue := common.NewTimeLock(&common.TimeLockItem{
						StartTime: end + 1,
						EndTime:   common.TimeLockForever,
						Value:     total,
					})
					st.state.AddTimeLockBalance(st.msg.From(), makeSwapParam.FromAssetID, totalValue)
				}
			}
		}

		swap := common.Swap{
			ID:            st.msg.AsTransaction().Hash(),
			Owner:         st.msg.From(),
			FromAssetID:   makeSwapParam.FromAssetID,
			FromStartTime: makeSwapParam.FromStartTime,
			FromEndTime:   makeSwapParam.FromEndTime,
			MinFromAmount: makeSwapParam.MinFromAmount,
			ToAssetID:     makeSwapParam.ToAssetID,
			ToStartTime:   makeSwapParam.ToStartTime,
			ToEndTime:     makeSwapParam.ToEndTime,
			MinToAmount:   makeSwapParam.MinToAmount,
			SwapSize:      makeSwapParam.SwapSize,
			Targes:        makeSwapParam.Targes,
			Time:          makeSwapParam.Time, // this will mean the block time
		}
		if err := st.state.AddSwap(swap); err != nil {
			st.addLog(common.MakeSwapFunc, makeSwapParam, common.NewKeyValue("Error", "System error can't add swap"))
			return err
		}

		// take from the owner the asset
		//
		if start == common.TimeLockNow && end == common.TimeLockForever {
			st.state.SubBalance(st.msg.From(), makeSwapParam.FromAssetID, total)
		} else {
			st.state.SubTimeLockBalance(st.msg.From(), makeSwapParam.FromAssetID, needValue)
		}
		st.addLog(common.MakeSwapFunc, makeSwapParam, common.NewKeyValue("SwapID", swap.ID))
		return nil
	case common.RecallSwapFunc:
		outputCommandInfo("RecallSwapFunc", "from", st.msg.From())
		recallSwapParam := common.RecallSwapParam{}
		big0 := big.NewInt(0)
		rlp.DecodeBytes(param.Data, &recallSwapParam)
		swaps, err := st.state.AllSwaps()
		if err != nil {
			log.Debug("RecallSwapFunc unable to retrieve previous swaps")
			st.addLog(common.RecallSwapFunc, recallSwapParam, common.NewKeyValue("Error", "unable to retrieve previous swaps"))
			return err
		}
		swap, ok := swaps[recallSwapParam.SwapID]
		if !ok {
			st.addLog(common.RecallSwapFunc, recallSwapParam, common.NewKeyValue("Error", "Swap not found"))
			return fmt.Errorf("Swap not found")
		}

		if swap.Owner != st.msg.From() {
			st.addLog(common.RecallSwapFunc, recallSwapParam, common.NewKeyValue("Error", "Must be swap onwer can recall"))
			return fmt.Errorf("Must be swap onwer can recall")
		}

		total := new(big.Int).Mul(swap.MinFromAmount, swap.SwapSize)
		if total.Cmp(big0) <= 0 {
			st.addLog(common.RecallSwapFunc, recallSwapParam, common.NewKeyValue("Error", "size * minFromAmount too large"))
			return fmt.Errorf("Error", "size * minToAmount too large")
		}

		start := swap.FromStartTime
		end := swap.FromEndTime
		needValue := common.NewTimeLock(&common.TimeLockItem{
			StartTime: start,
			EndTime:   end,
			Value:     total,
		})

		if err := st.state.RemoveSwap(swap.ID); err != nil {
			st.addLog(common.RecallSwapFunc, recallSwapParam, common.NewKeyValue("Error", "Unable to remove swap"))
			return err
		}

		// return to the owner the balance
		if start == common.TimeLockNow && end == common.TimeLockForever {
			st.state.AddBalance(st.msg.From(), swap.FromAssetID, total)
		} else {
			st.state.AddTimeLockBalance(st.msg.From(), swap.FromAssetID, needValue)
		}
		st.addLog(common.RecallSwapFunc, recallSwapParam, common.NewKeyValue("SwapID", swap.ID))
		return nil
	case common.TakeSwapFunc, common.TakeSwapFuncExt:
		outputCommandInfo("TakeSwapFunc", "from", st.msg.From())
		takeSwapParam := common.TakeSwapParam{}
		rlp.DecodeBytes(param.Data, &takeSwapParam)

		swaps, err := st.state.AllSwaps()
		if err != nil {
			log.Info("TakeSwapFunc unable to retrieve previous swaps")
			st.addLog(common.TakeSwapFunc, takeSwapParam, common.NewKeyValue("Error", "Swap not found"))
			return err
		}

		swap, ok := swaps[takeSwapParam.SwapID]
		if !ok {
			st.addLog(common.TakeSwapFunc, takeSwapParam, common.NewKeyValue("Error", "swap not found"))
			return fmt.Errorf("Swap not found")
		}

		big0 := big.NewInt(0)
		if swap.SwapSize.Cmp(takeSwapParam.Size) < 0 || takeSwapParam.Size.Cmp(big0) <= 0 {
			st.addLog(common.TakeSwapFunc, takeSwapParam, common.NewKeyValue("Error", "swapsize must be le and Size must be ge 1"))
			return fmt.Errorf("SwapSize must le and Size must be ge 1")
		}

		if swap.MinFromAmount.Cmp(big0) <= 0 {
			st.addLog(common.TakeSwapFunc, takeSwapParam, common.NewKeyValue("Error", "MinFromAmount less than  equal to zero"))
			return fmt.Errorf("MinFromAmount less than  equal to zero")
		}

		if swap.MinToAmount.Cmp(big0) <= 0 {
			st.addLog(common.TakeSwapFunc, takeSwapParam, common.NewKeyValue("Error", "MinToAmount less than  equal to zero"))
			return fmt.Errorf("MinToAmount less than  equal to zero")
		}

		fromTotal := new(big.Int).Mul(swap.MinFromAmount, takeSwapParam.Size)

		if fromTotal.Cmp(big0) <= 0 {
			st.addLog(common.TakeSwapFunc, takeSwapParam, common.NewKeyValue("Error", "fromTotal less than  equal to zero"))
			return fmt.Errorf("fromTotal less than  equal to zero")
		}

		fromStart := swap.FromStartTime
		fromEnd := swap.FromEndTime
		fromNeedValue := common.NewTimeLock(&common.TimeLockItem{
			StartTime: fromStart,
			EndTime:   fromEnd,
			Value:     fromTotal,
		})

		toTotal := new(big.Int).Mul(swap.MinToAmount, takeSwapParam.Size)
		if toTotal.Cmp(big0) <= 0 {
			log.Info("total too big")
			st.addLog(common.TakeSwapFunc, takeSwapParam, common.NewKeyValue("Error", "toTotal less than  equal to zero"))
			return fmt.Errorf("toTotal less than  equal to zero")
		}

		toStart := swap.ToStartTime
		toEnd := swap.ToEndTime
		toNeedValue := common.NewTimeLock(&common.TimeLockItem{
			StartTime: toStart,
			EndTime:   toEnd,
			Value:     toTotal,
		})

		log.Info("Swap", "fromTotal", fromTotal, " toTotal", toTotal, "takeSwapParam", takeSwapParam, "swap", swap)

		if toStart == common.TimeLockNow && toEnd == common.TimeLockForever {
			if st.state.GetBalance(swap.ToAssetID, st.msg.From()).Cmp(toTotal) < 0 {
				st.addLog(common.TakeSwapFunc, takeSwapParam, common.NewKeyValue("Error", "not enough from asset"))
				return fmt.Errorf("not enough from asset")
			}
		} else {
			available := st.state.GetTimeLockBalance(swap.ToAssetID, st.msg.From())
			if available.Cmp(toNeedValue) < 0 {
				if param.Func == common.TakeSwapFunc {
					// this was the legacy swap do not do
					// time lock and just return an error
					st.addLog(common.TakeSwapFunc, takeSwapParam, common.NewKeyValue("Error", "not enough time lock balance"))
					return fmt.Errorf("not enough time lock balance")
				}

				if st.state.GetBalance(swap.ToAssetID, st.msg.From()).Cmp(toTotal) < 0 {
					st.addLog(common.TakeSwapFunc, takeSwapParam, common.NewKeyValue("Error", "not enough time lock balance"))
					return fmt.Errorf("not enough time lock or asset balance")
				}

				// subtract the asset from the balance
				st.state.SubBalance(st.msg.From(), swap.ToAssetID, toTotal)
				// see if we need timelock from to start
				if toStart != common.TimeLockNow {
					todayValue := common.NewTimeLock(&common.TimeLockItem{
						StartTime: common.TimeLockNow,
						EndTime:   toStart - 1,
						Value:     toTotal,
					})
					st.state.AddTimeLockBalance(st.msg.From(), swap.ToAssetID, todayValue)
				}
				//lock needed portion
				st.state.AddTimeLockBalance(st.msg.From(), swap.ToAssetID, toNeedValue)
				// then make sure from end to forever is set
				if toEnd != common.TimeLockForever {
					totalValue := common.NewTimeLock(&common.TimeLockItem{
						StartTime: toEnd + 1,
						EndTime:   common.TimeLockForever,
						Value:     toTotal,
					})
					st.state.AddTimeLockBalance(st.msg.From(), swap.ToAssetID, totalValue)
				}
			}
		}

		if swap.SwapSize.Cmp(takeSwapParam.Size) == 0 {
			if err := st.state.RemoveSwap(swap.ID); err != nil {
				st.addLog(common.TakeSwapFunc, takeSwapParam, common.NewKeyValue("Error", "System Error"))
				return err
			}
		} else {
			swap.SwapSize = swap.SwapSize.Sub(swap.SwapSize, takeSwapParam.Size)
			if err := st.state.UpdateSwap(swap); err != nil {
				st.addLog(common.TakeSwapFunc, takeSwapParam, common.NewKeyValue("Error", "System Error"))
				return err
			}
		}

		if toStart == common.TimeLockNow && toEnd == common.TimeLockForever {
			st.state.AddBalance(swap.Owner, swap.ToAssetID, toTotal)
			st.state.SubBalance(st.msg.From(), swap.ToAssetID, toTotal)
		} else {
			st.state.AddTimeLockBalance(swap.Owner, swap.ToAssetID, toNeedValue)
			st.state.SubTimeLockBalance(st.msg.From(), swap.ToAssetID, toNeedValue)
		}

		if fromStart == common.TimeLockNow && fromEnd == common.TimeLockForever {
			st.state.AddBalance(st.msg.From(), swap.FromAssetID, fromTotal)
			// the owner of the swap already had their balance taken away
			// in MakeSwapFunc
			// there is no need to subtract this balance again
			//st.state.SubBalance(swap.Owner, swap.FromAssetID, fromTotal)
		} else {
			st.state.AddTimeLockBalance(st.msg.From(), swap.FromAssetID, fromNeedValue)
			// the owner of the swap already had their timelock balance taken away
			// in MakeSwapFunc
			// there is no need to subtract this balance again
			// st.state.SubTimeLockBalance(swap.Owner, swap.FromAssetID, fromNeedValue)
		}
		st.addLog(common.TakeSwapFunc, takeSwapParam, common.NewKeyValue("SwapID", swap.ID))
		return nil
	}
	return fmt.Errorf("Unsupported")
}

func (st *StateTransition) addLog(typ common.FSNCallFunc, value interface{}, keyValues ...*common.KeyValue) {

	t := reflect.TypeOf(value)
	v := reflect.ValueOf(value)

	maps := make(map[string]interface{})
	if t.Kind() == reflect.Struct {
		for i := 0; i < t.NumField(); i++ {
			if v.Field(i).CanInterface() {
				maps[t.Field(i).Name] = v.Field(i).Interface()
			}
		}
	} else {
		maps["Base"] = value
	}

	for i := 0; i < len(keyValues); i++ {
		maps[keyValues[i].Key] = keyValues[i].Value
	}

	data, _ := json.Marshal(maps)

	topic := common.Hash{}
	topic[common.HashLength-1] = (uint8)(typ)

	st.evm.StateDB.AddLog(&types.Log{
		Address:     common.FSNCallAddress,
		Topics:      []common.Hash{topic},
		Data:        data,
		BlockNumber: st.evm.BlockNumber.Uint64(),
	})
}
