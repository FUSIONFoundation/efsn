#!/bin/bash
# FUSION Foundation
FSN_SCRIPT_VERSION='1.0.0.104'
CWD_DIR="/home/$USER"
MD5_UTC_FILE=
MD5_PWD_FILE=
MD5_JSON_FILE=
nodename=

pause(){
  read -s -p "Press [Enter] key to continue..." fackEnterKey
}

askToContinue(){
    while true
    do
        read -r -p "$1" input
        
        case $input in
            [yY][eE][sS]|[yY])
            # echo "Yes"
        return 1;
        ;;
            [nN][oO]|[nN])
            # echo "No"
        return 0;
                ;;
            *)
        echo "Invalid input..."
        ;;
        esac
    done
}

installDocker(){
	echo "Updating packages"

	curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo apt-key add -
	sudo add-apt-repository "deb [arch=amd64] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable"
	sudo apt-get update
	clear
	echo "✓ Updated packages"
	echo "Installing docker.io"
	sudo apt-get install -y docker.io

    sudo apt-get -y install jq
}

getCfgValue(){
    arg=$1
    cfg_val=""
    if [ "$arg" == "address" ]; then
        keystore_files="$CWD_DIR/fusion-node/data/keystore/UTC.json"
        cfg_val="0x$(cat < ${keystore_files[0]} | jq -r '.address')"
    elif [ "$arg" == "autobt" ]; then
        cfg_files="$CWD_DIR/fusion-node/node.json"
        cfg_val="$(cat < ${cfg_files[0]} | jq -r '.autobt')"
    elif [ "$arg" == "nodeType" ]; then
        cfg_files="$CWD_DIR/fusion-node/node.json"
        cfg_val="$(cat < ${cfg_files[0]} | jq -r '.nodeType')"
    elif [ "$arg" == "nodeName" ]; then
        cfg_files="$CWD_DIR/fusion-node/node.json"
        cfg_val="$(cat < ${cfg_files[0]} | jq -r '.nodeName')"
    fi
    echo "$cfg_val";
}

resetAllDataFiles(){
	clear
	echo "✓ Updated packages"
	echo "✓ Installed docker.io"
	sudo rm -rf "$CWD_DIR/fusion-node/"
	mkdir -p "$CWD_DIR/fusion-node/data/keystore/"
	touch "$CWD_DIR/fusion-node/data/keystore/UTC.json"
	sudo chmod 775 "$CWD_DIR/fusion-node/data/keystore/UTC.json"
    
	# touch "$CWD_DIR/fusion-node/walletaddress.txt"
	touch "$CWD_DIR/fusion-node/password.txt"
	sudo chmod 775 "$CWD_DIR/fusion-node/password.txt"
	touch "$CWD_DIR/fusion-node/nodename.txt"

}

checkMD5(){

    local fileName="$CWD_DIR/fusion-node/data/keystore/UTC.json"
    if [ -f "$fileName" ]; then
        MD5_UTC_FILE=`md5sum ${fileName} | awk '{ print $1 }'`
    fi

    fileName="$CWD_DIR/fusion-node/password.txt"
    if [ -f "$fileName" ]; then
        MD5_PWD_FILE=`md5sum ${fileName} | awk '{ print $1 }'`
    fi

    fileName="$CWD_DIR/fusion-node/node.json"
    if [ -f "$fileName" ]; then
        MD5_JSON_FILE=`md5sum ${fileName} | awk '{ print $1 }'`
    fi

}


