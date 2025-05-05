package rag

import (
	"encoding/json"
	"time"

	"github.com/hunterwarburton/ya8hoda/internal/core"
	"github.com/hunterwarburton/ya8hoda/internal/logger"
)

// FormatSearchResultsAsJSON formats search results into a JSON string, including metadata and timestamp.
func FormatSearchResultsAsJSON(results []core.SearchResult) string {
	if len(results) == 0 {
		// Return a JSON representation of an empty list or a specific message
		return `{"memories": [], "message": "No relevant memories found."}`
	}

	// Prepare data for JSON marshalling
	type memoryResult struct {
		Memory     string                 `json:"memory"`
		Score      float32                `json:"score"`
		Name       string                 `json:"name,omitempty"`        // Corresponds to telegram_name now
		TelegramID string                 `json:"telegram_id,omitempty"` // Added telegram_id
		Timestamp  string                 `json:"timestamp,omitempty"`   // Added timestamp field
		Metadata   map[string]interface{} `json:"metadata,omitempty"`
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

		// Format timestamp
		timestampStr := ""
		if res.Document.CreateTime > 0 {
			timestampStr = time.Unix(res.Document.CreateTime, 0).Format("January 2, 2006 at 3:04 PM")
		}

		outputResults = append(outputResults, memoryResult{
			Memory:     memoryText,
			Score:      res.Score,
			Name:       res.Document.Name,    // Mapped to telegram_name
			TelegramID: res.Document.OwnerID, // Mapped to telegram_id
			Timestamp:  timestampStr,         // Populate timestamp
			Metadata:   res.Document.Metadata,
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
