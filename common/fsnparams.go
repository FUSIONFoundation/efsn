package common

import (
	"fmt"
	"math/big"

	"github.com/FusionFoundation/efsn/rlp"
)

/////////////////// param type ///////////////////////
// FSNCallParam wacom
type FSNCallParam struct {
	Func FSNCallFunc
	Data []byte
}

// GenAssetParam wacom
type GenAssetParam struct {
	Name        string
	Symbol      string
	Decimals    uint8
	Total       *big.Int `json:",string"`
	CanChange   bool
	Description string
}

// BuyTicketParam wacom
type BuyTicketParam struct {
	Start uint64
	End   uint64
}

// SendAssetParam wacom
type SendAssetParam struct {
	AssetID Hash
	To      Address
	Value   *big.Int `json:",string"`
}

// AssetValueChangeExParam wacom
type AssetValueChangeExParam struct {
	AssetID     Hash
	To          Address
	Value       *big.Int `json:",string"`
	IsInc       bool
	TransacData string
}

// TimeLockParam wacom
type TimeLockParam struct {
	Type      TimeLockType
	AssetID   Hash
	To        Address
	StartTime uint64
	EndTime   uint64
	Value     *big.Int `json:",string"`
}

// MakeSwapParam wacom
type MakeSwapParam struct {
	FromAssetID   Hash
	FromStartTime uint64
	FromEndTime   uint64
	MinFromAmount *big.Int `json:",string"`
	ToAssetID     Hash
	ToStartTime   uint64
	ToEndTime     uint64
	MinToAmount   *big.Int `json:",string"`
	SwapSize      *big.Int `json:",string"`
	Targes        []Address
	Time          *big.Int
	Description   string
}

// MakeMultiSwapParam wacom
type MakeMultiSwapParam struct {
	FromAssetID   []Hash
	FromStartTime []uint64
	FromEndTime   []uint64
	MinFromAmount []*big.Int `json:",string"`
	ToAssetID     []Hash
	ToStartTime   []uint64
	ToEndTime     []uint64
	MinToAmount   []*big.Int `json:",string"`
	SwapSize      *big.Int   `json:",string"`
	Targes        []Address
	Time          *big.Int
	Description   string
}

// RecallSwapParam wacom
type RecallSwapParam struct {
	SwapID Hash
}

// RecallMultiSwapParam wacom
type RecallMultiSwapParam struct {
	SwapID Hash
}

// TakeSwapParam wacom
type TakeSwapParam struct {
	SwapID Hash
	Size   *big.Int `json:",string"`
}

// TakeMultiSwapParam wacom
type TakeMultiSwapParam struct {
	SwapID Hash
	Size   *big.Int `json:",string"`
}

/////////////////// param ToBytes ///////////////////////
// ToBytes wacom
func (p *FSNCallParam) ToBytes() ([]byte, error) {
	return rlp.EncodeToBytes(p)
}

// ToBytes wacom
func (p *GenAssetParam) ToBytes() ([]byte, error) {
	return rlp.EncodeToBytes(p)
}

// ToBytes wacom
func (p *SendAssetParam) ToBytes() ([]byte, error) {
	return rlp.EncodeToBytes(p)
}

// ToBytes wacom
func (p *TimeLockParam) ToBytes() ([]byte, error) {
	return rlp.EncodeToBytes(p)
}

// ToBytes wacom
func (p *BuyTicketParam) ToBytes() ([]byte, error) {
	return rlp.EncodeToBytes(p)
}

// ToBytes wacom
func (p *AssetValueChangeExParam) ToBytes() ([]byte, error) {
	return rlp.EncodeToBytes(p)
}

// ToBytes wacom
func (p *MakeSwapParam) ToBytes() ([]byte, error) {
	return rlp.EncodeToBytes(p)
}

// ToBytes wacom
func (p *RecallSwapParam) ToBytes() ([]byte, error) {
	return rlp.EncodeToBytes(p)
}

// ToBytes wacom
func (p *TakeSwapParam) ToBytes() ([]byte, error) {
	return rlp.EncodeToBytes(p)
}

// ToBytes wacom
func (p *MakeMultiSwapParam) ToBytes() ([]byte, error) {
	return rlp.EncodeToBytes(p)
}

// ToBytes wacom
func (p *RecallMultiSwapParam) ToBytes() ([]byte, error) {
	return rlp.EncodeToBytes(p)
}

// ToBytes wacom
func (p *TakeMultiSwapParam) ToBytes() ([]byte, error) {
	return rlp.EncodeToBytes(p)
}

/////////////////// param checking ///////////////////////
// Check wacom
func (p *FSNCallParam) Check(blockNumber *big.Int) error {
	return nil
}

