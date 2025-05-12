package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hunterwarburton/ya8hoda/internal/core"
	"github.com/hunterwarburton/ya8hoda/internal/logger"
	"github.com/hunterwarburton/ya8hoda/internal/rag"
	"github.com/hunterwarburton/ya8hoda/internal/solana"
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
			CommunityName string                 `json:"community_name"`
			MemoryText    string                 `json:"memory_text"`
			Metadata      map[string]interface{} `json:"metadata"`
		}
		if err = json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return "", fmt.Errorf("failed to parse store_community_memory arguments: %w", err)
		}
		logger.Debug("Parsed store_community_memory arguments: %+v", args)
		if args.MemoryText == "" || args.CommunityName == "" {
			return "", fmt.Errorf("memory_text and community_name are required for store_community_memory")
		}
		// Call the RAGService interface method for storing community memory (using name as identifier)
		result, err = r.rag.RememberAboutCommunity(ctx, args.CommunityName, args.MemoryText, args.Metadata)
		if err != nil {
			return "", fmt.Errorf("failed to execute store_community_memory: %w", err)
		}

	// Retrieval functions
	case "remember_about_person":
		var args struct {
			TelegramID string `json:"telegram_id,omitempty"` // Optional, searches all if empty
			PersonName string `json:"person_name,omitempty"` // Optional, searches by name if provided
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
		searchResults, err = r.rag.SearchPersonalMemory(ctx, args.Query, args.TelegramID, args.PersonName, args.K)
		if err != nil {
			return "", fmt.Errorf("failed to execute remember_about_person: %w", err)
		}
		result = rag.FormatSearchResultsAsJSON(searchResults) // Use rag.FormatSearchResultsAsJSON

	case "remember_about_community":
		var args struct {
			CommunityName string `json:"community_name,omitempty"` // Optional, searches all if empty
			Query         string `json:"query"`
			K             int    `json:"k"`
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
		// Call the RAGService interface method for searching community memory (using name as identifier)
		var searchResults []core.SearchResult
		searchResults, err = r.rag.SearchCommunityMemory(ctx, args.Query, args.CommunityName, args.K)
		if err != nil {
			return "", fmt.Errorf("failed to execute remember_about_community: %w", err)
		}
		result = rag.FormatSearchResultsAsJSON(searchResults) // Use rag.FormatSearchResultsAsJSON

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
		result = rag.FormatSearchResultsAsJSON(searchResults) // Use rag.FormatSearchResultsAsJSON

	case "solana_get_tokens":
		var args struct {
			AddressOrName string `json:"address_or_name"`
		}
		if err = json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return "", fmt.Errorf("failed to parse solana_get_tokens arguments: %w", err)
		}
		logger.Debug("Parsed solana_get_tokens arguments: %+v", args)
		if args.AddressOrName == "" {
			return "", fmt.Errorf("address_or_name is required for solana_get_tokens")
		}
		// Create a lightweight Solana RPC client (could be reused if needed)
		client := solana.NewClient("") // default endpoint
		var owner string
		owner, err = client.ResolveAddress(ctx, args.AddressOrName)
		if err != nil {
			return "", fmt.Errorf("failed to resolve address: %w", err)
		}
		var balances []solana.TokenAccount
		balances, err = client.GetTokenBalances(ctx, owner)
		if err != nil {
			return "", fmt.Errorf("failed to fetch token balances: %w", err)
		}

		// NEW: Simplify the balances for the LLM
		type SimplifiedTokenAccount struct {
			Mint   string  `json:"mint"`
			Amount float64 `json:"amount"`
			Symbol string  `json:"symbol,omitempty"`
			Name   string  `json:"name,omitempty"`
		}
		simplifiedBalances := make([]SimplifiedTokenAccount, 0, len(balances))
		for _, acc := range balances {
			simplifiedBalances = append(simplifiedBalances, SimplifiedTokenAccount{
				Mint:   acc.Mint,
				Amount: acc.Amount,
				Symbol: acc.Symbol,
				Name:   acc.Name,
			})
		}

		// Marshal simplified balances as JSON for return
		var jsonRes []byte
		if jsonRes, err = json.Marshal(simplifiedBalances); err != nil { // Marshal simplifiedBalances instead
			return "", fmt.Errorf("failed to encode simplified balances: %w", err)
		}
		logger.Debug("Simplified Solana token balances: %s", string(jsonRes))
		result = string(jsonRes)

	case "solana_get_token_info":
		var args struct {
			MintAddresses []string `json:"mint_addresses"`
		}
		if err = json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return "", fmt.Errorf("failed to parse solana_get_token_info arguments: %w", err)
		}
		logger.Debug("Parsed solana_get_token_info arguments: %+v", args)
		if len(args.MintAddresses) == 0 {
			return "", fmt.Errorf("mint_addresses list cannot be empty for solana_get_token_info")
		}

		client := solana.NewClient("") // default endpoint
		var allTokenInfo []*solana.FullTokenInfo

		// Create a context for the metadata fetching operations
		// Consider using a shared context if multiple calls are made, managing timeouts appropriately.
		// For simplicity here, each GetTokenMetadata call will manage its own internal context if needed, or use the passed one.

		for _, mintAddr := range args.MintAddresses {
			// It's good practice to have a timeout for each external call.
			// The GetTokenMetadata function itself has internal timeouts for HTTP requests and RPC calls.
			// We can pass the main context `ctx` which might have an overall timeout for the entire tool execution.
			tokenInfo, fetchErr := client.GetTokenMetadata(ctx, mintAddr)
			if fetchErr != nil {
				// Decide how to handle errors: continue and collect partial results, or fail fast.
				// For now, log the error and add a placeholder or skip.
				// If GetTokenMetadata returns a FullTokenInfo even on error (e.g. for permanently bad tokens), use it.
				if tokenInfo != nil && tokenInfo.IsPermanentlyBad {
					logger.ToolWarn("Permanently bad token encountered for mint %s: %s. Including error info.", mintAddr, tokenInfo.ErrorMessage)
					allTokenInfo = append(allTokenInfo, tokenInfo)
				} else {
					logger.ToolWarn("Failed to fetch metadata for mint %s: %v. Skipping this token.", mintAddr, fetchErr)
					// Optionally, add a specific error entry for this mint if desired by the LLM
					allTokenInfo = append(allTokenInfo, &solana.FullTokenInfo{
						MintAddress:      mintAddr,
						IsPermanentlyBad: true, // Mark as bad if fetch failed critically
						ErrorType:        "fetch_failed",
						ErrorMessage:     fetchErr.Error(),
					})
				}
				continue
			}
			if tokenInfo != nil {
				allTokenInfo = append(allTokenInfo, tokenInfo)
			}
		}

		if len(allTokenInfo) == 0 && len(args.MintAddresses) > 0 {
			// This case means all fetches failed and we didn't even get error structs back for some reason.
			return "", fmt.Errorf("failed to fetch any token information for the provided mint addresses")
		}

		var jsonRes []byte
		if jsonRes, err = json.Marshal(allTokenInfo); err != nil {
			return "", fmt.Errorf("failed to encode token_info results: %w", err)
		}
		logger.Debug("Solana token info results: %s", string(jsonRes))
		result = string(jsonRes)

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
