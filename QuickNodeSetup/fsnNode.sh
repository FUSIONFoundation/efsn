#!/bin/bash
# FUSION Foundation

CWD_DIR="/home/$USER"

pause(){
  read -p "Press [Enter] key to continue..." fackEnterKey
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

createFilesForMiner(){

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

    nodetype=
    echo 'Please select node type: '
    options=("minerandlocalgateway2" "efsn2" "gateway2")
    select opt in "${options[@]}"
    do
        case $opt in
            "minerandlocalgateway2")
                nodetype=$opt
                break
                ;;
            "efsn2")
                nodetype=$opt
                break
                ;;
            "gateway2")
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

    if [ "$nodetype" = "minerandlocalgateway2" ] || [ "$nodetype" = "efsn2" ]; then

        echo Paste exact content of your keystore.json
        read keystorejson
        echo $keystorejson >> "$CWD_DIR/fusion-node/data/keystore/UTC.json"
        clear

        echo "✓ Updated packages"
        echo "✓ Installed docker.io"
        # echo "✓ Saved wallet address"
        echo "✓ Saved keystore"
        echo What is the password of this keystore?
        read password
        echo $password >> "$CWD_DIR/fusion-node/password.txt"
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

	echo "What name do you want the node to have on node.fusionnetwork.io (No spaces or special characters)" ?
    read nodename

    echo -e "{\"nodeName\":\""$nodename"\", \"autobt\":\""$autobuy"\", \"nodeType\":\""$nodetype"\"}"  >> "$CWD_DIR/fusion-node/node.json"

    # double check user public address
    if [ "$nodetype" = "minerandlocalgateway2" ] || [ "$nodetype" = "efsn2" ]; then
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

    if [[ "$(sudo docker images -q fusionnetwork/efsn2 2> /dev/null)" != "" ]]; then
        echo "Image fusionnetwork/efsn2 removed"
        sudo docker rmi fusionnetwork/efsn2 >/dev/null
    fi

    if [[ "$(sudo docker images -q fusionnetwork/minerandlocalgateway2 2> /dev/null)" != "" ]]; then
        echo "Image fusionnetwork/minerandlocalgateway2 removed"
        sudo docker rmi fusionnetwork/minerandlocalgateway2 >/dev/null
    fi

    if [[ "$(sudo docker images -q fusionnetwork/gateway2 2> /dev/null)" != "" ]]; then
        echo "Image fusionnetwork/gateway2 removed"
        sudo docker rmi fusionnetwork/gateway2 >/dev/null
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
        ethstats=" --ethstats $nodename:FusionPSN2v4@node.fusionnetwork.io"
    else
        ethstats=""
    fi

    if [ "$nodetype" == "minerandlocalgateway2" ]; then
        sudo docker pull fusionnetwork/minerandlocalgateway2:latest

        sudo docker create --name fusion -it -p 127.0.0.1:9000:9000 -p 127.0.0.1:9001:9001 -p 40408:40408 \
            -v "$CWD_DIR/fusion-node":/fusion-node \
            fusionnetwork/minerandlocalgateway2 \
            -u "$walletaddress" \
            -e "$nodename" "$autobt"

    elif [ "$nodetype" == "gateway2" ]; then
        sudo docker pull fusionnetwork/gateway2:latest

        sudo docker create --name fusion -it --restart unless-stopped \
            -p 40408:40408/tcp -p 40408:40408/udp -p 127.0.0.1:9000:9000 -p 127.0.0.1:9001:9001/tcp \
            -v "$CWD_DIR/fusion-node":/fusion-node \
            fusionnetwork/gateway2 \
            $ethstats

    elif [ "$nodetype" == "efsn2" ]; then
    	
        sudo docker pull fusionnetwork/efsn2:latest

        sudo docker create --name fusion -it --restart unless-stopped \
            -p 40408:40408/tcp -p 40408:40408/udp -p 127.0.0.1:9001:9001/tcp \
            -v "$CWD_DIR/fusion-node":/fusion-node \
            fusionnetwork/efsn2 \
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

    createFilesForMiner

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

	echo "Attaching to Node container"
	sudo docker attach fusion
    if [ $? -ne 0 ]; then
    	echo "Failed to attach!"
    fi
    pause
}

show_menus() {
	clear
	echo "---------------------"
	echo " FUSION Node Manager"
	echo "---------------------"
	echo "1. Install all prerequisites and node"
	echo "2. Update node"
	echo "3. Start node"
	echo "4. Stop node"
	echo "5. View node (only when started)"
	echo "6. Exit"
}

read_options(){
	local choice
	read -p "Enter choice [ 1 - 6] " choice
	case $choice in
		1) installNode ;;
		2) updateNode ;;
		3) startNode ;;
		4) stopNode ;;
		5) viewNode ;;
		6) exit 0;;
		*) echo -e "${RED}Invalid choice.${STD}"
	esac
}

trap '' SIGINT SIGQUIT SIGTSTP
while true
do
	show_menus
	read_options
done
