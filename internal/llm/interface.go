package llm

import "context"

// Character represents the character configuration loaded from JSON.
type Character struct {
	Name            string             `json:"name"`
	Bio             []string           `json:"bio"`
	Lore            []string           `json:"lore"`
	Knowledge       []string           `json:"knowledge"`
	MessageExamples [][]MessageExample `json:"messageExamples"`
	PostExamples    []string           `json:"postExamples"`
	Topics          []string           `json:"topics"`
	Style           Style              `json:"style"`
	Adjectives      []string           `json:"adjectives"`
}

// MessageExample represents a message in the message examples.
type MessageExample struct {
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

// LLMService defines the common interface for LLM services.
type LLMService interface {
	// ChatCompletion sends a chat completion request to the LLM service.
	ChatCompletion(ctx context.Context, messages []interface{}, toolSpecs []interface{}) (interface{}, error)

	// GenerateImage sends an image generation request to the LLM service.
	GenerateImage(ctx context.Context, prompt, size, style string) (string, error)
}

// CharacterAware defines an interface for LLM services that support character configuration.
type CharacterAware interface {
	// SetCharacter configures the service with a character personality.
	SetCharacter(character *Character) error

	// GetCharacter returns the current character configuration.
	GetCharacter() *Character
}
