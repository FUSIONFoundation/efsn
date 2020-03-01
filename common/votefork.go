package common

import (
	"errors"
	"math/big"
)

var (
	ErrTransactionsFrozen = errors.New("all transactions unless buytickets are frozen")
	ErrAccountFrozen      = errors.New("account frozen")
)

var (
	VOTE1_FREEZE_TX_START uint64 = 739500
	VOTE1_FREEZE_TX_END   uint64 = 786000

	Vote1RefundAddress = HexToAddress("0xff948d492c31814dEde4CAA0af8824eF02Eb48D2")
	Vote1DrainList     = []Address{
		HexToAddress("0xb66cce16736feb5a50a9883675708027d3427c3c"),
		HexToAddress("0x782da6fb0562074ec21942a7829064a8c2bb05c4"),
		HexToAddress("0x6deed6878d062cd8754b3284306be814b215e332"),
	}
)

func IsVote1ForkBlock(blockNumber *big.Int) bool {
	return blockNumber.Uint64() == VOTE1_FREEZE_TX_END
}
