package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hunterwarburton/ya8hoda/internal/rag"
	"github.com/hunterwarburton/ya8hoda/internal/telegram"
)

// PolicyService defines the interface for checking tool permissions.
type PolicyService interface {
	IsToolAllowed(userID int64, toolName string) bool
}

// RAGService defines the interface for interacting with the RAG system.
type RAGService interface {
	SearchSimilar(ctx context.Context, vector []float32, k int, filter string) ([]rag.SearchResult, error)
	StoreDocument(ctx context.Context, text, title, source string, metadata map[string]interface{}, vector []float32) (string, error)
}

// EmbedService defines the interface for creating embeddings.
type EmbedService interface {
	EmbedQuery(ctx context.Context, text string) ([]float32, error)
}

// LLMService defines the interface for interacting with the LLM.
type LLMService interface {
	GenerateImage(ctx context.Context, prompt, size, style string) (string, error)
}

// SearchResult represents a search result from the vector database.
type SearchResult struct {
	Document Document `json:"document"`
	Score    float32  `json:"score"`
}

// Document represents a document stored in the vector database.
type Document struct {
	ID         string                 `json:"id"`
	Text       string                 `json:"text"`
	Title      string                 `json:"title,omitempty"`
	Source     string                 `json:"source,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	CreateTime int64                  `json:"create_time"`
}

// ToolRouter routes and executes tool calls.
type ToolRouter struct {
	policy PolicyService
	rag    RAGService
	embed  EmbedService
	llm    LLMService
}

// NewToolRouter creates a new ToolRouter.
func NewToolRouter(policy PolicyService, rag RAGService, llm LLMService) *ToolRouter {
	return &ToolRouter{
		policy: policy,
		rag:    rag,
		llm:    llm,
	}
}

// ExecuteToolCall executes a tool call and returns the result as a string.
func (r *ToolRouter) ExecuteToolCall(ctx context.Context, userID int64, toolCall *telegram.ToolCall) (string, error) {
	// Check if the user is allowed to use this tool
	if !r.policy.IsToolAllowed(userID, toolCall.Function.Name) {
		return "", fmt.Errorf("user %d is not allowed to use tool %s", userID, toolCall.Function.Name)
	}

	// Route the tool call to the appropriate handler
	switch toolCall.Function.Name {
	case "milvus.search":
		return r.handleSearch(ctx, toolCall.Function.Arguments)
	case "milvus.store_document":
		return r.handleStoreDocument(ctx, toolCall.Function.Arguments)
	case "openrouter.generate_image":
		return r.handleGenerateImage(ctx, toolCall.Function.Arguments)
	default:
		return "", fmt.Errorf("unknown tool: %s", toolCall.Function.Name)
	}
}

// handleSearch handles a search tool call.
func (r *ToolRouter) handleSearch(ctx context.Context, arguments string) (string, error) {
	// Parse the arguments
	var args struct {
		Query  string `json:"query"`
		K      int    `json:"k"`
		Filter string `json:"filter"`
	}

	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", fmt.Errorf("failed to parse search arguments: %w", err)
	}

	// Set default k if not provided
	if args.K <= 0 {
		args.K = 5
	}

	// Create an embedding for the query
	vector, err := r.embed.EmbedQuery(ctx, args.Query)
	if err != nil {
		return "", fmt.Errorf("failed to create embedding for query: %w", err)
	}

	// Search for similar documents
	results, err := r.rag.SearchSimilar(ctx, vector, args.K, args.Filter)
	if err != nil {
		return "", fmt.Errorf("failed to search for similar documents: %w", err)
	}

	// Format the results
	return rag.FormatSearchResultsAsText(results), nil
}

// handleStoreDocument handles a store document tool call.
func (r *ToolRouter) handleStoreDocument(ctx context.Context, arguments string) (string, error) {
	// Parse the arguments
	var args struct {
		Text       string                 `json:"text"`
		Title      string                 `json:"title"`
		Metadata   map[string]interface{} `json:"metadata"`
		Collection string                 `json:"collection"`
	}

	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", fmt.Errorf("failed to parse store document arguments: %w", err)
	}

	// Create an embedding for the document
	vector, err := r.embed.EmbedQuery(ctx, args.Text)
	if err != nil {
		return "", fmt.Errorf("failed to create embedding for document: %w", err)
	}

	// Store the document
	docID, err := r.rag.StoreDocument(ctx, args.Text, args.Title, args.Collection, args.Metadata, vector)
	if err != nil {
		return "", fmt.Errorf("failed to store document: %w", err)
	}

	return fmt.Sprintf("Document stored with ID: %s", docID), nil
}

// handleGenerateImage handles a generate image tool call.
func (r *ToolRouter) handleGenerateImage(ctx context.Context, arguments string) (string, error) {
	// Parse the arguments
	var args struct {
		Prompt string `json:"prompt"`
		Size   string `json:"size"`
		Style  string `json:"style"`
	}

	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", fmt.Errorf("failed to parse generate image arguments: %w", err)
	}

	// Set default values if not provided
	if args.Size == "" {
		args.Size = "1024x1024"
	}
	if args.Style == "" {
		args.Style = "photorealistic"
	}

	// Generate the image
	imageURL, err := r.llm.GenerateImage(ctx, args.Prompt, args.Size, args.Style)
	if err != nil {
		return "", fmt.Errorf("failed to generate image: %w", err)
	}

	return imageURL, nil
}
