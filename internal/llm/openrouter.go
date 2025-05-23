package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/hunterwarburton/ya8hoda/internal/core"
	"github.com/hunterwarburton/ya8hoda/internal/logger"
	"github.com/hunterwarburton/ya8hoda/internal/telegram"
)

// OpenRouterService implements interactions with the OpenRouter API.
type OpenRouterService struct {
	apiKey          string
	model           string
	httpClient      *http.Client
	character       *Character
	promptGenerator *PromptGenerator
	rag             core.RAGService // RAG service for retrieving relevant facts
}

// OpenRouterError represents an error response from the OpenRouter API.
type OpenRouterError struct {
	Error struct {
		Message  string `json:"message"`
		Code     int    `json:"code"`
		Metadata struct {
			Raw          string `json:"raw"`
			ProviderName string `json:"provider_name"`
		} `json:"metadata"`
	} `json:"error"`
	UserID string `json:"user_id,omitempty"`
}

// Message represents a chat message.
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ImageURLs  []string   `json:"image_urls,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// ToolCall represents a function call from the model.
type ToolCall struct {
	ID       string          `json:"id"`
	Type     string          `json:"type"`
	Function FunctionDetails `json:"function"`
}

// FunctionDetails contains details about a function call.
type FunctionDetails struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ChatResponse represents a response from the chat model.
type ChatResponse struct {
	Message Message `json:"message"`
}

// ChatRequest represents a request to the chat completion API.
type ChatRequest struct {
	Model     string        `json:"model"`
	Messages  []Message     `json:"messages"`
	Tools     []interface{} `json:"tools,omitempty"`
	Stream    bool          `json:"stream,omitempty"`
	MaxTokens int           `json:"max_tokens,omitempty"`
}

// Tool represents a function tool that can be used by the model.
type Tool struct {
	Type     string         `json:"type"`
	Function FunctionSchema `json:"function"`
}

// FunctionSchema represents the schema for a function tool.
type FunctionSchema struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Parameters  interface{} `json:"parameters,omitempty"`
}

// Token/length limiting constants and helpers
const (
	// Rough estimate: 1 token ≈ 4 characters for typical English text.
	approximateCharsPerToken = 4
	maxPromptTokens          = 100000
	maxPromptChars           = approximateCharsPerToken * maxPromptTokens // ≈ 400 000 characters
)

// formatFactWithContext joins fact text with a human-readable timestamp and JSON-encoded metadata.
func formatFactWithContext(doc core.Document) string {
	fact := doc.Text
	var parts []string
	if doc.CreateTime > 0 {
		ts := time.Unix(doc.CreateTime, 0)
		parts = append(parts, fmt.Sprintf("learned %s", ts.Format("2006-01-02 15:04 MST")))
	}
	if len(doc.Metadata) > 0 {
		if metaBytes, err := json.Marshal(doc.Metadata); err == nil {
			parts = append(parts, string(metaBytes))
		}
	}
	if len(parts) > 0 {
		fact = fmt.Sprintf("%s (%s)", fact, strings.Join(parts, ", "))
	}
	return fact
}

// trimLLMMessagesToFit ensures the total length of the chat history (in characters)
// does not exceed the provided limit. It always keeps the first system message
// (if present) and then works backwards from the most-recent message, dropping
// older ones until the limit is satisfied. It returns the trimmed slice (still
// in chronological order) and the number of characters that were discarded.
func trimLLMMessagesToFit(messages []Message, charLimit int) ([]Message, int) {
	if charLimit <= 0 || len(messages) == 0 {
		return messages, 0
	}

	trimmedChars := 0
	startIdx := 0
	var systemMsg *Message

	// Preserve the first system message if it exists.
	if messages[0].Role == "system" {
		systemMsg = &messages[0]
		startIdx = 1
		charLimit -= len(systemMsg.Content)
		if charLimit <= 0 {
			// The system prompt alone exceeds the limit; nothing else can fit.
			return []Message{*systemMsg}, 0
		}
	}

	// Walk backwards through the remaining messages, keeping the newest until
	// we run out of space.
	total := 0
	kept := make([]Message, 0, len(messages)-startIdx)
	for i := len(messages) - 1; i >= startIdx; i-- {
		msgLen := len(messages[i].Content)
		if total+msgLen > charLimit {
			trimmedChars += msgLen
			continue
		}
		total += msgLen
		kept = append(kept, messages[i])
	}

	// Reverse kept to restore chronological order.
	for i := 0; i < len(kept)/2; i++ {
		kept[i], kept[len(kept)-1-i] = kept[len(kept)-1-i], kept[i]
	}

	// Re-assemble final slice.
	if systemMsg != nil {
		return append([]Message{*systemMsg}, kept...), trimmedChars
	}
	return kept, trimmedChars
}

// NewOpenRouterService creates a new instance of OpenRouterService.
func NewOpenRouterService(apiKey, model string) *OpenRouterService {
	return &OpenRouterService{
		apiKey: apiKey,
		model:  model,
		httpClient: &http.Client{
			Timeout: 120 * time.Second, // Set a generous timeout for LLM responses
		},
	}
}

// SetCharacter implements the CharacterAware interface.
func (s *OpenRouterService) SetCharacter(character *Character) error {
	s.character = character
	s.promptGenerator = NewPromptGenerator(character)
	logger.LLMInfo("Character configuration set: %s", character.Name)
	return nil
}

// GetCharacter implements the CharacterAware interface.
func (s *OpenRouterService) GetCharacter() *Character {
	return s.character
}

// SetRAGService allows wiring a RAGService instance into the LLM service after construction.
func (s *OpenRouterService) SetRAGService(rag core.RAGService) {
	s.rag = rag
}

// buildPromptContext embeds the user's most recent query, retrieves relevant facts from the RAG
// service, and converts them into a PromptContext for the prompt generator.
func (s *OpenRouterService) buildPromptContext(ctx context.Context, query string, userInfo *telegram.UserInfo) PromptContext {
	// If there's no RAG service or empty query, return empty context
	if s.rag == nil || query == "" {
		return PromptContext{}
	}

	k := 5

	// --- Personal facts (filtered by the current user if available) ---
	var personalFacts []string
	if userInfo != nil {
		if results, err := s.rag.SearchSelfMemory(ctx, query, k); err == nil {
			for _, res := range results {
				if res.Document.Text != "" {
					personalFacts = append(personalFacts, formatFactWithContext(res.Document))
				}
			}
		} else {
			logger.LLMWarn("Failed fetching personal facts: %v", err)
		}
		if len(personalFacts) > k {
			personalFacts = personalFacts[:k]
		}
	}

	k = 20
	// --- People facts (no user filter) ---
	peopleFacts := make(map[string][]string)
	if results, err := s.rag.SearchPersonalMemory(ctx, query, "", "", k); err == nil {
		for _, res := range results {
			name := res.Document.Name
			if name == "" {
				name = res.Document.OwnerID // fall back to owner id if name missing
			}
			if name == "" {
				name = "Unknown"
			}
			if res.Document.Text != "" {
				peopleFacts[name] = append(peopleFacts[name], formatFactWithContext(res.Document))
			}
		}
	} else {
		logger.LLMWarn("Failed fetching people facts: %v", err)
	}

	// Ensure each slice <= k
	for name, facts := range peopleFacts {
		if len(facts) > k {
			peopleFacts[name] = facts[:k]
		}
	}

	// --- Community facts ---
	communityFacts := make(map[string][]string)
	if results, err := s.rag.SearchCommunityMemory(ctx, query, "", k); err == nil {
		for _, res := range results {
			name := res.Document.Name
			if name == "" {
				name = res.Document.OwnerID
			}
			if name == "" {
				name = "Unknown"
			}
			if res.Document.Text != "" {
				communityFacts[name] = append(communityFacts[name], formatFactWithContext(res.Document))
			}
		}
	} else {
		logger.LLMWarn("Failed fetching community facts: %v", err)
	}

	for name, facts := range communityFacts {
		if len(facts) > k {
			communityFacts[name] = facts[:k]
		}
	}

	return PromptContext{
		PersonalFacts:  personalFacts,
		PeopleFacts:    peopleFacts,
		CommunityFacts: communityFacts,
	}
}

// ChatCompletion sends a chat completion request to OpenRouter.
func (s *OpenRouterService) ChatCompletion(ctx context.Context, telegramMessages []telegram.Message, toolSpecs []interface{}) (*telegram.ChatResponse, error) {
	return s.ChatCompletionWithUserInfo(ctx, telegramMessages, toolSpecs, nil)
}

// ChatCompletionWithUserInfo sends a chat completion request to OpenRouter with user information.
func (s *OpenRouterService) ChatCompletionWithUserInfo(ctx context.Context, telegramMessages []telegram.Message, toolSpecs []interface{}, userInfo *telegram.UserInfo) (*telegram.ChatResponse, error) {
	url := "https://openrouter.ai/api/v1/chat/completions"
	chatID := int64(0) // Default chatID if not available from userInfo (though less likely in this context)
	userID := int64(0)
	if userInfo != nil {
		// Assuming UserInfo has ChatID or can be inferred; adjust if needed.
		// If UserInfo only has UserID, we might need to pass ChatID explicitly.
		userID = userInfo.ID
		// chatID = userInfo.ChatID // Example if ChatID was available
	}

	// Convert telegram messages to openrouter messages
	messages := convertTelegramMessagesToLLM(telegramMessages)
	logger.LLMDebug("ChatID[%d] UserID[%d]: Converted %d Telegram messages to LLM format.", chatID, userID, len(messages))

	// Determine the most recent user query for RAG retrieval
	latestUserQuery := ""
	for i := len(telegramMessages) - 1; i >= 0; i-- {
		if telegramMessages[i].Role == "user" {
			latestUserQuery = telegramMessages[i].Content
			break
		}
	}

	promptCtx := s.buildPromptContext(ctx, latestUserQuery, userInfo)

	// Check if there's an existing system message and replace it with our character system message
	hasSystemMessage := len(messages) > 0 && messages[0].Role == "system"
	if hasSystemMessage {
		if s.promptGenerator != nil {
			var systemPrompt string
			if userInfo != nil {
				systemPrompt = s.promptGenerator.GenerateSystemPromptWithUserInfoAndContext(userInfo, promptCtx)
				logger.LLMDebug("ChatID[%d] UserID[%d]: Replacing system prompt using UserInfo.", chatID, userID)
			} else {
				systemPrompt = s.promptGenerator.GenerateSystemPromptWithUserInfoAndContext(nil, promptCtx)
				logger.LLMDebug("ChatID[%d] UserID[%d]: Replacing system prompt with default character prompt.", chatID, userID)
			}
			promptStart := systemPrompt
			if len(promptStart) > 80 {
				promptStart = promptStart[:80] + "..."
			}
			logger.LLMDebug("ChatID[%d] UserID[%d]: System prompt starts with: %s", chatID, userID, promptStart)
			messages[0].Content = systemPrompt
		} else {
			logger.LLMWarn("ChatID[%d] UserID[%d]: Found system message but no prompt generator available to replace it.", chatID, userID)
		}
	} else if s.promptGenerator != nil {
		var systemPrompt string
		if userInfo != nil {
			systemPrompt = s.promptGenerator.GenerateSystemPromptWithUserInfoAndContext(userInfo, promptCtx)
			logger.LLMDebug("ChatID[%d] UserID[%d]: Creating system prompt with UserInfo.", chatID, userID)
		} else {
			systemPrompt = s.promptGenerator.GenerateSystemPromptWithUserInfoAndContext(nil, promptCtx)
			logger.LLMDebug("ChatID[%d] UserID[%d]: Creating default character system prompt.", chatID, userID)
		}
		promptStart := systemPrompt
		if len(promptStart) > 80 {
			promptStart = promptStart[:80] + "..."
		}
		logger.LLMDebug("ChatID[%d] UserID[%d]: Generated system prompt starts with: %s", chatID, userID, promptStart)
		systemMessage := Message{
			Role:    "system",
			Content: systemPrompt,
		}
		messages = append([]Message{systemMessage}, messages...)
		logger.LLMDebug("ChatID[%d] UserID[%d]: Prepended character system prompt. History length: %d", chatID, userID, len(messages))
	} else {
		logger.LLMWarn("ChatID[%d] UserID[%d]: No existing system message and no prompt generator to create one.", chatID, userID)
	}

	// Trim the history to fit within the token limit
	trimmedMessages, discardedChars := trimLLMMessagesToFit(messages, maxPromptChars)
	logger.LLMInfo("ChatID[%d] UserID[%d]: Trimmed %d characters from history. Discarded %d characters.", chatID, userID, discardedChars, discardedChars)

	// Create the request body
	reqBody := ChatRequest{
		Model:    s.model,
		Messages: trimmedMessages,
	}

	// Add tools if provided
	if len(toolSpecs) > 0 {
		logger.LLMDebug("ChatID[%d] UserID[%d]: Adding %d tools to request.", chatID, userID, len(toolSpecs))
		// Logging each tool spec can be very verbose, comment out unless needed
		/*
			for i, tool := range toolSpecs {
				jsonTool, _ := json.Marshal(tool)
				logger.LLMDebug("ChatID[%d] UserID[%d]: Tool %d: %s", chatID, userID, i, string(jsonTool))
			}
		*/
		reqBody.Tools = toolSpecs
	} else {
		logger.LLMDebug("ChatID[%d] UserID[%d]: No tools provided for request.", chatID, userID)
	}

	// Convert the request to JSON
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		// Use LLMError as this relates to preparing the LLM call
		logger.LLMError("ChatID[%d] UserID[%d]: Failed to marshal LLM request: %v", chatID, userID, err)
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	// logger.LLMDebug("ChatID[%d] UserID[%d]: LLM Request Body: %s", chatID, userID, string(jsonData)) // Very verbose

	logger.LLMInfo("ChatID[%d] UserID[%d]: Sending request to LLM '%s' with %d messages and %d tools.", chatID, userID, s.model, len(trimmedMessages), len(reqBody.Tools))

	// Create the HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		logger.LLMError("ChatID[%d] UserID[%d]: Failed to create HTTP request for LLM: %v", chatID, userID, err)
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set the headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.apiKey)

	// Send the request
	resp, err := s.httpClient.Do(req)
	if err != nil {
		logger.LLMError("ChatID[%d] UserID[%d]: Failed to send HTTP request to LLM: %v", chatID, userID, err)
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.LLMError("ChatID[%d] UserID[%d]: Failed to read LLM response body: %v", chatID, userID, err)
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	// logger.LLMDebug("ChatID[%d] UserID[%d]: Raw LLM Response Body: %s", chatID, userID, string(body)) // Verbose

	// Check for error in response body regardless of status code
	var openRouterErr OpenRouterError
	if err := json.Unmarshal(body, &openRouterErr); err == nil && openRouterErr.Error.Message != "" {
		errMsg := fmt.Sprintf("OpenRouter API error: %s (code: %d)", openRouterErr.Error.Message, openRouterErr.Error.Code)
		if openRouterErr.Error.Metadata.ProviderName != "" {
			errMsg = fmt.Sprintf("OpenRouter API error (%s): %s", openRouterErr.Error.Metadata.ProviderName, openRouterErr.Error.Message)
			if openRouterErr.Error.Metadata.Raw != "" {
				errMsg += fmt.Sprintf(" - Raw: %s", openRouterErr.Error.Metadata.Raw)
			}
		}
		logger.LLMError("ChatID[%d] UserID[%d]: %s", chatID, userID, errMsg)
		return nil, fmt.Errorf(errMsg)
	}

	// Check for HTTP errors
	if resp.StatusCode != http.StatusOK {
		errMsg := fmt.Sprintf("OpenRouter API HTTP error (status %d): %s", resp.StatusCode, string(body))
		logger.LLMError("ChatID[%d] UserID[%d]: %s", chatID, userID, errMsg)
		return nil, fmt.Errorf(errMsg)
	}

	// Parse the complete response
	var openRouterResp struct { // Keep definition inline or move globally
		ID      string `json:"id"`
		Choices []struct {
			FinishReason       string  `json:"finish_reason"`
			NativeFinishReason string  `json:"native_finish_reason"`
			Message            Message `json:"message"`
		} `json:"choices"`
		Created           int64  `json:"created"`
		Model             string `json:"model"`
		Object            string `json:"object"`
		SystemFingerprint string `json:"system_fingerprint,omitempty"`
		Usage             struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage,omitempty"`
	}

	if err := json.Unmarshal(body, &openRouterResp); err != nil {
		logger.LLMError("ChatID[%d] UserID[%d]: Failed to decode LLM success response: %v", chatID, userID, err)
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Make sure we have a choice
	if len(openRouterResp.Choices) == 0 {
		logger.LLMError("ChatID[%d] UserID[%d]: OpenRouter API returned no choices in response.", chatID, userID)
		return nil, fmt.Errorf("OpenRouter API returned no choices")
	}

	// Log usage info
	if openRouterResp.Usage.TotalTokens > 0 {
		logger.LLMInfo("ChatID[%d] UserID[%d]: LLM Usage - Prompt: %d, Completion: %d, Total: %d tokens. Finish Reason: %s",
			chatID, userID,
			openRouterResp.Usage.PromptTokens,
			openRouterResp.Usage.CompletionTokens,
			openRouterResp.Usage.TotalTokens,
			openRouterResp.Choices[0].FinishReason,
		)
	} else {
		logger.LLMInfo("ChatID[%d] UserID[%d]: LLM call completed. Finish Reason: %s (Usage data unavailable)",
			chatID, userID, openRouterResp.Choices[0].FinishReason)
	}

	// Convert back to telegram format
	responseMessage := convertLLMMessageToTelegram(openRouterResp.Choices[0].Message)

	// Convert to the expected response format
	response := &telegram.ChatResponse{
		Message: responseMessage,
	}

	// Log response content preview
	preview := response.Message.Content
	if len(preview) > 80 {
		preview = preview[:80] + "..."
	}
	toolCallInfo := ""
	if len(response.Message.ToolCalls) > 0 {
		toolCallInfo = fmt.Sprintf(" (ToolCalls: %d)", len(response.Message.ToolCalls))
	}
	logger.LLMDebug("ChatID[%d] UserID[%d]: Prepared LLM response for Telegram: \"%s\"%s", chatID, userID, preview, toolCallInfo)

	return response, nil
}

