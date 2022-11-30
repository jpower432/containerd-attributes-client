package aritfact

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/content"
	"github.com/containerd/containerd/defaults"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/platforms"
	"github.com/containerd/containerd/snapshots"
	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/identity"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	v2 "github.com/uor-framework/uor-client-go/nodes/descriptor/v2"
)

// Image describes an image used by containers.
type Image interface {
	// Name of the image
	Name() string
	// Target descriptor for the image content
	Target() ocispec.Descriptor
	// Labels of the image
	Labels() map[string]string
	// Unpack unpacks the image's content into a snapshot
	Unpack(context.Context, string, ...containerd.UnpackOpt) error
	// RootFS returns the unpacked diffids that make up images rootfs.
	RootFS(ctx context.Context) ([]digest.Digest, error)
	// Size returns the total size of the image's packed resources.
	Size(ctx context.Context) (int64, error)
	// Usage returns a usage calculation for the image.
	Usage(context.Context, ...containerd.UsageOpt) (int64, error)
	// Config descriptor for the image.
	Config(ctx context.Context) (ocispec.Descriptor, error)
	// ConfigWithAttributes return image config information.
	ConfigWithAttributes(ctx context.Context) (ocispec.ImageConfig, error)
	// IsUnpacked returns whether or not an image is unpacked.
	IsUnpacked(context.Context, string) (bool, error)
	// ContentStore provides a content store which contains image blob data
	ContentStore() content.Store
	// Metadata returns the underlying image metadata
	Metadata() images.Image
	// Platform returns the platform match comparer. Can be nil.
	Platform() platforms.MatchComparer
	// Spec returns the OCI image spec for a given image.
	Spec(ctx context.Context) (ocispec.Image, error)
}

var _ = (Image)(&image{})

// NewImage returns a client image object from the metadata image.
func NewImage(client *containerd.Client, i images.Image, cI containerd.Image) Image {
	return &image{
		client:   client,
		i:        i,
		image:    cI,
		platform: cI.Platform(),
	}
}

// NewImageWithPlatform returns a client image object from the metadata image
func NewImageWithPlatform(client *containerd.Client, i images.Image, platform platforms.MatchComparer) Image {
	return &image{
		client:   client,
		i:        i,
		platform: platform,
	}
}

type image struct {
	client *containerd.Client

	i        images.Image
	image    containerd.Image
	platform platforms.MatchComparer
}

func (i *image) Metadata() images.Image {
	return i.i
}

func (i *image) Name() string {
	return i.i.Name
}

func (i *image) Target() ocispec.Descriptor {
	return i.i.Target
}

func (i *image) Labels() map[string]string {
	return i.i.Labels
}

func (i *image) RootFS(ctx context.Context) ([]digest.Digest, error) {
	cs := i.client.ContentStore()
	manifest, err := images.Manifest(ctx, cs, i.i.Target, i.platform)
	if err != nil {
		return nil, err
	}
	var digests []digest.Digest
	for _, layer := range manifest.Layers {
		digests = append(digests, layer.Digest)
	}
	return digests, nil
}

func (i *image) Size(ctx context.Context) (int64, error) {
	return i.image.Size(ctx)
}

func (i *image) Usage(ctx context.Context, opts ...containerd.UsageOpt) (int64, error) {
	return i.image.Usage(ctx, opts...)
}

func (i *image) Config(ctx context.Context) (ocispec.Descriptor, error) {
	return i.image.Config(ctx)
}

func (i *image) ConfigWithAttributes(ctx context.Context) (ocispec.ImageConfig, error) {
	provider := i.client.ContentStore()
	return ConfigFromAttributes(ctx, provider, i.Target(), i.platform)
}

func (i *image) IsUnpacked(ctx context.Context, snapshotterName string) (bool, error) {
	sn, err := getSnapshotter(ctx, i.client, snapshotterName)
	if err != nil {
		return false, err
	}

	diffs, err := i.RootFS(ctx)
	if err != nil {
		return false, err
	}

	chainID := identity.ChainID(diffs)
	_, err = sn.Stat(ctx, chainID.String())
	if err == nil {
		return true, nil
	} else if !errdefs.IsNotFound(err) {
		return false, err
	}

	return false, nil
}

func (i *image) Spec(ctx context.Context) (ocispec.Image, error) {
	var ociImage ocispec.Image

	desc, err := i.Config(ctx)
	if err != nil {
		return ociImage, fmt.Errorf("get image config descriptor: %w", err)
	}

	blob, err := content.ReadBlob(ctx, i.ContentStore(), desc)
	if err != nil {
		return ociImage, fmt.Errorf("read image config from content store: %w", err)
	}

	if err := json.Unmarshal(blob, &ociImage); err != nil {
		return ociImage, fmt.Errorf("unmarshal image config %s: %w", blob, err)
	}

	return ociImage, nil
}

