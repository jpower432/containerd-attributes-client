package aritfact

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/containerd/containerd/diff"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/log"
	"github.com/containerd/containerd/mount"
	"github.com/containerd/containerd/pkg/userns"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/sirupsen/logrus"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/content/file"
)

type artifactApplier struct {
	store content.Fetcher
}

var emptyDesc = ocispec.Descriptor{}

// Apply applies the content associated with the provided digests onto the
// provided mounts. Archive content will be extracted and decompressed if
// necessary.
func (a *artifactApplier) Apply(ctx context.Context, desc ocispec.Descriptor, mounts []mount.Mount, opts ...diff.ApplyOpt) (d ocispec.Descriptor, err error) {
	t1 := time.Now()
	defer func() {
		if err == nil {
			log.G(ctx).WithFields(logrus.Fields{
				"d":      time.Since(t1),
				"digest": desc.Digest,
				"size":   desc.Size,
				"media":  desc.MediaType,
			}).Debugf("diff applied")
		}
	}()

	var config diff.ApplyConfig
	for _, o := range opts {
		if err := o(ctx, desc, &config); err != nil {
			return emptyDesc, fmt.Errorf("failed to apply config opt: %w", err)
		}
	}

	rc, err := a.store.Fetch(ctx, desc)
	if err != nil {
		return emptyDesc, err
	}
	defer rc.Close()

	if err := apply(ctx, mounts, desc, rc); err != nil {
		return emptyDesc, err
	}

	// Read any trailing data
	if _, err := io.Copy(io.Discard, rc); err != nil {
		return emptyDesc, err
	}

	return ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageLayer,
		Size:      desc.Size,
		Digest:    desc.Digest,
	}, nil
}

func apply(ctx context.Context, mounts []mount.Mount, desc ocispec.Descriptor, r io.Reader) error {
	switch {
	case len(mounts) == 1 && mounts[0].Type == "overlay":
		// OverlayConvertWhiteout (mknod c 0 0) doesn't work in userns.
		// https://github.com/containerd/containerd/issues/3762
		if userns.RunningInUserNS() {
			break
		}

		// I will need to use parents eventually
		path, _, err := getOverlayPath(mounts[0].Options)
		if err != nil {
			if errdefs.IsInvalidArgument(err) {
				break
			}
			return err
		}

		store := file.New(path)
		return store.Push(ctx, desc, r)
	case len(mounts) == 1 && mounts[0].Type == "aufs":
		path, _, err := getAufsPath(mounts[0].Options)
		if err != nil {
			if errdefs.IsInvalidArgument(err) {
				break
			}
			return err
		}
		store := file.New(path)
		return store.Push(ctx, desc, r)

	}
	return mount.WithTempMount(ctx, mounts, func(root string) error {
		store := file.New(root)
		return store.Push(ctx, desc, r)
	})
}

func getOverlayPath(options []string) (upper string, lower []string, err error) {
	const upperdirPrefix = "upperdir="
	const lowerdirPrefix = "lowerdir="

	for _, o := range options {
		if strings.HasPrefix(o, upperdirPrefix) {
			upper = strings.TrimPrefix(o, upperdirPrefix)
		} else if strings.HasPrefix(o, lowerdirPrefix) {
			lower = strings.Split(strings.TrimPrefix(o, lowerdirPrefix), ":")
		}
	}
	if upper == "" {
		return "", nil, fmt.Errorf("upperdir not found: %w", errdefs.ErrInvalidArgument)
	}

	return
}

// getAufsPath handles options as given by the containerd aufs package only,
// formatted as "br:<upper>=rw[:<lower>=ro+wh]*"
func getAufsPath(options []string) (upper string, lower []string, err error) {
	const (
		sep      = ":"
		brPrefix = "br:"
		rwSuffix = "=rw"
		roSuffix = "=ro+wh"
	)
	for _, o := range options {
		if strings.HasPrefix(o, brPrefix) {
			o = strings.TrimPrefix(o, brPrefix)
		} else {
			continue
		}

		for _, b := range strings.Split(o, sep) {
			if strings.HasSuffix(b, rwSuffix) {
				if upper != "" {
					return "", nil, fmt.Errorf("multiple rw branch found: %w", errdefs.ErrInvalidArgument)
				}
				upper = strings.TrimSuffix(b, rwSuffix)
			} else if strings.HasSuffix(b, roSuffix) {
				if upper == "" {
					return "", nil, fmt.Errorf("rw branch be first: %w", errdefs.ErrInvalidArgument)
				}
				lower = append(lower, strings.TrimSuffix(b, roSuffix))
			} else {
				return "", nil, fmt.Errorf("unhandled aufs suffix: %w", errdefs.ErrInvalidArgument)
			}

		}
	}
	if upper == "" {
		return "", nil, fmt.Errorf("rw branch not found: %w", errdefs.ErrInvalidArgument)
	}
	return
}
