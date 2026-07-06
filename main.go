package main

import (
	"fmt"
	"os"

	"github.com/utopia0107/unity-cli/cmd"
)

var Version = "dev"

func init() {
	cmd.Version = Version
}

func main() {
	if err := cmd.Execute(); err != nil {
		if !cmd.IsReported(err) {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
		os.Exit(int(cmd.ExitCodeFor(err)))
	}
}
