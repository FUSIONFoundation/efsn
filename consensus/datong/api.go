package datong

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/FusionFoundation/efsn/v5/common"
	"github.com/FusionFoundation/efsn/v5/consensus"
	"github.com/FusionFoundation/efsn/v5/core/types"
	"github.com/FusionFoundation/efsn/v5/rlp"
	"github.com/FusionFoundation/efsn/v5/rpc"
)

// API wacom
type API struct {
	chain consensus.ChainReader
}

func getSnapshotByHeader(header *types.Header) (*Snapshot, error) {
	// Ensure we have an actually valid block and return its snapshot
	if header == nil {
		return nil, errUnknownBlock
	}
	snap, err := NewSnapshotFromHeader(header)
	if err != nil {
		if header.Number.Uint64() == 0 {
			// return an empty snapshot
			return newSnapshot().ToShow(), nil
		}
		return nil, err
	}
	return snap, nil
}

// GetSnapshot wacom
func (api *API) GetSnapshot(number *rpc.BlockNumber) (*Snapshot, error) {
	var header *types.Header
	if number == nil || *number == rpc.LatestBlockNumber {
		header = api.chain.CurrentHeader()
	} else {
		header = api.chain.GetHeaderByNumber(uint64(number.Int64()))
	}
	return getSnapshotByHeader(header)
}

// GetSnapshotAtHash wacom
func (api *API) GetSnapshotAtHash(hash common.Hash) (*Snapshot, error) {
	header := api.chain.GetHeaderByHash(hash)
	return getSnapshotByHeader(header)
}

// DecodeLogData decode log data
func DecodeLogData(data []byte) (interface{}, error) {
	maps := make(map[string]interface{})
	if err := json.Unmarshal(data, &maps); err != nil {
		return nil, fmt.Errorf("json unmarshal err: %v", err)
	}
	// adjust Value from float64 to big.Int to prevent losing precision
	if value, exist := maps["Value"]; exist {
		if _, ok := value.(float64); ok {
			bigVal := struct {
				Value *big.Int `json:",string"`
			}{}
			if err := json.Unmarshal(data, &bigVal); err == nil {
				maps["Value"] = bigVal.Value
			}
		}
	}
	if datastr, dataok := maps["Base"].(string); dataok {
		data, err := base64.StdEncoding.DecodeString(datastr)
		if err != nil {
			return nil, fmt.Errorf("base64 decode err: %v", err)
		}
		buyTicketParam := common.BuyTicketParam{}
		if err = rlp.DecodeBytes(data, &buyTicketParam); err == nil {
			delete(maps, "Base")
			maps["StartTime"] = buyTicketParam.Start
			maps["ExpireTime"] = buyTicketParam.End
		}
	}
	return maps, nil
}
