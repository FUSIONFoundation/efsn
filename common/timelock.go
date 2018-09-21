package common

import (
	"math/big"
	"sort"
)

// TimeLockType wacom
type TimeLockType uint

// TimeLockTypes wacom
const (
	AssetToTimeLock TimeLockType = iota
	TimeLockToTimeLock
	TimeLockToAsset
)

const (
	// TimeLockNow wacom
	TimeLockNow uint64 = 0
	// TimeLockForever wacom
	TimeLockForever uint64 = 0xffffffffffffffff
)

// TimeLockItem wacom
type TimeLockItem struct {
	StartTime uint64
	EndTime   uint64
	Value     *big.Int
}

// Sub wacom
func (z *TimeLockItem) Sub(x *TimeLockItem) (*big.Int, []*TimeLockItem) {
	remaining := make([]*TimeLockItem, 0)

	if z.StartTime < x.StartTime && (x.StartTime-z.StartTime) > 1 {
		remaining = append(remaining, &TimeLockItem{
			StartTime: z.StartTime,
			EndTime:   x.StartTime - 1,
			Value:     new(big.Int).SetBytes(z.Value.Bytes()),
		})
	}

	if z.EndTime > x.EndTime && (z.EndTime-x.EndTime) > 1 {
		remaining = append(remaining, &TimeLockItem{
			StartTime: x.EndTime + 1,
			EndTime:   z.EndTime,
			Value:     new(big.Int).SetBytes(z.Value.Bytes()),
		})
	}
	var missingValue *big.Int
	if z.Value.Cmp(x.Value) >= 0 {
		remaining = append(remaining, &TimeLockItem{
			StartTime: x.StartTime,
			EndTime:   x.EndTime,
			Value:     new(big.Int).Sub(z.Value, x.Value),
		})
		missingValue = big.NewInt(0)
	} else {
		missingValue = missingValue.Sub(x.Value, z.Value)
	}
	return missingValue, remaining
}

// Clone wacom
func (z *TimeLockItem) Clone() *TimeLockItem {
	return &TimeLockItem{
		StartTime: z.StartTime,
		EndTime:   z.EndTime,
		Value:     new(big.Int).SetBytes(z.Value.Bytes()),
	}
}

// TimeLock wacom
type TimeLock struct {
	Items    []*TimeLockItem
	changed  bool
	sortTime bool
}

// NewTimeLock wacom
func NewTimeLock(items ...*TimeLockItem) *TimeLock {
	timeLock := TimeLock{}
	timeLock.SetItems(items)
	return &timeLock
}

// IsEmpty wacom
func (z *TimeLock) IsEmpty() bool {
	return len(z.Items) == 0
}

// Add wacom
func (z *TimeLock) Add(x, y *TimeLock) *TimeLock {
	items := make([]*TimeLockItem, 0)
	if x != nil && x.Items != nil {
		items = append(items, x.Items...)
	}
	if y != nil && y.Items != nil {
		items = append(items, y.Items...)
	}
	z.SetItems(items)
	z.changed = true
	z.mergeAndSortValue()
	return z
}

// Sub wacom z = x - y
func (z *TimeLock) Sub(x, y *TimeLock) *TimeLock {
	if len(x.Items) != 1 {
		panic("Just Support One TimeLockItem")
	}
	item := y.Items[0]
	enough, remaining := x.findNeed(item)
	if enough {
		z.SetItems(remaining)
		z.changed = true
		z.mergeAndSortValue()
	}
	return z
}

// Set wacom
func (z *TimeLock) Set(x *TimeLock) *TimeLock {
	if x != nil && x.Items != nil {
		z.SetItems(x.Items)
	}
	return z
}

// SetItems wacom
func (z *TimeLock) SetItems(items []*TimeLockItem) {
	if items != nil {
		z.Items = make([]*TimeLockItem, len(items))
		for i := 0; i < len(items); i++ {
			z.Items[i] = &TimeLockItem{
				StartTime: items[i].StartTime,
				EndTime:   items[i].EndTime,
				Value:     new(big.Int).SetBytes(items[i].Value.Bytes()),
			}
		}
	}
}

// Cmp wacom
func (z *TimeLock) Cmp(x *TimeLock) int {
	if len(x.Items) != 1 {
		panic("Just Support One TimeLockItem")
	}
	item := x.Items[0]
	enough, remaining := z.findNeed(item)
	if !enough {
		return -1
	}
	if len(remaining) > 0 {
		return 1
	}
	return 0
}

// Clone wacom
func (z *TimeLock) Clone() *TimeLock {
	return NewTimeLock(z.Items...)
}

func (z *TimeLock) Len() int {
	return len(z.Items)
}
func (z *TimeLock) Swap(i, j int) {
	z.Items[i], z.Items[j] = z.Items[j], z.Items[i]
}
func (z *TimeLock) Less(i, j int) bool {
	if z.sortTime {
		return z.Items[i].StartTime < z.Items[j].StartTime || (z.Items[i].StartTime == z.Items[j].StartTime && z.Items[i].EndTime <= z.Items[j].EndTime)
	}
	return z.Items[i].Value.Cmp(z.Items[j].Value) < 0
}

func (z *TimeLock) findNeed(item *TimeLockItem) (bool, []*TimeLockItem) {
	item = item.Clone()
	z.mergeAndSortValue()
	remaining := make([]*TimeLockItem, 0)
	for i := 0; i < len(z.Items); i++ {
		temp := z.Items[i]
		if temp.StartTime <= item.StartTime && temp.EndTime >= item.EndTime {
			missingValue, tempItems := temp.Sub(item)
			item.Value = missingValue
			remaining = append(remaining, tempItems...)
			if item.Value.Sign() == 0 {
				break
			}
		} else {
			remaining = append(remaining, temp)
		}
	}
	return item.Value.Sign() == 0, remaining
}

func (z *TimeLock) mergeAndSortValue() {
	if !z.changed {
		return
	}
	z.changed = false
	z.sortTime = true
	sort.Sort(z)

	z.sortTime = false
	sort.Sort(z)
}
