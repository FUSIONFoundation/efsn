#!/bin/bash
# FUSION Foundation

txtred=$(tput setaf 1)    # Red
txtgrn=$(tput setaf 2)    # Green
txtylw=$(tput setaf 3)    # Yellow
txtrst=$(tput sgr0)       # Text reset

BASE_DIR="/home/$USER"

distroChecks() {
    # check for distribution and corresponding version (release)
    if [ "$(lsb_release -si)" = "Ubuntu" ]; then
        if [ $(lsb_release -sr | sed -E 's/([0-9]+).*/\1/') -lt 16 ]; then
            echo "${txtred}Unsupported legacy Ubuntu release${txtrst}"
            exit 1
        fi
    else
        echo "${txtred}Unsupported distribution${txtrst}"
        exit 1
    fi
}

sanityChecks() {
    distroChecks

    # checking the effective user id, where 0 is root
    if [ $EUID -ne 0 ]; then
        # validate user, no point in moving on as non-root user without sudo access
        if ! sudo -v 2>/dev/null; then
            echo "${txtred}You are neither logged in as user root, nor do you have sudo access.${txtrst}"
            echo "Please run the setup script again as user root or configure sudo access."
            exit 1
        fi
        # make sure that the script isn't run as root and non-root user alternately
        if sudo [ -f "/home/root/fusion-node/node.json" ]; then
            echo "${txtred}The setup script was originally run with root privileges.${txtrst}"
            echo "Please run it again as user root or by invoking sudo:"
            echo "sudo bash -c \"\$(curl -fsSL https://raw.githubusercontent.com/FUSIONFoundation/efsn/master/QuickNodeSetup/fsnNode.sh)\""
            exit 1
        fi
    fi

    # using locate without prior update is a perf vs reliability tradeoff
    # we don't want to wait until the whole fs is indexed or even use find
    if [ $(sudo locate -r .*/efsn/chaindata$ -c) -gt 1 ]; then
        echo "${txtred}Found more than one chaindata directory.${txtrst}"
        echo "Please clean up the system manually first."
        sudo locate -r .*/efsn/chaindata$

        echo
        local question="${txtylw}Do you believe this issue is already resolved?${txtrst} [Y/n] "
        askToContinue "$question"
        if [ $? -eq 0 ]; then
            echo "Running checks again..."
            sudo updatedb
            sanityChecks
        else
            exit 1
        fi
    fi

    # this also covers the case where the setup script was originally run as non-root user
    if [ -n "$(sudo locate -r .*/efsn/chaindata$ | grep -v $BASE_DIR)" ]; then
        echo "${txtred}Found chaindata directory outside of $BASE_DIR.${txtrst}"
        echo "Please clean up the system manually first."
        sudo locate -r .*/efsn/chaindata$

        echo
        local question="${txtylw}Do you believe this issue is already resolved?${txtrst} [Y/n] "
        askToContinue "$question"
        if [ $? -eq 0 ]; then
            echo "Running checks again..."
            sudo updatedb
            sanityChecks
        else
            exit 1
        fi
    fi

    # make sure jq is installed if node.json already exists
    if [ -f "$BASE_DIR/fusion-node/node.json" ]; then
        dpkg -s jq 2>/dev/null | grep -q -E "Status.+installed" || apt-get install jq
    fi
}

pauseScript() {
    local fackEnterKey
    read -s -p "${txtylw}Press [Enter] to continue...${txtrst}" fackEnterKey
}

askToContinue() {
    local input
    while true; do
        read -n1 -r -s -p "$1" input
        case $input in
            [yY]) echo; return 0 ;;
            [nN]) echo; return 1 ;;
            *)    echo -e "\n${txtred}Invalid input${txtrst}" ;;
        esac
    done
}

