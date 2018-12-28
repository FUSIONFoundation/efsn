#!/bin/sh
NODES_ROOT=/fusion-node
# NODES_ROOT=/Users/work/dev/tmp/fusion-node
DATA_DIR=$NODES_ROOT/data
KEYSTORE_DIR=$DATA_DIR/keystore
unlock=
ethstats=

display_usage() { 
    echo "Commands for Fusion efsn:" 
    echo -e "\n-e value    Reporting name of a ethstats service" 
    echo -e "\n-u value    Account to unlock" 
    } 

while [ "$1" != "" ]; do
    case $1 in
        -u | --unlock )         shift
                                unlock=$1
                                ;;
        -e | --ethstats )           shift
                                ethstats=$1
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
cmd_options="--datadir $DATA_DIR --port 8001 --password /fusion-node/password.txt --mine"

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
    
echo "final cmd_options updated to $cmd_options"


# efsn  --unlock $unlock --ethstats 
efsn $cmd_options