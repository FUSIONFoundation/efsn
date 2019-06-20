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
	fee        *big.Int
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
	mgval.Add(mgval, st.fee)
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
	st.fee = st.FsnCallFee()
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

		if st.to() == common.FSNCallAddress {
			errc := st.handleFsnCall()
			if errc != nil && common.DebugMode {
				param := common.FSNCallParam{}
				rlp.DecodeBytes(st.msg.Data(), &param)
				log.Info("handleFsnCall error", "number", st.evm.Context.BlockNumber, "Func", param.Func, "err", errc)
			}
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
	minerFees := new(big.Int).Mul(new(big.Int).SetUint64(st.gasUsed()), st.gasPrice)
	if st.fee.Sign() > 0 {
		minerFees.Add(minerFees, st.fee)
	}
	st.state.AddBalance(st.evm.Coinbase, common.SystemAssetID, minerFees)

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

var outputCommands = false

func outputCommandInfo(param1 string, param2 string, param3 interface{}) {
	if outputCommands {
		log.Info(param1, param2, param3)
	}
}

func (st *StateTransition) handleFsnCall() error {
	height := st.evm.Context.BlockNumber
	timestamp := st.evm.Context.ParentTime.Uint64()

	param := common.FSNCallParam{}
	rlp.DecodeBytes(st.msg.Data(), &param)
	switch param.Func {
	case common.GenNotationFunc:
		outputCommandInfo("GenNotationFunc", "from", st.msg.From())
		if err := st.state.GenNotation(st.msg.From()); err != nil {
			st.addLog(common.GenNotationFunc, param, common.NewKeyValue("Error", err.Error()))
			return err
		}
		st.addLog(common.GenNotationFunc, param, common.NewKeyValue("notation", st.state.GetNotation(st.msg.From())))
		return nil
	case common.GenAssetFunc:
		outputCommandInfo("GenAssetFunc", "from", st.msg.From())
		genAssetParam := common.GenAssetParam{}
		rlp.DecodeBytes(param.Data, &genAssetParam)
		if err := genAssetParam.Check(height); err != nil {
			st.addLog(common.GenAssetFunc, genAssetParam, common.NewKeyValue("Error", err.Error()))
			return err
		}
		asset := genAssetParam.ToAsset()
		asset.ID = st.msg.AsTransaction().Hash()
		asset.Owner = st.msg.From()
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
		if err := sendAssetParam.Check(height); err != nil {
			st.addLog(common.SendAssetFunc, sendAssetParam, common.NewKeyValue("Error", err.Error()))
			return err
		}
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

		// adjust param
		if timeLockParam.Type == common.TimeLockToAsset {
			if timeLockParam.StartTime > uint64(time.Now().Unix()) {
				st.addLog(common.TimeLockFunc, timeLockParam, common.NewKeyValue("LockType", "TimeLockToAsset"), common.NewKeyValue("Error", "Start time must be less than now"))
				return fmt.Errorf("Start time must be less than now")
			}
			timeLockParam.EndTime = common.TimeLockForever
		}
		if err := timeLockParam.Check(height, timestamp); err != nil {
			st.addLog(common.TimeLockFunc, timeLockParam, common.NewKeyValue("Error", err.Error()))
			return err
		}

		start := timeLockParam.StartTime
		end := timeLockParam.EndTime
		var needValue *common.TimeLock

		needValue = common.NewTimeLock(&common.TimeLockItem{
			StartTime: common.MaxUint64(start, timestamp),
			EndTime:   end,
			Value:     new(big.Int).SetBytes(timeLockParam.Value.Bytes()),
		})

		if err := needValue.IsValid(); err != nil {
			st.addLog(common.TimeLockFunc, timeLockParam, common.NewKeyValue("Error", err.Error()))
			return fmt.Errorf(err.Error())
		}

		switch timeLockParam.Type {
		case common.AssetToTimeLock:
			if st.state.GetBalance(timeLockParam.AssetID, st.msg.From()).Cmp(timeLockParam.Value) < 0 {
				st.addLog(common.TimeLockFunc, timeLockParam, common.NewKeyValue("LockType", "AssetToTimeLock"), common.NewKeyValue("Error", "not enough asset"))
				return fmt.Errorf("not enough asset")
			}
			st.state.SubBalance(st.msg.From(), timeLockParam.AssetID, timeLockParam.Value)

			totalValue := common.NewTimeLock(&common.TimeLockItem{
				StartTime: timestamp,
				EndTime:   common.TimeLockForever,
				Value:     new(big.Int).SetBytes(timeLockParam.Value.Bytes()),
			})
			if st.msg.From() == timeLockParam.To {
				st.state.AddTimeLockBalance(timeLockParam.To, timeLockParam.AssetID, totalValue, height, timestamp)
			} else {
				surplusValue := new(common.TimeLock).Sub(totalValue, needValue)
				if !surplusValue.IsEmpty() {
					st.state.AddTimeLockBalance(st.msg.From(), timeLockParam.AssetID, surplusValue, height, timestamp)
				}
				st.state.AddTimeLockBalance(timeLockParam.To, timeLockParam.AssetID, needValue, height, timestamp)
			}

			st.addLog(common.TimeLockFunc, timeLockParam, common.NewKeyValue("LockType", "AssetToTimeLock"), common.NewKeyValue("AssetID", timeLockParam.AssetID))
			return nil
		case common.TimeLockToTimeLock:
			if st.state.GetTimeLockBalance(timeLockParam.AssetID, st.msg.From()).Cmp(needValue) < 0 {
				st.addLog(common.TimeLockFunc, timeLockParam, common.NewKeyValue("LockType", "TimeLockToTimeLock"), common.NewKeyValue("Error", "not enough time lock balance"))
				return fmt.Errorf("not enough time lock balance")
			}
			st.state.SubTimeLockBalance(st.msg.From(), timeLockParam.AssetID, needValue, height, timestamp)
			st.state.AddTimeLockBalance(timeLockParam.To, timeLockParam.AssetID, needValue, height, timestamp)
			st.addLog(common.TimeLockFunc, timeLockParam, common.NewKeyValue("LockType", "TimeLockToTimeLock"), common.NewKeyValue("AssetID", timeLockParam.AssetID))
			return nil
		case common.TimeLockToAsset:
			if st.state.GetTimeLockBalance(timeLockParam.AssetID, st.msg.From()).Cmp(needValue) < 0 {
				st.addLog(common.TimeLockFunc, timeLockParam, common.NewKeyValue("LockType", "TimeLockToAsset"), common.NewKeyValue("Error", "not enough time lock balance"))
				return fmt.Errorf("not enough time lock balance")
			}
			st.state.SubTimeLockBalance(st.msg.From(), timeLockParam.AssetID, needValue, height, timestamp)
			st.state.AddBalance(timeLockParam.To, timeLockParam.AssetID, timeLockParam.Value)
			st.addLog(common.TimeLockFunc, timeLockParam, common.NewKeyValue("LockType", "TimeLockToAsset"), common.NewKeyValue("AssetID", timeLockParam.AssetID))
			return nil
		}
	case common.BuyTicketFunc:
		outputCommandInfo("BuyTicketFunc", "from", st.msg.From())
		from := st.msg.From()
		id := common.TicketID(from, height.Uint64(), 0)

		if st.state.IsTicketExist(id) {
			st.addLog(common.BuyTicketFunc, param.Data, common.NewKeyValue("Error", "Ticket already exist"))
			return fmt.Errorf(id.String() + " Ticket already exist")
		}

		buyTicketParam := common.BuyTicketParam{}
		rlp.DecodeBytes(param.Data, &buyTicketParam)

		if err := buyTicketParam.Check(height, 0, 0); err != nil {
			st.addLog(common.BuyTicketFunc, param.Data, common.NewKeyValue("Error", err.Error()))
			return err
		}

		start := buyTicketParam.Start
		end := buyTicketParam.End
		value := common.TicketPrice(height)
		var needValue *common.TimeLock

		needValue = common.NewTimeLock(&common.TimeLockItem{
			StartTime: common.MaxUint64(start, timestamp),
			EndTime:   end,
			Value:     value,
		})
		if err := needValue.IsValid(); err != nil {
			st.addLog(common.BuyTicketFunc, param.Data, common.NewKeyValue("Error", err.Error()))
			return fmt.Errorf(err.Error())
		}

		ticket := common.Ticket{
			ID: id,
			TicketBody: common.TicketBody{
				Height:     height.Uint64(),
				StartTime:  start,
				ExpireTime: end,
			},
		}

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
				StartTime: timestamp,
				EndTime:   common.TimeLockForever,
				Value:     value,
			})
			surplusValue := new(common.TimeLock).Sub(totalValue, needValue)
			if !surplusValue.IsEmpty() {
				st.state.AddTimeLockBalance(from, common.SystemAssetID, surplusValue, height, timestamp)
			}

		} else {
			st.state.SubTimeLockBalance(from, common.SystemAssetID, needValue, height, timestamp)
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

		if err := assetValueChangeParamEx.Check(height); err != nil {
			st.addLog(common.AssetValueChangeFunc, assetValueChangeParamEx, common.NewKeyValue("Error", err.Error()))
			return err
		}

		asset, err := st.state.GetAsset(assetValueChangeParamEx.AssetID)
		if err != nil {
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

		if asset.Owner != assetValueChangeParamEx.To {
			err := fmt.Errorf("to address must be owner")
			st.addLog(common.AssetValueChangeFunc, assetValueChangeParamEx, common.NewKeyValue("Error", err.Error()))
			return err
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
		notation := st.state.GetNotation(st.msg.From())
		makeSwapParam := common.MakeSwapParam{}
		rlp.DecodeBytes(param.Data, &makeSwapParam)
		swapId := st.msg.AsTransaction().Hash()

		_, err := st.state.GetSwap(swapId)
		if err == nil {
			st.addLog(common.MakeSwapFunc, makeSwapParam, common.NewKeyValue("Error", "Swap already exist"))
			return fmt.Errorf("Swap already exist")
		}

		if err := makeSwapParam.Check(height, timestamp); err != nil {
			st.addLog(common.MakeSwapFunc, makeSwapParam, common.NewKeyValue("Error", err.Error()))
			return err
		}

		var useAsset bool
		var total *big.Int
		var needValue *common.TimeLock

		if makeSwapParam.ToAssetID == common.OwnerUSANAssetID {
			err := fmt.Errorf("USAN's cannot be swapped")
			st.addLog(common.MakeSwapFunc, makeSwapParam, common.NewKeyValue("Error", err.Error()))
			return err
		}

		if _, err := st.state.GetAsset(makeSwapParam.ToAssetID); err != nil {
			err := fmt.Errorf("ToAssetID's asset not found")
			st.addLog(common.MakeSwapFunc, makeSwapParam, common.NewKeyValue("Error", err.Error()))
			return err
		}

		if makeSwapParam.FromAssetID == common.OwnerUSANAssetID {
			if notation == 0 {
				err := fmt.Errorf("the from address does not have a notation")
				st.addLog(common.MakeSwapFunc, makeSwapParam, common.NewKeyValue("Error", err.Error()))
				return err
			}
			makeSwapParam.MinFromAmount = big.NewInt(1)
			makeSwapParam.SwapSize = big.NewInt(1)
			makeSwapParam.FromStartTime = common.TimeLockNow
			makeSwapParam.FromEndTime = common.TimeLockForever
			useAsset = true
			total = new(big.Int).Mul(makeSwapParam.MinFromAmount, makeSwapParam.SwapSize)
		} else {
			total = new(big.Int).Mul(makeSwapParam.MinFromAmount, makeSwapParam.SwapSize)
			start := makeSwapParam.FromStartTime
			end := makeSwapParam.FromEndTime
			useAsset = start == common.TimeLockNow && end == common.TimeLockForever
			needValue = common.NewTimeLock(&common.TimeLockItem{
				StartTime: common.MaxUint64(start, timestamp),
				EndTime:   end,
				Value:     total,
			})
			if err := needValue.IsValid(); err != nil {
				st.addLog(common.MakeSwapFunc, makeSwapParam, common.NewKeyValue("Error", err.Error()))
				return fmt.Errorf(err.Error())
			}
		}
		swap := common.Swap{
			ID:            swapId,
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
			Description:   makeSwapParam.Description,
			Notation:      notation,
		}

		if makeSwapParam.FromAssetID == common.OwnerUSANAssetID {
			if err := st.state.AddSwap(swap); err != nil {
				st.addLog(common.MakeSwapFunc, makeSwapParam, common.NewKeyValue("Error", "System error can't add swap"))
				return err
			}
		} else {
			if useAsset == true {
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

					totalValue := common.NewTimeLock(&common.TimeLockItem{
						StartTime: timestamp,
						EndTime:   common.TimeLockForever,
						Value:     total,
					})
					st.state.AddTimeLockBalance(st.msg.From(), makeSwapParam.FromAssetID, totalValue, height, timestamp)

				}
			}

			if err := st.state.AddSwap(swap); err != nil {
				st.addLog(common.MakeSwapFunc, makeSwapParam, common.NewKeyValue("Error", "System error can't add swap"))
				return err
			}

			// take from the owner the asset
			if useAsset == true {
				st.state.SubBalance(st.msg.From(), makeSwapParam.FromAssetID, total)
			} else {
				st.state.SubTimeLockBalance(st.msg.From(), makeSwapParam.FromAssetID, needValue, height, timestamp)
			}
		}
		st.addLog(common.MakeSwapFunc, makeSwapParam, common.NewKeyValue("SwapID", swap.ID))
		return nil
	case common.RecallSwapFunc:
		outputCommandInfo("RecallSwapFunc", "from", st.msg.From())
		recallSwapParam := common.RecallSwapParam{}
		rlp.DecodeBytes(param.Data, &recallSwapParam)

		swap, err := st.state.GetSwap(recallSwapParam.SwapID)
		if err != nil {
			st.addLog(common.RecallSwapFunc, recallSwapParam, common.NewKeyValue("Error", "Swap not found"))
			return fmt.Errorf("Swap not found")
		}

		if swap.Owner != st.msg.From() {
			st.addLog(common.RecallSwapFunc, recallSwapParam, common.NewKeyValue("Error", "Must be swap onwer can recall"))
			return fmt.Errorf("Must be swap onwer can recall")
		}

		if err := recallSwapParam.Check(height, &swap); err != nil {
			st.addLog(common.RecallSwapFunc, recallSwapParam, common.NewKeyValue("Error", err.Error()))
			return err
		}

		if swap.FromAssetID == common.OwnerUSANAssetID {
			if err := st.state.RemoveSwap(swap.ID); err != nil {
				st.addLog(common.RecallSwapFunc, recallSwapParam, common.NewKeyValue("Error", "Unable to remove swap"))
				return err
			}
		} else {
			total := new(big.Int).Mul(swap.MinFromAmount, swap.SwapSize)
			start := swap.FromStartTime
			end := swap.FromEndTime
			useAsset := start == common.TimeLockNow && end == common.TimeLockForever
			var needValue *common.TimeLock

			needValue = common.NewTimeLock(&common.TimeLockItem{
				StartTime: common.MaxUint64(start, timestamp),
				EndTime:   end,
				Value:     total,
			})

			if err := st.state.RemoveSwap(swap.ID); err != nil {
				st.addLog(common.RecallSwapFunc, recallSwapParam, common.NewKeyValue("Error", "Unable to remove swap"))
				return err
			}

			// return to the owner the balance
			if useAsset == true {
				st.state.AddBalance(st.msg.From(), swap.FromAssetID, total)
			} else {
				if err := needValue.IsValid(); err == nil {
					st.state.AddTimeLockBalance(st.msg.From(), swap.FromAssetID, needValue, height, timestamp)
				}
			}
		}
		st.addLog(common.RecallSwapFunc, recallSwapParam, common.NewKeyValue("SwapID", swap.ID))
		return nil
	case common.TakeSwapFunc, common.TakeSwapFuncExt:
		outputCommandInfo("TakeSwapFunc", "from", st.msg.From())
		takeSwapParam := common.TakeSwapParam{}
		rlp.DecodeBytes(param.Data, &takeSwapParam)

		swap, err := st.state.GetSwap(takeSwapParam.SwapID)
		if err != nil {
			st.addLog(common.TakeSwapFunc, takeSwapParam, common.NewKeyValue("Error", "swap not found"))
			return fmt.Errorf("Swap not found")
		}

		if err := takeSwapParam.Check(height, &swap, timestamp); err != nil {
			st.addLog(common.TakeSwapFunc, takeSwapParam, common.NewKeyValue("Error", err.Error()))
			return err
		}

		var usanSwap bool
		if swap.FromAssetID == common.OwnerUSANAssetID {
			notation := st.state.GetNotation(swap.Owner)
			if notation == 0 || notation != swap.Notation {
				err := fmt.Errorf("notation in swap is no longer valid")
				st.addLog(common.TakeSwapFunc, takeSwapParam, common.NewKeyValue("Error", err.Error()))
				return err
			}
			usanSwap = true
		} else {
			usanSwap = false
		}

		fromTotal := new(big.Int).Mul(swap.MinFromAmount, takeSwapParam.Size)
		fromStart := swap.FromStartTime
		fromEnd := swap.FromEndTime
		fromUseAsset := fromStart == common.TimeLockNow && fromEnd == common.TimeLockForever

		toTotal := new(big.Int).Mul(swap.MinToAmount, takeSwapParam.Size)
		toStart := swap.ToStartTime
		toEnd := swap.ToEndTime
		toUseAsset := toStart == common.TimeLockNow && toEnd == common.TimeLockForever

		var fromNeedValue *common.TimeLock
		var toNeedValue *common.TimeLock

		fromNeedValue = common.NewTimeLock(&common.TimeLockItem{
			StartTime: common.MaxUint64(fromStart, timestamp),
			EndTime:   fromEnd,
			Value:     fromTotal,
		})
		toNeedValue = common.NewTimeLock(&common.TimeLockItem{
			StartTime: common.MaxUint64(toStart, timestamp),
			EndTime:   toEnd,
			Value:     toTotal,
		})

		//log.Info("Swap", "fromTotal", fromTotal, " toTotal", toTotal, "takeSwapParam", takeSwapParam, "swap", swap)

		if toUseAsset == true {
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

				totalValue := common.NewTimeLock(&common.TimeLockItem{
					StartTime: timestamp,
					EndTime:   common.TimeLockForever,
					Value:     toTotal,
				})
				st.state.AddTimeLockBalance(st.msg.From(), swap.ToAssetID, totalValue, height, timestamp)

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

		if toUseAsset == true {
			st.state.AddBalance(swap.Owner, swap.ToAssetID, toTotal)
			st.state.SubBalance(st.msg.From(), swap.ToAssetID, toTotal)
		} else {
			if err := toNeedValue.IsValid(); err == nil {
				st.state.AddTimeLockBalance(swap.Owner, swap.ToAssetID, toNeedValue, height, timestamp)
				st.state.SubTimeLockBalance(st.msg.From(), swap.ToAssetID, toNeedValue, height, timestamp)
			}
		}

		// credit the taker
		if usanSwap {
			err := st.state.TransferNotation(swap.Notation, swap.Owner, st.msg.From())
			if err != nil {
				st.addLog(common.TakeSwapFunc, takeSwapParam, common.NewKeyValue("Error", "System Error"))
				return err
			}
		} else {
			if fromUseAsset == true {
				st.state.AddBalance(st.msg.From(), swap.FromAssetID, fromTotal)
				// the owner of the swap already had their balance taken away
				// in MakeSwapFunc
				// there is no need to subtract this balance again
				//st.state.SubBalance(swap.Owner, swap.FromAssetID, fromTotal)
			} else {
				if err := fromNeedValue.IsValid(); err == nil {
					st.state.AddTimeLockBalance(st.msg.From(), swap.FromAssetID, fromNeedValue, height, timestamp)
				}
				// the owner of the swap already had their timelock balance taken away
				// in MakeSwapFunc
				// there is no need to subtract this balance again
				// st.state.SubTimeLockBalance(swap.Owner, swap.FromAssetID, fromNeedValue)
			}
		}
		st.addLog(common.TakeSwapFunc, takeSwapParam, common.NewKeyValue("SwapID", swap.ID))
		return nil
	case common.RecallMultiSwapFunc:
		outputCommandInfo("RecallMultiSwapFunc", "from", st.msg.From())
		recallSwapParam := common.RecallMultiSwapParam{}
		rlp.DecodeBytes(param.Data, &recallSwapParam)

		swap, err := st.state.GetMultiSwap(recallSwapParam.SwapID)
		if err != nil {
			st.addLog(common.RecallMultiSwapFunc, recallSwapParam, common.NewKeyValue("Error", "Swap not found"))
			return fmt.Errorf("Swap not found")
		}

		if swap.Owner != st.msg.From() {
			st.addLog(common.RecallMultiSwapFunc, recallSwapParam, common.NewKeyValue("Error", "Must be swap onwer can recall"))
			return fmt.Errorf("Must be swap onwer can recall")
		}

		if err := recallSwapParam.Check(height, &swap); err != nil {
			st.addLog(common.RecallMultiSwapFunc, recallSwapParam, common.NewKeyValue("Error", err.Error()))
			return err
		}

		ln := len(swap.FromAssetID)
		for i := 0; i < ln; i++ {
			if swap.FromAssetID[i] == common.OwnerUSANAssetID {
				err := fmt.Errorf("Cannot multiswap USANs")
				st.addLog(common.RecallMultiSwapFunc, recallSwapParam, common.NewKeyValue("Error", "Cannot multiswap USANs"))
				return err
			}
			total := new(big.Int).Mul(swap.MinFromAmount[i], swap.SwapSize)
			start := swap.FromStartTime[i]
			end := swap.FromEndTime[i]
			useAsset := start == common.TimeLockNow && end == common.TimeLockForever
			var needValue *common.TimeLock

			needValue = common.NewTimeLock(&common.TimeLockItem{
				StartTime: common.MaxUint64(start, timestamp),
				EndTime:   end,
				Value:     total,
			})

			if err := st.state.RemoveMultiSwap(swap.ID); err != nil {
				st.addLog(common.RecallMultiSwapFunc, recallSwapParam, common.NewKeyValue("Error", "Unable to remove swap"))
				return err
			}

			// return to the owner the balance
			if useAsset == true {
				st.state.AddBalance(st.msg.From(), swap.FromAssetID[i], total)
			} else {
				if err := needValue.IsValid(); err == nil {
					st.state.AddTimeLockBalance(st.msg.From(), swap.FromAssetID[i], needValue, height, timestamp)
				}
			}
		}
		st.addLog(common.RecallMultiSwapFunc, recallSwapParam, common.NewKeyValue("SwapID", swap.ID))
		return nil
	case common.MakeMultiSwapFunc:
		outputCommandInfo("MakeMultiSwapFunc", "from", st.msg.From())
		notation := st.state.GetNotation(st.msg.From())
		makeSwapParam := common.MakeMultiSwapParam{}
		rlp.DecodeBytes(param.Data, &makeSwapParam)
		swapID := st.msg.AsTransaction().Hash()

		_, err := st.state.GetSwap(swapID)
		if err == nil {
			st.addLog(common.MakeMultiSwapFunc, makeSwapParam, common.NewKeyValue("Error", "Swap already exist"))
			return fmt.Errorf("Swap already exist")
		}

		if err := makeSwapParam.Check(height, timestamp); err != nil {
			st.addLog(common.MakeMultiSwapFunc, makeSwapParam, common.NewKeyValue("Error", err.Error()))
			return err
		}

		for _, toAssetID := range makeSwapParam.ToAssetID {
			if toAssetID == common.OwnerUSANAssetID {
				err := fmt.Errorf("USAN's cannot be multi swapped")
				st.addLog(common.MakeMultiSwapFunc, makeSwapParam, common.NewKeyValue("Error", err.Error()))
				return err
			}

			if _, err := st.state.GetAsset(toAssetID); err != nil {
				err := fmt.Errorf("ToAssetID's asset not found")
				st.addLog(common.MakeMultiSwapFunc, makeSwapParam, common.NewKeyValue("Error", err.Error()))
				return err
			}
		}

		ln := len(makeSwapParam.FromAssetID)

		useAsset := make([]bool, ln)
		total := make([]*big.Int, ln)
		needValue := make([]*common.TimeLock, ln)

		for i := 0; i < ln; i++ {

			if makeSwapParam.FromAssetID[i] == common.OwnerUSANAssetID {
				err := fmt.Errorf("USAN's cannot be multi swapped")
				st.addLog(common.MakeMultiSwapFunc, makeSwapParam, common.NewKeyValue("Error", err.Error()))
				return err
			}

			total[i] = new(big.Int).Mul(makeSwapParam.MinFromAmount[i], makeSwapParam.SwapSize)
			start := makeSwapParam.FromStartTime[i]
			end := makeSwapParam.FromEndTime[i]
			useAsset[i] = start == common.TimeLockNow && end == common.TimeLockForever
			needValue[i] = common.NewTimeLock(&common.TimeLockItem{
				StartTime: common.MaxUint64(start, timestamp),
				EndTime:   end,
				Value:     total[i],
			})
			if err := needValue[i].IsValid(); err != nil {
				st.addLog(common.MakeMultiSwapFunc, makeSwapParam, common.NewKeyValue("Error", err.Error()))
				return fmt.Errorf(err.Error())
			}

		}
		swap := common.MultiSwap{
			ID:            swapID,
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
			Description:   makeSwapParam.Description,
			Notation:      notation,
		}

		ln = len(makeSwapParam.FromAssetID)
		// check balances first
		for i := 0; i < ln; i++ {
			if useAsset[i] == true {
				if st.state.GetBalance(makeSwapParam.FromAssetID[i], st.msg.From()).Cmp(total[i]) < 0 {
					st.addLog(common.MakeMultiSwapFunc, makeSwapParam, common.NewKeyValue("Error", "not enough from asset"))
					return fmt.Errorf("not enough from asset")
				}
			} else {
				available := st.state.GetTimeLockBalance(makeSwapParam.FromAssetID[i], st.msg.From())
				if available.Cmp(needValue[i]) < 0 {
					if st.state.GetBalance(makeSwapParam.FromAssetID[i], st.msg.From()).Cmp(total[i]) < 0 {
						st.addLog(common.MakeMultiSwapFunc, makeSwapParam, common.NewKeyValue("Error", "not enough time lock or asset balance"))
						return fmt.Errorf("not enough time lock or asset balance")
					}
				}
			}
		}
		// then deduct
		for i := 0; i < ln; i++ {
			if useAsset[i] == true {
				if st.state.GetBalance(makeSwapParam.FromAssetID[i], st.msg.From()).Cmp(total[i]) < 0 {
					st.addLog(common.MakeMultiSwapFunc, makeSwapParam, common.NewKeyValue("Error", "not enough from asset"))
					return fmt.Errorf("not enough from asset")
				}
			} else {
				available := st.state.GetTimeLockBalance(makeSwapParam.FromAssetID[i], st.msg.From())
				if available.Cmp(needValue[i]) < 0 {

					if st.state.GetBalance(makeSwapParam.FromAssetID[i], st.msg.From()).Cmp(total[i]) < 0 {
						st.addLog(common.MakeMultiSwapFunc, makeSwapParam, common.NewKeyValue("Error", "not enough time lock or asset balance"))
						return fmt.Errorf("not enough time lock or asset balance")
					}

					// subtract the asset from the balance
					st.state.SubBalance(st.msg.From(), makeSwapParam.FromAssetID[i], total[i])

					totalValue := common.NewTimeLock(&common.TimeLockItem{
						StartTime: timestamp,
						EndTime:   common.TimeLockForever,
						Value:     total[i],
					})
					st.state.AddTimeLockBalance(st.msg.From(), makeSwapParam.FromAssetID[i], totalValue, height, timestamp)
				}
			}

			if err := st.state.AddMultiSwap(swap); err != nil {
				st.addLog(common.MakeMultiSwapFunc, makeSwapParam, common.NewKeyValue("Error", "System error can't add swap"))
				return err
			}

			// take from the owner the asset
			if useAsset[i] == true {
				st.state.SubBalance(st.msg.From(), makeSwapParam.FromAssetID[i], total[i])
			} else {
				st.state.SubTimeLockBalance(st.msg.From(), makeSwapParam.FromAssetID[i], needValue[i], height, timestamp)
			}
		}
		st.addLog(common.MakeMultiSwapFunc, makeSwapParam, common.NewKeyValue("SwapID", swap.ID))
		return nil
	case common.TakeMultiSwapFunc:
		outputCommandInfo("TakeMultiSwapFunc", "from", st.msg.From())
		takeSwapParam := common.TakeMultiSwapParam{}
		rlp.DecodeBytes(param.Data, &takeSwapParam)

		swap, err := st.state.GetMultiSwap(takeSwapParam.SwapID)
		if err != nil {
			st.addLog(common.TakeSwapFunc, takeSwapParam, common.NewKeyValue("Error", "swap not found"))
			return fmt.Errorf("Swap not found")
		}

		if err := takeSwapParam.Check(height, &swap, timestamp); err != nil {
			st.addLog(common.TakeSwapFunc, takeSwapParam, common.NewKeyValue("Error", err.Error()))
			return err
		}

		lnFrom := len(swap.FromAssetID)

		fromUseAsset := make([]bool, lnFrom)
		fromTotal := make([]*big.Int, lnFrom)
		fromStart := make([]uint64, lnFrom)
		fromEnd := make([]uint64, lnFrom)
		fromNeedValue := make([]*common.TimeLock, lnFrom)
		for i := 0; i < lnFrom; i++ {
			fromTotal[i] = new(big.Int).Mul(swap.MinFromAmount[i], takeSwapParam.Size)
			fromStart[i] = swap.FromStartTime[i]
			fromEnd[i] = swap.FromEndTime[i]
			fromUseAsset[i] = fromStart[i] == common.TimeLockNow && fromEnd[i] == common.TimeLockForever

			fromNeedValue[i] = common.NewTimeLock(&common.TimeLockItem{
				StartTime: common.MaxUint64(fromStart[i], timestamp),
				EndTime:   fromEnd[i],
				Value:     fromTotal[i],
			})
		}

		lnTo := len(swap.ToAssetID)

		toUseAsset := make([]bool, lnTo)
		toTotal := make([]*big.Int, lnTo)
		toStart := make([]uint64, lnTo)
		toEnd := make([]uint64, lnTo)
		toNeedValue := make([]*common.TimeLock, lnFrom)

		// check to account balances
		for i := 0; i < lnTo; i++ {
			toTotal[i] = new(big.Int).Mul(swap.MinToAmount[i], takeSwapParam.Size)
			toStart[i] = swap.ToStartTime[i]
			toEnd[i] = swap.ToEndTime[i]
			toUseAsset[i] = toStart[i] == common.TimeLockNow && toEnd[i] == common.TimeLockForever
			toNeedValue[i] = common.NewTimeLock(&common.TimeLockItem{
				StartTime: common.MaxUint64(toStart[i], timestamp),
				EndTime:   toEnd[i],
				Value:     toTotal[i],
			})

			if toUseAsset[i] == true {
				if st.state.GetBalance(swap.ToAssetID[i], st.msg.From()).Cmp(toTotal[i]) < 0 {
					st.addLog(common.TakeMultiSwapFunc, takeSwapParam, common.NewKeyValue("Error", "not enough to asset"))
					return fmt.Errorf("not enough to asset")
				}
			} else {
				available := st.state.GetTimeLockBalance(swap.ToAssetID[i], st.msg.From())
				if available.Cmp(toNeedValue[i]) < 0 {

					if st.state.GetBalance(swap.ToAssetID[i], st.msg.From()).Cmp(toTotal[i]) < 0 {
						st.addLog(common.TakeMultiSwapFunc, takeSwapParam, common.NewKeyValue("Error", "not enough time lock balance"))
						return fmt.Errorf("not enough time lock or asset balance")
					}

					// subtract the asset from the balance
					st.state.SubBalance(st.msg.From(), swap.ToAssetID[i], toTotal[i])

					totalValue := common.NewTimeLock(&common.TimeLockItem{
						StartTime: timestamp,
						EndTime:   common.TimeLockForever,
						Value:     toTotal[i],
					})
					st.state.AddTimeLockBalance(st.msg.From(), swap.ToAssetID[i], totalValue, height, timestamp)

				}
			}
		}

		if swap.SwapSize.Cmp(takeSwapParam.Size) == 0 {
			if err := st.state.RemoveMultiSwap(swap.ID); err != nil {
				st.addLog(common.TakeMultiSwapFunc, takeSwapParam, common.NewKeyValue("Error", "System Error"))
				return err
			}
		} else {
			swap.SwapSize = swap.SwapSize.Sub(swap.SwapSize, takeSwapParam.Size)
			if err := st.state.UpdateMultiSwap(swap); err != nil {
				st.addLog(common.TakeMultiSwapFunc, takeSwapParam, common.NewKeyValue("Error", "System Error"))
				return err
			}
		}

		// credit the swap owner with to assets
		for i := 0; i < lnTo; i++ {
			if toUseAsset[i] == true {
				st.state.AddBalance(swap.Owner, swap.ToAssetID[i], toTotal[i])
				st.state.SubBalance(st.msg.From(), swap.ToAssetID[i], toTotal[i])
			} else {
				if err := toNeedValue[i].IsValid(); err == nil {
					st.state.AddTimeLockBalance(swap.Owner, swap.ToAssetID[i], toNeedValue[i], height, timestamp)
					st.state.SubTimeLockBalance(st.msg.From(), swap.ToAssetID[i], toNeedValue[i], height, timestamp)
				}
			}
		}

		// credit the swap take with the from assets
		for i := 0; i < lnFrom; i++ {
			if fromUseAsset[i] == true {
				st.state.AddBalance(st.msg.From(), swap.FromAssetID[i], fromTotal[i])
				// the owner of the swap already had their balance taken away
				// in MakeMultiSwapFunc
				// there is no need to subtract this balance again
				//st.state.SubBalance(swap.Owner, swap.FromAssetID, fromTotal)
			} else {
				if err := fromNeedValue[i].IsValid(); err == nil {
					st.state.AddTimeLockBalance(st.msg.From(), swap.FromAssetID[i], fromNeedValue[i], height, timestamp)
				}
				// the owner of the swap already had their timelock balance taken away
				// in MakeMultiSwapFunc
				// there is no need to subtract this balance again
				// st.state.SubTimeLockBalance(swap.Owner, swap.FromAssetID, fromNeedValue)
			}
		}
		st.addLog(common.TakeMultiSwapFunc, takeSwapParam, common.NewKeyValue("SwapID", swap.ID))
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

func (st *StateTransition) FsnCallFee() *big.Int {
	fee := common.Big0
	if st.to() != common.FSNCallAddress {
		return fee
	}
	param := common.FSNCallParam{}
	rlp.DecodeBytes(st.msg.Data(), &param)
	switch param.Func {
	case common.GenNotationFunc:
		fee = big.NewInt(1000000000000000000) // 1 FSN
	case common.GenAssetFunc:
		fee = big.NewInt(1000000000000000000) // 1 FSN
	case common.MakeSwapFunc, common.MakeSwapFuncExt, common.MakeMultiSwapFunc:
		fee = big.NewInt(10000000000000000) // 0.01 FSN
	case common.TimeLockFunc:
		fee = big.NewInt(1000000000000000) // 0.001 FSN
	}
	return fee
}
