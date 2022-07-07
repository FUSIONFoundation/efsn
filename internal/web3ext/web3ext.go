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

// package web3ext contains efsn specific web3.js extensions.
package web3ext

var Modules = map[string]string{
	"admin":    Admin_JS,
	"debug":    Debug_JS,
	"eth":      Eth_JS,
	"miner":    Miner_JS,
	"net":      Net_JS,
	"personal": Personal_JS,
	"rpc":      RPC_JS,
	"txpool":   TxPool_JS,
	"fsn":      FsnJS,
	"fsntx":    FsnTxJS,
	"fsnbt":    FsnBT,
}

const Admin_JS = `
web3._extend({
	property: 'admin',
	methods: [
		new web3._extend.Method({
			name: 'addPeer',
			call: 'admin_addPeer',
			params: 1
		}),
		new web3._extend.Method({
			name: 'removePeer',
			call: 'admin_removePeer',
			params: 1
		}),
		new web3._extend.Method({
			name: 'addTrustedPeer',
			call: 'admin_addTrustedPeer',
			params: 1
		}),
		new web3._extend.Method({
			name: 'removeTrustedPeer',
			call: 'admin_removeTrustedPeer',
			params: 1
		}),
		new web3._extend.Method({
			name: 'exportChain',
			call: 'admin_exportChain',
			params: 1,
			inputFormatter: [null]
		}),
		new web3._extend.Method({
			name: 'importChain',
			call: 'admin_importChain',
			params: 1
		}),
		new web3._extend.Method({
			name: 'sleepBlocks',
			call: 'admin_sleepBlocks',
			params: 2
		}),
		new web3._extend.Method({
			name: 'startRPC',
			call: 'admin_startRPC',
			params: 4,
			inputFormatter: [null, null, null, null]
		}),
		new web3._extend.Method({
			name: 'stopRPC',
			call: 'admin_stopRPC'
		}),
		new web3._extend.Method({
			name: 'startWS',
			call: 'admin_startWS',
			params: 4,
			inputFormatter: [null, null, null, null]
		}),
		new web3._extend.Method({
			name: 'stopWS',
			call: 'admin_stopWS'
		}),
	],
	properties: [
		new web3._extend.Property({
			name: 'nodeInfo',
			getter: 'admin_nodeInfo'
		}),
		new web3._extend.Property({
			name: 'peers',
			getter: 'admin_peers'
		}),
		new web3._extend.Property({
			name: 'datadir',
			getter: 'admin_datadir'
		}),
	]
});
`

