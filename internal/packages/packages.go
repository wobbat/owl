package packages

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"owl/internal/constants"
	"owl/internal/types"
)

// EnsurePackageManagerReady ensures that the package manager is ready for use
func EnsurePackageManagerReady() error {
	// With go-alpm, we use libalpm for most operations but still need pacman for AUR building
	// Just verify that pacman and basic tools are available
	if _, err := exec.LookPath("pacman"); err != nil {
		return fmt.Errorf("pacman not found on system")
	}

	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("git not found - required for AUR packages")
	}

	if _, err := exec.LookPath("makepkg"); err != nil {
		return fmt.Errorf("makepkg not found - required for AUR packages")
	}

	return nil
}

// AnalyzePackages analyzes what packages need to be installed or removed
func AnalyzePackages(configuredPackages []string, verbose bool) ([]types.PackageAction, error) {
	managedPackages, err := loadManagedPackages()
	if err != nil {
		return nil, fmt.Errorf("failed to load managed packages: %w", err)
	}

	installedPackages, err := getInstalledPackages()
	if err != nil {
		return nil, fmt.Errorf("failed to get installed packages: %w", err)
	}

	var actions []types.PackageAction

	// Check which configured packages need to be installed
	for _, pkg := range configuredPackages {
		if !contains(installedPackages, pkg) {
			// Check if it's a package group that might be fully installed
			if isPackageGroupFullyInstalled(pkg, verbose) {
				actions = append(actions, types.PackageAction{
					Name:   pkg,
					Status: "skip",
				})
			} else {
				// Check if this package is already in managed packages but not installed
				// If so, the managed package cleanup will handle it
				_, isManaged := managedPackages.Packages[pkg]
				if !isManaged {
					// New package not in managed list, mark for installation
					actions = append(actions, types.PackageAction{
						Name:   pkg,
						Status: "install",
					})
				}
				// If it is managed but not installed, the second loop will handle it
			}
		} else {
			actions = append(actions, types.PackageAction{
				Name:   pkg,
				Status: "skip",
			})
		}
	}

	// Check which managed packages should be removed (no longer in config or no longer installed)
	for managedPkg := range managedPackages.Packages {
		if !contains(configuredPackages, managedPkg) && !contains(constants.DefaultProtectedPackages, managedPkg) {
			actions = append(actions, types.PackageAction{
				Name:   managedPkg,
				Status: "remove",
			})
		} else if contains(configuredPackages, managedPkg) && !contains(installedPackages, managedPkg) {
			// Package is in config and managed but not installed - this can happen if it was manually removed
			// For package groups, check if the group is fully installed instead
			if isPackageGroupFullyInstalled(managedPkg, verbose) {
				// Package group is fully installed, mark as skip
				actions = append(actions, types.PackageAction{
					Name:   managedPkg,
					Status: "skip",
				})
			} else {
				// Individual package or incomplete group - mark for installation
				actions = append(actions, types.PackageAction{
					Name:   managedPkg,
					Status: "install",
				})
			}
		}
	}

	return actions, nil
}

// InstallPackages installs a list of packages using ALPM/pacman
func InstallPackages(packages []string, verbose, quiet bool) error {
	if len(packages) == 0 {
		return nil
	}

	alpmMgr, err := NewALPMManager()
	if err != nil {
		return fmt.Errorf("failed to initialize ALPM manager: %w", err)
	}
	defer alpmMgr.Release()

	for _, pkg := range packages {
		if err := alpmMgr.InstallPackage(pkg, verbose && !quiet); err != nil {
			return fmt.Errorf("failed to install package %s: %w", pkg, err)
		}
	}

	return nil
}

// InstallPackagesWithProgress installs packages with progress reporting
func InstallPackagesWithProgress(packages []string, verbose, quiet bool, progressCallback func(string)) error {
	if len(packages) == 0 {
		return nil
	}

	alpmMgr, err := NewALPMManager()
	if err != nil {
		return fmt.Errorf("failed to initialize ALPM manager: %w", err)
	}
	defer alpmMgr.Release()

	for _, pkg := range packages {
		if err := alpmMgr.InstallPackageWithProgress(pkg, verbose && !quiet, progressCallback); err != nil {
			return fmt.Errorf("failed to install package %s: %w", pkg, err)
		}
	}

	return nil
}

// RemoveUnmanagedPackages removes packages that are no longer managed
func RemoveUnmanagedPackages(packages []string, quiet bool) error {
	if len(packages) == 0 {
		return nil
	}

	alpmMgr, err := NewALPMManager()
	if err != nil {
		return fmt.Errorf("failed to initialize ALPM manager: %w", err)
	}
	defer alpmMgr.Release()

	for _, pkg := range packages {
		if !quiet {
			fmt.Printf("Removing package: %s\n", pkg)
		}
		if err := alpmMgr.RemovePackage(pkg); err != nil {
			return fmt.Errorf("failed to remove package %s: %w", pkg, err)
		}
	}

	return nil
}

