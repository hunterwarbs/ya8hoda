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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/hunterwarburton/ya8hoda/internal/logger"
)

// LLMService defines the interface for interacting with an LLM.
type LLMService interface {
	ChatCompletion(ctx context.Context, messages []Message, toolSpecs []interface{}) (*ChatResponse, error)
	GenerateImage(ctx context.Context, prompt, size, style string) (string, error)
}

// EmbedService defines the interface for creating embeddings.
type EmbedService interface {
	EmbedQuery(ctx context.Context, text string) ([]float32, error)
}

// ToolRouter defines the interface for routing and executing tool calls.
type ToolRouter interface {
	ExecuteToolCall(ctx context.Context, userID int64, toolCall *ToolCall) (string, error)
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
	bot        *bot.Bot
	llm        LLMService
	embed      EmbedService
	toolRouter ToolRouter
	sessions   map[int64][]Message
	mutex      sync.RWMutex
	// Character configuration
	characterPrompt string
}

// NewBot creates a new Bot instance.
func NewBot(token string, llm LLMService, embed EmbedService, toolRouter ToolRouter) (*Bot, error) {
	telegramBot := &Bot{
		llm:        llm,
		embed:      embed,
		toolRouter: toolRouter,
		sessions:   make(map[int64][]Message),
		mutex:      sync.RWMutex{},
	}

	// Initialize the bot with our handler
	b, err := bot.New(token, bot.WithDefaultHandler(telegramBot.handleUpdate))
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Telegram bot: %w", err)
	}

	telegramBot.bot = b

	return telegramBot, nil
}

// SetCharacter sets the character prompt for the bot
func (b *Bot) SetCharacter(characterPrompt string) {
	b.mutex.Lock()
	b.characterPrompt = characterPrompt
	b.mutex.Unlock()
}

// getSystemPrompt returns the appropriate system prompt based on character settings
func (b *Bot) getSystemPrompt() string {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	if b.characterPrompt != "" {
		return b.characterPrompt
	}

	return "You are a helpful assistant with access to a vector database using the milvus.search tool. When the user asks a question, consider whether to search for relevant information before answering."
}

// Start begins the bot's update handling loop.
func (b *Bot) Start(ctx context.Context) {
	b.bot.Start(ctx)
}

// Stop stops the bot.
func (b *Bot) Stop(ctx context.Context) {
	// The go-telegram/bot library doesn't have an explicit Stop method
	// It will stop when the context is canceled
}

// handleUpdate processes an incoming update.
func (b *Bot) handleUpdate(ctx context.Context, tgbot *bot.Bot, update *models.Update) {
	// Handle commands
	if update.Message != nil && update.Message.Text != "" && strings.HasPrefix(update.Message.Text, "/") {
		b.handleCommand(ctx, update.Message)
		return
	}

	// Handle regular messages with text
	if update.Message != nil && update.Message.Text != "" {
		b.handleTextMessage(ctx, update.Message)
		return
	}

	// Handle photos/images
	if update.Message != nil && len(update.Message.Photo) > 0 {
		b.handlePhotoMessage(ctx, update.Message)
		return
	}

	// Handle callback queries (button clicks)
	if update.CallbackQuery != nil {
		b.handleCallbackQuery(ctx, update.CallbackQuery)
		return
	}
}

// handleCommand processes a command message.
func (b *Bot) handleCommand(ctx context.Context, message *models.Message) {
	command := strings.Split(message.Text, " ")[0]
	command = strings.TrimPrefix(command, "/")

	switch command {
	case "start":
		text := "👋 Hello! I'm your RAG-enabled Telegram bot. You can ask me questions, and I'll use my vector database to provide informative answers."
		text += "\n\nCommands:"
		text += "\n/help - Show this help message"
		text += "\n/reset - Clear your conversation history"

		b.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: message.Chat.ID,
			Text:   text,
		})

		// Initialize a fresh session
		b.mutex.Lock()
		b.sessions[message.Chat.ID] = []Message{
			{
				Role:    "system",
				Content: b.getSystemPrompt(),
			},
		}
		b.mutex.Unlock()

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
				Content: b.getSystemPrompt(),
			},
		}
		b.mutex.Unlock()

		b.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: message.Chat.ID,
			Text:   "✅ Your conversation history has been reset.",
		})

	default:
		b.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: message.Chat.ID,
			Text:   "Unknown command. Try /help to see available commands.",
		})
	}
}

