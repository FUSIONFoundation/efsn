<div align="center"><img src ="https://i.imgur.com/lixyKZe.png" height="100px" /></div>

## Go eFSN

Fusion would like to extend its gratitude to the Ethereum Foundation. Fusion has used the official open-source golang implementation of the Ethereum protocol.

## Run a Miner

Change the parameter `YOURDIRECTORY` to your local directory

### Pull Miner image from repository

`docker pull fusionnetwork/efsn2:latest`

### Run a Miner from the image
1. With auto-purchase tickets disabled
`docker run -it -p 40408:40408 -v YOURDIRECTORY:/fusion-node fusionnetwork/efsn2 -u <account to unlock> -e MyFusionMiner`
2. With auto-purchase tickets enabled
`docker run -it -p 40408:40408 -v YOURDIRECTORY:/fusion-node fusionnetwork/efsn2 -u <account to unlock> -e MyFusionMiner -a`

### Build your own Miner image (optional)

`docker build --file Dockerfile -t YOUR-DOCKER-HUB-ID/efsn2 .`

### Run a Miner using your image

`docker run -it -p 40408:40408 -v YOURDIRECTORY:/fusion-node fusionnetwork/efsn2 -u <account to unlock> -e MyFusionMiner -a`

Remember to:

1. Replace `YOUR-DOCKER-HUB-ID` with your valid Docker Hub id.

2. Put your keystore file in `YOURDIRECTORY/UTC...`

3. Put the password.txt file in: `YOURDIRECTORY/password.txt`

4. (Optional) Add flag "-a" or "--autobt" to enabled auto-purchase tickets.

`Note: The password file must be named password.txt and the keystore file name must start with UTC...`

## Run a Gateway

Change the parameter `YOURDIRECTORY` to your local directory

### Pull Gateway image from repository

`docker pull fusionnetwork/gateway2:latest`

### Run a Gateway from the image

`docker run -it -p 9000:9000 -p 9001:9001 -p 40408:40408 -v YOURDIRECTORY:/fusion-node fusionnetwork/gateway2`

### Build your own Gateway image (optional)

`docker build --file Dockerfile.gtw -t YOUR-DOCKER-HUB-ID/gateway2 .`

### Run a Gateway using your image

`docker run -it -p 9000:9000 -p 9001:9001 -p 40408:40408 -v YOURDIRECTORY:/fusion-node YOUR-DOCKER-HUB-ID/gateway2`

Remember to replace `YOUR-DOCKER-HUB-ID` with your valid Docker Hub id.

You can now connect via `ws://localhost:9001`

## Run a MinerAndLocalGateway

Change the parameter `YOURDIRECTORY` to your local directory

### Pull MinerAndLocalGateway image from repository

`docker pull fusionnetwork/minerandlocalgateway2:latest`

### Run a MinerAndLocalGateway from the image

1. With auto-purchase tickets disabled
`docker run -it -p 127.0.0.1:9000:9000 -p 127.0.0.1:9001:9001 -p 40408:40408 -v YOURDIRECTORY:/fusion-node fusionnetwork/minerandlocalgateway2 -u <account to unlock> -e MyFusionMinerAndLocalGateway`
2. With auto-purchase tickets enabled
`docker run -it -p 127.0.0.1:9000:9000 -p 127.0.0.1:9001:9001 -p 40408:40408 -v YOURDIRECTORY:/fusion-node fusionnetwork/minerandlocalgateway2 -u <account to unlock> -e MyFusionMinerAndLocalGateway -a`

### Build your own MinerAndLocalGateway image (optional)
`docker build --file Dockerfile.minerLocalGtw -t YOUR-DOCKER-HUB-ID/minerandlocalgateway2 .`

### Run a MinerAndLocalGateway using your image

`docker run -it -p 127.0.0.1:9000:9000 -p 127.0.0.1:9001:9001 -p 40408:40408 -v YOURDIRECTORY:/fusion-node YOUR-DOCKER-HUB-ID/minerandlocalgateway2 -u <account to unlock> -e MyFusionMinerAndLocalGateway`

