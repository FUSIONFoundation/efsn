package datong

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/FusionFoundation/efsn/v4/common"
	"github.com/FusionFoundation/efsn/v4/common/hexutil"
	"github.com/FusionFoundation/efsn/v4/core/types"
	"github.com/FusionFoundation/efsn/v4/core/vm"
	"github.com/FusionFoundation/efsn/v4/rlp"
)

const (
	maxPunishTicketCount = 1
	maxReportDepth       = 100
)

func buildReportData(header1, header2 *types.Header) ([]byte, error) {
	data1, err := rlp.EncodeToBytes(header1)
	if err != nil {
		return nil, err
	}
	data2, err := rlp.EncodeToBytes(header2)
	if err != nil {
		return nil, err
	}

	cmp := bytes.Compare(data1, data2)
	if cmp > 0 {
		data1, data2 = data2, data1
	} else if cmp == 0 {
		return nil, fmt.Errorf("buildReportData error: headers are same")
	}

	data1len := len(data1)
	data := make([]byte, 0, 4+len(data1)+len(data2))
	data = append(data, common.IntToBytes(data1len)...)
	data = append(data, data1...)
	data = append(data, data2...)
	return data, nil
}

func ReportIllegal(header1, header2 *types.Header) {
	if !common.IsMultipleMiningCheckingEnabled(header1.Number) {
		return
	}

	data, err := buildReportData(header1, header2)
	if err != nil {
		return
	}

	go func() {
		common.ReportIllegalChan <- data
	}()
}

func DecodeReport(report []byte) (*types.Header, *types.Header, error) {
	if len(report) < 4 {
		return nil, nil, fmt.Errorf("wrong report length")
	}
	data1len := common.BytesToInt(report[:4])
	if len(report) < 4+data1len {
		return nil, nil, fmt.Errorf("wrong report length")
	}
	data1 := report[4 : data1len+4]
	data2 := report[data1len+4:]

	if bytes.Compare(data1, data2) >= 0 {
		return nil, nil, fmt.Errorf("wrong report sequence")
	}

	header1 := &types.Header{}
	header2 := &types.Header{}

	if err := rlp.DecodeBytes(data1, header1); err != nil {
		return nil, nil, fmt.Errorf("can not decode header1, err=%v", err)
	}
	if err := rlp.DecodeBytes(data2, header2); err != nil {
		return nil, nil, fmt.Errorf("can not decode header2, err=%v", err)
	}
	return header1, header2, nil
}

func CheckAddingReport(state vm.StateDB, report []byte, blockNumber *big.Int) (*types.Header, *types.Header, error) {
	if state.IsReportExist(report) {
		return nil, nil, fmt.Errorf("CheckAddingReport: report exist")
	}

	header1, header2, err := DecodeReport(report)
	if err != nil {
		return nil, nil, err
	}

	if blockNumber != nil && blockNumber.Uint64()-header1.Number.Uint64() > maxReportDepth {
		return nil, nil, fmt.Errorf("report error: too long ago")
	}

	if header1.Coinbase != header2.Coinbase {
		return nil, nil, fmt.Errorf("report error: miners are not same")
	}
	if header1.ParentHash != header2.ParentHash {
		return nil, nil, fmt.Errorf("report error: parents are not same")
	}
	if header1.Hash() == header2.Hash() {
		return nil, nil, fmt.Errorf("report error: use same block")
	}

	if err := VerifySignature(header1); err != nil {
		return nil, nil, fmt.Errorf("report error: verify header1 signature fail")
	}

	if err := VerifySignature(header2); err != nil {
		return nil, nil, fmt.Errorf("report error: verify header2 signature fail")
	}

	return header1, header2, nil
}

// punish miner and reward reporter
func ProcessReport(heade1, header2 *types.Header, reporter common.Address, state vm.StateDB, height *big.Int, timestamp uint64) []common.Hash {
	miner := heade1.Coinbase
	deleteTickets := punishTicket(state, miner)
	if len(deleteTickets) < maxPunishTicketCount {
		diffCount := int64(maxPunishTicketCount - len(deleteTickets))
		value := new(big.Int).Mul(common.TicketPrice(height), big.NewInt(diffCount))
		punishTimeLock(state, miner, value, height, timestamp)
	}
	return deleteTickets
}

