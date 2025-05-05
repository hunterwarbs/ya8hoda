package core

import "context"

// EmbedService defines the interface for generating potentially hybrid embeddings.
type EmbedService interface {
	// EmbedQuery generates dense and sparse embeddings for a given text.
	EmbedQuery(ctx context.Context, text string) (EmbeddingVector, error)
}

// RAGService defines the interface for interacting with the RAG system.
type RAGService interface {
	// Store methods - Must handle hybrid embeddings internally
	RememberAboutPerson(ctx context.Context, telegramID string, personName string, memoryText string, metadata map[string]interface{}) (string, error) // Returns success message or error
	RememberAboutSelf(ctx context.Context, memoryText string, metadata map[string]interface{}) (string, error)                                         // Returns success message or error
	RememberAboutCommunity(ctx context.Context, communityID string, memoryText string, metadata map[string]interface{}) (string, error)                // Returns success message or error

	// Search methods - Must handle hybrid embeddings internally
	SearchAllMemories(ctx context.Context, query string, k int) ([]SearchResult, error)
	SearchSelfMemory(ctx context.Context, query string, k int) ([]SearchResult, error)
	SearchPersonalMemory(ctx context.Context, query string, telegramID string, k int) ([]SearchResult, error)
	SearchCommunityMemory(ctx context.Context, query string, communityID string, k int) ([]SearchResult, error)
}
