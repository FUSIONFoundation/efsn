package common

import (
	"encoding/json"
	"fmt"
	"math/big"
)

// TicketPrice  place holder for ticket price
func TicketPrice(blocknumber *big.Int) *big.Int {
	oneFSN := big.NewInt(1000000000000000000)
	return new(big.Int).Mul(big.NewInt(5000), oneFSN)
}

// Ticket wacom
type Ticket struct {
	ID         Hash
	Height     uint64
	StartTime  uint64
	ExpireTime uint64
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

func (t *Ticket) IsInGenesis() bool {
	return t.Height == 0
}

func (t *Ticket) Owner() Address {
	return BytesToAddress(t.ID[:AddressLength])
}

func (t *Ticket) BlockHeight() *big.Int {
	return new(big.Int).SetUint64(t.Height)
}

func (t *Ticket) Value() *big.Int {
	return TicketPrice(new(big.Int).SetUint64(t.Height))
}

func (t *Ticket) ToValHash() Hash {
	h := Hash{}
	copy(h[:8], Uint64ToBytes(t.Height))
	copy(h[8:16], Uint64ToBytes(t.StartTime))
	copy(h[16:24], Uint64ToBytes(t.ExpireTime))
	return h
}

func TicketID(owner Address, height *big.Int, timestamp *big.Int, difficulty *big.Int) Hash {
	h := Hash{}
	copy(h[:20], owner[:])
	copy(h[20:24], Uint32ToBytes(uint32(height.Uint64())))
	copy(h[24:28], Uint32ToBytes(uint32(timestamp.Uint64())))
	copy(h[28:32], Uint32ToBytes(uint32(difficulty.Uint64())))
	return h
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

func ParseTicket(key, value Hash) (*Ticket, error) {
	if key == (Hash{}) {
		return nil, fmt.Errorf("ParseTicket: empty key")
	}
	if value == (Hash{}) {
		return nil, fmt.Errorf("ParseTicket: empty value")
	}
	height := BytesToUint64(value[:8])
	start := BytesToUint64(value[8:16])
	end := BytesToUint64(value[16:24])
	return &Ticket{
		ID:         key,
		Height:     height,
		StartTime:  start,
		ExpireTime: end,
	}, nil
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

func (s TicketSlice) AllIds() []Hash {
	ids := make([]Hash, len(s))
	for i, t := range s {
		ids[i] = t.ID
	}
	return ids
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