const Debug_JS = `
web3._extend({
	property: 'debug',
	methods: [
		new web3._extend.Method({
			name: 'printBlock',
			call: 'debug_printBlock',
			params: 1,
			inputFormatter: [web3._extend.formatters.inputBlockNumberFormatter]
		}),
		new web3._extend.Method({
			name: 'getBlockRlp',
			call: 'debug_getBlockRlp',
			params: 1
		}),
		new web3._extend.Method({
			name: 'setHead',
			call: 'debug_setHead',
			params: 1
		}),
		new web3._extend.Method({
			name: 'seedHash',
			call: 'debug_seedHash',
			params: 1
		}),
		new web3._extend.Method({
			name: 'dumpBlock',
			call: 'debug_dumpBlock',
			params: 1,
			inputFormatter: [web3._extend.formatters.inputBlockNumberFormatter]
		}),
		new web3._extend.Method({
			name: 'chaindbProperty',
			call: 'debug_chaindbProperty',
			params: 1,
			outputFormatter: console.log
		}),
		new web3._extend.Method({
			name: 'chaindbCompact',
			call: 'debug_chaindbCompact',
		}),
		new web3._extend.Method({
			name: 'verbosity',
			call: 'debug_verbosity',
			params: 1
		}),
		new web3._extend.Method({
			name: 'vmodule',
			call: 'debug_vmodule',
			params: 1
		}),
		new web3._extend.Method({
			name: 'backtraceAt',
			call: 'debug_backtraceAt',
			params: 1,
		}),
		new web3._extend.Method({
			name: 'stacks',
			call: 'debug_stacks',
			params: 0,
			outputFormatter: console.log
		}),
		new web3._extend.Method({
			name: 'freeOSMemory',
			call: 'debug_freeOSMemory',
			params: 0,
		}),
		new web3._extend.Method({
			name: 'setGCPercent',
			call: 'debug_setGCPercent',
			params: 1,
		}),
		new web3._extend.Method({
			name: 'memStats',
			call: 'debug_memStats',
			params: 0,
		}),
		new web3._extend.Method({
			name: 'gcStats',
			call: 'debug_gcStats',
			params: 0,
		}),
		new web3._extend.Method({
			name: 'cpuProfile',
			call: 'debug_cpuProfile',
			params: 2
		}),
		new web3._extend.Method({
			name: 'startCPUProfile',
			call: 'debug_startCPUProfile',
			params: 1
		}),
		new web3._extend.Method({
			name: 'stopCPUProfile',
			call: 'debug_stopCPUProfile',
			params: 0
		}),
		new web3._extend.Method({
			name: 'goTrace',
			call: 'debug_goTrace',
			params: 2
		}),
		new web3._extend.Method({
			name: 'startGoTrace',
			call: 'debug_startGoTrace',
			params: 1
		}),
		new web3._extend.Method({
			name: 'stopGoTrace',
			call: 'debug_stopGoTrace',
			params: 0
		}),
		new web3._extend.Method({
			name: 'blockProfile',
			call: 'debug_blockProfile',
			params: 2
		}),
		new web3._extend.Method({
			name: 'setBlockProfileRate',
			call: 'debug_setBlockProfileRate',
			params: 1
		}),
		new web3._extend.Method({
			name: 'writeBlockProfile',
			call: 'debug_writeBlockProfile',
			params: 1
		}),
		new web3._extend.Method({
			name: 'mutexProfile',
			call: 'debug_mutexProfile',
			params: 2
		}),
		new web3._extend.Method({
			name: 'setMutexProfileFraction',
			call: 'debug_setMutexProfileFraction',
			params: 1
		}),
		new web3._extend.Method({
			name: 'writeMutexProfile',
			call: 'debug_writeMutexProfile',
			params: 1
		}),
		new web3._extend.Method({
			name: 'writeMemProfile',
			call: 'debug_writeMemProfile',
			params: 1
		}),
		new web3._extend.Method({
			name: 'traceBlock',
			call: 'debug_traceBlock',
			params: 2,
			inputFormatter: [null, null]
		}),
		new web3._extend.Method({
			name: 'traceBlockFromFile',
			call: 'debug_traceBlockFromFile',
			params: 2,
			inputFormatter: [null, null]
		}),
		new web3._extend.Method({
			name: 'traceBadBlock',
			call: 'debug_traceBadBlock',
			params: 1,
			inputFormatter: [null]
		}),
		new web3._extend.Method({
			name: 'standardTraceBadBlockToFile',
			call: 'debug_standardTraceBadBlockToFile',
			params: 2,
			inputFormatter: [null, null]
		}),
		new web3._extend.Method({
			name: 'standardTraceBlockToFile',
			call: 'debug_standardTraceBlockToFile',
			params: 2,
			inputFormatter: [null, null]
		}),
		new web3._extend.Method({
			name: 'traceBlockByNumber',
			call: 'debug_traceBlockByNumber',
			params: 2,
			inputFormatter: [web3._extend.formatters.inputBlockNumberFormatter, null]
		}),
		new web3._extend.Method({
			name: 'traceBlockByHash',
			call: 'debug_traceBlockByHash',
			params: 2,
			inputFormatter: [null, null]
		}),
		new web3._extend.Method({
			name: 'traceTransaction',
			call: 'debug_traceTransaction',
			params: 2,
			inputFormatter: [null, null]
		}),
		new web3._extend.Method({
			name: 'preimage',
			call: 'debug_preimage',
			params: 1,
			inputFormatter: [null]
		}),
		new web3._extend.Method({
			name: 'getBadBlocks',
			call: 'debug_getBadBlocks',
			params: 0,
		}),
		new web3._extend.Method({
			name: 'storageRangeAt',
			call: 'debug_storageRangeAt',
			params: 5,
		}),
		new web3._extend.Method({
			name: 'getModifiedAccountsByNumber',
			call: 'debug_getModifiedAccountsByNumber',
			params: 2,
			inputFormatter: [null, null],
		}),
		new web3._extend.Method({
			name: 'getModifiedAccountsByHash',
			call: 'debug_getModifiedAccountsByHash',
			params: 2,
			inputFormatter:[null, null],
		}),
	],
	properties: []
});
`

const Eth_JS = `
web3._extend({
	property: 'eth',
	methods: [
		new web3._extend.Method({
			name: 'chainId',
			call: 'eth_chainId',
			params: 0
		}),
		new web3._extend.Method({
			name: 'sign',
			call: 'eth_sign',
			params: 2,
			inputFormatter: [web3._extend.formatters.inputAddressFormatter, null]
		}),
		new web3._extend.Method({
			name: 'resend',
			call: 'eth_resend',
			params: 3,
			inputFormatter: [web3._extend.formatters.inputTransactionFormatter, web3._extend.utils.fromDecimal, web3._extend.utils.fromDecimal]
		}),
		new web3._extend.Method({
			name: 'signTransaction',
			call: 'eth_signTransaction',
			params: 1,
			inputFormatter: [web3._extend.formatters.inputTransactionFormatter]
		}),
		new web3._extend.Method({
			name: 'submitTransaction',
			call: 'eth_submitTransaction',
			params: 1,
			inputFormatter: [web3._extend.formatters.inputTransactionFormatter]
		}),
		new web3._extend.Method({
			name: 'getRawTransaction',
			call: 'eth_getRawTransactionByHash',
			params: 1
		}),
		new web3._extend.Method({
			name: 'getRawTransactionFromBlock',
			call: function(args) {
				return (web3._extend.utils.isString(args[0]) && args[0].indexOf('0x') === 0) ? 'eth_getRawTransactionByBlockHashAndIndex' : 'eth_getRawTransactionByBlockNumberAndIndex';
			},
			params: 2,
			inputFormatter: [web3._extend.formatters.inputBlockNumberFormatter, web3._extend.utils.toHex]
		}),
	],
	properties: [
		new web3._extend.Property({
			name: 'pendingTransactions',
			getter: 'eth_pendingTransactions',
			outputFormatter: function(txs) {
				var formatted = [];
				for (var i = 0; i < txs.length; i++) {
					formatted.push(web3._extend.formatters.outputTransactionFormatter(txs[i]));
					formatted[i].blockHash = null;
				}
				return formatted;
			}
		}),
	]
});
`

