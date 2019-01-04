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
	"enode://dfe72080ee663ea3f95d5fa695d84b572769850046014d40417f175ef5eeb80279b2093c669fbbc80ca15145b130a3ea39362f5464e15a61eb3cd22b1a405870@n1.fusion.org:16002",
	"enode://c515710980e9ab965d219a3b132f157c05d6a7e0dfc3d1caa1861d641602c5a9e324ab6aaa47dbc647de5c2a3e344e51af6e8aebf0771acfe3516f534e2fb9a6@n2.fusion.org:16002",
	"enode://aa406dc1a13527171d97bdb611f671479104f56610186c8c7f7d90e6d16df9b9df7192a5138f6071f3935a3fc071cbd3c55bb6b7c2d04ffee54c6057da1e2ff2@n3.fusion.org:16002",
	"enode://7f464be47fe0774c955c86207d0a4e914a56acf56eed7f728e011a84146c555bfd4ff3d6f1310276341ac406d51a21e959b666513cc5062a4533c978b57a7536@n4.fusion.org:16002",
	"enode://aa67dcc402fe2a7542b6beb05cfa37bc187b4b901253e4a4448dc804e16b5685ddeb1356c683821b738f605fd37b1b95bf2132a831cfa5f741ec20ef62bdc029@n5.fusion.org:8001",
	"enode://d6607b944c521ff73d1e3a4a773408360cb13587d690b6035e588555b3d14c0aa5d03fcbf26f91dcc64aa73e55b8906fe44f652496ad58582f5b378cdb09d678@n6.fusion.org:8001",
	"enode://3a27d71f6554f971844f602176f1ff78e2feb91091230c820818fe9cae73d261528d05751d3fde5ae7833894c0d8b08754ec2e1442d120fea6c6cae240c2d18e@n7.fusion.org:8001",
	"enode://8da6f53c325448e0c24b220c13a4b16609f1b1cea3d32ddb862b64eda4ad639a62879fde5f8669e8764d073b27f47ad6fced28bca1d58aab07613b3d503bee76@n8.fusion.org:8001",
}

// TestnetBootnodes are the enode URLs of the P2P bootstrap nodes running on the
// Ropsten test network.
var TestnetBootnodes = []string{
	"enode://30b7ab30a01c124a6cceca36863ece12c4f5fa68e3ba9b0b51407ccc002eeed3b3102d20a88f1c1d3c3154e2449317b8ef95090e77b312d5cc39354f86d5d606@52.176.7.10:40404",    // US-Azure efsn
	"enode://865a63255b3bb68023b6bffd5095118fcc13e79dcf014fe4e47e065c350c7cc72af2e53eff895f11ba1bbb6a2b33271c1116ee870f266618eadfc2e78aa7349c@52.176.100.77:40404",  // US-Azure parity
	"enode://6332792c4a00e3e4ee0926ed89e0d27ef985424d97b6a45bf0f23e51f0dcb5e66b875777506458aea7af6f9e4ffb69f43f3778ee73c81ed9d34c51c4b16b0b0f@52.232.243.152:40404", // Parity
	"enode://94c15d1b9e2fe7ce56e458b9a3b672ef11894ddedd0c6f247e0f1d3487f52b66208fb4aeb8179fce6e3a749ea93ed147c37976d67af557508d199d9594c35f09@192.81.208.223:40404", // @gpip
}

// RinkebyBootnodes are the enode URLs of the P2P bootstrap nodes running on the
// Rinkeby test network.
var RinkebyBootnodes = []string{
	"enode://a24ac7c5484ef4ed0c5eb2d36620ba4e4aa13b8c84684e1b4aab0cebea2ae45cb4d375b77eab56516d34bfbd3c1a833fc51296ff084b770b94fb9028c4d25ccf@52.169.42.101:40404", // IE
	"enode://343149e4feefa15d882d9fe4ac7d88f885bd05ebb735e547f12e12080a9fa07c8014ca6fd7f373123488102fe5e34111f8509cf0b7de3f5b44339c9f25e87cb8@52.3.158.184:40404",  // INFURA
	"enode://b6b28890b006743680c52e64e0d16db57f28124885595fa03a562be1d2bf0f3a1da297d56b13da25fb992888fd556d4c1a27b1f39d531bde7de1921c90061cc6@159.89.28.211:40404", // AKASHA
}

// DiscoveryV5Bootnodes are the enode URLs of the P2P bootstrap nodes for the
// experimental RLPx v5 topic-discovery network.
var DiscoveryV5Bootnodes = []string{
	"enode://06051a5573c81934c9554ef2898eb13b33a34b94cf36b202b69fde139ca17a85051979867720d4bdae4323d4943ddf9aeeb6643633aa656e0be843659795007a@35.177.226.168:40404",
	"enode://0cc5f5ffb5d9098c8b8c62325f3797f56509bff942704687b6530992ac706e2cb946b90a34f1f19548cd3c7baccbcaea354531e5983c7d1bc0dee16ce4b6440b@40.118.3.223:30304",
	"enode://1c7a64d76c0334b0418c004af2f67c50e36a3be60b5e4790bdac0439d21603469a85fad36f2473c9a80eb043ae60936df905fa28f1ff614c3e5dc34f15dcd2dc@40.118.3.223:30306",
	"enode://85c85d7143ae8bb96924f2b54f1b3e70d8c4d367af305325d30a61385a432f247d2c75c45c6b4a60335060d072d7f5b35dd1d4c45f76941f62a4f83b6e75daaf@40.118.3.223:30307",
}
