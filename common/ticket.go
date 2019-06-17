package common

import (
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/FusionFoundation/efsn/log"
)

// TicketPrice  place holder for ticket price
func TicketPrice(blocknumber *big.Int) *big.Int {
	oneFSN := big.NewInt(1000000000000000000)
	return new(big.Int).Mul(big.NewInt(5000), oneFSN)
}

// Ticket wacom
type TicketBody struct {
	Height     uint64
	StartTime  uint64
	ExpireTime uint64
}

type TicketBodySlice []TicketBody

type Ticket struct {
	ID Hash
	TicketBody
}

type TicketSlice []Ticket
type TicketPtrSlice []*Ticket

type TicketDisplay struct {
	Owner      Address
	Height     uint64
	StartTime  uint64
	ExpireTime uint64
	Value      *big.Int
}

type TicketsData struct {
	Owner   Address
	Tickets TicketBodySlice
}

type TicketsDataSlice []TicketsData

func (t *TicketBody) IsInGenesis() bool {
	return t.Height == 0
}

func (t *Ticket) Owner() Address {
	return BytesToAddress(t.ID[:AddressLength])
}

func (t *TicketBody) BlockHeight() *big.Int {
	return new(big.Int).SetUint64(t.Height)
}

func (t *TicketBody) Value() *big.Int {
	return TicketPrice(new(big.Int).SetUint64(t.Height))
}

func TicketID(owner Address, height uint64, index uint64) Hash {
	h := Hash{}
	copy(h[:20], owner[:])
	copy(h[20:28], Uint64ToBytes(height))
	if height == 0 {
		copy(h[28:32], Uint32ToBytes(uint32(index)))
	}
	return h
}

func ParseTicketID(id Hash) (Address, uint64, uint64) {
	owner := BytesToAddress(id[:20])
	height := BytesToUint64(id[20:28])
	index := BytesToUint32(id[28:32])
	return owner, height, uint64(index)
}

func (t *Ticket) MarshalJSON() ([]byte, error) {
	return json.Marshal(&struct {
		ID         Hash
		Owner      Address
		Height     uint64
		StartTime  uint64
		ExpireTime uint64
		Value      string
	}{
		ID:         t.ID,
		Owner:      t.Owner(),
		Height:     t.Height,
		StartTime:  t.StartTime,
		ExpireTime: t.ExpireTime,
		Value:      t.Value().String(),
	})
}

func (t *Ticket) String() string {
	b, _ := json.Marshal(t)
	return string(b)
}

func (s TicketSlice) String() string {
	b, _ := json.Marshal(s)
	return string(b)
}

func (s TicketPtrSlice) String() string {
	b, _ := json.Marshal(s)
	return string(b)
}

func (t *Ticket) ToDisplay() TicketDisplay {
	return TicketDisplay{
		Owner:      t.Owner(),
		Height:     t.Height,
		StartTime:  t.StartTime,
		ExpireTime: t.ExpireTime,
		Value:      t.Value(),
	}
}

func (s TicketSlice) ToMap() map[Hash]TicketDisplay {
	r := make(map[Hash]TicketDisplay, len(s))
	for _, t := range s {
		r[t.ID] = t.ToDisplay()
	}
	return r
}

func (s TicketSlice) DeepCopy() TicketSlice {
	r := make(TicketSlice, len(s))
	for i, t := range s {
		r[i] = t
	}
	return r
}

func (s TicketSlice) Get(id Hash) (*Ticket, error) {
	for _, t := range s {
		if t.ID == id {
			return &t, nil
		}
	}
	return nil, fmt.Errorf("%v ticket not fount", id.String())
}

func (s TicketSlice) AddTicket(ticket *Ticket) (TicketSlice, error) {
	for _, t := range s {
		if t.ID == ticket.ID {
			return s, fmt.Errorf("AddTicket: %v ticket exist", t.ID.String())
		}
	}
	s = append(s, *ticket)
	return s, nil
}

func (s TicketSlice) RemoveTicket(id Hash) (TicketSlice, error) {
	for i, t := range s {
		if t.ID == id {
			s = append(s[:i], s[i+1:]...)
			return s, nil
		}
	}
	return nil, fmt.Errorf("RemoveTicket: %v ticket not fount", id.String())
}

func (s TicketBodySlice) DeepCopy() TicketBodySlice {
	res := make(TicketBodySlice, len(s))
	for i, v := range s {
		res[i] = v
	}
	return res
}

func (s TicketsData) DeepCopy() TicketsData {
	return TicketsData{
		Owner:   s.Owner,
		Tickets: s.Tickets.DeepCopy(),
	}
}

func (s TicketsData) ToMap() map[Hash]TicketDisplay {
	return s.ToTicketSlice().ToMap()
}

