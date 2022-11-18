package commands

import (
	"github.com/containerd/containerd/defaults"
	"github.com/urfave/cli"
)

// TODO(jpower432): Translate into a pflag set
var (
	// SnapshotterFlags are cli flags specifying snapshotter names
	SnapshotterFlags = []cli.Flag{
		cli.StringFlag{
			Name:   "snapshotter",
			Usage:  "snapshotter name. Empty value stands for the default value.",
			EnvVar: "CONTAINERD_SNAPSHOTTER",
		},
	}

	// SnapshotterLabels are cli flags specifying labels which will be added to the new snapshot for container.
	SnapshotterLabels = cli.StringSliceFlag{
		Name:  "snapshotter-label",
		Usage: "labels added to the new snapshot for this container.",
	}

	// LabelFlag is a cli flag specifying labels
	LabelFlag = cli.StringSliceFlag{
		Name:  "label",
		Usage: "labels to attach to the image",
	}

	// RegistryFlags are cli flags specifying registry options
	RegistryFlags = []cli.Flag{
		cli.BoolFlag{
			Name:  "skip-verify,k",
			Usage: "skip SSL certificate validation",
		},
		cli.BoolFlag{
			Name:  "plain-http",
			Usage: "allow connections using plain HTTP",
		},
		cli.StringFlag{
			Name:  "user,u",
			Usage: "user[:password] Registry user and password",
		},
		cli.StringFlag{
			Name:  "refresh",
			Usage: "refresh token for authorization server",
		},
		cli.StringFlag{
			Name: "hosts-dir",
			// compatible with "/etc/docker/certs.d"
			Usage: "Custom hosts configuration directory",
		},
		cli.StringFlag{
			Name:  "tlscacert",
			Usage: "path to TLS root CA",
		},
		cli.StringFlag{
			Name:  "tlscert",
			Usage: "path to TLS client certificate",
		},
		cli.StringFlag{
			Name:  "tlskey",
			Usage: "path to TLS client key",
		},
		cli.BoolFlag{
			Name:  "http-dump",
			Usage: "dump all HTTP request/responses when interacting with container registry",
		},
		cli.BoolFlag{
			Name:  "http-trace",
			Usage: "enable HTTP tracing for registry interactions",
		},
	}

	// ContainerFlags are cli flags specifying container options
	ContainerFlags = []cli.Flag{
		cli.StringFlag{
			Name:  "config,c",
			Usage: "path to the runtime-specific spec config file",
		},
		cli.StringFlag{
			Name:  "cwd",
			Usage: "specify the working directory of the process",
		},
		cli.StringSliceFlag{
			Name:  "env",
			Usage: "specify additional container environment variables (e.g. FOO=bar)",
		},
		cli.StringFlag{
			Name:  "env-file",
			Usage: "specify additional container environment variables in a file(e.g. FOO=bar, one per line)",
		},
		cli.StringSliceFlag{
			Name:  "label",
			Usage: "specify additional labels (e.g. foo=bar)",
		},
		cli.StringSliceFlag{
			Name:  "annotation",
			Usage: "specify additional OCI annotations (e.g. foo=bar)",
		},
		cli.StringSliceFlag{
			Name:  "mount",
			Usage: "specify additional container mount (e.g. type=bind,src=/tmp,dst=/host,options=rbind:ro)",
		},
		cli.BoolFlag{
			Name:  "net-host",
			Usage: "enable host networking for the container",
		},
		cli.BoolFlag{
			Name:  "privileged",
			Usage: "run privileged container",
		},
		cli.BoolFlag{
			Name:  "read-only",
			Usage: "set the containers filesystem as readonly",
		},
		cli.StringFlag{
			Name:  "runtime",
			Usage: "runtime name or absolute path to runtime binary",
			Value: defaults.DefaultRuntime,
		},
		cli.StringFlag{
			Name:  "runtime-config-path",
			Usage: "optional runtime config path",
		},
		cli.BoolFlag{
			Name:  "tty,t",
			Usage: "allocate a TTY for the container",
		},
		cli.StringSliceFlag{
			Name:  "with-ns",
			Usage: "specify existing Linux namespaces to join at container runtime (format '<nstype>:<path>')",
		},
		cli.StringFlag{
			Name:  "pid-file",
			Usage: "file path to write the task's pid",
		},
		cli.IntSliceFlag{
			Name:  "gpus",
			Usage: "add gpus to the container",
		},
		cli.BoolFlag{
			Name:  "allow-new-privs",
			Usage: "turn off OCI spec's NoNewPrivileges feature flag",
		},
		cli.Uint64Flag{
			Name:  "memory-limit",
			Usage: "memory limit (in bytes) for the container",
		},
		cli.StringSliceFlag{
			Name:  "cap-add",
			Usage: "add Linux capabilities (Set capabilities with 'CAP_' prefix)",
		},
		cli.StringSliceFlag{
			Name:  "cap-drop",
			Usage: "drop Linux capabilities (Set capabilities with 'CAP_' prefix)",
		},
		cli.BoolFlag{
			Name:  "seccomp",
			Usage: "enable the default seccomp profile",
		},
		cli.StringFlag{
			Name:  "seccomp-profile",
			Usage: "file path to custom seccomp profile. seccomp must be set to true, before using seccomp-profile",
		},
		cli.StringFlag{
			Name:  "apparmor-default-profile",
			Usage: "enable AppArmor with the default profile with the specified name, e.g. \"cri-containerd.apparmor.d\"",
		},
		cli.StringFlag{
			Name:  "apparmor-profile",
			Usage: "enable AppArmor with an existing custom profile",
		},
		cli.StringFlag{
			Name:  "blockio-config-file",
			Usage: "file path to blockio class definitions. By default class definitions are not loaded.",
		},
		cli.StringFlag{
			Name:  "blockio-class",
			Usage: "name of the blockio class to associate the container with",
		},
		cli.StringFlag{
			Name:  "rdt-class",
			Usage: "name of the RDT class to associate the container with. Specifies a Class of Service (CLOS) for cache and memory bandwidth management.",
		},
		cli.StringFlag{
			Name:  "hostname",
			Usage: "set the container's host name",
		},
		cli.StringFlag{
			Name:  "user,u",
			Usage: "username or user id, group optional (format: <name|uid>[:<group|gid>])",
		},
	}
)