installDocker() {
    # install recent Docker version; function currently unused
    echo
    echo "${txtylw}Adding Docker repository${txtrst}"
    curl -fsSL "https://download.docker.com/linux/ubuntu/gpg" | sudo apt-key add -
    sudo add-apt-repository -u -y "deb [arch=amd64] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable"
    echo "${txtgrn}✓${txtrst} Added Docker repository"

    echo
    echo "${txtylw}Installing Docker${txtrst}"
    sudo apt-get install -q -y docker-ce
    echo "${txtgrn}✓${txtrst} Installed Docker"
}

installDeps() {
    echo
    echo "${txtylw}Updating package lists${txtrst}"
    echo "This might take a moment, please wait..."
    sudo apt-get -qq update
    echo "${txtgrn}✓${txtrst} Updated package lists"

    echo
    echo "${txtylw}Installing dependencies${txtrst}"
    sudo apt-get install -q -y apt-transport-https ca-certificates curl docker.io gnupg-agent \
        jq software-properties-common | grep -v "is already the newest version"
    # install recent Docker version if distribution package not installed yet; currently unused
#    dpkg -s docker.io | grep -q -E "Status.+installed" || installDocker
    echo "${txtgrn}✓${txtrst} Installed dependencies"
}

getCfgValue() {
    # read configuration values from the respective JSON files
    local cfg_arg="$1"
    local cfg_val

    if [ $# -ne 1 ]; then
        echo "${txtred}Incorrect number of arguments ($#)${txtrst}"
        return 1
    fi

    local keystore_file="$BASE_DIR/fusion-node/data/keystore/UTC.json"
    local cfg_file="$BASE_DIR/fusion-node/node.json"

    # extra spacing for readability
    if [ "$cfg_arg" = "address" ]; then
        cfg_val="0x$(jq -r '.address' $keystore_file)"

    elif [ "$cfg_arg" = "nodeType" ]; then
        cfg_val="$(jq -r '.nodeType' $cfg_file)"

    elif [ "$cfg_arg" = "testnet" ]; then
        cfg_val="$(jq -r '.testnet' $cfg_file)"

    elif [ "$cfg_arg" = "autobt" ]; then
        cfg_val="$(jq -r '.autobt' $cfg_file)"

    elif [ "$cfg_arg" = "mining" ]; then
        cfg_val="$(jq -r '.mining' $cfg_file)"

    elif [ "$cfg_arg" = "nodeName" ]; then
        cfg_val="$(jq -r '.nodeName' $cfg_file)"

    else
        echo "${txtred}Unknown argument ($cfg_arg)${txtrst}"
        return 1
    fi

    echo "$cfg_val"
}

putCfgValue() {
    # write configuration values to the respective JSON files
    local cfg_arg="$1"
    local cfg_val="$2"

    if [ $# -ne 2 ]; then
        echo "${txtred}Incorrect number of arguments ($#)${txtrst}"
        return 1
    fi

    local cfg_file="$BASE_DIR/fusion-node/node.json"

    # extra spacing for readability
    if [ "$cfg_arg" = "nodeType" ]; then
        cat <<< "$(jq --arg nodeType "$nodeType" '.nodeType = $nodeType' < $cfg_file)" > "$cfg_file"

    elif [ "$cfg_arg" = "testnet" ]; then
        cat <<< "$(jq --arg testnet "$testnet" '.testnet = $testnet' < $cfg_file)" > "$cfg_file"

    elif [ "$cfg_arg" = "autobt" ]; then
        cat <<< "$(jq --arg autobt "$autobt" '.autobt = $autobt' < $cfg_file)" > "$cfg_file"

    elif [ "$cfg_arg" = "mining" ]; then
        cat <<< "$(jq --arg mining "$mining" '.mining = $mining' < $cfg_file)" > "$cfg_file"

    elif [ "$cfg_arg" = "nodeName" ]; then
        cat <<< "$(jq --arg nodeName "$nodeName" '.nodeName = $nodeName' < $cfg_file)" > "$cfg_file"

    else
        echo "${txtred}Unknown argument ($cfg_arg)${txtrst}"
        return 1
    fi
}

updateKeystoreFile() {
    echo
    echo "${txtylw}Open your keystore file (its name usually starts with UTC--) in any"
    echo "plaintext editor (like notepad) and copy and paste its full contents"
    echo "here; it should contain a lot of cryptic text surrounded by \"{...}\".${txtrst}"
    local keystorejson
    while true; do
        read -p "Paste keystore contents: " keystorejson
        if [ -z "$keystorejson" ]; then
            echo "${txtred}Keystore contents required${txtrst}"
        elif [[ "$keystorejson" != *"address"* ]]; then
            echo "${txtred}Invalid keystore format${txtrst}"
        elif ! jq -e . >/dev/null 2>&1 <<< "$keystorejson"; then
            echo "${txtred}Invalid keystore format${txtrst}"
        else
            # this breaks for me - avoid xargs if possible
            #echo $keystorejson | xargs > "$BASE_DIR/fusion-node/data/keystore/UTC.json"
            # this should be a pretty robust solution
            mkdir -p "$BASE_DIR/fusion-node/data/keystore/"
            printf '%s' "$keystorejson" > "$BASE_DIR/fusion-node/data/keystore/UTC.json"

            echo
            local address="$(getCfgValue 'address')"
            local question="${txtylw}Is this the expected address of your staking wallet?${txtrst} $address [Y/n] "
            askToContinue "$question"
            if [ $? -eq 1 ]; then
                echo "${txtred}Please use the right keystore file${txtrst}"
            else
                break
            fi
        fi
    done
}

updateKeystorePass() {
    echo
    echo "${txtylw}Please enter or paste the password that unlocks the keystore file.${txtrst}"
    local keystorepass
    while true; do
        read -p "Enter keystore password: " keystorepass
        if [ -z "$keystorepass" ]; then
            echo "${txtred}Keystore password required${txtrst}"
        else
            break
        fi
    done
    printf '%s' "$keystorepass" > "$BASE_DIR/fusion-node/password.txt"
}

warnRetreat() {
    local nodetype="$(getCfgValue 'nodeType')"
    if [ "$nodetype" = "minerandlocalgateway" -o "$nodetype" = "efsn" ]; then
        # check if the configured address is mining and has active tickets
        local address="$(getCfgValue 'address')"
        local mining="$(getCfgValue 'mining')"
        # talking to the RPC socket directly without invoking efsn is the fastest possible way to access the API locally
        # nc has to be called with sudo because the socket is owned by the container running with root privileges
        local tickets=$(printf '{"jsonrpc":"2.0","method":"fsn_totalNumberOfTicketsByAddress","params":["%s","latest"],"id":1}' "$address" \
            | sudo nc -N -U "$BASE_DIR/fusion-node/data/efsn.ipc" | jq -r '.result')
        # tickets override for testing purposes
        #tickets=12345
        if [ "$mining" != "false" -a $tickets -gt 0 ]; then
            echo
            echo "${txtylw}Your node is currently configured for mining and has $tickets active tickets.${txtrst}"
            echo "If you stop the node for too long, your tickets might get retreated."
            echo
            local question="${txtylw}Are you sure you want to continue?${txtrst} [Y/n] "
            askToContinue "$question"
            [ $? -eq 0 ] || return 1
        fi
    fi
}

initConfig() {
    local question

    echo
    echo "${txtylw}Please select the node type to install:${txtrst}"
    echo "${txtylw}1. minerandlocalgateway${txtrst} - miner with local API access; if unsure, select this"
    echo "${txtylw}2. efsn${txtrst} - pure miner without local API access; only select if you have a good reason"
    echo "${txtylw}3. gateway${txtrst} - local API access without mining; doesn't require keystore and password"
    echo
    local nodetype
    local input
    while true; do
        read -n1 -r -s -p "Select option [1-3] " input
        case $input in
            1) nodetype="minerandlocalgateway"; break ;;
            2) nodetype="efsn"; break ;;
            3) nodetype="gateway"; echo; break ;;
            *) echo -e "\n${txtred}Invalid input${txtrst}" ;;
        esac
    done
    echo
    echo "${txtgrn}✓${txtrst} Selected node type $nodetype"

    echo
    question="${txtylw}Do you want to install a mainnet node?${txtrst} [Y/n] "
    local testnet="false"
    askToContinue "$question"
    if [ $? -eq 0 ]; then
        echo "${txtgrn}✓${txtrst} Installing mainnet node"
    else
        echo
        question="${txtylw}Are you sure you want to install a testnet node?${txtrst} [Y/n] "
        askToContinue "$question"
        if [ $? -eq 0 ]; then
            testnet="true"
            echo "${txtgrn}✓${txtrst} Installing testnet node"
        else
            echo "${txtgrn}✓${txtrst} Installing mainnet node"
        fi
    fi

    # purging existing node data as late as possible
    sudo rm -rf "$BASE_DIR/fusion-node/"

    if [ "$nodetype" = "minerandlocalgateway" -o "$nodetype" = "efsn" ]; then
        updateKeystoreFile
        echo "${txtgrn}✓${txtrst} Saved keystore file"

        updateKeystorePass
        echo "${txtgrn}✓${txtrst} Saved keystore password"

        echo
        question="${txtylw}Do you want your node to auto-buy tickets required for staking?${txtrst} [Y/n] "
        local autobuy="false"
        askToContinue "$question"
        if [ $? -eq 0 ]; then
            autobuy="true"
            echo "${txtgrn}✓${txtrst} Enabled ticket auto-buy"
        else
            echo "${txtred}✓${txtrst} Disabled ticket auto-buy"
        fi

        echo
        question="${txtylw}Do you want your node to mine blocks (participate in staking)?${txtrst} [Y/n] "
        local mining="false"
        askToContinue "$question"
        if [ $? -eq 0 ]; then
            mining="true"
            echo "${txtgrn}✓${txtrst} Enabled mining of blocks"
        else
            echo "${txtred}✓${txtrst} Disabled mining of blocks"
        fi
    fi

    echo
    question="${txtylw}Do you want your node to auto-start at boot to prevent downtimes?${txtrst} [Y/n] "
    askToContinue "$question"
    if [ $? -eq 0 ]; then
        sudo curl -fsSL "https://raw.githubusercontent.com/FUSIONFoundation/efsn/master/QuickNodeSetup/fusion.service" \
            -o "/etc/systemd/system/fusion.service"
        sudo systemctl daemon-reload
        sudo systemctl -q enable fusion
        echo "${txtgrn}✓${txtrst} Enabled node auto-start"
    else
        # suppress warning about nonexisting service unit on fresh install
        sudo systemctl -q disable fusion 2>/dev/null
        echo "${txtred}✓${txtrst} Disabled node auto-start"
    fi

    echo
    echo "${txtylw}What name do you want the node to have on node.fusionnetwork.io? No spaces or special characters, three characters minimum.${txtrst}"
    local nodename
    while true; do
        read -p "Enter node name: " nodename
        if [ -z "$nodename" ]; then
            echo "${txtred}Node name required${txtrst}"
        elif [[ ! "$nodename" =~ ^[-_a-zA-Z0-9]{3,}$ ]]; then
            echo "${txtred}Invalid characters in node name or too short${txtrst}"
            echo "Only a-z, 0-9, - and _ allowed, minimum 3 chars"
        else
            break
        fi
    done
    echo "${txtgrn}✓${txtrst} Saved node name"

    # write configuration to file in proper JSON format
    echo
    echo "${txtylw}Writing node configuration file${txtrst}"
    mkdir -p "$BASE_DIR/fusion-node/"
    jq -n --arg nodeType "$nodetype" --arg testnet "$testnet" --arg autobt "$autobuy" --arg mining "$mining" --arg nodeName "$nodename" \
        '{"nodeType": $nodeType, "testnet": $testnet, "autobt": $autobt, "mining": $mining, "nodeName": $nodeName}' > "$BASE_DIR/fusion-node/node.json"
    echo "${txtgrn}✓${txtrst} Wrote node configuration file"
}

