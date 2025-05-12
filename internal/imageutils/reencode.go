package imageutils

import (
	"fmt"
	"image"
	"image/jpeg"
	"image/png" // Import for PNG decoding support
	"os"
	"path/filepath"
	"strings"

	"github.com/hunterwarburton/ya8hoda/internal/logger" // Assuming logger path
)

// ReEncodeToJPEG attempts to decode an image from inputPath, re-encode it as a JPEG
// with the specified quality, and save it to a new temporary file in the same directory.
// It returns the path to the new file, its size, and any error encountered.
// It cleans up the new file if an error occurs after its creation.
func ReEncodeToJPEG(inputPath string, quality int) (newPath string, newSize int64, err error) {
	inputFile, err := os.Open(inputPath)
	if err != nil {
		return "", 0, fmt.Errorf("failed to open input file %s: %w", inputPath, err)
	}
	defer inputFile.Close()

	// Decode the image
	img, format, err := image.Decode(inputFile)
	if err != nil {
		// Don't treat decoding errors as fatal for the whole process, just skip re-encoding this one.
		logger.TelegramWarn("Failed to decode image %s (format: %s): %v. Skipping re-encoding.", inputPath, format, err)
		return "", 0, fmt.Errorf("failed to decode image: %w", err) // Return specific error for skipping
	}
	logger.TelegramDebug("Decoded image %s, original format: %s", inputPath, format)

	// Create a new temporary file path for the JPEG output
	ext := filepath.Ext(inputPath)
	baseName := strings.TrimSuffix(filepath.Base(inputPath), ext)
	// Ensure the temp dir exists (it should, as the original was downloaded there)
	tempDir := filepath.Dir(inputPath)
	newPath = filepath.Join(tempDir, fmt.Sprintf("%s_reencoded.jpg", baseName))

	outputFile, err := os.Create(newPath)
	if err != nil {
		return "", 0, fmt.Errorf("failed to create output file %s: %w", newPath, err)
	}
	defer func() {
		// Ensure file is closed, and remove it if an error occurred *after* creation
		if closeErr := outputFile.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("failed to close output file %s: %w", newPath, closeErr)
		}
		if err != nil {
			os.Remove(newPath) // Clean up the partially created/failed output file
			newPath = ""       // Ensure we don't return the path if there was an error
			newSize = 0
		}
	}()

	// Encode the image as JPEG
	jpegOptions := &jpeg.Options{Quality: quality}
	if err = jpeg.Encode(outputFile, img, jpegOptions); err != nil {
		return "", 0, fmt.Errorf("failed to encode image %s to JPEG: %w", newPath, err)
	}

	// Get the size of the new file
	fileInfo, err := outputFile.Stat()
	if err != nil {
		return "", 0, fmt.Errorf("failed to stat output file %s: %w", newPath, err)
	}
	newSize = fileInfo.Size()

	logger.TelegramInfo("Successfully re-encoded %s to %s (Size: %d bytes, Quality: %d)", inputPath, newPath, newSize, quality)

	return newPath, newSize, nil // Success
}

// Helper to ensure necessary decoders are registered
func init() {
	image.RegisterFormat("png", "png", png.Decode, png.DecodeConfig)
	// Add other formats if needed (gif, webp, etc.) but require respective packages
	// image.RegisterFormat("jpeg", "jpeg", jpeg.Decode, jpeg.DecodeConfig) // Usually registered by default via image/jpeg import
}

// EnsureTmpDirExists checks if the tmp directory exists, creating it if not.
// It relies on a shared understanding or configuration of the tmp path.
// TODO: Get tmp path from config or bot struct
func EnsureTmpDirExists() error {
	tmpDir := filepath.Join(".", "data", "tmp") // Example path, adjust as needed
	if _, err := os.Stat(tmpDir); os.IsNotExist(err) {
		logger.TelegramInfo("Creating temporary directory: %s", tmpDir)
		err = os.MkdirAll(tmpDir, 0755) // Use appropriate permissions
		if err != nil {
			logger.TelegramError("Failed to create temporary directory %s: %v", tmpDir, err)
			return fmt.Errorf("failed to create temporary directory %s: %w", tmpDir, err)
		}
	} else if err != nil {
		// Handle other potential errors with stat
		logger.TelegramError("Error checking temporary directory %s: %v", tmpDir, err)
		return fmt.Errorf("error checking temporary directory %s: %w", tmpDir, err)
	}
	return nil
}
