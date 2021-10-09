// Copyright 2014 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package state

import (
	"encoding/json"
	"fmt"
	"github.com/FusionFoundation/efsn/common/hexutil"
	"github.com/FusionFoundation/efsn/log"
	"math/big"

	"github.com/FusionFoundation/efsn/common"
	"github.com/FusionFoundation/efsn/rlp"
	"github.com/FusionFoundation/efsn/trie"
)

type DumpAccount struct {
	Balances         map[common.Hash]*big.Int         `json:"balance"`
	TimeLockBalances map[common.Hash]*common.TimeLock `json:"timelock"`
	Nonce            uint64                           `json:"nonce"`
	Root             hexutil.Bytes                    `json:"root"`
	CodeHash         hexutil.Bytes                    `json:"codeHash"`
	Code             hexutil.Bytes                    `json:"code,omitempty"`
	Storage          map[common.Hash]string           `json:"storage,omitempty"`
}

type Dump struct {
	Root     string                         `json:"root"`
	Accounts map[common.Address]DumpAccount `json:"accounts"`
}

// RawDump returns the entire state an a single large object
func (s *StateDB) RawDump() Dump {
	dump := Dump{
		Root:     fmt.Sprintf("%x", s.trie.Hash()),
		Accounts: make(map[common.Address]DumpAccount),
	}

	it := trie.NewIterator(s.trie.NodeIterator(nil))
	for it.Next() {
		var data Account
		if err := rlp.DecodeBytes(it.Value, &data); err != nil {
			panic(err)
		}

		addrBytes := s.trie.GetKey(it.Key)
		addr := common.BytesToAddress(addrBytes)
		obj := newObject(s, addr, data)
		bal := make(map[common.Hash]*big.Int)
		for i, v := range data.BalancesHash {
			bal[v] = data.BalancesVal[i]
		}
		timelocks := make(map[common.Hash]*common.TimeLock)
		for i, v := range data.TimeLockBalancesHash {
			timelock := data.TimeLockBalancesVal[i]
			timelocks[v] = timelock
		}
		account := DumpAccount{
			Balances:         bal,
			TimeLockBalances: timelocks,
			Nonce:            data.Nonce,
			Root:             data.Root[:],
			CodeHash:         data.CodeHash,
			Code:             obj.Code(s.db),
			Storage:          make(map[common.Hash]string),
		}
		storageIt := trie.NewIterator(obj.getTrie(s.db).NodeIterator(nil))
		for storageIt.Next() {
			_, content, _, err := rlp.Split(storageIt.Value)
			if err != nil {
				log.Error("Failed to decode the value returned by iterator", "error", err)
				continue
			}
			account.Storage[common.BytesToHash(s.trie.GetKey(storageIt.Key))] = common.Bytes2Hex(content)
		}
		dump.Accounts[addr] = account
	}
	return dump
}

// Dump returns a JSON string representing the entire state as a single json-object
func (s *StateDB) Dump() []byte {
	dump := s.RawDump()
	json, err := json.MarshalIndent(dump, "", "    ")
	if err != nil {
		fmt.Println("dump err", err)
	}
	return json
}