// Check wacom
func (p *GenAssetParam) Check(blockNumber *big.Int) error {
	if len(p.Name) == 0 || len(p.Symbol) == 0 || p.Total == nil || p.Total.Cmp(Big0) < 0 {
		return fmt.Errorf("GenAssetFunc name, symbol and total must be set")
	}
	if p.Decimals > 18 {
		return fmt.Errorf("GenAssetFunc decimals must be between 0 and 18")
	}
	if len(p.Description) > 1024 {
		return fmt.Errorf("GenAsset description length is greater than 1024 chars")
	}
	if len(p.Name) > 128 {
		return fmt.Errorf("GenAsset name length is greater than 128 chars")
	}
	if len(p.Symbol) > 64 {
		return fmt.Errorf("GenAsset symbol length is greater than 64 chars")

	}
	return nil
}

// Check wacom
func (p *SendAssetParam) Check(blockNumber *big.Int) error {
	if p.Value == nil || p.Value.Cmp(Big0) <= 0 {
		return fmt.Errorf("Value must be set and greater than 0")
	}
	if p.To == (Address{}) {
		return fmt.Errorf("receiver address must be set and not zero address")
	}
	if p.AssetID == (Hash{}) {
		return fmt.Errorf("empty asset ID, 'asset' must be specified instead of AssetID.")
	}
	return nil
}

// Check wacom
func (p *TimeLockParam) Check(blockNumber *big.Int, timestamp uint64) error {

	if p.Value == nil || p.Value.Cmp(Big0) <= 0 {
		return fmt.Errorf("Value must be set and greater than 0")
	}
	if p.StartTime > p.EndTime {
		return fmt.Errorf("StartTime must be less than or equal to EndTime")
	}
	if p.EndTime < timestamp {
		return fmt.Errorf("EndTime must be greater than latest block time")
	}

	return nil
}

// Check wacom
func (p *BuyTicketParam) Check(blockNumber *big.Int, timestamp uint64, adjust int64) error {
	start, end := p.Start, p.End
	// check lifetime too short ticket
	if end <= start || end < start+30*24*3600 {
		return fmt.Errorf("BuyTicket end must be greater than start + 1 month")
	}
	if timestamp != 0 {
		// check future ticket
		if start > timestamp+3*3600 {
			return fmt.Errorf("BuyTicket start must be lower than latest block time + 3 hour")
		}
		// check expired soon ticket
		if end < uint64(int64(timestamp)+7*24*3600+adjust) {
			return fmt.Errorf("BuyTicket end must be greater than latest block time + 1 week")
		}
	}
	return nil
}

// Check wacom
func (p *AssetValueChangeExParam) Check(blockNumber *big.Int) error {
	if p.Value == nil || p.Value.Cmp(Big0) <= 0 {
		return fmt.Errorf("Value must be set and greater than 0")
	}
	if len(p.TransacData) > 256 {
		return fmt.Errorf("TransacData must not be greater than 256")
	}
	return nil
}

// Check wacom
func (p *MakeSwapParam) Check(blockNumber *big.Int, timestamp uint64) error {
	if p.MinFromAmount == nil || p.MinFromAmount.Cmp(Big0) <= 0 ||
		p.MinToAmount == nil || p.MinToAmount.Cmp(Big0) <= 0 ||
		p.SwapSize == nil || p.SwapSize.Cmp(Big0) <= 0 {
		return fmt.Errorf("MinFromAmount,MinToAmount and SwapSize must be ge 1")
	}
	if len(p.Description) > 1024 {
		return fmt.Errorf("MakeSwap description length is greater than 1024 chars")
	}
	total := new(big.Int).Mul(p.MinFromAmount, p.SwapSize)
	if total.Cmp(Big0) <= 0 {
		return fmt.Errorf("size * MinFromAmount too large")
	}

	toTotal := new(big.Int).Mul(p.MinToAmount, p.SwapSize)
	if toTotal.Cmp(Big0) <= 0 {
		return fmt.Errorf("size * MinToAmount too large")
	}

	if p.FromStartTime > p.FromEndTime {
		return fmt.Errorf("MakeSwap FromStartTime > FromEndTime")
	}
	if p.ToStartTime > p.ToEndTime {
		return fmt.Errorf("MakeSwap ToStartTime > ToEndTime")
	}

	if p.FromEndTime <= timestamp {
		return fmt.Errorf("MakeSwap FromEndTime <= latest blockTime")
	}
	if p.ToEndTime <= timestamp {
		return fmt.Errorf("MakeSwap ToEndTime <= latest blockTime")
	}

	return nil
}

// Check wacom
func (p *RecallSwapParam) Check(blockNumber *big.Int, swap *Swap) error {
	return nil
}