createFilesForMiner(){
    
    checkMD5
    
    local cfgFilesUpdated=0
    local oldNodeType=
    if [ ! -z "$MD5_UTC_FILE" ]; then
        nodename="$(getCfgValue 'nodeName')"
        oldNodeType="[$(getCfgValue 'nodeType')]"
    fi

    if [ "$1" -eq "1" ]; then
        echo "Deleting old configuration and chain data!"
        resetAllDataFiles
        pause
    fi
    nodetype=
    echo ""
    echo "Please select node type $oldNodeType: "
    options=("minerandlocalgateway" "efsn" "gateway")
    select opt in "${options[@]}"
    do
        case $opt in
            "minerandlocalgateway")
                nodetype=$opt
                break
                ;;
            "efsn")
                nodetype=$opt
                break
                ;;
            "gateway")
                nodetype=$opt
                break
                ;;
            *) echo "invalid option: $nodetype";;
        esac
    done

    # echo What is your wallet address?
    # read walletaddress
    # echo $walletaddress >> "$CWD_DIR/fusion-node/walletaddress.txt"
    clear
    echo "✓ Updated packages"
	echo "✓ Installed docker.io"
	# echo "✓ Saved wallet address"

    local askToModify=1
    local updateSettings=1
    if [ "$nodetype" = "minerandlocalgateway" ] || [ "$nodetype" = "efsn" ]; then

        if [ -z "$MD5_UTC_FILE" ] && [ -z "$MD5_PWD_FILE" ]; then
            askToModify=0
        else
            if [ "$MD5_UTC_FILE" == "$MD5_PWD_FILE" ]; then
                askToModify=0
            fi
        fi

        if [ "$askToModify" -eq "1" ]; then
            # echo "MD5_UTC_FILE: $MD5_UTC_FILE"
            # echo "MD5_PWD_FILE: $MD5_PWD_FILE"
            question="Would you like to update the keystore and password file? [Y/n] ";
            askToContinue "$question"
            updateSettings=$?
        fi

        if [ "$updateSettings" -eq "1" ]; then
            echo "Paste exact content of your keystore.json"
            read keystorejson
            echo $keystorejson > "$CWD_DIR/fusion-node/data/keystore/UTC.json"
            clear

            echo "✓ Updated packages"
            echo "✓ Installed docker.io"
            # echo "✓ Saved wallet address"
            echo "✓ Saved keystore"
            echo "What is the password of this keystore?"
            read password
            echo $password > "$CWD_DIR/fusion-node/password.txt"
        fi

        clear
        
        echo "✓ Updated packages"
        echo "✓ Installed docker.io"
        # echo "✓ Saved wallet address"
        echo "✓ Saved keystore"
        echo "✓ Saved password"

        question="Would you like your node to auto-buy tickets? [Y/n] ";

        autobuy=false
        askToContinue "$question"
        continueYesNo=$?
        if [ "$continueYesNo" -eq "1" ]; then
            autobuy=true
        fi

        clear
        echo "✓ Updated packages"
        echo "✓ Installed docker.io"
        echo "✓ Saved keystore"
        echo "✓ Saved password"
        echo "✓ Purchase ticket flag set"
    fi

    if [ -z "$MD5_JSON_FILE" ]; then
        askToModify=0
    else
        askToModify=1
    fi
    if [ "$askToModify" -eq "1" ]; then
        question="Would you like to update node name[$nodename]? [Y/n] ";
        askToContinue "$question"
        updateSettings=$?
    fi

    if [ "$updateSettings" -eq "1" ]; then
        echo "What name do you want the node to have on node.fusionnetwork.io (No spaces or special characters)" ?
        read nodename
    fi

    echo -e "{\"nodeName\":\""$nodename"\", \"autobt\":\""$autobuy"\", \"nodeType\":\""$nodetype"\"}"  > "$CWD_DIR/fusion-node/node.json"

    # double check user public address
    if [ "$nodetype" = "minerandlocalgateway" ] || [ "$nodetype" = "efsn" ]; then
        askWalletQuestion
    fi

    clear
}

