package telegram

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/hunterwarburton/ya8hoda/internal/imageutils"
	"github.com/hunterwarburton/ya8hoda/internal/logger"
)

const maxTotalMediaSize = 45 * 1024 * 1024 // 45 MB as a safety margin
const jpegReEncodeQuality = 75             // Quality for JPEG re-encoding
const estimatedSpeakingWPM = 150.0         // Words per minute for TTS voice duration estimation

// SendURLsAsMediaGroup is a method on the Bot struct to send images from URLs as a media group.
// It downloads images first, attempts re-encoding if needed, and uses the attach:// scheme.
func (b *Bot) SendURLsAsMediaGroup(ctx context.Context, chatID int64, urls []string, caption string) (string, error) {
	if len(urls) == 0 {
		return "", fmt.Errorf("no URLs provided")
	}
	if len(urls) > 10 {
		logger.TelegramWarn("Chat[%d]: Attempted to send %d URLs, but max is 10. Truncating.", chatID, len(urls))
		urls = urls[:10]
	}

	mediaItems := make([]models.InputMedia, 0, len(urls))
	openedFiles := make([]*os.File, 0, len(urls)) // Slice to hold opened files for cleanup (originals AND re-encoded)
	var tempFilePaths []string                    // To keep track of ALL temp file paths for cleanup (originals AND re-encoded)
	var currentTotalSize int64 = 0                // Track cumulative size

	// Ensure the base temporary directory exists before downloading
	if err := imageutils.EnsureTmpDirExists(); err != nil {
		// Log the error but proceed; downloading might still work if subdirs exist
		logger.TelegramError("Chat[%d]: Failed initial check/create of tmp dir: %v", chatID, err)
	}

	defer func() {
		for _, f := range openedFiles { // Close all opened files from the slice
			if f != nil {
				f.Close()
			}
		}
		for _, path := range tempFilePaths { // Remove all temporary files (originals and re-encoded)
			os.Remove(path)
		}
	}()

	for i, urlString := range urls {
		fileBaseNameForDownload := fmt.Sprintf("user_%d_chat_%d_img_%d_%d", b.userInfo[chatID].ID, chatID, time.Now().UnixNano(), i)
		localPath, err := b.downloadImage(urlString, fileBaseNameForDownload)
		if err != nil {
			logger.TelegramError("Chat[%d]: Failed to download image from URL %s: %v. Skipping.", chatID, urlString, err)
			continue
		}
		tempFilePaths = append(tempFilePaths, localPath) // Add original path for cleanup

		// Check file size before adding
		fileInfo, err := os.Stat(localPath)
		if err != nil {
			logger.TelegramError("Chat[%d]: Failed to get file info for %s: %v. Skipping.", chatID, localPath, err)
			// No need to manage removal here, defer handles it
			continue
		}
		originalSize := fileInfo.Size()
		usePath := localPath
		useSize := originalSize
		needsReEncoding := false

		if currentTotalSize+originalSize > maxTotalMediaSize {
			// If adding the original size exceeds the limit, try re-encoding
			logger.TelegramWarn("Chat[%d]: Image %s (%.2f MB) would exceed total size limit (%dMB). Attempting re-encoding.",
				chatID, filepath.Base(localPath), float64(originalSize)/(1024*1024), maxTotalMediaSize/(1024*1024))
			needsReEncoding = true
		}

		if needsReEncoding {
			reencodedPath, reencodedSize, reencodeErr := imageutils.ReEncodeToJPEG(localPath, jpegReEncodeQuality)
			if reencodeErr != nil {
				// Log the specific re-encoding error, but don't stop the whole process unless it *still* exceeds the size limit
				logger.TelegramError("Chat[%d]: Failed to re-encode %s: %v. Will check if original fits.", chatID, filepath.Base(localPath), reencodeErr)
				// Stick with original size check below
			} else {
				// Re-encoding succeeded, check if the NEW size fits
				if currentTotalSize+reencodedSize <= maxTotalMediaSize {
					logger.TelegramInfo("Chat[%d]: Re-encoded %s to %.2f MB. It now fits.", chatID, filepath.Base(reencodedPath), float64(reencodedSize)/(1024*1024))
					usePath = reencodedPath
					useSize = reencodedSize
					tempFilePaths = append(tempFilePaths, reencodedPath) // Add re-encoded path for cleanup
				} else {
					// Re-encoded, but still too large when added to the total
					logger.TelegramWarn("Chat[%d]: Re-encoded %s to %.2f MB, but total size would still exceed limit. Stopping here.",
						chatID, filepath.Base(reencodedPath), float64(reencodedSize)/(1024*1024))
					os.Remove(reencodedPath)                             // Clean up the useless re-encoded file now
					tempFilePaths = tempFilePaths[:len(tempFilePaths)-1] // Remove re-encoded path from cleanup list if added
					break                                                // Stop processing more URLs
				}
			}
		}

		// Final check: Can we add the selected file (original or re-encoded)?
		if currentTotalSize+useSize > maxTotalMediaSize {
			logger.TelegramWarn("Chat[%d]: Reached total size limit (%dMB) even after checking/trying re-encoding with %s. Sending %d images instead of %d.",
				chatID, maxTotalMediaSize/(1024*1024), filepath.Base(usePath), len(mediaItems), len(urls))
			break // Stop processing more URLs for this media group
		}

		// Open the file we decided to use (original or re-encoded)
		file, err := os.Open(usePath)
		if err != nil {
			logger.TelegramError("Chat[%d]: Failed to open final image file %s: %v. Skipping.", chatID, usePath, err)
			// Don't modify tempFilePaths here, defer handles cleanup
			continue
		}
		openedFiles = append(openedFiles, file) // Add to slice for deferred closing
		currentTotalSize += useSize

		// Use the base name of the file we are actually attaching as the key.
		attachmentKey := filepath.Base(usePath)

		itemCaption := ""
		if len(mediaItems) == 0 && caption != "" {
			itemCaption = caption
		}

		// Media field must be a string using attach:// scheme.
		mediaItems = append(mediaItems, &models.InputMediaPhoto{
			Media:           fmt.Sprintf("attach://%s", attachmentKey),
			Caption:         itemCaption,
			MediaAttachment: file, // Assign the file reader here
		})
	}

	if len(mediaItems) == 0 {
		logger.TelegramError("Chat[%d]: %s", chatID, "Failed to download or prepare any images.")
		return "Failed to download or prepare any images.", fmt.Errorf("failed to download or prepare any images")
	}

	b.bot.SendChatAction(ctx, &bot.SendChatActionParams{
		ChatID: chatID,
		Action: models.ChatActionUploadPhoto,
	})

	params := &bot.SendMediaGroupParams{
		ChatID: chatID,
		Media:  mediaItems,
	}

	_, err := b.bot.SendMediaGroup(ctx, params)

	if err != nil {
		logger.TelegramError("Chat[%d]: Failed to send media group: %v", chatID, err)
		return fmt.Sprintf("Failed to send images: %v", err), err
	}

	resultMsg := fmt.Sprintf("Successfully sent %d image(s).", len(mediaItems))
	if caption != "" && len(mediaItems) > 0 {
		resultMsg = fmt.Sprintf("Successfully sent %d image(s) with caption starting with '%s'.", len(mediaItems), firstWords(caption, 5))
	}
	logger.TelegramInfo("Chat[%d]: Successfully sent media group with %d images.", chatID, len(mediaItems))
	return resultMsg, nil
}

