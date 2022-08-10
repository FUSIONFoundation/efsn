// Copyright 2021 The go-ethereum Authors
// This file is part of go-ethereum.
//
// go-ethereum is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// go-ethereum is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with go-ethereum. If not, see <http://www.gnu.org/licenses/>.

package t8ntool

import (
	"crypto/ecdsa"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"

	"github.com/FusionFoundation/efsn/v4/common"
	"github.com/FusionFoundation/efsn/v4/common/hexutil"
	"github.com/FusionFoundation/efsn/v4/common/math"
	"github.com/FusionFoundation/efsn/v4/core/types"
	"github.com/FusionFoundation/efsn/v4/crypto"
	"github.com/FusionFoundation/efsn/v4/log"
	"github.com/FusionFoundation/efsn/v4/rlp"
	"github.com/urfave/cli/v2"
)

//go:generate go run github.com/fjl/gencodec -type header -field-override headerMarshaling -out gen_header.go
type header struct {
	ParentHash  common.Hash       `json:"parentHash"`
	OmmerHash   *common.Hash      `json:"sha3Uncles"`
	Coinbase    *common.Address   `json:"miner"`
	Root        common.Hash       `json:"stateRoot"        gencodec:"required"`
	TxHash      *common.Hash      `json:"transactionsRoot"`
	ReceiptHash *common.Hash      `json:"receiptsRoot"`
	Bloom       types.Bloom       `json:"logsBloom"`
	Difficulty  *big.Int          `json:"difficulty"`
	Number      *big.Int          `json:"number"           gencodec:"required"`
	GasLimit    uint64            `json:"gasLimit"         gencodec:"required"`
	GasUsed     uint64            `json:"gasUsed"`
	Time        uint64            `json:"timestamp"        gencodec:"required"`
	Extra       []byte            `json:"extraData"`
	MixDigest   common.Hash       `json:"mixHash"`
	Nonce       *types.BlockNonce `json:"nonce"`
	BaseFee     *big.Int          `json:"baseFeePerGas" rlp:"optional"`
}

type headerMarshaling struct {
	Difficulty *math.HexOrDecimal256
	Number     *math.HexOrDecimal256
	GasLimit   math.HexOrDecimal64
	GasUsed    math.HexOrDecimal64
	Time       math.HexOrDecimal64
	Extra      hexutil.Bytes
	BaseFee    *math.HexOrDecimal256
}

type bbInput struct {
	Header    *header      `json:"header,omitempty"`
	OmmersRlp []string     `json:"ommers,omitempty"`
	TxRlp     string       `json:"txs,omitempty"`
	Clique    *cliqueInput `json:"clique,omitempty"`

	Ethash    bool                 `json:"-"`
	EthashDir string               `json:"-"`
	Txs       []*types.Transaction `json:"-"`
	Ommers    []*types.Header      `json:"-"`
}

type cliqueInput struct {
	Key       *ecdsa.PrivateKey
	Voted     *common.Address
	Authorize *bool
	Vanity    common.Hash
}

// UnmarshalJSON implements json.Unmarshaler interface.
func (c *cliqueInput) UnmarshalJSON(input []byte) error {
	var x struct {
		Key       *common.Hash    `json:"secretKey"`
		Voted     *common.Address `json:"voted"`
		Authorize *bool           `json:"authorize"`
		Vanity    common.Hash     `json:"vanity"`
	}
	if err := json.Unmarshal(input, &x); err != nil {
		return err
	}
	if x.Key == nil {
		return errors.New("missing required field 'secretKey' for cliqueInput")
	}
	if ecdsaKey, err := crypto.ToECDSA(x.Key[:]); err != nil {
		return err
	} else {
		c.Key = ecdsaKey
	}
	c.Voted = x.Voted
	c.Authorize = x.Authorize
	c.Vanity = x.Vanity
	return nil
}

// ToBlock converts i into a *types.Block
func (i *bbInput) ToBlock() *types.Block {
	header := &types.Header{
		ParentHash:  i.Header.ParentHash,
		UncleHash:   types.EmptyUncleHash,
		Coinbase:    common.Address{},
		Root:        i.Header.Root,
		TxHash:      types.EmptyRootHash,
		ReceiptHash: types.EmptyRootHash,
		Bloom:       i.Header.Bloom,
		Difficulty:  common.Big0,
		Number:      i.Header.Number,
		GasLimit:    i.Header.GasLimit,
		GasUsed:     i.Header.GasUsed,
		Time:        i.Header.Time,
		Extra:       i.Header.Extra,
		MixDigest:   i.Header.MixDigest,
		BaseFee:     i.Header.BaseFee,
	}

	// Fill optional values.
	if i.Header.OmmerHash != nil {
		header.UncleHash = *i.Header.OmmerHash
	} else if len(i.Ommers) != 0 {
		// Calculate the ommer hash if none is provided and there are ommers to hash
		header.UncleHash = types.CalcUncleHash(i.Ommers)
	}
	if i.Header.Coinbase != nil {
		header.Coinbase = *i.Header.Coinbase
	}
	if i.Header.TxHash != nil {
		header.TxHash = *i.Header.TxHash
	}
	if i.Header.ReceiptHash != nil {
		header.ReceiptHash = *i.Header.ReceiptHash
	}
	if i.Header.Nonce != nil {
		header.Nonce = *i.Header.Nonce
	}
	if header.Difficulty != nil {
		header.Difficulty = i.Header.Difficulty
	}
	return types.NewBlockWithHeader(header).WithBody(i.Txs, i.Ommers)
}