// GenerateImage sends an image generation request to OpenRouter.
func (s *OpenRouterService) GenerateImage(ctx context.Context, prompt, size, style string) (string, error) {
	url := "https://openrouter.ai/api/v1/images/generations"

	// Enhance prompt with character personality if available
	if s.promptGenerator != nil {
		prompt = s.promptGenerator.EnhanceImagePrompt(prompt)
		logger.LLMDebug("Enhanced image prompt using character personality.")
	}

	// Create the request body
	reqBody := map[string]interface{}{ // Use specific struct if preferred
		"prompt": prompt,
		"model":  s.model, // Ensure this model supports image generation!
		"size":   size,
		"style":  style,
		// "n": 1, // Usually default
		// "response_format": "url", // Usually default
	}

	// Convert the request to JSON
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		logger.LLMError("Failed to marshal image request: %v", err)
		return "", fmt.Errorf("failed to marshal image request: %w", err)
	}

	logger.LLMInfo("Sending image generation request to LLM '%s' (Style: %s, Size: %s). Prompt: \"%s\"", s.model, style, size, prompt)

	// Create the HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		logger.LLMError("Failed to create image request HTTP object: %v", err)
		return "", fmt.Errorf("failed to create image request: %w", err)
	}

	// Set the headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.apiKey)

	// Send the request
	resp, err := s.httpClient.Do(req)
	if err != nil {
		logger.LLMError("Failed to send image request: %v", err)
		return "", fmt.Errorf("failed to send image request: %w", err)
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.LLMError("Failed to read image response body: %v", err)
		return "", fmt.Errorf("failed to read image response body: %w", err)
	}

	// Check for HTTP errors first
	if resp.StatusCode != http.StatusOK {
		// Try to parse as an OpenRouter error response
		var openRouterErr OpenRouterError
		if err := json.Unmarshal(body, &openRouterErr); err == nil && openRouterErr.Error.Message != "" {
			errMsg := fmt.Sprintf("OpenRouter Image API error: %s (code: %d)", openRouterErr.Error.Message, openRouterErr.Error.Code)
			if openRouterErr.Error.Metadata.ProviderName != "" {
				errMsg = fmt.Sprintf("OpenRouter Image API error (%s): %s", openRouterErr.Error.Metadata.ProviderName, openRouterErr.Error.Message)
				if openRouterErr.Error.Metadata.Raw != "" {
					errMsg += fmt.Sprintf(" - Raw: %s", openRouterErr.Error.Metadata.Raw)
				}
			}
			logger.LLMError(errMsg)
			return "", fmt.Errorf(errMsg)
		}
		// Fallback to generic HTTP error
		errMsg := fmt.Sprintf("OpenRouter Image API HTTP error (status %d): %s", resp.StatusCode, string(body))
		logger.LLMError(errMsg)
		return "", fmt.Errorf(errMsg)
	}

	// Parse the success response
	var imageResp struct { // Keep inline or move globally
		Data []struct {
			URL string `json:"url"`
			// Add other fields if needed, e.g., b64_json
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &imageResp); err != nil {
		logger.LLMError("Failed to decode image success response: %v. Body: %s", err, string(body))
		return "", fmt.Errorf("failed to decode image response: %w", err)
	}

	// Make sure we have an image URL
	if len(imageResp.Data) == 0 || imageResp.Data[0].URL == "" {
		logger.LLMError("OpenRouter Image API returned no image URL in data.")
		return "", fmt.Errorf("OpenRouter API returned no image URL")
	}

	logger.LLMInfo("Image generated successfully. URL: %s", imageResp.Data[0].URL)
	return imageResp.Data[0].URL, nil
}

