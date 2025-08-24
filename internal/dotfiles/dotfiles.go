package dotfiles

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"owl/internal/types"
	"owl/internal/ui"
	"owl/internal/utils"
)

// ConfigAction represents an action to be taken on a config file
type ConfigAction struct {
	Destination string
	Source      string
	Status      string // copy, skip, update, conflict, create
	Reason      string
}

// AnalyzeConfigs analyzes what config actions need to be taken using lock file state
func AnalyzeConfigs(configs map[string]string) ([]ConfigAction, error) {
	var actions []ConfigAction

	// Load the config lock to check what was last applied
	lock, err := utils.LoadConfigLock()
	if err != nil {
		return nil, fmt.Errorf("failed to load config lock: %w", err)
	}

	for destination, source := range configs {
		homeDir, _ := os.UserHomeDir()
		destinationPath := destination
		if strings.HasPrefix(destination, "~") {
			destinationPath = filepath.Join(homeDir, destination[1:])
		} else if !filepath.IsAbs(destination) {
			destinationPath = filepath.Join(homeDir, destination)
		}

		sourcePath, err := filepath.Abs(source)
		if err != nil {
			sourcePath = source
		}

		var action ConfigAction

		// Check if source exists
		if _, err := os.Stat(sourcePath); os.IsNotExist(err) {
			action = ConfigAction{
				Destination: destination,
				Source:      source,
				Status:      "conflict",
				Reason:      "Source file/folder does not exist",
			}
		} else if _, err := os.Stat(destinationPath); os.IsNotExist(err) {
			action = ConfigAction{
				Destination: destination,
				Source:      source,
				Status:      "create",
				Reason:      "Destination does not exist",
			}
		} else {
			// Check if source has changed since last application using lock file
			currentSourceHash, err := getFileHash(sourcePath)
			if err != nil {
				action = ConfigAction{
					Destination: destination,
					Source:      source,
					Status:      "update",
					Reason:      "Failed to hash source, will update",
				}
			} else {
				lastAppliedHash := lock.Configs[destination]

				if currentSourceHash == lastAppliedHash && lastAppliedHash != "" {
					action = ConfigAction{
						Destination: destination,
						Source:      source,
						Status:      "skip",
						Reason:      "No changes detected",
					}
				} else {
					action = ConfigAction{
						Destination: destination,
						Source:      source,
						Status:      "update",
						Reason:      "Changes detected or first time setup",
					}
				}
			}
		}

		actions = append(actions, action)
	}

	return actions, nil
}

// ProcessConfigsPerPackage processes dotfiles for each package individually
func ProcessConfigsPerPackage(configEntries []types.ConfigEntry, dryRun bool, globalUI *ui.UI) error {
	packagesWithConfigs := filterPackagesWithConfigs(configEntries)

	if len(packagesWithConfigs) == 0 {
		return nil
	}

	if dryRun {
		fmt.Println("Configuration files to sync:")
		for _, entry := range packagesWithConfigs {
			configMap := convertConfigsToMap(entry.Configs)
			actions, err := AnalyzeConfigs(configMap)
			if err != nil {
				continue
			}

			for _, action := range actions {
				if action.Status != "skip" {
					fmt.Printf("  %s Would sync: %s -> %s\n", ui.Icon.Link, action.Source, action.Destination)
				}
			}
		}
		fmt.Println()
		return nil
	}

	// First pass: analyze all packages to see if we can show a summary
	packagesWithChanges := []types.ConfigEntry{}
	packagesUpToDate := []types.ConfigEntry{}
	packagesWithConflicts := []types.ConfigEntry{}

	for _, entry := range packagesWithConfigs {
		configMap := convertConfigsToMap(entry.Configs)
		actions, err := AnalyzeConfigs(configMap)
		if err != nil {
			packagesWithChanges = append(packagesWithChanges, entry)
			continue
		}

		hasChanges := false
		hasConflicts := false

		for _, action := range actions {
			if action.Status == "conflict" {
				hasConflicts = true
			} else if action.Status != "skip" {
				hasChanges = true
			}
		}

		if hasConflicts {
			packagesWithConflicts = append(packagesWithConflicts, entry)
		} else if hasChanges {
			packagesWithChanges = append(packagesWithChanges, entry)
		} else {
			packagesUpToDate = append(packagesUpToDate, entry)
		}
	}

	// Show summary for up-to-date packages if there are any
	if len(packagesUpToDate) > 0 {
		showDotfilesSummary(packagesUpToDate, globalUI)
	}

	// Process packages that need changes individually
	for _, entry := range packagesWithChanges {
		err := processPackageConfigs(entry, globalUI)
		if err != nil {
			return fmt.Errorf("failed to process configs for package %s: %w", entry.Package, err)
		}
	}

	// Process packages with conflicts individually
	for _, entry := range packagesWithConflicts {
		err := processPackageConfigs(entry, globalUI)
		if err != nil {
			return fmt.Errorf("failed to process configs for package %s: %w", entry.Package, err)
		}
	}

	return nil
}

