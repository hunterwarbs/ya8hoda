package embed

import (
	"context"

	"github.com/hunterwarburton/ya8hoda/internal/core" // Import core types
)

// EmbedService interface definition removed (moved to internal/core)

// BGEAdapter is a wrapper around BGEEmbedder that implements core.EmbedService interface
type BGEAdapter struct {
	embedder *BGEEmbedder
}

// NewBGEAdapter creates a new adapter for the BGEEmbedder
func NewBGEAdapter(embedder *BGEEmbedder) *BGEAdapter {
	return &BGEAdapter{
		embedder: embedder,
	}
}

// EmbedQuery implements the core.EmbedService interface.
// It calls the underlying BGEEmbedder which returns both dense and sparse vectors.
func (a *BGEAdapter) EmbedQuery(ctx context.Context, text string) (core.EmbeddingVector, error) {
	// Get both dense and sparse embeddings from the BGEEmbedder
	// BGEEmbedder.EmbedQuery already returns the core.EmbeddingVector format
	return a.embedder.EmbedQuery(ctx, text)
}

// EmbedQueryFull method removed (renamed to EmbedQueryHybrid to match interface)

// UnwrapEmbedder provides access to the underlying BGEEmbedder
// Returns the embedder and true if successful, nil and false otherwise
func (a *BGEAdapter) UnwrapEmbedder() (*BGEEmbedder, bool) {
	if a.embedder == nil {
		return nil, false
	}
	return a.embedder, true
}
