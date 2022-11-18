package commands

import (
	"context"
	"fmt"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/log"
	"github.com/containerd/containerd/namespaces"
	"github.com/spf13/cobra"
)

type DeleteOptions struct {
	*RootOptions
	ID string
}

// NewDeleteCmd creates a new cobra.Command for the delete subcommand.
func NewDeleteCmd(options *RootOptions) *cobra.Command {
	o := DeleteOptions{
		RootOptions: options,
	}

	cmd := &cobra.Command{
		Use:           "delete ID",
		SilenceErrors: false,
		SilenceUsage:  false,
		Args:          cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			cobra.CheckErr(o.Complete(args))
			cobra.CheckErr(o.Validate())
			cobra.CheckErr(o.Run(cmd.Context()))
		},
	}

	return cmd
}

func (o *DeleteOptions) Complete(args []string) error {
	o.ID = args[0]
	return nil
}

func (o *DeleteOptions) Validate() error {
	return nil
}

func (o *DeleteOptions) Run(ctx context.Context) error {
	ctx = namespaces.WithNamespace(ctx, "default")
	var exitErr error
	client, ctx, cancel, err := NewClient(ctx, o.Address)
	if err != nil {
		return err
	}
	defer cancel()
	var deleteOpts []containerd.DeleteOpts
	if err := deleteContainer(ctx, client, o.ID, deleteOpts...); err != nil {
		if exitErr == nil {
			exitErr = err
		}
		log.G(ctx).WithError(err).Errorf("failed to delete container %q", o.ID)
	}
	return exitErr
}

func deleteContainer(ctx context.Context, client *containerd.Client, id string, opts ...containerd.DeleteOpts) error {
	container, err := client.LoadContainer(ctx, id)
	if err != nil {
		return err
	}
	task, err := container.Task(ctx, cio.Load)
	if err != nil {
		return container.Delete(ctx, opts...)
	}
	status, err := task.Status(ctx)
	if err != nil {
		return err
	}
	if status.Status == containerd.Stopped || status.Status == containerd.Created {
		if _, err := task.Delete(ctx); err != nil {
			return err
		}
		return container.Delete(ctx, opts...)
	}
	return fmt.Errorf("cannot delete a non stopped container: %v", status)
}

// NewClient returns a new containerd client
func NewClient(ctx context.Context, address string, opts ...containerd.ClientOpt) (*containerd.Client, context.Context, context.CancelFunc, error) {
	client, err := containerd.New(address, opts...)
	if err != nil {
		return nil, nil, nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	return client, ctx, cancel, nil
}