removeDockerImages(){
    running=$(sudo docker inspect -f "{{.State.Running}}" fusion)

    if [ "$running" == "true" ]; then
        containerid=$(sudo docker ps --filter "name=fusion" -q)
        sudo docker kill $containerid >/dev/null
        sudo docker rm $containerid >/dev/null
    else
        echo "Fusion Docker container does not exist, fresh install."
    fi

    sudo docker stop fusion > /dev/null
    sudo docker rm fusion > /dev/null

    if [[ "$(sudo docker images -q fusionnetwork/efsn 2> /dev/null)" != "" ]]; then
        echo "Image fusionnetwork/efsn removed"
        sudo docker rmi fusionnetwork/efsn >/dev/null
    fi

    if [[ "$(sudo docker images -q fusionnetwork/minerandlocalgateway 2> /dev/null)" != "" ]]; then
        echo "Image fusionnetwork/minerandlocalgateway removed"
        sudo docker rmi fusionnetwork/minerandlocalgateway >/dev/null
    fi

    if [[ "$(sudo docker images -q fusionnetwork/gateway 2> /dev/null)" != "" ]]; then
        echo "Image fusionnetwork/gateway removed"
        sudo docker rmi fusionnetwork/gateway >/dev/null
    fi
}

createDockerContainer(){
    # check the installed node type
    nodetype="$(getCfgValue 'nodeType')"
    autobt="$(getCfgValue 'autobt')"
    walletaddress="$(getCfgValue 'address')"
    nodename="$(getCfgValue 'nodeName')"
    
    if [ "$autobt" == "true" ]; then
        # turn autobuy on
        autobt="--autobt"
    else
        # turn autobuy off
        autobt=""
    fi

    ethstats=""
    if [ ${#nodename} -gt 0 ]; then 
        ethstats=" --ethstats $nodename:fsnMainnet@node.fusionnetwork.io"
    else
        ethstats=""
    fi

    if [ "$nodetype" == "minerandlocalgateway" ]; then
        sudo docker pull fusionnetwork/minerandlocalgateway:latest

        sudo docker create --name fusion -it --restart unless-stopped \
	    -p 127.0.0.1:9000:9000 -p 127.0.0.1:9001:9001 -p 40408:40408 \
            -v "$CWD_DIR/fusion-node":/fusion-node \
            fusionnetwork/minerandlocalgateway \
            -u "$walletaddress" \
            -e "$nodename" "$autobt"

    elif [ "$nodetype" == "gateway" ]; then
        sudo docker pull fusionnetwork/gateway:latest

        sudo docker create --name fusion -it --restart unless-stopped \
            -p 40408:40408/tcp -p 40408:40408/udp -p 127.0.0.1:9000:9000 -p 127.0.0.1:9001:9001/tcp \
            -v "$CWD_DIR/fusion-node":/fusion-node \
            fusionnetwork/gateway \
            $ethstats

    elif [ "$nodetype" == "efsn" ]; then
    	
        sudo docker pull fusionnetwork/efsn:latest

        sudo docker create --name fusion -it --restart unless-stopped \
            -p 40408:40408/tcp -p 40408:40408/udp -p 127.0.0.1:9001:9001/tcp \
            -v "$CWD_DIR/fusion-node":/fusion-node \
            fusionnetwork/efsn \
            -u "$walletaddress" \
            -e "$nodename" "$autobt"
    else
        echo "Invalid node type! $nodetype"
        exit 10
    fi
}

askWalletQuestion(){
    walletaddress="$(getCfgValue 'address')"

    question="Is this your wallet address: $walletaddress [Y/n]?"

    askToContinue "$question"
    
    continueYesNo="$?"

    if [ "$continueYesNo" -eq "0" ]; then
        echo "Please check your JSON keystore file, exiting!"; 
        exit 11
    fi
}

installNode(){
    
    clear
	echo "NOTE: If you have an existing node installed on $CWD_DIR/fusion-node it will be wiped!!"
	
    question="Would you like to contiue? [Y/n] ";

    askToContinue "$question"

    continueYesNo=$?

    if [ "$continueYesNo" -eq "0" ]; then
        echo "Terminating installation!"; 
        exit 0
    fi

    installDocker

    local resetFiles=1
    createFilesForMiner $resetFiles

    # clear previously installed fusionnetwork docker images
    removeDockerImages

    clear
    echo "✓ Updated packages"
	echo "✓ Installed docker.io"
	# echo "✓ Saved wallet address"
	echo "✓ Saved keystore"
	echo "✓ Saved password"
	echo "✓ Saved node monitor name"
	echo "  Starting node"

    createDockerContainer

    startNode
    
    clear
    echo "✓ Updated packages"
	echo "✓ Installed docker.io"
	# echo "✓ Saved wallet address"
	echo "✓ Saved keystore"
	echo "✓ Saved password"
	echo "✓ Saved node monitor name"
	echo "✓ Node started in background"
        
    pause
}

startNode(){
	echo "Starting node"
	sudo docker start fusion
    if [ $? -eq 0 ]; then
    	echo "Node successfully started"
    else
    	echo "Node failed to start"
    fi
}

stopNode(){
	echo "Stopping node"
	sudo docker stop fusion
    if [ $? -eq 0 ]; then
    	echo "Node successfully stopped"
    else
    	echo "Failed to stop the node!"
    fi
    pause
}

updateNode(){
	echo "Updating node"
	echo "Stopping and removing all instances"

    removeDockerImages
    
    clear
    
    createDockerContainer

    startNode

    pause
}

viewNode(){

    #echo "Attaching to Node container"
    #sudo docker attach fusion
    #if [ $? -ne 0 ]; then
    #    echo "Failed to attach!"
    #fi
    #pause

    echo
    echo "-------------------"
    echo "| Node Log Viewer |"
    echo "-------------------"
    echo
    echo "Press Ctrl + C to quit!"
    echo
    pause
    echo
    echo
    # entering a subshell here so killing the logger doesn't exit the main script
    (
        # restoring SIGINT so Ctrl + C works
        trap - SIGINT
        # we don't have to attach because we don't need interactive access
        sudo docker logs fusion --tail=25 -f
        if [ $? -ne 0 ]; then
            echo "Failed to show node log"
        fi
        # ignoring SIGINT again
        trap '' SIGINT
    )
    echo
    echo
    pause
}


configNode(){
    
    local resetFiles=0
    local updating=0
    createFilesForMiner $resetFiles

    local org_md5_utc_val=$MD5_UTC_FILE
    local org_md5_pwd_val=$MD5_PWD_FILE
    local org_md5_json_val=$MD5_JSON_FILE

    checkMD5

    # check if settings were updated
    if [ "$org_md5_utc_val" != "$MD5_UTC_FILE" ]; then
        updating=1
    fi
    if [ "$org_md5_pwd_val" != "$MD5_PWD_FILE" ]; then
        updating=1
    fi

    if [ "$org_md5_json_val" != "$MD5_JSON_FILE" ]; then
        updating=1
    fi


    if [ "$updating" -eq "1" ]; then
        updating=1
    fi

    # echo "org_md5_utc_val=$org_md5_utc_val, MD5_UTC_FILE=$MD5_UTC_FILE"
    # echo "org_md5_pwd_val=$org_md5_pwd_val, MD5_PWD_FILE=$MD5_PWD_FILE"
    # echo "org_md5_json_val=$org_md5_json_val, MD5_JSON_FILE=$MD5_JSON_FILE"
    if [ "$updating" -eq "1" ]; then
        # restart if config was updated
        removeDockerImages
        clear
        createDockerContainer
        startNode
    else
        echo "Using original node configuration!"
        pause
    fi

}

show_menus() {
	clear
	echo "----------------------------"
	echo " FUSION Node Manager v$FSN_SCRIPT_VERSION"
	echo "----------------------------"
	echo "1. Install all prerequisites and node"
	echo "2. Update node"
	echo "3. Start node"
	echo "4. Stop node"
	echo "5. View node (only when started)"
	echo "6. Configure node"
	echo "7. Exit"
}

read_options(){
	local choice
	read -n1 -p "Enter choice [1-7] " choice
	case $choice in
		1) installNode ;;
		2) updateNode ;;
		3) startNode ;;
		4) stopNode ;;
		5) viewNode ;;
		6) configNode ;;
		7) exit 0 ;;
		*) echo -e "${RED}Invalid choice.${STD}"
	esac
}

trap '' SIGINT SIGQUIT SIGTSTP
while true
do
	show_menus
	read_options
done

#/* vim: set ts=4 sts=4 sw=4 et : */
