package main

import (
	"fmt"
	"os"

	"github.com/brimbleapp/brimble-cli/internal/cmd"
)

func main() {
	root := cmd.NewRoot()
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
