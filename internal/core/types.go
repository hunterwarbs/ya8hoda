package core

// Needed if we keep embedding vector details here

// Document represents a document stored in Milvus
type Document struct {
	ID         string                 `json:"id"`
	OwnerID    string                 `json:"owner_id,omitempty"`
	Name       string                 `json:"name,omitempty"`
	Text       string                 `json:"text"`
	Title      string                 `json:"title,omitempty"`
	Source     string                 `json:"source,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	CreateTime int64                  `json:"create_time"`
	// Consider if embedding vectors should be part of this core type
}

// SearchResult represents a search result with a document and a score
type SearchResult struct {
	Document Document `json:"document"`
	Score    float32  `json:"score"`
	// Consider if ExplainInfo, etc., should be included here
}

// EmbeddingVector represents potentially hybrid embedding vectors.
// Moved from internal/embed to avoid import cycles if interfaces need it.
type EmbeddingVector struct {
	Dense  []float32     `json:"dense"`
	Sparse *SparseVector `json:"sparse,omitempty"`
}

// SparseVector represents a sparse vector with indices, values and shape.
// Moved from internal/embed.
type SparseVector struct {
	Indices []int     `json:"indices"`
	Values  []float32 `json:"values"`
	Shape   []int     `json:"shape"` // Shape might be optional depending on usage
}
