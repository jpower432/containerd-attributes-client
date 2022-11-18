package commands

import (
	"context"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/oci"
	"github.com/opencontainers/runtime-spec/specs-go"

	"github.com/jpower432/runc-attribute-wrapper/aritfact"
)

// NewContainer creates a new container
func NewContainer(ctx context.Context, client *containerd.Client, runOpts RunOptions) (containerd.Container, error) {

	var (
		opts    []oci.SpecOpts
		cOpts   []containerd.NewContainerOpts
		spec    containerd.NewContainerOpts
		user    string
		envFile string
		env     []string
	)

	opts = append(opts, oci.WithDefaultSpec(), oci.WithDefaultUnixDevices)

	if ef := envFile; ef != "" {
		opts = append(opts, oci.WithEnvFile(ef))
	}
	opts = append(opts, oci.WithEnv(env))

	opts = append(opts, withMounts(runOpts))

	snapshotter := ""
	var image aritfact.Image
	i, err := client.ImageService().Get(ctx, runOpts.Reference)
	if err != nil {
		return nil, err
	}

	underlyingImage := containerd.NewImage(client, i)
	image = aritfact.NewImage(client, i, underlyingImage)

	unpacked, err := image.IsUnpacked(ctx, snapshotter)
	if err != nil {
		return nil, err
	}

	if !unpacked {
		if err := image.Unpack(ctx, snapshotter); err != nil {
			return nil, err
		}
	}

	opts = append(opts, WithImageConfig(image))
	cOpts = append(cOpts, containerd.WithSnapshotter(snapshotter))

	cOpts = append(cOpts, containerd.WithNewSnapshot(runOpts.ID, image))

	cOpts = append(cOpts, WithImageStopSignal(image, "SIGTERM"))

	if len(runOpts.ContainerArgs) > 0 {
		opts = append(opts, oci.WithProcessArgs(runOpts.ContainerArgs...))
	}

	if user != "" {
		opts = append(opts, oci.WithUser(user), oci.WithAdditionalGIDs(user))
	}

	if runOpts.TTY {
		opts = append(opts, oci.WithTTY)
	}

	if runOpts.CGroup != "" {
		// NOTE: can be set to "" explicitly for disabling cgroup.
		opts = append(opts, oci.WithCgroup(runOpts.CGroup))
	}

	var s specs.Spec
	spec = containerd.WithSpec(&s, opts...)

	cOpts = append(cOpts, spec)

	// oci.WithImageConfig (WithUsername, WithUserID) depends on access to rootfs for resolving via
	// the /etc/{passwd,group} files. So cOpts needs to have precedence over opts.
	return client.NewContainer(ctx, runOpts.ID, cOpts...)
}
