package rag

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/milvus-io/milvus-sdk-go/v2/client"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"
)

// Field names for Milvus collection
const (
	FieldID         = "id"
	FieldText       = "text"
	FieldTitle      = "title"
	FieldSource     = "source"
	FieldMetadata   = "metadata"
	FieldCreateTime = "create_time"
	FieldVector     = "vector"
)

// Collection name
const (
	CollectionName = "documents"
)

// MilvusClient is a wrapper around the Milvus client to provide better compatibility
type MilvusClient struct {
	client       client.Client
	embeddingDim int
}

// NewMilvusClient creates a new instance of MilvusClient
func NewMilvusClient(ctx context.Context, addr string, embeddingDim int) (*MilvusClient, error) {
	log.Printf("Connecting to Milvus at %s with dimension %d", addr, embeddingDim)

	// Set default dimension if not specified
	if embeddingDim <= 0 {
		embeddingDim = DefaultEmbeddingDim
	}

	// Create a Milvus client
	c, err := client.NewClient(ctx, client.Config{
		Address: addr,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Milvus: %w", err)
	}

	return &MilvusClient{
		client:       c,
		embeddingDim: embeddingDim,
	}, nil
}

// EnsureCollection ensures the vector collection exists with the correct schema
func (c *MilvusClient) EnsureCollection(ctx context.Context, collectionName string) error {
	// Check if collection exists
	exists, err := c.client.HasCollection(ctx, collectionName)
	if err != nil {
		return fmt.Errorf("failed to check if collection exists: %w", err)
	}

	// If collection doesn't exist, create it
	if !exists {
		// Define schema
		schema := &entity.Schema{
			CollectionName: collectionName,
			Description:    "Document vectors for RAG",
			Fields: []*entity.Field{
				{
					Name:       FieldID,
					DataType:   entity.FieldTypeVarChar,
					PrimaryKey: true,
					AutoID:     false,
				},
				{
					Name:     FieldText,
					DataType: entity.FieldTypeVarChar,
				},
				{
					Name:     FieldTitle,
					DataType: entity.FieldTypeVarChar,
				},
				{
					Name:     FieldSource,
					DataType: entity.FieldTypeVarChar,
				},
				{
					Name:     FieldMetadata,
					DataType: entity.FieldTypeJSON,
				},
				{
					Name:     FieldCreateTime,
					DataType: entity.FieldTypeInt64,
				},
				{
					Name:     FieldVector,
					DataType: entity.FieldTypeFloatVector,
					TypeParams: map[string]string{
						"dim": fmt.Sprintf("%d", c.embeddingDim),
					},
				},
			},
		}

		// Create the collection
		err = c.client.CreateCollection(ctx, schema, 1)
		if err != nil {
			return fmt.Errorf("failed to create collection: %w", err)
		}

		// Create index on vector field
		idx, err := entity.NewIndexHNSW(entity.L2, 16, 200)
		if err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}

		err = c.client.CreateIndex(ctx, collectionName, FieldVector, idx, false)
		if err != nil {
			return fmt.Errorf("failed to create index on vector field: %w", err)
		}

		// Load collection
		err = c.client.LoadCollection(ctx, collectionName, false)
		if err != nil {
			return fmt.Errorf("failed to load collection: %w", err)
		}

		log.Printf("Created and loaded collection: %s", collectionName)
	}

	return nil
}

// StoreDocument stores a document in Milvus
func (c *MilvusClient) StoreDocument(ctx context.Context, text, title, source string, metadata map[string]interface{}, vector []float32) (string, error) {
	// Generate document ID
	docID := fmt.Sprintf("doc_%d", time.Now().UnixNano())

	// Convert metadata to JSON string (simplified)
	metadataStr := "{}"
	if metadata != nil {
		metadataBytes, _ := json.Marshal(metadata)
		metadataStr = string(metadataBytes)
	}

	// Prepare columns
	columns := []entity.Column{
		entity.NewColumnVarChar(FieldID, []string{docID}),
		entity.NewColumnVarChar(FieldText, []string{text}),
		entity.NewColumnVarChar(FieldTitle, []string{title}),
		entity.NewColumnVarChar(FieldSource, []string{source}),
		entity.NewColumnJSONBytes(FieldMetadata, [][]byte{[]byte(metadataStr)}),
		entity.NewColumnInt64(FieldCreateTime, []int64{time.Now().Unix()}),
		entity.NewColumnFloatVector(FieldVector, c.embeddingDim, [][]float32{vector}),
	}

	// Insert data
	_, err := c.client.Insert(ctx, CollectionName, "", columns...)
	if err != nil {
		return "", fmt.Errorf("failed to insert document: %w", err)
	}

	return docID, nil
}

