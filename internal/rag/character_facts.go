package rag

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/hunterwarburton/ya8hoda/internal/core" // Import core for interfaces
	"github.com/hunterwarburton/ya8hoda/internal/embed"
	"github.com/hunterwarburton/ya8hoda/internal/llm"    // Import tools for RAGService interface
	"github.com/hunterwarburton/ya8hoda/internal/logger" // Import custom logger
	"github.com/milvus-io/milvus/client/v2/milvusclient"
)

// EnsureCharacterFacts loads a character's facts into Milvus if they don't already exist
// DEPRECATED: Use EnsureCharacterFactsWithOptions and pass the initialized RAGService
// func EnsureCharacterFacts(ctx context.Context, milvusClient *milvusclient.Client, embedder *embed.BGEAdapter, character *llm.Character) error {
// 	return EnsureCharacterFactsWithOptions(ctx, milvusClient, embedder, character, false)
// }

// EnsureCharacterFactsWithOptions loads a character's facts into Milvus with additional options
func EnsureCharacterFactsWithOptions(ctx context.Context, ragService core.RAGService, embedder *embed.BGEAdapter, character *llm.Character, forceReload bool) error {
	logger.Info("Ensuring character facts for %s are loaded into Milvus (forceReload: %v)", character.Name, forceReload)

	// Type assert the RAGService to get the concrete MilvusClient
	milvusWrapper, ok := ragService.(*MilvusClient)
	if !ok {
		// If it's not the real MilvusClient (e.g., mock), we can't load facts.
		logger.Warn("RAGService is not *MilvusClient, skipping character facts loading.")
		// Depending on requirements, you might return an error or just skip.
		// Returning nil to allow the bot to start even with mock service.
		return nil
	}

	// Get the underlying client for direct operations if needed (like factExists)
	rawMilvusClient := milvusWrapper.GetClient()
	if rawMilvusClient == nil {
		return fmt.Errorf("failed to get underlying milvus client from MilvusClient wrapper")
	}

	// Process bio facts
	if err := loadCharacterFacts(ctx, milvusWrapper, rawMilvusClient, embedder, character.Bio, "bio", character.Name, forceReload); err != nil {
		return fmt.Errorf("failed to load character bio facts: %w", err)
	}

	// Process lore facts
	if err := loadCharacterFacts(ctx, milvusWrapper, rawMilvusClient, embedder, character.Lore, "lore", character.Name, forceReload); err != nil {
		return fmt.Errorf("failed to load character lore facts: %w", err)
	}

	// Process knowledge facts
	if err := loadCharacterFacts(ctx, milvusWrapper, rawMilvusClient, embedder, character.Knowledge, "knowledge", character.Name, forceReload); err != nil {
		return fmt.Errorf("failed to load character knowledge facts: %w", err)
	}

	logger.Info("Successfully finished ensuring character facts for %s are in Milvus", character.Name)
	return nil
}

