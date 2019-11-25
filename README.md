<div align="center"><img src ="https://i.imgur.com/lixyKZe.png" height="100px" /></div>

## Go eFSN

FUSION would like to extend its gratitude to the Ethereum Foundation. FUSION has used the official open-source golang implementation of the Ethereum protocol.

# Automatic node setup 

The fastest way to get a node up and running and to start staking automatically is by using the FUSION Node Manager script.  
Just execute the following command on Ubuntu 18.04 (or newer), press 1 and answer the questions:

``bash -c "$(curl -fsSL https://raw.githubusercontent.com/FUSIONFoundation/efsn/master/QuickNodeSetup/fsnNode.sh)"``

The Node Manager script and an example video can also be found under this link: [Quick Setup](https://github.com/FUSIONFoundation/efsn/tree/master/QuickNodeSetup)  
The video shows how to quickly setup a staking node.

# Manual node setup

## How to run a Miner

Change the parameter `YOURDIRECTORY` to your local directory

Install Docker first, e.g. on Ubuntu do `sudo apt-get install docker.io`

### Pull Miner image from repository

`docker pull fusionnetwork/efsn:latest`

### Run a Miner from the image

1. With ticket auto-buy disabled

`docker run -it -p 40408:40408 -v YOURDIRECTORY:/fusion-node fusionnetwork/efsn -u <account to unlock> -e MyFusionMiner`

2. With ticket auto-buy enabled

`docker run -it -p 40408:40408 -v YOURDIRECTORY:/fusion-node fusionnetwork/efsn -u <account to unlock> -e MyFusionMiner -a`

### Build your own Miner image (optional)

`docker build --file Dockerfile -t YOUR-DOCKER-HUB-ID/efsn .`

### Run a Miner using your image

`docker run -it -p 40408:40408 -v YOURDIRECTORY:/fusion-node fusionnetwork/efsn -u <account to unlock> -e MyFusionMiner -a`

Remember to:

1. Replace `YOUR-DOCKER-HUB-ID` with your valid Docker Hub id.

2. Save the keystore file as `YOURDIRECTORY/UTC...`

3. Save the password.txt as `YOURDIRECTORY/password.txt`

4. (Optional) Add flag "-a" or "--autobt" to enable ticket auto-buy.

5. (Optional) Add flag "-tn" or "--testnet" to connect to the public testnet.

`Note: The password file must be named password.txt and the keystore file name must start with UTC...`

## How to run a Gateway

Change the parameter `YOURDIRECTORY` to your local directory

Install Docker first, e.g. on Ubuntu do `sudo apt-get install docker.io`

### Pull Gateway image from repository

`docker pull fusionnetwork/gateway:latest`

### Run a Gateway from the image

1. Connect to mainnet

`docker run -it -p 9000:9000 -p 9001:9001 -p 40408:40408 -v YOURDIRECTORY:/fusion-node fusionnetwork/gateway`

2. Connect to testnet

`docker run -it -p 9000:9000 -p 9001:9001 -p 40408:40408 -v YOURDIRECTORY:/fusion-node fusionnetwork/gateway -tn`

### Build your own Gateway image (optional)

`docker build --file Dockerfile.gtw -t YOUR-DOCKER-HUB-ID/gateway .`

### Run a Gateway using your image

`docker run -it -p 9000:9000 -p 9001:9001 -p 40408:40408 -v YOURDIRECTORY:/fusion-node YOUR-DOCKER-HUB-ID/gateway`

Remember to replace `YOUR-DOCKER-HUB-ID` with your valid Docker Hub id.

You can now connect to the websocket API via `ws://localhost:9001`

Note that this creates a public gateway, unless the system is protected by an external firewall. Additional configuration steps should be taken to ensure the security and integrity of the API communication, like setting up encryption (e.g. via an nginx proxy).
To run a purely local gateway for testing, use:

`docker run -it -p 127.0.0.1:9000:9000 -p 127.0.0.1:9001:9001 -p 40408:40408 -v YOURDIRECTORY:/fusion-node YOUR-DOCKER-HUB-ID/gateway`

## How to run a MinerAndLocalGateway

Change the parameter `YOURDIRECTORY` to your local directory

Install Docker first, e.g. on Ubuntu do `sudo apt-get install docker.io`

### Pull MinerAndLocalGateway image from repository

`docker pull fusionnetwork/minerandlocalgateway:latest`

### Run a MinerAndLocalGateway from the image

1. With ticket auto-buy disabled

`docker run -it -p 127.0.0.1:9000:9000 -p 127.0.0.1:9001:9001 -p 40408:40408 -v YOURDIRECTORY:/fusion-node fusionnetwork/minerandlocalgateway -u <account to unlock> -e MyFusionMinerAndLocalGateway`

2. With ticket auto-buy enabled

`docker run -it -p 127.0.0.1:9000:9000 -p 127.0.0.1:9001:9001 -p 40408:40408 -v YOURDIRECTORY:/fusion-node fusionnetwork/minerandlocalgateway -u <account to unlock> -e MyFusionMinerAndLocalGateway -a`

### Build your own MinerAndLocalGateway image (optional)
`docker build --file Dockerfile.minerLocalGtw -t YOUR-DOCKER-HUB-ID/minerandlocalgateway .`

### Run a MinerAndLocalGateway using your image

`docker run -it -p 127.0.0.1:9000:9000 -p 127.0.0.1:9001:9001 -p 40408:40408 -v YOURDIRECTORY:/fusion-node YOUR-DOCKER-HUB-ID/minerandlocalgateway -u <account to unlock> -e MyFusionMinerAndLocalGateway`

Remember to:
1. Replace `YOUR-DOCKER-HUB-ID` with your valid Docker Hub id.

2. Save the keystore file as `YOURDIRECTORY/UTC...`

3. Save the password.txt as `YOURDIRECTORY/password.txt`

4. (Optional) Add flag "-a" or "--autobt" to enabled ticket auto-buy.

`Note: The password file must be named password.txt and the keystore file name must start with UTC...`

You can now connect to the websocket API via `ws://localhost:9001`

## API Reference

The API reference can be found [here](https://fusionapi.readthedocs.io/en/latest/) 

## Building from source

Building efsn requires both a Go (version 1.11 or later) and a C compiler.  
You can install them using your favourite package manager.

On Ubuntu 18.04, run these commands to build efsn:

```
add-apt-repository ppa:longsleep/golang-backports
apt-get update
apt-get install golang-go build-essential
git clone https://github.com/FUSIONFoundation/efsn.git
cd efsn
make efsn
```

## Executables

The FUSION project comes with a wrapper/executable found in the `cmd` directory.

| Command    | Description |
|:----------:|-------------|
| **`efsn`** | Our main FUSION CLI client. It is the entry point into the FUSION network (main-, test- or private net), capable of running as a full node (default) or archive node (retaining all historical state). It can be used by other processes as a gateway into the FUSION network via JSON RPC endpoints exposed on top of HTTP, WebSocket and/or IPC transports. See `efsn --help` for command line options. |

## Running FUSION

Going through all the possible command line flags is out of scope here (please see `efsn --help`), but we've enumerated a few common parameter combos to get you up to speed quickly on how you can run your own efsn instance.

### Interacting with the FUSION network

By far the most common scenario is people wanting to simply interact with the FUSION network: create swaps, transfer time-locked assets; deploy and interact with contracts. To do so run

```
$ efsn console
```

This command will start up efsn's built-in interactive JavaScript console, through which you can invoke all official [`web3` methods](https://github.com/ethereum/wiki/wiki/JavaScript-API) as well as FUSION's own [APIs](https://fusionapi.readthedocs.io/en/latest/).  
This tool is optional; if you leave it out you can always attach to an already running efsn instance with `efsn attach`.

### Programmatically interfacing with FUSION

As a developer, sooner rather than later you'll want to start interacting with efsn and the FUSION network via your own programs and not manually through the console. To aid this, efsn has built-in support for JSON-RPC based APIs ([standard APIs](https://github.com/ethereum/wiki/wiki/JSON-RPC) and [FUSION RPC APIs](https://github.com/FUSIONFoundation/efsn/wiki/FSN-RPC-API)). These can be exposed via HTTP, WebSockets and IPC (unix sockets on unix based platforms).

The IPC interface is enabled by default and exposes all APIs supported by efsn, whereas the HTTP and WS interfaces need to be manually enabled and only expose a subset of the APIs due to security reasons. These can be turned on/off and configured as you'd expect.

JSON-RPC API options:

  * `--rpc` Enable the HTTP-RPC server
  * `--rpcaddr` HTTP-RPC server listening interface (default: "localhost")
  * `--rpcport` HTTP-RPC server listening port (default: 8545)
  * `--rpcapi` APIs offered over the HTTP-RPC interface (default: "eth,net,web3")
  * `--rpccorsdomain` Comma-separated list of domains from which to accept cross origin requests (browser enforced)
  * `--ws` Enable the WS-RPC server
  * `--wsaddr` WS-RPC server listening interface (default: "localhost")
  * `--wsport` WS-RPC server listening port (default: 8546)
  * `--wsapi` APIs offered over the WS-RPC interface (default: "eth,net,web3")
  * `--wsorigins` Origins from which to accept websockets requests
  * `--ipcdisable` Disable the IPC-RPC server
  * `--ipcapi` APIs offered over the IPC-RPC interface (default: all)
  * `--ipcpath` Filename for IPC socket/pipe within the datadir (explicit paths escape it)

You'll need to use your own programming environments' capabilities (libraries, tools, etc) to connect via HTTP, WS or IPC to an efsn node configured with the above flags, and you'll need to speak [JSON-RPC](http://www.jsonrpc.org/specification) on all transports. You can reuse the same connection for multiple requests!

**Note: Please understand the security implications of opening up an HTTP/WS based transport before doing so! Hackers on the internet are actively trying to subvert FUSION nodes with exposed APIs! Further, all browser tabs can access locally running webservers, so malicious webpages could try to subvert locally available APIs!**

### Operating a private network

Maintaining your own private network is more complicated as a lot of configurations taken for granted in the official networks need to be set up manually.

#### Defining the private genesis state

First, you'll need to create the genesis state of your network, which all nodes need to be aware of and agree upon. This consists of a small JSON file (e.g. call it `genesis.json`):

```json
{
  "config": {
        "chainId": 0,
        "homesteadBlock": 0,
        "eip155Block": 0,
        "eip158Block": 0
    },
  "alloc"      : {},
  "coinbase"   : "0x0000000000000000000000000000000000000000",
  "difficulty" : "0x20000",
  "extraData"  : "",
  "gasLimit"   : "0x2fefd8",
  "nonce"      : "0x0000000000000042",
  "mixhash"    : "0x0000000000000000000000000000000000000000000000000000000000000000",
  "parentHash" : "0x0000000000000000000000000000000000000000000000000000000000000000",
  "timestamp"  : "0x00"
}
```

The above fields should be fine for most purposes, although we'd recommend changing the `nonce` to some random value so you prevent unknown remote nodes from being able to connect to you. If you'd like to pre-fund some accounts for easier testing, you can populate the `alloc` field with account configs:

```json
"alloc": {
  "0x0000000000000000000000000000000000000001": {"balance": "111111111"},
  "0x0000000000000000000000000000000000000002": {"balance": "222222222"}
}
```

With the genesis state defined in the above JSON file, you'll need to initialize **every** efsn node with it prior to starting it up to ensure all blockchain parameters are correctly set:

```
$ efsn init path/to/genesis.json
```

#### Creating the rendezvous point

With all nodes that you want to run initialized to the desired genesis state, you'll need to start a bootstrap node (bootnode) that others can use to find each other in your network and/or over the internet. The clean way is to configure and run a dedicated bootnode:

```
$ bootnode --genkey=boot.key
$ bootnode --nodekey=boot.key
```

With the bootnode online, it will display an [`enode` URL](https://github.com/ethereum/wiki/wiki/enode-url-format) that other nodes can use to connect to it and exchange peer information. Make sure to replace the displayed IP address information (most probably `[::]`) with your externally accessible IP address to get the actual `enode` URL.

*Note: You could also use a full fledged efsn node as a bootnode, but that is not the recommended way.*

#### Starting up your member nodes

With the bootnode operational and externally reachable (you can try `telnet <ip> <port>` to ensure it's indeed reachable), start every subsequent efsn node pointed to the bootnode for peer discovery via the `--bootnodes` flag. It will probably also be desirable to keep the data directory of your private network separated, so do also specify a custom `--datadir` flag.

```
$ efsn --datadir=path/to/custom/data/folder --bootnodes=<bootnode-enode-url-from-above>
```

*Note: Since your network will be completely cut off from the main and test networks, you'll also need to configure a miner to process transactions and create new blocks for you.*


## Contribution

Thank you for considering to help out with the source code! We welcome contributions from anyone on the internet, and are grateful for even the smallest of fixes!

If you'd like to contribute to FUSION, please fork, fix, commit and send a pull request for the maintainers to review and merge into the main code base. If you wish to submit more complex changes though, please check up with the core devs first on [our Telegram channel](https://t.me/FsnDevCommunity) to ensure those changes are in line with the general philosophy of the project and/or get some early feedback which can make both your efforts much lighter as well as our review and merge procedures quick and simple.

Please make sure your contributions adhere to our coding guidelines:

 * Code must adhere to the official Go [formatting](https://golang.org/doc/effective_go.html#formatting) guidelines (i.e. uses [gofmt](https://golang.org/cmd/gofmt/)).
 * Code must be documented adhering to the official Go [commentary](https://golang.org/doc/effective_go.html#commentary) guidelines.
 * Pull requests need to be based on and opened against the `master` branch.

## License

The efsn and go-ethereum libraries (i.e. all code outside of the `cmd` directory) are licensed under the [GNU Lesser General Public License v3.0](https://www.gnu.org/licenses/lgpl-3.0.en.html), also included in our repository in the `COPYING.LESSER` file.

The efsn and go-ethereum binaries (i.e. all code inside of the `cmd` directory) are licensed under the [GNU General Public License v3.0](https://www.gnu.org/licenses/gpl-3.0.en.html), also included in our repository in the `COPYING` file.
