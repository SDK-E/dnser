package main

import (
	"context"
	"os"
	"runtime/debug"

	"github.com/SDK-E/dnser/internal/cli"
)

const memLimitBytes = 48 << 20

func main() {
	debug.SetMemoryLimit(memLimitBytes)
	args := os.Args[1:]
	if err := cli.Execute(context.Background(), args); err != nil {
		os.Exit(cli.ExitCode(err))
	}
}
