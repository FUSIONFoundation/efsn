package common

import (
	"bytes"
	"errors"
	"math/big"

	"golang.org/x/crypto/sha3"
)

var (
	// If a contract want to receive Fusion Asset and TimeLock from an EOA,
	// the contract must impl the following 'receiveAsset' interface.
	ReceiveAssetFuncHash = Keccak256Hash([]byte("receiveAsset(bytes32,uint64,uint64,uint8,uint256[])")) // = 0xda28283ac3f28139b37690353d09e3b910702b960661a29c039ad0e5b6460329

	LogFusionAssetReceivedTopic = Keccak256Hash([]byte("LogFusionAssetReceived(bytes32,address,uint256,uint64,uint64,uint8)")) // = 0x8a9c8666f57c1ade38343d03dc1d891f209af2efb28c551a4a2c96160e7a2a6b

	LogFusionAssetSentTopic = Keccak256Hash([]byte("LogFusionAssetSent(bytes32,address,uint256,uint64,uint64,uint8)")) // = 0xf9c07f165baf6a7868a16aa9de8b6f41fe0849ba33af6ece038847047c6606e7
)

func Keccak256Hash(data ...[]byte) (h Hash) {
	d := sha3.NewLegacyKeccak256()
	for _, b := range data {
		d.Write(b)
	}
	d.Sum(h[:0])
	return h
}

func MinUint64(x, y uint64) uint64 {
	if x <= y {
		return x
	}
	return y
}

func MaxUint64(x, y uint64) uint64 {
	if x < y {
		return y
	}
	return x
}

func GetData(data []byte, start uint64, size uint64) []byte {
	length := uint64(len(data))
	if start > length {
		start = length
	}
	end := start + size
	if end > length {
		end = length
	}
	return RightPadBytes(data[start:end], int(size))
}

func BigUint64(v *big.Int) (uint64, bool) {
	return v.Uint64(), !v.IsUint64()
}

func GetBigInt(data []byte, start uint64, size uint64) *big.Int {
	return new(big.Int).SetBytes(GetData(data, start, size))
}

func GetUint64(data []byte, start uint64, size uint64) (uint64, bool) {
	return BigUint64(GetBigInt(data, start, size))
}

type FcSendAssetFlag uint8

const (
	FcUseAny                FcSendAssetFlag = iota // 0
	FcUseAnyToTimeLock                             // 1
	FcUseTimeLock                                  // 2
	FcUseTimeLockToTimeLock                        // 3
	FcUseAsset                                     // 4
	FcUseAssetToTimeLock                           // 5
	FcInvalidSendAssetFlag
)

func (flag FcSendAssetFlag) IsUseTimeLock() bool {
	return flag == FcUseTimeLock || flag == FcUseTimeLockToTimeLock
}

func (flag FcSendAssetFlag) IsUseAsset() bool {
	return flag == FcUseAsset || flag == FcUseAssetToTimeLock
}

func (flag FcSendAssetFlag) IsToTimeLock() bool {
	return flag == FcUseAnyToTimeLock ||
		flag == FcUseTimeLockToTimeLock ||
		flag == FcUseAssetToTimeLock
}

func IsReceiveAssetPayableTx(blockNumber *big.Int, input []byte) bool {
	if !IsHardFork(2, blockNumber) {
		return false
	}
	inputLen := uint64(len(input))
	if inputLen < 196 || !bytes.Equal(input[:4], ReceiveAssetFuncHash[:4]) {
		return false
	}
	biFlag := GetBigInt(input, 100, 32)
	if biFlag.Cmp(big.NewInt(int64(FcInvalidSendAssetFlag))) >= 0 {
		return false
	}
	offset, overflow := GetUint64(input, 132, 32)
	if offset != 160 || overflow {
		return false
	}
	length, overflow := GetUint64(input, 164, 32)
	if length > 20 || overflow {
		return false
	}
	return inputLen == 196+length*32
}

type TransferTimeLockParam struct {
	AssetID     Hash
	StartTime   uint64
	EndTime     uint64
	Timestamp   uint64
	Flag        FcSendAssetFlag
	Value       *big.Int
	GasValue    *big.Int
	BlockNumber *big.Int
	IsReceive   bool
}

func ParseReceiveAssetPayableTxInput(p *TransferTimeLockParam, input []byte, timestamp uint64) error {
	p.IsReceive = true
	p.Timestamp = timestamp
	p.AssetID = BytesToHash(GetData(input, 4, 32))
	var overflow bool
	if p.StartTime, overflow = GetUint64(input, 36, 32); overflow {
		return errors.New("start time overflow")
	}
	if p.EndTime, overflow = GetUint64(input, 68, 32); overflow {
		return errors.New("end time overflow")
	}
	biFlag := GetBigInt(input, 100, 32)
	if biFlag.Cmp(big.NewInt(int64(FcInvalidSendAssetFlag))) >= 0 {
		return errors.New("invalid send asset flag")
	}
	p.Flag = FcSendAssetFlag(biFlag.Uint64())
	// adjust
	if p.StartTime < timestamp {
		p.StartTime = timestamp
	}
	if p.EndTime == 0 {
		p.EndTime = TimeLockForever
	}
	// check
	if p.StartTime > p.EndTime {
		return errors.New("start time > end time")
	}
	return nil
}
