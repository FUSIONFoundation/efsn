#!/bin/sh
NODES_ROOT=/fusion-node
# NODES_ROOT=/Users/work/dev/tmp/fusion-node
DATA_DIR=$NODES_ROOT/data
KEYSTORE_DIR=$DATA_DIR/keystore
unlock=
ethstats=
autobt=

display_usage() { 
    echo "Commands for Fusion efsn:" 
    echo -e "\n-e value    Reporting name of a ethstats service" 
    echo -e "\n-u value    Account to unlock" 
    echo -e "\n-a value    Auto buy tickets (0 = maximum possible)" 
    } 

while [ "$1" != "" ]; do
    case $1 in
        -u | --unlock )         shift
                                unlock=$1
                                ;;
        -e | --ethstats )           shift
                                ethstats=$1
                                ;;
        -a | --autobt )         shift
                                autobt=$1
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
cp $NODES_ROOT/UTC* $KEYSTORE_DIR/

# format command option
cmd_options="--datadir $DATA_DIR --password /fusion-node/password.txt --mine"

# cmd_options_local_gtw=' --identity 1 --rpc --ws --rpcaddr 127.0.0.1 --rpccorsdomain 127.0.0.1 --wsapi "eth,net,fsn,fsntx" --rpcapi "eth,net,fsn,fsntx" --wsaddr 127.0.0.1 --wsport 9001 --rpcport 9000'
cmd_options_local_gtw=' --rpc --ws --rpcaddr 0.0.0.0 --rpccorsdomain 0.0.0.0  --wsapi "eth,net,fsn,fsntx" --rpcapi "eth,net,fsn,fsntx" --wsaddr 0.0.0.0 --wsport 9001 --wsorigins=* --rpcport 9000'

if [ "$ethstats" ]; then
    ethstats=" --ethstats $ethstats:fusion@node.fusionnetwork.io"
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
    
if [ "$autobt" ]; then
    # limit for auto buy tickets not implemented yet, so it's always 0 = unlimited for now
    autobt=0
    [ $autobt -gt 0 ] && autobt=" --autobt $autobt" || autobt=" --autobt"
    cmd_options=$cmd_options$autobt 
fi

echo "flags: $cmd_options$cmd_options_local_gtw"

# efsn  --unlock $unlock --ethstats 
efsn $cmd_options$cmd_options_local_gtw

