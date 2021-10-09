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
	"time"

	"github.com/FusionFoundation/efsn/common"
	"github.com/FusionFoundation/efsn/rlp"
	"github.com/FusionFoundation/efsn/trie"
)

// DumpAccount represents an account in the state.
type DumpAccount struct {
	Balances         map[common.Hash]*big.Int         `json:"balance"`
	TimeLockBalances map[common.Hash]*common.TimeLock `json:"timelock"`
	Nonce            uint64                           `json:"nonce"`
	Root             hexutil.Bytes                    `json:"root"`
	CodeHash         hexutil.Bytes                    `json:"codeHash"`
	Code             hexutil.Bytes                    `json:"code,omitempty"`
	Storage          map[common.Hash]string           `json:"storage,omitempty"`
	Address          *common.Address                  `json:"address,omitempty"` // Address only present in iterative (line-by-line) mode
	SecureKey        hexutil.Bytes                    `json:"key,omitempty"`     // If we don't have address, we can output the key
}

// Dump represents the full dump in a collected format, as one large map.
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

	var (
		missingPreimages int
		accounts         uint64
		start            = time.Now()
		logged           = time.Now()
	)
	log.Info("Trie dumping started", "root", s.trie.Hash())

	it := trie.NewIterator(s.trie.NodeIterator(nil))
	for it.Next() {
		var data Account
		if err := rlp.DecodeBytes(it.Value, &data); err != nil {
			panic(err)
		}
		// Fusion Balance
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
			SecureKey:        it.Key,
		}
		addrBytes := s.trie.GetKey(it.Key)
		if addrBytes == nil {
			// Preimage missing
			missingPreimages++
			account.SecureKey = it.Key
		}
		addr := common.BytesToAddress(addrBytes)
		obj := newObject(s, addr, data)

		account.Storage = make(map[common.Hash]string)
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
		accounts++
		if time.Since(logged) > 8*time.Second {
			log.Info("Trie dumping in progress", "at", it.Key, "accounts", accounts,
				"elapsed", common.PrettyDuration(time.Since(start)))
			logged = time.Now()
		}
	}
	if missingPreimages > 0 {
		log.Warn("Dump incomplete due to missing preimages", "missing", missingPreimages)
	}
	log.Info("Trie dumping complete", "accounts", accounts,
		"elapsed", common.PrettyDuration(time.Since(start)))
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
