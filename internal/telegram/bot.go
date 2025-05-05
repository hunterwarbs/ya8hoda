package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/hunterwarburton/ya8hoda/internal/core"
	"github.com/hunterwarburton/ya8hoda/internal/logger"
)

// UserInfo holds basic information about the user
type UserInfo struct {
	ID        int64
	Username  string
	FirstName string
	LastName  string
	FullName  string // Combined first and last name
}

// LLMService defines the interface for interacting with an LLM.
type LLMService interface {
	ChatCompletion(ctx context.Context, messages []Message, toolSpecs []interface{}) (*ChatResponse, error)
	ChatCompletionWithUserInfo(ctx context.Context, messages []Message, toolSpecs []interface{}, userInfo *UserInfo) (*ChatResponse, error)
	GenerateImage(ctx context.Context, prompt, size, style string) (string, error)
}

// EmbedService interface definition removed. Using core.EmbedService.

// ToolRouter defines the interface for routing and executing tool calls.
type ToolRouter interface {
	ExecuteToolCall(ctx context.Context, userID int64, toolCall *ToolCall) (string, error)
}

// PolicyService defines the interface for checking user permissions.
type PolicyService interface {
	IsToolAllowed(userID int64, toolName string) bool
	GetAllowedTools(userID int64) []string
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

// Bot represents a Telegram bot.
type Bot struct {
	bot           *bot.Bot
	llm           LLMService
	embed         core.EmbedService
	toolRouter    ToolRouter
	policyService PolicyService
	sessions      map[int64][]Message
	userInfo      map[int64]*UserInfo // Store user information by chat ID
	mutex         sync.RWMutex
	// Character configuration
	characterPrompt string
}

// NewBot creates a new bot instance.
func NewBot(token string, llm LLMService, embed core.EmbedService, toolRouter ToolRouter, policyService PolicyService) (*Bot, error) {
	b := &Bot{
		llm:           llm,
		embed:         embed,
		toolRouter:    toolRouter,
		policyService: policyService,
		sessions:      make(map[int64][]Message),
		userInfo:      make(map[int64]*UserInfo),
		mutex:         sync.RWMutex{},
	}

	// Initialize the bot with our handler
	botAPI, err := bot.New(token, bot.WithDefaultHandler(b.handleUpdate))
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Telegram bot: %w", err)
	}

	b.bot = botAPI
	return b, nil
}

// SetCharacter sets the character prompt for the bot
func (b *Bot) SetCharacter(characterPrompt string) {
	b.mutex.Lock()
	b.characterPrompt = characterPrompt
	b.mutex.Unlock()
}

// getSystemPrompt returns the appropriate system prompt based on character settings
func (b *Bot) getSystemPrompt(chatID int64) string {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	if b.characterPrompt != "" {
		// If we have user information for this chat, include it in the prompt
		if userInfo, exists := b.userInfo[chatID]; exists {
			return b.characterPrompt + fmt.Sprintf(" You are talking to %s (ID: %d).",
				userInfo.FullName, userInfo.ID)
		}
		return b.characterPrompt
	}

	// Default prompt, with user info if available
	if userInfo, exists := b.userInfo[chatID]; exists {
		return fmt.Sprintf("You are a helpful assistant talking to %s (ID: %d). You have access to a vector database using the milvus.search tool. When the user asks a question, consider whether to search for relevant information before answering.",
			userInfo.FullName, userInfo.ID)
	}

	return "You are a helpful assistant with access to a vector database using the milvus.search tool. When the user asks a question, consider whether to search for relevant information before answering."
}

// Start starts the bot.
func (b *Bot) Start(ctx context.Context) {
	b.bot.Start(ctx)
}

// Stop stops the bot.
func (b *Bot) Stop(ctx context.Context) {
	// The go-telegram/bot library doesn't have an explicit Stop method
	// It will stop when the context is canceled
}

// handleUpdate handles a Telegram update.
func (b *Bot) handleUpdate(ctx context.Context, tgbot *bot.Bot, update *models.Update) {
	// Handle different types of updates
	if update.Message != nil {
		// Store user information when a message is received
		b.updateUserInfo(update.Message)

		// If the message is a command
		if update.Message.Text != "" && update.Message.Text[0] == '/' {
			b.handleCommand(ctx, update.Message)
			return
		}

		// If the message has text
		if update.Message.Text != "" {
			b.handleTextMessage(ctx, update.Message)
			return
		}

		// If the message has photos
		if len(update.Message.Photo) > 0 {
			b.handlePhotoMessage(ctx, update.Message)
			return
		}

		// Ignore other types of messages for now
		chatID := update.Message.Chat.ID
		userID := update.Message.From.ID
		logger.TelegramInfo("Chat[%d] User[%d]: Ignored unhandled message type.", chatID, userID)

	} else if update.CallbackQuery != nil {
		// Handle button clicks
		b.handleCallbackQuery(ctx, update.CallbackQuery)
	}
}

