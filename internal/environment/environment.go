package environment

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"owl/internal/constants"
	"owl/internal/types"
	"owl/internal/ui"
)

// GlobalEnvState represents the global environment state
type GlobalEnvState struct {
	SchemaVersion string   `json:"schema_version"`
	GlobalEnvVars []string `json:"global_env_vars"`
}

// writeEnvironmentFileBash writes environment variables to bash format
func writeEnvironmentFileBash(envMap map[string]string, filePath string, debug bool) error {
	content := "#!/bin/bash\n"
	content += "# This file is managed by Owl package manager\n"
	content += "# Manual changes may be overwritten\n"

	if len(envMap) > 0 {
		content += "\n"
		for key, value := range envMap {
			// Escape single quotes in the value and wrap in single quotes
			escapedValue := strings.ReplaceAll(value, "'", "'\\''")
			content += fmt.Sprintf("export %s='%s'\n", key, escapedValue)
		}
	}

	if debug {
		fmt.Printf("  Writing to %s:\n", filePath)
		fmt.Println(content)
		fmt.Println("  --- End of content ---")
	}

	return os.WriteFile(filePath, []byte(content), 0644)
}

// writeEnvironmentFileFish writes environment variables to fish format
func writeEnvironmentFileFish(envMap map[string]string, filePath string, debug bool) error {
	content := "# This file is managed by Owl package manager\n"
	content += "# Manual changes may be overwritten"

	if len(envMap) > 0 {
		content += "\n\n"
		for key, value := range envMap {
			// For Fish shell, use set -x (export) command
			escapedValue := strings.ReplaceAll(value, "'", "\\'")
			content += fmt.Sprintf("set -x %s '%s'\n", key, escapedValue)
		}
	}

	if debug {
		fmt.Printf("  Writing to %s:\n", filePath)
		fmt.Println(content)
		fmt.Println("  --- End of content ---")
	}

	return os.WriteFile(filePath, []byte(content), 0644)
}

// saveGlobalEnvState saves the global environment state
func saveGlobalEnvState(state *GlobalEnvState) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get user home directory: %w", err)
	}

	stateDir := filepath.Join(homeDir, constants.OwlRootDir, constants.OwlStateDir)
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	statePath := filepath.Join(stateDir, "global-env.lock")
	file, err := os.Create(statePath)
	if err != nil {
		return fmt.Errorf("failed to create global env state file: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(state)
}

// ProcessEnvironmentVariables processes environment variables
func ProcessEnvironmentVariables(envs []types.EnvVar, dryRun bool, debug bool, globalUI *ui.UI) error {
	if len(envs) == 0 {
		return nil
	}

	if dryRun {
		fmt.Println("Environment variables to set:")
		for _, env := range envs {
			fmt.Printf("  %s Would set: %s=%s\n", ui.Icon.Ok, env.Key, env.Value)
		}
		fmt.Println()
		return nil
	}

	if debug {
		fmt.Printf("Processing %d environment variables\n", len(envs))
	}

	// Create environment map
	envMap := make(map[string]string)
	for _, env := range envs {
		envMap[env.Key] = env.Value
		if debug {
			fmt.Printf("  Setting environment variable: %s=%s\n", env.Key, env.Value)
		}
	}

	homeDir, _ := os.UserHomeDir()
	envFileSh := filepath.Join(homeDir, constants.OwlRootDir, "env.sh")

	err := writeEnvironmentFileBash(envMap, envFileSh, debug)
	if err != nil {
		return fmt.Errorf("failed to write environment variables to %s: %w", envFileSh, err)
	}

	return nil
}

// ProcessGlobalEnvironmentVariables processes global environment variables
func ProcessGlobalEnvironmentVariables(globalEnvs []types.EnvVar, dryRun bool, debug bool, globalUI *ui.UI) error {
	if len(globalEnvs) == 0 {
		return nil
	}

	if debug {
		fmt.Printf("Processing %d global environment variables\n", len(globalEnvs))
	}

	if dryRun {
		fmt.Println("Global environment variables to set:")
		for _, env := range globalEnvs {
			fmt.Printf("  %s Would set global: %s=%s\n", ui.Icon.Ok, env.Key, env.Value)
		}
		fmt.Println()
		return nil
	}

	// Create environment map
	envMap := make(map[string]string)
	var keys []string

	for _, env := range globalEnvs {
		envMap[env.Key] = env.Value
		keys = append(keys, env.Key)
		if debug {
			fmt.Printf("  Setting global environment variable: %s=%s\n", env.Key, env.Value)
		}
	}

	homeDir, _ := os.UserHomeDir()
	envFileSh := filepath.Join(homeDir, constants.OwlRootDir, "env.sh")
	envFileFish := filepath.Join(homeDir, constants.OwlRootDir, "env.fish")

	if debug {
		fmt.Printf("Writing environment files with %d variables\n", len(envMap))
	}

	// Write both bash and fish files
	if err := writeEnvironmentFileBash(envMap, envFileSh, debug); err != nil {
		return fmt.Errorf("failed to write bash environment file: %w", err)
	}

	if err := writeEnvironmentFileFish(envMap, envFileFish, debug); err != nil {
		return fmt.Errorf("failed to write fish environment file: %w", err)
	}

	if debug {
		fmt.Println("Successfully wrote environment files")
	}

	// Update state
	newState := &GlobalEnvState{
		SchemaVersion: constants.SchemaVersion,
		GlobalEnvVars: keys,
	}

	if err := saveGlobalEnvState(newState); err != nil {
		return fmt.Errorf("failed to save global env state: %w", err)
	}

	return nil
}
