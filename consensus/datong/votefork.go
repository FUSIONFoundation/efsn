package datong

import (
	"github.com/FusionFoundation/efsn/v5/common"
	"github.com/FusionFoundation/efsn/v5/core/state"
)

// -------------------------- vote1 fork -------------------------
func ApplyVote1HardFork(statedb *state.StateDB, timestamp uint64) {
	for _, addr := range common.Vote1DrainList {
		statedb.TransferAll(addr, common.Vote1RefundAddress, timestamp)
	}
}
