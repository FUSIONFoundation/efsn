package datong

import (
	"errors"

	"github.com/FusionFoundation/efsn/common"
	"github.com/FusionFoundation/efsn/core/types"
	"github.com/FusionFoundation/efsn/crypto"
)

// Snapshot wacom
type Snapshot struct {
	Selected     common.Hash   `json:"selected"`
	Retreat      []common.Hash `json:"retreat"`
	TicketNumber int           `json:"ticketNumber"`
}

type ticketLogType byte

const (
	ticketSelect ticketLogType = iota + 1
	ticketRetreat
)

type ticketLog struct {
	TicketID common.Hash
	Type     ticketLogType
}

type snapshot struct {
	logs         []*ticketLog
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

	snap.ticketNumber = common.BytesToInt(realData[:4])

	realData = realData[4:]
	dataLength := len(realData)
	logLength := common.HashLength + 1

	if dataLength%logLength != 0 {
		return errors.New("data length error")
	}

	count := dataLength / logLength
	snap.logs = make([]*ticketLog, count)
	for i := 0; i < count; i++ {
		base := logLength * i
		snap.logs[i] = &ticketLog{
			TicketID: common.BytesToHash(realData[base : logLength+base-1]),
			Type:     ticketLogType(realData[logLength+base-1]),
		}
	}

	return nil
}

func (snap *snapshot) Bytes() []byte {
	data := make([]byte, 0)

	data = append(data, common.IntToBytes(snap.ticketNumber)...)

	for i := 0; i < len(snap.logs); i++ {
		data = append(data, snap.logs[i].TicketID[:]...)
		data = append(data, byte(snap.logs[i].Type))
	}

	data = append(data, calcCheck(data))
	return data
}

func (snap *snapshot) TicketNumber() int {
	return snap.ticketNumber
}

func (snap *snapshot) SetTicketNumber(ticketNumber int) {
	snap.ticketNumber = ticketNumber
}

func (snap *snapshot) ToShow() *Snapshot {
	var retreat []common.Hash
	if len(snap.logs) == 0 {
		retreat = make([]common.Hash, 0, 0 )
	} else {
		retreat = make([]common.Hash, 0, len(snap.logs)-1)
		for i := 1; i < len(snap.logs); i++ {
			if snap.logs[i].Type == ticketRetreat {
				retreat = append(retreat, snap.logs[i].TicketID)
			}
		}
	}
	return &Snapshot{
		Selected:     snap.GetVoteTicket(),
		Retreat:      retreat,
		TicketNumber: snap.ticketNumber,
	}
}

func calcCheck(data []byte) byte {
	hash := crypto.Keccak256Hash(data)
	return hash[0]
}