Remember to:
1. Replace `YOUR-DOCKER-HUB-ID` with your valid Docker Hub id.

2. Put your keystore file in `YOURDIRECTORY/UTC...`

3. Put the password.txt file in: `YOURDIRECTORY/password.txt`

4. (Optional) Add flag "-a" or "--autobt" to enabled auto-purchase tickets.

`Note: The password file must be named password.txt and the keystore file name must start with UTC...`

You can now connect via `ws://localhost:9001`

## Run a miner (Ubuntu 18.04)

To run a miner on Ubuntu, please use the following guide.

Open up the terminal and execute these commands:

`sudo apt-get update`

`sudo apt-get install docker.io`

Now it is time to pay attention, since we need to have all directories set up correctly.

Replace the following to match your local environment:

`YOURDIRECTORY` = (example: /var/lib/fusion)

`YOURWALLETADDRESS` = (example: '0x0000000000000000000000000000000000000000') | Note : Single quotes must not be removed.

`NAMEOFYOURNODE` = (example: MyFusionNode)

Type in: `sudo nano runpsn.sh`
Paste the following line with your corresponding replacements:

`sudo docker run -it -p 40408:40408 -v YOURDIRECTORY:/fusion-node fusionnetwork/efsn2 -u YOURWALLETADDRESS -e NAMEOFYOURNODE`

Save the file by hitting `CTRL+O` and exit nano by pressing `CTRL+X`

We now have to make the file executable. Run the following command:

`sudo chmod +x runpsn.sh`

Put your keystore into `YOURDIRECTORY`. If the directory doesn't exist yet, create it like this:

`sudo mkdir -p YOURDIRECTORY`

Also put a file called password.txt into the same directory. It must contain only the password for the keystore.

`Note: The password file must be named password.txt and the keystore file name must start with UTC...`

Now start the miner by running:

`sudo ./runpsn.sh`

Your miner is now set up successfully. Note that you also have to buy tickets to participate in staking.

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

The fusion project comes with several wrappers/executables found in the `cmd` directory.