removeContainer() {
    # only try to stop the container if it's running
    [ "$(sudo docker inspect -f "{{.State.Running}}" fusion 2>/dev/null)" = "true" ] && stopNode
    # remove container and base images no matter what
    echo
    echo "${txtylw}Removing container and base images${txtrst}"
    sudo docker rm fusion >/dev/null 2>&1
    sudo docker rmi fusionnetwork/minerandlocalgateway fusionnetwork/efsn \
        fusionnetwork/gateway >/dev/null 2>&1
    echo "${txtgrn}✓${txtrst} Removed container and base images"
}

createContainer() {
    # read configuration files
    echo
    echo "${txtylw}Reading node configuration${txtrst}"
    local nodetype="$(getCfgValue 'nodeType')"
    local testnet="$(getCfgValue 'testnet')"
    local nodename="$(getCfgValue 'nodeName')"
    if [ "$nodetype" = "minerandlocalgateway" -o "$nodetype" = "efsn" ]; then
        local autobuy="$(getCfgValue 'autobt')"
        local mining="$(getCfgValue 'mining')"
        local address="$(getCfgValue 'address')"
    fi
    echo "${txtgrn}✓${txtrst} Read node configuration"

    if [ "$testnet" = "true" ]; then
        # add testnet suffix to image
        imgsfx="2"
    else
        # don't add testnet suffix to image
        imgsfx=""
    fi

    if [ "$autobuy" = "true" ]; then
        # turn autobuy on
        autobuy="--autobt"
    else
        # turn autobuy off
        autobuy=""
    fi

    # make sure mining remains enabled for old configs
    if [ "$mining" != "false" ]; then
        # turn mining on
        mining=""
    else
        # turn mining off
        mining="--disable-mining"
    fi

    echo
    echo "${txtylw}Creating node container${txtrst}"
    if [ "$nodetype" = "minerandlocalgateway" ]; then
        # docker create automatically pulls the image if it's not there
        # we don't need -i here as it's not really an interactive terminal
        sudo docker create --name fusion -t --restart unless-stopped \
            -p 127.0.0.1:9000:9000 -p 127.0.0.1:9001:9001 -p 40408:40408 -p 40408:40408/udp \
            -v "$BASE_DIR/fusion-node":/fusion-node \
            fusionnetwork/minerandlocalgateway$imgsfx \
            -u "$address" "$autobuy" "$mining" \
            -e "$nodename"

    elif [ "$nodetype" = "efsn" ]; then
        sudo docker create --name fusion -t --restart unless-stopped \
            -p 40408:40408 -p 40408:40408/udp \
            -v "$BASE_DIR/fusion-node":/fusion-node \
            fusionnetwork/efsn$imgsfx \
            -u "$address" "$autobuy" "$mining" \
            -e "$nodename"

    elif [ "$nodetype" = "gateway" ]; then
        # the gateway image doesn't have an entrypoint script,
        # so we're passing the ethstats option directly to efsn
        local ethstats
        if [ -n "$nodename" ]; then
            # set node name to be shown in node explorer
            ethstats="--ethstats $nodename:fsnMainnet@node.fusionnetwork.io"
        else
            # node won't appear in node explorer at all
            ethstats=""
        fi

        sudo docker create --name fusion -t --restart unless-stopped \
            -p 127.0.0.1:9000:9000 -p 127.0.0.1:9001:9001 -p 40408:40408 -p 40408:40408/udp \
            -v "$BASE_DIR/fusion-node":/fusion-node \
            fusionnetwork/gateway$imgsfx \
            $ethstats

    else
        echo "${txtred}Invalid node type${txtrst}"
        exit 1
    fi

    echo "${txtgrn}✓${txtrst} Created node container"
}

