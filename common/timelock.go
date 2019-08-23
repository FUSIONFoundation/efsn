package common

import (
	"encoding/json"
	"fmt"
	"math/big"
	"sort"

	"github.com/FusionFoundation/efsn/log"
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
	Value     *big.Int `json:",string"`
}

func (z *TimeLockItem) IsValid() error {
	if z.StartTime > z.EndTime {
		return fmt.Errorf("TimeLockItem time is invalid, StartTime:%v > EndTime:%v", z.StartTime, z.EndTime)
	}
	if z.Value.Sign() <= 0 {
		return fmt.Errorf("TimeLockItem value is invalid, Value:%v", z.Value)
	}
	return nil
}

func (z *TimeLockItem) Clone() *TimeLockItem {
	return &TimeLockItem{
		StartTime: z.StartTime,
		EndTime:   z.EndTime,
		Value:     new(big.Int).SetBytes(z.Value.Bytes()),
	}
}

func (z *TimeLockItem) String() string {
	b, _ := json.Marshal(z)
	return string(b)
}

func (z *TimeLockItem) AdjustStart(startTime uint64) *TimeLockItem {
	return z.AdjustStartEnd(startTime, z.EndTime)
}

func (z *TimeLockItem) AdjustEnd(endTime uint64) *TimeLockItem {
	return z.AdjustStartEnd(z.StartTime, endTime)
}

func (z *TimeLockItem) AdjustStartEnd(startTime uint64, endTime uint64) *TimeLockItem {
	return &TimeLockItem{
		StartTime: startTime,
		EndTime:   endTime,
		Value:     new(big.Int).SetBytes(z.Value.Bytes()),
	}
}

func (z *TimeLockItem) EqualRange(x *TimeLockItem) bool {
	return x != nil && z.StartTime == x.StartTime && z.EndTime == x.EndTime
}

func (z *TimeLockItem) CanMerge(x *TimeLockItem) bool {
	return z.EndTime < x.StartTime && z.EndTime+1 == x.StartTime && z.Value.Cmp(x.Value) == 0
}

// please ensure CanMerge condition is satisfied
func (z *TimeLockItem) Merge(x *TimeLockItem) *TimeLockItem {
	return z.AdjustEnd(x.EndTime)
}

type TailFlag int

const (
	TailInBoth TailFlag = iota
	TailInFirst
	TailInSecond
)

func (z *TimeLockItem) Add(x *TimeLockItem) ([]*TimeLockItem, TailFlag) {
	if x.EndTime < z.StartTime {
		if x.CanMerge(z) == true {
			return []*TimeLockItem{x.Merge(z)}, TailInFirst
		}
		return []*TimeLockItem{x, z}, TailInFirst
	}
	if z.EndTime < x.StartTime {
		if z.CanMerge(x) == true {
			return []*TimeLockItem{z.Merge(x)}, TailInSecond
		}
		return []*TimeLockItem{z, x}, TailInSecond
	}
	items := make([]*TimeLockItem, 0)
	// head part
	if x.StartTime < z.StartTime {
		items = append(items, x.AdjustEnd(z.StartTime-1))
	} else if z.StartTime < x.StartTime {
		items = append(items, z.AdjustEnd(x.StartTime-1))
	}

	// middle part
	temp := intersectionAdd(x, z)
	if temp != nil {
		items = append(items, temp)
	}

	// tail part
	var tailFlag TailFlag
	if x.EndTime < z.EndTime {
		tailFlag = TailInFirst
		items = append(items, z.AdjustStart(x.EndTime+1))
	} else if z.EndTime < x.EndTime {
		tailFlag = TailInSecond
		items = append(items, x.AdjustStart(z.EndTime+1))
	} else if x.EndTime == z.EndTime {
		tailFlag = TailInBoth
	}
	return items, tailFlag
}

func (z *TimeLockItem) Sub(x *TimeLockItem) ([]*TimeLockItem, *TimeLockItem) {
	if x == nil {
		return []*TimeLockItem{z}, nil
	}
	missing := x
	if z.EndTime < x.StartTime || z.StartTime > x.EndTime {
		return []*TimeLockItem{z}, missing
	}
	if z.Value.Cmp(x.Value) < 0 {
		return []*TimeLockItem{z}, missing
	}
	items := make([]*TimeLockItem, 0)
	// head part
	if z.StartTime < x.StartTime {
		items = append(items, z.AdjustEnd(x.StartTime-1))
	}
	// middle part
	temp := intersectionSub(z, x)
	if temp != nil {
		items = append(items, temp)
	}
	// tail part
	if z.EndTime >= x.EndTime {
		if z.EndTime > x.EndTime {
			items = append(items, z.AdjustStart(x.EndTime+1))
		}
		missing = nil
	} else if z.EndTime >= x.StartTime {
		missing = missing.AdjustStart(z.EndTime + 1)
	}
	return items, missing
}

