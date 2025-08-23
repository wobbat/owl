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

// EnsureYayInstalled ensures that yay package manager is installed
func EnsureYayInstalled() error {
	// Check if yay is already installed
	if _, err := exec.LookPath("yay"); err == nil {
		return nil
	}

	// Install yay using pacman
	fmt.Println("Installing yay package manager...")

	// First, update pacman keyring
	cmd := exec.Command("sudo", "pacman", "-Sy", "--noconfirm", "archlinux-keyring")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to update archlinux keyring: %w", err)
	}

	// Install git and base-devel if not present
	cmd = exec.Command("sudo", "pacman", "-S", "--noconfirm", "git", "base-devel")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to install git and base-devel: %w", err)
	}

	// Clone and install yay
	tempDir := "/tmp/yay-install"
	os.RemoveAll(tempDir)

	cmd = exec.Command("git", "clone", "https://aur.archlinux.org/yay.git", tempDir)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to clone yay repository: %w", err)
	}

	cmd = exec.Command("makepkg", "-si", "--noconfirm")
	cmd.Dir = tempDir
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to build and install yay: %w", err)
	}

	// Clean up
	os.RemoveAll(tempDir)

	fmt.Println("yay installed successfully")
	return nil
}

// AnalyzePackages analyzes what packages need to be installed or removed
func AnalyzePackages(configuredPackages []string) ([]types.PackageAction, error) {
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
			actions = append(actions, types.PackageAction{
				Name:   pkg,
				Status: "install",
			})
		} else {
			actions = append(actions, types.PackageAction{
				Name:   pkg,
				Status: "skip",
			})
		}
	}

	// Check which managed packages should be removed (no longer in config)
	for managedPkg := range managedPackages.Packages {
		if !contains(configuredPackages, managedPkg) && !contains(constants.DefaultProtectedPackages, managedPkg) {
			actions = append(actions, types.PackageAction{
				Name:   managedPkg,
				Status: "remove",
			})
		}
	}

	return actions, nil
}

// InstallPackages installs a list of packages using yay
func InstallPackages(packages []string, verbose, quiet bool) error {
	if len(packages) == 0 {
		return nil
	}

	args := []string{"-S", "--noconfirm"}
	args = append(args, packages...)

	cmd := exec.Command("yay", args...)

	if verbose {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	} else if quiet {
		cmd.Stdout = nil
		cmd.Stderr = nil
	}

	return cmd.Run()
}

// RemoveUnmanagedPackages removes packages that are no longer managed
func RemoveUnmanagedPackages(packages []string, quiet bool) error {
	if len(packages) == 0 {
		return nil
	}

	args := []string{"-Rns", "--noconfirm"}
	args = append(args, packages...)

	cmd := exec.Command("yay", args...)

	if quiet {
		cmd.Stdout = nil
		cmd.Stderr = nil
	} else {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

	return cmd.Run()
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
		if existing, exists := managedLock.Packages[pkg]; exists {
			existing.LastSeen = now
			managedLock.Packages[pkg] = existing
		} else {
			managedLock.Packages[pkg] = types.ManagedPackage{
				FirstManaged:  now,
				LastSeen:      now,
				AutoInstalled: true,
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
	cmd := exec.Command("pacman", "-Qq")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get installed packages: %w", err)
	}

	var packages []string
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		pkg := strings.TrimSpace(scanner.Text())
		if pkg != "" {
			packages = append(packages, pkg)
		}
	}

	return packages, scanner.Err()
}

// UpgradeAllPackages upgrades all packages to their latest versions
func UpgradeAllPackages(verbose bool) error {
	args := []string{"-Syu", "--noconfirm"}

	cmd := exec.Command("yay", args...)

	if verbose {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

	return cmd.Run()
}

// LoadManagedPackages loads the managed packages lock file (exported version)
func LoadManagedPackages() (*types.ManagedLock, error) {
	return loadManagedPackages()
}

// SaveManagedPackages saves the managed packages lock file (exported version)
func SaveManagedPackages(managedLock *types.ManagedLock) error {
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

// contains checks if a slice contains a specific string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
