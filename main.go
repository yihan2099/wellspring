package main

import (
	"os"

	"github.com/wellspring-cli/wellspring/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
