package loader

import (
	"context"
	"encoding/json"

	"github.com/google/go-containerregistry/pkg/v1/types"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	uorspec "github.com/uor-framework/collection-spec/specs-go/v1alpha1"

	"github.com/uor-framework/uor-client-go/model"
	"github.com/uor-framework/uor-client-go/model/traversal"
	"github.com/uor-framework/uor-client-go/nodes/collection"
	"github.com/uor-framework/uor-client-go/nodes/descriptor"
	v2 "github.com/uor-framework/uor-client-go/nodes/descriptor/v2"
)

// FetcherFunc fetches content for the specified descriptor
type FetcherFunc func(context.Context, ocispec.Descriptor) ([]byte, error)

// LoadFromManifest loads an OCI DAG into a Collection.
func LoadFromManifest(ctx context.Context, graph *collection.Collection, fetcher FetcherFunc, manifest ocispec.Descriptor) error {
	// prepare pre-handler
	root, err := v2.NewNode(manifest.Digest.String(), manifest)
	if err != nil {
		return err
	}

	// track content status
	tracker := traversal.NewTracker(root, nil)

	seen := map[string]struct{}{}
	handler := traversal.HandlerFunc(func(ctx context.Context, tracker traversal.Tracker, node model.Node) ([]model.Node, error) {
		if _, ok := seen[node.ID()]; ok {
			return nil, traversal.ErrSkip
		}

		desc, ok := node.(*v2.Node)
		if !ok {
			return nil, traversal.ErrSkip
		}

		// We do not want to expect to traversal outside the repo doing this
		// traversal, so we just make the links leaf nodes that can be
		// lazily loaded.

		if desc.Properties != nil && desc.Properties.IsALink() {
			return nil, nil
		}

		successors, err := getSuccessors(ctx, fetcher, desc.Descriptor())
		if err != nil {
			return nil, err
		}

		nodes, err := indexNode(graph, desc.Descriptor(), successors)
		if err != nil {
			return nil, err
		}

		seen[node.ID()] = struct{}{}

		return nodes, nil
	})

	return tracker.Walk(ctx, handler, root)
}

// AddManifest will add a single manifest to the Collection.
func AddManifest(ctx context.Context, graph *collection.Collection, fetcher FetcherFunc, node ocispec.Descriptor) error {
	successors, err := getSuccessors(ctx, fetcher, node)
	if err != nil {
		return err
	}
	if _, err := indexNode(graph, node, successors); err != nil {
		return err
	}
	return nil
}

// indexNode indexes relationships between child and parent nodes.
func indexNode(graph *collection.Collection, node ocispec.Descriptor, successors []ocispec.Descriptor) ([]model.Node, error) {
	n, err := addOrGetNode(graph, node)
	if err != nil {
		return nil, err
	}
	var result []model.Node
	for _, successor := range successors {
		s, err := addOrGetNode(graph, successor)
		if err != nil {
			return nil, err
		}
		result = append(result, s)
		e := collection.NewEdge(n, s)
		if err := graph.AddEdge(e); err != nil {
			return nil, err
		}
	}
	return result, nil
}

// addOrGetNode will return the node if it exists in the graph or will create a new
// descriptor node.
func addOrGetNode(graph *collection.Collection, desc ocispec.Descriptor) (model.Node, error) {
	n, err := v2.NewNode(desc.Digest.String(), desc)
	if err != nil {
		return nil, err
	}

	// Determine if the node is existing. If the existing node is link,
	// update the node to get the full info and return it. If it is existing
	// and not a link, return the existing node.
	existing := graph.NodeByID(desc.Digest.String())
	if existing != nil {
		desc, ok := existing.(*v2.Node)
		if ok && desc.Properties.IsALink() {
			err := graph.UpdateNode(n)
			return n, err
		}
		return existing, nil
	}

	if err := graph.AddNode(n); err != nil {
		return nil, err
	}
	return n, nil
}

// getSuccessor returns the nodes directly pointed by the current node. This is adapted from the `oras` content.Successors
// to allow the use of a function signature to pull descriptor content.
func getSuccessors(ctx context.Context, fetcher FetcherFunc, node ocispec.Descriptor) ([]ocispec.Descriptor, error) {
	switch node.MediaType {
	case string(types.DockerManifestSchema2), ocispec.MediaTypeImageManifest:
		content, err := fetcher(ctx, node)
		if err != nil {
			return nil, err
		}

		// docker manifest and oci manifest are equivalent for successors.
		var manifest ocispec.Manifest
		if err := json.Unmarshal(content, &manifest); err != nil {
			return nil, err
		}

		nodes := append([]ocispec.Descriptor{manifest.Config}, manifest.Layers...)

		if manifest.Annotations != nil {
			link, ok := manifest.Annotations[uorspec.AnnotationLink]
			if ok {
				var desc ocispec.Descriptor
				if err := json.Unmarshal([]byte(link), &desc); err != nil {
					return nil, err
				}
				nodes = append(nodes, desc)
			}
		}
		return nodes, nil
	case string(types.DockerManifestList), ocispec.MediaTypeImageIndex:
		content, err := fetcher(ctx, node)
		if err != nil {
			return nil, err
		}

		// docker manifest list and oci index are equivalent for successors.
		var index ocispec.Index
		if err := json.Unmarshal(content, &index); err != nil {
			return nil, err
		}

		return index.Manifests, nil
	case ocispec.MediaTypeArtifactManifest:
		content, err := fetcher(ctx, node)
		if err != nil {
			return nil, err
		}

		var manifest ocispec.Artifact
		if err := json.Unmarshal(content, &manifest); err != nil {
			return nil, err
		}
		var nodes []ocispec.Descriptor
		if manifest.Subject != nil {
			nodes = append(nodes, *manifest.Subject)
		}

		if manifest.Annotations != nil {
			link, ok := manifest.Annotations[uorspec.AnnotationLink]
			if ok {
				var desc ocispec.Descriptor
				if err := json.Unmarshal([]byte(link), &desc); err != nil {
					return nil, err
				}
				nodes = append(nodes, desc)
			}
		}

		return append(nodes, manifest.Blobs...), nil
	case uorspec.MediaTypeCollectionManifest:
		content, err := fetcher(ctx, node)
		if err != nil {
			return nil, err
		}

		var manifest uorspec.Manifest
		if err := json.Unmarshal(content, &manifest); err != nil {
			return nil, err
		}
		var nodes []ocispec.Descriptor
		for _, blob := range manifest.Blobs {
			collectionBlob, err := descriptor.CollectionToOCI(blob)
			if err != nil {
				return nil, err
			}
			nodes = append(nodes, collectionBlob)
		}
		for _, link := range manifest.Links {
			collectionBlob, err := descriptor.CollectionToOCI(link)
			if err != nil {
				return nil, err
			}
			nodes = append(nodes, collectionBlob)
		}
		return nodes, nil
	}

	return nil, nil
}