// sendVoiceNoteTool handles the generation and sending of a voice note synchronously.
func (b *Bot) sendVoiceNoteTool(ctx context.Context, chatID int64, messageText string) (string, error) {
	if b.elevenlabs == nil {
		logger.ToolError("Chat[%d]: ElevenLabs service is not configured, cannot perform send_voice_note.", chatID)
		return "Error: Text-to-speech service is not available.", fmt.Errorf("elevenlabs service not configured")
	}

	// --- Calculate and apply delay ---
	startTime := time.Now()

	// Generate audio data
	// Use a timeout context appropriate for TTS generation
	ttsCtx, cancel := context.WithTimeout(ctx, 60*time.Second) // 60-second timeout for TTS
	defer cancel()
	audioData, ttsErr := b.elevenlabs.TextToSpeech(ttsCtx, messageText)

	elevenLabsQueryDuration := time.Since(startTime)

	if ttsErr != nil {
		logger.ToolError("Chat[%d]: Error generating voice note via ElevenLabs: %v", chatID, ttsErr)
		// Return a user-friendly error message, but also the internal error
		return fmt.Sprintf("Error generating voice note: %v", ttsErr), ttsErr
	}

	wordCount := len(strings.Fields(messageText))
	var estimatedAudioDuration time.Duration
	if wordCount > 0 {
		estimatedAudioDurationSeconds := (float64(wordCount) / estimatedSpeakingWPM) * 100.0
		estimatedAudioDuration = time.Duration(estimatedAudioDurationSeconds * float64(time.Second))
	}

	delayNeeded := estimatedAudioDuration - elevenLabsQueryDuration

	if delayNeeded > 0 {
		logger.ToolDebug("Chat[%d]: TTS query took %v. Estimated audio duration %v. Waiting for an additional %v before sending voice note.", chatID, elevenLabsQueryDuration, estimatedAudioDuration, delayNeeded)
		select {
		case <-time.After(delayNeeded):
			// Delay elapsed
		case <-ctx.Done():
			logger.ToolWarn("Chat[%d]: Context cancelled during voice note pre-send delay. Error: %v", chatID, ctx.Err())
			return "Voice note generation cancelled", ctx.Err()
		}
	} else {
		logger.ToolDebug("Chat[%d]: TTS query took %v. Estimated audio duration %v. No additional delay needed before sending voice note.", chatID, elevenLabsQueryDuration, estimatedAudioDuration)
	}
	// --- End delay calculation and application ---

	logger.ToolInfo("Chat[%d]: Generated voice note (%d bytes). Sending...", chatID, len(audioData))

	// Send the voice note
	// Use a separate timeout context for sending the voice message
	sendCtx, sendCancel := context.WithTimeout(ctx, 30*time.Second) // 30-second timeout for upload
	defer sendCancel()
	_, sendErr := b.bot.SendVoice(sendCtx, &bot.SendVoiceParams{
		ChatID: chatID,
		Voice: &models.InputFileUpload{
			Filename: "voice.ogg", // Telegram prefers ogg
			Data:     bytes.NewReader(audioData),
		},
	})

	if sendErr != nil {
		logger.ToolError("Chat[%d]: Error sending voice note: %v", chatID, sendErr)
		return fmt.Sprintf("Error sending voice note: %v", sendErr), sendErr
	}

	logger.ToolInfo("Chat[%d]: Successfully sent voice note.", chatID)
	// Return a minimal success message, less likely to prompt the LLM to comment.
	return "OK", nil
}

func firstWords(value string, count int) string {
	words := strings.Fields(value)
	if len(words) < count {
		return value
	}
	return strings.Join(words[:count], " ") + "..."
}
