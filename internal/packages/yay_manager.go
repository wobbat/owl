package packages

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// YayPackageManager provides package management operations using yay as the backend
type YayPackageManager struct{}

// NewYayPackageManager creates a new yay-based package manager
func NewYayPackageManager() *YayPackageManager {
	return &YayPackageManager{}
}

// InstallPackage installs a package using yay
func (y *YayPackageManager) InstallPackage(packageName string, verbose bool) error {
	if verbose {
		fmt.Printf("Checking if package %s is already installed...\n", packageName)
	}

	// Check if package is already installed (works for individual packages)
	checkCmd := exec.Command("yay", "-Qq", packageName)
	if checkCmd.Run() == nil {
		// Package is already installed
		if verbose {
			fmt.Printf("Package %s is already installed\n", packageName)
		}
		return nil
	}

	if verbose {
		fmt.Printf("Package %s not found as individual package, checking if it's a group...\n", packageName)
	}

	// Skip the flawed package group detection logic and go directly to proper group checking
	groupCmd := exec.Command("yay", "-Sg", packageName)
	if groupCmd.Run() == nil {
		// It's a package group, check if packages in the group are installed
		if verbose {
			fmt.Printf("Detected %s as package group, checking if installed...\n", packageName)
		}
		if y.isPackageGroupInstalled(packageName) {
			if verbose {
				fmt.Printf("Package group %s is already installed\n", packageName)
			}
			return nil // Group is already installed
		}
		if verbose {
			fmt.Printf("Package group %s is not fully installed, proceeding with installation\n", packageName)
		}
	} else if verbose {
		fmt.Printf("Package %s is not a package group\n", packageName)
	}

	args := []string{"-S", "--noconfirm", packageName}
	if verbose {
		args = append(args, "--verbose")
	}

	cmd := exec.Command("yay", args...)

	// Capture stderr for better error messages
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if verbose {
		cmd.Stdout = nil
	} else {
		cmd.Stdout = nil // Suppress stdout in non-verbose mode
	}

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("yay install failed: %s", stderr.String())
	}

	return nil
}

// InstallPackages installs multiple packages using yay
func (y *YayPackageManager) InstallPackages(packages []string, verbose bool) error {
	if len(packages) == 0 {
		return nil
	}

	// Filter out already installed packages and package groups
	var packagesToInstall []string
	for _, pkg := range packages {
		if verbose {
			fmt.Printf("Checking package: %s\n", pkg)
		}

		// First check if it's an individual package
		checkCmd := exec.Command("yay", "-Qq", pkg)
		if checkCmd.Run() == nil {
			// Individual package is already installed
			if verbose {
				fmt.Printf("  %s is already installed (individual package)\n", pkg)
			}
			continue
		}

		// Check if it's a package group
		groupCmd := exec.Command("yay", "-Sg", pkg)
		if groupCmd.Run() == nil {
			// It's a package group, check if it's already installed
			if verbose {
				fmt.Printf("  %s is a package group, checking if installed...\n", pkg)
			}
			if y.isPackageGroupInstalled(pkg) {
				if verbose {
					fmt.Printf("  %s is already installed (package group)\n", pkg)
				}
				continue // Group is already installed
			}
			if verbose {
				fmt.Printf("  %s is not fully installed, will install\n", pkg)
			}
		} else if verbose {
			fmt.Printf("  %s is not a package group\n", pkg)
		}

		// Package/group is not installed, add to install list
		packagesToInstall = append(packagesToInstall, pkg)
	}

	if len(packagesToInstall) == 0 {
		return nil
	}

	args := []string{"-S", "--noconfirm"}
	if verbose {
		args = append(args, "--verbose")
	}
	args = append(args, packagesToInstall...)

	cmd := exec.Command("yay", args...)

	// Capture stderr for better error messages
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if verbose {
		cmd.Stdout = nil
	} else {
		cmd.Stdout = nil // Suppress stdout in non-verbose mode
	}

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("yay install failed: %s", stderr.String())
	}

	return nil
}

// RemovePackage removes a package using yay
func (y *YayPackageManager) RemovePackage(packageName string) error {
	// First check if the package is actually installed
	checkCmd := exec.Command("yay", "-Qq", packageName)
	if checkCmd.Run() != nil {
		// Package is not installed, nothing to remove
		return nil
	}

	args := []string{"-Rns", "--noconfirm", packageName}

	cmd := exec.Command("yay", args...)

	// Capture stderr to provide better error messages
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("yay remove failed: %s", stderr.String())
	}

	return nil
}

// UpgradeSystem upgrades all packages using yay
func (y *YayPackageManager) UpgradeSystem(verbose bool) error {
	args := []string{"-Syu", "--noconfirm"}
	if verbose {
		args = append(args, "--verbose")
	}

	cmd := exec.Command("yay", args...)

	// Capture stderr for better error messages
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if verbose {
		cmd.Stdout = nil
	} else {
		cmd.Stdout = nil // Suppress stdout in non-verbose mode
	}

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("yay upgrade failed: %s", stderr.String())
	}

	return nil
}

