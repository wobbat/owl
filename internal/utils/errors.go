package utils

import (
	"fmt"
	"os"
	"strings"
)

// OwlError represents a general Owl error
type OwlError struct {
	Message string
	Code    string
	Cause   error
}

func (e *OwlError) Error() string {
	return e.Message
}

func (e *OwlError) Unwrap() error {
	return e.Cause
}

// NewOwlError creates a new OwlError
func NewOwlError(message, code string, cause error) *OwlError {
	return &OwlError{
		Message: message,
		Code:    code,
		Cause:   cause,
	}
}

// ConfigError represents a configuration parsing error
type ConfigError struct {
	FilePath   string
	LineNumber int
	Line       string
	Message    string
}

func (e *ConfigError) Error() string {
	return fmt.Sprintf("%s:%d: %s\n  → %s", e.FilePath, e.LineNumber, e.Message, strings.TrimSpace(e.Line))
}

// NewConfigError creates a new ConfigError
func NewConfigError(filePath string, lineNumber int, line, message string) *ConfigError {
	return &ConfigError{
		FilePath:   filePath,
		LineNumber: lineNumber,
		Line:       line,
		Message:    message,
	}
}

// HandleError handles errors consistently across the application
func HandleError(message string, err error) {
	var errorMessage string
	if err != nil {
		errorMessage = err.Error()
	} else {
		errorMessage = "Unknown error"
	}
	fmt.Fprintf(os.Stderr, "%s: %s\n", message, errorMessage)
	os.Exit(1)
}

// SafeExecute safely executes operations with consistent error handling
func SafeExecute(operation func() error, errorMessage string) {
	if err := operation(); err != nil {
		HandleError(errorMessage, err)
	}
}

// SafeExecuteWithFallback wraps a function with error handling and returns a default value on failure
func SafeExecuteWithFallback[T any](operation func() (T, error), fallback T, errorMessage string) T {
	result, err := operation()
	if err != nil {
		if errorMessage != "" {
			fmt.Fprintf(os.Stderr, "Warning: %s: %v\n", errorMessage, err)
		}
		return fallback
	}
	return result
}