// updateUserInfo updates the stored information about a user
func (b *Bot) updateUserInfo(message *models.Message) {
	if message == nil || message.From == nil {
		return
	}

	// Create a full name from first name and last name
	fullName := message.From.FirstName
	if message.From.LastName != "" {
		fullName += " " + message.From.LastName
	}

	// If we just have a username but no first/last name, use that
	if fullName == "" && message.From.Username != "" {
		fullName = "@" + message.From.Username
	}

	// Create the user info
	userInfo := &UserInfo{
		ID:        message.From.ID,
		Username:  message.From.Username,
		FirstName: message.From.FirstName,
		LastName:  message.From.LastName,
		FullName:  fullName,
	}

	// Store it for this chat
	b.mutex.Lock()
	b.userInfo[message.Chat.ID] = userInfo
	b.mutex.Unlock()

	logger.TelegramDebug("Updated user info for chat %d: %s (ID: %d)", message.Chat.ID, fullName, message.From.ID)
}

// handleCommand processes a command message.
func (b *Bot) handleCommand(ctx context.Context, message *models.Message) {
	command := strings.Split(message.Text, " ")[0]
	command = strings.TrimPrefix(command, "/")
	chatID := message.Chat.ID
	userID := message.From.ID
	logger.TelegramInfo("Chat[%d] User[%d]: Received command: /%s", chatID, userID, command)

	switch command {
	case "start":
		text := "👋 Hello! I'm Ya8hoda. AMA!"
		text += "\n\nCommands:"
		text += "\n/help - Show this help message"
		text += "\n/reset - Clear your conversation history"

		b.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    message.Chat.ID,
			Text:      text,
			ParseMode: "MarkdownV2",
		})

		// Get the prompt *before* locking
		systemPrompt := b.getSystemPrompt(message.Chat.ID)

		// Initialize a fresh session
		b.mutex.Lock()
		b.sessions[message.Chat.ID] = []Message{
			{
				Role:    "system",
				Content: systemPrompt,
			},
		}
		b.mutex.Unlock()
		logger.TelegramDebug("Chat[%d]: Initialized new session on /start", chatID)

	case "help":
		text := "Available commands:"
		text += "\n/start - Start or restart the bot"
		text += "\n/help - Show this help message"
		text += "\n/reset - Clear your conversation history"

		b.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: message.Chat.ID,
			Text:   text,
		})

	case "reset":
		b.mutex.Lock()
		b.sessions[message.Chat.ID] = []Message{
			{
				Role:    "system",
				Content: b.getSystemPrompt(message.Chat.ID),
			},
		}
		b.mutex.Unlock()
		logger.TelegramInfo("Chat[%d]: User reset conversation history.", chatID)

		b.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: message.Chat.ID,
			Text:   "✅ Your conversation history has been reset.",
		})

	default:
		logger.TelegramInfo("Chat[%d] User[%d]: Unknown command received: /%s", chatID, userID, command)
		b.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: message.Chat.ID,
			Text:   "Unknown command. Try /help to see available commands.",
		})
	}
}

// sendContinuousTypingAction sends the typing action periodically until the done channel is closed
func (b *Bot) sendContinuousTypingAction(ctx context.Context, chatID int64, done chan struct{}) {
	ticker := time.NewTicker(4 * time.Second) // Telegram typing status lasts ~5 seconds
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			b.bot.SendChatAction(ctx, &bot.SendChatActionParams{
				ChatID: chatID,
				Action: "typing",
			})
		case <-ctx.Done():
			logger.TelegramDebug("Chat[%d]: Context cancelled, stopping typing action.", chatID)
			return
		}
	}
}

