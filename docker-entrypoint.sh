#!/bin/sh
NODES_ROOT=/fusion-node
# NODES_ROOT=/Users/work/dev/tmp/fusion-node
DATA_DIR=$NODES_ROOT/data
KEYSTORE_DIR=$DATA_DIR/keystore
unlock=
ethstats=
autobt="false"
mining="true"

display_usage() {
    echo "Commands for Fusion efsn:"
    echo -e "-e value    Reporting name of a ethstats service"
    echo -e "-u value    Account to unlock"
    echo -e "-a          Auto buy tickets"
    }

while true; do
    case $1 in
        -u | --unlock )         shift
                                unlock=$1
                                ;;
        -e | --ethstats )       shift
                                ethstats=$1
                                ;;
        -a | --autobt )         autobt="true"
                                ;;
        --disable-mining )      mining="false"
                                ;;
        * )                     display_usage
                                exit 1
    esac
    shift || break
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

                                                                                                                                                                                                         
                                                                                                                                                                                                   

if [ "$ethstats" ]; then
    ethstats=" --ethstats $ethstats:fsnMainnet@node.fusionnetwork.io"
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

if [ "$autobt" = "true" ]; then
    autobt=" --autobt"
    cmd_options=$cmd_options$autobt
fi

if [ "$mining" = "true" ]; then
    mining=" --mine"
    cmd_options=$cmd_options$mining
fi

echo "efsn flags: $cmd_options"

# efsn  --unlock $unlock --ethstats
eval "efsn $cmd_options"
