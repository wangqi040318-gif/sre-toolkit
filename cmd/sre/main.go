package main

import (
	"fmt"
	"os"

	"github.com/wangqi040318-gif/sre-toolkit/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}
}
