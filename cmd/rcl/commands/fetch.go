package commands

import (
	"context"
	"crypto/tls"
	"net/http/httptrace"
	"os"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cmd/ctr/commands"
	"github.com/containerd/containerd/cmd/ctr/commands/content"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/remotes"
	"github.com/containerd/containerd/remotes/docker"
	"github.com/containerd/containerd/remotes/docker/config"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// GetResolver prepares the resolver from the environment and options
func GetResolver(ctx context.Context, runOpts RunOptions) (remotes.Resolver, error) {
	options := docker.ResolverOptions{
		Tracker: commands.PushTracker,
	}

	hostOptions := config.HostOptions{}
	if runOpts.PlainHTTP {
		hostOptions.DefaultScheme = "http"
	}

	defaultTLS := &tls.Config{}
	if runOpts.SkipTLSVerify {
		defaultTLS.InsecureSkipVerify = true
	}

	hostOptions.DefaultTLS = defaultTLS

	options.Hosts = config.ConfigureHosts(ctx, hostOptions)

	return docker.NewResolver(options), nil
}

// NewFetchConfig returns the default FetchConfig from cli flags
func NewFetchConfig(ctx context.Context, runOpts RunOptions) (*content.FetchConfig, error) {
	resolver, err := GetResolver(ctx, runOpts)
	if err != nil {
		return nil, err
	}
	config := &content.FetchConfig{
		Resolver: resolver,
	}
	if !runOpts.Debug {
		config.ProgressOutput = os.Stdout
	}

	return config, nil
}

// Fetch loads all resources into the content store and returns the image
func Fetch(ctx context.Context, client *containerd.Client, ref string, config *content.FetchConfig) (images.Image, error) {
	ongoing := content.NewJobs(ref)

	if config.TraceHTTP {
		ctx = httptrace.WithClientTrace(ctx, commands.NewDebugClientTrace(ctx))
	}

	pctx, stopProgress := context.WithCancel(ctx)
	progress := make(chan struct{})

	go func() {
		if config.ProgressOutput != nil {
			// no progress bar, because it hides some debug logs
			content.ShowProgress(pctx, ongoing, client.ContentStore(), config.ProgressOutput)
		}
		close(progress)
	}()

	h := images.HandlerFunc(func(ctx context.Context, desc ocispec.Descriptor) ([]ocispec.Descriptor, error) {
		if desc.MediaType != images.MediaTypeDockerSchema1Manifest {
			ongoing.Add(desc)
		}
		return nil, nil
	})

	labels := commands.LabelArgs(config.Labels)
	opts := []containerd.RemoteOpt{
		containerd.WithPullLabels(labels),
		containerd.WithResolver(config.Resolver),
		containerd.WithImageHandler(h),
	}
	opts = append(opts, config.RemoteOpts...)

	if config.AllMetadata {
		opts = append(opts, containerd.WithAllMetadata())
	}

	if config.PlatformMatcher != nil {
		opts = append(opts, containerd.WithPlatformMatcher(config.PlatformMatcher))
	} else {
		for _, platform := range config.Platforms {
			opts = append(opts, containerd.WithPlatform(platform))
		}
	}

	img, err := client.Fetch(pctx, ref, opts...)
	stopProgress()
	if err != nil {
		return images.Image{}, err
	}

	<-progress
	return img, nil
}