func intersectionAdd(x, y *TimeLockItem) *TimeLockItem {
	startTime := x.StartTime
	if startTime < y.StartTime {
		startTime = y.StartTime
	}
	endTime := x.EndTime
	if endTime > y.EndTime {
		endTime = y.EndTime
	}
	if startTime > endTime {
		return nil
	}
	return &TimeLockItem{
		StartTime: startTime,
		EndTime:   endTime,
		Value:     new(big.Int).Add(x.Value, y.Value),
	}
}

func intersectionSub(x, y *TimeLockItem) *TimeLockItem {
	if x.Value.Cmp(y.Value) == 0 {
		return nil
	}
	startTime := x.StartTime
	if startTime < y.StartTime {
		startTime = y.StartTime
	}
	endTime := x.EndTime
	if endTime > y.EndTime {
		endTime = y.EndTime
	}
	if startTime > endTime {
		return nil
	}
	return &TimeLockItem{
		StartTime: startTime,
		EndTime:   endTime,
		Value:     new(big.Int).Sub(x.Value, y.Value),
	}
}

/////////////////////////////// TimeLock ///////////////////////////
// TimeLock wacom
type TimeLock struct {
	Items []*TimeLockItem
}

// NewTimeLock wacom
func NewTimeLock(items ...*TimeLockItem) *TimeLock {
	timeLock := TimeLock{}
	timeLock.SetItems(items)
	return &timeLock
}

func (z *TimeLock) IsEmpty() bool {
	return len(z.Items) == 0
}

func appendAndMergeItem(items []*TimeLockItem, addItem *TimeLockItem) []*TimeLockItem {
	if addItem == nil {
		return items
	}
	if len(items) == 0 {
		return []*TimeLockItem{addItem}
	}
	last := items[len(items)-1]
	if last.CanMerge(addItem) {
		items[len(items)-1] = last.Merge(addItem)
	} else {
		items = append(items, addItem)
	}
	return items
}

func appendAndMergeItems(items []*TimeLockItem, addItems []*TimeLockItem) []*TimeLockItem {
	if len(addItems) > 0 {
		items = appendAndMergeItem(items, addItems[0])
		items = append(items, addItems[1:]...)
	}
	return items
}

// Add wacom
func (z *TimeLock) Add(x, y *TimeLock) *TimeLock {
	xIsEmpty := x == nil || x.IsEmpty()
	yIsEmpty := y == nil || y.IsEmpty()
	if xIsEmpty && yIsEmpty {
		return nil
	}

	items := make([]*TimeLockItem, 0)
	var xV, yV, last *TimeLockItem
	var res []*TimeLockItem
	var tailFlag TailFlag
	i, j := 0, 0
	for i < len(x.Items) && j < len(y.Items) {
		xV = x.Items[i]
		yV = y.Items[j]
		if last != nil {
			if tailFlag == TailInFirst {
				xV = last
			} else if tailFlag == TailInSecond {
				yV = last
			}
		}
		if xV.EndTime < yV.StartTime {
			items = appendAndMergeItem(items, xV)
			i++
			if tailFlag == TailInFirst {
				last = nil
			}
			continue
		}
		if yV.EndTime < xV.StartTime {
			items = appendAndMergeItem(items, yV)
			j++
			if tailFlag == TailInSecond {
				last = nil
			}
			continue
		}
		res, tailFlag = xV.Add(yV)
		last = res[len(res)-1]
		if len(res) > 1 {
			items = appendAndMergeItems(items, res[:len(res)-1])
		}
		if tailFlag == TailInFirst {
			j++
		} else if tailFlag == TailInSecond {
			i++
		} else if tailFlag == TailInBoth {
			i++
			j++
			items = appendAndMergeItem(items, last)
			last = nil
		}
	}
	if last != nil {
		items = appendAndMergeItem(items, last)
		if tailFlag == TailInFirst {
			i++
		} else if tailFlag == TailInSecond {
			j++
		}
	}
	if i >= len(x.Items) && j < len(y.Items) {
		items = appendAndMergeItems(items, y.Items[j:])
	}
	if j >= len(y.Items) && i < len(x.Items) {
		items = appendAndMergeItems(items, x.Items[i:])
	}

	z.SetItems(items)
	DebugCall(func() { z.CheckValid() })
	return z
}

// Sub wacom
func (z *TimeLock) Sub(x, y *TimeLock) *TimeLock {
	if y == nil || len(y.Items) != 1 {
		panic("Sub Just Support One TimeLockItem")
	}
	if x.Cmp(y) < 0 {
		panic("Sub TimeLock not enough")
	}
	yV := y.Items[0]
	items := make([]*TimeLockItem, 0)
	for i := 0; i < len(x.Items); i++ {
		xV := x.Items[i]
		res, missing := xV.Sub(yV)
		items = appendAndMergeItems(items, res)
		if missing == nil {
			items = appendAndMergeItems(items, x.Items[i+1:])
			break
		}
		yV = missing
	}
	z.SetItems(items)
	DebugCall(func() { z.CheckValid() })
	return z
}

