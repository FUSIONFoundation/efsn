// Copyright 2016 The go-ethereum Authors
// This file is part of go-ethereum.
//
// go-ethereum is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// go-ethereum is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with go-ethereum. If not, see <http://www.gnu.org/licenses/>.

package main

import (
	"github.com/FusionFoundation/efsn/v4/cmd/utils"
	"github.com/FusionFoundation/efsn/v4/console"
	"github.com/FusionFoundation/efsn/v4/internal/flags"
	"github.com/FusionFoundation/efsn/v4/node"
	"github.com/FusionFoundation/efsn/v4/rpc"
	"github.com/urfave/cli/v2"
	"strings"
)

var (
	consoleFlags = []cli.Flag{utils.JSpathFlag, utils.ExecFlag, utils.PreloadJSFlag}

	consoleCommand = &cli.Command{
		Action: localConsole,
		Name:   "console",
		Usage:  "Start an interactive JavaScript environment",
		Flags:  flags.Merge(nodeFlags, rpcFlags, consoleFlags),
		Description: `
The Geth console is an interactive shell for the JavaScript runtime environment
which exposes a node admin interface as well as the Ðapp JavaScript API.
See https://geth.ethereum.org/docs/interface/javascript-console.`,
	}

	attachCommand = &cli.Command{
		Action:    remoteConsole,
		Name:      "attach",
		Usage:     "Start an interactive JavaScript environment (connect to node)",
		ArgsUsage: "[endpoint]",
		Flags:     flags.Merge([]cli.Flag{utils.DataDirFlag}, consoleFlags),
		Description: `
The Geth console is an interactive shell for the JavaScript runtime environment
which exposes a node admin interface as well as the Ðapp JavaScript API.
See https://geth.ethereum.org/docs/interface/javascript-console.
This command allows to open a console on a running geth node.`,
	}
)

// localConsole starts a new efsn node, attaching a JavaScript console to it at the
// same time.
func localConsole(ctx *cli.Context) error {
	// Create and start the node based on the CLI flags
	prepare(ctx)
	stack, backend := makeFullNode(ctx)
	startNode(ctx, stack, backend)
	defer stack.Close()

	// Attach to the newly started node and start the JavaScript console
	client, err := stack.Attach()
	if err != nil {
		utils.Fatalf("Failed to attach to the inproc efsn: %v", err)
	}
	config := console.Config{
		DataDir: utils.MakeDataDir(ctx),
		DocRoot: ctx.String(utils.JSpathFlag.Name),
		Client:  client,
		Preload: utils.MakeConsolePreloads(ctx),
	}
	console, err := console.New(config)
	if err != nil {
		utils.Fatalf("Failed to start the JavaScript console: %v", err)
	}
	defer console.Stop(false)

	// If only a short execution was requested, evaluate and return
	if script := ctx.String(utils.ExecFlag.Name); script != "" {
		console.Evaluate(script)
		return nil
	}

	// Track node shutdown and stop the console when it goes down.
	// This happens when SIGTERM is sent to the process.
	go func() {
		stack.Wait()
		//console.StopInteractive()
	}()

	// Print the welcome screen and enter interactive mode.
	console.Welcome()
	console.Interactive()
	return nil
}

// remoteConsole will connect to a remote efsn instance, attaching a JavaScript
// console to it.
func remoteConsole(ctx *cli.Context) error {
	if ctx.Args().Len() > 1 {
		utils.Fatalf("invalid command-line: too many arguments")
	}

	// Attach to a remotely running efsn instance and start the JavaScript console
	endpoint := ctx.Args().First()
	if endpoint == "" {
		cfg := defaultNodeConfig()
		utils.SetDataDir(ctx, &cfg)
		endpoint = cfg.IPCEndpoint()
	}
	client, err := dialRPC(endpoint)
	if err != nil {
		utils.Fatalf("Unable to attach to remote efsn: %v", err)
	}
	config := console.Config{
		DataDir: utils.MakeDataDir(ctx),
		DocRoot: ctx.String(utils.JSpathFlag.Name),
		Client:  client,
		Preload: utils.MakeConsolePreloads(ctx),
	}
	console, err := console.New(config)
	if err != nil {
		utils.Fatalf("Failed to start the JavaScript console: %v", err)
	}
	defer console.Stop(false)

	if script := ctx.String(utils.ExecFlag.Name); script != "" {
		console.Evaluate(script)
		return nil
	}

	// Otherwise print the welcome screen and enter interactive mode
	console.Welcome()
	console.Interactive()
	return nil
}

// dialRPC returns a RPC client which connects to the given endpoint.
// The check for empty endpoint implements the defaulting logic
// for "efsn attach" with no argument.
func dialRPC(endpoint string) (*rpc.Client, error) {
	if endpoint == "" {
		endpoint = node.DefaultIPCEndpoint(clientIdentifier)
	} else if strings.HasPrefix(endpoint, "rpc:") || strings.HasPrefix(endpoint, "ipc:") {
		// Backwards compatibility with efsn < 1.5 which required
		// these prefixes.
		endpoint = endpoint[4:]
	}
	return rpc.Dial(endpoint)
}