// UpdateManagedPackages updates the managed packages lock file
func UpdateManagedPackages(packages []string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get user home directory: %w", err)
	}

	stateDir := filepath.Join(homeDir, constants.OwlRootDir, constants.OwlStateDir)
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	managedLock, err := loadManagedPackages()
	if err != nil {
		// Create new lock file if it doesn't exist
		managedLock = &types.ManagedLock{
			SchemaVersion:     constants.SchemaVersion,
			Packages:          make(map[string]types.ManagedPackage),
			ProtectedPackages: constants.DefaultProtectedPackages,
		}
	}

	now := time.Now()

	// Update packages
	for _, pkg := range packages {
		// Get the installed version of the package using pacman
		var installedVersion string
		cmd := exec.Command("pacman", "-Q", pkg)
		output, err := cmd.Output()
		if err == nil {
			parts := strings.Fields(strings.TrimSpace(string(output)))
			if len(parts) >= 2 {
				installedVersion = parts[1]
			}
		}

		if existing, exists := managedLock.Packages[pkg]; exists {
			existing.LastSeen = now
			if installedVersion != "" {
				existing.InstalledVersion = installedVersion
			}
			managedLock.Packages[pkg] = existing
		} else {
			managedLock.Packages[pkg] = types.ManagedPackage{
				FirstManaged:     now,
				LastSeen:         now,
				InstalledVersion: installedVersion,
				AutoInstalled:    true,
			}
		}
	}

	return saveManagedPackages(managedLock)
}

// loadManagedPackages loads the managed packages lock file
func loadManagedPackages() (*types.ManagedLock, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user home directory: %w", err)
	}

	lockPath := filepath.Join(homeDir, constants.OwlRootDir, constants.OwlStateDir, constants.ManagedLockFile)

	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		// Return empty lock if file doesn't exist
		return &types.ManagedLock{
			SchemaVersion:     constants.SchemaVersion,
			Packages:          make(map[string]types.ManagedPackage),
			ProtectedPackages: constants.DefaultProtectedPackages,
		}, nil
	}

	file, err := os.Open(lockPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open managed lock file: %w", err)
	}
	defer file.Close()

	// Try to parse as JSON first (for compatibility with existing files)
	var managedLock types.ManagedLock
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&managedLock); err == nil {
		// Successfully parsed as JSON
		return &managedLock, nil
	}

	// If JSON parsing failed, try simple text format
	file.Seek(0, 0) // Reset file pointer
	managedLock = types.ManagedLock{
		SchemaVersion:     constants.SchemaVersion,
		Packages:          make(map[string]types.ManagedPackage),
		ProtectedPackages: constants.DefaultProtectedPackages,
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) >= 1 {
			packageName := parts[0]
			managedLock.Packages[packageName] = types.ManagedPackage{
				FirstManaged:  time.Now(),
				LastSeen:      time.Now(),
				AutoInstalled: true,
			}
		}
	}

	return &managedLock, scanner.Err()
}

// saveManagedPackages saves the managed packages lock file
func saveManagedPackages(managedLock *types.ManagedLock) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get user home directory: %w", err)
	}

	stateDir := filepath.Join(homeDir, constants.OwlRootDir, constants.OwlStateDir)
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	lockPath := filepath.Join(stateDir, constants.ManagedLockFile)
	file, err := os.Create(lockPath)
	if err != nil {
		return fmt.Errorf("failed to create managed lock file: %w", err)
	}
	defer file.Close()

	// Save as JSON for compatibility with existing format
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(managedLock)
}

// getInstalledPackages returns a list of all installed packages
func getInstalledPackages() ([]string, error) {
	// Use ALPM instead of pacman command
	alpmMgr, err := NewALPMManager()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize ALPM manager: %w", err)
	}
	defer alpmMgr.Release()

	return alpmMgr.GetInstalledPackages()
}

// UpgradeAllPackages upgrades all packages to their latest versions
func UpgradeAllPackages(verbose bool) error {
	alpmMgr, err := NewALPMManager()
	if err != nil {
		return fmt.Errorf("failed to initialize ALPM manager: %w", err)
	}
	defer alpmMgr.Release()

	return alpmMgr.UpgradeSystem(verbose)
}

// UpgradeAllPackagesWithProgress upgrades all packages with progress reporting
func UpgradeAllPackagesWithProgress(verbose bool, progressCallback func(string)) error {
	alpmMgr, err := NewALPMManager()
	if err != nil {
		return fmt.Errorf("failed to initialize ALPM manager: %w", err)
	}
	defer alpmMgr.Release()

	return alpmMgr.UpgradeSystemWithProgress(verbose, progressCallback)
}

// LoadManagedPackages loads the managed packages lock file (exported version)
func LoadManagedPackages() (*types.ManagedLock, error) {
	return loadManagedPackages()
}

// SaveManagedPackages saves the managed packages lock file (exported version)
func SaveManagedPackages(managedLock *types.ManagedLock) error {
	return saveManagedPackages(managedLock)
}

// RemoveFromManagedPackages removes packages from the managed packages lock file
func RemoveFromManagedPackages(packageNames []string) error {
	managedLock, err := loadManagedPackages()
	if err != nil {
		return fmt.Errorf("failed to load managed packages: %w", err)
	}

	// Remove the specified packages from the managed list
	for _, pkgName := range packageNames {
		delete(managedLock.Packages, pkgName)
	}

	return saveManagedPackages(managedLock)
}

// GetManagedPackages returns a list of all managed package names
func GetManagedPackages() ([]string, error) {
	managedLock, err := LoadManagedPackages()
	if err != nil {
		return nil, err
	}

	var packages []string
	for packageName := range managedLock.Packages {
		packages = append(packages, packageName)
	}

	return packages, nil
}

// isPackageGroupFullyInstalled checks if a package group is already fully installed
func isPackageGroupFullyInstalled(groupName string, verbose bool) bool {
	// Get all packages in the group
	allCmd := exec.Command("yay", "-Sg", groupName)
	allOutput, err := allCmd.Output()
	if err != nil {
		// Debug: print why we're returning false only if verbose is enabled
		if verbose {
			fmt.Printf("DEBUG: isPackageGroupFullyInstalled(%s) failed: %v\n", groupName, err)
		}
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

// contains checks if a slice contains a specific string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
