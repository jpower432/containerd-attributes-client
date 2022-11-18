package commands

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/content"
	"github.com/containerd/containerd/oci"
	"github.com/moby/sys/signal"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/opencontainers/runtime-spec/specs-go"
)

var (
	defaultUnixEnv = []string{
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
	}
)

// Image interface used by some SpecOpt to query image configuration
type Image interface {
	ConfigWithAttributes(ctx context.Context) (ocispec.ImageConfig, error)
	// Config descriptor for the image.
	Config(ctx context.Context) (ocispec.Descriptor, error)
	// ContentStore provides a content store which contains image blob data
	ContentStore() content.Store
}

// WithImageConfig configures the spec to from the configuration of an Image
func WithImageConfig(image Image) oci.SpecOpts {
	return WithImageConfigArgs(image, nil)
}

// WithImageConfigArgs configures the spec to from the configuration of an Image with additional args that
// replaces the CMD of the image
func WithImageConfigArgs(image Image, args []string) oci.SpecOpts {
	return func(ctx context.Context, client oci.Client, c *containers.Container, s *oci.Spec) error {
		config, err := image.ConfigWithAttributes(ctx)
		if err != nil {
			return err
		}

		setProcess(s)
		if s.Linux != nil {
			defaults := config.Env
			if len(defaults) == 0 {
				defaults = defaultUnixEnv
			}
			s.Process.Env = replaceOrAppendEnvValues(defaults, s.Process.Env)
			cmd := config.Cmd
			if len(args) > 0 {
				cmd = args
			}
			s.Process.Args = append(config.Entrypoint, cmd...)

			cwd := config.WorkingDir
			if cwd == "" {
				cwd = "/"
			}
			s.Process.Cwd = cwd
			if config.User != "" {
				if err := oci.WithUser(config.User)(ctx, client, c, s); err != nil {
					return err
				}
				return oci.WithAdditionalGIDs(fmt.Sprintf("%d", s.Process.User.UID))(ctx, client, c, s)
			}
			// we should query the image's /etc/group for additional GIDs
			// even if there is no specified user in the image config
			return oci.WithAdditionalGIDs("root")(ctx, client, c, s)
		} else if s.Windows != nil {
			s.Process.Env = replaceOrAppendEnvValues(config.Env, s.Process.Env)

			// To support Docker ArgsEscaped on Windows we need to combine the
			// image Entrypoint & (Cmd Or User Args) while taking into account
			// if Docker has already escaped them in the image config. When
			// Docker sets `ArgsEscaped==true` in the config it has pre-escaped
			// either Entrypoint or Cmd or both. Cmd should always be treated as
			// arguments appended to Entrypoint unless:
			//
			// 1. Entrypoint does not exist, in which case Cmd[0] is the
			// executable.
			//
			// 2. The user overrides the Cmd with User Args when activating the
			// container in which case those args should be appended to the
			// Entrypoint if it exists.
			//
			// To effectively do this we need to know if the arguments came from
			// the user or if the arguments came from the image config when
			// ArgsEscaped==true. In this case we only want to escape the
			// additional user args when forming the complete CommandLine. This
			// is safe in both cases of Entrypoint or Cmd being set because
			// Docker will always escape them to an array of length one. Thus in
			// both cases it is the "executable" portion of the command.
			//
			// In the case ArgsEscaped==false, Entrypoint or Cmd will contain
			// any number of entries that are all unescaped and can simply be
			// combined (potentially overwriting Cmd with User Args if present)
			// and forwarded the container start as an Args array.
			cmd := config.Cmd
			cmdFromImage := true
			if len(args) > 0 {
				cmd = args
				cmdFromImage = false
			}

			cmd = append(config.Entrypoint, cmd...)
			if len(cmd) == 0 {
				return errors.New("no arguments specified")
			}

			if config.ArgsEscaped && (len(config.Entrypoint) > 0 || cmdFromImage) {
				s.Process.Args = nil
				s.Process.CommandLine = cmd[0]
				if len(cmd) > 1 {
					s.Process.CommandLine += " " + escapeAndCombineArgs(cmd[1:])
				}
			} else {
				s.Process.Args = cmd
				s.Process.CommandLine = ""
			}

			s.Process.Cwd = config.WorkingDir
			s.Process.User = specs.User{
				Username: config.User,
			}
		} else {
			return errors.New("spec does not contain Linux or Windows section")
		}
		return nil
	}
}

// setProcess sets Process to empty if unset
func setProcess(s *oci.Spec) {
	if s.Process == nil {
		s.Process = &specs.Process{}
	}
}

// replaceOrAppendEnvValues returns the defaults with the overrides either
// replaced by env key or appended to the list
func replaceOrAppendEnvValues(defaults, overrides []string) []string {
	cache := make(map[string]int, len(defaults))
	results := make([]string, 0, len(defaults))
	for i, e := range defaults {
		k, _, _ := strings.Cut(e, "=")
		results = append(results, e)
		cache[k] = i
	}

	for _, value := range overrides {
		// Values w/o = means they want this env to be removed/unset.
		k, _, ok := strings.Cut(value, "=")
		if !ok {
			if i, exists := cache[k]; exists {
				results[i] = "" // Used to indicate it should be removed
			}
			continue
		}

		// Just do a normal set/update
		if i, exists := cache[k]; exists {
			results[i] = value
		} else {
			results = append(results, value)
		}
	}

	// Now remove all entries that we want to "unset"
	for i := 0; i < len(results); i++ {
		if results[i] == "" {
			results = append(results[:i], results[i+1:]...)
			i--
		}
	}

	return results
}

func escapeAndCombineArgs(args []string) string {
	panic("not supported")
}

// WithImageStopSignal sets a well-known containerd label (StopSignalLabel)
// on the container for storing the stop signal specified in the OCI image
// config
func WithImageStopSignal(image Image, defaultSignal string) containerd.NewContainerOpts {
	return func(ctx context.Context, _ *containerd.Client, c *containers.Container) error {
		if c.Labels == nil {
			c.Labels = make(map[string]string)
		}
		stopSignal, err := GetOCIStopSignal(ctx, image, defaultSignal)
		if err != nil {
			return err
		}
		c.Labels[containerd.StopSignalLabel] = stopSignal
		return nil
	}
}

// GetOCIStopSignal retrieves the stop signal specified in the OCI image config
func GetOCIStopSignal(ctx context.Context, image Image, defaultSignal string) (string, error) {
	_, err := signal.ParseSignal(defaultSignal)
	if err != nil {
		return "", err
	}
	config, err := image.ConfigWithAttributes(ctx)
	if err != nil {
		return "", err
	}

	if config.StopSignal == "" {
		return defaultSignal, nil
	}

	return config.StopSignal, nil
}