// handleTextMessage processes a text message.
func (b *Bot) handleTextMessage(ctx context.Context, message *models.Message) {
	chatID := message.Chat.ID
	userID := message.From.ID

	// Get or initialize the session
	session := b.getOrCreateSession(chatID)

	// Add the user message to the session
	userMessage := Message{
		Role:    "user",
		Content: message.Text,
	}

	b.mutex.Lock()
	session = append(session, userMessage)
	b.sessions[chatID] = session
	b.mutex.Unlock()

	// Load tool specifications from JSON files
	toolSpecs, err := loadToolSpecs()
	if err != nil {
		log.Printf("Error loading tool specs: %v", err)
		b.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   "Sorry, I encountered an error loading my capabilities.",
		})
		return
	}

	// Send "typing" action
	b.bot.SendChatAction(ctx, &bot.SendChatActionParams{
		ChatID: chatID,
		Action: "typing",
	})

	// Get a response from the LLM
	response, err := b.llm.ChatCompletion(ctx, session, toolSpecs)
	if err != nil {
		log.Printf("Error getting chat completion: %v", err)
		b.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    chatID,
			Text:      "Sorry, I encountered an error while processing your message.",
			ParseMode: models.ParseModeMarkdown,
		})
		return
	}

	// Process tool calls if any
	if len(response.Message.ToolCalls) > 0 {
		// Handle tool calls and get the results
		for _, toolCall := range response.Message.ToolCalls {
			// Add the tool call to the session
			b.mutex.Lock()
			session = append(session, response.Message)
			b.sessions[chatID] = session
			b.mutex.Unlock()

			// Execute the tool call
			result, err := b.toolRouter.ExecuteToolCall(ctx, userID, &toolCall)
			if err != nil {
				log.Printf("Error executing tool call: %v", err)
				b.bot.SendMessage(ctx, &bot.SendMessageParams{
					ChatID: chatID,
					Text:   "Sorry, I encountered an error while using a tool",
				})
				return
			}

			// Add the tool response to the session
			toolResponse := Message{
				Role:       "tool",
				Content:    result,
				ToolCallID: toolCall.ID,
			}

			b.mutex.Lock()
			session = append(session, toolResponse)
			b.sessions[chatID] = session
			b.mutex.Unlock()
		}

		// Get a new response with the tool results
		response, err = b.llm.ChatCompletion(ctx, session, nil)
		if err != nil {
			log.Printf("Error getting follow-up chat completion: %v", err)
			b.bot.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: chatID,
				Text:   "Sorry, I encountered an error while processing the results.",
			})
			return
		}
	}

	// Add the assistant's response to the session
	b.mutex.Lock()
	session = append(session, response.Message)
	b.sessions[chatID] = session
	b.mutex.Unlock()

	// Debug output
	logger.Debug("Sending Telegram message: %s", response.Message.Content)

	// Send the response
	b.bot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   response.Message.Content,
		// ParseMode: models.ParseModeMarkdown, // This can cause issues with formatting
	})
}

