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

	"github.com/hunterwarburton/ya8hoda/internal/logger"
	"github.com/hunterwarburton/ya8hoda/internal/telegram"
)

// OpenRouterService implements interactions with the OpenRouter API.
type OpenRouterService struct {
	apiKey     string
	model      string
	httpClient *http.Client
	character  *Character
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
	logger.Info("Character configuration set: %s", character.Name)
	return nil
}

// GetCharacter implements the CharacterAware interface.
func (s *OpenRouterService) GetCharacter() *Character {
	return s.character
}

// generateSystemPrompt creates a system prompt based on the character configuration.
func (s *OpenRouterService) generateSystemPrompt() string {
	if s.character == nil {
		return "You are a helpful assistant."
	}

	var builder strings.Builder

	// Add character name and basic identity
	builder.WriteString(fmt.Sprintf("You are %s. ", s.character.Name))

	// Add bio information
	if len(s.character.Bio) > 0 {
		builder.WriteString("Here's your background: ")
		builder.WriteString(strings.Join(s.character.Bio, " "))
		builder.WriteString("\n\n")
	}

	// Add lore
	if len(s.character.Lore) > 0 {
		builder.WriteString("Additional background details: ")
		builder.WriteString(strings.Join(s.character.Lore, " "))
		builder.WriteString("\n\n")
	}

	// Add communication style
	if len(s.character.Style.Chat) > 0 {
		builder.WriteString("Your communication style: ")
		builder.WriteString(strings.Join(s.character.Style.Chat, ", "))
		builder.WriteString("\n\n")
	}

	// Add example topics
	if len(s.character.Topics) > 0 {
		builder.WriteString("Topics you're knowledgeable about: ")
		builder.WriteString(strings.Join(s.character.Topics, ", "))
		builder.WriteString("\n\n")
	}

	// Add personality traits
	if len(s.character.Adjectives) > 0 {
		builder.WriteString("Your personality traits: ")
		builder.WriteString(strings.Join(s.character.Adjectives, ", "))
		builder.WriteString("\n\n")
	}

	builder.WriteString("Respond to the user as this character, maintaining consistency with your background and personality at all times.")

	return builder.String()
}

// ChatCompletion sends a chat completion request to OpenRouter.
func (s *OpenRouterService) ChatCompletion(ctx context.Context, telegramMessages []telegram.Message, toolSpecs []interface{}) (*telegram.ChatResponse, error) {
	url := "https://openrouter.ai/api/v1/chat/completions"

	// Convert telegram messages to openrouter messages
	messages := convertTelegramMessagesToLLM(telegramMessages)

	// Add system message if character is configured and no system message exists yet
	hasSystemMessage := len(messages) > 0 && messages[0].Role == "system"
	if s.character != nil && !hasSystemMessage {
		systemPrompt := s.generateSystemPrompt()
		// Prepend the system message
		systemMessage := Message{
			Role:    "system",
			Content: systemPrompt,
		}
		messages = append([]Message{systemMessage}, messages...)
		logger.Debug("Added character system prompt: %s", systemPrompt)
	}

	// Create the request body
	reqBody := ChatRequest{
		Model:    s.model,
		Messages: messages,
	}

	// Add tools if provided
	//if len(toolSpecs) > 0 {
	//	reqBody.Tools = toolSpecs
	//}
	// Convert the request to JSON
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Pretty print the request JSON for debugging
	if logger.IsDebugEnabled() {
		var prettyJSON bytes.Buffer
		if err := json.Indent(&prettyJSON, jsonData, "", "  "); err == nil {
			logger.Debug("OpenRouter request: %s", prettyJSON.String())
		} else {
			logger.Debug("OpenRouter request (raw): %s", string(jsonData))
		}
	}

	// Create the HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set the headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.apiKey)

	// Send the request
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Check for error in response body regardless of status code
	var openRouterErr OpenRouterError
	if err := json.Unmarshal(body, &openRouterErr); err == nil && openRouterErr.Error.Message != "" {
		// Return a detailed error message
		if openRouterErr.Error.Metadata.Raw != "" {
			return nil, fmt.Errorf("OpenRouter API error (%s): %s - Raw provider error: %s",
				openRouterErr.Error.Metadata.ProviderName,
				openRouterErr.Error.Message,
				openRouterErr.Error.Metadata.Raw)
		}
		return nil, fmt.Errorf("OpenRouter API error: %s (code: %d)",
			openRouterErr.Error.Message,
			openRouterErr.Error.Code)
	}

	// Check for HTTP errors
	if resp.StatusCode != http.StatusOK {
		// Fallback to generic error if not parsed above
		return nil, fmt.Errorf("OpenRouter API error (status %d): %s", resp.StatusCode, string(body))
	}

	// Parse the complete response
	var openRouterResp struct {
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
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Make sure we have a choice
	if len(openRouterResp.Choices) == 0 {
		return nil, fmt.Errorf("OpenRouter API returned no choices")
	}

	// Convert back to telegram format
	responseMessage := convertLLMMessageToTelegram(openRouterResp.Choices[0].Message)

	// Convert to the expected response format
	response := &telegram.ChatResponse{
		Message: responseMessage,
	}

	logger.Debug("OpenRouter response: %+v", response)

	return response, nil
}

// GenerateImage sends an image generation request to OpenRouter.
func (s *OpenRouterService) GenerateImage(ctx context.Context, prompt, size, style string) (string, error) {
	url := "https://openrouter.ai/api/v1/images/generations"

	// Enhance prompt with character personality if available
	if s.character != nil {
		prompt = fmt.Sprintf("Create an image as if you were %s, who is %s. %s",
			s.character.Name,
			strings.Join(s.character.Adjectives[:min(3, len(s.character.Adjectives))], ", "),
			prompt)
	}

	// Create the request body
	reqBody := map[string]interface{}{
		"prompt": prompt,
		"model":  "anthropic/claude-3-opus", // Use a model that supports image generation, adjust as needed
		"size":   size,
		"style":  style,
	}

	// Convert the request to JSON
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal image request: %w", err)
	}

	// Create the HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create image request: %w", err)
	}

	// Set the headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.apiKey)

	// Send the request
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send image request: %w", err)
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	// Check for HTTP errors
	if resp.StatusCode != http.StatusOK {
		// Try to parse as an OpenRouter error response
		var openRouterErr OpenRouterError
		if err := json.Unmarshal(body, &openRouterErr); err == nil && openRouterErr.Error.Message != "" {
			// Return a detailed error message
			if openRouterErr.Error.Metadata.Raw != "" {
				return "", fmt.Errorf("OpenRouter API error (%s): %s - Raw provider error: %s",
					openRouterErr.Error.Metadata.ProviderName,
					openRouterErr.Error.Message,
					openRouterErr.Error.Metadata.Raw)
			}
			return "", fmt.Errorf("OpenRouter API error: %s (code: %d)",
				openRouterErr.Error.Message,
				openRouterErr.Error.Code)
		}
		// Fallback to generic error
		return "", fmt.Errorf("OpenRouter API error (status %d): %s", resp.StatusCode, string(body))
	}

	// Parse the response
	var imageResp struct {
		Data []struct {
			URL string `json:"url"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &imageResp); err != nil {
		return "", fmt.Errorf("failed to decode image response: %w", err)
	}

	// Make sure we have an image URL
	if len(imageResp.Data) == 0 || imageResp.Data[0].URL == "" {
		return "", fmt.Errorf("OpenRouter API returned no image URL")
	}

	return imageResp.Data[0].URL, nil
}

// Helper function to get minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
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
