package main

import (
	"context"
	"os"

	"github.com/SDK-E/dnser/internal/cli"
)

func main() {
	if err := cli.Execute(context.Background(), os.Args[1:]); err != nil {
		os.Exit(cli.ExitCode(err))
	}
}
