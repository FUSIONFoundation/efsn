// Copyright 2016 The go-ethereum Authors
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
	"math/big"

	"github.com/FusionFoundation/efsn/common"
	"github.com/FusionFoundation/efsn/consensus"
	"github.com/FusionFoundation/efsn/core/types"
	"github.com/FusionFoundation/efsn/core/vm"
)

// ChainContext supports retrieving headers and consensus parameters from the
// current blockchain to be used during transaction processing.
type ChainContext interface {
	// Engine retrieves the chain's consensus engine.
	Engine() consensus.Engine

	// GetHeader returns the hash corresponding to their hash.
	GetHeader(common.Hash, uint64) *types.Header
}

// NewEVMContext creates a new context for use in the EVM.
func NewEVMContext(msg Message, header *types.Header, chain ChainContext, author *common.Address) vm.Context {
	// If we don't have an explicit author (i.e. not mining), extract from the header
	var beneficiary common.Address
	if author == nil {
		beneficiary, _ = chain.Engine().Author(header) // Ignore error, we're past header validation
	} else {
		beneficiary = *author
	}
	parentTime := big.NewInt(0)
	parent := chain.GetHeader(header.ParentHash, header.Number.Uint64()-1)
	if parent != nil {
		parentTime = parent.Time
	}
	return vm.Context{
		CanTransfer: CanTransfer,
		Transfer:    Transfer,
		GetHash:     GetHashFn(header, chain),
		Origin:      msg.From(),
		Coinbase:    beneficiary,
		BlockNumber: new(big.Int).Set(header.Number),
		Time:        new(big.Int).Set(header.Time),
		ParentTime:  new(big.Int).Set(parentTime),
		Difficulty:  new(big.Int).Set(header.Difficulty),
		GasLimit:    header.GasLimit,
		GasPrice:    new(big.Int).Set(msg.GasPrice()),
		MixDigest:   header.MixDigest,

		CanTransferTimeLock: CanTransferTimeLock,
		TransferTimeLock:    TransferTimeLock,
	}
}

// GetHashFn returns a GetHashFunc which retrieves header hashes by number
func GetHashFn(ref *types.Header, chain ChainContext) func(n uint64) common.Hash {
	var cache map[uint64]common.Hash

	return func(n uint64) common.Hash {
		// If there's no hash cache yet, make one
		if cache == nil {
			cache = map[uint64]common.Hash{
				ref.Number.Uint64() - 1: ref.ParentHash,
			}
		}
		// Try to fulfill the request from the cache
		if hash, ok := cache[n]; ok {
			return hash
		}
		// Not cached, iterate the blocks and cache the hashes
		for header := chain.GetHeader(ref.ParentHash, ref.Number.Uint64()-1); header != nil; header = chain.GetHeader(header.ParentHash, header.Number.Uint64()-1) {
			cache[header.Number.Uint64()-1] = header.ParentHash
			if n == header.Number.Uint64()-1 {
				return header.ParentHash
			}
		}
		return common.Hash{}
	}
}

// CanTransfer checks whether there are enough funds in the address' account to make a transfer.
// This does not take the necessary gas in to account to make the transfer valid.
func CanTransfer(db vm.StateDB, addr common.Address, amount *big.Int) bool {
	return db.GetBalance(common.SystemAssetID, addr).Cmp(amount) >= 0
}

// Transfer subtracts amount from sender and adds amount to recipient using the given Db
func Transfer(db vm.StateDB, sender, recipient common.Address, amount *big.Int) {
	db.SubBalance(sender, common.SystemAssetID, amount)
	db.AddBalance(recipient, common.SystemAssetID, amount)
}

func CanTransferTimeLock(db vm.StateDB, addr common.Address, p *common.TransferTimeLockParam) bool {
	if p.Value.Sign() <= 0 {
		return true
	}
	timelock := common.GetTimeLock(p.Value, p.StartTime, p.EndTime)
	if err := timelock.IsValid(); err != nil {
		return false
	}

	assetBalance := db.GetBalance(p.AssetID, addr)
	if p.GasValue != nil && p.GasValue.Sign() > 0 {
		if p.AssetID == common.SystemAssetID {
			if assetBalance.Cmp(p.GasValue) < 0 {
				return false
			}
			assetBalance = new(big.Int).Sub(assetBalance, p.GasValue)
		} else if db.GetBalance(common.SystemAssetID, addr).Cmp(p.GasValue) < 0 {
			return false
		}
	}

	if p.Flag.IsUseAsset() {
		if assetBalance.Cmp(p.Value) < 0 {
			return false
		}
	} else {
		timeLockBalance := db.GetTimeLockBalance(p.AssetID, addr)
		if timeLockBalance.Cmp(timelock) < 0 {
			if p.Flag.IsUseTimeLock() {
				return false
			}
			timeLockValue := timeLockBalance.GetSpendableValue(p.StartTime, p.EndTime)
			if new(big.Int).Add(timeLockValue, assetBalance).Cmp(p.Value) < 0 {
				return false
			}
		}
	}
	return true
}

