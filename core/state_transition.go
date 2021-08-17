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
	"fmt"
	"math"
	"math/big"
	"reflect"
	"time"

	"github.com/FusionFoundation/efsn/common"
	"github.com/FusionFoundation/efsn/common/hexutil"
	"github.com/FusionFoundation/efsn/consensus/datong"
	"github.com/FusionFoundation/efsn/core/types"
	"github.com/FusionFoundation/efsn/core/vm"
	"github.com/FusionFoundation/efsn/crypto"
	"github.com/FusionFoundation/efsn/log"
	"github.com/FusionFoundation/efsn/params"
	"github.com/FusionFoundation/efsn/rlp"
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
	To() *common.Address

	GasPrice() *big.Int
	Gas() uint64
	Value() *big.Int

	Nonce() uint64
	CheckNonce() bool
	Data() []byte

	AsTransaction() *types.Transaction
}

// ExecutionResult includes all output after executing given evm
// message no matter the execution itself is successful or not.
type ExecutionResult struct {
	UsedGas    uint64 // Total used gas but include the refunded gas
	Err        error  // Any error encountered during the execution(listed in core/vm/errors.go)
	ReturnData []byte // Returned data from evm(function result or data supplied with revert opcode)
}

// Unwrap returns the internal evm error which allows us for further
// analysis outside.
func (result *ExecutionResult) Unwrap() error {
	return result.Err
}

// Failed returns the indicator whether the execution is successful or not
func (result *ExecutionResult) Failed() bool { return result.Err != nil }

// Return is a helper function to help caller distinguish between revert reason
// and function return. Return returns the data after execution if no error occurs.
func (result *ExecutionResult) Return() []byte {
	if result.Err != nil {
		return nil
	}
	return common.CopyBytes(result.ReturnData)
}

// Revert returns the concrete revert reason if the execution is aborted by `REVERT`
// opcode. Note the reason can be nil if no data supplied with revert opcode.
func (result *ExecutionResult) Revert() []byte {
	if result.Err != vm.ErrExecutionReverted {
		return nil
	}
	return common.CopyBytes(result.ReturnData)
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
		nonZeroGas := params.TxDataNonZeroGasFrontier
		if (math.MaxUint64-gas)/nonZeroGas < nz {
			return 0, vm.ErrGasUintOverflow
		}
		gas += nz * nonZeroGas

		z := uint64(len(data)) - nz
		if (math.MaxUint64-gas)/params.TxDataZeroGas < z {
			return 0, vm.ErrGasUintOverflow
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
		fee:      big.NewInt(0),
	}
}

// ApplyMessage computes the new state by applying the given message
// against the old state within the environment.
//
// ApplyMessage returns the bytes returned by any EVM execution (if it took place),
// the gas used (which includes gas refunds) and an error if it failed. An error always
// indicates a core error meaning that the message would always fail for that particular
// state and would never be accepted within a block.
func ApplyMessage(evm *vm.EVM, msg Message, gp *GasPool) (*ExecutionResult, error) {
	return NewStateTransition(evm, msg, gp).TransitionDb()
}

// to returns the recipient of the message.
func (st *StateTransition) to() common.Address {
	if st.msg == nil || st.msg.To() == nil /* contract creation */ {
		return common.Address{}
	}
	return *st.msg.To()
}