func punishTicket(state vm.StateDB, miner common.Address) []common.Hash {
	// delete tickets from the miner
	allTickets, err := state.AllTickets()
	if err != nil {
		return nil
	}
	var tickets common.TicketSlice
	for _, v := range allTickets {
		if v.Owner == miner {
			tickets = v.ToTicketSlice()
			break
		}
	}

	leftTickets := len(tickets)
	if leftTickets == 0 {
		return nil
	}

	count := maxPunishTicketCount
	if leftTickets < count {
		count = leftTickets
	}

	ids := make([]common.Hash, count)
	for i := 0; i < count; i++ {
		ids[i] = tickets[len(tickets)-1-i].ID
	}
	for _, id := range ids {
		state.RemoveTicket(id)
	}
	return ids
}

func punishTimeLock(state vm.StateDB, miner common.Address, value *big.Int, height *big.Int, timestamp uint64) {
	needStart := timestamp
	needEnd := needStart + 30*24*3600
	needValue := value
	needItem := &common.TimeLockItem{
		StartTime: needStart,
		EndTime:   needEnd,
		Value:     needValue,
	}
	needTimelock := common.NewTimeLock(needItem)
	timelockBalance := state.GetTimeLockBalance(common.SystemAssetID, miner)
	if timelockBalance.Cmp(needTimelock) >= 0 {
		state.SubTimeLockBalance(miner, common.SystemAssetID, needTimelock, height, timestamp)
	} else {
		var leftItems []*common.TimeLockItem
		for i, item := range timelockBalance.Items {
			if item.EndTime < needStart { // expired
				continue
			}
			if item.StartTime > needEnd { // kept
				leftItems = append(leftItems, timelockBalance.Items[i:]...)
				break
			}
			if item.Value.Cmp(needValue) > 0 { // punished
				leftItems = append(leftItems, &common.TimeLockItem{
					StartTime: common.MaxUint64(needStart, item.StartTime),
					EndTime:   common.MinUint64(needEnd, item.EndTime),
					Value:     new(big.Int).Sub(item.Value, needValue),
				})
			}
			if item.EndTime > needEnd { // left (has intersection)
				leftItems = append(leftItems, &common.TimeLockItem{
					StartTime: needEnd + 1,
					EndTime:   item.EndTime,
					Value:     item.Value,
				})
			}
		}
		timelockBalance.SetItems(leftItems)
		state.SetTimeLockBalance(miner, common.SystemAssetID, timelockBalance)
	}
}

func DecodeTxInput(input []byte) (interface{}, error) {
	res, err := common.DecodeTxInput(input)
	if err == nil {
		return res, err
	}
	fsnCall, ok := res.(common.FSNCallParam)
	if !ok {
		return res, err
	}
	switch fsnCall.Func {
	case common.ReportIllegalFunc:
		h1, h2, err := DecodeReport(fsnCall.Data)
		if err != nil {
			return nil, fmt.Errorf("DecodeReport err %v", err)
		}
		reportContent := &struct {
			Header1 *types.Header
			Header2 *types.Header
		}{
			Header1: h1,
			Header2: h2,
		}
		fsnCall.Data = nil
		return common.DecodeFsnCallParam(&fsnCall, reportContent)
	}
	return nil, fmt.Errorf("Unknown FuncType %v", fsnCall.Func)
}

func DecodePunishTickets(data []byte) ([]common.Hash, error) {
	maps := make(map[string]interface{})
	err := json.Unmarshal(data, &maps)
	if err != nil {
		return nil, err
	}

	if _, hasError := maps["Error"]; hasError {
		return nil, nil
	}

	ids, idsok := maps["DeleteTickets"].(string)
	if !idsok {
		return nil, fmt.Errorf("report log has wrong data")
	}

	bs, err := hexutil.Decode(ids)
	if err != nil {
		return nil, fmt.Errorf("decode hex data error: %v", err)
	}
	delTickets := []common.Hash{}
	if err := rlp.DecodeBytes(bs, &delTickets); err != nil {
		return nil, fmt.Errorf("decode report log error: %v", err)
	}

	return delTickets, nil
}
