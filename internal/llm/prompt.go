package llm

import (
	"fmt"
	"strings"

	"github.com/hunterwarburton/ya8hoda/internal/telegram"
)

// PromptGenerator handles the generation of prompts for LLM interactions.
type PromptGenerator struct {
	character *Character
}

// NewPromptGenerator creates a new prompt generator with the specified character.
func NewPromptGenerator(character *Character) *PromptGenerator {
	return &PromptGenerator{
		character: character,
	}
}

// GenerateSystemPrompt creates a system prompt based on the character configuration.
func (pg *PromptGenerator) GenerateSystemPrompt() string {
	return pg.GenerateSystemPromptWithUserInfo(nil)
}

// GenerateSystemPromptWithUserInfo creates a system prompt that includes user information.
func (pg *PromptGenerator) GenerateSystemPromptWithUserInfo(userInfo *telegram.UserInfo) string {
	if pg.character == nil {
		if userInfo != nil {
			return fmt.Sprintf("You are a helpful assistant talking to %s (ID: %d).",
				userInfo.FullName, userInfo.ID)
		}
		return "You are a helpful assistant."
	}

	var builder strings.Builder
	// Add pre-prompt for consistent behavior
	builder.WriteString(pg.character.PrePrompt)

	// Add character name and basic identity
	builder.WriteString(fmt.Sprintf("You are %s. ", pg.character.Name))

	// Add user information if available
	if userInfo != nil {
		builder.WriteString(fmt.Sprintf("You are currently talking to %s (ID: %d). ",
			userInfo.FullName, userInfo.ID))
	}

	builder.WriteString("The following is your background to pull from when relevant:")
	builder.WriteString("\n\n")

	// Add communication style
	if len(pg.character.Style.Chat) > 0 {
		builder.WriteString("Your communication style: ")
		builder.WriteString(strings.Join(pg.character.Style.Chat, ", "))
		builder.WriteString("\n\n")
	}

	// Add example topics
	if len(pg.character.Topics) > 0 {
		builder.WriteString("Topics you're knowledgeable about: ")
		builder.WriteString(strings.Join(pg.character.Topics, ", "))
		builder.WriteString("\n\n")
	}

	// Add personality traits
	if len(pg.character.Adjectives) > 0 {
		builder.WriteString("Your personality traits: ")
		builder.WriteString(strings.Join(pg.character.Adjectives, ", "))
		builder.WriteString("\n\n")
	}

	// Add message examples if available
	if len(pg.character.MessageExamples) > 0 {
		builder.WriteString("Here are examples of how you respond to various questions:\n\n")

		for _, conversation := range pg.character.MessageExamples {
			if len(conversation) >= 2 {
				userMsg := conversation[0]
				botMsg := conversation[1]

				builder.WriteString(fmt.Sprintf("User: %s\n", userMsg.Content.Text))
				builder.WriteString(fmt.Sprintf("You: %s\n\n", botMsg.Content.Text))
			}
		}
	}

	builder.WriteString("Respond to the user as this character, maintaining consistency with your background and personality at all times.")

	return builder.String()
}

// EnhanceImagePrompt enhances an image generation prompt with character personality.
func (pg *PromptGenerator) EnhanceImagePrompt(prompt string) string {
	if pg.character == nil {
		return prompt
	}

	// Get a few adjectives to describe the character
	adjCount := 3
	if len(pg.character.Adjectives) < 3 {
		adjCount = len(pg.character.Adjectives)
	}

	if adjCount == 0 {
		return prompt
	}

	enhancedPrompt := fmt.Sprintf("Create an image as if you were %s, who is %s. %s",
		pg.character.Name,
		strings.Join(pg.character.Adjectives[:adjCount], ", "),
		prompt)

	return enhancedPrompt
}
