package datong

import (
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// Snapshot wacom
type Snapshot struct {
	Selected     common.Hash   `json:"selected"`
	Retreat      []common.Hash `json:"retreat"`
	Expired      []common.Hash `json:"expired"`
	Deleted      []common.Hash `json:"deleted"`
	Weight       *big.Int      `json:"weight"`
	TicketWeight *big.Int      `json:"ticketWeight"`
	TicketNumber int           `json:"ticketNumber"`
}

type ticketLogType byte

const (
	ticketSelect ticketLogType = iota + 1
	ticketRetreat
	ticketExpired
	ticketDelete
)

type ticketLog struct {
	TicketID common.Hash
	Type     ticketLogType
}

type snapshot struct {
	logs         []*ticketLog
	weight       *big.Int
	ticketWeight *big.Int
	ticketNumber int
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

// NewSnapshotFromHeader wacom
func NewSnapshotFromHeader(header *types.Header) (*Snapshot, error) {
	snap := newSnapshot()
	if err := snap.SetBytes(getSnapDataByHeader(header)); err != nil {
		return nil, err
	}
	return snap.ToShow(), nil
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
	ticketWeightLength := common.BytesToInt(realData[4:8])
	snap.ticketNumber = common.BytesToInt(realData[8:12])

	indexTotal := 12
	start := weightLength + ticketWeightLength + indexTotal

	weightBytes := realData[indexTotal : indexTotal+weightLength]
	snap.weight = new(big.Int).SetBytes(weightBytes)

	ticketWeightBytes := realData[indexTotal+weightLength : indexTotal+weightLength+ticketWeightLength]
	snap.ticketWeight = new(big.Int).SetBytes(ticketWeightBytes)

	realData = realData[start:]

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
	ticketWeightBytes := snap.ticketWeight.Bytes()

	data = append(data, common.IntToBytes(len(weightBytes))...)
	data = append(data, common.IntToBytes(len(ticketWeightBytes))...)
	data = append(data, common.IntToBytes(snap.ticketNumber)...)

	data = append(data, weightBytes...)
	data = append(data, ticketWeightBytes...)

	for i := 0; i < len(snap.logs); i++ {
		data = append(data, snap.logs[i].TicketID[:]...)
		data = append(data, byte(snap.logs[i].Type))
	}

	data = append(data, calcCheck(data))
	return data
}

func (snap *snapshot) Weight() *big.Int {
	return snap.weight
}

func (snap *snapshot) SetWeight(weight *big.Int) {
	snap.weight = new(big.Int).SetBytes(weight.Bytes())
}

func (snap *snapshot) TicketWeight() *big.Int {
	return snap.ticketWeight
}

func (snap *snapshot) SetTicketWeight(ticketWeight *big.Int) {
	snap.ticketWeight = new(big.Int).SetBytes(ticketWeight.Bytes())
}

func (snap *snapshot) TicketNumber() int {
	return snap.ticketNumber
}

func (snap *snapshot) SetTicketNumber(ticketNumber int) {
	snap.ticketNumber = ticketNumber
}

func (snap *snapshot) ToShow() *Snapshot {
	retreat := make([]common.Hash, 0)
	expired := make([]common.Hash, 0)
	deleted := make([]common.Hash, 0)
	for i := 0; i < len(snap.logs); i++ {
		if snap.logs[i].Type == ticketRetreat {
			retreat = append(retreat, snap.logs[i].TicketID)
		} else if snap.logs[i].Type == ticketExpired {
			expired = append(expired, snap.logs[i].TicketID)
		} else if snap.logs[i].Type == ticketDelete {
			deleted = append(deleted, snap.logs[i].TicketID)
		}

	}
	return &Snapshot{
		Selected:     snap.GetVoteTicket(),
		Retreat:      retreat,
		Expired:      expired,
		Deleted:      deleted,
		Weight:       snap.weight,
		TicketWeight: snap.ticketWeight,
		TicketNumber: snap.ticketNumber,
	}
}

func calcCheck(data []byte) byte {
	var check byte
	for i := 0; i < len(data); i++ {
		check += data[i]
	}
	return check
}