// handleTextMessage processes a message with text.
func (b *Bot) handleTextMessage(ctx context.Context, message *models.Message) {
	chatID := message.Chat.ID
	userID := message.From.ID
	logger.TelegramInfo("Chat[%d] User[%d]: Received text message.", chatID, userID)

	// Update user information
	b.updateUserInfo(message)

	// Get the current session for this chat
	session := b.getOrCreateSession(chatID)
	logger.TelegramDebug("Chat[%d]: Session retrieved/created. History length: %d", chatID, len(session))

	// Start typing indicator
	typingDone := make(chan struct{})
	go b.sendContinuousTypingAction(ctx, chatID, typingDone)
	defer close(typingDone) // Ensure typing stops

	// Create a user message
	userMessage := Message{
		Role:    "user",
		Content: message.Text,
	}

	// Add the user message to the session
	b.mutex.Lock()
	session = append(session, userMessage)
	b.sessions[chatID] = session
	b.mutex.Unlock()
	logger.TelegramDebug("Chat[%d]: Added user message to session. History length: %d", chatID, len(session))

	// Load tool specifications from JSON files for this specific user
	toolSpecs, err := b.filterToolSpecs(userID)
	if err != nil {
		log.Printf("Chat[%d] User[%d]: Error loading tool specs: %v", chatID, userID, err)
		b.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   "Sorry, I encountered an internal error preparing my tools.",
		})
		return
	}
	logger.TelegramDebug("Chat[%d] User[%d]: Loaded %d allowed tools.", chatID, userID, len(toolSpecs))

	// Get the user info from our storage
	b.mutex.RLock()
	userInfo := b.userInfo[chatID]
	b.mutex.RUnlock()

	// === First LLM Call ===
	logger.LLMDebug("Chat[%d]: Initiating LLM call (User Request). History length: %d", chatID, len(session))
	var response *ChatResponse
	if userInfo != nil {
		response, err = b.llm.ChatCompletionWithUserInfo(ctx, session, toolSpecs, userInfo)
	} else {
		response, err = b.llm.ChatCompletion(ctx, session, toolSpecs)
	}

	if err != nil {
		logger.LLMError("Chat[%d] User[%d]: Error from LLM (1st call): %v", chatID, userID, err)
		b.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   "Sorry, I encountered an error while generating a response.",
		})
		return
	}
	logger.LLMDebug("Chat[%d]: Received LLM response (1st call). Tool calls requested: %t", chatID, len(response.Message.ToolCalls) > 0)

	// Process tool calls from the LLM response
	if len(response.Message.ToolCalls) > 0 {
		logger.LLMInfo("Chat[%d]: LLM requested %d tool calls.", chatID, len(response.Message.ToolCalls))

		// Add the assistant message *requesting* the tool calls to the session
		b.mutex.Lock()
		session = append(session, response.Message)
		b.sessions[chatID] = session
		b.mutex.Unlock()
		logger.TelegramDebug("Chat[%d]: Added assistant message (tool request) to session. History length: %d", chatID, len(session))

		// Execute each tool call
		toolResults := make([]Message, 0, len(response.Message.ToolCalls))
		for i, toolCall := range response.Message.ToolCalls {
			toolName := toolCall.Function.Name
			logger.ToolInfo("Chat[%d] User[%d]: Executing tool call %d/%d: %s", chatID, userID, i+1, len(response.Message.ToolCalls), toolName)

			// Automatically inject the user's telegram ID and name for store_person_memory if needed
			if toolCall.Function.Name == "store_person_memory" {
				var args map[string]interface{}
				if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
					logger.ToolError("Chat[%d]: Error parsing tool arguments for %s: %v", chatID, toolName, err)
				} else {
					// Ensure telegram_id exists
					if _, exists := args["telegram_id"]; !exists {
						args["telegram_id"] = fmt.Sprintf("%d", userID)
						logger.ToolDebug("Chat[%d]: Injected telegram_id=%d into %s arguments", chatID, userID, toolName)
					}

					// Inject person_name if not present
					if _, exists := args["person_name"]; !exists {
						b.mutex.RLock()
						callingUserInfo := b.userInfo[chatID] // Get info of the user making the call
						b.mutex.RUnlock()
						if callingUserInfo != nil {
							personName := callingUserInfo.FullName
							if personName == "" {
								personName = callingUserInfo.Username // Fallback to username
							}
							if personName != "" {
								args["person_name"] = personName
								logger.ToolDebug("Chat[%d]: Injected person_name='%s' into %s arguments", chatID, personName, toolName)
							} else {
								logger.ToolWarn("Chat[%d]: Could not find name for user %d to inject into %s", chatID, userID, toolName)
							}
						} else {
							logger.ToolWarn("Chat[%d]: User info not found for user %d to inject name into %s", chatID, userID, toolName)
						}
					}

					// Re-marshal arguments if modified
					newArgs, err := json.Marshal(args)
					if err != nil {
						logger.ToolError("Chat[%d]: Error encoding modified arguments for %s: %v", chatID, toolName, err)
					} else {
						toolCall.Function.Arguments = string(newArgs)
					}
				}
			}

			// Execute the tool
			toolResultContent, toolErr := b.toolRouter.ExecuteToolCall(ctx, userID, &toolCall)
			if toolErr != nil {
				logger.ToolError("Chat[%d] User[%d]: Error executing tool %s: %v", chatID, userID, toolName, toolErr)
				b.bot.SendMessage(ctx, &bot.SendMessageParams{
					ChatID: chatID,
					Text:   fmt.Sprintf("Sorry, I encountered an error while trying to use the '%s' tool.", toolName),
				})
				return
			}
			logger.ToolInfo("Chat[%d]: Tool call '%s' executed successfully.", chatID, toolName)

			// Create a tool response message
			toolResponse := Message{
				Role:       "tool",
				Content:    toolResultContent,
				ToolCallID: toolCall.ID,
			}
			toolResults = append(toolResults, toolResponse)

			// Add the tool response to the session immediately
			b.mutex.Lock()
			session = append(session, toolResponse)
			b.sessions[chatID] = session
			b.mutex.Unlock()
			logger.TelegramDebug("Chat[%d]: Added tool result (%s) to session. History length: %d", chatID, toolName, len(session))
		}

		// === Second LLM Call ===
		logger.LLMDebug("Chat[%d]: Initiating LLM call (Tool Results). History length: %d", chatID, len(session))
		if userInfo != nil {
			response, err = b.llm.ChatCompletionWithUserInfo(ctx, session, toolSpecs, userInfo)
		} else {
			response, err = b.llm.ChatCompletion(ctx, session, toolSpecs)
		}

		if err != nil {
			logger.LLMError("Chat[%d] User[%d]: Error from LLM (2nd call, after tools): %v", chatID, userID, err)
			b.bot.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: chatID,
				Text:   "Sorry, I encountered an error while processing the tool results.",
			})
			return
		}
		logger.LLMDebug("Chat[%d]: Received LLM response (2nd call, final). Tool calls requested: %t", chatID, len(response.Message.ToolCalls) > 0)
		if len(response.Message.ToolCalls) > 0 {
			logger.LLMInfo("Chat[%d]: Received unexpected tool calls after processing previous tool results.", chatID)
		}
	}

	// Add the final assistant's response to the session
	b.mutex.Lock()
	session = append(session, response.Message)
	b.sessions[chatID] = session
	b.mutex.Unlock()
	logger.TelegramDebug("Chat[%d]: Added final assistant response to session. History length: %d", chatID, len(session))

	// Prepare final message content for logging/sending
	finalContent := response.Message.Content
	logPreview := finalContent
	if len(logPreview) > 80 {
		logPreview = logPreview[:80] + "..."
	}
	logger.TelegramInfo("Chat[%d]: Sending final response to Telegram: \"%s\"", chatID, logPreview)

	// Send the response
	b.bot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   finalContent,
	})
}