// Helper functions to convert between telegram and llm types

func convertTelegramMessagesToLLM(telegramMessages []telegram.Message) []Message {
	llmMessages := make([]Message, len(telegramMessages))
	for i, tgMsg := range telegramMessages {
		llmMsg := Message{
			Role:       tgMsg.Role,
			Content:    tgMsg.Content,
			ImageURLs:  tgMsg.ImageURLs,
			ToolCallID: tgMsg.ToolCallID,
		}

		// Convert tool calls if any
		if len(tgMsg.ToolCalls) > 0 {
			llmMsg.ToolCalls = make([]ToolCall, len(tgMsg.ToolCalls))
			for j, tgToolCall := range tgMsg.ToolCalls {
				llmMsg.ToolCalls[j] = ToolCall{
					ID:   tgToolCall.ID,
					Type: tgToolCall.Type,
					Function: FunctionDetails{
						Name:      tgToolCall.Function.Name,
						Arguments: tgToolCall.Function.Arguments,
					},
				}
			}
		}

		llmMessages[i] = llmMsg
	}
	return llmMessages
}

func convertLLMMessageToTelegram(llmMsg Message) telegram.Message {
	tgMsg := telegram.Message{
		Role:       llmMsg.Role,
		Content:    llmMsg.Content,
		ImageURLs:  llmMsg.ImageURLs,
		ToolCallID: llmMsg.ToolCallID,
	}

	// Convert tool calls if any
	if len(llmMsg.ToolCalls) > 0 {
		tgMsg.ToolCalls = make([]telegram.ToolCall, len(llmMsg.ToolCalls))
		for i, llmToolCall := range llmMsg.ToolCalls {
			tgMsg.ToolCalls[i] = telegram.ToolCall{
				ID:   llmToolCall.ID,
				Type: llmToolCall.Type,
				Function: telegram.FunctionDetails{
					Name:      llmToolCall.Function.Name,
					Arguments: llmToolCall.Function.Arguments,
				},
			}
		}
	}

	return tgMsg
}
