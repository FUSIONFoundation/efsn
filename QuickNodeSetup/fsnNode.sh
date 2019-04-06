#!/bin/bash
# FUSION Foundation

pause(){
  read -p "Press [Enter] key to continue..." fackEnterKey
}

installNode(){
    clear
	echo "NOTE: If you have an existing node installed on /home/$USER/fusion-node it will be wiped!!"
	pause
	echo "Updating packages"
	sudo apt-get update > dev/null
	clear
	echo "✓ Updated packages"
	echo "Installing docker.io"

	sudo apt-get install -y docker.io
	clear
	echo "✓ Updated packages"
	echo "✓ Installed docker.io"
	sudo rm -rf "/home/$USER/fusion-node/"
	mkdir -p "/home/$USER/fusion-node/data/keystore/"
	touch "/home/$USER/fusion-node/data/keystore/UTC.json"
	sudo chmod 775 "/home/$USER/fusion-node/data/keystore/UTC.json"
	touch "/home/$USER/fusion-node/walletaddress.txt"
	touch "/home/$USER/fusion-node/password.txt"
	sudo chmod 775 "/home/$USER/fusion-node/password.txt"
	touch "/home/$USER/fusion-node/nodename.txt"
    echo What is your wallet address?
    read walletaddress
    echo $walletaddress >> "/home/$USER/fusion-node/walletaddress.txt"
    clear
    echo "✓ Updated packages"
	echo "✓ Installed docker.io"
	echo "✓ Saved wallet address"
	echo Paste exact content of your keystore.json
    read keystorejson
    echo $keystorejson >> "/home/$USER/fusion-node/data/keystore/UTC.json"
    clear
    echo "✓ Updated packages"
	echo "✓ Installed docker.io"
	echo "✓ Saved wallet address"
	echo "✓ Saved keystore"
	echo What is the password of this keystore?
    read password
    echo $password >> "/home/$USER/fusion-node/password.txt"
    clear
    echo "✓ Updated packages"
	echo "✓ Installed docker.io"
	echo "✓ Saved wallet address"
	echo "✓ Saved keystore"
	echo "✓ Saved password"
	echo "What name do you want the node to have on node.fusionnetwork.io" ?
    read nodename
    echo $nodename >> "/home/$USER/fusion-node/nodename.txt"
    clear
    sudo docker kill $(sudo docker ps -q > dev/null) >/dev/null
    sudo docker rm $(sudo docker ps -a -q  > dev/null) >/dev/null
    sudo docker stop fusion >/dev/null
    sudo docker rm fusion >/dev/null
    sudo docker rmi fusionnetwork/efsn2 >/dev/null
    clear
    echo "✓ Updated packages"
	echo "✓ Installed docker.io"
	echo "✓ Saved wallet address"
	echo "✓ Saved keystore"
	echo "✓ Saved password"
	echo "✓ Saved node monitor name"
	echo "  Starting node"
    sudo docker create --name fusion -t --restart unless-stopped \
-p 40408:40408/tcp -p 40408:40408/udp -p 127.0.0.1:9001:9001/tcp \
-v "/home/$USER/fusion-node":/fusion-node \
   fusionnetwork/efsn2 \
-u "$walletaddress" \
-e "$nodename"
    sudo docker start fusion
    clear
    echo "✓ Updated packages"
	echo "✓ Installed docker.io"
	echo "✓ Saved wallet address"
	echo "✓ Saved keystore"
	echo "✓ Saved password"
	echo "✓ Saved node monitor name"
	echo "✓ Node started in background"
        pause
}

startNode(){
	echo "Starting node"
	sudo docker start fusion
	echo "Node successfully started"
    pause
}

stopNode(){
	echo "Stopping node"
	sudo docker stop fusion
	echo "Node successfully stopped"
        pause
}

updateNode(){
	echo "Updating node"
	echo "Stopping and removing all instances"
    sudo docker kill $(sudo docker ps -q) >/dev/null
    sudo docker rm $(sudo docker ps -a -q) >/dev/null
    sudo docker stop fusion >/dev/null
    sudo docker rm fusion > /dev/null
    clear
	sudo docker pull fusionnetwork/efsn2:latest
	walletaddress=`cat /home/$USER/fusion-node/walletaddress.txt`
	nodename=`cat /home/$USER/fusion-node/nodename.txt`
	echo "Creating new container"
	sudo docker create --name fusion -t --restart unless-stopped \
-p 40408:40408/tcp -p 40408:40408/udp -p 127.0.0.1:9001:9001/tcp \
-v "/home/$USER/fusion-node":/fusion-node \
   fusionnetwork/efsn2 \
-u $walletaddress \
-e $nodename
	echo "Starting node"
    sudo docker start fusion
    echo "Node successfully started"

        pause
}

viewNode(){
	echo "Attaching to Node container"
	sudo docker attach fusion
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
