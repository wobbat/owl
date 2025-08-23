package utils

import "strings"

// Compact filters out empty strings from a slice, similar to the TypeScript compact function
func Compact(slice []string) []string {
	var result []string
	for _, item := range slice {
		trimmed := strings.TrimSpace(item)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// CompactWithFilter filters out elements that don't match the predicate function
func CompactWithFilter[T any](slice []T, predicate func(T) bool) []T {
	var result []T
	for _, item := range slice {
		if predicate(item) {
			result = append(result, item)
		}
	}
	return result
}
