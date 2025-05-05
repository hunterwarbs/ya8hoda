package rag

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/milvus-io/milvus/client/v2/entity"
	"github.com/milvus-io/milvus/client/v2/index"
	"github.com/milvus-io/milvus/client/v2/milvusclient"
)

// Default constants reused across packages
const (
	DefaultMaxVarCharLength = "65535"
	DefaultIDMaxLength      = "255" // Max length for IDs (PKs)
)

// Collection names
const (
	DocumentCollection = "documents"

	// Facts collections
	PeopleFactsCollection    = "people_facts"
	CommunityFactsCollection = "community_facts"
	BotFactsCollection       = "bot_facts"
)

// EnsureAllCollections ensures all collections exist
func EnsureAllCollections(ctx context.Context, milvusClient *milvusclient.Client, embeddingDim int) error {
	return EnsureAllCollectionsWithOptions(ctx, milvusClient, embeddingDim, false)
}

// EnsureAllCollectionsWithOptions ensures all collections exist with additional options
func EnsureAllCollectionsWithOptions(ctx context.Context, milvusClient *milvusclient.Client, embeddingDim int, freshStart bool) error {
	log.Printf("EnsureAllCollectionsWithOptions called with freshStart=%v, embeddingDim=%d", freshStart, embeddingDim)

	if freshStart {
		log.Printf("Fresh start requested, dropping all existing collections...")
		// Get list of all collections
		listOpt := milvusclient.NewListCollectionOption()
		collections, err := milvusClient.ListCollections(ctx, listOpt)
		if err != nil {
			return fmt.Errorf("failed to list collections: %w", err)
		}

		log.Printf("Found %d collections to drop: %v", len(collections), collections)

		// Drop each collection
		for _, collName := range collections {
			log.Printf("Dropping collection: %s", collName)
			dropOpt := milvusclient.NewDropCollectionOption(collName)
			if err := milvusClient.DropCollection(ctx, dropOpt); err != nil {
				log.Printf("Error dropping collection %s: %v", collName, err)
				return fmt.Errorf("failed to drop collection %s: %w", collName, err)
			}
			log.Printf("Successfully dropped collection: %s", collName)
		}
		log.Printf("Successfully dropped all collections")

		// Add a small delay to ensure all cleanup operations complete
		delay := 2 * time.Second
		log.Printf("Waiting %v for cleanup operations to complete...", delay)
		time.Sleep(delay)
		log.Printf("Resuming after delay")
	} else {
		log.Printf("Fresh start not requested, keeping existing collections")
	}

	// Ensure the core facts collections exist (these have hybrid indices)
	log.Printf("Ensuring people facts collection...")
	if err := createPeopleFactsCollection(ctx, milvusClient, embeddingDim); err != nil {
		return fmt.Errorf("failed to create/ensure people facts collection: %w", err)
	}

	log.Printf("Ensuring community facts collection...")
	if err := createCommunityFactsCollection(ctx, milvusClient, embeddingDim); err != nil {
		return fmt.Errorf("failed to create/ensure community facts collection: %w", err)
	}

	log.Printf("Ensuring bot facts collection...")
	if err := createBotFactsCollection(ctx, milvusClient, embeddingDim); err != nil {
		return fmt.Errorf("failed to create/ensure bot facts collection: %w", err)
	}

	// Add a small delay to allow index creation if collections were just made
	time.Sleep(500 * time.Millisecond)

	// Finally, load collections into memory (Milvus requires this for searching)
	// It's okay to load even if they were already loaded.
	for _, collName := range []string{
		PeopleFactsCollection, CommunityFactsCollection, BotFactsCollection,
	} {
		log.Printf("Loading collection into memory: %s", collName)
		loadOpt := milvusclient.NewLoadCollectionOption(collName)
		_, err := milvusClient.LoadCollection(ctx, loadOpt)
		if err != nil {
			// Check if the error indicates it's already loaded (optional, might depend on Milvus version/error codes)
			// For now, return the error as loading is critical.
			return fmt.Errorf("failed to load collection %s into memory: %w", collName, err)
		}
		log.Printf("Successfully loaded collection: %s", collName)
	}

	log.Println("All required collections ensured and loaded successfully")
	return nil
}

