// Copyright 2015 The go-ethereum Authors
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

package params

// MainnetBootnodes are the enode URLs of the P2P bootstrap nodes running on
// the main Ethereum network.
var MainnetBootnodes = []string{
	"enode://dfe72080ee663ea3f95d5fa695d84b572769850046014d40417f175ef5eeb80279b2093c669fbbc80ca15145b130a3ea39362f5464e15a61eb3cd22b1a405870@bn1.fusionnetwork.io:40407",
	"enode://c515710980e9ab965d219a3b132f157c05d6a7e0dfc3d1caa1861d641602c5a9e324ab6aaa47dbc647de5c2a3e344e51af6e8aebf0771acfe3516f534e2fb9a6@bn2.fusionnetwork.io:40407",
	"enode://aa406dc1a13527171d97bdb611f671479104f56610186c8c7f7d90e6d16df9b9df7192a5138f6071f3935a3fc071cbd3c55bb6b7c2d04ffee54c6057da1e2ff2@bn3.fusionnetwork.io:40407",
	"enode://7f464be47fe0774c955c86207d0a4e914a56acf56eed7f728e011a84146c555bfd4ff3d6f1310276341ac406d51a21e959b666513cc5062a4533c978b57a7536@bn4.fusionnetwork.io:40407",
}

// TestnetBootnodes are the enode URLs of the P2P bootstrap nodes running on the
// Ropsten test network.
var TestnetBootnodes = []string{
	"enode://dfe72080ee663ea3f95d5fa695d84b572769850046014d40417f175ef5eeb80279b2093c669fbbc80ca15145b130a3ea39362f5464e15a61eb3cd22b1a405870@boottestnode1.fusionnetwork.io:40407",
	"enode://c515710980e9ab965d219a3b132f157c05d6a7e0dfc3d1caa1861d641602c5a9e324ab6aaa47dbc647de5c2a3e344e51af6e8aebf0771acfe3516f534e2fb9a6@boottestnode2.fusionnetwork.io:40407",
}

// DevnetBootnodes are the enode URLs of the P2P bootstrap nodes running on the
// Develop test network.
var DevnetBootnodes = []string{}

// DiscoveryV5Bootnodes are the enode URLs of the P2P bootstrap nodes for the
// experimental RLPx v5 topic-discovery network.
var DiscoveryV5Bootnodes = []string{
	"enode://06051a5573c81934c9554ef2898eb13b33a34b94cf36b202b69fde139ca17a85051979867720d4bdae4323d4943ddf9aeeb6643633aa656e0be843659795007a@35.177.226.168:40404",
	"enode://0cc5f5ffb5d9098c8b8c62325f3797f56509bff942704687b6530992ac706e2cb946b90a34f1f19548cd3c7baccbcaea354531e5983c7d1bc0dee16ce4b6440b@40.118.3.223:30304",
	"enode://1c7a64d76c0334b0418c004af2f67c50e36a3be60b5e4790bdac0439d21603469a85fad36f2473c9a80eb043ae60936df905fa28f1ff614c3e5dc34f15dcd2dc@40.118.3.223:30306",
	"enode://85c85d7143ae8bb96924f2b54f1b3e70d8c4d367af305325d30a61385a432f247d2c75c45c6b4a60335060d072d7f5b35dd1d4c45f76941f62a4f83b6e75daaf@40.118.3.223:30307",
}
