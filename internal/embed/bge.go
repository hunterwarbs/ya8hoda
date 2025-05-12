package embed

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/hunterwarburton/ya8hoda/internal/core" // Import core types
)

// MilvusEmbedderService defines the interface for a service that can provide embeddings through Milvus
// BGEEmbedder needs this during initialization to get dimension info.
type MilvusEmbedderService interface {
	GetEmbeddingDim() int
	GetSparseEmbeddingDim() int // Returns 0 if not supported
	SupportsSparseEmbeddings() bool
}

// MilvusConfigProvider implements MilvusEmbedderService using static configuration values.
// Used for initializing BGEEmbedder before the real Milvus client is ready.
type MilvusConfigProvider struct {
	Dim       int
	SparseDim int
	UseSparse bool
}

func (p *MilvusConfigProvider) GetEmbeddingDim() int {
	if p.Dim <= 0 {
		return 1024 // Default fallback
	}
	return p.Dim
}

func (p *MilvusConfigProvider) GetSparseEmbeddingDim() int {
	if !p.UseSparse {
		return 0
	}
	// You might need a way to configure this sparse dimension if used
	return p.SparseDim // Return configured sparse dim or 0
}

func (p *MilvusConfigProvider) SupportsSparseEmbeddings() bool {
	return p.UseSparse
}

// EmbeddingVector definition removed (moved to internal/core)

// SparseVector definition removed (moved to internal/core)

// BGEEmbedder implements EmbedService using Milvus's BGE-M3 embeddings
type BGEEmbedder struct {
	milvusService MilvusEmbedderService
	modelName     string
	apiURL        string
	httpClient    *http.Client
}

// BGEEmbedderConfig holds configuration for the BGE embedder
type BGEEmbedderConfig struct {
	ModelName  string
	ApiURL     string
	TimeoutSec int
}

// NewBGEEmbedder creates a new BGEEmbedder instance
func NewBGEEmbedder(milvusService MilvusEmbedderService, config BGEEmbedderConfig) *BGEEmbedder {
	if config.ModelName == "" {
		config.ModelName = "BAAI/bge-m3" // Default model name
	}

	if config.TimeoutSec == 0 {
		config.TimeoutSec = 30 // Default timeout
	}

	return &BGEEmbedder{
		milvusService: milvusService,
		modelName:     config.ModelName,
		apiURL:        config.ApiURL,
		httpClient: &http.Client{
			Timeout: time.Duration(config.TimeoutSec) * time.Second,
		},
	}
}

// EmbedRequest represents a request to the embedding API
type EmbedRequest struct {
	Texts        []string `json:"texts"`
	ModelName    string   `json:"model_name"`
	ReturnSparse bool     `json:"return_sparse"`
}

// EmbedResponse represents a response from the embedding API
type EmbedResponse struct {
	Embeddings struct {
		Dense  [][]float32         `json:"dense"`
		Sparse []core.SparseVector `json:"sparse"`
	} `json:"embeddings"`
	Error string `json:"error,omitempty"`
}

// EmbedQuery embeds a query using BGE-M3 model via Milvus
// Returns the core.EmbeddingVector now
func (e *BGEEmbedder) EmbedQuery(ctx context.Context, text string) (core.EmbeddingVector, error) {
	log.Printf("Creating embedding for text: %s", text)

	// If no API URL is provided, fall back to the dummy implementation
	if e.apiURL == "" {
		return e.createDummyEmbedding(), fmt.Errorf("no embedding API URL configured, using dummy embedding")
	}

	// Prepare the request body, explicitly asking for sparse
	reqBody := EmbedRequest{
		Texts:        []string{text},
		ModelName:    e.modelName,
		ReturnSparse: true,
	}

	reqJSON, err := json.Marshal(reqBody)
	if err != nil {
		return core.EmbeddingVector{}, fmt.Errorf("failed to marshal embedding request: %w", err)
	}

	// Create and send HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", e.apiURL+"/embed", strings.NewReader(string(reqJSON)))
	if err != nil {
		return core.EmbeddingVector{}, fmt.Errorf("failed to create embedding request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return core.EmbeddingVector{}, fmt.Errorf("embedding request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return core.EmbeddingVector{}, fmt.Errorf("embedding API returned non-OK status: %d", resp.StatusCode)
	}

	// Parse the response
	var embedResp EmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&embedResp); err != nil {
		return core.EmbeddingVector{}, fmt.Errorf("failed to decode embedding response: %w", err)
	}

	if embedResp.Error != "" {
		return core.EmbeddingVector{}, fmt.Errorf("embedding API returned error: %s", embedResp.Error)
	}

	// Extract the embeddings
	if len(embedResp.Embeddings.Dense) == 0 {
		return core.EmbeddingVector{}, fmt.Errorf("no dense embeddings in response")
	}

	// Use core.EmbeddingVector and core.SparseVector
	result := core.EmbeddingVector{
		Dense: embedResp.Embeddings.Dense[0],
	}

	// Add sparse embeddings if available and supported by the service
	if e.milvusService.SupportsSparseEmbeddings() && len(embedResp.Embeddings.Sparse) > 0 {
		// Need to convert the sparse vector format from API response to core.SparseVector
		apiSparse := embedResp.Embeddings.Sparse[0]
		result.Sparse = &core.SparseVector{
			Indices: apiSparse.Indices,
			Values:  apiSparse.Values,
			Shape:   apiSparse.Shape,
		}
	}

	return result, nil
}

// createDummyEmbedding generates a dummy embedding for fallback
// Returns core.EmbeddingVector now
func (e *BGEEmbedder) createDummyEmbedding() core.EmbeddingVector {
	denseDim := e.milvusService.GetEmbeddingDim()
	denseVector := make([]float32, denseDim)

	for i := 0; i < denseDim; i++ {
		denseVector[i] = float32(i%10) * 0.1
	}

	result := core.EmbeddingVector{
		Dense: denseVector,
	}

	// Add sparse embedding if supported
	if e.milvusService.SupportsSparseEmbeddings() {
		sparseDim := e.milvusService.GetSparseEmbeddingDim()
		indices := []int{1, 42, 100, 500, 1000}
		values := []float32{0.5, 0.3, 0.7, 0.1, 0.9}

		result.Sparse = &core.SparseVector{
			Indices: indices,
			Values:  values,
			Shape:   []int{1, sparseDim},
		}

		log.Printf("Created dummy embedding vectors: dense dim=%d, sparse nnz=%d",
			denseDim, len(values))
	} else {
		log.Printf("Created dummy embedding vector with dimension %d (dense only)", denseDim)
	}

	return result
}

// EmbedResponse's SparseVector needs to match core definition if used directly
// (or be mapped like in EmbedQuery)
type SparseVector struct { // This definition might be redundant now if EmbedResponse uses core.SparseVector
	Indices []int     `json:"indices"`
	Values  []float32 `json:"values"`
	Shape   []int     `json:"shape"`
}