// loadCharacterFacts loads a specific set of character facts into Milvus
// Updated to accept the raw client for factExists
func loadCharacterFacts(ctx context.Context, milvusWrapper *MilvusClient, rawMilvusClient *milvusclient.Client, embedder *embed.BGEAdapter, facts []string, factType string, characterName string, forceReload bool) error {
	if len(facts) == 0 {
		logger.Debug("No %s facts to load for character %s", factType, characterName)
		return nil
	}

	logger.Debug("Processing %d %s facts for character %s", len(facts), factType, characterName)

	// Get the specific BGE embedder capable of hybrid embeddings
	bgeEmbedder, ok := embedder.UnwrapEmbedder()
	if !ok {
		// Fallback or error if BGE embedder not available?
		// For now, let's try using the adapter's basic EmbedQuery, which might lack sparse data
		logger.Warn("Could not unwrap full BGEEmbedder for %s. Hybrid embedding might not work as expected.", characterName)
		// If hybrid is essential, return an error here:
		return fmt.Errorf("failed to get BGE embedder required for hybrid embeddings for %s", characterName)
	}

	factsLoaded := 0
	for _, fact := range facts {
		// Create deterministic ID based on the fact content
		hash := sha256.Sum256([]byte(fact))
		id := fmt.Sprintf("%s:%s", characterName, hex.EncodeToString(hash[:8])) // Use first 8 bytes of hash

		// Check if this fact already exists
		shouldInsert := true
		if !forceReload {
			exists, err := factExists(ctx, rawMilvusClient, id) // Use the raw client
			if err != nil {
				return fmt.Errorf("failed to check if fact %s exists: %w", id, err)
			}
			if exists {
				// logger.Debug("Fact %s already exists, skipping.", id) // Optional: Keep for very verbose debugging
				shouldInsert = false
			}
		}

		if !shouldInsert {
			continue
		}

		// Create embedding (dense + sparse)
		// logger.Debug("Creating embedding for fact ID %s", id) // Removed excess logging
		embedding, embedErr := bgeEmbedder.EmbedQuery(ctx, fact) // Use unwrapped embedder
		if embedErr != nil {
			logger.Error("Failed to embed fact ID %s: %v", id, embedErr)
			return fmt.Errorf("failed to embed fact '%s' (ID: %s): %w", fact[:min(50, len(fact))], id, embedErr)
		}
		if embedding.Dense == nil || embedding.Sparse == nil {
			logger.Error("Embedding service returned incomplete data for fact ID %s (missing dense or sparse)", id)
			return fmt.Errorf("embedding service returned incomplete data for fact '%s' (ID: %s, missing dense or sparse)", fact[:min(50, len(fact))], id)
		}
		// logger.Debug("Created embedding vectors for fact ID %s: dense dim=%d, sparse nnz=%d", id, len(embedding.Dense), len(embedding.Sparse.Indices)) // Removed excess logging

		// Prepare metadata map
		metadata := map[string]interface{}{
			"type":      factType,
			"source":    "character_definition",
			"character": characterName, // Add character name to metadata
		}

		// Use the StoreBotFact function via the wrapper
		// StoreBotFact now internally handles the primary key generation or uses the provided one.
		// We pass the fact text, metadata, and embedding.
		_, insertErr := milvusWrapper.StoreBotFact(ctx,
			characterName, // Pass characterName as the 'name' field
			fact,          // Pass the raw fact text
			metadata,      // Pass the prepared metadata
			embedding,     // Pass the whole embedding struct
		)
		if insertErr != nil {
			logger.Error("Failed to store fact ID %s: %v", id, insertErr)
			return fmt.Errorf("failed to store fact '%s' (ID: %s): %w", fact[:min(50, len(fact))], id, insertErr)
		}

		// Log success only after insertion confirmed
		logger.Debug("Loaded memory fact [%s] %s: %s", factType, id, fact[:min(80, len(fact))])
		factsLoaded++

	}

	logger.Info("Finished processing %s facts for %s. Loaded %d new facts.", factType, characterName, factsLoaded)
	return nil
}

// factExists checks if a fact with the given ID already exists in the collection
func factExists(ctx context.Context, milvusClient *milvusclient.Client, id string) (bool, error) {
	expr := fmt.Sprintf(`id == "%s"`, id) // Query based on the primary key 'id'
	// logger.Debug("Checking if fact exists with expression: %s", expr) // Keep commented unless debugging needed

	queryOpt := milvusclient.NewQueryOption(BotFactsCollection)
	queryOpt.WithFilter(expr)
	queryOpt.WithOutputFields("id") // Check for the primary key field
	queryOpt.WithLimit(1)           // We only need to know if at least one exists

	result, err := milvusClient.Query(ctx, queryOpt)
	if err != nil {
		logger.Error("ERROR querying Milvus for fact %s: %v", id, err)
		return false, fmt.Errorf("failed to query fact existence for ID %s: %w", id, err)
	}

	// Check if any records were found
	exists := result.ResultCount > 0

	if exists {
		logger.Warn("Fact with id %s already exists in database.", id)
	}
	// else {
	// logger.Debug("Fact with id %s does not exist, will insert", id) // Removed "will insert" log
	// }

	return exists, nil
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
