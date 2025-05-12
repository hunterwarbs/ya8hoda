package elevenlabs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/hunterwarburton/ya8hoda/internal/logger"
)

// Client interacts with the ElevenLabs API.
type Client struct {
	apiKey     string
	voiceID    string // Default voice ID to use
	httpClient *http.Client
}

// NewClient creates a new ElevenLabs client.
// Requires an API key and a default voice ID.
func NewClient(apiKey, voiceID string) *Client {
	if apiKey == "" || voiceID == "" {
		logger.Warn("ElevenLabs API key or Voice ID is missing. TTS functionality will be disabled.")
		// Return a client that will error out if used
		return &Client{httpClient: &http.Client{Timeout: 1 * time.Nanosecond}} // Effectively disabled client
	}
	return &Client{
		apiKey:  apiKey,
		voiceID: voiceID,
		httpClient: &http.Client{
			Timeout: 60 * time.Second, // Timeout for API calls
		},
	}
}

// TextToSpeech generates audio from text using the configured voice.
// Returns the audio data as []byte (suitable for sending as a voice note) or an error.
func (c *Client) TextToSpeech(ctx context.Context, text string) ([]byte, error) {
	if c.apiKey == "" || c.voiceID == "" {
		return nil, fmt.Errorf("ElevenLabs client not configured (missing API key or voice ID)")
	}

	if text == "" {
		return nil, fmt.Errorf("cannot convert empty text to speech")
	}

	apiURL := fmt.Sprintf("https://api.elevenlabs.io/v1/text-to-speech/%s", c.voiceID)

	requestBody := map[string]interface{}{
		"text": text,
		// Add model_id or voice_settings if needed
		// "model_id": "eleven_multilingual_v2",
		// "voice_settings": {
		//  "stability": 0.5,
		//  "similarity_boost": 0.75
		// }
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		logger.Error("Failed to marshal ElevenLabs request body: %v", err)
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		logger.Error("Failed to create ElevenLabs request: %v", err)
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("xi-api-key", c.apiKey)
	req.Header.Set("Accept", "audio/mpeg") // Request MP3 audio

	logger.Info("Sending request to ElevenLabs TTS API for voice %s...", c.voiceID)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		logger.Error("Failed to send request to ElevenLabs: %v", err)
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		errMsg := fmt.Sprintf("ElevenLabs API error (status %d): %s", resp.StatusCode, string(bodyBytes))
		logger.Error(errMsg)
		return nil, fmt.Errorf(errMsg)
	}

	// Read the audio data
	audioData, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Error("Failed to read ElevenLabs audio response body: %v", err)
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	logger.Info("Successfully received %d bytes of audio data from ElevenLabs.", len(audioData))
	return audioData, nil
}

// --- Helper needed for bot.go ---
// Need to import "encoding/json" for the client code to compile.
