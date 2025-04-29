package logger

import (
	"fmt"
	"log"
	"os"
)

var (
	// Debug flag to control debug logging
	debugEnabled = false
	// The logger instance
	debugLogger *log.Logger
	infoLogger  *log.Logger
	errorLogger *log.Logger
)

// Init initializes the logger
func Init(debug bool) {
	debugEnabled = debug

	// Create loggers with appropriate prefixes
	debugLogger = log.New(os.Stdout, "DEBUG: ", log.Ldate|log.Ltime|log.Lshortfile)
	infoLogger = log.New(os.Stdout, "INFO: ", log.Ldate|log.Ltime)
	errorLogger = log.New(os.Stderr, "ERROR: ", log.Ldate|log.Ltime|log.Lshortfile)

	if debugEnabled {
		Debug("Debug logging enabled")
	}
}

// Debug logs a debug message if debug mode is enabled
func Debug(format string, v ...interface{}) {
	if debugEnabled {
		debugLogger.Output(2, fmt.Sprintf(format, v...))
	}
}

// Info logs an info message
func Info(format string, v ...interface{}) {
	infoLogger.Output(2, fmt.Sprintf(format, v...))
}

// Error logs an error message
func Error(format string, v ...interface{}) {
	errorLogger.Output(2, fmt.Sprintf(format, v...))
}

// IsDebugEnabled returns whether debug logging is enabled
func IsDebugEnabled() bool {
	return debugEnabled
}