// handlePhotoMessage processes a message with photos.
func (b *Bot) handlePhotoMessage(ctx context.Context, message *models.Message) {
	chatID := message.Chat.ID
	userID := message.From.ID

	logger.TelegramInfo("Chat[%d] User[%d]: Received photo message (Caption: %s)", userID, chatID, message.Caption)

	// Update user information
	b.updateUserInfo(message)

	// Get the current session for this chat
	session := b.getOrCreateSession(chatID)
	logger.TelegramDebug("Chat[%d]: Session retrieved/created. History length: %d", chatID, len(session))

	// Start typing indicator
	typingDone := make(chan struct{})
	go b.sendContinuousTypingAction(ctx, chatID, typingDone)
	defer close(typingDone)

	// Get the largest photo (highest resolution)
	photoSize := message.Photo[len(message.Photo)-1]

	// Get the file info
	file, err := b.bot.GetFile(ctx, &bot.GetFileParams{
		FileID: photoSize.FileID,
	})
	if err != nil {
		logger.TelegramError("Chat[%d]: Error getting file info for photo: %v", chatID, err)
		b.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   "Sorry, I couldn't get the info for your image.",
		})
		return
	}

	// Get the file URL
	fileURL := b.bot.FileDownloadLink(file)
	logger.TelegramDebug("Chat[%d]: Got file URL for photo: %s", chatID, fileURL)

	// Create a user message with the image
	userMessage := Message{
		Role:      "user",
		Content:   message.Caption, // Caption acts as text content
		ImageURLs: []string{fileURL},
	}

	// Add the user message to the session
	b.mutex.Lock()
	session = append(session, userMessage)
	b.sessions[chatID] = session
	b.mutex.Unlock()
	logger.TelegramDebug("Chat[%d]: Added user message (with image) to session. History length: %d", chatID, len(session))

	// Load tool specifications from JSON files for this specific user
	toolSpecs, err := b.filterToolSpecs(userID)
	if err != nil {
		log.Printf("Chat[%d] User[%d]: Error loading tool specs: %v", chatID, userID, err)
		b.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   "Sorry, I encountered an internal error preparing my tools.",
		})
		return
	}
	logger.TelegramDebug("Chat[%d] User[%d]: Loaded %d allowed tools.", chatID, userID, len(toolSpecs))

	// Get the user info from our storage
	b.mutex.RLock()
	userInfo := b.userInfo[chatID]
	b.mutex.RUnlock()

	// === First LLM Call (with image) ===
	logger.LLMDebug("Chat[%d]: Initiating LLM call (User Request with Image). History length: %d", chatID, len(session))
	var response *ChatResponse
	if userInfo != nil {
		response, err = b.llm.ChatCompletionWithUserInfo(ctx, session, toolSpecs, userInfo)
	} else {
		response, err = b.llm.ChatCompletion(ctx, session, toolSpecs)
	}

	if err != nil {
		logger.LLMError("Chat[%d] User[%d]: Error from LLM (1st call, with image): %v", chatID, userID, err)
		b.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   "Sorry, I encountered an error analyzing your image.",
		})
		return
	}
	logger.LLMDebug("Chat[%d]: Received LLM response (1st call, with image). Tool calls requested: %t", chatID, len(response.Message.ToolCalls) > 0)

	// Process tool calls if any
	if len(response.Message.ToolCalls) > 0 {
		logger.LLMInfo("Chat[%d]: LLM requested %d tool calls after image.", chatID, len(response.Message.ToolCalls))

		// Add the assistant message with tool calls to the session
		b.mutex.Lock()
		session = append(session, response.Message)
		b.sessions[chatID] = session
		b.mutex.Unlock()
		logger.TelegramDebug("Chat[%d]: Added assistant message (tool request) to session. History length: %d", chatID, len(session))

		// Execute each tool call
		toolResults := make([]Message, 0, len(response.Message.ToolCalls))
		for i, toolCall := range response.Message.ToolCalls {
			toolName := toolCall.Function.Name
			logger.ToolInfo("Chat[%d] User[%d]: Executing tool call %d/%d: %s", chatID, userID, i+1, len(response.Message.ToolCalls), toolName)

			// Automatically inject the user's telegram ID and name for store_person_memory if needed
			if toolCall.Function.Name == "store_person_memory" {
				var args map[string]interface{}
				if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
					logger.ToolError("Chat[%d]: Error parsing tool arguments for %s: %v", chatID, toolName, err)
				} else {
					// Ensure telegram_id exists
					if _, exists := args["telegram_id"]; !exists {
						args["telegram_id"] = fmt.Sprintf("%d", userID)
						logger.ToolDebug("Chat[%d]: Injected telegram_id=%d into %s arguments", chatID, userID, toolName)
					}

					// Inject person_name if not present
					if _, exists := args["person_name"]; !exists {
						b.mutex.RLock()
						callingUserInfo := b.userInfo[chatID] // Get info of the user making the call
						b.mutex.RUnlock()
						if callingUserInfo != nil {
							personName := callingUserInfo.FullName
							if personName == "" {
								personName = callingUserInfo.Username // Fallback to username
							}
							if personName != "" {
								args["person_name"] = personName
								logger.ToolDebug("Chat[%d]: Injected person_name='%s' into %s arguments", chatID, personName, toolName)
							} else {
								logger.ToolWarn("Chat[%d]: Could not find name for user %d to inject into %s", chatID, userID, toolName)
							}
						} else {
							logger.ToolWarn("Chat[%d]: User info not found for user %d to inject name into %s", chatID, userID, toolName)
						}
					}

					// Re-marshal arguments if modified
					newArgs, err := json.Marshal(args)
					if err != nil {
						logger.ToolError("Chat[%d]: Error encoding modified arguments for %s: %v", chatID, toolName, err)
					} else {
						toolCall.Function.Arguments = string(newArgs)
					}
				}
			}

			// Execute the tool
			toolResultContent, toolErr := b.toolRouter.ExecuteToolCall(ctx, userID, &toolCall)
			if toolErr != nil {
				logger.ToolError("Chat[%d] User[%d]: Error executing tool %s: %v", chatID, userID, toolName, toolErr)
				b.bot.SendMessage(ctx, &bot.SendMessageParams{
					ChatID: chatID,
					Text:   fmt.Sprintf("Sorry, I encountered an error while trying to use the '%s' tool.", toolName),
				})
				return
			}
			logger.ToolInfo("Chat[%d]: Tool call '%s' executed successfully.", chatID, toolName)

			// Create a tool response message
			toolResponse := Message{
				Role:       "tool",
				Content:    toolResultContent,
				ToolCallID: toolCall.ID,
			}
			toolResults = append(toolResults, toolResponse)

			// Add the tool response to the session immediately
			b.mutex.Lock()
			session = append(session, toolResponse)
			b.sessions[chatID] = session
			b.mutex.Unlock()
			logger.TelegramDebug("Chat[%d]: Added tool result (%s) to session. History length: %d", chatID, toolName, len(session))
		}

		// === Second LLM Call (after tools, image context) ===
		logger.LLMDebug("Chat[%d]: Initiating LLM call (Tool Results, Image Context). History length: %d", chatID, len(session))
		if userInfo != nil {
			response, err = b.llm.ChatCompletionWithUserInfo(ctx, session, toolSpecs, userInfo)
		} else {
			response, err = b.llm.ChatCompletion(ctx, session, toolSpecs)
		}

		if err != nil {
			logger.LLMError("Chat[%d] User[%d]: Error from LLM (2nd call, after tools, image): %v", chatID, userID, err)
			b.bot.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: chatID,
				Text:   "Sorry, I encountered an error processing the tool results for the image.",
			})
			return
		}
		logger.LLMDebug("Chat[%d]: Received LLM response (2nd call, final, image). Tool calls requested: %t", chatID, len(response.Message.ToolCalls) > 0)
		if len(response.Message.ToolCalls) > 0 {
			logger.LLMInfo("Chat[%d]: Received unexpected tool calls after processing previous tool results (image context).", chatID)
		}
	}

	// Add the assistant's response to the session
	b.mutex.Lock()
	session = append(session, response.Message)
	b.sessions[chatID] = session
	b.mutex.Unlock()
	logger.TelegramDebug("Chat[%d]: Added final assistant response to session. History length: %d", chatID, len(session))

	// Prepare final message content for logging/sending
	finalContent := response.Message.Content
	logPreview := finalContent
	if len(logPreview) > 0 && logPreview[0] == ' ' { // Handle potential leading space from LLM with images
		logPreview = logPreview[1:]
	}
	if len(logPreview) > 80 {
		logPreview = logPreview[:80] + "..."
	}
	logger.TelegramInfo("Chat[%d]: Sending final response (image context) to Telegram: \"%s\"", chatID, logPreview)

	// Send the response
	b.bot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   finalContent,
	})
}

