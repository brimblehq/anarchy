package main

import (
	"os"

	"github.com/brimbleapp/brimble-cli/internal/cmd"
)

func main() {
	os.Exit(cmd.Execute())
}
