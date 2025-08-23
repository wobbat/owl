package utils

import (
	"fmt"
	"os"
	"path/filepath"

	"owl/internal/constants"
)

// EnsureOwlDirectories creates the necessary Owl directories if they don't exist
func EnsureOwlDirectories() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get user home directory: %w", err)
	}

	owlRoot := filepath.Join(homeDir, constants.OwlRootDir)

	directories := []string{
		owlRoot,
		filepath.Join(owlRoot, constants.OwlStateDir),
		filepath.Join(owlRoot, constants.OwlDotfilesDir),
		filepath.Join(owlRoot, constants.OwlSetupDir),
		filepath.Join(owlRoot, constants.OwlHostsDir),
		filepath.Join(owlRoot, constants.OwlGroupsDir),
	}

	for _, dir := range directories {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	return nil
}

// FileExists checks if a file exists
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

// DirExists checks if a directory exists
func DirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
