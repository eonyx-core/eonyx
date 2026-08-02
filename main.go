package main

import (
	"os"

	"eonyx/cmd"
)

func main() {
	os.Exit(cmd.ExitCode(cmd.Execute()))
}