func (st *StateTransition) buyGas() error {
	mgval := new(big.Int).Mul(new(big.Int).SetUint64(st.msg.Gas()), st.gasPrice)
	mgval.Add(mgval, st.fee)
	if st.state.GetBalance(common.SystemAssetID, st.msg.From()).Cmp(mgval) < 0 {
		return ErrInsufficientFunds
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
// returning the evm execution result with following fields.
//
// - used gas:
//      total gas used (including gas being refunded)
// - returndata:
//      the returned data from evm
// - concrete execution error:
//      various **EVM** error which aborts the execution,
//      e.g. ErrOutOfGas, ErrExecutionReverted
//
// However if any consensus issue encountered, return the error directly with
// nil evm execution result.
func (st *StateTransition) TransitionDb() (*ExecutionResult, error) {
	msg := st.msg
	var fsnCallParam *common.FSNCallParam
	if common.IsFsnCall(msg.To()) {
		fsnCallParam = &common.FSNCallParam{}
		rlp.DecodeBytes(msg.Data(), fsnCallParam)
		st.fee = common.GetFsnCallFee(msg.To(), fsnCallParam.Func)
	}

	// First check this message satisfies all consensus rules before
	// applying the message. The rules include these clauses
	//
	// 1. the nonce of the message caller is correct
	// 2. caller has enough balance to cover transaction fee(gaslimit * gasprice)
	// 3. the amount of gas required is available in the block
	// 4. the purchased gas is enough to cover intrinsic usage
	// 5. there is no overflow when calculating intrinsic gas
	// 6. caller has enough balance to cover asset transfer for **topmost** call

	// Check clauses 1-3, buy gas if everything is correct
	if err := st.preCheck(); err != nil {
		return nil, err
	}
	sender := vm.AccountRef(msg.From())
	homestead := st.evm.ChainConfig().IsHomestead(st.evm.Context.BlockNumber)
	contractCreation := msg.To() == nil

	// Check clauses 4-5, subtract intrinsic gas if everything is correct
	gas, err := IntrinsicGas(st.data, contractCreation, homestead)
	if err != nil {
		return nil, err
	}
	if st.gas < gas {
		return nil, fmt.Errorf("%w: have %d, want %d", ErrIntrinsicGas, st.gas, gas)
	}
	st.gas -= gas

	// Check clause 6
	if msg.Value().Sign() > 0 && !st.evm.Context.CanTransfer(st.state, msg.From(), msg.Value()) {
		return nil, fmt.Errorf("%w: address %v", ErrInsufficientFundsForTransfer, msg.From().Hex())
	}
	var (
		ret   []byte
		vmerr error // vm errors do not effect consensus and are therefore not assigned to err
	)
	if contractCreation {
		ret, _, st.gas, vmerr = st.evm.Create(sender, st.data, st.gas, st.value)
	} else {
		// Increment the nonce for the next transaction
		st.state.SetNonce(msg.From(), st.state.GetNonce(sender.Address())+1)

		if fsnCallParam != nil {
			errc := st.handleFsnCall(fsnCallParam)
			if errc != nil {
				isInMining := st.evm.Context.MixDigest == (common.Hash{})
				if isInMining {
					// don't pack tx if handle FsnCall meet error
					return nil, errc
				}
				common.DebugInfo("handleFsnCall error", "number", st.evm.Context.BlockNumber, "Func", fsnCallParam.Func, "err", errc)
			}
		}

		ret, st.gas, vmerr = st.evm.Call(sender, st.to(), st.data, st.gas, st.value)
	}
	st.refundGas()
	minerFees := new(big.Int).Mul(new(big.Int).SetUint64(st.gasUsed()), st.gasPrice)
	if st.fee.Sign() > 0 {
		minerFees.Add(minerFees, st.fee)
	}
	st.state.AddBalance(st.evm.Context.Coinbase, common.SystemAssetID, minerFees)

	return &ExecutionResult{
		UsedGas:    st.gasUsed(),
		Err:        vmerr,
		ReturnData: ret,
	}, nil
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

func (st *StateTransition) handleFsnCall(param *common.FSNCallParam) error {
	height := st.evm.Context.BlockNumber
	timestamp := st.evm.Context.ParentTime.Uint64()

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
		if start < timestamp {
			start = timestamp
		}

		needValue := common.NewTimeLock(&common.TimeLockItem{
			StartTime: start,
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
		case common.SmartTransfer:
			if !common.IsSmartTransferEnabled(height) {
				st.addLog(common.TimeLockFunc, timeLockParam, common.NewKeyValue("LockType", "SmartTransfer"), common.NewKeyValue("Error", "not enabled"))
				return fmt.Errorf("SendTimeLock not enabled")
			}
			timeLockBalance := st.state.GetTimeLockBalance(timeLockParam.AssetID, st.msg.From())
			if timeLockBalance.Cmp(needValue) < 0 {
				timeLockValue := timeLockBalance.GetSpendableValue(start, end)
				assetBalance := st.state.GetBalance(timeLockParam.AssetID, st.msg.From())
				if new(big.Int).Add(timeLockValue, assetBalance).Cmp(timeLockParam.Value) < 0 {
					st.addLog(common.TimeLockFunc, timeLockParam, common.NewKeyValue("LockType", "SmartTransfer"), common.NewKeyValue("Error", "not enough balance"))
					return fmt.Errorf("not enough balance")
				}
				if timeLockValue.Sign() > 0 {
					subTimeLock := common.GetTimeLock(timeLockValue, start, end)
					st.state.SubTimeLockBalance(st.msg.From(), timeLockParam.AssetID, subTimeLock, height, timestamp)
				}
				useAssetAmount := new(big.Int).Sub(timeLockParam.Value, timeLockValue)
				st.state.SubBalance(st.msg.From(), timeLockParam.AssetID, useAssetAmount)
				surplus := common.GetSurplusTimeLock(useAssetAmount, start, end, timestamp)
				if !surplus.IsEmpty() {
					st.state.AddTimeLockBalance(st.msg.From(), timeLockParam.AssetID, surplus, height, timestamp)
				}
			} else {
				st.state.SubTimeLockBalance(st.msg.From(), timeLockParam.AssetID, needValue, height, timestamp)
			}

			if !common.IsWholeAsset(start, end, timestamp) {
				st.state.AddTimeLockBalance(timeLockParam.To, timeLockParam.AssetID, needValue, height, timestamp)
			} else {
				st.state.AddBalance(timeLockParam.To, timeLockParam.AssetID, timeLockParam.Value)
			}
			st.addLog(common.TimeLockFunc, timeLockParam, common.NewKeyValue("LockType", "SmartTransfer"), common.NewKeyValue("AssetID", timeLockParam.AssetID))
			return nil
		}
	case common.BuyTicketFunc:
		outputCommandInfo("BuyTicketFunc", "from", st.msg.From())
		from := st.msg.From()
		hash := st.evm.Context.GetHash(height.Uint64() - 1)
		id := crypto.Keccak256Hash(from[:], hash[:])

		if st.state.IsTicketExist(id) {
			st.addLog(common.BuyTicketFunc, param.Data, common.NewKeyValue("Error", "Ticket already exist"))
			return fmt.Errorf(id.String() + " Ticket already exist")
		}

		buyTicketParam := common.BuyTicketParam{}
		rlp.DecodeBytes(param.Data, &buyTicketParam)

		// check buy ticket param
		if common.IsHardFork(2, height) {
			if err := buyTicketParam.Check(height, timestamp); err != nil {
				st.addLog(common.BuyTicketFunc, param.Data, common.NewKeyValue("Error", err.Error()))
				return err
			}
		} else {
			if err := buyTicketParam.Check(height, 0); err != nil {
				st.addLog(common.BuyTicketFunc, param.Data, common.NewKeyValue("Error", err.Error()))
				return err
			}
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
			Owner: from,
			TicketBody: common.TicketBody{
				ID:         id,
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
		st.addLog(common.BuyTicketFunc, param.Data, common.NewKeyValue("TicketID", ticket.ID), common.NewKeyValue("TicketOwner", ticket.Owner))
		return nil
	case common.AssetValueChangeFunc:
		outputCommandInfo("AssetValueChangeFunc", "from", st.msg.From())
		assetValueChangeParamEx := common.AssetValueChangeExParam{}
		rlp.DecodeBytes(param.Data, &assetValueChangeParamEx)

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
			st.addLog(common.AssetValueChangeFunc, assetValueChangeParamEx, common.NewKeyValue("Error", "can only be changed by owner"))
			return fmt.Errorf("can only be changed by owner")
		}

		if asset.Owner != assetValueChangeParamEx.To && !assetValueChangeParamEx.IsInc {
			err := fmt.Errorf("decrement can only happen to asset's own account")
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
			if useAsset == false {
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

		if err := st.state.RemoveSwap(swap.ID); err != nil {
			st.addLog(common.RecallSwapFunc, recallSwapParam, common.NewKeyValue("Error", "Unable to remove swap"))
			return err
		}

		if swap.FromAssetID != common.OwnerUSANAssetID {
			total := new(big.Int).Mul(swap.MinFromAmount, swap.SwapSize)
			start := swap.FromStartTime
			end := swap.FromEndTime
			useAsset := start == common.TimeLockNow && end == common.TimeLockForever

			// return to the owner the balance
			if useAsset == true {
				st.state.AddBalance(st.msg.From(), swap.FromAssetID, total)
			} else {
				needValue := common.NewTimeLock(&common.TimeLockItem{
					StartTime: common.MaxUint64(start, timestamp),
					EndTime:   end,
					Value:     total,
				})
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

		if common.IsPrivateSwapCheckingEnabled(height) {
			if err := common.CheckSwapTargets(swap.Targes, st.msg.From()); err != nil {
				st.addLog(common.TakeSwapFunc, takeSwapParam, common.NewKeyValue("Error", err.Error()))
				return err
			}
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

		if fromUseAsset == false {
			fromNeedValue = common.NewTimeLock(&common.TimeLockItem{
				StartTime: common.MaxUint64(fromStart, timestamp),
				EndTime:   fromEnd,
				Value:     fromTotal,
			})
		}
		if toUseAsset == false {
			toNeedValue = common.NewTimeLock(&common.TimeLockItem{
				StartTime: common.MaxUint64(toStart, timestamp),
				EndTime:   toEnd,
				Value:     toTotal,
			})
		}

		if toUseAsset == true {
			if st.state.GetBalance(swap.ToAssetID, st.msg.From()).Cmp(toTotal) < 0 {
				st.addLog(common.TakeSwapFunc, takeSwapParam, common.NewKeyValue("Error", "not enough from asset"))
				return fmt.Errorf("not enough from asset")
			}
		} else {
			isValid := true
			if err := toNeedValue.IsValid(); err != nil {
				isValid = false
			}
			available := st.state.GetTimeLockBalance(swap.ToAssetID, st.msg.From())
			if isValid && available.Cmp(toNeedValue) < 0 {
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

		swapDeleted := "false"

		if swap.SwapSize.Cmp(takeSwapParam.Size) == 0 {
			if err := st.state.RemoveSwap(swap.ID); err != nil {
				st.addLog(common.TakeSwapFunc, takeSwapParam, common.NewKeyValue("Error", "System Error"))
				return err
			}
			swapDeleted = "true"
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
		st.addLog(common.TakeSwapFunc, takeSwapParam, common.NewKeyValue("SwapID", swap.ID), common.NewKeyValue("Deleted", swapDeleted))
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

		if err := st.state.RemoveMultiSwap(swap.ID); err != nil {
			st.addLog(common.RecallMultiSwapFunc, recallSwapParam, common.NewKeyValue("Error", "Unable to remove swap"))
			return err
		}

		ln := len(swap.FromAssetID)
		for i := 0; i < ln; i++ {
			total := new(big.Int).Mul(swap.MinFromAmount[i], swap.SwapSize)
			start := swap.FromStartTime[i]
			end := swap.FromEndTime[i]
			useAsset := start == common.TimeLockNow && end == common.TimeLockForever

			// return to the owner the balance
			if useAsset == true {
				st.state.AddBalance(st.msg.From(), swap.FromAssetID[i], total)
			} else {
				needValue := common.NewTimeLock(&common.TimeLockItem{
					StartTime: common.MaxUint64(start, timestamp),
					EndTime:   end,
					Value:     total,
				})

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

		accountBalances := make(map[common.Hash]*big.Int)
		accountTimeLockBalances := make(map[common.Hash]*common.TimeLock)

		for i := 0; i < ln; i++ {
			if _, exist := accountBalances[makeSwapParam.FromAssetID[i]]; !exist {
				balance := st.state.GetBalance(makeSwapParam.FromAssetID[i], st.msg.From())
				timelock := st.state.GetTimeLockBalance(makeSwapParam.FromAssetID[i], st.msg.From())
				accountBalances[makeSwapParam.FromAssetID[i]] = new(big.Int).Set(balance)
				accountTimeLockBalances[makeSwapParam.FromAssetID[i]] = timelock.Clone()
			}

			total[i] = new(big.Int).Mul(makeSwapParam.MinFromAmount[i], makeSwapParam.SwapSize)
			start := makeSwapParam.FromStartTime[i]
			end := makeSwapParam.FromEndTime[i]
			useAsset[i] = start == common.TimeLockNow && end == common.TimeLockForever
			if useAsset[i] == false {
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

		// check balances first
		for i := 0; i < ln; i++ {
			balance := accountBalances[makeSwapParam.FromAssetID[i]]
			timeLockBalance := accountTimeLockBalances[makeSwapParam.FromAssetID[i]]
			if useAsset[i] == true {
				if balance.Cmp(total[i]) < 0 {
					err = fmt.Errorf("not enough from asset")
					st.addLog(common.MakeMultiSwapFunc, makeSwapParam, common.NewKeyValue("Error", err.Error()))
					return err
				}
				balance.Sub(balance, total[i])
			} else {
				if timeLockBalance.Cmp(needValue[i]) < 0 {
					if balance.Cmp(total[i]) < 0 {
						err = fmt.Errorf("not enough time lock or asset balance")
						st.addLog(common.MakeMultiSwapFunc, makeSwapParam, common.NewKeyValue("Error", err.Error()))
						return err
					}

					balance.Sub(balance, total[i])
					totalValue := common.NewTimeLock(&common.TimeLockItem{
						StartTime: timestamp,
						EndTime:   common.TimeLockForever,
						Value:     total[i],
					})
					timeLockBalance.Add(timeLockBalance, totalValue)
				}
				timeLockBalance.Sub(timeLockBalance, needValue[i])
			}
		}

		// then deduct
		var deductErr error
		for i := 0; i < ln; i++ {
			if useAsset[i] == true {
				if st.state.GetBalance(makeSwapParam.FromAssetID[i], st.msg.From()).Cmp(total[i]) < 0 {
					deductErr = fmt.Errorf("not enough from asset")
					break
				}
			} else {
				available := st.state.GetTimeLockBalance(makeSwapParam.FromAssetID[i], st.msg.From())
				if available.Cmp(needValue[i]) < 0 {

					if st.state.GetBalance(makeSwapParam.FromAssetID[i], st.msg.From()).Cmp(total[i]) < 0 {
						deductErr = fmt.Errorf("not enough time lock or asset balance")
						break
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

			// take from the owner the asset
			if useAsset[i] == true {
				st.state.SubBalance(st.msg.From(), makeSwapParam.FromAssetID[i], total[i])
			} else {
				st.state.SubTimeLockBalance(st.msg.From(), makeSwapParam.FromAssetID[i], needValue[i], height, timestamp)
			}
		}

		if deductErr != nil {
			common.DebugInfo("MakeMultiSwapFunc deduct error, why check balance before have no effect?")
			st.addLog(common.MakeMultiSwapFunc, makeSwapParam, common.NewKeyValue("Error", deductErr.Error()))
			return deductErr
		}

		if err := st.state.AddMultiSwap(swap); err != nil {
			st.addLog(common.MakeMultiSwapFunc, makeSwapParam, common.NewKeyValue("Error", "System error can't add swap"))
			return err
		}

		st.addLog(common.MakeMultiSwapFunc, makeSwapParam, common.NewKeyValue("SwapID", swap.ID))
		return nil
	case common.TakeMultiSwapFunc:
		outputCommandInfo("TakeMultiSwapFunc", "from", st.msg.From())
		takeSwapParam := common.TakeMultiSwapParam{}
		rlp.DecodeBytes(param.Data, &takeSwapParam)

		swap, err := st.state.GetMultiSwap(takeSwapParam.SwapID)
		if err != nil {
			st.addLog(common.TakeMultiSwapFunc, takeSwapParam, common.NewKeyValue("Error", "swap not found"))
			return fmt.Errorf("Swap not found")
		}

		if err := takeSwapParam.Check(height, &swap, timestamp); err != nil {
			st.addLog(common.TakeMultiSwapFunc, takeSwapParam, common.NewKeyValue("Error", err.Error()))
			return err
		}

		if common.IsPrivateSwapCheckingEnabled(height) {
			if err := common.CheckSwapTargets(swap.Targes, st.msg.From()); err != nil {
				st.addLog(common.TakeMultiSwapFunc, takeSwapParam, common.NewKeyValue("Error", err.Error()))
				return err
			}
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

			if fromUseAsset[i] == false {
				fromNeedValue[i] = common.NewTimeLock(&common.TimeLockItem{
					StartTime: common.MaxUint64(fromStart[i], timestamp),
					EndTime:   fromEnd[i],
					Value:     fromTotal[i],
				})
			}
		}

		lnTo := len(swap.ToAssetID)

		toUseAsset := make([]bool, lnTo)
		toTotal := make([]*big.Int, lnTo)
		toStart := make([]uint64, lnTo)
		toEnd := make([]uint64, lnTo)
		toNeedValue := make([]*common.TimeLock, lnTo)

		accountBalances := make(map[common.Hash]*big.Int)
		accountTimeLockBalances := make(map[common.Hash]*common.TimeLock)

		for i := 0; i < lnTo; i++ {
			if _, exist := accountBalances[swap.ToAssetID[i]]; !exist {
				balance := st.state.GetBalance(swap.ToAssetID[i], st.msg.From())
				timelock := st.state.GetTimeLockBalance(swap.ToAssetID[i], st.msg.From())
				accountBalances[swap.ToAssetID[i]] = new(big.Int).Set(balance)
				accountTimeLockBalances[swap.ToAssetID[i]] = timelock.Clone()
			}

			toTotal[i] = new(big.Int).Mul(swap.MinToAmount[i], takeSwapParam.Size)
			toStart[i] = swap.ToStartTime[i]
			toEnd[i] = swap.ToEndTime[i]
			toUseAsset[i] = toStart[i] == common.TimeLockNow && toEnd[i] == common.TimeLockForever
			if toUseAsset[i] == false {
				toNeedValue[i] = common.NewTimeLock(&common.TimeLockItem{
					StartTime: common.MaxUint64(toStart[i], timestamp),
					EndTime:   toEnd[i],
					Value:     toTotal[i],
				})
			}
		}

		// check to account balances
		for i := 0; i < lnTo; i++ {
			balance := accountBalances[swap.ToAssetID[i]]
			timeLockBalance := accountTimeLockBalances[swap.ToAssetID[i]]
			if toUseAsset[i] == true {
				if balance.Cmp(toTotal[i]) < 0 {
					err = fmt.Errorf("not enough from asset")
					st.addLog(common.TakeMultiSwapFunc, takeSwapParam, common.NewKeyValue("Error", err.Error()))
					return err
				}
				balance.Sub(balance, toTotal[i])
			} else {
				if err := toNeedValue[i].IsValid(); err != nil {
					continue
				}
				if timeLockBalance.Cmp(toNeedValue[i]) < 0 {
					if balance.Cmp(toTotal[i]) < 0 {
						err = fmt.Errorf("not enough time lock or asset balance")
						st.addLog(common.TakeMultiSwapFunc, takeSwapParam, common.NewKeyValue("Error", err.Error()))
						return err
					}

					balance.Sub(balance, toTotal[i])
					totalValue := common.NewTimeLock(&common.TimeLockItem{
						StartTime: timestamp,
						EndTime:   common.TimeLockForever,
						Value:     toTotal[i],
					})
					timeLockBalance.Add(timeLockBalance, totalValue)
				}
				timeLockBalance.Sub(timeLockBalance, toNeedValue[i])
			}
		}

		// then deduct
		var deductErr error
		for i := 0; i < lnTo; i++ {
			if toUseAsset[i] == true {
				if st.state.GetBalance(swap.ToAssetID[i], st.msg.From()).Cmp(toTotal[i]) < 0 {
					deductErr = fmt.Errorf("not enough from asset")
					break
				}
				st.state.SubBalance(st.msg.From(), swap.ToAssetID[i], toTotal[i])
			} else {
				if err := toNeedValue[i].IsValid(); err != nil {
					continue
				}
				available := st.state.GetTimeLockBalance(swap.ToAssetID[i], st.msg.From())
				if available.Cmp(toNeedValue[i]) < 0 {

					if st.state.GetBalance(swap.ToAssetID[i], st.msg.From()).Cmp(toTotal[i]) < 0 {
						deductErr = fmt.Errorf("not enough time lock or asset balance")
						break
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
				st.state.SubTimeLockBalance(st.msg.From(), swap.ToAssetID[i], toNeedValue[i], height, timestamp)
			}
		}

		if deductErr != nil {
			common.DebugInfo("TakeMultiSwapFunc deduct error, why check balance before have no effect?")
			st.addLog(common.TakeMultiSwapFunc, takeSwapParam, common.NewKeyValue("Error", deductErr.Error()))
			return deductErr
		}

		swapDeleted := "false"

		if swap.SwapSize.Cmp(takeSwapParam.Size) == 0 {
			if err := st.state.RemoveMultiSwap(swap.ID); err != nil {
				st.addLog(common.TakeMultiSwapFunc, takeSwapParam, common.NewKeyValue("Error", "System Error"))
				return err
			}
			swapDeleted = "true"
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
			} else {
				if err := toNeedValue[i].IsValid(); err == nil {
					st.state.AddTimeLockBalance(swap.Owner, swap.ToAssetID[i], toNeedValue[i], height, timestamp)
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
		st.addLog(common.TakeMultiSwapFunc, takeSwapParam, common.NewKeyValue("SwapID", swap.ID), common.NewKeyValue("Deleted", swapDeleted))
		return nil
	case common.ReportIllegalFunc:
		if !common.IsMultipleMiningCheckingEnabled(height) {
			return fmt.Errorf("report not enabled")
		}
		report := param.Data
		header1, header2, err := datong.CheckAddingReport(st.state, report, height)
		if err != nil {
			return err
		}
		if err := st.state.AddReport(report); err != nil {
			return err
		}
		delTickets := datong.ProcessReport(header1, header2, st.msg.From(), st.state, height, timestamp)
		enc, _ := rlp.EncodeToBytes(delTickets)
		str := hexutil.Encode(enc)
		st.addLog(common.ReportIllegalFunc, "", common.NewKeyValue("DeleteTickets", str))
		common.DebugInfo("ReportIllegal", "reporter", st.msg.From(), "double-miner", header1.Coinbase, "current-block-height", height, "double-mining-height", header1.Number, "DeleteTickets", delTickets)
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
		BlockNumber: st.evm.Context.BlockNumber.Uint64(),
	})
}