// handleCallbackQuery processes a callback query (button click).
func (b *Bot) handleCallbackQuery(ctx context.Context, query *models.CallbackQuery) {
	var chatID int64
	// Safely get ChatID if the message is accessible
	// query.Message is the MaybeInaccessibleMessage struct.
	// We check if its internal Message field is non-nil.
	if query.Message.Message != nil {
		chatID = query.Message.Message.Chat.ID
	} else {
		// Handle inaccessible message case - log without ChatID or use alternative
		logger.TelegramWarn("User[%d]: Received callback query with inaccessible message. ChatID unavailable. Data: %s", query.From.ID, query.Data)
		// Acknowledge and potentially return if ChatID is essential for further processing
		b.bot.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: query.ID})
		return // Or handle differently if possible without ChatID
	}

	userID := query.From.ID
	logger.TelegramInfo("Chat[%d] User[%d]: Received callback query: %s", chatID, userID, query.Data)

	// Acknowledge the callback query
	b.bot.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: query.ID,
	})

	// Process the callback data
	data := query.Data

	// Example: If the callback data is "generate_image"
	if strings.HasPrefix(data, "generate_image:") {
		// Extract the parts from the callback data
		parts := strings.SplitN(data, ":", 2) // Only need prompt part from data now
		if len(parts) < 2 {
			logger.TelegramError("Chat[%d]: Invalid callback data format for generate_image: %s", chatID, data)
			return
		}
		prompt := parts[1]
		logger.TelegramDebug("Chat[%d]: Parsed image generation prompt from callback: %s", chatID, prompt)

		// Send typing action
		b.bot.SendChatAction(ctx, &bot.SendChatActionParams{
			ChatID: chatID,
			Action: "upload_photo",
		})

		// Generate the image (This involves an LLM call)
		logger.LLMDebug("Chat[%d]: Initiating LLM image generation.", chatID)
		imageURL, err := b.llm.GenerateImage(ctx, prompt, "1024x1024", "photorealistic")
		if err != nil {
			logger.LLMError("Chat[%d]: Error generating image via LLM: %v", chatID, err)
			b.bot.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: chatID,
				Text:   "Sorry, I couldn't generate the image.",
			})
			return
		}
		logger.LLMInfo("Chat[%d]: LLM generated image URL: %s", chatID, imageURL)

		// Download the image to send it via Telegram
		localPath, err := b.downloadImage(imageURL, fmt.Sprintf("gen_%d", time.Now().Unix()))
		if err != nil {
			logger.TelegramError("Chat[%d]: Error downloading generated image (%s): %v", chatID, imageURL, err)
			b.bot.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: chatID,
				Text:   "I generated an image but couldn't download it. Here's the URL: " + imageURL,
			})
			return
		}
		logger.TelegramDebug("Chat[%d]: Downloaded generated image to %s", chatID, localPath)

		// Send the image
		photo, err := os.Open(localPath)
		if err != nil {
			logger.TelegramError("Chat[%d]: Error opening downloaded image file (%s): %v", chatID, localPath, err)
			b.bot.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: chatID,
				Text:   "I generated an image but couldn't send it. Here's the URL: " + imageURL,
			})
			return
		}
		defer photo.Close()
		defer os.Remove(localPath) // Clean up the temp file

		logger.TelegramInfo("Chat[%d]: Sending generated photo to Telegram.", chatID)
		b.bot.SendPhoto(ctx, &bot.SendPhotoParams{
			ChatID:  chatID,
			Photo:   &models.InputFileUpload{Filename: "image.jpg", Data: photo},
			Caption: "Generated image for: " + prompt,
		})
	} else {
		logger.TelegramInfo("Chat[%d] User[%d]: Ignoring unhandled callback query data: %s", chatID, userID, data)
	}
}