startNode() {
    echo
    echo "${txtylw}Starting the node${txtrst}"
    sudo docker start fusion >/dev/null
    if [ $? -eq 0 ]; then
        echo "${txtgrn}✓${txtrst} Node started"
        echo
        echo "---------------------------------------------------------------"
        echo "| Please use the \"Show node logs\" function from the main menu |"
        echo "|  to verify that the node is really running without errors!  |"
        echo "---------------------------------------------------------------"
    else
        echo "${txtred}Node failed to start${txtrst}"
    fi
}

stopNode() {
    echo
    echo "${txtylw}Stopping the node${txtrst}"
    echo "This might take a moment, please wait..."
    sudo docker stop fusion >/dev/null
    if [ $? -eq 0 ]; then
        echo "${txtgrn}✓${txtrst} Node stopped"
    else
        echo "${txtred}Node failed to stop${txtrst}"
    fi
}

installNode() {
    clear
    echo
    echo "---------------------"
    echo "| Node Installation |"
    echo "---------------------"

    if [ -d "$BASE_DIR/fusion-node" ]; then
        echo
        echo "${txtylw}You already seem to have a node installed in $BASE_DIR/fusion-node.${txtrst}"
        echo "It will be stopped and its configuration and chaindata will be purged."
        echo "This means it has to sync from scratch again, which could take a while."
        echo
        local question="${txtylw}Are you sure you want to continue?${txtrst} [Y/n] "
        askToContinue "$question"
        if [ $? -eq 1 ]; then
            echo "${txtred}✓${txtrst} Installation cancelled"
            return 1
        fi
    fi

    echo
    echo "<<< Installing node >>>"
    installDeps
    initConfig
    removeContainer
    createContainer
    startNode
    echo
    echo "<<< ${txtgrn}✓${txtrst} Installed node >>>"
    echo
    pauseScript
}

