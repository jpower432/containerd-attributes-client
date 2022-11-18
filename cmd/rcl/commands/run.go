package commands

import (
	"context"
	"encoding/csv"
	"fmt"
	"strings"

	"github.com/containerd/console"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/cmd/ctr/commands"
	"github.com/containerd/containerd/cmd/ctr/commands/tasks"
	"github.com/containerd/containerd/containers"
	clabels "github.com/containerd/containerd/labels"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/oci"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/urfave/cli"
)

// RunOptions configure options when pulling image references and running
// containers
type RunOptions struct {
	*RootOptions
	ID            string
	Reference     string
	Remove        bool
	NullIO        bool
	LogURI        string
	Detach        bool
	CGroup        string
	Platform      string
	CNI           bool
	FIFODir       string
	Mounts        []string
	ContainerArgs []string
	TTY           bool
	Debug         bool
	PlainHTTP     bool
	SkipTLSVerify bool
	// Fetch the image from remote
	Fetch bool
}

// NewRunCmd creates a new cobra.Command for the run subcommand.
func NewRunCmd(options *RootOptions) *cobra.Command {
	o := RunOptions{
		RootOptions: options,
	}

	cmd := &cobra.Command{
		Use:           "run IMG",
		SilenceErrors: false,
		SilenceUsage:  false,
		Args:          cobra.MinimumNArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			cobra.CheckErr(o.Complete(args))
			cobra.CheckErr(o.Validate())
			cobra.CheckErr(o.Run(cmd.Context()))
		},
	}

	cmd.Flags().BoolVar(&o.Remove, "rm", o.Remove, "remove container after running")
	cmd.Flags().BoolVar(&o.NullIO, "null-io", o.NullIO, "send all IO to /dev/null")
	cmd.Flags().StringVar(&o.LogURI, "log-uri", o.LogURI, "log uri")
	cmd.Flags().BoolVar(&o.CNI, "cni", o.CNI, "enable cni networking for the container")
	cmd.Flags().BoolVarP(&o.Detach, "detach", "d", o.Detach, "detach from the task after it has started execution")
	cmd.Flags().StringVar(&o.Platform, "platform", o.Platform, "run image for specific platform")
	cmd.Flags().StringVar(&o.CGroup, "cgroup", o.CGroup, "cgroup path (To disable use of cgroup, set to \"\" explicitly)")
	cmd.Flags().StringVar(&o.FIFODir, "fifo-dir", o.FIFODir, "directory used for storing IO FIFOs")
	cmd.Flags().BoolVarP(&o.TTY, "tty", "t", o.TTY, "allocate a TTY for the container")
	cmd.Flags().BoolVar(&o.Debug, "debug", o.Debug, "debug moder")
	cmd.Flags().BoolVar(&o.PlainHTTP, "plain-http", o.PlainHTTP, "use HTTP to connect to registries")
	cmd.Flags().BoolVar(&o.SkipTLSVerify, "skip-tls-verify", o.SkipTLSVerify, "skip TLS validation when connecting to registries")
	cmd.Flags().BoolVar(&o.Fetch, "fetch", o.Fetch, "fetch the image reference from remote registry")

	return cmd
}

func (o *RunOptions) Complete(args []string) error {
	o.Reference = args[0]
	o.ID = args[1]
	if len(args) > 2 {
		o.ContainerArgs = args[2:]
	}
	return nil
}

func (o *RunOptions) Validate() error {
	return nil
}

func (o *RunOptions) Run(ctx context.Context) error {
	ctx = namespaces.WithNamespace(ctx, "default")
	client, ctx, cancel, err := NewClient(ctx, o.Address)
	if err != nil {
		return err
	}
	defer cancel()

	ctx, done, err := client.WithLease(ctx)
	if err != nil {
		return err
	}
	defer done(ctx)

	if o.Fetch {
		config, err := NewFetchConfig(ctx, *o)
		if err != nil {
			return err
		}

		_, err = Fetch(ctx, client, o.Reference, config)
		if err != nil {
			return err
		}
	}

	container, err := NewContainer(ctx, client, *o)
	if err != nil {
		return err
	}
	if o.Remove && !o.Detach {
		defer container.Delete(ctx, containerd.WithSnapshotCleanup)
	}
	var con console.Console
	if o.TTY {
		con = console.Current()
		defer con.Reset()
		if err := con.SetRaw(); err != nil {
			return err
		}
	}

	opts := o.getNewTaskOpts()
	ioOpts := []cio.Opt{cio.WithFIFODir(o.FIFODir)}
	task, err := tasks.NewTask(ctx, client, container, "", con, o.NullIO, o.LogURI, ioOpts, opts...)
	if err != nil {
		return err
	}

	var statusC <-chan containerd.ExitStatus
	if !o.Detach {
		defer func() {
			task.Delete(ctx)
		}()

		if statusC, err = task.Wait(ctx); err != nil {
			return err
		}
	}

	if err := task.Start(ctx); err != nil {
		return err
	}
	if o.Detach {
		return nil
	}
	if o.TTY {
		if err := tasks.HandleConsoleResize(ctx, task, con); err != nil {
			logrus.WithError(err).Error("console resize")
		}
	} else {
		sigc := commands.ForwardAllSignals(ctx, task)
		defer commands.StopCatch(sigc)
	}
	status := <-statusC
	code, _, err := status.Result()
	if err != nil {
		return err
	}
	if _, err := task.Delete(ctx); err != nil {
		return err
	}
	if code != 0 {
		return cli.NewExitError("", int(code))
	}
	return nil
}

// buildLabel builds the labels from command line labels and the image labels
func buildLabels(cmdLabels, imageLabels map[string]string) map[string]string {
	labels := make(map[string]string)
	for k, v := range imageLabels {
		if err := clabels.Validate(k, v); err == nil {
			labels[k] = v
		} else {
			// In case the image label is invalid, we output a warning and skip adding it to the
			// container.
			logrus.WithError(err).Warnf("unable to add image label with key %s to the container", k)
		}
	}
	// labels from the command line will override image and the initial image config labels
	for k, v := range cmdLabels {
		labels[k] = v
	}
	return labels
}

func withMounts(options RunOptions) oci.SpecOpts {
	return func(ctx context.Context, client oci.Client, container *containers.Container, s *specs.Spec) error {
		mounts := make([]specs.Mount, 0)
		for _, mount := range options.Mounts {
			m, err := parseMountFlag(mount)
			if err != nil {
				return err
			}
			mounts = append(mounts, m)
		}
		return oci.WithMounts(mounts)(ctx, client, container, s)
	}
}

// parseMountFlag parses a mount string in the form "type=foo,source=/path,destination=/target,options=rbind:rw"
func parseMountFlag(m string) (specs.Mount, error) {
	mount := specs.Mount{}
	r := csv.NewReader(strings.NewReader(m))

	fields, err := r.Read()
	if err != nil {
		return mount, err
	}

	for _, field := range fields {
		key, val, ok := strings.Cut(field, "=")
		if !ok {
			return mount, fmt.Errorf("invalid mount specification: expected key=val")
		}

		switch key {
		case "type":
			mount.Type = val
		case "source", "src":
			mount.Source = val
		case "destination", "dst":
			mount.Destination = val
		case "options":
			mount.Options = strings.Split(val, ":")
		default:
			return mount, fmt.Errorf("mount option %q not supported", key)
		}
	}

	return mount, nil
}

func (o *RunOptions) getNewTaskOpts() []containerd.NewTaskOpts {
	var (
		tOpts []containerd.NewTaskOpts
	)

	return tOpts
}
