package logger

import (
	"fmt"
	"log"
	"os"
)

// ANSI color codes for levels
const (
	ColorReset   = "\033[0m"
	ColorRed     = "\033[31m"
	ColorGreen   = "\033[32m"
	ColorYellow  = "\033[33m"
	ColorBlue    = "\033[34m"
	ColorMagenta = "\033[35m"
	ColorCyan    = "\033[36m"
	ColorOrange  = "\033[38;5;208m" // Orange for Warn
)

// Service Identifiers and Colors
const (
	SrvTelegram = "TG"
	SrvLLM      = "LLM"
	SrvTool     = "TOOL"
	SrvGeneric  = "GEN"

	ColorTelegram = ColorBlue
	ColorLLM      = ColorYellow
	ColorTool     = ColorMagenta
	ColorGeneric  = ColorReset // Or another color like white
)

var (
	// Debug flag to control debug logging
	debugEnabled = false
	// Base logger instances (without service prefix)
	baseDebugLogger *log.Logger
	baseInfoLogger  *log.Logger
	baseWarnLogger  *log.Logger // Added Warn logger
	baseErrorLogger *log.Logger
)

// Init initializes the logger
func Init(debug bool) {
	debugEnabled = debug

	// Create base loggers with level prefixes and colors
	debugPrefix := fmt.Sprintf("%sDEBUG: %s", ColorCyan, ColorReset)
	infoPrefix := fmt.Sprintf("%sINFO:  %s", ColorGreen, ColorReset)
	warnPrefix := fmt.Sprintf("%sWARN:  %s", ColorOrange, ColorReset) // Added Warn prefix
	errorPrefix := fmt.Sprintf("%sERROR: %s", ColorRed, ColorReset)

	baseDebugLogger = log.New(os.Stdout, debugPrefix, log.Ldate|log.Ltime|log.Lshortfile)
	baseInfoLogger = log.New(os.Stdout, infoPrefix, log.Ldate|log.Ltime)
	baseWarnLogger = log.New(os.Stdout, warnPrefix, log.Ldate|log.Ltime|log.Lshortfile) // Log Warn to Stdout with file info
	baseErrorLogger = log.New(os.Stderr, errorPrefix, log.Ldate|log.Ltime|log.Lshortfile)

	if debugEnabled {
		// Use the new generic Debug function for initialization messages
		Debug(SrvGeneric, "Debug logging enabled")
	}
}

// --- Generic Log Functions (Original behavior) ---

// Debug logs a debug message if debug mode is enabled (use for generic/unclassified logs)
func Debug(format string, v ...interface{}) {
	logInternal(baseDebugLogger, SrvGeneric, ColorGeneric, ColorCyan, format, v...)
}

// Info logs an info message (use for generic/unclassified logs)
func Info(format string, v ...interface{}) {
	logInternal(baseInfoLogger, SrvGeneric, ColorGeneric, ColorGreen, format, v...)
}

// Warn logs a warning message
func Warn(format string, v ...interface{}) {
	logInternal(baseWarnLogger, SrvGeneric, ColorGeneric, ColorOrange, format, v...)
}

// Error logs an error message (use for generic/unclassified logs)
func Error(format string, v ...interface{}) {
	logInternal(baseErrorLogger, SrvGeneric, ColorGeneric, ColorRed, format, v...)
}

// --- Service-Specific Log Functions ---

// Telegram
func TelegramDebug(format string, v ...interface{}) {
	logInternal(baseDebugLogger, SrvTelegram, ColorTelegram, ColorCyan, format, v...)
}
func TelegramInfo(format string, v ...interface{}) {
	logInternal(baseInfoLogger, SrvTelegram, ColorTelegram, ColorGreen, format, v...)
}
func TelegramWarn(format string, v ...interface{}) {
	logInternal(baseWarnLogger, SrvTelegram, ColorTelegram, ColorOrange, format, v...)
}
func TelegramError(format string, v ...interface{}) {
	logInternal(baseErrorLogger, SrvTelegram, ColorTelegram, ColorRed, format, v...)
}

// LLM
func LLMDebug(format string, v ...interface{}) {
	logInternal(baseDebugLogger, SrvLLM, ColorLLM, ColorCyan, format, v...)
}
func LLMInfo(format string, v ...interface{}) {
	logInternal(baseInfoLogger, SrvLLM, ColorLLM, ColorGreen, format, v...)
}
func LLMWarn(format string, v ...interface{}) {
	logInternal(baseWarnLogger, SrvLLM, ColorLLM, ColorOrange, format, v...)
}
func LLMError(format string, v ...interface{}) {
	logInternal(baseErrorLogger, SrvLLM, ColorLLM, ColorRed, format, v...)
}

// Tool
func ToolDebug(format string, v ...interface{}) {
	logInternal(baseDebugLogger, SrvTool, ColorTool, ColorCyan, format, v...)
}
func ToolInfo(format string, v ...interface{}) {
	logInternal(baseInfoLogger, SrvTool, ColorTool, ColorGreen, format, v...)
}
func ToolWarn(format string, v ...interface{}) {
	logInternal(baseWarnLogger, SrvTool, ColorTool, ColorOrange, format, v...)
}
func ToolError(format string, v ...interface{}) {
	logInternal(baseErrorLogger, SrvTool, ColorTool, ColorRed, format, v...)
}

// --- Internal Helper ---

// logInternal handles the actual logging logic with service prefix and colors
func logInternal(logger *log.Logger, service, serviceColor, levelColor, format string, v ...interface{}) {
	if logger == baseDebugLogger && !debugEnabled {
		return // Don't log debug if not enabled
	}
	// Format: [SERVICE_COLOR][SERVICE_TAG][RESET] LEVEL_PREFIX (from logger) Formatted_Message
	// We need to construct the full message string including the service prefix
	// The logger's prefix already includes the level tag (e.g., DEBUG:)
	servicePrefix := fmt.Sprintf("%s[%s]%s ", serviceColor, service, ColorReset)
	msg := fmt.Sprintf(format, v...)

	// Output takes the full string to print after the standard logger prefix
	// Call depth 3: logInternal -> Service Func -> Actual Caller
	logger.Output(3, servicePrefix+msg)
}

// IsDebugEnabled returns whether debug logging is enabled
func IsDebugEnabled() bool {
	return debugEnabled
}
