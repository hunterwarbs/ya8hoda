package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/hunterwarburton/ya8hoda/internal/auth"
	"github.com/hunterwarburton/ya8hoda/internal/embed"
	"github.com/hunterwarburton/ya8hoda/internal/llm"
	"github.com/hunterwarburton/ya8hoda/internal/logger"
	"github.com/hunterwarburton/ya8hoda/internal/rag"
	"github.com/hunterwarburton/ya8hoda/internal/telegram"
	"github.com/hunterwarburton/ya8hoda/internal/tools"
	"github.com/joho/godotenv"
)

// Config represents the application configuration.
type Config struct {
	TelegramToken    string
	OpenRouterAPIKey string
	OpenRouterModel  string
	MilvusHost       string
	MilvusPort       string
	LogLevel         string
	AdminUserIDs     string
	AllowedUserIDs   string
	EmbeddingDim     int
	CharacterFile    string
}

// Character represents the character configuration loaded from JSON.
type Character struct {
	Name            string      `json:"name"`
	Bio             []string    `json:"bio"`
	Lore            []string    `json:"lore"`
	Knowledge       []string    `json:"knowledge"`
	MessageExamples [][]Message `json:"messageExamples"`
	PostExamples    []string    `json:"postExamples"`
	Topics          []string    `json:"topics"`
	Style           Style       `json:"style"`
	Adjectives      []string    `json:"adjectives"`
}

// Message represents a message in the message examples.
type Message struct {
	User    string  `json:"user"`
	Content Content `json:"content"`
}

// Content represents the content of a message.
type Content struct {
	Text string `json:"text"`
}

// Style represents the character's communication style.
type Style struct {
	All  []string `json:"all"`
	Chat []string `json:"chat"`
	Post []string `json:"post"`
}

// loadConfig loads configuration from environment variables.
func loadConfig() *Config {
	embeddingDim := 1024 // Default embedding dimension for BGE-M3

	return &Config{
		TelegramToken:    os.Getenv("TG_BOT_TOKEN"),
		OpenRouterAPIKey: os.Getenv("OPENROUTER_API_KEY"),
		OpenRouterModel:  getEnvWithDefault("OPENROUTER_MODEL", "meta-llama/llama-3-70b-instruct"),
		MilvusHost:       getEnvWithDefault("MILVUS_HOST", "milvus"),
		MilvusPort:       getEnvWithDefault("MILVUS_PORT", "19530"),
		LogLevel:         getEnvWithDefault("LOG_LEVEL", "info"),
		AdminUserIDs:     os.Getenv("ADMIN_USER_IDS"),
		AllowedUserIDs:   os.Getenv("ALLOWED_USER_IDS"),
		EmbeddingDim:     embeddingDim,
		CharacterFile:    getEnvWithDefault("CHARACTER_FILE", "cmd/bot/character.json"),
	}
}

