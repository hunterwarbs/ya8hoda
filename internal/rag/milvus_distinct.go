package rag

import (
	"context"
	"fmt"

	"github.com/hunterwarburton/ya8hoda/internal/logger"
	"github.com/milvus-io/milvus/client/v2/milvusclient"
)

// ListKnownPeople returns up to `max` distinct telegram_name values from the PeopleFactsCollection.
func (c *MilvusClient) ListKnownPeople(ctx context.Context, max int) ([]string, error) {
	if max <= 0 {
		max = 5 // Default to 5 if invalid max is provided
	}

	queryOpt := milvusclient.NewQueryOption(PeopleFactsCollection).
		WithOutputFields("telegram_name"). // Field specific to people_facts
		WithLimit(max * 2)                 // Fetch more to account for potential duplicates before distinct operation. Corrected type.

	results, err := c.client.Query(ctx, queryOpt)
	if err != nil {
		logger.Error("Failed to query PeopleFactsCollection for distinct names", err, "collection", PeopleFactsCollection)
		return nil, fmt.Errorf("failed to query PeopleFactsCollection for distinct names: %w", err)
	}

	seenNames := make(map[string]struct{})
	distinctNames := make([]string, 0)

	nameCol := results.GetColumn("telegram_name")
	if nameCol == nil {
		logger.Warn("telegram_name column not found in query result", "collection", PeopleFactsCollection)
		return distinctNames, nil
	}

	for i := 0; i < nameCol.Len(); i++ {
		name, err := nameCol.GetAsString(i)
		if err != nil {
			logger.Warn("Error getting name from column", err, "index", i)
			continue
		}
		if name == "" { // Skip empty names
			continue
		}
		if _, exists := seenNames[name]; !exists {
			seenNames[name] = struct{}{}
			distinctNames = append(distinctNames, name)
			if len(distinctNames) >= max {
				break
			}
		}
	}
	logger.Debug("Successfully fetched distinct people names", "count", len(distinctNames))
	return distinctNames, nil
}

// ListKnownCommunities returns up to `max` distinct name values from the CommunityFactsCollection.
func (c *MilvusClient) ListKnownCommunities(ctx context.Context, max int) ([]string, error) {
	if max <= 0 {
		max = 5 // Default to 5 if invalid max is provided
	}

	queryOpt := milvusclient.NewQueryOption(CommunityFactsCollection).
		WithOutputFields(FieldName). // Generic "name" field for communities
		WithLimit(max * 2)           // Corrected type.

	results, err := c.client.Query(ctx, queryOpt)
	if err != nil {
		logger.Error("Failed to query CommunityFactsCollection for distinct names", err, "collection", CommunityFactsCollection)
		return nil, fmt.Errorf("failed to query CommunityFactsCollection for distinct names: %w", err)
	}

	seenNames := make(map[string]struct{})
	distinctNames := make([]string, 0)

	nameCol := results.GetColumn(FieldName)
	if nameCol == nil {
		logger.Warn("name column not found in query result", "collection", CommunityFactsCollection)
		return distinctNames, nil
	}

	for i := 0; i < nameCol.Len(); i++ {
		name, err := nameCol.GetAsString(i)
		if err != nil {
			logger.Warn("Error getting name from column", err, "index", i)
			continue
		}
		if name == "" { // Skip empty names
			continue
		}
		if _, exists := seenNames[name]; !exists {
			seenNames[name] = struct{}{}
			distinctNames = append(distinctNames, name)
			if len(distinctNames) >= max {
				break
			}
		}
	}
	logger.Debug("Successfully fetched distinct community names", "count", len(distinctNames))
	return distinctNames, nil
}