const Miner_JS = `
web3._extend({
	property: 'miner',
	methods: [
		new web3._extend.Method({
			name: 'start',
			call: 'miner_start',
			params: 1,
			inputFormatter: [null]
		}),
		new web3._extend.Method({
			name: 'stop',
			call: 'miner_stop'
		}),
		new web3._extend.Method({
			name: 'setEtherbase',
			call: 'miner_setEtherbase',
			params: 1,
			inputFormatter: [web3._extend.formatters.inputAddressFormatter]
		}),
		new web3._extend.Method({
			name: 'setExtra',
			call: 'miner_setExtra',
			params: 1
		}),
		new web3._extend.Method({
			name: 'setGasPrice',
			call: 'miner_setGasPrice',
			params: 1,
			inputFormatter: [web3._extend.utils.fromDecimal]
		}),
		new web3._extend.Method({
			name: 'setRecommitInterval',
			call: 'miner_setRecommitInterval',
			params: 1,
		}),
		new web3._extend.Method({
			name: 'getHashrate',
			call: 'miner_getHashrate'
		}),
		new web3._extend.Method({
			name: 'startAutoBuyTicket',
			call: 'miner_startAutoBuyTicket',
			params: 0
		}),
		new web3._extend.Method({
			name: 'stopAutoBuyTicket',
			call: 'miner_stopAutoBuyTicket',
			params: 0
		}),
	],
	properties: []
});
`

const Net_JS = `
web3._extend({
	property: 'net',
	methods: [],
	properties: [
		new web3._extend.Property({
			name: 'version',
			getter: 'net_version'
		}),
	]
});
`

const Personal_JS = `
web3._extend({
	property: 'personal',
	methods: [
		new web3._extend.Method({
			name: 'importRawKey',
			call: 'personal_importRawKey',
			params: 2
		}),
		new web3._extend.Method({
			name: 'sign',
			call: 'personal_sign',
			params: 3,
			inputFormatter: [null, web3._extend.formatters.inputAddressFormatter, null]
		}),
		new web3._extend.Method({
			name: 'ecRecover',
			call: 'personal_ecRecover',
			params: 2
		}),
		new web3._extend.Method({
			name: 'openWallet',
			call: 'personal_openWallet',
			params: 2
		}),
		new web3._extend.Method({
			name: 'deriveAccount',
			call: 'personal_deriveAccount',
			params: 3
		}),
		new web3._extend.Method({
			name: 'signTransaction',
			call: 'personal_signTransaction',
			params: 2,
			inputFormatter: [web3._extend.formatters.inputTransactionFormatter, null]
		}),
	],
	properties: [
		new web3._extend.Property({
			name: 'listWallets',
			getter: 'personal_listWallets'
		}),
	]
})
`

const RPC_JS = `
web3._extend({
	property: 'rpc',
	methods: [],
	properties: [
		new web3._extend.Property({
			name: 'modules',
			getter: 'rpc_modules'
		}),
	]
});
`

const TxPool_JS = `
web3._extend({
	property: 'txpool',
	methods: [
		new web3._extend.Method({
			name: 'addBlacklist',
			call: 'txpool_addBlacklist',
			params: 1,
			inputFormatter: [web3._extend.formatters.inputAddressFormatter]
		}),
		new web3._extend.Method({
			name: 'removeBlacklist',
			call: 'txpool_removeBlacklist',
			params: 1,
			inputFormatter: [web3._extend.formatters.inputAddressFormatter]
		}),
	],
	properties:
	[
		new web3._extend.Property({
			name: 'content',
			getter: 'txpool_content'
		}),
		new web3._extend.Property({
			name: 'inspect',
			getter: 'txpool_inspect'
		}),
		new web3._extend.Property({
			name: 'status',
			getter: 'txpool_status',
			outputFormatter: function(status) {
				status.pending = web3._extend.utils.toDecimal(status.pending);
				status.queued = web3._extend.utils.toDecimal(status.queued);
				return status;
			}
		}),
		new web3._extend.Property({
			name: 'blacklist',
			getter: 'txpool_getBlacklist'
		}),
	]
});
`

