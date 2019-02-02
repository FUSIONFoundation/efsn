package datong

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
)

// API wacom
type API struct {
	chain consensus.ChainReader
}

// GetSnapshot wacom
func (api *API) GetSnapshot(number *rpc.BlockNumber) (*Snapshot, error) {
	var header *types.Header
	if number == nil || *number == rpc.LatestBlockNumber {
		header = api.chain.CurrentHeader()
	} else {
		header = api.chain.GetHeaderByNumber(uint64(number.Int64()))
	}
	// Ensure we have an actually valid block and return its snapshot
	if header == nil {
		return nil, errUnknownBlock
	}
	snap, err := newSnapshotWithData(getSnapDataByHeader(header))
	if err != nil {
		return nil, err
	}
	return snap.ToShow(), nil
}

// GetSnapshotAtHash wacom
func (api *API) GetSnapshotAtHash(hash common.Hash) (*Snapshot, error) {
	header := api.chain.GetHeaderByHash(hash)
	if header == nil {
		return nil, errUnknownBlock
	}
	snap, err := newSnapshotWithData(getSnapDataByHeader(header))
	if err != nil {
		return nil, err
	}
	return snap.ToShow(), nil
}