deinstallNode() {
    clear
    echo
    echo "-----------------------"
    echo "| Node Deinstallation |"
    echo "-----------------------"

    if [ -d "$BASE_DIR/fusion-node" ]; then
        echo
        echo "You already seem to have a node installed in $BASE_DIR/fusion-node."
        echo "It will be stopped and its configuration and chaindata will be purged."
        echo
        local question="${txtylw}Are you sure you want to continue?${txtrst} [Y/n] "
        askToContinue "$question"
        if [ $? -eq 1 ]; then
            echo "${txtred}✓${txtrst} Deinstallation cancelled"
            return 1
        fi
    fi

    echo
    echo "<<< Deinstalling node >>>"
    sudo rm -rf "$BASE_DIR/fusion-node/"
    removeContainer
    sudo systemctl -q stop fusion
    sudo systemctl -q disable fusion
    sudo systemctl daemon-reload
    sudo rm -f "/etc/systemd/system/fusion.service"
    echo
    echo "<<< ${txtgrn}✓${txtrst} Deinstalled node >>>"
    echo
    pauseScript
}

updateNode() {
    echo
    echo "<<< Updating node >>>"
    removeContainer
    createContainer
    startNode
    echo
    echo "<<< ${txtgrn}✓${txtrst} Updated node >>>"
}