// getOrCreateSession gets an existing session or creates a new one.
func (b *Bot) getOrCreateSession(chatID int64) []Message {
	b.mutex.RLock()
	session, exists := b.sessions[chatID]
	b.mutex.RUnlock()

	if !exists {
		logger.TelegramDebug("Chat[%d]: No existing session found, creating new one.", chatID)
		// Initialize a new session with a system message
		session = []Message{
			{
				Role:    "system",
				Content: b.getSystemPrompt(chatID),
			},
		}
		b.mutex.Lock()
		b.sessions[chatID] = session
		b.mutex.Unlock()
	} else {
		logger.TelegramDebug("Chat[%d]: Found existing session.", chatID)
	}

	return session
}

// downloadImage downloads an image from a URL and returns the local path.
func (b *Bot) downloadImage(url, fileID string) (string, error) {
	// Create the temporary directory if it doesn't exist
	tmpDir := "/tmp/images"
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create temp directory %s: %w", tmpDir, err)
	}

	// Generate a filename based on the fileID
	fileExt := ".jpg" // Default extension
	if strings.Contains(strings.ToLower(url), ".png") {
		fileExt = ".png"
	} else if strings.Contains(strings.ToLower(url), ".webp") {
		fileExt = ".webp"
	}

	filename := filepath.Join(tmpDir, fileID+fileExt)

	// Download the file
	// Use a client with timeout
	client := http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("http get failed for %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed with status %d for %s", resp.StatusCode, url)
	}

	// Create the file
	file, err := os.Create(filename)
	if err != nil {
		return "", fmt.Errorf("failed to create file %s: %w", filename, err)
	}
	defer file.Close()

	// Copy the file contents
	_, err = io.Copy(file, resp.Body)
	if err != nil {
		// Clean up partially written file on error
		os.Remove(filename)
		return "", fmt.Errorf("failed to save file %s: %w", filename, err)
	}

	return filename, nil
}