// showDotfilesSummary shows a summary for packages with up-to-date dotfiles
func showDotfilesSummary(packages []types.ConfigEntry, globalUI *ui.UI) {
	if len(packages) == 0 {
		return
	}

	// Count total packages
	packageCount := len(packages)

	// Show combined summary
	if packageCount == 1 {
		// Single package, show individual entry
		entry := packages[0]
		sourcePrefix := globalUI.FormatPackageSource(&entry)
		fmt.Printf("%s%s %s\n", sourcePrefix,
			func(s string) string { return "\x1b[36m" + s + "\x1b[0m" }(entry.Package),
			func(s string) string { return "\x1b[2m" + s + "\x1b[0m" }("->"))

		spinner := ui.NewSpinner("  Dotfiles - checking...", types.SpinnerOptions{Enabled: true})
		spinner.Stop("")
		fmt.Println()
	} else {
		// Multiple packages, show combined summary
		packageNames := make([]string, packageCount)
		for i, entry := range packages {
			packageNames[i] = entry.Package
		}

		// Create a summary line with package names
		summary := fmt.Sprintf("%d packages", packageCount)
		if packageCount <= 5 {
			summary = strings.Join(packageNames, ", ")
		}

		fmt.Printf("%s %s\n",
			func(s string) string { return "\x1b[36m" + s + "\x1b[0m" }(summary),
			func(s string) string { return "\x1b[2m" + s + "\x1b[0m" }("->"))

		spinner := ui.NewSpinner("  Dotfiles - checking...", types.SpinnerOptions{Enabled: true})
		spinner.Stop("")
		fmt.Println()
	}
}

// processPackageConfigs processes configs for a single package
func processPackageConfigs(entry types.ConfigEntry, globalUI *ui.UI) error {
	configMap := convertConfigsToMap(entry.Configs)
	actions, err := AnalyzeConfigs(configMap)
	if err != nil {
		return err
	}

	// Check if anything needs to be done
	needsAction := false
	hasConflicts := false

	for _, action := range actions {
		if action.Status != "skip" {
			needsAction = true
		}
		if action.Status == "conflict" {
			hasConflicts = true
		}
	}

	// Show package header
	sourcePrefix := globalUI.FormatPackageSource(&entry)
	fmt.Printf("%s%s %s\n", sourcePrefix,
		func(s string) string { return "\x1b[36m" + s + "\x1b[0m" }(entry.Package),
		func(s string) string { return "\x1b[2m" + s + "\x1b[0m" }("->"))

	if hasConflicts {
		// Show conflicts
		spinner := ui.NewSpinner("  Dotfiles - checking...", types.SpinnerOptions{Enabled: true})
		spinner.Fail("conflicts detected")
	} else if !needsAction {
		// All files are up to date
		spinner := ui.NewSpinner("  Dotfiles - checking...", types.SpinnerOptions{Enabled: true})
		spinner.Stop("")
	} else {
		// Process the changes
		spinner := ui.NewSpinner("  Dotfiles - syncing...", types.SpinnerOptions{Enabled: true})

		successCount := 0
		errorCount := 0

		// Load lock file to update it after successful operations
		lock, err := utils.LoadConfigLock()
		if err != nil {
			lock = &utils.ConfigLock{
				Configs: make(map[string]string),
				Setups:  make(map[string]string),
			}
		}

		for _, action := range actions {
			if action.Status == "skip" || action.Status == "conflict" {
				continue
			}

			err := applyConfigAction(action)
			if err != nil {
				errorCount++
			} else {
				successCount++

				// Update the lock with the new hash after successful application
				sourcePath, _ := filepath.Abs(action.Source)
				if newHash, err := getFileHash(sourcePath); err == nil {
					lock.Configs[action.Destination] = newHash
				}
			}
		}

		// Save the updated lock file if we had any successes
		if successCount > 0 {
			if err := utils.SaveConfigLock(lock); err != nil {
				// Don't fail the whole operation if lock save fails, just warn
				fmt.Printf("Warning: Failed to save config lock: %v\n", err)
			}
		}

		if errorCount == 0 {
			spinner.Stop("")
		} else {
			spinner.Fail("sync failed")
		}
	}

	fmt.Println() // Add spacing between packages
	return nil
}