// SearchSimilar searches for similar documents in Milvus
func (c *MilvusClient) SearchSimilar(ctx context.Context, vector []float32, k int, filter string) ([]SearchResult, error) {
	// Set default k if not provided
	if k <= 0 {
		k = 5
	}

	// Prepare search parameters
	sp, err := entity.NewIndexHNSWSearchParam(100)
	if err != nil {
		return nil, fmt.Errorf("failed to create search parameters: %w", err)
	}

	// Additional output fields to retrieve
	outputFields := []string{FieldID, FieldText, FieldTitle, FieldSource, FieldMetadata, FieldCreateTime}

	// Execute search query
	vectors := []entity.Vector{entity.FloatVector(vector)}
	result, err := c.client.Search(
		ctx,
		CollectionName, // Collection name
		[]string{},     // Partitions
		filter,         // Filter expression
		outputFields,   // Output fields
		vectors,        // Search vectors
		FieldVector,    // Vector field name
		entity.L2,      // Metric type
		k,              // TopK
		sp,             // Search parameters
	)
	if err != nil {
		return nil, fmt.Errorf("failed to execute search: %w", err)
	}

	// Check for empty results
	if len(result) == 0 || result[0].ResultCount == 0 {
		return []SearchResult{}, nil
	}

	// Process search results
	searchResult := result[0]
	var results []SearchResult

	for i := 0; i < searchResult.ResultCount; i++ {
		// Extract ID
		id, ok := searchResult.IDs.(*entity.ColumnVarChar)
		if !ok {
			continue
		}

		// Extract text
		texts, ok := searchResult.Fields.GetColumn(FieldText).(*entity.ColumnVarChar)
		if !ok {
			continue
		}

		// Extract title
		titles, ok := searchResult.Fields.GetColumn(FieldTitle).(*entity.ColumnVarChar)
		if !ok {
			// Title is optional
			titles = entity.NewColumnVarChar(FieldTitle, []string{""})
		}

		// Extract source
		sources, ok := searchResult.Fields.GetColumn(FieldSource).(*entity.ColumnVarChar)
		if !ok {
			// Source is optional
			sources = entity.NewColumnVarChar(FieldSource, []string{""})
		}

		// Extract metadata
		var metadata map[string]interface{}
		metadataCol, ok := searchResult.Fields.GetColumn(FieldMetadata).(*entity.ColumnJSONBytes)
		if ok && i < len(metadataCol.Data()) {
			// Parse metadata JSON
			json.Unmarshal(metadataCol.Data()[i], &metadata)
		} else {
			metadata = make(map[string]interface{})
		}

		// Extract creation time
		createTime := int64(0)
		createTimeCol, ok := searchResult.Fields.GetColumn(FieldCreateTime).(*entity.ColumnInt64)
		if ok && i < len(createTimeCol.Data()) {
			createTime = createTimeCol.Data()[i]
		}

		// Create document
		doc := Document{
			ID:         id.Data()[i],
			Text:       texts.Data()[i],
			Title:      titles.Data()[i],
			Source:     sources.Data()[i],
			Metadata:   metadata,
			CreateTime: createTime,
		}

		// Get score
		score := float32(0)
		if i < len(searchResult.Scores) {
			score = searchResult.Scores[i]
		}

		// Add to results
		results = append(results, SearchResult{
			Document: doc,
			Score:    score,
		})
	}

	return results, nil
}

// Close closes the connection to Milvus
func (c *MilvusClient) Close() error {
	return c.client.Close()
}

// GetEmbeddingDim returns the dimensionality of embeddings
func (c *MilvusClient) GetEmbeddingDim() int {
	return c.embeddingDim
}
