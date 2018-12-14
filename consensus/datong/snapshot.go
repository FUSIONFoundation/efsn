package datong

import (
	"math/big"

	"github.com/FusionFoundation/efsn/common"
)

type ticketLogType byte

const (
	ticketSelect ticketLogType = iota + 1
	ticketRetreat
	ticketExpired
)

type ticketLog struct {
	TicketID common.Hash
	Type     ticketLogType
}

type snapshot struct {
	logs   []*ticketLog
	weight *big.Int
}

func newSnapshot() *snapshot {
	return &snapshot{
		logs: make([]*ticketLog, 0),
	}

}

func newSnapshotWithData(data []byte) (*snapshot, error) {
	snap := newSnapshot()
	if err := snap.SetBytes(data); err != nil {
		return nil, err
	}
	return snap, nil
}

func (snap *snapshot) GetVoteTicket() common.Hash {
	if len(snap.logs) > 0 && snap.logs[0].Type == ticketSelect {
		return snap.logs[0].TicketID
	}
	return common.Hash{}
}

func (snap *snapshot) AddLog(log *ticketLog) {
	snap.logs = append(snap.logs, log)
}

func (snap *snapshot) SetBytes(data []byte) error {
	return nil
}

func (snap *snapshot) Bytes() []byte {
	return nil
}

func (snap *snapshot) Weight() *big.Int {
	return snap.weight
}

func (snap *snapshot) SetWeight(weight *big.Int) {
	snap.weight = new(big.Int).SetBytes(weight.Bytes())
}
