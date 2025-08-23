package utils

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"owl/internal/constants"
)

// ConfigLock represents the config files lock file
type ConfigLock struct {
	Configs map[string]string `json:"configs"` // destination -> source_hash
	Setups  map[string]string `json:"setups"`  // script -> script_hash
}

// LoadConfigLock loads the config lock file
func LoadConfigLock() (*ConfigLock, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user home directory: %w", err)
	}

	lockPath := filepath.Join(homeDir, constants.OwlRootDir, constants.OwlStateDir, "config.lock")

	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		return &ConfigLock{
			Configs: make(map[string]string),
			Setups:  make(map[string]string),
		}, nil
	}

	file, err := os.Open(lockPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open config lock file: %w", err)
	}
	defer file.Close()

	var lock ConfigLock
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&lock); err != nil {
		return nil, fmt.Errorf("failed to parse config lock file: %w", err)
	}

	// Initialize maps if they're nil
	if lock.Configs == nil {
		lock.Configs = make(map[string]string)
	}
	if lock.Setups == nil {
		lock.Setups = make(map[string]string)
	}

	return &lock, nil
}

// SaveConfigLock saves the config lock file
func SaveConfigLock(lock *ConfigLock) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get user home directory: %w", err)
	}

	stateDir := filepath.Join(homeDir, constants.OwlRootDir, constants.OwlStateDir)
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	lockPath := filepath.Join(stateDir, "config.lock")
	file, err := os.Create(lockPath)
	if err != nil {
		return fmt.Errorf("failed to create config lock file: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(lock)
}
