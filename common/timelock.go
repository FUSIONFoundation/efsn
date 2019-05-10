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

// added after hard fork 5 to indentify different storage
var FlagTimeLockItem = TimeLockItem{0, 0, Big0}

func (z *TimeLockItem) IsFlagTimeLockItem() bool {
	f := &FlagTimeLockItem
	return z.StartTime == f.StartTime && z.EndTime == f.EndTime && z.Value.Cmp(f.Value) == 0
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

func (z *TimeLockItem) SubWithMissingValue(x *TimeLockItem) (*big.Int, []*TimeLockItem) {
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
	if z.IsFlagTimeLockItem() || x.IsFlagTimeLockItem() {
		return false
	}
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

func (z *TimeLock) IsEmpty() bool {
	return len(z.Items) == 0 || (len(z.Items) == 1 && z.Items[0].IsFlagTimeLockItem())
}

func (z *TimeLock) HasOnlyOneItem() bool {
	if z.IsNormalized() == true {
		return len(z.Items) == 2
	}
	return len(z.Items) == 1
}

// use this special way to identify different TimeLock storage method after hard fork 4
func (z *TimeLock) IsNormalized() bool {
	return len(z.Items) > 0 && z.Items[0].IsFlagTimeLockItem()
}

func appendAndMergeItem(items []*TimeLockItem, addItem *TimeLockItem) []*TimeLockItem {
	if addItem == nil || addItem.IsFlagTimeLockItem() {
		return items
	}
	if len(items) == 0 {
		return []*TimeLockItem{&FlagTimeLockItem, addItem}
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
	if len(addItems) == 0 {
		return items
	}
	i := 0
	firstAddItem := addItems[i]
	if firstAddItem.IsFlagTimeLockItem() {
		if len(addItems) == 1 {
			return items
		}
		i++
		firstAddItem = addItems[i]
	}
	items = appendAndMergeItem(items, firstAddItem)
	if i+1 < len(addItems) {
		items = append(items, addItems[i+1:]...)
	}
	return items
}

// Add wacom
func (z *TimeLock) Add2(x, y *TimeLock) *TimeLock {
	xIsEmpty := x == nil || x.IsEmpty()
	yIsEmpty := y == nil || y.IsEmpty()
	if xIsEmpty && yIsEmpty {
		return nil
	}

	items := make([]*TimeLockItem, 0)
	items = append(items, &FlagTimeLockItem)
	var xV, yV, last *TimeLockItem
	var res []*TimeLockItem
	var tailFlag TailFlag
	i, j := 0, 0
	if x.IsNormalized() == true {
		i++
	}
	if y.IsNormalized() == true {
		j++
	}
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
	if DebugMode == true {
		z.CheckValid()
	}
	return z
}

// Sub wacom
func (z *TimeLock) Sub2(x, y *TimeLock) *TimeLock {
	if y == nil || y.HasOnlyOneItem() == false {
		panic("Sub Just Support One TimeLockItem")
	}
	if x == nil || x.IsEmpty() {
		return nil
	}
	if x.Cmp2(y) < 0 {
		panic("Sub TimeLock not enough")
	}
	yV := y.Items[len(y.Items)-1]
	items := make([]*TimeLockItem, 0)
	items = append(items, &FlagTimeLockItem)
	i := 0
	if x.IsNormalized() == true {
		i++
	}
	for ; i < len(x.Items); i++ {
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
	if DebugMode == true {
		z.CheckValid()
	}
	return z
}

// Cmp wacom
func (z *TimeLock) Cmp2(x *TimeLock) int {
	if x == nil || x.HasOnlyOneItem() == false {
		panic("Cmp2 Just Support One TimeLockItem")
	}
	if z.IsEmpty() == true {
		return -1
	}
	if z.IsNormalized() == false {
		z.Normalize()
	}
	if err := x.IsValid(); err != nil {
		log.Info("TimeLock:%v is invalid", x)
		return -1
	}
	item := x.Items[len(x.Items)-1]
	if z.Items[1].StartTime > item.StartTime {
		return -1 // has head gap
	}
	if z.Items[len(z.Items)-1].EndTime < item.EndTime {
		return -1 // has tail gap
	}
	minVal := big.NewInt(0)
	tempTime := uint64(0)
	maybeGreat := z.Items[1].StartTime < item.StartTime || z.Items[len(z.Items)-1].EndTime > item.EndTime
	for _, t := range z.Items[1:] {
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
	if z.IsNormalized() == false {
		panic("ClearExpired must have normalized time lock items")
	}
	for i := 1; i < len(z.Items); i++ {
		t := z.Items[i]
		if t.EndTime >= timestamp {
			if i > 1 {
				z.Items = append(z.Items[:1], z.Items[i:]...)
			}
			return z
		}
	}
	z.Items = []*TimeLockItem{&FlagTimeLockItem}
	return z
}

func (z *TimeLock) Normalize() *TimeLock {
	if z.IsNormalized() == true {
		return z
	}
	res := NewTimeLock(&FlagTimeLockItem)
	for _, t := range z.Items {
		if err := t.IsValid(); err != nil {
			continue
		}
		res = new(TimeLock).Add2(res, NewTimeLock(t))
	}
	z.Items = res.Items
	return z
}

func (z *TimeLock) IsValid() error {
	var prev, next *TimeLockItem
	i := 0
	if z.IsNormalized() == true {
		i++
	}
	for ; i < len(z.Items); i++ {
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

// Add wacom
func (z *TimeLock) Add(x, y *TimeLock) *TimeLock {
	if z.IsNormalized() == true {
		return z.Add2(x, y)
	}
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
	if z.IsNormalized() == true {
		return z.Sub2(x, y)
	}
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
	if x != nil {
		z = x.Clone()
	} else {
		z = &TimeLock{}
	}
	return z
}

// SetItems wacom
func (z *TimeLock) SetItems(items []*TimeLockItem) {
	if items != nil {
		z.Items = make([]*TimeLockItem, len(items))
		for i := 0; i < len(items); i++ {
			z.Items[i] = items[i].Clone()
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
	//return NewTimeLock(z.Items...)
	t := NewTimeLock(z.Items...)
	t.changed = z.changed
	t.sortTime = z.sortTime
	return t
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

func (u *TimeLockItem) MarshalJSON() ([]byte, error) {
	return json.Marshal(&struct {
		StartTime uint64
		Value     string
		EndTime   uint64
	}{
		StartTime: u.StartTime,
		EndTime:   u.EndTime,
		Value:     u.Value.String(),
	})
}

func (z *TimeLock) ToDisplay() *TimeLock {
	if z.IsNormalized() == true {
		t := &TimeLock{}
		t.Items = z.Items[1:]
		return t
	}
	return z
}

// String wacom
func (z *TimeLock) String() string {
	b, _ := json.Marshal(z.ToDisplay().Items)
	return string(b)
}

func (z *TimeLock) findNeed(item *TimeLockItem) (bool, []*TimeLockItem) {
	item = item.Clone()
	z.mergeAndSortValue()
	remaining := make([]*TimeLockItem, 0)
	for i := 0; i < len(z.Items); i++ {
		temp := z.Items[i]
		if temp.StartTime <= item.StartTime && temp.EndTime >= item.EndTime {
			missingValue, tempItems := temp.SubWithMissingValue(item)
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
			_, rem := temp.SubWithMissingValue(&TimeLockItem{
				StartTime: temp.StartTime,
				EndTime:   temp.EndTime,
				Value:     mergeValue,
			})

			rems = append(rems, rem...)

			_, rem = item.SubWithMissingValue(&TimeLockItem{
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
