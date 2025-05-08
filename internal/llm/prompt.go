package llm

import (
	"fmt"
	"strings"
	"time"

	"github.com/hunterwarburton/ya8hoda/internal/telegram"
)

// PromptContext holds additional information to enrich the system prompt.
type PromptContext struct {
	PersonalFacts  []string            // Facts about the current user
	PeopleFacts    map[string][]string // Keyed by person name, list of facts per person
	CommunityFacts map[string][]string // Keyed by community name, list of facts per community
}

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
	return pg.GenerateSystemPromptWithUserInfoAndContext(nil, PromptContext{}) // Pass empty context
}

// GenerateSystemPromptWithUserInfoAndContext creates a system prompt that includes user information and contextual details.
func (pg *PromptGenerator) GenerateSystemPromptWithUserInfoAndContext(userInfo *telegram.UserInfo, promptCtx PromptContext) string {
	currentTime := time.Now().Format(time.RFC1123)

	if pg.character == nil {
		if userInfo != nil {
			return fmt.Sprintf("You are a helpful assistant talking to %s (ID: %d). The current time is %s.",
				userInfo.FullName, userInfo.ID, currentTime)
		}
		return fmt.Sprintf("You are a helpful assistant. The current time is %s.", currentTime)
	}

	var builder strings.Builder
	// Add pre-prompt for consistent behavior
	builder.WriteString(pg.character.PrePrompt + "\n\n")

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
	// Add relevant personal facts
	if len(promptCtx.PersonalFacts) > 0 {
		builder.WriteString("Relevant personal facts:\n")
		for _, fact := range promptCtx.PersonalFacts {
			builder.WriteString(fmt.Sprintf("   - %s\n", fact))
		}
		builder.WriteString("\n")
	}

	// Add relevant people and their facts
	if len(promptCtx.PeopleFacts) > 0 {
		builder.WriteString("Known relevant people (you can tag these people to introduce them to the user you're chatting with by prepending a listed person with the @ symbol):\n")
		for name, facts := range promptCtx.PeopleFacts {
			builder.WriteString(fmt.Sprintf("    - %s\n", name))
			for _, f := range facts {
				builder.WriteString(fmt.Sprintf("         - %s\n", f))
			}
		}
		builder.WriteString("\n")
	}

	// Add relevant communities and their facts
	if len(promptCtx.CommunityFacts) > 0 {
		builder.WriteString("Known relevant communities:\n")
		for name, facts := range promptCtx.CommunityFacts {
			builder.WriteString(fmt.Sprintf("      - %s\n", name))
			for _, f := range facts {
				builder.WriteString(fmt.Sprintf("          - %s\n", f))
			}
		}
		builder.WriteString("\n")
	}
	// Add user information if available
	if userInfo != nil {
		builder.WriteString(fmt.Sprintf("You are currently talking to %s (username: %s) (ID: %d) on Telegram. \n\n ",
			userInfo.FullName, userInfo.Username, userInfo.ID))
	}

	// Provide the current time in all relevant time zones
	locations := []struct {
		Name string
		TZ   string
	}{
		{"Bangkok", "Asia/Bangkok"},
		{"Berlin", "Europe/Berlin"},
		{"Kathmandu", "Asia/Kathmandu"},
	}

	var timeStrings []string
	for _, loc := range locations {
		if tz, err := time.LoadLocation(loc.TZ); err == nil {
			timeStrings = append(timeStrings, fmt.Sprintf("%s: %s", loc.Name, time.Now().In(tz).Format("2006-01-02 15:04")))
		}
	}
	if len(timeStrings) > 0 {
		builder.WriteString("Current local times — ")
		builder.WriteString(strings.Join(timeStrings, ", "))
		builder.WriteString(".\n\n")
	} else {
		// Fallback to UTC if time zone conversions fail
		builder.WriteString(fmt.Sprintf("The current time (UTC) is %s.\n\n", currentTime))
	}

	builder.WriteString("Respond to the user as this character, maintaining consistency with your background and personality at all times.")

	fmt.Println(builder.String())
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
