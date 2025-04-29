package rag

import (
	"context"
	"fmt"
	"log"
	"time"
)

// MockMilvusService provides a mock implementation for the MilvusService
type MockMilvusService struct {
	embeddingDim int
}

// NewMockMilvusService creates a new instance of MockMilvusService
func NewMockMilvusService(ctx context.Context, addr string, embeddingDim int) (*MockMilvusService, error) {
	log.Printf("Initializing mock Milvus service with addr: %s, embeddingDim: %d", addr, embeddingDim)

	// Set default dimension if not specified
	if embeddingDim <= 0 {
		embeddingDim = DefaultEmbeddingDim
	}

	return &MockMilvusService{
		embeddingDim: embeddingDim,
	}, nil
}

// StoreDocument mocks storing a document in the vector database
func (s *MockMilvusService) StoreDocument(ctx context.Context, text, title, source string, metadata map[string]interface{}, vector []float32) (string, error) {
	log.Printf("Mock: Storing document with title: %s", title)

	// Generate a mock document ID
	docID := fmt.Sprintf("mock_doc_%d", time.Now().UnixNano())

	return docID, nil
}

// SearchSimilar mocks searching for similar documents in the vector database
func (s *MockMilvusService) SearchSimilar(ctx context.Context, vector []float32, k int, filter string) ([]SearchResult, error) {
	log.Printf("Mock: Searching similar documents with k=%d, filter=%s", k, filter)

	// Create mock search results
	results := []SearchResult{
		{
			Document: Document{
				ID:         "mock_doc_1",
				Text:       "This is a mock document for testing purposes.",
				Title:      "Mock Document 1",
				Source:     "Testing",
				Metadata:   map[string]interface{}{"type": "mock"},
				CreateTime: time.Now().Unix(),
			},
			Score: 0.95,
		},
		{
			Document: Document{
				ID:         "mock_doc_2",
				Text:       "Another mock document with different content.",
				Title:      "Mock Document 2",
				Source:     "Testing",
				Metadata:   map[string]interface{}{"type": "mock"},
				CreateTime: time.Now().Unix(),
			},
			Score: 0.85,
		},
	}

	return results, nil
}

// Close mocks closing the connection to Milvus
func (s *MockMilvusService) Close() error {
	log.Printf("Mock: Closing Milvus connection")
	return nil
}

// GetEmbeddingDim returns the dimensionality of embeddings
func (s *MockMilvusService) GetEmbeddingDim() int {
	return s.embeddingDim
}
