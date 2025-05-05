package rag

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/hunterwarburton/ya8hoda/internal/core"
	"github.com/milvus-io/milvus/client/v2/column"
	"github.com/milvus-io/milvus/client/v2/entity"
	"github.com/milvus-io/milvus/client/v2/index"
	"github.com/milvus-io/milvus/client/v2/milvusclient"
	// "github.com/milvus-io/milvus/client/v2/search" // Removed, assuming results are in milvusclient or entity
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

	// New fields for RAG facts
	FieldOwnerID     = "owner_id"
	FieldName        = "name"
	FieldDenseVector = "dense"
	FieldCreatedAt   = "created_at"
)

// Default constants specific to MilvusClient

// MilvusClient implements core.RAGService
type MilvusClient struct {
	client       *milvusclient.Client
	embed        core.EmbedService
	embeddingDim int
}

// Document definition removed (moved to internal/core)
// SearchResult definition removed (moved to internal/core)
// Fact definition removed (can be local if only used here, but keep for consistency for now)
type Fact struct {
	ID        string                 `json:"id"`
	OwnerID   string                 `json:"owner_id"`
	Name      string                 `json:"name,omitempty"`
	Text      string                 `json:"text"`
	CreatedAt int64                  `json:"created_at"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// NewMilvusClient creates a new instance of MilvusClient
func NewMilvusClient(ctx context.Context, addr string, embeddingDim int, freshStart bool, embedSvc core.EmbedService) (*MilvusClient, error) {
	log.Printf("Connecting to Milvus at %s with dimension %d (freshStart=%v)", addr, embeddingDim, freshStart)

	if embedSvc == nil {
		return nil, fmt.Errorf("EmbedService cannot be nil")
	}

	// Set default dimension if not specified
	if embeddingDim <= 0 {
		embeddingDim = 1024 // DefaultEmbeddingDim
	}

	// Create a Milvus client
	c, err := milvusclient.New(ctx, &milvusclient.ClientConfig{
		Address: addr,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Milvus: %w", err)
	}

	milvusClient := &MilvusClient{
		client:       c,
		embed:        embedSvc,
		embeddingDim: embeddingDim,
	}

	// Ensure all collections exist using the function from milvus_collections.go
	if err := EnsureAllCollectionsWithOptions(ctx, c, embeddingDim, freshStart); err != nil {
		return nil, fmt.Errorf("failed to ensure collections: %w", err)
	}

	log.Println("Successfully initialized Milvus client with all required collections")

	return milvusClient, nil
}

// ensureSingleCollection ensures a specific collection exists with the given schema and vector field.
// Note: This function seems redundant now as EnsureAllCollectionsWithOptions handles creation.
// Consider removing it if EnsureAllCollectionsWithOptions covers all creation/ensure logic.
func (c *MilvusClient) ensureSingleCollection(ctx context.Context, collectionName string, schema *entity.Schema, vectorFieldName string) error {
	hasOpt := milvusclient.NewHasCollectionOption(collectionName)
	exists, err := c.client.HasCollection(ctx, hasOpt)
	if err != nil {
		return fmt.Errorf("failed to check if collection %s exists: %w", collectionName, err)
	}

	if !exists {
		log.Printf("Collection %s does not exist. Creating...", collectionName)
		// Create the collection
		createOpt := milvusclient.NewCreateCollectionOption(collectionName, schema)
		createOpt.WithShardNum(1) // Simple setup with 1 shard
		err = c.client.CreateCollection(ctx, createOpt)
		if err != nil {
			return fmt.Errorf("failed to create collection %s: %w", collectionName, err)
		}
		log.Printf("Successfully created collection %s", collectionName)

		// Create index on the vector field if it exists in the schema
		hasVectorField := false
		for _, field := range schema.Fields {
			if field.Name == vectorFieldName && field.DataType == entity.FieldTypeFloatVector {
				hasVectorField = true
				break
			}
		}

		if hasVectorField {
			log.Printf("Creating HNSW index on field %s for collection %s", vectorFieldName, collectionName)
			idx := index.NewHNSWIndex(entity.L2, 16, 200) // Example HNSW parameters
			indexOpt := milvusclient.NewCreateIndexOption(collectionName, vectorFieldName, idx)
			_, err = c.client.CreateIndex(ctx, indexOpt)
			if err != nil {
				// Attempt to drop collection if index creation fails
				dropOpt := milvusclient.NewDropCollectionOption(collectionName)
				_ = c.client.DropCollection(ctx, dropOpt) // Ignore error during cleanup
				return fmt.Errorf("failed to create index on field %s for collection %s: %w", vectorFieldName, collectionName, err)
			}
			log.Printf("Successfully created index on field %s for collection %s", vectorFieldName, collectionName)
		} else {
			log.Printf("No float vector field named %s found in schema for %s. Skipping index creation.", vectorFieldName, collectionName)
		}

		// Load collection into memory
		log.Printf("Loading collection %s", collectionName)
		loadOpt := milvusclient.NewLoadCollectionOption(collectionName)
		_, err = c.client.LoadCollection(ctx, loadOpt)
		if err != nil {
			// Attempt to drop collection if load fails
			dropOpt := milvusclient.NewDropCollectionOption(collectionName)
			_ = c.client.DropCollection(ctx, dropOpt) // Ignore error during cleanup
			return fmt.Errorf("failed to load collection %s: %w", collectionName, err)
		}
		log.Printf("Successfully loaded collection %s", collectionName)
	} else {
		log.Printf("Collection %s already exists. Ensuring it is loaded.", collectionName)
		// Ensure collection is loaded if it already exists
		descOpt := milvusclient.NewDescribeCollectionOption(collectionName)
		collDesc, err := c.client.DescribeCollection(ctx, descOpt)
		if err != nil {
			return fmt.Errorf("failed to describe existing collection %s: %w", collectionName, err)
		}
		if !collDesc.Loaded {
			log.Printf("Collection %s exists but is not loaded. Loading...", collectionName)
			loadOpt := milvusclient.NewLoadCollectionOption(collectionName)
			_, err = c.client.LoadCollection(ctx, loadOpt)
			if err != nil {
				return fmt.Errorf("failed to load existing collection %s: %w", collectionName, err)
			}
			log.Printf("Successfully loaded existing collection %s", collectionName)
		} else {
			log.Printf("Collection %s is already loaded.", collectionName)
		}
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

	// Prepare row data using the row-based insert option
	row := map[string]interface{}{
		FieldID:         docID,
		FieldText:       text,
		FieldTitle:      title,
		FieldSource:     source,
		FieldMetadata:   metadataStr,
		FieldCreateTime: time.Now().Unix(),
		FieldVector:     vector,
	}

	// Insert data using row-based insert
	insertOpt := milvusclient.NewRowBasedInsertOption(DocumentCollection, row)
	result, err := c.client.Insert(ctx, insertOpt)
	if err != nil {
		return "", fmt.Errorf("failed to insert document: %w", err)
	}

	if result.InsertCount != 1 {
		log.Printf("Warning: Expected to insert 1 document, but inserted %d", result.InsertCount)
	}

	return docID, nil
}

// SearchSimilar searches for documents similar to the provided vector in the DocumentCollection
func (c *MilvusClient) SearchSimilar(ctx context.Context, vector []float32, k int, filter string) ([]core.SearchResult, error) {
	if k <= 0 {
		k = 5
	}
	outputFields := []string{FieldID, FieldText, FieldTitle, FieldSource, FieldCreateTime, FieldMetadata}

	// Prepare search request using the builder pattern (Attempt 6 - based on examples)
	searchVectors := []entity.Vector{entity.FloatVector(vector)}
	searchOpt := milvusclient.NewSearchOption(DocumentCollection, int(k), searchVectors).
		WithFilter(filter) // Apply filter expression
	// Add output fields one by one (or check if variadic works)
	for _, field := range outputFields {
		searchOpt.WithOutputFields(field)
	}
	// Omit WithPartitions to search all partitions
	// searchOpt.WithPartitions("specific_partition") // Example for specific partition
	// MetricType might be inferred or set via index params.
	// Search Params might be set via WithAnnParam if needed.
	// searchOpt.WithAnnParam(index.NewCustomAnnParam().WithExtraParam("ef", 72)) // Example for search params
	// searchOpt.WithConsistencyLevel(entity.ClStrong)

	// Perform the search
	searchResults, err := c.client.Search(ctx, searchOpt)
	if err != nil {
		// Collection name is retrieved from the option object if needed
		return nil, fmt.Errorf("failed to search collection: %w", err)
	}

	// Process results - Assuming searchResults is []ResultSet
	if len(searchResults) == 0 {
		return []core.SearchResult{}, nil // No results found
	}

	// Use the first result set (since we searched with one vector)
	firstResultSet := searchResults[0]

	// Check the result count for the first result set
	if firstResultSet.ResultCount == 0 {
		return []core.SearchResult{}, nil // No results found
	}

	// Pass the actual ResultSet, not a pointer
	// For DocumentCollection, use the standard FieldOwnerID/FieldName as placeholders,
	// as this collection doesn't have those fields structured like the facts collections.
	// parseSearchResults will handle nil columns gracefully.
	parsedResults, err := parseSearchResults(&firstResultSet, core.Document{}, FieldOwnerID, FieldName)
	if err != nil {
		return nil, fmt.Errorf("failed to parse search results for: %w", err)
	}

	return parsedResults, nil
}

// Close closes the connection to Milvus
func (c *MilvusClient) Close() error {
	return c.client.Close(context.Background())
}

// GetEmbeddingDim returns the dimensionality of embeddings
func (c *MilvusClient) GetEmbeddingDim() int {
	return c.embeddingDim
}

// GetSparseEmbeddingDim returns the dimensionality of sparse embeddings
func (c *MilvusClient) GetSparseEmbeddingDim() int {
	// BGE-M3 uses 250002 dimensions for its sparse embeddings
	return 250002
}

// SupportsSparseEmbeddings returns whether sparse embeddings are supported
func (c *MilvusClient) SupportsSparseEmbeddings() bool {
	// We're now supporting sparse embeddings with hybrid search
	return true
}

// GetClient returns the underlying Milvus client
func (c *MilvusClient) GetClient() *milvusclient.Client {
	return c.client
}

// StoreFact stores a fact with potentially hybrid vectors.
// Replaces the old dense-only StoreFact and StoreFactWithHybridVector.
func (c *MilvusClient) StoreFact(ctx context.Context, collection, ownerID, name, text string,
	metadata map[string]interface{}, denseVector []float32, sparseIndices []int,
	sparseValues []float32, sparseShape []int) (string, error) {

	// Generate a unique ID for the fact
	factID := fmt.Sprintf("fact_%d", time.Now().UnixNano())
	now := time.Now().Unix()

	// Create a sparse vector representation using Milvus entity type
	var sparseVecData entity.SparseEmbedding
	if len(sparseIndices) > 0 && len(sparseValues) > 0 && c.SupportsSparseEmbeddings() {
		uindices := make([]uint32, len(sparseIndices))
		for i, v := range sparseIndices {
			uindices[i] = uint32(v)
		}
		var err error
		sparseVecData, err = entity.NewSliceSparseEmbedding(uindices, sparseValues)
		if err != nil {
			return "", fmt.Errorf("failed to create sparse embedding entity: %w", err)
		}
	} else {
		sparseVecData, _ = entity.NewSliceSparseEmbedding([]uint32{}, []float32{}) // Empty sparse vector
		if c.SupportsSparseEmbeddings() {
			log.Printf("Warning: No sparse data provided or sparse not supported for fact %s, using empty sparse vector.", factID)
		}
	}

	// Prepare row data
	row := map[string]interface{}{}
	row[FieldID] = factID

	// Use correct field names based on collection
	if collection == PeopleFactsCollection {
		row["telegram_id"] = ownerID // Use "telegram_id", value comes from telegramID parameter
		row["telegram_name"] = name  // Use "telegram_name", value comes from telegramName parameter
	} else {
		row[FieldOwnerID] = ownerID // Use standard FieldOwnerID for other collections
		row[FieldName] = name       // Use standard FieldName for other collections
	}

	row[FieldText] = text
	row[FieldDenseVector] = denseVector
	if c.SupportsSparseEmbeddings() {
		row["sparse"] = sparseVecData // Assuming field name is "sparse"
	}
	row[FieldCreatedAt] = now
	if metadata == nil {
		row[FieldMetadata] = map[string]interface{}{} // Use empty map instead of nil
	} else {
		row[FieldMetadata] = metadata // Pass the original map directly
	}

	// Insert data
	insertOpt := milvusclient.NewRowBasedInsertOption(collection, row)
	_, err := c.client.Insert(ctx, insertOpt)
	if err != nil {
		// Log the row data for debugging (excluding potentially large vectors)
		debugRow := make(map[string]interface{})
		for k, v := range row {
			if k != FieldDenseVector && k != "sparse" {
				debugRow[k] = v
			}
		}
		log.Printf("Error inserting row: %v\nRow data (partial): %+v", err, debugRow)
		return "", fmt.Errorf("failed to insert fact: %w", err)
	}

	return factID, nil
}

// StorePersonFact stores a fact about a person using hybrid vectors.
// Renamed ownerID -> telegramID, name -> telegramName
func (c *MilvusClient) StorePersonFact(ctx context.Context, telegramID, telegramName, text string, metadata map[string]interface{}, denseVector []float32, sparseIndices []int, sparseValues []float32, sparseShape []int) (string, error) {
	// Pass telegramID as ownerID and telegramName as name to the generic StoreFact
	return c.StoreFact(ctx, PeopleFactsCollection, telegramID, telegramName, text, metadata, denseVector, sparseIndices, sparseValues, sparseShape)
}

// StoreCommunityFact stores a fact about a community using hybrid vectors.
func (c *MilvusClient) StoreCommunityFact(ctx context.Context, ownerID, name, text string, metadata map[string]interface{}, denseVector []float32, sparseIndices []int, sparseValues []float32, sparseShape []int) (string, error) {
	return c.StoreFact(ctx, CommunityFactsCollection, ownerID, name, text, metadata, denseVector, sparseIndices, sparseValues, sparseShape)
}

// StoreBotFact stores a bot fact in the dedicated collection.
// It uses the hybrid embedding service to get both dense and sparse vectors.
func (c *MilvusClient) StoreBotFact(ctx context.Context, name, text string, metadata map[string]interface{}, embedding core.EmbeddingVector) (string, error) {
	// Extract components from the embedding
	denseVector := embedding.Dense
	sparseIndices := embedding.Sparse.Indices
	sparseValues := embedding.Sparse.Values
	sparseShape := embedding.Sparse.Shape

	if denseVector == nil || len(denseVector) == 0 {
		return "", fmt.Errorf("dense vector is required for StoreBotFact")
	}
	if sparseIndices == nil || sparseValues == nil || sparseShape == nil {
		// Handle cases where sparse vector might be missing, depending on requirements.
		// Option 1: Error out
		return "", fmt.Errorf("sparse vector components are required for StoreBotFact")
		// Option 2: Log a warning and proceed without sparse (if schema allows)
		// log.Printf("Warning: Sparse vector components missing for fact: %s", text[:50])
		// sparseIndices = []int{}
		// sparseValues = []float32{}
		// sparseShape = []int{0} // Or appropriate default/empty shape
	}

	return c.StoreFact(ctx, BotFactsCollection, "", name, text, metadata, denseVector, sparseIndices, sparseValues, sparseShape)
}

// SearchFacts performs a search, potentially using hybrid vectors if available.
// Uses client.HybridSearch with HybridSearchOption builder to specify all parameters.
func (c *MilvusClient) SearchFacts(ctx context.Context, collection string,
	denseVector []float32, sparseData entity.SparseEmbedding, // VALUE passed here
	k int, filter string) ([]core.SearchResult, error) {
	log.Printf("Starting Hybrid SearchFacts for collection: %s, k=%d, filter='%s'", collection, k, filter)
	if k <= 0 {
		k = 5 // Default k
	}

	// Define output fields based on the collection
	var outputFields []string
	var ownerIDFieldName, nameFieldName string // Variables to hold the correct field names for parsing

	if collection == PeopleFactsCollection {
		ownerIDFieldName = "telegram_id"
		nameFieldName = "telegram_name"
		outputFields = []string{
			FieldID,          // "id"
			ownerIDFieldName, // "telegram_id"
			nameFieldName,    // "telegram_name"
			FieldText,
			FieldCreatedAt,
			FieldMetadata,
		}
	} else {
		ownerIDFieldName = FieldOwnerID // "owner_id"
		nameFieldName = FieldName       // "name"
		outputFields = []string{
			FieldID,
			FieldOwnerID, // Use constant for other collections
			FieldName,    // Use constant for other collections
			FieldText,
			FieldCreatedAt,
			FieldMetadata,
		}
	}

	// --- Prepare AnnRequests ---+

	// 1. Dense ANN Request (always created)
	denseReq := milvusclient.NewAnnRequest(FieldDenseVector, k, entity.FloatVector(denseVector)).
		WithFilter(filter) // Apply filter to dense request
	// Add specific search params for dense if needed, e.g.:
	// denseReq.WithSearchParam("ef", "64") // Example HNSW parameter

	// 2. Sparse ANN Request (conditional)
	var hybridOpt milvusclient.HybridSearchOption // Use interface type
	numRequests := 1
	if c.SupportsSparseEmbeddings() && sparseData != nil && sparseData.Dim() > 0 { // Check Dim > 0
		log.Println("Creating Sparse ANN Request")
		sparseReq := milvusclient.NewAnnRequest("sparse", k, sparseData). // Assuming sparse field name is "sparse"
											WithFilter(filter) // Apply filter to sparse request
		// Add specific search params for sparse if needed, e.g., weight:
		// sparseReq.WithSearchParam("weight", "0.5") // Example weight

		// Create HybridSearchOption with BOTH requests
		hybridOpt = milvusclient.NewHybridSearchOption(collection, k, denseReq, sparseReq).
			WithOutputFields(outputFields...)
		// Add reranker if needed (e.g., RRFRanker)
		// hybridOpt.WithReranker(milvusclient.NewRRFRanker(60))
		numRequests = 2
	} else {
		log.Println("Skipping Sparse ANN Request (not supported or no data)")
		// Create HybridSearchOption with ONLY dense request
		hybridOpt = milvusclient.NewHybridSearchOption(collection, k, denseReq).
			WithOutputFields(outputFields...)
	}

	// Optionally add other parameters like consistency level AFTER creation
	// hybridOpt.WithConsistencyLevel(entity.ClBounded) // This method might not exist on the interface

	log.Printf("Searching collection '%s' with filter '%s' (k=%d) using client.HybridSearch with %d AnnRequests", collection, filter, k, numRequests)

	// Perform the search using the client.HybridSearch method
	searchResults, err := c.client.HybridSearch(ctx, hybridOpt)
	if err != nil {
		return nil, fmt.Errorf("failed to execute hybrid search on collection '%s': %w", collection, err)
	}

	// Process results (Search returns []ResultSet)
	if len(searchResults) == 0 {
		return []core.SearchResult{}, nil // No results found
	}
	firstResultSet := searchResults[0]
	if firstResultSet.ResultCount == 0 {
		return []core.SearchResult{}, nil // No results found
	}

	// Parse results into core.SearchResult format
	// Pass the correct field names for owner ID and name to the parser
	parsedResults, err := parseSearchResults(&firstResultSet, core.Document{}, ownerIDFieldName, nameFieldName)
	if err != nil {
		return nil, fmt.Errorf("failed to parse search results for collection '%s': %w", collection, err)
	}

	return parsedResults, nil
}

// SearchPersonFacts searches the people facts collection using hybrid vectors.
func (c *MilvusClient) SearchPersonFacts(ctx context.Context, denseVector []float32, sparseData entity.SparseEmbedding, k int, filter string) ([]core.SearchResult, error) {
	return c.SearchFacts(ctx, PeopleFactsCollection, denseVector, sparseData, k, filter)
}

// SearchCommunityFacts searches the community facts collection using hybrid vectors.
func (c *MilvusClient) SearchCommunityFacts(ctx context.Context, denseVector []float32, sparseData entity.SparseEmbedding, k int, filter string) ([]core.SearchResult, error) {
	return c.SearchFacts(ctx, CommunityFactsCollection, denseVector, sparseData, k, filter)
}

// SearchBotFacts searches the bot facts collection using hybrid vectors.
func (c *MilvusClient) SearchBotFacts(ctx context.Context, denseVector []float32, sparseData entity.SparseEmbedding, k int, filter string) ([]core.SearchResult, error) {
	return c.SearchFacts(ctx, BotFactsCollection, denseVector, sparseData, k, filter)
}

// SearchAllFacts searches all facts collections using hybrid vectors.
func (c *MilvusClient) SearchAllFacts(ctx context.Context, denseVector []float32, sparseData entity.SparseEmbedding, k int) ([]core.SearchResult, error) {
	if k <= 0 {
		k = 5
	}
	perCollectionK := (k + 2) / 3 // Round up

	peopleResults, err := c.SearchPersonFacts(ctx, denseVector, sparseData, perCollectionK, "")
	if err != nil {
		return nil, fmt.Errorf("failed to search people facts: %w", err)
	}
	communityResults, err := c.SearchCommunityFacts(ctx, denseVector, sparseData, perCollectionK, "")
	if err != nil {
		return nil, fmt.Errorf("failed to search community facts: %w", err)
	}
	botResults, err := c.SearchBotFacts(ctx, denseVector, sparseData, perCollectionK, "")
	if err != nil {
		return nil, fmt.Errorf("failed to search bot facts: %w", err)
	}

	allResults := append(peopleResults, communityResults...)
	allResults = append(allResults, botResults...)

	// Sort by score (descending) - Implement proper sorting
	// sort.Slice(allResults, func(i, j int) bool { return allResults[i].Score > allResults[j].Score })

	if len(allResults) > k {
		allResults = allResults[:k]
	}
	return allResults, nil
}

// RememberAboutSelf stores a fact about the bot
func (c *MilvusClient) RememberAboutSelf(ctx context.Context, text string, metadata map[string]interface{}) (string, error) {
	name := "Bot"
	embedding, err := c.embed.EmbedQuery(ctx, text)
	if err != nil {
		return "", fmt.Errorf("failed to generate embedding for self memory: %w", err)
	}

	if embedding.Dense == nil || embedding.Sparse == nil {
		return "", fmt.Errorf("embedding service returned incomplete data (missing dense or sparse)")
	}

	// We no longer need to manually extract sparse components here
	// as StoreBotFact now accepts the full EmbeddingVector struct.
	// sparseIndices := embedding.Sparse.Indices
	// sparseValues := embedding.Sparse.Values
	// sparseShape := embedding.Sparse.Shape

	// Call the updated StoreBotFact
	factID, err := c.StoreBotFact(ctx, name, text, metadata, embedding)
	if err != nil {
		return "", err
	}

	log.Printf("Stored self memory with ID: %s", factID)
	return factID, nil
}

// RememberAboutPerson stores a fact about a person identified by telegramID and telegramName
// Renamed personID -> telegramID, personName -> telegramName
func (c *MilvusClient) RememberAboutPerson(ctx context.Context, telegramID string, telegramName string, text string, metadata map[string]interface{}) (string, error) {
	// Generate embedding
	embedding, err := c.embed.EmbedQuery(ctx, text)
	if err != nil {
		return "", fmt.Errorf("failed to generate embedding for person memory: %w", err)
	}

	if embedding.Dense == nil || embedding.Sparse == nil {
		return "", fmt.Errorf("embedding service returned incomplete data (missing dense or sparse)")
	}

	// Prepare sparse data components
	sparseIndicesInt := make([]int, len(embedding.Sparse.Indices))
	for i, v := range embedding.Sparse.Indices {
		sparseIndicesInt[i] = int(v) // Assuming indices fit in int
	}
	sparseValues := embedding.Sparse.Values
	sparseShape := embedding.Sparse.Shape

	// Call the updated StorePersonFact
	return c.StorePersonFact(ctx, telegramID, telegramName, text, metadata, embedding.Dense, sparseIndicesInt, sparseValues, sparseShape)
}

// RememberAboutCommunity stores a fact about a community
func (c *MilvusClient) RememberAboutCommunity(ctx context.Context, communityID string, text string, metadata map[string]interface{}) (string, error) {
	name := "Community"
	embedding, err := c.embed.EmbedQuery(ctx, text)
	if err != nil {
		return "", fmt.Errorf("failed to generate embedding for community memory: %w", err)
	}

	var sparseIndices []int
	var sparseValues []float32
	var sparseShape []int
	if embedding.Sparse != nil {
		sparseIndices = embedding.Sparse.Indices
		sparseValues = embedding.Sparse.Values
		sparseShape = embedding.Sparse.Shape
	}

	factID, err := c.StoreCommunityFact(ctx, communityID, name, text, metadata, embedding.Dense, sparseIndices, sparseValues, sparseShape)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Memory about community %s stored successfully with ID: %s", communityID, factID), nil
}

// SearchAllMemories searches all facts collections
func (c *MilvusClient) SearchAllMemories(ctx context.Context, query string, k int) ([]core.SearchResult, error) {
	embedding, err := c.embed.EmbedQuery(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to generate embedding for search query: %w", err)
	}
	// Prepare sparse data for search, checking for validity first
	var sparseForSearch entity.SparseEmbedding
	if embedding.Sparse != nil && len(embedding.Sparse.Indices) > 0 && len(embedding.Sparse.Values) > 0 {
		uindices := make([]uint32, len(embedding.Sparse.Indices))
		for i, v := range embedding.Sparse.Indices {
			uindices[i] = uint32(v)
		}
		spData, spErr := entity.NewSliceSparseEmbedding(uindices, embedding.Sparse.Values)
		if spErr != nil {
			log.Printf("Warning: failed to create sparse embedding entity for search: %v. Proceeding with dense only.", spErr)
			// Create an empty one to pass down, SearchFacts will handle it
			sparseForSearch, _ = entity.NewSliceSparseEmbedding([]uint32{}, []float32{})
		} else {
			sparseForSearch = spData
		}
	} else {
		// Create an empty one if no sparse data was generated
		sparseForSearch, _ = entity.NewSliceSparseEmbedding([]uint32{}, []float32{})
	}

	return c.SearchAllFacts(ctx, embedding.Dense, sparseForSearch, k)
}

// SearchSelfMemory searches the bot's facts
func (c *MilvusClient) SearchSelfMemory(ctx context.Context, query string, k int) ([]core.SearchResult, error) {
	embedding, err := c.embed.EmbedQuery(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to generate embedding for self memory query: %w", err)
	}
	// Prepare sparse data for search, checking for validity first
	var sparseForSearch entity.SparseEmbedding
	if embedding.Sparse != nil && len(embedding.Sparse.Indices) > 0 && len(embedding.Sparse.Values) > 0 {
		uindices := make([]uint32, len(embedding.Sparse.Indices))
		for i, v := range embedding.Sparse.Indices {
			uindices[i] = uint32(v)
		}
		spData, spErr := entity.NewSliceSparseEmbedding(uindices, embedding.Sparse.Values)
		if spErr != nil {
			log.Printf("Warning: failed to create sparse embedding entity for search: %v. Proceeding with dense only.", spErr)
			sparseForSearch, _ = entity.NewSliceSparseEmbedding([]uint32{}, []float32{})
		} else {
			sparseForSearch = spData
		}
	} else {
		sparseForSearch, _ = entity.NewSliceSparseEmbedding([]uint32{}, []float32{})
	}
	return c.SearchBotFacts(ctx, embedding.Dense, sparseForSearch, k, "")
}

// SearchPersonalMemory searches a person's facts, allowing filtering by telegramID and/or telegramName.
// Renamed personID -> telegramID, personName -> telegramName
func (c *MilvusClient) SearchPersonalMemory(ctx context.Context, query, telegramID, telegramName string, k int) ([]core.SearchResult, error) {
	embedding, err := c.embed.EmbedQuery(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to generate embedding for personal memory query: %w", err)
	}

	// Build the filter expression using the correct field names for people_facts
	var filters []string
	if telegramID != "" {
		// Use the actual field name "telegram_id" for filtering this specific collection
		filters = append(filters, fmt.Sprintf("telegram_id == \"%s\"", telegramID))
	}
	if telegramName != "" {
		// Use the actual field name "telegram_name" for filtering this specific collection
		// Using 'like' for prefix matching as before.
		// Ensure telegramName is escaped for safety if needed.
		filters = append(filters, fmt.Sprintf("telegram_name like \"%s%%\"", telegramName))
		// Or for exact match:
		// filters = append(filters, fmt.Sprintf("telegram_name == \"%s\"", telegramName))
	}

	filter := strings.Join(filters, " and ") // Combine filters with 'and'
	log.Printf("Constructed filter for SearchPersonalMemory: %s", filter)

	// Prepare sparse data for search, checking for validity first
	var sparseForSearch entity.SparseEmbedding
	if embedding.Sparse != nil && len(embedding.Sparse.Indices) > 0 && len(embedding.Sparse.Values) > 0 {
		uindices := make([]uint32, len(embedding.Sparse.Indices))
		for i, v := range embedding.Sparse.Indices {
			uindices[i] = uint32(v)
		}
		spData, spErr := entity.NewSliceSparseEmbedding(uindices, embedding.Sparse.Values)
		if spErr != nil {
			log.Printf("Warning: failed to create sparse embedding entity for search: %v. Proceeding with dense only.", spErr)
			sparseForSearch, _ = entity.NewSliceSparseEmbedding([]uint32{}, []float32{})
		} else {
			sparseForSearch = spData
		}
	} else {
		sparseForSearch, _ = entity.NewSliceSparseEmbedding([]uint32{}, []float32{})
	}

	return c.SearchPersonFacts(ctx, embedding.Dense, sparseForSearch, k, filter)
}

// SearchCommunityMemory searches a community's facts
func (c *MilvusClient) SearchCommunityMemory(ctx context.Context, query, communityID string, k int) ([]core.SearchResult, error) {
	embedding, err := c.embed.EmbedQuery(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to generate embedding for community memory query: %w", err)
	}
	filter := ""
	if communityID != "" {
		filter = fmt.Sprintf("%s == \"%s\"", FieldOwnerID, communityID)
	}
	// Prepare sparse data for search, checking for validity first
	var sparseForSearch entity.SparseEmbedding
	if embedding.Sparse != nil && len(embedding.Sparse.Indices) > 0 && len(embedding.Sparse.Values) > 0 {
		uindices := make([]uint32, len(embedding.Sparse.Indices))
		for i, v := range embedding.Sparse.Indices {
			uindices[i] = uint32(v)
		}
		spData, spErr := entity.NewSliceSparseEmbedding(uindices, embedding.Sparse.Values)
		if spErr != nil {
			log.Printf("Warning: failed to create sparse embedding entity for search: %v. Proceeding with dense only.", spErr)
			sparseForSearch, _ = entity.NewSliceSparseEmbedding([]uint32{}, []float32{})
		} else {
			sparseForSearch = spData
		}
	} else {
		sparseForSearch, _ = entity.NewSliceSparseEmbedding([]uint32{}, []float32{})
	}
	return c.SearchCommunityFacts(ctx, embedding.Dense, sparseForSearch, k, filter)
}

// Helper type for column access compatibility
type columnProvider interface {
	GetColumn(name string) column.Column
	Scores() []float32
}

// Helper function to parse search results
// Updated to return []core.SearchResult and use core.Document
func parseSearchResults(resultSet *milvusclient.ResultSet, baseDocType core.Document, ownerIDFieldName, nameFieldName string) ([]core.SearchResult, error) {
	// Get result count
	resultCount := resultSet.ResultCount
	if resultCount == 0 {
		return []core.SearchResult{}, nil
	}

	// In Milvus 2.5.x, scores is just a standard []float32 array field in the ResultSet
	// Accessing directly rather than through a method
	scores := resultSet.Scores
	if len(scores) == 0 {
		return []core.SearchResult{}, nil
	}

	results := make([]core.SearchResult, 0, len(scores))

	// Extract data from the columns
	idCol := resultSet.GetColumn(FieldID)
	ownerIDCol := resultSet.GetColumn(ownerIDFieldName)
	nameCol := resultSet.GetColumn(nameFieldName)
	textCol := resultSet.GetColumn(FieldText)
	timeCol := resultSet.GetColumn(FieldCreatedAt)
	metadataCol := resultSet.GetColumn(FieldMetadata)

	// Process each result row
	for i := 0; i < len(scores); i++ {
		doc := baseDocType // Start with the base document type

		// Extract and set fields
		if idCol != nil && idCol.Len() > i {
			if val, err := idCol.GetAsString(i); err == nil {
				doc.ID = val
			}
		}

		if ownerIDCol != nil && ownerIDCol.Len() > i {
			if val, err := ownerIDCol.GetAsString(i); err == nil {
				doc.OwnerID = val
			}
		}

		if nameCol != nil && nameCol.Len() > i {
			if val, err := nameCol.GetAsString(i); err == nil {
				doc.Name = val
			}
		}

		if textCol != nil && textCol.Len() > i {
			if val, err := textCol.GetAsString(i); err == nil {
				doc.Text = val
			}
		}

		if timeCol != nil && timeCol.Len() > i {
			if val, err := timeCol.GetAsInt64(i); err == nil {
				doc.CreateTime = val
			}
		}

		if metadataCol != nil && metadataCol.Len() > i {
			if val, err := metadataCol.GetAsString(i); err == nil && val != "" && val != "null" {
				var metadata map[string]interface{}
				if err := json.Unmarshal([]byte(val), &metadata); err == nil {
					doc.Metadata = metadata
				}
			}
		}

		// Add to results
		results = append(results, core.SearchResult{
			Document: doc,
			Score:    scores[i],
		})
	}

	return results, nil
}
