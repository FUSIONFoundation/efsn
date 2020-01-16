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
	SmartTransfer
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
	return z.StartTime == x.StartTime && z.EndTime == x.EndTime
}

func (z *TimeLockItem) EqualTo(x *TimeLockItem) bool {
	return z.EqualRange(x) && z.Value.Cmp(x.Value) == 0
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
		if x.CanMerge(z) {
			return []*TimeLockItem{x.Merge(z)}, TailInFirst
		}
		return []*TimeLockItem{x, z}, TailInFirst
	}
	if z.EndTime < x.StartTime {
		if z.CanMerge(x) {
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
	return z == nil || len(z.Items) == 0
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
	if x.IsEmpty() {
		z.Set(y)
		return z
	}
	if y.IsEmpty() {
		z.Set(x)
		return z
	}
	items := make([]*TimeLockItem, 0)
	i, j := 0, 0
	var xV, yV *TimeLockItem
	for i < len(x.Items) && j < len(y.Items) {
		if xV == nil {
			xV = x.Items[i]
		}
		if yV == nil {
			yV = y.Items[j]
		}
		res, tailFlag := xV.Add(yV)
		if tailFlag == TailInBoth {
			i++
			j++
			xV = nil
			yV = nil
			items = appendAndMergeItems(items, res)
		} else {
			items = appendAndMergeItems(items, res[:len(res)-1])
			last := res[len(res)-1]
			if tailFlag == TailInFirst {
				j++
				xV = last
				yV = nil
				if j == len(y.Items) {
					items = appendAndMergeItem(items, last)
					i++
				}
			} else if tailFlag == TailInSecond {
				i++
				xV = nil
				yV = last
				if i == len(x.Items) {
					items = appendAndMergeItem(items, last)
					j++
				}
			}
		}
	}
	if i < len(x.Items) {
		items = appendAndMergeItems(items, x.Items[i:])
	} else if j < len(y.Items) {
		items = appendAndMergeItems(items, y.Items[j:])
	}
	z.SetItems(items)
	DebugCall(func() { z.CheckValid() })
	return z
}

// Sub wacom
func (z *TimeLock) Sub(x, y *TimeLock) *TimeLock {
	if !x.CanSub(y) {
		log.Info("TimeLock::Sub failed", "x", x.RawString(), "y", y.RawString())
		panic("Sub TimeLock not enough")
	}
	items := make([]*TimeLockItem, 0)
	i, j := 0, 0
	var xV, yV *TimeLockItem
	for i < len(x.Items) && j < len(y.Items) {
		if xV == nil {
			xV = x.Items[i]
		}
		if yV == nil {
			yV = y.Items[j]
		}
		res, missing := xV.Sub(yV)
		if xV.EndTime <= yV.EndTime {
			i++
			xV = nil
			items = appendAndMergeItems(items, res)
		} else {
			items = appendAndMergeItems(items, res[:len(res)-1])
			xV = res[len(res)-1]
		}
		if missing == nil {
			j++
			if j == len(y.Items) && xV != nil {
				items = appendAndMergeItem(items, xV)
				i++
			}
		}
		yV = missing
	}
	if i < len(x.Items) {
		items = appendAndMergeItems(items, x.Items[i:])
	}
	z.SetItems(items)
	DebugCall(func() { z.CheckValid() })
	return z
}

func (z *TimeLock) CanSub(x *TimeLock) bool {
	if x.IsEmpty() {
		return true
	}
	if err := x.IsValid(); err != nil {
		return false
	}
	for _, item := range x.Items {
		value := z.GetSpendableValue(item.StartTime, item.EndTime)
		cmp := value.Cmp(item.Value)
		if cmp < 0 {
			return false
		}
	}
	return true
}

// Cmp wacom
func (z *TimeLock) Cmp(x *TimeLock) int {
	if z.CanSub(x) {
		if z.EqualTo(x) {
			return 0
		}
		return 1
	}
	return -1
}

func (z *TimeLock) GetSpendableValue(start, end uint64) *big.Int {
	if start > end || z.IsEmpty() {
		return big.NewInt(0)
	}
	if z.Items[len(z.Items)-1].EndTime < end {
		return big.NewInt(0) // has tail gap
	}
	result := big.NewInt(0)
	var tempEnd uint64
	for _, item := range z.Items {
		if item.EndTime < start {
			continue
		}
		if tempEnd == 0 {
			if item.StartTime > start {
				return big.NewInt(0) // has head gap
			}
			result = item.Value
		} else {
			if item.StartTime != tempEnd+1 {
				return big.NewInt(0) // has middle gap
			}
			if item.Value.Cmp(result) < 0 {
				result = item.Value
			}
		}
		tempEnd = item.EndTime
		if tempEnd >= end {
			break
		}
	}
	return result
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
	if z.IsEmpty() {
		return nil
	}
	var prev, next *TimeLockItem
	for i := 0; i < len(z.Items); i++ {
		next = z.Items[i]
		if err := next.IsValid(); err != nil {
			return fmt.Errorf("TimeLock is invalid, index:%v err:%v", i, err)
		}
		if prev != nil {
			if prev.EndTime >= next.StartTime || prev.CanMerge(next) {
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

func (z *TimeLock) EqualTo(x *TimeLock) bool {
	if z.IsEmpty() != x.IsEmpty() {
		return false
	}
	if len(z.Items) != len(x.Items) {
		return false
	}
	for i := 0; i < len(z.Items); i++ {
		if !z.Items[i].EqualTo(x.Items[i]) {
			return false
		}
	}
	return true
}

// Set wacom
func (z *TimeLock) Set(x *TimeLock) *TimeLock {
	var items []*TimeLockItem
	if !x.IsEmpty() {
		items = x.Items
	}
	z.SetItems(items)
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
	var items []*TimeLockItem
	if !z.IsEmpty() {
		items = z.Items
	}
	return NewTimeLock(items...)
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

func IsWholeAsset(start, end, timestamp uint64) bool {
	return end == TimeLockForever && start <= timestamp
}

func GetTimeLock(value *big.Int, start, end uint64) *TimeLock {
	if value.Sign() <= 0 {
		return NewTimeLock()
	}
	return NewTimeLock(&TimeLockItem{
		Value:     value,
		StartTime: start,
		EndTime:   end,
	})
}

func GetSurplusTimeLock(value *big.Int, start, end, timestamp uint64) *TimeLock {
	left := NewTimeLock()
	if value.Sign() <= 0 {
		return left
	}
	if start > timestamp {
		left.Items = append(left.Items, &TimeLockItem{
			Value:     value,
			StartTime: timestamp,
			EndTime:   start - 1,
		})
	}
	if end < TimeLockForever {
		left.Items = append(left.Items, &TimeLockItem{
			Value:     value,
			StartTime: end + 1,
			EndTime:   TimeLockForever,
		})
	}
	return left
}
