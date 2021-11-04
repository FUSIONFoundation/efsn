package datong

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/big"

	"github.com/FusionFoundation/efsn/v4/common"
	"github.com/FusionFoundation/efsn/v4/core/types"
	"github.com/FusionFoundation/efsn/v4/log"
	"github.com/FusionFoundation/efsn/v4/params"
)

var (
	// hard coded check points
	mainnetCheckPoints = map[uint64]common.Hash{
		10000:   common.HexToHash("0x1830e440e17cda3a26d02ea650331a584d58a499cbbd3821c622568c8de9b470"),
		100000:  common.HexToHash("0x0e4a4045c06984472f86bb8f1e40bb70dee87b6227877ffcaeab7624db745cd2"),
		500000:  common.HexToHash("0xad25bc52b8e674494970aa3a1bb7e775564714cbc4798975730ad1b9c62ccc89"),
		739500:  common.HexToHash("0xa64a4ecee941a7710d2942ac7e1b69e7ba6431ab38d82e6771ede7051bafee8f"),
		1000000: common.HexToHash("0x4b0d0d5a0739c801c3d4fe91258d3b9ddf81f471464e221921442ea503d711a6"),
		1500000: common.HexToHash("0x2808c2f24aa2280453e257970a9cf05ef9a4cf5c1a742a9a466a633aff45400a"),
		1700000: common.HexToHash("0x39a997b94b24a050a3b222cd89427cdffedc8706a24160e6bab06ffa95f1c7fa"),
		2680000: common.HexToHash("0x4c49d0119abea53f5cb0ba6107bf481670397e8b814fa4360353eedded9bf383"),
	}
	mainnetLastCheckPoint uint64 = 2680000

	testnetCheckPoints = map[uint64]common.Hash{
		10000:   common.HexToHash("0x26a9441584f9b312e9e42df099e5b72f06e71a6335a31b65eba48782b506af5f"),
		100000:  common.HexToHash("0xfcdd0b71de9b84bf635a5b30ae4b6f483ccd49fa0c9780f49bfae030a6bc2064"),
		500000:  common.HexToHash("0x710920258a903b3af92f8ac4f1cd66784cb001efa3b1c2ab4a178479ce8bdfe8"),
		534500:  common.HexToHash("0xe8049f5930c8e5ebf3d79b9de9eb267666afb8225e797d24010abc2341b264b5"),
		1000000: common.HexToHash("0x6165f4fd79216afc5ef3f15e01e42aff9d1f252d1b7baa4395aabeeb89368615"),
		1500000: common.HexToHash("0xd984e123d3a16f754faf77b4d18afc532a9101b7a3b4cf20cd374d376223d1a5"),
		1577000: common.HexToHash("0xeeec5e5781ef697e5b181bc5da8c90fe6c80624e235bbe331e19da62766e1345"),
		2621077: common.HexToHash("0xada50d3c256310c2eb41127ebbb426673b6b12d7c89b5f51f973cb4c944c302c"),
	}
	testnetLastCheckPoint uint64 = 2621077

	devnetCheckPoints           = map[uint64]common.Hash{}
	devnetLastCheckPoint uint64 = 0
)

var (
	CheckPoints    map[uint64]common.Hash
	LastCheckPoint uint64
)

func InitCheckPoints(file string) {
	if common.UseTestnetRule {
		CheckPoints = testnetCheckPoints
		LastCheckPoint = testnetLastCheckPoint
	} else if common.UseDevnetRule {
		CheckPoints = devnetCheckPoints
		LastCheckPoint = devnetLastCheckPoint
	} else {
		CheckPoints = mainnetCheckPoints
		LastCheckPoint = mainnetLastCheckPoint
	}

	defer func() {
		log.Info("InitCheckPoints finished", "count", len(CheckPoints), "last", LastCheckPoint)
	}()

	// custom check points
	if file == "" {
		return
	}
	data, err := ioutil.ReadFile(file)
	if err != nil {
		log.Error("Could not read check ponits file", "err", err)
		return
	}
	var cpoints map[uint64]string
	if err := json.Unmarshal(data, &cpoints); err != nil {
		log.Error("InitCheckPoints unmarshal json failed", "err", err)
		return
	}
	for k, v := range cpoints {
		CheckPoints[k] = common.HexToHash(v)
		if k > LastCheckPoint {
			LastCheckPoint = k
		}
	}
}

func IsInCheckPointsRange(blockHeight uint64) bool {
	return blockHeight <= LastCheckPoint
}

func CheckPoint(chainID *big.Int, blockHeight uint64, blockHash common.Hash) (isInRange bool, err error) {
	var defaultChainID *big.Int
	switch {
	case common.UseTestnetRule:
		defaultChainID = params.DevnetChainConfig.ChainID
	case common.UseDevnetRule:
		defaultChainID = params.TestnetChainConfig.ChainID
	default: // maininet
		defaultChainID = params.MainnetChainConfig.ChainID
	}
	// private chain
	if chainID == nil || chainID.Cmp(defaultChainID) != 0 {
		return false, nil
	}
	if blockHeight > LastCheckPoint {
		return false, nil
	}
	hash, exist := CheckPoints[blockHeight]
	if exist {
		if blockHash != hash {
			log.Info("check point failed, block hash mismatch", "number", blockHeight, "have", blockHash, "want", hash)
			return true, fmt.Errorf("check point failed, block hash mismatch: number=%v, have 0x%x, want 0x%x", blockHeight, blockHash, hash)
		} else {
			log.Info("check point passed", "number", blockHeight, "hash", blockHash)
		}
	}
	return true, nil
}

func CheckPointsInBlockChain(chainID *big.Int, chain types.Blocks) (int, error) {
	headers := make([]*types.Header, len(chain))
	for i, block := range chain {
		headers[i] = block.Header()
	}
	return CheckPointsInHeaderChain(chainID, headers)
}

func CheckPointsInHeaderChain(chainID *big.Int, chain []*types.Header) (int, error) {
	var defaultChainID *big.Int
	switch {
	case common.UseTestnetRule:
		defaultChainID = params.DevnetChainConfig.ChainID
	case common.UseDevnetRule:
		defaultChainID = params.TestnetChainConfig.ChainID
	default: // maininet
		defaultChainID = params.MainnetChainConfig.ChainID
	}
	// private chain
	if chainID == nil || chainID.Cmp(defaultChainID) != 0 {
		return 0, nil
	}
	for i, header := range chain {
		blockHeight := header.Number.Uint64()
		if blockHeight > LastCheckPoint {
			break
		}
		hash, exist := CheckPoints[blockHeight]
		if !exist {
			continue
		}
		blockHash := header.Hash()
		if blockHash != hash {
			log.Info("check point failed, block hash mismatch", "number", blockHeight, "have", blockHash, "want", hash)
			return i, fmt.Errorf("check point failed, block hash mismatch: number=%v, have 0x%x, want 0x%x", blockHeight, blockHash, hash)
		} else {
			log.Info("check point passed", "number", blockHeight, "hash", blockHash)
		}
	}
	return 0, nil
}
