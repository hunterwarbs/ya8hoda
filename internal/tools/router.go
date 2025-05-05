package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hunterwarburton/ya8hoda/internal/core"
	"github.com/hunterwarburton/ya8hoda/internal/logger"
	"github.com/hunterwarburton/ya8hoda/internal/telegram"
)

// PolicyService defines the interface for checking tool permissions.
type PolicyService interface {
	IsToolAllowed(userID int64, toolName string) bool
}

// RAGService interface definition removed. Using core.RAGService.

// EmbedService interface definition removed. Using core.EmbedService.

// SearchResult definition removed. Using core.SearchResult.

// Document definition removed. Using core.Document.

// ToolRouter routes and executes tool calls.
type ToolRouter struct {
	policy PolicyService
	rag    core.RAGService
	embed  core.EmbedService
}

// NewToolRouter creates a new ToolRouter.
func NewToolRouter(policy PolicyService, rag core.RAGService, embedSvc core.EmbedService) *ToolRouter {
	return &ToolRouter{
		policy: policy,
		rag:    rag,
		embed:  embedSvc,
	}
}

// ExecuteToolCall executes a tool call and returns the result as a string.
func (r *ToolRouter) ExecuteToolCall(ctx context.Context, userID int64, call *telegram.ToolCall) (string, error) {
	// Check if the user is allowed to use this tool
	if !r.policy.IsToolAllowed(userID, call.Function.Name) {
		err := fmt.Errorf("user %d is not allowed to use tool %s", userID, call.Function.Name)
		logger.Error("Tool execution failed: %v", err)
		return "", err // Return error directly
	}

	logger.Debug("Executing tool '%s' for user %d...", call.Function.Name, userID)

	var result string // Declare result string here
	var err error     // Declare error here

	// Route the tool call to the appropriate handler
	switch call.Function.Name {
	// Storage functions
	case "store_person_memory":
		var args struct {
			TelegramID string                 `json:"telegram_id"`           // Assume this is injected if needed
			PersonName string                 `json:"person_name,omitempty"` // Injected by bot
			MemoryText string                 `json:"memory_text"`
			Metadata   map[string]interface{} `json:"metadata"`
		}
		if err = json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return "", fmt.Errorf("failed to parse store_person_memory arguments: %w", err)
		}
		logger.Debug("Parsed store_person_memory arguments: %+v", args)
		if args.MemoryText == "" {
			return "", fmt.Errorf("memory_text is required for store_person_memory")
		}
		// Call the RAGService interface method for storing person memory
		result, err = r.rag.RememberAboutPerson(ctx, args.TelegramID, args.PersonName, args.MemoryText, args.Metadata)
		if err != nil {
			return "", fmt.Errorf("failed to execute store_person_memory: %w", err)
		}

	case "store_self_memory":
		var args struct {
			MemoryText string                 `json:"memory_text"`
			Metadata   map[string]interface{} `json:"metadata"`
		}
		if err = json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return "", fmt.Errorf("failed to parse store_self_memory arguments: %w", err)
		}
		logger.Debug("Parsed store_self_memory arguments: %+v", args)
		if args.MemoryText == "" {
			return "", fmt.Errorf("memory_text is required for store_self_memory")
		}
		// Call the RAGService interface method for storing self memory
		result, err = r.rag.RememberAboutSelf(ctx, args.MemoryText, args.Metadata)
		if err != nil {
			return "", fmt.Errorf("failed to execute store_self_memory: %w", err)
		}

	case "store_community_memory":
		var args struct {
			CommunityID string                 `json:"community_id"`
			MemoryText  string                 `json:"memory_text"`
			Metadata    map[string]interface{} `json:"metadata"`
		}
		if err = json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return "", fmt.Errorf("failed to parse store_community_memory arguments: %w", err)
		}
		logger.Debug("Parsed store_community_memory arguments: %+v", args)
		if args.MemoryText == "" || args.CommunityID == "" {
			return "", fmt.Errorf("memory_text and community_id are required for store_community_memory")
		}
		// Call the RAGService interface method for storing community memory
		result, err = r.rag.RememberAboutCommunity(ctx, args.CommunityID, args.MemoryText, args.Metadata)
		if err != nil {
			return "", fmt.Errorf("failed to execute store_community_memory: %w", err)
		}

	// Retrieval functions
	case "remember_about_person":
		var args struct {
			TelegramID string `json:"telegram_id,omitempty"` // Optional, searches all if empty
			Query      string `json:"query"`
			K          int    `json:"k"`
		}
		if err = json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return "", fmt.Errorf("failed to parse remember_about_person arguments: %w", err)
		}
		logger.Debug("Parsed remember_about_person arguments: %+v", args)
		if args.Query == "" {
			return "", fmt.Errorf("query is required for remember_about_person")
		}
		if args.K <= 0 {
			args.K = 5 // Default
		}
		// Call the RAGService interface method for searching personal memory
		var searchResults []core.SearchResult
		searchResults, err = r.rag.SearchPersonalMemory(ctx, args.Query, args.TelegramID, args.K)
		if err != nil {
			return "", fmt.Errorf("failed to execute remember_about_person: %w", err)
		}
		result = formatSearchResults(searchResults) // Assign formatted results

	case "remember_about_community":
		var args struct {
			CommunityID string `json:"community_id,omitempty"` // Optional, searches all if empty
			Query       string `json:"query"`
			K           int    `json:"k"`
		}
		if err = json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return "", fmt.Errorf("failed to parse remember_about_community arguments: %w", err)
		}
		logger.Debug("Parsed remember_about_community arguments: %+v", args)
		if args.Query == "" {
			return "", fmt.Errorf("query is required for remember_about_community")
		}
		if args.K <= 0 {
			args.K = 5 // Default
		}
		// Call the RAGService interface method for searching community memory
		var searchResults []core.SearchResult
		searchResults, err = r.rag.SearchCommunityMemory(ctx, args.Query, args.CommunityID, args.K)
		if err != nil {
			return "", fmt.Errorf("failed to execute remember_about_community: %w", err)
		}
		result = formatSearchResults(searchResults) // Assign formatted results

	case "remember_about_self": // Implemented based on other search functions
		var args struct {
			Query string `json:"query"`
			K     int    `json:"k"`
		}
		if err = json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return "", fmt.Errorf("failed to parse remember_about_self arguments: %w", err)
		}
		logger.Debug("Parsed remember_about_self arguments: %+v", args)
		if args.Query == "" {
			return "", fmt.Errorf("query is required for remember_about_self")
		}
		if args.K <= 0 {
			args.K = 5 // Default value from JSON spec
		}
		// Call the RAGService interface method for searching self memory
		var searchResults []core.SearchResult
		searchResults, err = r.rag.SearchSelfMemory(ctx, args.Query, args.K)
		if err != nil {
			return "", fmt.Errorf("failed to execute remember_about_self: %w", err)
		}
		result = formatSearchResults(searchResults) // Assign formatted results

	default:
		err = fmt.Errorf("unknown tool: %s", call.Function.Name)
	}

	if err != nil {
		logger.Error("Tool '%s' execution failed for user %d: %v", call.Function.Name, userID, err)
		return "", err // Return error
	} else {
		// Log the result (truncated)
		resultText := result
		if len(resultText) > 100 {
			resultText = resultText[:100] + "..."
		}
		logger.Debug("Tool '%s' execution successful for user %d. Result: \"%s\"", call.Function.Name, userID, resultText)
		return result, nil // Return the result string
	}
}

