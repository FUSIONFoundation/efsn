#!/bin/sh
NODES_ROOT=/fusion-node
# NODES_ROOT=/Users/work/dev/tmp/fusion-node
DATA_DIR=$NODES_ROOT/data
ethstats=
testnet=

display_usage() { 
    echo "Commands for Fusion local gateway:" 
    echo -e "\n-e value    Reporting name of a ethstats service" 
    echo -e "\n-tn         Connect to testnet" 
    } 

while [ "$1" != "" ]; do
    case $1 in
        -u | --unlock )         shift
                                unlock=$1
                                ;;
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

# cmd_options_local_gtw=' --identity 1 --rpc --ws --rpcaddr 127.0.0.1 --rpccorsdomain 127.0.0.1 --wsapi "eth,net,fsn,fsntx" --rpcapi "eth,net,fsn,fsntx" --wsaddr 127.0.0.1 --wsport 9001 --rpcport 9000'
cmd_options_local_gtw=' --identity 1 --rpc --ws --rpcaddr 0.0.0.0 --rpccorsdomain "*" --wsorigins "*" --wsapi "eth,net,fsn,fsntx" --rpcapi "eth,net,fsn,fsntx" --wsaddr 0.0.0.0 --wsport 9001 --rpcport 9000'

if [ "$testnet" ]; then
    testnet=" --testnet"
    cmd_options=$cmd_options$testnet
fi     

if [ "$ethstats" ]; then
    if [ "$testnet" ]; then
        ethstats=" --ethstats $ethstats:devFusioInfo2019142@devnodestats.fusionnetwork.io"
        cmd_options=$cmd_options$ethstats
    else 
        ethstats=" --ethstats $ethstats:fsnMainnet@node.fusionnetwork.io"
        cmd_options=$cmd_options$ethstats
    fi
fi

echo "flags: $cmd_options$cmd_options_local_gtw"

# efsn  --unlock $unlock --ethstats 
eval "efsn $cmd_options$cmd_options_local_gtw"