// applyConfigAction applies a single config action
func applyConfigAction(action ConfigAction) error {
	homeDir, _ := os.UserHomeDir()
	destinationPath := action.Destination
	if strings.HasPrefix(action.Destination, "~") {
		destinationPath = filepath.Join(homeDir, action.Destination[1:])
	} else if !filepath.IsAbs(action.Destination) {
		destinationPath = filepath.Join(homeDir, action.Destination)
	}

	sourcePath, err := filepath.Abs(action.Source)
	if err != nil {
		sourcePath = action.Source
	}

	// Create parent directory if it doesn't exist
	parentDir := filepath.Dir(destinationPath)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return fmt.Errorf("failed to create parent directory: %w", err)
	}

	// Remove existing destination if it exists
	if _, err := os.Stat(destinationPath); err == nil {
		if err := os.RemoveAll(destinationPath); err != nil {
			return fmt.Errorf("failed to remove existing destination: %w", err)
		}
	}

	// Check if source is directory or file
	sourceInfo, err := os.Stat(sourcePath)
	if err != nil {
		return fmt.Errorf("failed to stat source: %w", err)
	}

	// Copy source to destination
	if sourceInfo.IsDir() {
		cmd := exec.Command("cp", "-r", sourcePath, destinationPath)
		return cmd.Run()
	} else {
		cmd := exec.Command("cp", sourcePath, destinationPath)
		return cmd.Run()
	}
}

// filterPackagesWithConfigs filters packages that have configs
func filterPackagesWithConfigs(entries []types.ConfigEntry) []types.ConfigEntry {
	var result []types.ConfigEntry
	for _, entry := range entries {
		if len(entry.Configs) > 0 {
			result = append(result, entry)
		}
	}
	return result
}

// convertConfigsToMap converts configs array to map
func convertConfigsToMap(configs []types.ConfigMapping) map[string]string {
	result := make(map[string]string)
	for _, config := range configs {
		result[config.Destination] = config.Source
	}
	return result
}

// getFileHash calculates SHA256 hash of a file or directory
func getFileHash(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}

	if info.IsDir() {
		// For directories, create a hash based on all files
		return getDirHash(path)
	}

	// For files, calculate SHA256
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

// getDirHash calculates a hash for a directory based on all its contents
func getDirHash(dirPath string) (string, error) {
	hash := sha256.New()

	var files []string

	// First, collect all file paths for consistent ordering
	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip hidden files and directories (starting with .)
		if strings.HasPrefix(info.Name(), ".") && info.Name() != "." {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		files = append(files, path)
		return nil
	})

	if err != nil {
		return "", err
	}

	// Sort files to ensure consistent hash generation
	for i := 0; i < len(files); i++ {
		for j := i + 1; j < len(files); j++ {
			if files[i] > files[j] {
				files[i], files[j] = files[j], files[i]
			}
		}
	}

	// Process each file in sorted order
	for _, path := range files {
		info, err := os.Stat(path)
		if err != nil {
			continue // Skip files that can't be accessed
		}

		// Include the relative path in the hash for directory structure
		relPath, err := filepath.Rel(dirPath, path)
		if err != nil {
			relPath = path
		}
		hash.Write([]byte(relPath + "\n"))

		if !info.IsDir() {
			// Include file modification time and size for change detection
			hash.Write([]byte(fmt.Sprintf("mtime:%d\n", info.ModTime().Unix())))
			hash.Write([]byte(fmt.Sprintf("size:%d\n", info.Size())))

			// Include file content for accurate change detection
			file, err := os.Open(path)
			if err != nil {
				// If we can't read the file, include just the metadata
				hash.Write([]byte(fmt.Sprintf("unreadable:%s\n", err.Error())))
				continue
			}

			_, copyErr := io.Copy(hash, file)
			closeErr := file.Close()

			if copyErr != nil {
				// If content can't be read, at least use metadata
				hash.Write([]byte(fmt.Sprintf("content-error:%s\n", copyErr.Error())))
			}
			if closeErr != nil {
				// Log close error but don't fail
				hash.Write([]byte(fmt.Sprintf("close-error:%s\n", closeErr.Error())))
			}
		} else {
			// For directories, just include the name and mod time
			hash.Write([]byte(fmt.Sprintf("dir-mtime:%d\n", info.ModTime().Unix())))
		}
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}