updateNodeScreen() {
    clear
    echo
    echo "---------------"
    echo "| Node Update |"
    echo "---------------"
    updateNode
    echo
    pauseScript
}

showNodeLogs() {
    clear
    echo
    echo "-------------------"
    echo "| Node Log Viewer |"
    echo "-------------------"
    echo
    echo "${txtylw}Press Ctrl + C to quit!${txtrst}"
    echo
    pauseScript
    echo
    echo
    # entering a subshell here so killing the logger doesn't exit the main script
    (
        # restoring SIGINT so Ctrl + C works
        trap - SIGINT
        # we don't have to attach because we don't need interactive access
        # this also has the advantage that the node doesn't need to be running
        sudo docker logs fusion --tail=25 -f
        if [ $? -ne 0 ]; then
            echo "${txtred}Failed to show node log${txtrst}"
        fi
        # ignoring SIGINT again
        trap '' SIGINT
    )
    echo
    echo
    pauseScript
}

change_autobuy() {
    local nodetype="$(getCfgValue 'nodeType')"
    if [ "$nodetype" = "minerandlocalgateway" -o "$nodetype" = "efsn" ]; then
        local autobuy="$(getCfgValue 'autobt')"
        local state
        # just making the output a bit nicer
        [ "$autobuy" = "true" ] && state="${txtgrn}enabled${txtrst}" || state="${txtred}disabled${txtrst}"
        echo
        echo "Ticket auto-buy is currently $state"
        # we can't just use the API here as an unexpected node restart would cause problems
        local question="${txtylw}Do you want to change this setting? Doing so will enforce a node update and restart!${txtrst} [Y/n] "
        askToContinue "$question"
        if [ $? -eq 0 ]; then
            echo
            echo "<<< Changing auto-buy setting >>>"
            [ "$autobuy" = "true" ] && autobuy="false" || autobuy="true"
            putCfgValue 'autobt' "$autobuy"
            updateNode
            echo
            echo "<<< ${txtgrn}✓${txtrst} Changed auto-buy setting >>>"
            echo
            pauseScript
        fi
    else
        echo
        echo "This setting is only available for minerandlocalgateway and efsn node types"
        echo
        pauseScript
    fi
}

