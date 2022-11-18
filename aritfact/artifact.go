package aritfact

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/containerd/containerd/diff"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/log"
	"github.com/containerd/containerd/mount"
	"github.com/containerd/containerd/snapshots"
	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/identity"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

type Artifact struct {
	Blob ocispec.Descriptor
}

func ApplyArtifacts(ctx context.Context, layers []Artifact, sn snapshots.Snapshotter, a diff.Applier) (digest.Digest, error) {
	return ApplyArtifactsWithOpts(ctx, layers, sn, a, nil)
}

func ApplyArtifactsWithOpts(ctx context.Context, artifacts []Artifact, sn snapshots.Snapshotter, a diff.Applier, applyOpts []diff.ApplyOpt) (digest.Digest, error) {
	chain := make([]digest.Digest, len(artifacts))
	for i, artifact := range artifacts {
		chain[i] = artifact.Blob.Digest
	}
	chainID := identity.ChainID(chain)

	// Just stat top layer, remaining layers will have their existence checked
	// on prepare. Calling prepare on upper layers first guarantees that upper
	// layers are not removed while calling stat on lower layers
	_, err := sn.Stat(ctx, chainID.String())
	if err != nil {
		if !errdefs.IsNotFound(err) {
			return "", fmt.Errorf("failed to stat snapshot %s: %w", chainID, err)
		}

		if err := applyArtifacts(ctx, artifacts, chain, sn, a, nil, applyOpts); err != nil && !errdefs.IsAlreadyExists(err) {
			return "", err
		}
	}

	return chainID, nil
}

func ApplyArtifact(ctx context.Context, layer Artifact, chain []digest.Digest, sn snapshots.Snapshotter, a diff.Applier, opts ...snapshots.Opt) (bool, error) {
	return ApplyArtifactWithOpts(ctx, layer, chain, sn, a, opts, nil)
}

func ApplyArtifactWithOpts(ctx context.Context, artifact Artifact, chain []digest.Digest, sn snapshots.Snapshotter, a diff.Applier, opts []snapshots.Opt, applyOpts []diff.ApplyOpt) (bool, error) {
	var (
		chainID = identity.ChainID(append(chain, artifact.Blob.Digest)).String()
		applied bool
	)

	if _, err := sn.Stat(ctx, chainID); err != nil {

		if !errdefs.IsNotFound(err) {
			return false, fmt.Errorf("failed to stat snapshot %s: %w", chainID, err)
		}

		if err := applyArtifacts(ctx, []Artifact{artifact}, append(chain, artifact.Blob.Digest), sn, a, opts, applyOpts); err != nil {
			if !errdefs.IsAlreadyExists(err) {
				return false, err
			}
		} else {
			applied = true
		}
	}
	return applied, nil
}

func applyArtifacts(ctx context.Context, artifacts []Artifact, chain []digest.Digest, sn snapshots.Snapshotter, a diff.Applier, opts []snapshots.Opt, applyOpts []diff.ApplyOpt) error {
	var (
		parent   = identity.ChainID(chain[:len(chain)-1])
		chainID  = identity.ChainID(chain)
		artifact = artifacts[len(artifacts)-1]
		diff     ocispec.Descriptor
		key      string
		mounts   []mount.Mount
		err      error
	)

	for {
		key = fmt.Sprintf(snapshots.UnpackKeyFormat, uniquePart(), chainID)

		// Prepare snapshot with from parent, label as root
		mounts, err = sn.Prepare(ctx, key, parent.String(), opts...)
		if err != nil {
			if errdefs.IsNotFound(err) && len(artifacts) > 1 {
				if err := applyArtifacts(ctx, artifacts[:len(artifacts)-1], chain[:len(chain)-1], sn, a, opts, applyOpts); err != nil {
					if !errdefs.IsAlreadyExists(err) {
						return err
					}
				}
				// Do no try applying layers again
				artifacts = nil
				continue
			} else if errdefs.IsAlreadyExists(err) {
				// Try a different key
				continue
			}

			// Already exists should have the caller retry
			return fmt.Errorf("failed to prepare extraction snapshot %q: %w", key, err)

		}
		break
	}
	defer func() {
		if err != nil {
			if !errdefs.IsAlreadyExists(err) {
				log.G(ctx).WithError(err).WithField("key", key).Infof("apply failure, attempting cleanup")
			}

			if rerr := sn.Remove(ctx, key); rerr != nil {
				log.G(ctx).WithError(rerr).WithField("key", key).Warnf("extraction snapshot removal failed")
			}
		}
	}()

	diff, err = a.Apply(ctx, artifact.Blob, mounts, applyOpts...)
	if err != nil {
		err = fmt.Errorf("failed to extract layer %s: %w", artifact.Blob.Digest, err)
		return err
	}
	if diff.Digest != artifact.Blob.Digest {
		err = fmt.Errorf("wrong diff id calculated on extraction %q", diff.Digest)
		return err
	}

	if err = sn.Commit(ctx, chainID.String(), key, opts...); err != nil {
		err = fmt.Errorf("failed to commit snapshot %s: %w", key, err)
		return err
	}

	return nil
}

func uniquePart() string {
	t := time.Now()
	var b [3]byte
	// Ignore read failures, just decreases uniqueness
	rand.Read(b[:])
	return fmt.Sprintf("%d-%s", t.Nanosecond(), base64.URLEncoding.EncodeToString(b[:]))
}