// Check wacom
func (p *TakeSwapParam) Check(blockNumber *big.Int, swap *Swap, timestamp uint64) error {
	if p.Size == nil || p.Size.Cmp(Big0) <= 0 ||
		swap.SwapSize == nil || p.Size.Cmp(swap.SwapSize) > 0 {

		return fmt.Errorf("Size must be ge 1 and le Swapsize")
	}

	if swap.FromEndTime <= timestamp {
		return fmt.Errorf("swap expired: FromEndTime <= latest blockTime")
	}
	if swap.ToEndTime <= timestamp {
		return fmt.Errorf("swap expired: ToEndTime <= latest blockTime")
	}

	return nil
}

// Check wacom
func (p *MakeMultiSwapParam) Check(blockNumber *big.Int, timestamp uint64) error {
	if p.MinFromAmount == nil || len(p.MinFromAmount) == 0 {
		return fmt.Errorf("MinFromAmount must be specifed")
	}
	if p.MinToAmount == nil || len(p.MinToAmount) == 0 {
		return fmt.Errorf("MinToAmount must be specifed")
	}
	if p.SwapSize == nil || p.SwapSize.Cmp(Big0) <= 0 {
		return fmt.Errorf("SwapSize must be ge 1")
	}

	if len(p.MinFromAmount) != len(p.FromEndTime) ||
		len(p.MinFromAmount) != len(p.FromAssetID) ||
		len(p.MinFromAmount) != len(p.FromStartTime) {
		return fmt.Errorf("MinFromAmount FromEndTime and FromStartTime array length must be same size")
	}
	if len(p.MinToAmount) != len(p.ToEndTime) ||
		len(p.MinToAmount) != len(p.ToAssetID) ||
		len(p.MinToAmount) != len(p.ToStartTime) {
		return fmt.Errorf("MinToAmount ToEndTime and ToStartTime array length must be same size")
	}

	ln := len(p.MinFromAmount)
	for i := 0; i < ln; i++ {
		if p.MinFromAmount[i] == nil || p.MinFromAmount[i].Cmp(Big0) <= 0 {
			return fmt.Errorf("MinFromAmounts must be ge 1")
		}
		total := new(big.Int).Mul(p.MinFromAmount[i], p.SwapSize)
		if total.Cmp(Big0) <= 0 {
			return fmt.Errorf("size * MinFromAmount too large")
		}
		if p.FromStartTime[i] > p.FromEndTime[i] {
			return fmt.Errorf("MakeMultiSwap FromStartTime > FromEndTime")
		}
		if p.FromEndTime[i] <= timestamp {
			return fmt.Errorf("MakeMultiSwap FromEndTime <= latest blockTime")
		}
	}

	ln = len(p.MinToAmount)
	for i := 0; i < ln; i++ {
		if p.MinToAmount[i] == nil || p.MinToAmount[i].Cmp(Big0) <= 0 {
			return fmt.Errorf("MinToAmounts must be ge 1")
		}
		toTotal := new(big.Int).Mul(p.MinToAmount[i], p.SwapSize)
		if toTotal.Cmp(Big0) <= 0 {
			return fmt.Errorf("size * MinToAmount too large")
		}
		if p.ToStartTime[i] > p.ToEndTime[i] {
			return fmt.Errorf("MakeMultiSwap ToStartTime > ToEndTime")
		}
		if p.ToEndTime[i] <= timestamp {
			return fmt.Errorf("MakeMultiSwap ToEndTime <= latest blockTime")
		}
	}

	if len(p.Description) > 1024 {
		return fmt.Errorf("MakeSwap description length is greater than 1024 chars")
	}
	return nil
}

// Check wacom
func (p *RecallMultiSwapParam) Check(blockNumber *big.Int, swap *MultiSwap) error {
	return nil
}

// Check wacom
func (p *TakeMultiSwapParam) Check(blockNumber *big.Int, swap *MultiSwap, timestamp uint64) error {
	if p.Size == nil || p.Size.Cmp(Big0) <= 0 ||
		swap.SwapSize == nil || p.Size.Cmp(swap.SwapSize) > 0 {

		return fmt.Errorf("Size must be ge 1 and le Swapsize")
	}

	ln := len(swap.FromEndTime)
	for i := 0; i < ln; i++ {
		if swap.FromEndTime[i] <= timestamp {
			return fmt.Errorf("swap expired: FromEndTime <= latest blockTime")
		}
	}

	ln = len(swap.ToEndTime)
	for i := 0; i < ln; i++ {
		if swap.ToEndTime[i] <= timestamp {
			return fmt.Errorf("swap expired: ToEndTime <= latest blockTime")
		}
	}
	return nil
}
