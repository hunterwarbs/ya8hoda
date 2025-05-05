package rag

import (
	"fmt"
	"strings"
	"time"

	"github.com/hunterwarburton/ya8hoda/internal/core"
)

// FormatSearchResultsAsText formats search results as a readable text string
func FormatSearchResultsAsText(results []core.SearchResult) string {
	if len(results) == 0 {
		return "No results found."
	}

	var sb strings.Builder
	for i, result := range results {
		sb.WriteString(fmt.Sprintf("%s\n", result.Document.Title))
		sb.WriteString(fmt.Sprintf("%s\n", result.Document.Text))
		sb.WriteString(fmt.Sprintf("Learned: %s\n", formatTimestamp(result.Document.CreateTime)))
		if i < len(results)-1 {
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// formatTimestamp converts a Unix timestamp to a human-readable date string
func formatTimestamp(timestamp int64) string {
	return time.Unix(timestamp, 0).Format("January 2, 2006 at 3:04 PM")
}
