package setup

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"owl/internal/constants"
	"owl/internal/types"
	"owl/internal/ui"
)

// SetupAction represents an action to be taken on a setup script
type SetupAction struct {
	Script     string
	ScriptPath string
	Status     string // execute, skip, error
	Reason     string
}

// SetupLock represents the setup scripts lock file
type SetupLock struct {
	Setups map[string]string `json:"setups"`
}

// getScriptExecutor returns the command and args to execute a script
func getScriptExecutor(scriptPath string) (string, []string, error) {
	ext := strings.ToLower(filepath.Ext(scriptPath))

	switch ext {
	case ".js", ".ts":
		return "node", []string{scriptPath}, nil
	case ".sh":
		return "bash", []string{scriptPath}, nil
	default:
		return "", nil, fmt.Errorf("unsupported script type: %s", ext)
	}
}

// analyzeSetupScripts analyzes what setup scripts need to be executed
func analyzeSetupScripts(scripts []string) ([]SetupAction, error) {
	var actions []SetupAction
	lock, err := loadSetupLock()
	if err != nil {
		// If lock doesn't exist, create empty one
		lock = &SetupLock{Setups: make(map[string]string)}
	}

	homeDir, _ := os.UserHomeDir()

	for _, script := range scripts {
		scriptPath := filepath.Join(homeDir, constants.OwlRootDir, constants.OwlSetupDir, script)

		// Check if script exists
		if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
			actions = append(actions, SetupAction{
				Script:     script,
				ScriptPath: scriptPath,
				Status:     "error",
				Reason:     "Script file does not exist",
			})
			continue
		}

		// Check if script type is supported
		ext := strings.ToLower(filepath.Ext(scriptPath))
		if ext != ".js" && ext != ".ts" && ext != ".sh" {
			actions = append(actions, SetupAction{
				Script:     script,
				ScriptPath: scriptPath,
				Status:     "error",
				Reason:     fmt.Sprintf("Unsupported script type: %s (supported: .js, .ts, .sh)", ext),
			})
			continue
		}

		// Check if script has changed using hash comparison
		currentHash, err := getFileHash(scriptPath)
		if err != nil {
			actions = append(actions, SetupAction{
				Script:     script,
				ScriptPath: scriptPath,
				Status:     "error",
				Reason:     "Failed to calculate script hash",
			})
			continue
		}

		lastExecutedHash := lock.Setups[script]

		if currentHash == lastExecutedHash && lastExecutedHash != "" {
			actions = append(actions, SetupAction{
				Script:     script,
				ScriptPath: scriptPath,
				Status:     "skip",
				Reason:     "No changes detected",
			})
		} else {
			actions = append(actions, SetupAction{
				Script:     script,
				ScriptPath: scriptPath,
				Status:     "execute",
				Reason:     "Changes detected or first time execution",
			})
		}
	}

	return actions, nil
}

// ProcessSetupScripts processes setup scripts
func ProcessSetupScripts(scripts []string, dryRun bool, globalUI *ui.UI) error {
	if len(scripts) == 0 {
		return nil
	}

	actions, err := analyzeSetupScripts(scripts)
	if err != nil {
		return fmt.Errorf("failed to analyze setup scripts: %w", err)
	}

	if dryRun {
		fmt.Println("Setup scripts to run:")
		for _, action := range actions {
			if action.Status != "skip" {
				fmt.Printf("  %s Would run: %s\n", ui.Icon.Script, action.Script)
			}
		}
		fmt.Println()
		return nil
	}

	// Show what will be done
	fmt.Println("Setup scripts:")

	executable := []SetupAction{}
	errors := []SetupAction{}

	for _, action := range actions {
		switch action.Status {
		case "execute":
			fmt.Printf("  %s Execute: %s\n", ui.Icon.Script, action.Script)
			executable = append(executable, action)
		case "skip":
			fmt.Printf("  %s Skip: %s (%s)\n", ui.Icon.Skip, action.Script, action.Reason)
		case "error":
			fmt.Printf("  %s Error: %s (%s)\n", ui.Icon.Err, action.Script, action.Reason)
			errors = append(errors, action)
		}
	}

	if len(executable) == 0 {
		if len(errors) > 0 {
			globalUI.Err("All scripts have errors")
		} else {
			globalUI.Ok("No setup scripts to execute")
		}
		return nil
	}

	spinner := ui.NewSpinner(fmt.Sprintf("Executing %d setup scripts", len(executable)), types.SpinnerOptions{Enabled: true})

	successCount := 0
	errorCount := 0
	lock, _ := loadSetupLock()
	if lock == nil {
		lock = &SetupLock{Setups: make(map[string]string)}
	}

	for _, action := range executable {
		spinner.Update(fmt.Sprintf("Executing %s...", action.Script))

		command, args, err := getScriptExecutor(action.ScriptPath)
		if err != nil {
			errorCount++
			globalUI.Err(fmt.Sprintf("Failed to execute %s: %v", action.Script, err))
			continue
		}

		cmd := exec.Command(command, args...)
		err = cmd.Run()
		if err != nil {
			errorCount++
			globalUI.Err(fmt.Sprintf("Failed to execute %s: %v", action.Script, err))
		} else {
			// Update the lock with the new hash
			newHash, _ := getFileHash(action.ScriptPath)
			lock.Setups[action.Script] = newHash
			successCount++
		}
	}

	// Save the updated lock file
	if successCount > 0 {
		saveSetupLock(lock)
	}

	if errorCount == 0 {
		spinner.Stop(fmt.Sprintf("%d scripts executed successfully", successCount))
	} else {
		spinner.Fail(fmt.Sprintf("%d failed, %d succeeded", errorCount, successCount))
	}

	fmt.Println()
	return nil
}

// loadSetupLock loads the setup scripts lock file
func loadSetupLock() (*SetupLock, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user home directory: %w", err)
	}

	lockPath := filepath.Join(homeDir, constants.OwlRootDir, constants.OwlStateDir, "setup.lock")

	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		return &SetupLock{Setups: make(map[string]string)}, nil
	}

	file, err := os.Open(lockPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open setup lock file: %w", err)
	}
	defer file.Close()

	var lock SetupLock
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&lock); err != nil {
		return nil, fmt.Errorf("failed to parse setup lock file: %w", err)
	}

	return &lock, nil
}

// saveSetupLock saves the setup scripts lock file
func saveSetupLock(lock *SetupLock) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get user home directory: %w", err)
	}

	stateDir := filepath.Join(homeDir, constants.OwlRootDir, constants.OwlStateDir)
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	lockPath := filepath.Join(stateDir, "setup.lock")
	file, err := os.Create(lockPath)
	if err != nil {
		return fmt.Errorf("failed to create setup lock file: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(lock)
}

// getFileHash calculates SHA256 hash of a file
func getFileHash(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}
