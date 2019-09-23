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
	739500, // fork 1
}

// testnet fork heights
var TESTNET_FORKS = []uint64{
	534500, // fork 1
}

const (
	PosV1 = iota + 1
	PosV2
)

func GetForkHeight(n int) uint64 {
	if UseDevnetRule || n <= 0 {
		return 0
	}
	if UseTestnetRule {
		if n <= len(TESTNET_FORKS) {
			return TESTNET_FORKS[n-1]
		}
		return math.MaxUint64
	}
	if n <= len(MAINNET_FORKS) {
		return MAINNET_FORKS[n-1]
	}
	return math.MaxUint64
}

func IsHardFork(n int, blockNumber *big.Int) bool {
	return blockNumber.Uint64() >= GetForkHeight(n)
}

func GetPoSHashVersion(blockNumber *big.Int) int {
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