// handlePhotoMessage processes a message containing a photo.
func (b *Bot) handlePhotoMessage(ctx context.Context, message *models.Message) {
	chatID := message.Chat.ID
	userID := message.From.ID

	// Send "typing" action
	b.bot.SendChatAction(ctx, &bot.SendChatActionParams{
		ChatID: chatID,
		Action: "typing",
	})

	// Get the file ID of the largest photo (last one in the array)
	photo := message.Photo[len(message.Photo)-1]
	fileID := photo.FileID

	// Get file info from Telegram
	fileObj, err := b.bot.GetFile(ctx, &bot.GetFileParams{
		FileID: fileID,
	})
	if err != nil {
		log.Printf("Error getting file info: %v", err)
		b.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   "Sorry, I couldn't process your image.",
		})
		return
	}

	// Download the image
	fileURL := b.bot.FileDownloadLink(fileObj)
	localPath, err := b.downloadImage(fileURL, fileID)
	if err != nil {
		log.Printf("Error downloading image: %v", err)
		b.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   "Sorry, I couldn't download your image.",
		})
		return
	}

	// Get or initialize the session
	session := b.getOrCreateSession(chatID)

	// Add the user message with image to the session
	userMessage := Message{
		Role:      "user",
		Content:   message.Caption,
		ImageURLs: []string{localPath},
	}

	b.mutex.Lock()
	session = append(session, userMessage)
	b.sessions[chatID] = session
	b.mutex.Unlock()

	// Load tool specifications from JSON files
	toolSpecs, err := loadToolSpecs()
	if err != nil {
		log.Printf("Error loading tool specs: %v", err)
		b.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   "Sorry, I encountered an error loading my capabilities.",
		})
		return
	}

	// Get a response from the LLM
	response, err := b.llm.ChatCompletion(ctx, session, toolSpecs)
	if err != nil {
		log.Printf("Error getting chat completion for image: %v", err)
		b.bot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   "Sorry, I encountered an error while processing your image.",
		})
		return
	}

	// Process tool calls if any (same as in handleTextMessage)
	if len(response.Message.ToolCalls) > 0 {
		// Handle tool calls and get the results
		for _, toolCall := range response.Message.ToolCalls {
			// Add the tool call to the session
			b.mutex.Lock()
			session = append(session, response.Message)
			b.sessions[chatID] = session
			b.mutex.Unlock()

			// Execute the tool call
			result, err := b.toolRouter.ExecuteToolCall(ctx, userID, &toolCall)
			if err != nil {
				log.Printf("Error executing tool call: %v", err)
				b.bot.SendMessage(ctx, &bot.SendMessageParams{
					ChatID: chatID,
					Text:   "Sorry, I encountered an error while using a tool",
				})
				return
			}

			// Add the tool response to the session
			toolResponse := Message{
				Role:       "tool",
				Content:    result,
				ToolCallID: toolCall.ID,
			}

			b.mutex.Lock()
			session = append(session, toolResponse)
			b.sessions[chatID] = session
			b.mutex.Unlock()
		}

		// Get a new response with the tool results
		response, err = b.llm.ChatCompletion(ctx, session, nil)
		if err != nil {
			log.Printf("Error getting follow-up chat completion: %v", err)
			b.bot.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: chatID,
				Text:   "Sorry, I encountered an error while processing the results.",
			})
			return
		}
	}

	// Add the assistant's response to the session
	b.mutex.Lock()
	session = append(session, response.Message)
	b.sessions[chatID] = session
	b.mutex.Unlock()

	// Debug output
	logger.Debug("Sending Telegram message: %s", response.Message.Content)

	// Send the response
	b.bot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   response.Message.Content,
		// ParseMode: models.ParseModeMarkdown, // This can cause issues with formatting
	})
}

