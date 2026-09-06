package main

import (
	"os"

	"ai-identity-manager/internal/cli"
)

func main() {
	cli.MaybeHideConsole(os.Args[1:])
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
