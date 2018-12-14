package datong

import (
	"errors"
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
	if len(data) <= 0 {
		return errors.New("Empty data")
	}

	realData := data[:len(data)-1]
	check := data[len(data)-1]

	if calcCheck(realData) != check {
		return errors.New("check error")
	}
	weightLength := common.BytesToInt(realData[0:4])
	weightBytes := realData[len(realData)-weightLength:]
	snap.weight = new(big.Int).SetBytes(weightBytes)
	realData = realData[4 : len(realData)-weightLength]

	logLength := common.HashLength + 1

	if len(realData)%logLength != 0 {
		return errors.New("data length error")
	}

	for i := 0; i < len(realData)/(common.HashLength+1); i++ {
		base := logLength * i
		log := &ticketLog{
			TicketID: common.BytesToHash(realData[base : logLength+base-1]),
			Type:     ticketLogType(realData[logLength+base-1]),
		}
		snap.logs = append(snap.logs, log)
	}

	return nil
}

func (snap *snapshot) Bytes() []byte {
	data := make([]byte, 0)
	weightBytes := snap.weight.Bytes()
	data = append(data, common.IntToBytes(len(weightBytes))...)

	for i := 0; i < len(snap.logs); i++ {
		data = append(data, snap.logs[i].TicketID[:]...)
		data = append(data, byte(snap.logs[i].Type))
	}

	data = append(data, weightBytes...)

	data = append(data, calcCheck(data))
	return data
}

func (snap *snapshot) Weight() *big.Int {
	return snap.weight
}

func (snap *snapshot) SetWeight(weight *big.Int) {
	snap.weight = new(big.Int).SetBytes(weight.Bytes())
}

func calcCheck(data []byte) byte {
	var check byte
	for i := 0; i < len(data); i++ {
		check += data[i]
	}
	return check
}