change_mining() {
    local nodetype="$(getCfgValue 'nodeType')"
    if [ "$nodetype" = "minerandlocalgateway" -o "$nodetype" = "efsn" ]; then
        local mining="$(getCfgValue 'mining')"
        local state
        # just making the output a bit nicer
        [ "$mining" != "false" ] && state="${txtgrn}enabled${txtrst}" || state="${txtred}disabled${txtrst}"
        echo
        echo "Mining of new blocks is currently $state"
        # we can't just use the API here as an unexpected node restart would cause problems
        local question="${txtylw}Do you want to change this setting? Doing so will enforce a node update and restart!${txtrst} [Y/n] "
        askToContinue "$question"
        if [ $? -eq 0 ]; then
            echo
            echo "<<< Changing mining setting >>>"
            # make sure mining remains enabled for old configs
            [ "$mining" != "false" ] && mining="false" || mining="true"
            putCfgValue 'mining' "$mining"
            updateNode
            echo
            echo "<<< ${txtgrn}✓${txtrst} Changed mining setting >>>"
            echo
            pauseScript
        fi
    else
        echo
        echo "This setting is only available for minerandlocalgateway and efsn node types"
        echo
        pauseScript
    fi
}

change_wallet() {
    local nodetype="$(getCfgValue 'nodeType')"
    if [ "$nodetype" = "minerandlocalgateway" -o "$nodetype" = "efsn" ]; then
        local address="$(getCfgValue 'address')"
        echo
        echo "The current staking wallet address is ${txtgrn}$address${txtrst}"
        # we can't just use the API here as an unexpected node restart would cause problems
        local question="${txtylw}Do you want to change this setting? Doing so will enforce a node update and restart!${txtrst} [Y/n] "
        askToContinue "$question"
        if [ $? -eq 0 ]; then
            echo
            echo "<<< Changing staking wallet >>>"
            updateKeystoreFile
            echo "${txtgrn}✓${txtrst} Updated keystore file"
            updateKeystorePass
            echo "${txtgrn}✓${txtrst} Updated keystore password"
            updateNode
            echo
            echo "<<< ${txtgrn}✓${txtrst} Changed staking wallet >>>"
            echo
            pauseScript
        fi
    else
        echo
        echo "This setting is only available for minerandlocalgateway and efsn node types"
        echo
        pauseScript
    fi
}

