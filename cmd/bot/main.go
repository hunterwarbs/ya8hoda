package main

import (
	"context"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/hunterwarburton/ya8hoda/internal/auth"
	"github.com/hunterwarburton/ya8hoda/internal/core"
	embedder "github.com/hunterwarburton/ya8hoda/internal/embed"
	"github.com/hunterwarburton/ya8hoda/internal/llm"
	"github.com/hunterwarburton/ya8hoda/internal/logger"
	"github.com/hunterwarburton/ya8hoda/internal/rag"
	"github.com/hunterwarburton/ya8hoda/internal/telegram"
	"github.com/hunterwarburton/ya8hoda/internal/tools"
	"github.com/joho/godotenv"
)

//go:embed character.json
var embeddedFS embed.FS

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
	MilvusUser       string
	MilvusPassword   string
	MilvusCollection string
	MilvusUseSSL     bool
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
		MilvusHost:       "127.0.0.1",
		MilvusPort:       getEnvWithDefault("MILVUS_PORT", "19530"),
		LogLevel:         getEnvWithDefault("LOG_LEVEL", "info"),
		AdminUserIDs:     os.Getenv("ADMIN_USER_IDS"),
		AllowedUserIDs:   os.Getenv("ALLOWED_USER_IDS"),
		EmbeddingDim:     embeddingDim,
		CharacterFile:    getEnvWithDefault("CHARACTER_FILE", "cmd/bot/character.json"),
		MilvusUser:       getenv("MILVUS_USER", ""),
		MilvusPassword:   getenv("MILVUS_PASSWORD", ""),
		MilvusCollection: getenv("MILVUS_COLLECTION_NAME", "a_collection"),
		MilvusUseSSL:     getenvBool("MILVUS_USE_SSL", false),
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
	// First, try to load from the embedded data
	if filePath == "cmd/bot/character.json" || filePath == "" {
		logger.Info("Using embedded character configuration")
		data, err := embeddedFS.ReadFile("character.json")
		if err != nil {
			return nil, err
		}

		var character llm.Character
		if err := json.Unmarshal(data, &character); err != nil {
			return nil, err
		}
		return &character, nil
	}

	// If a different file is specified, try to load it from disk
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
	// Define command line flags
	debug := flag.Bool("debug", false, "Enable debug logging")
	characterFile := flag.String("character", "", "Path to character.json file")
	freshStartFlag := flag.Bool("freshStart", false, "Drop all existing collections before loading data")

	// Parse command line flags initially
	flag.Parse()

	// Also check environment variable for fresh start (allows override via env)
	freshStartEnvStr := os.Getenv("FRESH_START")
	freshStartEnv, _ := strconv.ParseBool(freshStartEnvStr)

	// Determine final freshStart value (environment variable takes precedence if set)
	freshStart := *freshStartFlag || freshStartEnv

	// Initialize logger
	logger.Init(*debug)
	logger.Info("Starting bot... Debug=%v, FreshStartFlag=%v, FreshStartEnv=%v, FinalFreshStart=%v", *debug, *freshStartFlag, freshStartEnv, freshStart)

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

	// --- Initialize Embedding Service FIRST ---
	logger.Info("Initializing Embedding Service...")
	// Configure the BGE embedder
	bgeConfig := embedder.BGEEmbedderConfig{
		ModelName:  "BAAI/bge-m3", // Or from config if needed
		TimeoutSec: 30,            // Or from config if needed
		ApiURL:     "",            // Will be set below
	}

	// Use EMBEDDING_API_URL directly from environment
	bgeConfig.ApiURL = getenvOrPanic("EMBEDDING_API_URL")

	logger.Info("Using BGE Embedding API URL: ", bgeConfig.ApiURL)

	// Create a temporary config provider for BGEEmbedder initialization
	dummyMilvusConfig := &embedder.MilvusConfigProvider{
		Dim:       config.EmbeddingDim,
		SparseDim: 250002, // Sparse dimension for BGE-M3
		UseSparse: true,   // We are using sparse embeddings
	}
	// Initialize the core embedder
	bgeEmbedder := embedder.NewBGEEmbedder(dummyMilvusConfig, bgeConfig)
	// Initialize the EmbedService adapter using the core interface type
	var embedService core.EmbedService = embedder.NewBGEAdapter(bgeEmbedder) // Use core.EmbedService
	logger.Info("Embedding Service initialized.")
	// --- Embedding Service Initialized ---

	// --- Initialize RAG Service SECOND (passing EmbedService) ---
	logger.Info("Initializing RAG Service...")
	// Use MILVUS_ADDRESS directly from environment
	milvusAddr := getenvOrPanic("MILVUS_ADDRESS")

	logger.Info("Connecting to Milvus at: ", milvusAddr)
	milvusClient, err := rag.NewMilvusClient(ctx, milvusAddr, config.EmbeddingDim, freshStart, embedService) // Pass embedService
	if err != nil {
		logger.Error("Failed to initialize Milvus client: %v", err)
		os.Exit(1)
	}
	ragService := milvusClient // Assign the concrete type that implements the interface
	logger.Info("RAG Service initialized.")
	// --- RAG Service Initialized ---

	// Initialize OpenRouter LLM service with character configuration
	llmService := llm.NewOpenRouterService(config.OpenRouterAPIKey, config.OpenRouterModel)

	// Set character configuration in the OpenRouterService
	// The service should already implement the CharacterAware interface
	if err := llmService.SetCharacter(character); err != nil {
		logger.Error("Failed to set character configuration: %v", err)
	} else {
		logger.Info("Character '%s' set for the LLM service", character.Name)
	}

	// Ensure character facts are loaded if needed (outside the removed block)
	if milvusClient != nil && embedService != nil {
		adapter, ok := embedService.(*embedder.BGEAdapter) // Still need the concrete BGEAdapter here
		if ok {
			logger.Info("Ensuring character facts are loaded/up-to-date...")
			// Add retry logic with exponential backoff for loading facts
			maxRetries := 5
			initialDelay := 3 * time.Second
			var lastErr error
			for attempt := 0; attempt < maxRetries; attempt++ {
				if attempt > 0 {
					retryDelay := initialDelay * time.Duration(1<<uint(attempt-1))
					logger.Info("Retrying character facts loading in %v... (attempt %d/%d)", retryDelay, attempt+1, maxRetries)
					time.Sleep(retryDelay)
				}
				// Pass milvusClient (which implements core.RAGService) directly
				if err := rag.EnsureCharacterFactsWithOptions(ctx, milvusClient, adapter, character, false); err != nil { // Assuming 'false' here means don't *re-ensure* collections, just load facts
					logger.Error("Attempt %d/%d: Failed to load character facts into Milvus: %v",
						attempt+1, maxRetries, err)
					lastErr = err
					continue
				} else {
					logger.Info("Character facts successfully loaded/ensured into Milvus")
					lastErr = nil
					break
				}
			}
			if lastErr != nil {
				logger.Error("All attempts to load character facts into Milvus failed: %v", lastErr)
			}
		} else {
			logger.Info("Could not convert embed service to BGEAdapter, skipping loading character facts into Milvus")
		}
	} else if milvusClient == nil || embedService == nil {
		logger.Info("RAG service or embed service not available, skipping loading character facts into Milvus")
	}

	// Initialize tool router (now uses the initialized ragService and embedService)
	toolRouter := tools.NewToolRouter(policyService, ragService, embedService) // Pass core interfaces

	// Initialize Telegram bot
	bot, err := telegram.NewBot(config.TelegramToken, llmService, embedService, toolRouter, policyService) // Pass core.EmbedService
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

// Helper function to get environment variable or use default
func getenv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}

// Helper function to get environment variable or panic if not set
func getenvOrPanic(key string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	panic(fmt.Sprintf("Environment variable %s not set", key))
}

// Helper function to get boolean environment variable
func getenvBool(key string, fallback bool) bool {
	if value, exists := os.LookupEnv(key); exists {
		boolVal, err := strconv.ParseBool(strings.ToLower(value))
		if err == nil {
			return boolVal
		}
		logger.Warn("Invalid boolean value for env var", "key", key, "value", value, "error", err)
	}
	return fallback
}