// createPeopleFactsCollection creates the people facts collection
func createPeopleFactsCollection(ctx context.Context, milvusClient *milvusclient.Client, embeddingDim int) error {
	hasOpt := milvusclient.NewHasCollectionOption(PeopleFactsCollection)
	exists, err := milvusClient.HasCollection(ctx, hasOpt)
	if err != nil {
		return fmt.Errorf("failed to check if collection exists: %w", err)
	}

	if !exists {
		schema := &entity.Schema{
			CollectionName: PeopleFactsCollection,
			Description:    "Hybrid-search facts about each person",
			Fields: []*entity.Field{
				{
					Name:       "id",
					DataType:   entity.FieldTypeVarChar,
					PrimaryKey: true,
					AutoID:     false,
					TypeParams: map[string]string{
						"max_length": "100", // Specify max length for VarChar
					},
				},
				{
					Name:     "telegram_id",
					DataType: entity.FieldTypeVarChar,
					TypeParams: map[string]string{
						"max_length": "100", // Specify max length for VarChar
					},
				},
				{
					Name:     "telegram_name",
					DataType: entity.FieldTypeVarChar,
					TypeParams: map[string]string{
						"max_length": "255", // Specify max length for VarChar
					},
				},
				{
					Name:     "text",
					DataType: entity.FieldTypeVarChar,
					TypeParams: map[string]string{
						"max_length": "65535", // Specify max length for VarChar
					},
				},
				{
					Name:     "dense",
					DataType: entity.FieldTypeFloatVector,
					TypeParams: map[string]string{
						"dim": fmt.Sprintf("%d", embeddingDim),
					},
				},
				{
					Name:     "sparse",
					DataType: entity.FieldTypeSparseVector,
				},
				{
					Name:     "created_at",
					DataType: entity.FieldTypeInt64,
				},
				{
					Name:     "metadata",
					DataType: entity.FieldTypeJSON,
				},
			},
		}

		createOpt := milvusclient.NewCreateCollectionOption(PeopleFactsCollection, schema)
		createOpt.WithShardNum(2)
		err = milvusClient.CreateCollection(ctx, createOpt)
		if err != nil {
			return fmt.Errorf("failed to create people facts collection: %w", err)
		}

		// Create index on dense vector field
		denseIdx := index.NewHNSWIndex(entity.IP, 16, 200)
		indexOpt := milvusclient.NewCreateIndexOption(PeopleFactsCollection, "dense", denseIdx)
		_, err = milvusClient.CreateIndex(ctx, indexOpt)
		if err != nil {
			return fmt.Errorf("failed to create index on dense vector field: %w", err)
		}

		// Create index on sparse vector field using SPARSE_INVERTED_INDEX
		sparseIdx := index.NewSparseInvertedIndex(entity.IP, 0.2)
		sparseIndexOpt := milvusclient.NewCreateIndexOption(PeopleFactsCollection, "sparse", sparseIdx)
		_, err = milvusClient.CreateIndex(ctx, sparseIndexOpt)
		if err != nil {
			return fmt.Errorf("failed to create index on sparse vector field: %w", err)
		}

		log.Printf("Created collection with dense and sparse indices: %s", PeopleFactsCollection)
	}

	return nil
}

// EnsurePeopleFactsCollection ensures the people facts collection exists
func EnsurePeopleFactsCollection(ctx context.Context, milvusClient *milvusclient.Client, embeddingDim int) error {
	if err := createPeopleFactsCollection(ctx, milvusClient, embeddingDim); err != nil {
		return err
	}

	// Load the collection if needed
	loadOpt := milvusclient.NewLoadCollectionOption(PeopleFactsCollection)
	_, err := milvusClient.LoadCollection(ctx, loadOpt)
	return err
}

// createCommunityFactsCollection creates the community facts collection
func createCommunityFactsCollection(ctx context.Context, milvusClient *milvusclient.Client, embeddingDim int) error {
	hasOpt := milvusclient.NewHasCollectionOption(CommunityFactsCollection)
	exists, err := milvusClient.HasCollection(ctx, hasOpt)
	if err != nil {
		return fmt.Errorf("failed to check if collection exists: %w", err)
	}

	if !exists {
		schema := &entity.Schema{
			CollectionName: CommunityFactsCollection,
			Description:    "Hybrid-search facts about each community",
			Fields: []*entity.Field{
				{
					Name:       "id",
					DataType:   entity.FieldTypeVarChar,
					PrimaryKey: true,
					AutoID:     false,
					TypeParams: map[string]string{
						"max_length": "100", // Specify max length for VarChar
					},
				},
				{
					Name:     "owner_id",
					DataType: entity.FieldTypeVarChar,
					TypeParams: map[string]string{
						"max_length": "100", // Specify max length for VarChar
					},
				},
				{
					Name:     "name",
					DataType: entity.FieldTypeVarChar,
					TypeParams: map[string]string{
						"max_length": "255", // Specify max length for VarChar
					},
				},
				{
					Name:     "text",
					DataType: entity.FieldTypeVarChar,
					TypeParams: map[string]string{
						"max_length": "65535", // Specify max length for VarChar
					},
				},
				{
					Name:     "dense",
					DataType: entity.FieldTypeFloatVector,
					TypeParams: map[string]string{
						"dim": fmt.Sprintf("%d", embeddingDim),
					},
				},
				{
					Name:     "sparse",
					DataType: entity.FieldTypeSparseVector,
				},
				{
					Name:     "created_at",
					DataType: entity.FieldTypeInt64,
				},
				{
					Name:     "metadata",
					DataType: entity.FieldTypeJSON,
				},
			},
		}

		createOpt := milvusclient.NewCreateCollectionOption(CommunityFactsCollection, schema)
		createOpt.WithShardNum(2)
		err = milvusClient.CreateCollection(ctx, createOpt)
		if err != nil {
			return fmt.Errorf("failed to create community facts collection: %w", err)
		}

		// Create index on dense vector field
		denseIdx := index.NewHNSWIndex(entity.IP, 16, 200)
		indexOpt := milvusclient.NewCreateIndexOption(CommunityFactsCollection, "dense", denseIdx)
		_, err = milvusClient.CreateIndex(ctx, indexOpt)
		if err != nil {
			return fmt.Errorf("failed to create index on dense vector field: %w", err)
		}

		// Create index on sparse vector field using SPARSE_INVERTED_INDEX
		sparseIdx := index.NewSparseInvertedIndex(entity.IP, 0.2)
		sparseIndexOpt := milvusclient.NewCreateIndexOption(CommunityFactsCollection, "sparse", sparseIdx)
		_, err = milvusClient.CreateIndex(ctx, sparseIndexOpt)
		if err != nil {
			return fmt.Errorf("failed to create index on sparse vector field: %w", err)
		}

		log.Printf("Created collection with dense and sparse indices: %s", CommunityFactsCollection)
	}

	return nil
}