// SealBlock seals the given block using the configured engine.
func (i *bbInput) SealBlock(block *types.Block) (*types.Block, error) {
	switch {
	case i.Ethash:
		return block, nil
	case i.Clique != nil:
		return block, nil
	default:
		return block, nil
	}
}

// BuildBlock constructs a block from the given inputs.
func BuildBlock(ctx *cli.Context) error {
	// Configure the go-ethereum logger
	glogger := log.NewGlogHandler(log.StreamHandler(os.Stderr, log.TerminalFormat(false)))
	glogger.Verbosity(log.Lvl(ctx.Int(VerbosityFlag.Name)))
	log.Root().SetHandler(glogger)

	baseDir, err := createBasedir(ctx)
	if err != nil {
		return NewError(ErrorIO, fmt.Errorf("failed creating output basedir: %v", err))
	}
	inputData, err := readInput(ctx)
	if err != nil {
		return err
	}
	block := inputData.ToBlock()
	block, err = inputData.SealBlock(block)
	if err != nil {
		return err
	}
	return dispatchBlock(ctx, baseDir, block)
}

func readInput(ctx *cli.Context) (*bbInput, error) {
	var (
		headerStr = ctx.String(InputHeaderFlag.Name)
		ommersStr = ctx.String(InputOmmersFlag.Name)
		txsStr    = ctx.String(InputTxsRlpFlag.Name)
		cliqueStr = ctx.String(SealCliqueFlag.Name)
		ethashOn  = ctx.Bool(SealEthashFlag.Name)
		ethashDir = ctx.String(SealEthashDirFlag.Name)
		//ethashMode = ctx.String(SealEthashModeFlag.Name)
		inputData = &bbInput{}
	)
	if ethashOn && cliqueStr != "" {
		return nil, NewError(ErrorConfig, fmt.Errorf("both ethash and clique sealing specified, only one may be chosen"))
	}
	if ethashOn {
		inputData.Ethash = ethashOn
		inputData.EthashDir = ethashDir
		//switch ethashMode {
		//case "normal":
		//	inputData.PowMode = ethash.ModeNormal
		//case "test":
		//	inputData.PowMode = ethash.ModeTest
		//case "fake":
		//	inputData.PowMode = ethash.ModeFake
		//default:
		//	return nil, NewError(ErrorConfig, fmt.Errorf("unknown pow mode: %s, supported modes: test, fake, normal", ethashMode))
		//}
	}
	if headerStr == stdinSelector || ommersStr == stdinSelector || txsStr == stdinSelector || cliqueStr == stdinSelector {
		decoder := json.NewDecoder(os.Stdin)
		if err := decoder.Decode(inputData); err != nil {
			return nil, NewError(ErrorJson, fmt.Errorf("failed unmarshaling stdin: %v", err))
		}
	}
	if cliqueStr != stdinSelector && cliqueStr != "" {
		var clique cliqueInput
		if err := readFile(cliqueStr, "clique", &clique); err != nil {
			return nil, err
		}
		inputData.Clique = &clique
	}
	if headerStr != stdinSelector {
		var env header
		if err := readFile(headerStr, "header", &env); err != nil {
			return nil, err
		}
		inputData.Header = &env
	}
	if ommersStr != stdinSelector && ommersStr != "" {
		var ommers []string
		if err := readFile(ommersStr, "ommers", &ommers); err != nil {
			return nil, err
		}
		inputData.OmmersRlp = ommers
	}
	if txsStr != stdinSelector {
		var txs string
		if err := readFile(txsStr, "txs", &txs); err != nil {
			return nil, err
		}
		inputData.TxRlp = txs
	}
	// Deserialize rlp txs and ommers
	var (
		ommers = []*types.Header{}
		txs    = []*types.Transaction{}
	)
	if inputData.TxRlp != "" {
		if err := rlp.DecodeBytes(common.FromHex(inputData.TxRlp), &txs); err != nil {
			return nil, NewError(ErrorRlp, fmt.Errorf("unable to decode transaction from rlp data: %v", err))
		}
		inputData.Txs = txs
	}
	for _, str := range inputData.OmmersRlp {
		type extblock struct {
			Header *types.Header
			Txs    []*types.Transaction
			Ommers []*types.Header
		}
		var ommer *extblock
		if err := rlp.DecodeBytes(common.FromHex(str), &ommer); err != nil {
			return nil, NewError(ErrorRlp, fmt.Errorf("unable to decode ommer from rlp data: %v", err))
		}
		ommers = append(ommers, ommer.Header)
	}
	inputData.Ommers = ommers

	return inputData, nil
}

// dispatchOutput writes the output data to either stderr or stdout, or to the specified
// files
func dispatchBlock(ctx *cli.Context, baseDir string, block *types.Block) error {
	raw, _ := rlp.EncodeToBytes(block)

	type blockInfo struct {
		Rlp  hexutil.Bytes `json:"rlp"`
		Hash common.Hash   `json:"hash"`
	}
	var enc blockInfo
	enc.Rlp = raw
	enc.Hash = block.Hash()

	b, err := json.MarshalIndent(enc, "", "  ")
	if err != nil {
		return NewError(ErrorJson, fmt.Errorf("failed marshalling output: %v", err))
	}
	switch dest := ctx.String(OutputBlockFlag.Name); dest {
	case "stdout":
		os.Stdout.Write(b)
		os.Stdout.WriteString("\n")
	case "stderr":
		os.Stderr.Write(b)
		os.Stderr.WriteString("\n")
	default:
		if err := saveFile(baseDir, dest, enc); err != nil {
			return err
		}
	}
	return nil
}