func (s TicketsData) ToTicketSlice() TicketSlice {
	res := make(TicketSlice, len(s.Tickets))
	for i, t := range s.Tickets {
		res[i] = Ticket{
			ID:         TicketID(s.Owner, t.Height, t.StartTime),
			TicketBody: t,
		}
	}
	return res
}

func (s TicketsDataSlice) DeepCopy() TicketsDataSlice {
	res := make(TicketsDataSlice, len(s))
	for i, v := range s {
		res[i] = v.DeepCopy()
	}
	return res
}

func (s TicketsDataSlice) ToMap() map[Hash]TicketDisplay {
	return s.ToTicketSlice().ToMap()
}

func (s TicketsDataSlice) ToTicketSlice() TicketSlice {
	res := make(TicketSlice, 0, s.NumberOfTickets())
	for _, v := range s {
		res = append(res, v.ToTicketSlice()...)
	}
	return res
}

func (s TicketsDataSlice) NumberOfTicketsByAddress(addr Address) uint64 {
	for _, v := range s {
		if v.Owner == addr {
			return uint64(len(v.Tickets))
		}
	}
	return 0
}

func (s TicketsDataSlice) NumberOfTickets() uint64 {
	numTickets := 0
	for _, v := range s {
		numTickets += len(v.Tickets)
	}
	return uint64(numTickets)
}

func (s TicketsDataSlice) NumberOfOwners() uint64 {
	return uint64(len(s))
}

func (s TicketsDataSlice) NumberOfTicketsAndOwners() (uint64, uint64) {
	return s.NumberOfTickets(), s.NumberOfOwners()
}

func (s TicketsDataSlice) Get(id Hash) (*Ticket, error) {
	owner, height, index := ParseTicketID(id)
	isInGenesis := height == 0
	var tickets TicketBodySlice
	for _, v := range s {
		if v.Owner == owner {
			tickets = v.Tickets
			break
		}
	}
	for _, t := range tickets {
		if t.Height != height {
			continue
		}
		if isInGenesis && t.StartTime != index {
			continue
		}
		return &Ticket{ID: id, TicketBody: t}, nil
	}
	return nil, fmt.Errorf("%v ticket not fount", id.String())
}

func (s TicketsDataSlice) AddTicket(ticket *Ticket) (TicketsDataSlice, error) {
	owner := ticket.Owner()
	var tickets TicketBodySlice
	row := 0
	for i, v := range s {
		if v.Owner == owner {
			tickets = v.Tickets
			row = i
			break
		}
	}
	if tickets == nil {
		s = append(s, TicketsData{
			Owner:   owner,
			Tickets: TicketBodySlice{ticket.TicketBody},
		})
		return s, nil
	}

	if ticket.IsInGenesis() {
		tickets = append(tickets, ticket.TicketBody)
	} else {
		for _, t := range tickets {
			if t.Height == ticket.Height {
				log.Info("AddTicket: ticket exist", "id", ticket.ID.String())
				return s, fmt.Errorf("AddTicket: %v ticket exist", ticket.ID.String())
			}
		}
		tickets = append(tickets, ticket.TicketBody)
	}
	s[row].Tickets = tickets
	return s, nil
}

func (s TicketsDataSlice) RemoveTicket(id Hash) (TicketsDataSlice, error) {
	owner, height, index := ParseTicketID(id)
	isInGenesis := height == 0
	var tickets TicketBodySlice
	row := 0
	for i, v := range s {
		if v.Owner == owner {
			tickets = v.Tickets
			row = i
			break
		}
	}
	for j, t := range tickets {
		if t.Height != height {
			continue
		}
		if isInGenesis && t.StartTime != index {
			continue
		}
		if len(tickets) == 1 {
			s = append(s[:row], s[row+1:]...)
		} else {
			tickets = append(tickets[:j], tickets[j+1:]...)
			s[row].Tickets = tickets
		}
		return s, nil
	}
	log.Info("RemoveTicket: ticket not found", "id", id.String())
	return s, fmt.Errorf("RemoveTicket: %v ticket not found", id.String())
}

func (s TicketsDataSlice) ClearExpiredTickets(timestamp uint64) (TicketsDataSlice, error) {
	haveTicket := false
	expiredIds := make([]Hash, 0)
	for _, v := range s {
		for _, t := range v.Tickets {
			if t.ExpireTime <= timestamp {
				id := TicketID(v.Owner, t.Height, t.StartTime)
				expiredIds = append(expiredIds, id)
			} else if !haveTicket {
				haveTicket = true
			}
		}
	}
	if !haveTicket {
		return s, fmt.Errorf("Next block have no ticket, wait buy ticket.")
	}
	if len(expiredIds) == 0 {
		return s, nil
	}
	for _, id := range expiredIds {
		s, _ = s.RemoveTicket(id)
	}
	return s, nil
}
