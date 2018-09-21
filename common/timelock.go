package common

import (
	"encoding/json"
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
		if z.Value.Cmp(x.Value) > 0 {
			remaining = append(remaining, &TimeLockItem{
				StartTime: x.StartTime,
				EndTime:   x.EndTime,
				Value:     new(big.Int).Sub(z.Value, x.Value),
			})
		}
		missingValue = big.NewInt(0)
	} else {
		missingValue = new(big.Int).Sub(x.Value, z.Value)
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
	if len(y.Items) != 1 {
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
	return z.Items[i].Value.Cmp(z.Items[j].Value) > 0
}

// String wacom
func (z *TimeLock) String() string {
	b, _ := json.Marshal(z.Items)
	return string(b)
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
				remaining = append(remaining, z.Items[i+1:]...)
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
	items := mergeValue(z.Items)
	items = mergeTime(items)
	z.Items = items
	z.sortTime = true
	sort.Sort(z)
	z.sortTime = false
	sort.Sort(z)
}

func mergeValue(items []*TimeLockItem) []*TimeLockItem {
	i, j := 0, 1
	for {
		size := len(items)
		if i >= size || j >= size {
			break
		}
		x, y := items[i], items[j]
		if x.StartTime == y.StartTime && x.EndTime == y.EndTime {
			x.Value = x.Value.Add(x.Value, y.Value)
			items = append(items[:j], items[j+1:]...)
		} else if x.StartTime == y.StartTime {
			j++
		} else {
			i = j
			j++
		}
	}
	return items
}

func mergeOneTime(items []*TimeLockItem) (*TimeLockItem, []*TimeLockItem) {
	if len(items) == 0 {
		return nil, nil
	}
	temp := items[0]
	items = items[1:]
	rems := make([]*TimeLockItem, 0)
	i := 0
	for {
		if i >= len(items) {
			break
		}
		item := items[i]
		if temp.StartTime < item.StartTime && temp.EndTime+1 >= item.StartTime && temp.EndTime < item.EndTime {
			items = append(items[:i], items[i+1:]...)
			mergeValue := temp.Value
			if mergeValue.Cmp(item.Value) > 0 {
				mergeValue = item.Value
			}
			mergeValue = new(big.Int).SetBytes(mergeValue.Bytes())
			_, rem := temp.Sub(&TimeLockItem{
				StartTime: temp.StartTime,
				EndTime:   temp.EndTime,
				Value:     mergeValue,
			})

			rems = append(rems, rem...)

			_, rem = item.Sub(&TimeLockItem{
				StartTime: temp.EndTime + 1,
				EndTime:   item.EndTime,
				Value:     mergeValue,
			})
			rems = append(rems, rem...)

			temp.EndTime = item.EndTime
			temp.Value = mergeValue
		} else {
			i++
		}

	}
	items = append(items, rems...)
	items = mergeValue(items)
	return temp, items
}

func mergeTime(items []*TimeLockItem) []*TimeLockItem {
	temps := make([]*TimeLockItem, 0)
	for {
		temp, tempItems := mergeOneTime(items)
		if temp == nil {
			break
		}
		if len(tempItems) > 0 {
			t := &TimeLock{
				Items:    tempItems,
				sortTime: true,
			}
			sort.Sort(t)
			tempItems = t.Items
		}
		temps = append(temps, temp)
		items = tempItems
	}
	temps = mergeValue(temps)
	return temps
}