// EnsureCommunityFactsCollection ensures the community facts collection exists
func EnsureCommunityFactsCollection(ctx context.Context, milvusClient *milvusclient.Client, embeddingDim int) error {
	if err := createCommunityFactsCollection(ctx, milvusClient, embeddingDim); err != nil {
		return err
	}

	// Load the collection if needed
	loadOpt := milvusclient.NewLoadCollectionOption(CommunityFactsCollection)
	_, err := milvusClient.LoadCollection(ctx, loadOpt)
	return err
}

// createBotFactsCollection creates the bot facts collection
func createBotFactsCollection(ctx context.Context, milvusClient *milvusclient.Client, embeddingDim int) error {
	hasOpt := milvusclient.NewHasCollectionOption(BotFactsCollection)
	exists, err := milvusClient.HasCollection(ctx, hasOpt)
	if err != nil {
		return fmt.Errorf("failed to check if collection exists: %w", err)
	}

	if !exists {
		schema := &entity.Schema{
			CollectionName: BotFactsCollection,
			Description:    "Hybrid-search facts about the bot itself",
			Fields: []*entity.Field{
				{
					Name:       "id",
					DataType:   entity.FieldTypeVarChar,
					PrimaryKey: true,
					AutoID:     false,
					TypeParams: map[string]string{
						"max_length": "100", // Specify max length for VarChar
					},
				},
				{
					Name:     "owner_id", // Always "bot"
					DataType: entity.FieldTypeVarChar,
					TypeParams: map[string]string{
						"max_length": "100", // Specify max length for VarChar
					},
				},
				{
					Name:     "name",
					DataType: entity.FieldTypeVarChar,
					TypeParams: map[string]string{
						"max_length": "255", // Specify max length for VarChar
					},
				},
				{
					Name:     "text",
					DataType: entity.FieldTypeVarChar,
					TypeParams: map[string]string{
						"max_length": "65535", // Specify max length for VarChar
					},
				},
				{
					Name:     "dense",
					DataType: entity.FieldTypeFloatVector,
					TypeParams: map[string]string{
						"dim": fmt.Sprintf("%d", embeddingDim),
					},
				},
				{
					Name:     "sparse",
					DataType: entity.FieldTypeSparseVector,
				},
				{
					Name:     "created_at",
					DataType: entity.FieldTypeInt64,
				},
				{
					Name:     "metadata",
					DataType: entity.FieldTypeJSON,
				},
			},
		}

		createOpt := milvusclient.NewCreateCollectionOption(BotFactsCollection, schema)
		createOpt.WithShardNum(2)
		err = milvusClient.CreateCollection(ctx, createOpt)
		if err != nil {
			return fmt.Errorf("failed to create bot facts collection: %w", err)
		}

		// Create index on dense vector field
		denseIdx := index.NewHNSWIndex(entity.IP, 16, 200)
		indexOpt := milvusclient.NewCreateIndexOption(BotFactsCollection, "dense", denseIdx)
		_, err = milvusClient.CreateIndex(ctx, indexOpt)
		if err != nil {
			return fmt.Errorf("failed to create index on dense vector field: %w", err)
		}

		// Create index on sparse vector field using SPARSE_INVERTED_INDEX
		sparseIdx := index.NewSparseInvertedIndex(entity.IP, 0.2)
		sparseIndexOpt := milvusclient.NewCreateIndexOption(BotFactsCollection, "sparse", sparseIdx)
		_, err = milvusClient.CreateIndex(ctx, sparseIndexOpt)
		if err != nil {
			return fmt.Errorf("failed to create index on sparse vector field: %w", err)
		}

		log.Printf("Created collection with dense and sparse indices: %s", BotFactsCollection)
	}

	return nil
}

// EnsureBotFactsCollection ensures the bot facts collection exists
func EnsureBotFactsCollection(ctx context.Context, milvusClient *milvusclient.Client, embeddingDim int) error {
	if err := createBotFactsCollection(ctx, milvusClient, embeddingDim); err != nil {
		return err
	}

	// Load the collection if needed
	loadOpt := milvusclient.NewLoadCollectionOption(BotFactsCollection)
	_, err := milvusClient.LoadCollection(ctx, loadOpt)
	return err
}
