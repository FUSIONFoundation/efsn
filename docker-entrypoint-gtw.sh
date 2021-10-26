#!/bin/sh
NODES_ROOT=/fusion-node
# NODES_ROOT=/Users/work/dev/tmp/fusion-node
DATA_DIR=$NODES_ROOT/data
ethstats=
testnet=

display_usage() {
    echo "Commands for Fusion efsn:"
    echo -e "-e value    Reporting name of a ethstats service"
    echo -e "-tn         Connect to testnet"
    }

while [ "$1" != "" ]; do
    case $1 in
        -e | --ethstats )       shift
                                ethstats=$1
                                ;;
        -tn | --testnet )       testnet=true
                                ;;
        * )                     display_usage
                                exit 1
    esac
    shift
done

# create data folder if does not exit
if [ ! -d "$DATA_DIR" ]; then
    mkdir $DATA_DIR
fi

# format command option
cmd_options="--datadir $DATA_DIR"

# Following the geth documentation at https://geth.ethereum.org/docs/rpc/server
# cmd_options_local_gtw=' --identity 1 --rpc --rpcapi "eth,net,fsn,fsntx" --rpcaddr 127.0.0.1 --rpcport 9000 --rpccorsdomain "*" --ws  --wsapi "eth,net,fsn,fsntx" --wsaddr 127.0.0.1 --wsport 9001 --wsorigins "*"'
cmd_options_local_gtw=' --identity 1 --http --http.api "eth,net,fsn,fsntx" --http.addr 0.0.0.0 --http.port 9000 --http.corsdomain "*" --ws --ws.api "eth,net,fsn,fsntx" --ws.addr 0.0.0.0 --ws.port 9001 --ws.origins "*"'

if [ "$testnet" ]; then
    testnet=" --testnet"
    cmd_options=$cmd_options$testnet
fi     

if [ "$ethstats" ]; then
    if [ "$testnet" ]; then
        ethstats=" --ethstats $ethstats:devFusioInfo2019142@devnodestats.fusionnetwork.io"
    else 
        ethstats=" --ethstats $ethstats:fsnMainnet@node.fusionnetwork.io"
    fi
    cmd_options=$cmd_options$ethstats
fi

echo "flags: $cmd_options$cmd_options_local_gtw"

# efsn  --unlock $unlock --ethstats
exec efsn $cmd_options$cmd_options_local_gtw