func (i *image) Unpack(ctx context.Context, snapshotterName string, opts ...containerd.UnpackOpt) error {
	ctx, done, err := i.client.WithLease(ctx)
	if err != nil {
		return err
	}
	defer done(ctx)

	var config containerd.UnpackConfig
	for _, o := range opts {
		if err := o(ctx, &config); err != nil {
			return err
		}
	}

	manifest, err := i.getManifest(ctx, i.platform)
	if err != nil {
		return err
	}

	artifacts, err := i.getArtifacts(ctx, i.platform, manifest)
	if err != nil {
		return err
	}

	var (
		cs = i.client.ContentStore()
		a  = &artifactApplier{&contentStore{cs}}

		chain    []digest.Digest
		unpacked bool
	)
	snapshotterName, err = resolveSnapshotterName(ctx, i.client, snapshotterName)
	if err != nil {
		return err
	}
	sn, err := getSnapshotter(ctx, i.client, snapshotterName)
	if err != nil {
		return err
	}
	if config.CheckPlatformSupported {
		if err := i.checkSnapshotterSupport(ctx, snapshotterName, manifest); err != nil {
			return err
		}
	}

	for _, artifact := range artifacts {
		unpacked, err = ApplyArtifactWithOpts(ctx, artifact, chain, sn, a, config.SnapshotOpts, config.ApplyOpts)
		if err != nil {
			return err
		}

		if unpacked {
			// Set the uncompressed label after the uncompressed
			// digest has been verified through apply.
			cinfo := content.Info{
				Digest: artifact.Blob.Digest,
				Labels: map[string]string{
					"containerd.io/uncompressed": artifact.Blob.Digest.String(),
				},
			}
			if _, err := cs.Update(ctx, cinfo, "labels.containerd.io/uncompressed"); err != nil {
				return err
			}
		}

		chain = append(chain, artifact.Blob.Digest)
	}

	desc, err := i.i.Config(ctx, cs, i.platform)
	if err != nil {
		return err
	}

	rootfs := identity.ChainID(chain).String()

	cinfo := content.Info{
		Digest: desc.Digest,
		Labels: map[string]string{
			fmt.Sprintf("containerd.io/gc.ref.snapshot.%s", snapshotterName): rootfs,
		},
	}

	_, err = cs.Update(ctx, cinfo, fmt.Sprintf("labels.containerd.io/gc.ref.snapshot.%s", snapshotterName))
	return err
}

func (i *image) getManifest(ctx context.Context, platform platforms.MatchComparer) (ocispec.Manifest, error) {
	cs := i.ContentStore()
	manifest, err := images.Manifest(ctx, cs, i.i.Target, platform)
	if err != nil {
		return ocispec.Manifest{}, err
	}
	return manifest, nil
}

func (i *image) getArtifacts(ctx context.Context, platform platforms.MatchComparer, manifest ocispec.Manifest) ([]Artifact, error) {
	artifacts := make([]Artifact, len(manifest.Layers))
	for i := range manifest.Layers {
		artifacts[i].Blob = manifest.Layers[i]
	}
	return artifacts, nil
}

func (i *image) getManifestPlatform(ctx context.Context, manifest ocispec.Manifest) (ocispec.Platform, error) {
	cs := i.ContentStore()
	p, err := content.ReadBlob(ctx, cs, manifest.Config)
	if err != nil {
		return ocispec.Platform{}, err
	}

	var image ocispec.Image
	if err := json.Unmarshal(p, &image); err != nil {
		return ocispec.Platform{}, err
	}
	return platforms.Normalize(ocispec.Platform{OS: image.OS, Architecture: image.Architecture}), nil
}

func (i *image) checkSnapshotterSupport(ctx context.Context, snapshotterName string, manifest ocispec.Manifest) error {
	snapshotterPlatformMatcher, err := i.client.GetSnapshotterSupportedPlatforms(ctx, snapshotterName)
	if err != nil {
		return err
	}

	manifestPlatform, err := i.getManifestPlatform(ctx, manifest)
	if err != nil {
		return err
	}

	if snapshotterPlatformMatcher.Match(manifestPlatform) {
		return nil
	}
	return fmt.Errorf("snapshotter %s does not support platform %s for image %s", snapshotterName, manifestPlatform, manifest.Config.Digest)
}

func (i *image) ContentStore() content.Store {
	return i.client.ContentStore()
}

func (i *image) Platform() platforms.MatchComparer {
	return i.platform
}

type contentStore struct {
	content.Store
}

func (c *contentStore) Fetch(ctx context.Context, desc ocispec.Descriptor) (io.ReadCloser, error) {
	ra, err := c.Store.ReaderAt(ctx, desc)
	if err != nil {
		return nil, err
	}
	rdr := content.NewReader(ra)
	rc := io.NopCloser(rdr)
	return rc, nil
}

// ConfigFromAttributes resolves the image configuration descriptor using manifest attributes.
//
// The caller can then use the descriptor to resolve and process the
// configuration of the image.
func ConfigFromAttributes(ctx context.Context, provider content.Provider, image ocispec.Descriptor, platform platforms.MatchComparer) (ocispec.ImageConfig, error) {
	manifest, err := images.Manifest(ctx, provider, image, platform)
	if err != nil {
		return ocispec.ImageConfig{}, err
	}

	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		return ocispec.ImageConfig{}, err
	}
	d := digest.FromBytes(manifestBytes)

	desc := ocispec.Descriptor{
		MediaType:   ocispec.MediaTypeImageManifest,
		Digest:      d,
		Size:        int64(len(manifestBytes)),
		Annotations: manifest.Annotations,
	}

	node, err := v2.NewNode(d.String(), desc)
	if err != nil {
		return ocispec.ImageConfig{}, err
	}

	props := node.Properties
	if props == nil || !props.HasRuntimeInfo() {
		return ocispec.ImageConfig{}, err
	}

	return *props.Runtime, err
}

func getSnapshotter(ctx context.Context, c *containerd.Client, name string) (snapshots.Snapshotter, error) {
	name, err := resolveSnapshotterName(ctx, c, name)
	if err != nil {
		return nil, err
	}

	s := c.SnapshotService(name)
	if s == nil {
		return nil, fmt.Errorf("snapshotter %s was not found: %w", name, errdefs.ErrNotFound)
	}

	return s, nil
}

func resolveSnapshotterName(ctx context.Context, c *containerd.Client, name string) (string, error) {
	if name == "" {
		label, err := c.GetLabel(ctx, defaults.DefaultSnapshotterNSLabel)
		if err != nil {
			return "", err
		}

		if label != "" {
			name = label
		} else {
			name = containerd.DefaultSnapshotter
		}
	}

	return name, nil
}
