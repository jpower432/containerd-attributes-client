package commands

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

type RootOptions struct {
	genericclioptions.IOStreams
	Address string
}

// NewRootCmd creates a new cobra.Command for the command root.
func NewRootCmd() *cobra.Command {
	o := RootOptions{}

	o.IOStreams = genericclioptions.IOStreams{
		In:     os.Stdin,
		Out:    os.Stdout,
		ErrOut: os.Stderr,
	}
	cmd := &cobra.Command{
		Use:           filepath.Base(os.Args[0]),
		Short:         "A containerd client",
		SilenceErrors: false,
		SilenceUsage:  false,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			socket := os.Getenv("CONTAINERD-SOCKET")
			if socket == "" {
				socket = "/run/containerd/containerd.sock"
			}
			o.Address = socket
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(NewRunCmd(&o))
	cmd.AddCommand(NewDeleteCmd(&o))

	return cmd
}