// formatSearchResults formats search results into a JSON string.
func formatSearchResults(results []core.SearchResult) string {
	if len(results) == 0 {
		// Return a JSON representation of an empty list or a specific message
		return `{"memories": [], "message": "No relevant memories found."}`
	}

	// Prepare data for JSON marshalling
	type memoryResult struct {
		Memory string  `json:"memory"`
		Score  float32 `json:"score"`
		// Add other metadata fields if needed
		// Metadata map[string]interface{} `json:"metadata,omitempty"`
	}

	outputResults := make([]memoryResult, 0, len(results))
	for _, res := range results {
		memoryText := res.Document.Text // Fallback
		if res.Document.Metadata != nil {
			if mt, ok := res.Document.Metadata["memory_text"]; ok {
				if text, ok := mt.(string); ok && text != "" {
					memoryText = text // Prefer metadata["memory_text"]
				}
			}
		}
		if memoryText == "" {
			memoryText = "(memory text not available)" // Final fallback
		}

		outputResults = append(outputResults, memoryResult{
			Memory: memoryText,
			Score:  res.Score,
			// Metadata: res.Document.Metadata, // Optionally include metadata
		})
	}

	// Marshal the results into a JSON string
	jsonData, err := json.Marshal(map[string]interface{}{"memories": outputResults})
	if err != nil {
		logger.Error("Failed to marshal search results to JSON: %v", err)
		// Return an error JSON or a simple error message string
		return `{"error": "Failed to format results as JSON"}`
	}

	return string(jsonData)
}

// buildCollectionName constructs the Milvus collection name. (Removed as helpers were removed)
// func (r *ToolRouter) buildCollectionName(memType, specificID string) string { ... }

// Removed StoreMemory helper function
// func (r *ToolRouter) StoreMemory(...) { ... }

// Removed SearchMemory helper function
// func (r *ToolRouter) SearchMemory(...) { ... }

// Removed commented out old handler functions
// // func (r *ToolRouter) handleRememberAboutPerson(...) { ... }
// // func (r *ToolRouter) handleRememberAboutSelf(...) { ... }
// // func (r *ToolRouter) handleRememberAboutCommunity(...) { ... }
// // func (r *ToolRouter) handleSearchPersonalMemory(...) { ... }
// // func (r *ToolRouter) handleSearchCommunityMemory(...) { ... }
// // func (r *ToolRouter) storeMemoryWithEmbedding(...) { ... }