// loadToolSpecs loads tool specifications from JSON files.
func loadToolSpecs() ([]interface{}, error) {
	// Get the executable directory
	execDir, err := os.Executable()
	if err != nil {
		// Generic error as this is setup related
		return nil, fmt.Errorf("failed to get executable path: %w", err)
	}
	execDir = filepath.Dir(execDir)

	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		logger.Error("Could not get current working directory: %v", err)
		cwd = "."
	}

	logger.Debug("Executable directory: %s, CWD: %s", execDir, cwd)

	// Try multiple potential paths
	potentialPaths := []string{
		"/app/tools-spec",                          // Docker container standard location
		filepath.Join(execDir, "tools-spec"),       // Relative to executable
		filepath.Join(execDir, "..", "tools-spec"), // One level up from executable
		"tools-spec",                               // Relative to current working directory
		filepath.Join(cwd, "tools-spec"),           // Explicit current working directory
		filepath.Join("..", "tools-spec"),          // One level up from current working directory
	}

	logger.Debug("Searching for tool specs in: %v", potentialPaths)

	// Find first valid path with JSON files
	var files []string
	var toolsDir string
	found := false
	for _, path := range potentialPaths {
		matches, err := filepath.Glob(filepath.Join(path, "*.json"))
		if err == nil && len(matches) > 0 {
			// Check if the directory actually exists
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				files = matches
				toolsDir = path
				logger.Info("Found %d tool spec files in %s", len(files), toolsDir)
				found = true
				break
			}
		}
	}

	if !found {
		logger.Info("No tool spec files found in searched directories. Tool usage will be disabled.")
		return []interface{}{}, nil // Return empty slice, not an error
	}

	// Load each tool spec
	var toolSpecs []interface{}
	for _, file := range files {
		logger.Debug("Loading tool spec from: %s", file)
		data, err := os.ReadFile(file)
		if err != nil {
			logger.Error("Failed to read tool spec file %s: %v", file, err)
			// Decide: skip this tool or return error? Skipping for robustness.
			continue
		}

		// Unmarshal the JSON into a map
		var toolSpec interface{}
		if err := json.Unmarshal(data, &toolSpec); err != nil {
			logger.Error("Failed to parse tool spec file %s: %v", file, err)
			// Skip invalid spec
			continue
		}

		toolSpecs = append(toolSpecs, toolSpec)
		// logger.Debug("Loaded tool spec: %+v", toolSpec) // Very verbose
	}

	logger.Info("Successfully loaded %d tool specifications.", len(toolSpecs))
	return toolSpecs, nil
}