// SearchPackages searches for packages using yay
func (y *YayPackageManager) SearchPackages(searchTerm string) ([]SearchResult, error) {
	cmd := exec.Command("yay", "-Ss", searchTerm)

	// Capture stderr for better error messages
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("yay search failed: %s", stderr.String())
	}

	return y.parseSearchOutput(string(output)), nil
}

// GetInstalledPackages returns a list of installed packages
func (y *YayPackageManager) GetInstalledPackages() ([]string, error) {
	cmd := exec.Command("yay", "-Qq")

	// Capture stderr for better error messages
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("yay list failed: %s", stderr.String())
	}

	var packages []string
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		if line != "" {
			packages = append(packages, line)
		}
	}

	return packages, nil
}

// GetOutdatedPackages returns a list of outdated packages
func (y *YayPackageManager) GetOutdatedPackages() ([]string, error) {
	cmd := exec.Command("yay", "-Qu")

	// Capture stderr for better error messages
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	output, err := cmd.Output()
	if err != nil {
		// If no updates available, yay returns exit code 1 but no error output
		if cmd.ProcessState.ExitCode() == 1 && len(output) == 0 {
			return []string{}, nil
		}
		return nil, fmt.Errorf("yay outdated failed: %s", stderr.String())
	}

	var outdated []string
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		if line != "" {
			// Extract package name from the first field
			parts := strings.Fields(line)
			if len(parts) > 0 {
				outdated = append(outdated, parts[0])
			}
		}
	}

	return outdated, nil
}

// IsPackageInstalled checks if a package is installed
func (y *YayPackageManager) IsPackageInstalled(packageName string) (bool, error) {
	cmd := exec.Command("yay", "-Qq", packageName)
	err := cmd.Run()
	return err == nil, nil
}

// parseSearchOutput parses yay search output into SearchResult structs
func (y *YayPackageManager) parseSearchOutput(output string) []SearchResult {
	var results []SearchResult
	lines := strings.Split(output, "\n")

	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}

		// Check if this is a package line (starts with repo/package)
		if strings.Contains(line, "/") && !strings.HasPrefix(line, "    ") {
			parts := strings.SplitN(line, " ", 2)
			if len(parts) < 2 {
				continue
			}

			fullName := parts[0]
			versionInfo := strings.TrimSpace(parts[1])

			// Parse repository and package name
			nameParts := strings.Split(fullName, "/")
			if len(nameParts) != 2 {
				continue
			}

			repository := nameParts[0]
			packageName := nameParts[1]

			// Extract version from the version info
			version := ""
			if strings.Contains(versionInfo, " ") {
				versionParts := strings.Split(versionInfo, " ")
				if len(versionParts) > 0 {
					version = versionParts[0]
				}
			}

			// Get description from next line if available
			description := ""
			if i+1 < len(lines) {
				nextLine := strings.TrimSpace(lines[i+1])
				if strings.HasPrefix(nextLine, "    ") {
					description = strings.TrimSpace(nextLine)
					i++ // Skip the description line in next iteration
				}
			}

			// Check if package is installed (this is a simplified check)
			installed := false
			if strings.Contains(versionInfo, "[installed") {
				installed = true
			}

			result := SearchResult{
				Name:        packageName,
				Version:     version,
				Description: description,
				Repository:  repository,
				Installed:   installed,
				InConfig:    false, // Not tracked by owl config
			}

			results = append(results, result)
		}
	}

	return results
}

// isPackageGroupInstalled checks if a package group is already installed
func (y *YayPackageManager) isPackageGroupInstalled(groupName string) bool {
	// Get all packages in the group
	allCmd := exec.Command("yay", "-Sg", groupName)
	allOutput, err := allCmd.Output()
	if err != nil {
		return false
	}

	// Get installed packages from the group
	installedCmd := exec.Command("yay", "-Qg", groupName)
	installedOutput, err := installedCmd.Output()
	if err != nil {
		return false
	}

	// Parse all packages in the group
	allLines := strings.Split(strings.TrimSpace(string(allOutput)), "\n")
	if len(allLines) == 0 {
		return false
	}

	// Parse installed packages from the group
	installedLines := strings.Split(strings.TrimSpace(string(installedOutput)), "\n")

	// Extract package names from both lists
	allPackages := make(map[string]bool)
	for _, line := range allLines {
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			allPackages[parts[1]] = true // Second field is the package name
		}
	}

	installedPackages := make(map[string]bool)
	for _, line := range installedLines {
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			installedPackages[parts[1]] = true // Second field is the package name
		}
	}

	// Check if all packages in the group are installed
	for packageName := range allPackages {
		if !installedPackages[packageName] {
			return false // This package from the group is not installed
		}
	}

	return true // All packages in the group are installed
}

// Release cleans up resources (no-op for yay)
func (y *YayPackageManager) Release() {
	// No resources to clean up for yay
}
