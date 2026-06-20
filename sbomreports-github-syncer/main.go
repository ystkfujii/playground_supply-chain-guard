package main

import (
	"fmt"
	"os"

	"github.com/ystkfujii/playground_supply-chain-guard/sbomreports-github-syncer/cmd"
)

func main() {
	if err := cmd.NewRootCommand().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