// FsnJS wacom
const FsnJS = `
web3._extend({
	property: 'fsn',
	methods: [
		new web3._extend.Method({
			name: 'getSnapshot',
			call: 'fsn_getSnapshot',
			params: 1,
			inputFormatter: [
				web3._extend.formatters.inputDefaultBlockNumberFormatter
			]
		}),
		new web3._extend.Method({
			name: 'getSnapshotAtHash',
			call: 'fsn_getSnapshotAtHash',
			params: 1
		}),
		new web3._extend.Method({
			name: 'getBlockReward',
			call: 'fsn_getBlockReward',
			params: 1,
			inputFormatter: [
				web3._extend.formatters.inputDefaultBlockNumberFormatter
			]
		}),
		new web3._extend.Method({
			name: 'getBalance',
			call: 'fsn_getBalance',
			params: 3,
			inputFormatter: [
				null,
				web3._extend.formatters.inputAddressFormatter,
				web3._extend.formatters.inputDefaultBlockNumberFormatter
			]
		}),
		new web3._extend.Method({
			name: 'getAllBalances',
			call: 'fsn_getAllBalances',
			params: 2,
			inputFormatter: [
				web3._extend.formatters.inputAddressFormatter,
				web3._extend.formatters.inputDefaultBlockNumberFormatter
			]
		}),
		new web3._extend.Method({
			name: 'getTimeLockBalance',
			call: 'fsn_getTimeLockBalance',
			params: 3,
			inputFormatter: [
				web3._extend.formatters.inputTransactionFormatter,
				web3._extend.formatters.inputAddressFormatter,
				web3._extend.formatters.inputDefaultBlockNumberFormatter
			]
		}),
		new web3._extend.Method({
			name: 'getTimeLockValueByInterval',
			call: 'fsn_getTimeLockValueByInterval',
			params: 5,
			inputFormatter: [
				null,
				web3._extend.formatters.inputAddressFormatter,
				null,
				null,
				web3._extend.formatters.inputDefaultBlockNumberFormatter
			]
		}),
		new web3._extend.Method({
			name: 'getAllTimeLockBalances',
			call: 'fsn_getAllTimeLockBalances',
			params: 2,
			inputFormatter: [
				web3._extend.formatters.inputAddressFormatter,
				web3._extend.formatters.inputDefaultBlockNumberFormatter
			]
		}),
		new web3._extend.Method({
			name: 'getRawTimeLockBalance',
			call: 'fsn_getRawTimeLockBalance',
			params: 3,
			inputFormatter: [
				null,
				web3._extend.formatters.inputAddressFormatter,
				web3._extend.formatters.inputDefaultBlockNumberFormatter
			]
		}),
		new web3._extend.Method({
			name: 'getAllRawTimeLockBalances',
			call: 'fsn_getAllRawTimeLockBalances',
			params: 2,
			inputFormatter: [
				web3._extend.formatters.inputAddressFormatter,
				web3._extend.formatters.inputDefaultBlockNumberFormatter
			]
		}),
		new web3._extend.Method({
			name: 'getNotation',
			call: 'fsn_getNotation',
			params: 2,
			inputFormatter: [
				web3._extend.formatters.inputAddressFormatter,
				web3._extend.formatters.inputDefaultBlockNumberFormatter
			]
		}),
		new web3._extend.Method({
			name: 'getAddressByNotation',
			call: 'fsn_getAddressByNotation',
			params: 2,
			inputFormatter: [
				null,
				web3._extend.formatters.inputDefaultBlockNumberFormatter
			]
		}),
		new web3._extend.Method({
			name: 'allNotation',
			call: 'fsn_allNotation',
			params: 1,
			inputFormatter: [
				web3._extend.formatters.inputDefaultBlockNumberFormatter
			]
		}),
		new web3._extend.Method({
			name: 'genNotation',
			call: 'fsn_genNotation',
			params: 2,
			inputFormatter: [
				web3._extend.formatters.inputTransactionFormatter,
				null
			]
		}),
		new web3._extend.Method({
			name: 'genAsset',
			call: 'fsn_genAsset',
			params: 2,
			inputFormatter: [
				function(options){
					if(options.name === undefined || !options.name){
						throw new Error('invalid name');
					}
					if(options.symbol === undefined || !options.symbol){
						throw new Error('invalid symbol');
					}
					if(options.decimals === undefined || options.decimals <= 0 || options.decimals > 255){
						throw new Error('invalid decimals');
					}
					if(options.total !== undefined){
						options.total = web3.fromDecimal(options.total)
					}
					return web3._extend.formatters.inputTransactionFormatter(options)
				},
				null
			]
		}),
		new web3._extend.Method({
			name: 'allAssets',
			call: 'fsn_allAssets',
			params: 1,
			inputFormatter: [
				web3._extend.formatters.inputDefaultBlockNumberFormatter
			]
		}),
		new web3._extend.Method({
			name: 'getAsset',
			call: 'fsn_getAsset',
			params: 2,
			inputFormatter: [
				null,
				web3._extend.formatters.inputDefaultBlockNumberFormatter
			]
		}),
		new web3._extend.Method({
			name: 'sendAsset',
			call: 'fsn_sendAsset',
			params: 2,
			inputFormatter: [
				web3._extend.formatters.inputTransactionFormatter,
				null
			]
		}),
		new web3._extend.Method({
			name: 'assetToTimeLock',
			call: 'fsn_assetToTimeLock',
			params: 2,
			inputFormatter: [
				function(options){
					return web3._extend.formatters.inputTransactionFormatter(options)
				},
				null
			]
		}),
		new web3._extend.Method({
			name: 'timeLockToTimeLock',
			call: 'fsn_timeLockToTimeLock',
			params: 2,
			inputFormatter: [
				function(options){
					return web3._extend.formatters.inputTransactionFormatter(options)
				},
				null
			]
		}),
		new web3._extend.Method({
			name: 'timeLockToAsset',
			call: 'fsn_timeLockToAsset',
			params: 2,
			inputFormatter: [
				function(options){
					return web3._extend.formatters.inputTransactionFormatter(options)
				},
				null
			]
		}),
		new web3._extend.Method({
			name: 'sendTimeLock',
			call: 'fsn_sendTimeLock',
			params: 2,
			inputFormatter: [
				web3._extend.formatters.inputTransactionFormatter,
				null
			]
		}),
		new web3._extend.Method({
			name: 'allTickets',
			call: 'fsn_allTickets',
			params: 1,
			inputFormatter: [
				web3._extend.formatters.inputDefaultBlockNumberFormatter
			]
		}),
		new web3._extend.Method({
			name: 'allTicketsByAddress',
			call: 'fsn_allTicketsByAddress',
			params: 2,
			inputFormatter: [
				web3._extend.formatters.inputAddressFormatter,
				web3._extend.formatters.inputDefaultBlockNumberFormatter
			]
		}),
		new web3._extend.Method({
			name: 'allInfoByAddress',
			call: 'fsn_allInfoByAddress',
			params: 2,
			inputFormatter: [
				web3._extend.formatters.inputAddressFormatter,
				web3._extend.formatters.inputDefaultBlockNumberFormatter
			]
		}),
		new web3._extend.Method({
			name: 'getTransactionAndReceipt',
			call: 'fsn_getTransactionAndReceipt',
			params: 1,
		}),
		new web3._extend.Method({
			name: 'allAssetsByAddress',
			call: 'fsn_allAssetsByAddress',
			params: 2,
			inputFormatter: [
				web3._extend.formatters.inputAddressFormatter,
				web3._extend.formatters.inputDefaultBlockNumberFormatter
			]
		}),
		new web3._extend.Method({
			name: 'assetExistForAddress',
			call: 'fsn_assetExistForAddress',
			params: 3,
			inputFormatter: [
				null,
				web3._extend.formatters.inputAddressFormatter,
				web3._extend.formatters.inputDefaultBlockNumberFormatter
			]
		}),
		new web3._extend.Method({
			name: 'totalNumberOfTickets',
			call: 'fsn_totalNumberOfTickets',
			params: 1,
			inputFormatter: [
				web3._extend.formatters.inputDefaultBlockNumberFormatter
			]
		}),
		new web3._extend.Method({
			name: 'totalNumberOfTicketsByAddress',
			call: 'fsn_totalNumberOfTicketsByAddress',
			params: 2,
			inputFormatter: [
				web3._extend.formatters.inputAddressFormatter,
				web3._extend.formatters.inputDefaultBlockNumberFormatter
			]
		}),
		new web3._extend.Method({
			name: 'ticketPrice',
			call: 'fsn_ticketPrice',
			params: 1,
			inputFormatter: [
				web3._extend.formatters.inputDefaultBlockNumberFormatter
			]
		}),
		new web3._extend.Method({
			name: 'buyTicket',
			call: 'fsn_buyTicket',
			params: 2,
			inputFormatter: [
				web3._extend.formatters.inputTransactionFormatter,
				null
			]
		}),
		new web3._extend.Method({
			name: 'incAsset',
			call: 'fsn_incAsset',
			params: 2,
			inputFormatter: [
				web3._extend.formatters.inputTransactionFormatter,
				null
			]
		}),
		new web3._extend.Method({
			name: 'decAsset',
			call: 'fsn_decAsset',
			params: 2,
			inputFormatter: [
				web3._extend.formatters.inputTransactionFormatter,
				null
			]
		}),
		new web3._extend.Method({
			name: 'allSwaps',
			call: 'fsn_allSwaps',
			params: 1,
			inputFormatter: [
				web3._extend.formatters.inputDefaultBlockNumberFormatter
			]
		}),
		new web3._extend.Method({
			name: 'allSwapsByAddress',
			call: 'fsn_allSwapsByAddress',
			params: 2,
			inputFormatter: [
				web3._extend.formatters.inputAddressFormatter,
				web3._extend.formatters.inputDefaultBlockNumberFormatter
			]
		}),
		new web3._extend.Method({
			name: 'makeSwap',
			call: 'fsn_makeSwap',
			params: 2,
			inputFormatter: [
				web3._extend.formatters.inputTransactionFormatter,
				null
			]
		}),
		new web3._extend.Method({
			name: 'makeMultiSwap',
			call: 'fsn_makeMultiSwap',
			params: 2,
			inputFormatter: [
				web3._extend.formatters.inputTransactionFormatter,
				null
			]
		}),
		new web3._extend.Method({
			name: 'recallMultiSwap',
			call: 'fsn_recallMultiSwap',
			params: 2,
			inputFormatter: [
				web3._extend.formatters.inputTransactionFormatter,
				null
			]
		}),
		new web3._extend.Method({
			name: 'takeMultiSwap',
			call: 'fsn_takeMultiSwap',
			params: 2,
			inputFormatter: [
				web3._extend.formatters.inputTransactionFormatter,
				null
			]
		}),
		new web3._extend.Method({
			name: 'recallSwap',
			call: 'fsn_recallSwap',
			params: 2,
			inputFormatter: [
				web3._extend.formatters.inputTransactionFormatter,
				null
			]
		}),
		new web3._extend.Method({
			name: 'takeSwap',
			call: 'fsn_takeSwap',
			params: 2,
			inputFormatter: [
				web3._extend.formatters.inputTransactionFormatter,
				null
			]
		}),
		new web3._extend.Method({
			name: 'getSwap',
			call: 'fsn_getSwap',
			params: 2,
			inputFormatter: [
				null,
				web3._extend.formatters.inputDefaultBlockNumberFormatter
			]
		}),
		new web3._extend.Method({
			name: 'getMultiSwap',
			call: 'fsn_getMultiSwap',
			params: 2,
			inputFormatter: [
				null,
				web3._extend.formatters.inputDefaultBlockNumberFormatter
			]
		}),
		new web3._extend.Method({
			name: 'getStakeInfo',
			call: 'fsn_getStakeInfo',
			params: 1,
			inputFormatter: [
				web3._extend.formatters.inputDefaultBlockNumberFormatter
			]
		}),
		new web3._extend.Method({
			name: 'isAutoBuyTicket',
			call: 'fsn_isAutoBuyTicket',
			params: 0
		}),
		new web3._extend.Method({
			name: 'getLatestNotation',
			call: 'fsn_getLatestNotation',
			params: 1,
			inputFormatter: [
				web3._extend.formatters.inputDefaultBlockNumberFormatter
			]
		}),
		new web3._extend.Method({
			name: 'getRetreatTickets',
			call: 'fsn_getRetreatTickets',
			params: 1,
			inputFormatter: [
				web3._extend.formatters.inputDefaultBlockNumberFormatter
			]
		}),
	],
	properties:[
		new web3._extend.Property({
			name: 'coinbase',
			getter: 'eth_coinbase'
		}),
	]
});
`

