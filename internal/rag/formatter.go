package rag

import (
	"fmt"
	"strings"
)

// FormatSearchResultsAsText formats search results as a readable text string
func FormatSearchResultsAsText(results []SearchResult) string {
	if len(results) == 0 {
		return "No results found."
	}

	var sb strings.Builder
	for i, result := range results {
		sb.WriteString(fmt.Sprintf("[%d] %s (Score: %.4f)\n", i+1, result.Document.Title, result.Score))
		sb.WriteString(fmt.Sprintf("Source: %s\n", result.Document.Source))
		sb.WriteString(fmt.Sprintf("Text: %s\n", result.Document.Text))
		if i < len(results)-1 {
			sb.WriteString("\n")
		}
	}

	return sb.String()
}