// getEnvWithDefault gets an environment variable or returns a default value.
func getEnvWithDefault(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

// loadCharacter loads the character configuration from a JSON file.
func loadCharacter(filePath string) (*llm.Character, error) {
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var character llm.Character
	if err := json.Unmarshal(data, &character); err != nil {
		return nil, err
	}

	return &character, nil
}

func main() {
	// Parse command line flags
	debug := flag.Bool("debug", false, "Enable debug logging")
	characterFile := flag.String("character", "", "Path to character.json file")
	flag.Parse()

	// Initialize logger
	logger.Init(*debug)

	logger.Info("Starting bot...")

	// Load environment variables from .env file
	if err := godotenv.Load(); err != nil {
		logger.Info("Warning: No .env file found or error loading it")
	}

	// Load configuration
	config := loadConfig()

	// Override character file path from command line if provided
	if *characterFile != "" {
		config.CharacterFile = *characterFile
	}

	if logger.IsDebugEnabled() {
		logger.Debug("Configuration loaded: TelegramToken=%v, OpenRouterModel=%s, MilvusHost=%s, MilvusPort=%s, CharacterFile=%s",
			config.TelegramToken != "", config.OpenRouterModel, config.MilvusHost, config.MilvusPort, config.CharacterFile)
	}

	// Validate required settings
	if config.TelegramToken == "" {
		logger.Error("TG_BOT_TOKEN environment variable is required")
		os.Exit(1)
	}
	if config.OpenRouterAPIKey == "" {
		logger.Error("OPENROUTER_API_KEY environment variable is required")
		os.Exit(1)
	}

	// Load character configuration
	character, err := loadCharacter(config.CharacterFile)
	if err != nil {
		logger.Error("Failed to load character configuration: %v", err)
		os.Exit(1)
	}
	logger.Info("Character '%s' loaded successfully", character.Name)

	// Set up context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize services
	logger.Info("Initializing services...")

	// Initialize policy service
	policyService := auth.NewPolicyService(config.AdminUserIDs, config.AllowedUserIDs)

	// Initialize Milvus RAG service
	milvusAddr := config.MilvusHost + ":" + config.MilvusPort

	// Use the mock implementation instead of the real one
	ragService, err := rag.NewMockMilvusService(ctx, milvusAddr, config.EmbeddingDim)
	if err != nil {
		logger.Error("Failed to initialize Milvus service: %v", err)
		os.Exit(1)
	}

	// Initialize embedding service (integrated with Milvus BGE-M3)
	embedService := embed.NewBGEEmbedder(ragService)

	// Initialize OpenRouter LLM service with character configuration
	llmService := llm.NewOpenRouterService(config.OpenRouterAPIKey, config.OpenRouterModel)

	// Set character configuration in the OpenRouterService
	// The service should already implement the CharacterAware interface
	if err := llmService.SetCharacter(character); err != nil {
		logger.Error("Failed to set character configuration: %v", err)
	} else {
		logger.Info("Character '%s' set for the LLM service", character.Name)
	}

	// Initialize tool router
	toolRouter := tools.NewToolRouter(policyService, ragService, llmService)

	// Initialize Telegram bot
	bot, err := telegram.NewBot(config.TelegramToken, llmService, embedService, toolRouter)
	if err != nil {
		logger.Error("Failed to initialize Telegram bot: %v", err)
		os.Exit(1)
	}

	// Set the character prompt in the Telegram bot
	systemPrompt := buildSystemPromptFromCharacter(character)
	bot.SetCharacter(systemPrompt)
	logger.Info("Character prompt set for Telegram bot")

	// Start the bot
	logger.Info("Starting bot...")
	go bot.Start(ctx)

	// Set up graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Wait for shutdown signal
	<-quit
	logger.Info("Shutting down bot...")

	// Create a context with timeout for shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	// Perform cleanup
	bot.Stop(shutdownCtx)

	logger.Info("Bot has been shut down")
}

// buildSystemPromptFromCharacter creates a system prompt string from a character
func buildSystemPromptFromCharacter(character *llm.Character) string {
	var builder strings.Builder

	// Add character name and basic identity
	builder.WriteString(fmt.Sprintf("You are %s. ", character.Name))

	// Add bio information
	if len(character.Bio) > 0 {
		builder.WriteString("Here's your background: ")
		builder.WriteString(strings.Join(character.Bio, " "))
		builder.WriteString("\n\n")
	}

	// Add lore
	if len(character.Lore) > 0 {
		builder.WriteString("Additional background details: ")
		builder.WriteString(strings.Join(character.Lore, " "))
		builder.WriteString("\n\n")
	}

	// Add communication style
	if len(character.Style.Chat) > 0 {
		builder.WriteString("Your communication style: ")
		builder.WriteString(strings.Join(character.Style.Chat, ", "))
		builder.WriteString("\n\n")
	}

	// Add example topics
	if len(character.Topics) > 0 {
		builder.WriteString("Topics you're knowledgeable about: ")
		builder.WriteString(strings.Join(character.Topics, ", "))
		builder.WriteString("\n\n")
	}

	// Personality traits from adjectives
	if len(character.Adjectives) > 0 {
		builder.WriteString("Your personality traits: ")
		builder.WriteString(strings.Join(character.Adjectives, ", "))
		builder.WriteString("\n\n")
	}

	builder.WriteString("Respond to the user as this character, maintaining consistency with your background and personality at all times.")

	return builder.String()
}