// handleCallbackQuery processes a callback query (button click).
func (b *Bot) handleCallbackQuery(ctx context.Context, query *models.CallbackQuery) {
	// Acknowledge the callback query
	b.bot.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: query.ID,
	})

	// Process the callback data
	data := query.Data

	// Since the message might be inaccessible, we need to safely handle it
	// For demonstration purposes, we'll extract the chatID from the data
	// In a real application, you might need to store chat IDs associated with callback data

	// Example: If the callback data is "generate_image"
	if strings.HasPrefix(data, "generate_image:") {
		// Extract the parts from the callback data
		parts := strings.SplitN(data, ":", 3)
		if len(parts) < 3 {
			log.Printf("Invalid callback data format: %s", data)
			return
		}

		prompt := parts[1]
		chatID, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil {
			log.Printf("Invalid chat ID in callback data: %s", parts[2])
			return
		}

		// Send typing action
		b.bot.SendChatAction(ctx, &bot.SendChatActionParams{
			ChatID: chatID,
			Action: "upload_photo",
		})

		// Generate the image
		imageURL, err := b.llm.GenerateImage(ctx, prompt, "1024x1024", "photorealistic")
		if err != nil {
			log.Printf("Error generating image: %v", err)
			b.bot.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: chatID,
				Text:   "Sorry, I couldn't generate the image.",
			})
			return
		}

		// Download the image to send it via Telegram
		localPath, err := b.downloadImage(imageURL, fmt.Sprintf("gen_%d", time.Now().Unix()))
		if err != nil {
			log.Printf("Error downloading generated image: %v", err)
			b.bot.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: chatID,
				Text:   "I generated an image but couldn't download it. Here's the URL: " + imageURL,
			})
			return
		}

		// Send the image
		photo, err := os.Open(localPath)
		if err != nil {
			log.Printf("Error opening image file: %v", err)
			b.bot.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: chatID,
				Text:   "I generated an image but couldn't send it. Here's the URL: " + imageURL,
			})
			return
		}
		defer photo.Close()

		b.bot.SendPhoto(ctx, &bot.SendPhotoParams{
			ChatID:  chatID,
			Photo:   &models.InputFileUpload{Filename: "image.jpg", Data: photo},
			Caption: "Generated image for: " + prompt,
		})
	}
}

// getOrCreateSession gets an existing session or creates a new one.
func (b *Bot) getOrCreateSession(chatID int64) []Message {
	b.mutex.RLock()
	session, exists := b.sessions[chatID]
	b.mutex.RUnlock()

	if !exists {
		// Initialize a new session with a system message
		session = []Message{
			{
				Role:    "system",
				Content: b.getSystemPrompt(),
			},
		}
		b.mutex.Lock()
		b.sessions[chatID] = session
		b.mutex.Unlock()
	}

	return session
}

// downloadImage downloads an image from a URL and returns the local path.
func (b *Bot) downloadImage(url, fileID string) (string, error) {
	// Create the temporary directory if it doesn't exist
	tmpDir := "/tmp/images"
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}

	// Generate a filename based on the fileID
	fileExt := ".jpg" // Default extension
	if strings.Contains(url, ".png") {
		fileExt = ".png"
	} else if strings.Contains(url, ".webp") {
		fileExt = ".webp"
	}

	filename := filepath.Join(tmpDir, fileID+fileExt)

	// Download the file
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to download image: %w", err)
	}
	defer resp.Body.Close()

	// Create the file
	file, err := os.Create(filename)
	if err != nil {
		return "", fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	// Copy the file contents
	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to save file: %w", err)
	}

	return filename, nil
}

// loadToolSpecs loads tool specifications from JSON files.
func loadToolSpecs() ([]interface{}, error) {
	toolsDir := "tools-spec"

	// List of tool spec files
	files, err := filepath.Glob(filepath.Join(toolsDir, "*.json"))
	if err != nil {
		return nil, fmt.Errorf("failed to list tool spec files: %w", err)
	}

	if len(files) == 0 {
		return []interface{}{}, nil
	}

	// Load each tool spec
	var toolSpecs []interface{}
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("failed to read tool spec file %s: %w", file, err)
		}

		// Unmarshal the JSON into a map
		var toolSpec interface{}
		if err := json.Unmarshal(data, &toolSpec); err != nil {
			return nil, fmt.Errorf("failed to parse tool spec file %s: %w", file, err)
		}

		toolSpecs = append(toolSpecs, toolSpec)
	}

	return toolSpecs, nil
}
