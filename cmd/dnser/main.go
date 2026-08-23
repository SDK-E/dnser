package main

import (
	"os"

	"github.com/SDK-E/dnser/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
