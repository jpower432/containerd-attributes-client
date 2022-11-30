# runc-attributes-wrapper
A containerd client for attribute based chroot construction.

## Required components

# For building 
- Go 1.18+

# For running
- containerd v1.6.10
- uor-client-go

## Steps to test

- Run registry

- Publish a collection with runtime instruction

  - See `uor-client-go` documentation on fork development branch
  [Location](https://github.com/jpower432/client/blob/feat/collection-spec/README.md#publish-content-to-use-with-a-container-runtime)

- Run containerd

[Release location](https://github.com/containerd/containerd/releases/tag/v1.6.10)
```bash
containerd
```

- Launch a container
```bash
rcl run -t localhost:5001/myartifact:latest mycontainer --fetch
```
> The fetch flag will pull down the container images. This is only required on the first run.

- Delete container
```bash
rcl delete mycontainer
```

# TODO

- Add support for linked artifacts
- Allow artifacts to be stored with different file attributes in committed snapshots
- Allow index manifest attribute overrides
