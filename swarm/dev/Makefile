.PHONY: build cluster test

default: build

build:
	go build -o bin/swarm    github.com/FusionFoundation/efsn/cmd/swarm
	go build -o bin/geth     github.com/FusionFoundation/efsn/cmd/geth
	go build -o bin/bootnode github.com/FusionFoundation/efsn/cmd/bootnode

cluster: build
	scripts/boot-cluster.sh

test:
	go test -v github.com/FusionFoundation/efsn/swarm/...