// FsnBT wacome
const FsnBT = `
web3._extend({
	property: 'fsnbt',
	methods: [
		new web3._extend.Method({
			name: 'getBalance',
			call: 'fsn_getBalance',
			params: 3,
			inputFormatter: [
				null,
				web3._extend.formatters.inputAddressFormatter,
				web3._extend.formatters.inputDefaultBlockNumberFormatter
			]
		}),
		new web3._extend.Method({
			name: 'getAllBalances',
			call: 'fsn_getAllBalances',
			params: 2,
			inputFormatter: [
				web3._extend.formatters.inputAddressFormatter,
				web3._extend.formatters.inputDefaultBlockNumberFormatter
			]
		}),
		new web3._extend.Method({
			name: 'getTimeLockBalance',
			call: 'fsn_getTimeLockBalance',
			params: 3,
			inputFormatter: [
				null,
				web3._extend.formatters.inputAddressFormatter,
				web3._extend.formatters.inputDefaultBlockNumberFormatter
			]
		}),
		new web3._extend.Method({
			name: 'getAllTimeLockBalances',
			call: 'fsn_getAllTimeLockBalances',
			params: 2,
			inputFormatter: [
				web3._extend.formatters.inputAddressFormatter,
				web3._extend.formatters.inputDefaultBlockNumberFormatter
			]
		}),
		new web3._extend.Method({
			name: 'getNotation',
			call: 'fsn_getNotation',
			params: 2,
			inputFormatter: [
				web3._extend.formatters.inputAddressFormatter,
				web3._extend.formatters.inputDefaultBlockNumberFormatter
			]
		}),
		new web3._extend.Method({
			name: 'getAddressByNotation',
			call: 'fsn_getAddressByNotation',
			params: 2,
			inputFormatter: [
				null,
				web3._extend.formatters.inputDefaultBlockNumberFormatter
			]
		}),
		new web3._extend.Method({
			name: 'allTickets',
			call: 'fsn_allTickets',
			params: 1,
			inputFormatter: [
				web3._extend.formatters.inputDefaultBlockNumberFormatter
			]
		}),
		new web3._extend.Method({
			name: 'allTicketsByAddress',
			call: 'fsn_allTicketsByAddress',
			params: 2,
			inputFormatter: [
				web3._extend.formatters.inputAddressFormatter,
				web3._extend.formatters.inputDefaultBlockNumberFormatter
			]
		}),
		new web3._extend.Method({
			name: 'allInfoByAddress',
			call: 'fsn_allInfoByAddress',
			params: 2,
			inputFormatter: [
				web3._extend.formatters.inputAddressFormatter,
				web3._extend.formatters.inputDefaultBlockNumberFormatter
			]
		}),
		new web3._extend.Method({
			name: 'getTransactionAndReceipt',
			call: 'fsn_getTransactionAndReceipt',
			params: 1,
		}),
		new web3._extend.Method({
			name: 'totalNumberOfTickets',
			call: 'fsn_totalNumberOfTickets',
			params: 1,
			inputFormatter: [
				web3._extend.formatters.inputDefaultBlockNumberFormatter
			]
		}),
		new web3._extend.Method({
			name: 'totalNumberOfTicketsByAddress',
			call: 'fsn_totalNumberOfTicketsByAddress',
			params: 2,
			inputFormatter: [
				web3._extend.formatters.inputAddressFormatter,
				web3._extend.formatters.inputDefaultBlockNumberFormatter
			]
		}),
		new web3._extend.Method({
			name: 'ticketPrice',
			call: 'fsn_ticketPrice',
			params: 1,
			inputFormatter: [
				web3._extend.formatters.inputDefaultBlockNumberFormatter
			]
		}),
		new web3._extend.Method({
			name: 'buyTicket',
			call: 'fsn_buyTicket',
			params: 2,
			inputFormatter: [
				web3._extend.formatters.inputTransactionFormatter,
				null
			]
		}),
		new web3._extend.Method({
			name: 'isAutoBuyTicket',
			call: 'fsn_isAutoBuyTicket',
			params: 0
		}),
		]
	});
`

