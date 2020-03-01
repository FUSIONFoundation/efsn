package common

import (
	"math"
	"math/big"
)

var (
	UseTestnetRule = false
	UseDevnetRule  = false
)

// mainnet fork heights
var MAINNET_FORKS = []uint64{
	739500,  // fork 1
	1818300, // fork 2
}

// testnet fork heights
var TESTNET_FORKS = []uint64{
	534500,  // fork 1
	1577000, // fork 2
}

const (
	PosV1 = iota + 1
	PosV2
	PosV3
)

func GetForkHeight(n int) uint64 {
	if UseDevnetRule || n <= 0 {
		return 0
	}
	forkArray := MAINNET_FORKS
	if UseTestnetRule {
		forkArray = TESTNET_FORKS
	}
	if n <= len(forkArray) {
		return forkArray[n-1]
	}
	return math.MaxUint64
}

func IsHardFork(n int, blockNumber *big.Int) bool {
	return blockNumber == nil || blockNumber.Uint64() >= GetForkHeight(n)
}

func GetPoSHashVersion(blockNumber *big.Int) int {
	if IsHardFork(2, blockNumber) {
		return PosV3
	}
	if IsHardFork(1, blockNumber) {
		return PosV2
	}
	return PosV1
}

func IsPrivateSwapCheckingEnabled(blockNumber *big.Int) bool {
	return IsHardFork(1, blockNumber)
}

func IsHeaderSnapCheckingEnabled(blockNumber *big.Int) bool {
	return IsHardFork(1, blockNumber)
}

func IsMultipleMiningCheckingEnabled(blockNumber *big.Int) bool {
	return IsHardFork(1, blockNumber)
}

func IsSmartTransferEnabled(blockNumber *big.Int) bool {
	return IsHardFork(2, blockNumber)
}
