package main

import (
	"os"

	"github.com/brimblehq/anarchy/internal/cmd"
)

func main() {
	os.Exit(cmd.Execute())
}