// FsnTxJS wacom
const FsnTxJS = `
web3._extend({
	property: 'fsntx',
	methods: [
		new web3._extend.Method({
			name: 'sendRawTransaction',
			call: 'fsntx_sendRawTransaction',
			params: 1
		}),
		new web3._extend.Method({
			name: 'buildGenNotationTx',
			call: 'fsntx_buildGenNotationTx',
			params: 1,
			inputFormatter: [
				web3._extend.formatters.inputTransactionFormatter
			]
		}),
		new web3._extend.Method({
			name: 'genNotation',
			call: 'fsntx_genNotation',
			params: 1,
			inputFormatter: [
				web3._extend.formatters.inputTransactionFormatter,
			]
		}),
		new web3._extend.Method({
			name: 'buildGenAssetTx',
			call: 'fsntx_buildGenAssetTx',
			params: 1,
			inputFormatter: [
				function(options){
					if(options.name === undefined || !options.name){
						throw new Error('invalid name');
					}
					if(options.symbol === undefined || !options.symbol){
						throw new Error('invalid symbol');
					}
					if(options.decimals === undefined || options.decimals <= 0 || options.decimals > 255){
						throw new Error('invalid decimals');
					}
					if(options.total !== undefined){
						options.total = web3.fromDecimal(options.total)
					}
					return web3._extend.formatters.inputTransactionFormatter(options)
				}
			]
		}),
		new web3._extend.Method({
			name: 'genAsset',
			call: 'fsntx_genAsset',
			params: 1,
			inputFormatter: [
				function(options){
					if(options.name === undefined || !options.name){
						throw new Error('invalid name');
					}
					if(options.symbol === undefined || !options.symbol){
						throw new Error('invalid symbol');
					}
					if(options.decimals === undefined || options.decimals <= 0 || options.decimals > 255){
						throw new Error('invalid decimals');
					}
					if(options.total !== undefined){
						options.total = web3.fromDecimal(options.total)
					}
					return web3._extend.formatters.inputTransactionFormatter(options)
				}
			]
		}),
		new web3._extend.Method({
			name: 'buildSendAssetTx',
			call: 'fsntx_buildSendAssetTx',
			params: 1,
			inputFormatter: [
				web3._extend.formatters.inputTransactionFormatter
			]
		}),
		new web3._extend.Method({
			name: 'sendAsset',
			call: 'fsntx_sendAsset',
			params: 1,
			inputFormatter: [
				web3._extend.formatters.inputTransactionFormatter
			]
		}),
		new web3._extend.Method({
			name: 'buildAssetToTimeLockTx',
			call: 'fsntx_buildAssetToTimeLockTx',
			params: 1,
			inputFormatter: [
				function(options){
					return web3._extend.formatters.inputTransactionFormatter(options)
				}
			]
		}),
		new web3._extend.Method({
			name: 'assetToTimeLock',
			call: 'fsntx_assetToTimeLock',
			params: 1,
			inputFormatter: [
				function(options){
					return web3._extend.formatters.inputTransactionFormatter(options)
				}
			]
		}),
		new web3._extend.Method({
			name: 'buildTimeLockToTimeLockTx',
			call: 'fsntx_buildTimeLockToTimeLockTx',
			params: 1,
			inputFormatter: [
				function(options){
					return web3._extend.formatters.inputTransactionFormatter(options)
				}
			]
		}),
		new web3._extend.Method({
			name: 'timeLockToTimeLock',
			call: 'fsntx_timeLockToTimeLock',
			params: 1,
			inputFormatter: [
				function(options){
					return web3._extend.formatters.inputTransactionFormatter(options)
				}
			]
		}),
		new web3._extend.Method({
			name: 'buildTimeLockToAssetTx',
			call: 'fsntx_buildTimeLockToAssetTx',
			params: 1,
			inputFormatter: [
				function(options){
					return web3._extend.formatters.inputTransactionFormatter(options)
				}
			]
		}),
		new web3._extend.Method({
			name: 'timeLockToAsset',
			call: 'fsntx_timeLockToAsset',
			params: 1,
			inputFormatter: [
				function(options){
					return web3._extend.formatters.inputTransactionFormatter(options)
				}
			]
		}),
		new web3._extend.Method({
			name: 'buildSendTimeLockTx',
			call: 'fsntx_buildSendTimeLockTx',
			params: 1,
			inputFormatter: [
				web3._extend.formatters.inputTransactionFormatter
			]
		}),
		new web3._extend.Method({
			name: 'sendTimeLock',
			call: 'fsntx_sendTimeLock',
			params: 1,
			inputFormatter: [
				web3._extend.formatters.inputTransactionFormatter
			]
		}),
		new web3._extend.Method({
			name: 'buildBuyTicketTx',
			call: 'fsntx_buildBuyTicketTx',
			params: 1,
			inputFormatter: [
				web3._extend.formatters.inputTransactionFormatter
			]
		}),
		new web3._extend.Method({
			name: 'buyTicket',
			call: 'fsntx_buyTicket',
			params: 1,
			inputFormatter: [
				web3._extend.formatters.inputTransactionFormatter
			]
		}),
		new web3._extend.Method({
			name: 'buildIncAssetTx',
			call: 'fsntx_buildIncAssetTx',
			params: 1,
			inputFormatter: [
				web3._extend.formatters.inputTransactionFormatter
			]
		}),
		new web3._extend.Method({
			name: 'incAsset',
			call: 'fsntx_incAsset',
			params: 1,
			inputFormatter: [
				web3._extend.formatters.inputTransactionFormatter
			]
		}),
		new web3._extend.Method({
			name: 'buildDecAssetTx',
			call: 'fsntx_buildDecAssetTx',
			params: 1,
			inputFormatter: [
				web3._extend.formatters.inputTransactionFormatter
			]
		}),
		new web3._extend.Method({
			name: 'decAsset',
			call: 'fsntx_decAsset',
			params: 1,
			inputFormatter: [
				web3._extend.formatters.inputTransactionFormatter
			]
		}),
		new web3._extend.Method({
			name: 'buildMakeSwapTx',
			call: 'fsntx_buildMakeSwapTx',
			params: 1,
			inputFormatter: [
				web3._extend.formatters.inputTransactionFormatter
			]
		}),
		new web3._extend.Method({
			name: 'makeSwap',
			call: 'fsntx_makeSwap',
			params: 1,
			inputFormatter: [
				web3._extend.formatters.inputTransactionFormatter
			]
		}),
		new web3._extend.Method({
			name: 'buildRecallSwapTx',
			call: 'fsntx_buildRecallSwapTx',
			params: 1,
			inputFormatter: [
				web3._extend.formatters.inputTransactionFormatter
			]
		}),
		new web3._extend.Method({
			name: 'recallSwap',
			call: 'fsntx_recallSwap',
			params: 1,
			inputFormatter: [
				web3._extend.formatters.inputTransactionFormatter
			]
		}),
		new web3._extend.Method({
			name: 'buildTakeSwapTx',
			call: 'fsntx_buildTakeSwapTx',
			params: 1,
			inputFormatter: [
				web3._extend.formatters.inputTransactionFormatter
			]
		}),
		new web3._extend.Method({
			name: 'takeSwap',
			call: 'fsntx_takeSwap',
			params: 1,
			inputFormatter: [
				web3._extend.formatters.inputTransactionFormatter
			]
		}),
		new web3._extend.Method({
			name: 'buildMakeMultiSwapTx',
			call: 'fsntx_buildMakeMultiSwapTx',
			params: 1,
			inputFormatter: [
				web3._extend.formatters.inputTransactionFormatter
			]
		}),
		new web3._extend.Method({
			name: 'makeMultiSwap',
			call: 'fsntx_makeMultiSwap',
			params: 1,
			inputFormatter: [
				web3._extend.formatters.inputTransactionFormatter
			]
		}),
		new web3._extend.Method({
			name: 'buildRecallMultiSwapTx',
			call: 'fsntx_buildRecallMultiSwapTx',
			params: 1,
			inputFormatter: [
				web3._extend.formatters.inputTransactionFormatter
			]
		}),
		new web3._extend.Method({
			name: 'recallMultiSwap',
			call: 'fsntx_recallMultiSwap',
			params: 1,
			inputFormatter: [
				web3._extend.formatters.inputTransactionFormatter
			]
		}),
		new web3._extend.Method({
			name: 'buildTakeMultiSwapTx',
			call: 'fsntx_buildTakeMultiSwapTx',
			params: 1,
			inputFormatter: [
				web3._extend.formatters.inputTransactionFormatter
			]
		}),
		new web3._extend.Method({
			name: 'takeMultiSwap',
			call: 'fsntx_takeMultiSwap',
			params: 1,
			inputFormatter: [
				web3._extend.formatters.inputTransactionFormatter
			]
		}),
	]
});
`
