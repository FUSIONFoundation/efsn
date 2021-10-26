#!/bin/sh
NODES_ROOT=/fusion-node
# NODES_ROOT=/Users/work/dev/tmp/fusion-node
DATA_DIR=$NODES_ROOT/data
KEYSTORE_DIR=$DATA_DIR/keystore
unlock=
ethstats=
testnet=
autobt=false
mining=true

display_usage() {
    echo "Commands for Fusion efsn:"
    echo -e "-e value    Reporting name of a ethstats service"
    echo -e "-u value    Account to unlock"
    echo -e "-a          Auto buy tickets"
    echo -e "-tn         Connect to testnet"
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
        -a | --autobt )         autobt=true
                                ;;
        --disable-mining )      mining=false
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

# create keystore folder if does not exit
if [ ! -d "$KEYSTORE_DIR" ]; then
    mkdir $KEYSTORE_DIR
fi

# copy keystore file
cp $NODES_ROOT/UTC* $KEYSTORE_DIR/ 2>/dev/null

# format command option
cmd_options="--datadir $DATA_DIR --password /fusion-node/password.txt"

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

if [ "$unlock" ]; then

    # pattern=$KEYSTORE_DIR/UTC*
    # echo "pater: $pattern"

    # keystore_files=($pattern)

    # # for f in "${keystore_files[@]}"; do
    # #     echo "file: $f"
    # # done

    # echo "file: ${keystore_files[0]}"

    # utc_json_address="$(cat < ${keystore_files[0]} | jq -r '.address')"
    # echo "utc_json_address: $utc_json_address"

    # if [ "$utc_json_address" = "$unlock" ]; then
    #     echo "$unlock is 'valid'"
    # fi

    # echo "unlock set to: $unlock"
    unlock=" --unlock $unlock"
    cmd_options=$cmd_options$unlock
fi

if [ "$autobt" = true ]; then
    autobt=" --autobt"
    cmd_options=$cmd_options$autobt
fi

if [ "$mining" = true ]; then
    mining=" --mine"
    cmd_options=$cmd_options$mining
fi

echo "efsn flags: $cmd_options$cmd_options_local_gtw"

# efsn  --unlock $unlock --ethstats
exec efsn $cmd_options$cmd_options_local_gtw