func TransferTimeLock(db vm.StateDB, sender, recipient common.Address, p *common.TransferTimeLockParam) {
	if p.Value.Sign() <= 0 {
		return
	}
	timelock := common.GetTimeLock(p.Value, p.StartTime, p.EndTime)
	if err := timelock.IsValid(); err != nil {
		return
	}
	if p.Flag.IsUseAsset() {
		assetBalance := db.GetBalance(p.AssetID, sender)
		if assetBalance.Cmp(p.Value) < 0 {
			return
		}
		db.SubBalance(sender, p.AssetID, p.Value)
		surplus := common.GetSurplusTimeLock(p.Value, p.StartTime, p.EndTime, p.Timestamp)
		if !surplus.IsEmpty() {
			db.AddTimeLockBalance(sender, p.AssetID, surplus, p.BlockNumber, p.Timestamp)
		}
	} else {
		timeLockBalance := db.GetTimeLockBalance(p.AssetID, sender)
		if timeLockBalance.Cmp(timelock) < 0 {
			if p.Flag.IsUseTimeLock() {
				return
			}
			timeLockValue := timeLockBalance.GetSpendableValue(p.StartTime, p.EndTime)
			assetBalance := db.GetBalance(p.AssetID, sender)
			if new(big.Int).Add(timeLockValue, assetBalance).Cmp(p.Value) < 0 {
				return
			}
			if timeLockValue.Sign() > 0 {
				subTimeLock := common.GetTimeLock(timeLockValue, p.StartTime, p.EndTime)
				db.SubTimeLockBalance(sender, p.AssetID, subTimeLock, p.BlockNumber, p.Timestamp)
			}
			useAssetAmount := new(big.Int).Sub(p.Value, timeLockValue)
			db.SubBalance(sender, p.AssetID, useAssetAmount)
			surplus := common.GetSurplusTimeLock(useAssetAmount, p.StartTime, p.EndTime, p.Timestamp)
			if !surplus.IsEmpty() {
				db.AddTimeLockBalance(sender, p.AssetID, surplus, p.BlockNumber, p.Timestamp)
			}
		} else {
			db.SubTimeLockBalance(sender, p.AssetID, timelock, p.BlockNumber, p.Timestamp)
		}
	}

	if p.Flag.IsToTimeLock() || !common.IsWholeAsset(p.StartTime, p.EndTime, p.Timestamp) {
		db.AddTimeLockBalance(recipient, p.AssetID, timelock, p.BlockNumber, p.Timestamp)
	} else {
		db.AddBalance(recipient, p.AssetID, p.Value)
	}

	logData := make([]byte, 128)
	copy(logData[0:32], common.BigToHash(p.Value).Bytes())
	copy(logData[32:64], common.BigToHash(new(big.Int).SetUint64(p.StartTime)).Bytes())
	copy(logData[64:96], common.BigToHash(new(big.Int).SetUint64(p.EndTime)).Bytes())
	copy(logData[96:128], common.BigToHash(big.NewInt(int64(p.Flag))).Bytes())

	if p.IsReceive {
		db.AddLog(&types.Log{
			Address: recipient,
			Topics: []common.Hash{
				common.LogFusionAssetReceivedTopic,
				p.AssetID,
				common.BytesToHash(sender.Bytes()),
			},
			Data:        logData,
			BlockNumber: p.BlockNumber.Uint64(),
		})
	} else {
		db.AddLog(&types.Log{
			Address: sender,
			Topics: []common.Hash{
				common.LogFusionAssetSentTopic,
				p.AssetID,
				common.BytesToHash(recipient.Bytes()),
			},
			Data:        logData,
			BlockNumber: p.BlockNumber.Uint64(),
		})
	}
}