// Cmp wacom
func (z *TimeLock) Cmp(x *TimeLock) int {
	if x == nil || len(x.Items) != 1 {
		panic("Cmp Just Support One TimeLockItem")
	}
	if z.IsEmpty() == true {
		return -1
	}
	if err := x.IsValid(); err != nil {
		log.Info("TimeLock:%v is invalid", x)
		return -1
	}
	item := x.Items[0]
	if z.Items[0].StartTime > item.StartTime {
		return -1 // has head gap
	}
	if z.Items[len(z.Items)-1].EndTime < item.EndTime {
		return -1 // has tail gap
	}
	minVal := big.NewInt(0)
	tempTime := uint64(0)
	maybeGreat := z.Items[0].StartTime < item.StartTime || z.Items[len(z.Items)-1].EndTime > item.EndTime
	for _, t := range z.Items {
		if t.EndTime < item.StartTime {
			continue
		}
		if tempTime == 0 {
			if t.StartTime > item.StartTime {
				return -1 // has head gap
			}
			minVal = t.Value
		} else if tempTime < t.StartTime && tempTime+1 < t.StartTime {
			return -1 // has middle gap
		}
		if t.Value.Cmp(minVal) < 0 {
			minVal = t.Value
			res := minVal.Cmp(item.Value)
			if res < 0 {
				return -1 // has lower value
			} else if res > 0 {
				maybeGreat = true
			}
		}
		tempTime = t.EndTime
		if tempTime >= item.EndTime {
			break
		}
	}
	cmpVal := minVal.Cmp(item.Value)
	if cmpVal == 0 && maybeGreat {
		return 1
	}
	return cmpVal
}

func (z *TimeLock) ClearExpired(timestamp uint64) *TimeLock {
	for i := 0; i < len(z.Items); i++ {
		t := z.Items[i]
		if t.EndTime >= timestamp {
			z.Items = z.Items[i:]
			return z
		}
	}
	z.Items = []*TimeLockItem{}
	return z
}

func (z *TimeLock) IsValid() error {
	var prev, next *TimeLockItem
	for i := 0; i < len(z.Items); i++ {
		next = z.Items[i]
		if err := next.IsValid(); err != nil {
			return fmt.Errorf("TimeLock is invalid, index:%v err:%v", i, err)
		}
		if prev != nil {
			if prev.EndTime >= next.StartTime || prev.CanMerge(next) == true {
				return fmt.Errorf("TimeLock is invalid, index:%v, prev:%v, next:%v", i, prev, next)
			}
		}
		prev = next
	}
	return nil
}

func (z *TimeLock) CheckValid() {
	if err := z.IsValid(); err != nil {
		log.Info("TimeLock checkValid failed", "err", err)
		panic("TimeLock checkValid failed, err:" + err.Error())
	}
}

// Set wacom
func (z *TimeLock) Set(x *TimeLock) *TimeLock {
	if x != nil {
		z = x.Clone()
	} else {
		z = &TimeLock{}
	}
	return z
}

// SetItems wacom
func (z *TimeLock) SetItems(items []*TimeLockItem) {
	z.Items = make([]*TimeLockItem, len(items))
	for i := 0; i < len(items); i++ {
		z.Items[i] = items[i].Clone()
	}
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
	return z.Items[i].Value.Cmp(z.Items[j].Value) > 0
}

func (u *TimeLockItem) MarshalJSON() ([]byte, error) {
	return json.Marshal(&struct {
		StartTime uint64
		EndTime   uint64
		Value     string
	}{
		StartTime: u.StartTime,
		EndTime:   u.EndTime,
		Value:     u.Value.String(),
	})
}

func (z *TimeLock) ToDisplay() *TimeLock {
	t := z.Clone()
	items := t.Items
	res := make([]*TimeLockItem, 0)
	for {
		if len(items) == 0 {
			break
		}
		start := items[0].StartTime
		end := items[0].EndTime
		value := items[0].Value
		i := 1
		for ; i < len(items); i++ {
			item := items[i]
			if !(end < item.StartTime && end+1 == item.StartTime) {
				break
			}
			end = item.EndTime
			if item.Value.Cmp(value) < 0 {
				value = item.Value
			}
		}
		if i == 1 {
			res = append(res, items[0])
			items = items[1:]
			continue
		}
		res = append(res, &TimeLockItem{
			StartTime: start,
			EndTime:   end,
			Value:     value,
		})
		for j := 0; j < i; {
			item := items[j]
			if item.Value.Cmp(value) == 0 {
				items = append(items[:j], items[j+1:]...)
				i--
			} else {
				item.Value.Sub(item.Value, value)
				j++
			}
		}
	}
	t.Items = res
	sort.Stable(t)
	return t
}

// String wacom
func (z *TimeLock) String() string {
	b, _ := json.Marshal(z.ToDisplay().Items)
	return string(b)
}

func (z *TimeLock) RawString() string {
	b, _ := json.Marshal(z.Items)
	return string(b)
}