// filterToolSpecs filters tool specifications based on user permissions.
func (b *Bot) filterToolSpecs(userID int64) ([]interface{}, error) {
	logger.ToolDebug("User[%d]: Filtering tool specs...", userID)

	// Load all tool specs
	allToolSpecs, err := loadToolSpecs() // loadToolSpecs uses generic logger
	if err != nil {
		logger.ToolError("User[%d]: Error loading all tool specs: %v", userID, err)
		return nil, err
	}

	// If there's no policy service, return all tools
	if b.policyService == nil {
		logger.ToolInfo("User[%d]: No policy service configured. Allowing all %d tools.", userID, len(allToolSpecs))
		return allToolSpecs, nil
	}

	// Filter tool specs based on user permissions
	allowedTools := b.policyService.GetAllowedTools(userID)
	allowedToolsMap := make(map[string]bool)
	for _, tool := range allowedTools {
		allowedToolsMap[tool] = true
	}

	logger.ToolDebug("User[%d]: Found %d allowed tools: %v", userID, len(allowedToolsMap), allowedTools)

	// Filter tool specs
	var filteredToolSpecs []interface{}
	var skippedTools []string
	for _, spec := range allToolSpecs {
		// Extract tool name from the spec
		specMap, ok1 := spec.(map[string]interface{})
		function, ok2 := specMap["function"].(map[string]interface{})
		name, ok3 := function["name"].(string)

		if !ok1 || !ok2 || !ok3 {
			logger.ToolError("User[%d]: Malformed tool spec encountered during filtering: %+v", userID, spec)
			continue
		}

		// Check if the tool is allowed
		if allowedToolsMap[name] {
			filteredToolSpecs = append(filteredToolSpecs, spec)
		} else {
			skippedTools = append(skippedTools, name)
		}
	}

	logger.ToolInfo("User[%d]: Final toolset: %d allowed, %d skipped. Skipped: %v", userID, len(filteredToolSpecs), len(skippedTools), skippedTools)
	return filteredToolSpecs, nil
}
