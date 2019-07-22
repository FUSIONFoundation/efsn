package common

import (
	"math"
	"math/big"
)

var (
	UseTestnetRule = false
	UseDevnetRule  = false
)

func GetPoSHashVersion(blockNumber *big.Int) int {
	if UseDevnetRule {
		return 2
	}
	if UseTestnetRule {
		switch {
		case blockNumber.Uint64() >= math.MaxUint64:
			return 2
		default:
			return 1
		}
	}
	switch {
	case blockNumber.Uint64() >= math.MaxUint64:
		return 2
	default:
		return 1
	}
}

func IsPrivateSwapCheckingEnabled(blockNumber *big.Int) bool {
	if UseDevnetRule {
		return true
	}
	if UseTestnetRule {
		return blockNumber.Uint64() >= math.MaxUint64
	}
	return blockNumber.Uint64() >= math.MaxUint64
}

func IsHeaderSnapCheckingEnabled(blockNumber *big.Int) bool {
	if UseDevnetRule {
		return true
	}
	if UseTestnetRule {
		return blockNumber.Uint64() >= math.MaxUint64
	}
	return blockNumber.Uint64() >= math.MaxUint64
}

func IsMultipleMiningCheckingEnabled(blockNumber *big.Int) bool {
	if UseDevnetRule {
		return true
	}
	if UseTestnetRule {
		return blockNumber.Uint64() >= math.MaxUint64
	}
	return blockNumber.Uint64() >= math.MaxUint64
}
