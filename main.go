package main

import (
	"os"

	"github.com/eonyx-core/eonyx/cmd"
)

func main() {
	os.Exit(cmd.ExitCode(cmd.Execute()))
}
