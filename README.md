# runc-attributes-wrapper
A containerd client for attribute based chroot construction.

## Required components

# For building 
- Go 1.18+

# For running
- containerd v1.6.10
- uor-client-go (version TBD)
- CNCF distribution registry UOR fork (more information TBD)

## Steps to test

- Run registry

- Publish a collection with runtime instruction

- Run containerd

```bash
containerd
```

- Launch a container
```bash
rcl run -t localhost:5001/myartifact:latest mycontainer
```

- Delete container
```bash
rcl delete mycontainer
```
