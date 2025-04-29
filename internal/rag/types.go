package rag

// DefaultEmbeddingDim is the default dimension for embedding vectors
const DefaultEmbeddingDim = 1536

// Document represents a document in the vector database
type Document struct {
	ID         string                 `json:"id"`
	Text       string                 `json:"text"`
	Title      string                 `json:"title,omitempty"`
	Source     string                 `json:"source,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	CreateTime int64                  `json:"create_time,omitempty"`
}

// SearchResult represents a search result with a document and its similarity score
type SearchResult struct {
	Document Document `json:"document"`
	Score    float32  `json:"score"`
}