| Command    | Description |
|:----------:|-------------|
| **`efsn`** | Our main Fusion CLI client. It is the entry point into the fusion network (main-, test- or private net), capable of running as a full node (default), archive node (retaining all historical state) or a light node (retrieving data live). It can be used by other processes as a gateway into the fusion network via JSON RPC endpoints exposed on top of HTTP, WebSocket and/or IPC transports. `efsn --help` and the [CLI Wiki page](https://github.com/FusionFoundation/efsn/wiki/Command-Line-Options) for command line options. |
| `abigen` | Source code generator to convert Ethereum contract definitions into easy to use, compile-time type-safe Go packages. It operates on plain [Ethereum contract ABIs](https://github.com/ethereum/wiki/wiki/Ethereum-Contract-ABI) with expanded functionality if the contract bytecode is also available. However it also accepts Solidity source files, making development much more streamlined. Please see our [Native DApps](https://github.com/FusionFoundation/efsn/wiki/Native-DApps:-Go-bindings-to-Ethereum-contracts) wiki page for details. |
| `bootnode` | Stripped down version of our Fusion client implementation that only takes part in the network node discovery protocol, but does not run any of the higher level application protocols. It can be used as a lightweight bootstrap node to aid in finding peers in private networks. |
| `evm` | Developer utility version of the EVM (Ethereum Virtual Machine) that is capable of running bytecode snippets within a configurable environment and execution mode. Its purpose is to allow isolated, fine-grained debugging of EVM opcodes (e.g. `evm --code 60ff60ff --debug`). |
| `gethrpctest` | Developer utility tool to support our [ethereum/rpc-test](https://github.com/ethereum/rpc-tests) test suite which validates baseline conformity to the [Ethereum JSON RPC](https://github.com/ethereum/wiki/wiki/JSON-RPC) specs. Please see the [test suite's readme](https://github.com/ethereum/rpc-tests/blob/master/README.md) for details. |
| `rlpdump` | Developer utility tool to convert binary RLP ([Recursive Length Prefix](https://github.com/ethereum/wiki/wiki/RLP)) dumps (data encoding used by the Ethereum protocol both network as well as consensus wise) to user friendlier hierarchical representation (e.g. `rlpdump --hex CE0183FFFFFFC4C304050583616263`). |
| `swarm`    | Swarm daemon and tools. This is the entrypoint for the Swarm network. `swarm --help` for command line options and subcommands. See [Swarm README](https://github.com/FusionFoundation/efsn/tree/master/swarm) for more information. |
| `puppeth`    | a CLI wizard that aids in creating a new Ethereum network. |

## Running fusion

Going through all the possible command line flags is out of scope here (please consult our
[CLI Wiki page](https://github.com/FusionFoundation/efsn/wiki/Command-Line-Options)), but we've
enumerated a few common parameter combos to get you up to speed quickly on how you can run your
own Efsn instance.

### Full node on the main fusion network

By far the most common scenario is people wanting to simply interact with the Fusion network:
create accounts; transfer funds; deploy and interact with contracts. For this particular use-case
the user doesn't care about years-old historical data, so we can fast-sync quickly to the current
state of the network. To do so:

```
$ efsn console
```

This command will:

 * Start up efsn's built-in interactive [JavaScript console](https://github.com/FusionFoundation/efsn/wiki/JavaScript-Console),
   (via the trailing `console` subcommand) through which you can invoke all official [`web3` methods](https://github.com/ethereum/wiki/wiki/JavaScript-API)
   as well as Efsn's own [management APIs](https://github.com/FusionFoundation/efsn/wiki/Management-APIs).
   This tool is optional and if you leave it out you can always attach to an already running Efsn instance
   with `efsn attach`.

### Programatically interfacing fusion nodes

As a developer, sooner rather than later you'll want to start interacting with efsn and the fusion
network via your own programs and not manually through the console. To aid this, Efsn has built-in
support for a JSON-RPC based APIs ([standard APIs](https://github.com/ethereum/wiki/wiki/JSON-RPC) and
[Efsn specific APIs](https://github.com/FusionFoundation/efsn/wiki/Management-APIs)). These can be
exposed via HTTP, WebSockets and IPC (unix sockets on unix based platforms, and named pipes on Windows).

The IPC interface is enabled by default and exposes all the APIs supported by Efsn, whereas the HTTP
and WS interfaces need to manually be enabled and only expose a subset of APIs due to security reasons.
These can be turned on/off and configured as you'd expect.

HTTP based JSON-RPC API options:

  * `--rpc` Enable the HTTP-RPC server
  * `--rpcaddr` HTTP-RPC server listening interface (default: "localhost")
  * `--rpcport` HTTP-RPC server listening port (default: 8545)
  * `--rpcapi` API's offered over the HTTP-RPC interface (default: "eth,net,web3")
  * `--rpccorsdomain` Comma separated list of domains from which to accept cross origin requests (browser enforced)
  * `--ws` Enable the WS-RPC server
  * `--wsaddr` WS-RPC server listening interface (default: "localhost")
  * `--wsport` WS-RPC server listening port (default: 8546)
  * `--wsapi` API's offered over the WS-RPC interface (default: "eth,net,web3")
  * `--wsorigins` Origins from which to accept websockets requests
  * `--ipcdisable` Disable the IPC-RPC server
  * `--ipcapi` API's offered over the IPC-RPC interface (default: "admin,debug,eth,miner,net,personal,shh,txpool,web3")
  * `--ipcpath` Filename for IPC socket/pipe within the datadir (explicit paths escape it)

You'll need to use your own programming environments' capabilities (libraries, tools, etc) to connect
via HTTP, WS or IPC to a Efsn node configured with the above flags and you'll need to speak [JSON-RPC](http://www.jsonrpc.org/specification)
on all transports. You can reuse the same connection for multiple requests!

**Note: Please understand the security implications of opening up an HTTP/WS based transport before
doing so! Hackers on the internet are actively trying to subvert Fusion nodes with exposed APIs!
Further, all browser tabs can access locally running webservers, so malicious webpages could try to
subvert locally available APIs!**

### Operating a private network

Maintaining your own private network is more involved as a lot of configurations taken for granted in
the official networks need to be manually set up.

#### Defining the private genesis state

First, you'll need to create the genesis state of your networks, which all nodes need to be aware of
and agree upon. This consists of a small JSON file (e.g. call it `genesis.json`):

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

The above fields should be fine for most purposes, although we'd recommend changing the `nonce` to
some random value so you prevent unknown remote nodes from being able to connect to you. If you'd
like to pre-fund some accounts for easier testing, you can populate the `alloc` field with account
configs:

```json
"alloc": {
  "0x0000000000000000000000000000000000000001": {"balance": "111111111"},
  "0x0000000000000000000000000000000000000002": {"balance": "222222222"}
}
```

With the genesis state defined in the above JSON file, you'll need to initialize **every** Efsn node
with it prior to starting it up to ensure all blockchain parameters are correctly set:

```
$ efsn init path/to/genesis.json
```

#### Creating the rendezvous point

With all nodes that you want to run initialized to the desired genesis state, you'll need to start a
bootstrap node that others can use to find each other in your network and/or over the internet. The
clean way is to configure and run a dedicated bootnode:

```
$ bootnode --genkey=boot.key
$ bootnode --nodekey=boot.key
```

With the bootnode online, it will display an [`enode` URL](https://github.com/ethereum/wiki/wiki/enode-url-format)
that other nodes can use to connect to it and exchange peer information. Make sure to replace the
displayed IP address information (most probably `[::]`) with your externally accessible IP to get the
actual `enode` URL.

*Note: You could also use a full fledged Efsn node as a bootnode, but it's the less recommended way.*

#### Starting up your member nodes

With the bootnode operational and externally reachable (you can try `telnet <ip> <port>` to ensure
it's indeed reachable), start every subsequent Efsn node pointed to the bootnode for peer discovery
via the `--bootnodes` flag. It will probably also be desirable to keep the data directory of your
private network separated, so do also specify a custom `--datadir` flag.

```
$ efsn --datadir=path/to/custom/data/folder --bootnodes=<bootnode-enode-url-from-above>
```

*Note: Since your network will be completely cut off from the main and test networks, you'll also
need to configure a miner to process transactions and create new blocks for you.*


## Contribution

Thank you for considering to help out with the source code! We welcome contributions from
anyone on the internet, and are grateful for even the smallest of fixes!

If you'd like to contribute to fusion, please fork, fix, commit and send a pull request
for the maintainers to review and merge into the main code base. If you wish to submit more
complex changes though, please check up with the core devs first on [our gitter channel](https://gitter.im/FusionFoundation/efsn)
to ensure those changes are in line with the general philosophy of the project and/or get some
early feedback which can make both your efforts much lighter as well as our review and merge
procedures quick and simple.

Please make sure your contributions adhere to our coding guidelines:

 * Code must adhere to the official Go [formatting](https://golang.org/doc/effective_go.html#formatting) guidelines (i.e. uses [gofmt](https://golang.org/cmd/gofmt/)).
 * Code must be documented adhering to the official Go [commentary](https://golang.org/doc/effective_go.html#commentary) guidelines.
 * Pull requests need to be based on and opened against the `master` branch.
 * Commit messages should be prefixed with the package(s) they modify.
   * E.g. "eth, rpc: make trace configs optional"

Please see the [Developers' Guide](https://github.com/FusionFoundation/efsn/wiki/Developers'-Guide)
for more details on configuring your environment, managing project dependencies and testing procedures.

## License

The fusion and go-ethereum library (i.e. all code outside of the `cmd` directory) is licensed under the
[GNU Lesser General Public License v3.0](https://www.gnu.org/licenses/lgpl-3.0.en.html), also
included in our repository in the `COPYING.LESSER` file.

The fusion and go-ethereum binaries (i.e. all code inside of the `cmd` directory) is licensed under the
[GNU General Public License v3.0](https://www.gnu.org/licenses/gpl-3.0.en.html), also included
in our repository in the `COPYING` file.