change_autostart() {
    local state="$(systemctl show fusion -p UnitFileState --value)"
    # just making the output a bit nicer
    local statemsg
    [ "$state" = "enabled" ] && statemsg="${txtgrn}enabled${txtrst}" || statemsg="${txtred}disabled${txtrst}"
    echo
    echo "Node auto-start is currently $statemsg"
    local question="${txtylw}Do you want to change this setting?${txtrst} [Y/n] "
    askToContinue "$question"
    if [ $? -eq 0 ]; then
        echo
        echo "<<< Changing auto-start setting >>>"
        # if auto-start wasn't enabled during installation, state will be empty here
        if [ "$state" != "enabled" ]; then
            # download systemd service unit definition if it doesn't exist yet
            if [ ! -f /etc/systemd/system/fusion.service ]; then
                sudo curl -fsSL "https://raw.githubusercontent.com/FUSIONFoundation/efsn/master/QuickNodeSetup/fusion.service" \
                    -o "/etc/systemd/system/fusion.service"
            fi
            # reload systemd service unit definitions
            sudo systemctl daemon-reload
            # enable the systemd service unit
            sudo systemctl -q enable fusion
        else
            # disable the systemd service unit
            sudo systemctl -q disable fusion
        fi
        echo
        echo "${txtylw}Modified systemd service unit${txtrst}"
        echo
        echo "<<< ${txtgrn}✓${txtrst} Changed auto-start setting >>>"
        echo
        pauseScript
    fi
}

configureNode() {
    while true; do
        clear
        echo
        echo "----------------------"
        echo "| Node Configuration |"
        echo "----------------------"
        echo
        echo "${txtylw}1. Enable/disable ticket auto-buy"
        echo "2. Enable/disable mining of new blocks"
        echo "3. Change staking wallet to be unlocked"
        echo "4. Enable/disable auto-start at boot"
        echo "5. Return to main menu${txtrst}"
        echo
        local input
        read -n1 -r -s -p "Select option [1-5] " input
        case $input in
            1) echo; change_autobuy ;;
            2) echo; warnRetreat && change_mining ;;
            3) echo; warnRetreat && change_wallet ;;
            4) echo; change_autostart ;;
            5) break ;;
            *) echo -e "\n${txtred}Invalid input${txtrst}"; sleep 1 ;;
        esac
    done
}

show_menus_init() {
    clear
    echo
    echo "-----------------------"
    echo "| FUSION Node Manager |"
    echo "-----------------------"
    echo
    echo "${txtylw}1. Install node and dependencies"
    echo "2. Exit to shell${txtrst}"
    echo
}

read_options_init(){
    local input
    read -n1 -r -s -p "Select option [1-2] " input
    case $input in
        1) installNode ;;
        2) echo -e "\n${txtylw}Bye...${txtrst}"; exit 0 ;;
        *) echo -e "\n${txtred}Invalid input${txtrst}"; sleep 1 ;;
    esac
}

show_menus() {
    clear
    echo
    echo "-----------------------"
    echo "| FUSION Node Manager |"
    echo "-----------------------"
    echo
    echo "${txtylw}1. Install node and dependencies"
    echo "2. Update node to current version"
    echo "3. Start the node"
    echo "4. Stop the node"
    echo "5. Deinstall node"
    echo "6. Show node logs"
    echo "7. Configure node"
    echo "8. Exit to shell${txtrst}"
    echo
}

read_options(){
    local input
    read -n1 -r -s -p "Select option [1-8] " input
    case $input in
        1) echo; warnRetreat && installNode ;;
        2) updateNodeScreen ;;
        3) echo; startNode; echo; pauseScript ;;
        4) echo; warnRetreat && stopNode && echo && pauseScript ;;
        5) echo; warnRetreat && deinstallNode ;;
        6) showNodeLogs ;;
        7) configureNode ;;
        8) echo -e "\n${txtylw}Bye...${txtrst}"; exit 0 ;;
        *) echo -e "\n${txtred}Invalid input${txtrst}"; sleep 1 ;;
    esac
}

clear
echo
echo "-----------------------"
echo "| FUSION Node Manager |"
echo "-----------------------"
echo
echo "${txtylw}Initializing script, please wait...${txtrst}"
# make sure we're not running into avoidable problems during setup
sanityChecks
clear

# ignoring some signals to keep the script running
trap '' SIGINT SIGQUIT SIGTSTP
while true; do
    # showing only the install option on initial launch
    if [ ! -f "$BASE_DIR/fusion-node/node.json" ]; then
        show_menus_init
        read_options_init
    else
        show_menus
        read_options
    fi
done
