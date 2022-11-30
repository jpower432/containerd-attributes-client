package main

import (
	"github.com/spf13/cobra"

	"github.com/jpower432/runc-attribute-wrapper/cmd/rcl/commands"
)

func main() {
	rootCmd := commands.NewRootCmd()
	cobra.CheckErr(rootCmd.Execute())
}
