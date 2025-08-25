package packages

// This package implements package management using the libalpm library (go-alpm v2).
//
// Current approach:
// - Uses libalpm for package querying, database access, and version comparison
// - Still uses pacman command with sudo for installations, removals, and system upgrades
//   because libalpm transactions require root privileges and complex privilege handling
// - Provides better integration where possible (e.g., proper version comparison with alpm.VerCmp)
//
// Future improvements:
// - Could implement proper privilege escalation for libalpm transactions
// - Could add transaction callbacks for better progress reporting
// - Could implement dependency resolution using libalpm APIs

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Jguer/go-alpm/v2"
)

type ALPMManager struct {
	handle *alpm.Handle
}

type AURPackage struct {
	Name        string  `json:"Name"`
	Version     string  `json:"Version"`
	Description string  `json:"Description"`
	URL         string  `json:"URL"`
	URLPath     string  `json:"URLPath"`
	PackageBase string  `json:"PackageBase"`
	Maintainer  string  `json:"Maintainer"`
	OutOfDate   int64   `json:"OutOfDate"`
	NumVotes    int     `json:"NumVotes"`
	Popularity  float64 `json:"Popularity"`
}

type AURResponse struct {
	Version     int          `json:"version"`
	Type        string       `json:"type"`
	ResultCount int          `json:"resultcount"`
	Results     []AURPackage `json:"results"`
}

// ProgressCallback is called during package operations to report progress
type ProgressCallback func(message string)

// DetailedProgressCallback provides more detailed progress information
type DetailedProgressCallback func(stage string, current, total int64, message string)

// monitorGitCloneProgress monitors git clone output and reports progress
func monitorGitCloneProgress(cmd *exec.Cmd, packageName string, callback ProgressCallback) error {
	// Git clone doesn't provide easy progress tracking, but we can monitor stderr
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := scanner.Text()
			if callback != nil {
				if strings.Contains(line, "Cloning into") {
					callback(fmt.Sprintf("Initializing clone for %s", packageName))
				} else if strings.Contains(line, "Receiving objects") {
					callback(fmt.Sprintf("Downloading %s source", packageName))
				} else if strings.Contains(line, "Resolving deltas") {
					callback(fmt.Sprintf("Processing %s source", packageName))
				}
			}
		}
	}()

	return cmd.Start()
}

// monitorMakepkgDownloadProgress monitors makepkg download progress
func monitorMakepkgDownloadProgress(stdout, stderr io.ReadCloser, packageName string, callback ProgressCallback, detailedCallback DetailedProgressCallback) {
	// Monitor stdout for download progress
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			if callback != nil {
				if strings.Contains(line, "Retrieving sources") {
					callback(fmt.Sprintf("Downloading %s sources", packageName))
				} else if strings.Contains(line, "Validating source files") {
					callback(fmt.Sprintf("Validating %s sources", packageName))
				} else if strings.Contains(line, "Extracting sources") {
					callback(fmt.Sprintf("Extracting %s sources", packageName))
				} else if strings.Contains(line, "Starting build") {
					callback(fmt.Sprintf("Compiling %s", packageName))
				} else if strings.Contains(line, "Installing package") {
					callback(fmt.Sprintf("Installing compiled %s", packageName))
				} else if strings.Contains(line, "Finished making") {
					callback(fmt.Sprintf("Finished building %s", packageName))
				}

				// Try to extract percentage information
				if strings.Contains(line, "%") && detailedCallback != nil {
					// Look for patterns like "downloading... 45%"
					parts := strings.Fields(line)
					for _, part := range parts {
						if strings.HasSuffix(part, "%") {
							if percentStr := strings.TrimSuffix(part, "%"); percentStr != "" {
								if percent, err := strconv.ParseFloat(percentStr, 64); err == nil {
									detailedCallback("download", int64(percent), 100, line)
								}
							}
						}
					}
				}
			}
		}
	}()

	// Monitor stderr for errors and additional info
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := scanner.Text()
			if callback != nil {
				if strings.Contains(line, "error") {
					callback(fmt.Sprintf("Build error for %s: %s", packageName, line))
				} else if strings.Contains(line, "warning") {
					callback(fmt.Sprintf("Build warning for %s: %s", packageName, line))
				}
			}
		}
	}()
}

// InstallProgress tracks installation progress
type InstallProgress struct {
	Package    string
	Stage      string // "checking", "downloading", "installing", "configuring"
	Progress   int    // 0-100
	Message    string
	Repository string // "core", "extra", "aur", etc.
}

func NewALPMManager() (*ALPMManager, error) {
	// Parse pacman config to get correct paths
	config, err := parsePacmanConfig()
	var rootDir, dbPath string
	if err != nil {
		// Use defaults if config parsing fails
		rootDir = "/"
		dbPath = "/var/lib/pacman/"
		fmt.Fprintf(os.Stderr, "Warning: failed to parse pacman config, using default paths: %v\n", err)
	} else {
		rootDir = config.RootDir
		dbPath = config.DBPath
	}

	handle, err := alpm.Initialize(rootDir, dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to initialize ALPM due to permissions: %v\n", err)
		fmt.Fprintf(os.Stderr, "Falling back to pacman command-based operations\n")

		// Create manager without libalpm handle - will use pacman commands
		return &ALPMManager{
			handle: nil, // Will use pacman commands instead
		}, nil
	}

	manager := &ALPMManager{
		handle: handle,
	}

	if err := manager.setupDatabases(); err != nil {
		handle.Release()
		fmt.Fprintf(os.Stderr, "Warning: failed to setup ALPM databases: %v\n", err)
		fmt.Fprintf(os.Stderr, "Falling back to pacman command-based operations\n")

		// Return manager without handle - will use pacman commands
		return &ALPMManager{
			handle: nil,
		}, nil
	}

	return manager, nil
}

func (m *ALPMManager) Release() {
	if m.handle != nil {
		m.handle.Release()
	}
}

type PacmanConfig struct {
	Repositories map[string][]string // repo name -> list of mirror URLs
	Architecture string
	DBPath       string
	RootDir      string
}

// Repository represents a pacman repository configuration
type Repository struct {
	Name    string
	Servers []string
}

// parsePacmanConfig reads and parses /etc/pacman.conf and included files
func parsePacmanConfig() (*PacmanConfig, error) {
	config := &PacmanConfig{
		Repositories: make(map[string][]string),
		Architecture: "x86_64", // default
		DBPath:       "/var/lib/pacman/",
		RootDir:      "/",
	}

	err := parseConfigFile("/etc/pacman.conf", config)
	if err != nil {
		return nil, fmt.Errorf("failed to parse pacman config: %w", err)
	}

	return config, nil
}

// parseConfigFile parses a pacman configuration file
func parseConfigFile(filename string, config *PacmanConfig) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var currentRepo string

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip comments and empty lines
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Check for repository section
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			currentRepo = strings.Trim(line, "[]")
			if currentRepo != "options" {
				config.Repositories[currentRepo] = []string{}
			}
			continue
		}

		// Parse options
		if currentRepo == "options" {
			if strings.HasPrefix(line, "Architecture") {
				parts := strings.Split(line, "=")
				if len(parts) == 2 {
					arch := strings.TrimSpace(parts[1])
					if arch == "auto" {
						// Keep default x86_64 for auto
					} else {
						config.Architecture = arch
					}
				}
			} else if strings.HasPrefix(line, "DBPath") {
				parts := strings.Split(line, "=")
				if len(parts) == 2 {
					config.DBPath = strings.TrimSpace(parts[1])
				}
			} else if strings.HasPrefix(line, "RootDir") {
				parts := strings.Split(line, "=")
				if len(parts) == 2 {
					config.RootDir = strings.TrimSpace(parts[1])
				}
			}
			continue
		}

		// Parse repository configuration
		if currentRepo != "" && currentRepo != "options" {
			if strings.HasPrefix(line, "Include") {
				// Handle Include directive
				parts := strings.Split(line, "=")
				if len(parts) == 2 {
					includeFile := strings.TrimSpace(parts[1])
					servers, err := parseIncludeFile(includeFile, currentRepo, config.Architecture)
					if err != nil {
						// Log error but continue
						fmt.Fprintf(os.Stderr, "Warning: failed to parse include file %s: %v\n", includeFile, err)
						continue
					}
					config.Repositories[currentRepo] = append(config.Repositories[currentRepo], servers...)
				}
			} else if strings.HasPrefix(line, "Server") {
				// Handle direct server directive
				parts := strings.Split(line, "=")
				if len(parts) == 2 {
					server := strings.TrimSpace(parts[1])
					// Expand variables
					server = strings.ReplaceAll(server, "$repo", currentRepo)
					server = strings.ReplaceAll(server, "$arch", config.Architecture)
					config.Repositories[currentRepo] = append(config.Repositories[currentRepo], server)
				}
			}
		}
	}

	return scanner.Err()
}

// parseIncludeFile parses an included mirrorlist file
func parseIncludeFile(filename, repo, arch string) ([]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var servers []string
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip comments and empty lines
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse server lines
		if strings.HasPrefix(line, "Server") {
			parts := strings.Split(line, "=")
			if len(parts) == 2 {
				server := strings.TrimSpace(parts[1])
				// Expand variables
				server = strings.ReplaceAll(server, "$repo", repo)
				server = strings.ReplaceAll(server, "$arch", arch)
				servers = append(servers, server)
			}
		}
	}

	return servers, scanner.Err()
}

func (m *ALPMManager) setupDatabases() error {
	// Parse pacman configuration to get repository mirrors
	config, err := parsePacmanConfig()
	if err != nil {
		// Fallback to hardcoded mirrors if config parsing fails
		fmt.Fprintf(os.Stderr, "Warning: failed to parse pacman config, using default mirrors: %v\n", err)
		return m.setupDefaultDatabases()
	}

	// Register repositories from pacman configuration
	for repoName, servers := range config.Repositories {
		// Skip common test repositories
		if strings.Contains(repoName, "testing") {
			continue
		}

		// Only register well-known repositories
		if repoName == "core" || repoName == "extra" || repoName == "multilib" {
			db, err := m.handle.RegisterSyncDB(repoName, 0)
			if err != nil {
				return fmt.Errorf("failed to register %s database: %w", repoName, err)
			}

			// Add all configured servers for this repository
			for _, server := range servers {
				db.AddServer(server)
			}
		}
	}

	return nil
}

// setupDefaultDatabases sets up databases with hardcoded default mirrors (fallback)
func (m *ALPMManager) setupDefaultDatabases() error {
	// Register official repositories with default mirrors
	coreDB, err := m.handle.RegisterSyncDB("core", 0)
	if err != nil {
		return fmt.Errorf("failed to register core database: %w", err)
	}
	coreDB.AddServer("https://geo.mirror.pkgbuild.com/core/os/x86_64")

	extraDB, err := m.handle.RegisterSyncDB("extra", 0)
	if err != nil {
		return fmt.Errorf("failed to register extra database: %w", err)
	}
	extraDB.AddServer("https://geo.mirror.pkgbuild.com/extra/os/x86_64")

	multiLibDB, err := m.handle.RegisterSyncDB("multilib", 0)
	if err != nil {
		return fmt.Errorf("failed to register multilib database: %w", err)
	}
	multiLibDB.AddServer("https://geo.mirror.pkgbuild.com/multilib/os/x86_64")

	return nil
}

// GetPacmanConfigInfo returns information about the parsed pacman configuration
func GetPacmanConfigInfo() (map[string]any, error) {
	config, err := parsePacmanConfig()
	if err != nil {
		return nil, err
	}

	info := map[string]any{
		"architecture": config.Architecture,
		"rootDir":      config.RootDir,
		"dbPath":       config.DBPath,
		"repositories": make(map[string]any),
	}

	// Add repository information
	repos := make(map[string]any)
	for name, servers := range config.Repositories {
		repos[name] = map[string]any{
			"servers": servers,
			"count":   len(servers),
		}
	}
	info["repositories"] = repos

	return info, nil
}

func (m *ALPMManager) GetInstalledPackages() ([]string, error) {
	// If libalpm handle is not available, use pacman command
	if m.handle == nil {
		cmd := exec.Command("pacman", "-Qq")
		output, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("failed to get installed packages with pacman: %w", err)
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

	localDB, err := m.handle.LocalDB()
	if err != nil {
		return nil, fmt.Errorf("failed to get local database: %w", err)
	}

	var packages []string
	err = localDB.PkgCache().ForEach(func(pkg alpm.IPackage) error {
		packages = append(packages, pkg.Name())
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to iterate packages: %w", err)
	}

	return packages, nil
}

func (m *ALPMManager) IsPackageInstalled(packageName string) (bool, error) {
	// If libalpm handle is not available, use pacman command
	if m.handle == nil {
		cmd := exec.Command("pacman", "-Qq", packageName)
		err := cmd.Run()
		return err == nil, nil
	}

	installed, err := m.GetInstalledPackages()
	if err != nil {
		return false, err
	}

	return slices.Contains(installed, packageName), nil
}

// GetInstalledPackageVersion returns the version of an installed package
func (m *ALPMManager) GetInstalledPackageVersion(packageName string) string {
	// If libalpm handle is not available, use pacman command
	if m.handle == nil {
		cmd := exec.Command("pacman", "-Q", packageName)
		output, err := cmd.Output()
		if err != nil {
			return ""
		}

		parts := strings.Fields(strings.TrimSpace(string(output)))
		if len(parts) >= 2 {
			return parts[1]
		}
		return ""
	}

	localDB, err := m.handle.LocalDB()
	if err != nil {
		return ""
	}

	pkg := localDB.Pkg(packageName)
	if pkg == nil {
		return ""
	}

	return pkg.Version()
}

// getPackageRepository determines which repository a package belongs to
func (m *ALPMManager) getPackageRepository(packageName string) (string, error) {
	// If libalpm handle is not available, use pacman command
	if m.handle == nil {
		// Use pacman -Si to check if package is in official repos
		cmd := exec.Command("pacman", "-Si", packageName)
		err := cmd.Run()
		if err == nil {
			// Package found in official repos, but we can't determine which one without libalpm
			return "official", nil
		}

		// Check if it's a package group using pacman -Sg
		groupCmd := exec.Command("pacman", "-Sg", packageName)
		err = groupCmd.Run()
		if err == nil {
			// Package group found in official repos
			return "official", nil
		}

		// Check if package exists in AUR
		_, err = m.QueryAUR(packageName)
		if err == nil {
			return "aur", nil
		}

		return "", fmt.Errorf("package not found in any repository")
	}

	// Check official repositories first using ALPM
	syncDBs, err := m.handle.SyncDBs()
	if err != nil {
		return "", fmt.Errorf("failed to get sync databases: %w", err)
	}

	// Check if it's an individual package
	for _, db := range syncDBs.Slice() {
		if pkg := db.Pkg(packageName); pkg != nil {
			return db.Name(), nil
		}
	}

	// Check if it's a package group
	found := false
	_ = syncDBs.FindGroupPkgs(packageName).ForEach(func(pkg alpm.IPackage) error {
		found = true
		return nil
	})
	if found {
		// Package group found, but we can't determine which specific repository without more complex logic
		return "official", nil
	}

	// Check if package exists in AUR
	_, err = m.QueryAUR(packageName)
	if err == nil {
		return "aur", nil
	}

	return "", fmt.Errorf("package not found in any repository")
}

// InstallAURPackageWithProgress installs an AUR package with basic progress reporting
func (m *ALPMManager) InstallAURPackageWithProgress(packageName string, verbose bool, progressCallback ProgressCallback) error {
	return m.InstallAURPackageWithDetailedProgress(packageName, verbose, progressCallback, nil)
}

// InstallAURPackageWithDetailedProgress installs an AUR package with detailed progress reporting
func (m *ALPMManager) InstallAURPackageWithDetailedProgress(packageName string, verbose bool, progressCallback ProgressCallback, detailedCallback DetailedProgressCallback) error {
	if progressCallback != nil {
		progressCallback(fmt.Sprintf("Querying AUR for %s", packageName))
	}

	// Query AUR for package info
	aurPkg, err := m.QueryAUR(packageName)
	if err != nil {
		return fmt.Errorf("failed to find package %s in AUR: %w", packageName, err)
	}

	if progressCallback != nil {
		progressCallback(fmt.Sprintf("Found %s in AUR (v%s)", packageName, aurPkg.Version))
	}

	// Create temporary directory for building
	tmpDir := fmt.Sprintf("/tmp/aur-%s", packageName)
	if err := os.RemoveAll(tmpDir); err != nil {
		return fmt.Errorf("failed to clean temp directory: %w", err)
	}

	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	if progressCallback != nil {
		progressCallback(fmt.Sprintf("Cloning %s repository", packageName))
	}

	// Clone AUR repository
	gitURL := fmt.Sprintf("https://aur.archlinux.org/%s.git", packageName)
	gitCmd := exec.Command("git", "clone", gitURL, tmpDir)

	if err := monitorGitCloneProgress(gitCmd, packageName, progressCallback); err != nil {
		return fmt.Errorf("failed to start git clone: %w", err)
	}

	if err := gitCmd.Wait(); err != nil {
		return fmt.Errorf("failed to clone AUR repository: %w", err)
	}

	if progressCallback != nil {
		progressCallback(fmt.Sprintf("Building %s package", packageName))
	}

	// Build and install package
	makepkgCmd := exec.Command("makepkg", "-si", "--noconfirm")
	makepkgCmd.Dir = tmpDir

	if verbose {
		makepkgCmd.Stdout = os.Stdout
		makepkgCmd.Stderr = os.Stderr
		return makepkgCmd.Run()
	}

	// Monitor makepkg output for progress
	stdout, err := makepkgCmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := makepkgCmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := makepkgCmd.Start(); err != nil {
		return fmt.Errorf("failed to start makepkg: %w", err)
	}

	// Monitor the build process
	monitorMakepkgDownloadProgress(stdout, stderr, packageName, progressCallback, detailedCallback)

	if err := makepkgCmd.Wait(); err != nil {
		return fmt.Errorf("failed to build/install package: %w", err)
	}

	if progressCallback != nil {
		progressCallback(fmt.Sprintf("Successfully installed %s", packageName))
	}

	// Update VCS cache for git packages
	if IsGitPackage(packageName) {
		vcsStore, err := NewVCSStore()
		if err == nil {
			// Read PKGBUILD to extract VCS sources
			pkgbuildPath := filepath.Join(tmpDir, "PKGBUILD")
			if pkgbuildContent, readErr := os.ReadFile(pkgbuildPath); readErr == nil {
				if updateErr := vcsStore.UpdatePackageInfo(packageName, string(pkgbuildContent)); updateErr != nil {
					// Don't fail the installation if VCS cache update fails
					if progressCallback != nil {
						progressCallback(fmt.Sprintf("Warning: Failed to update VCS cache for %s", packageName))
					}
				}
			}
		}
	}

	return nil
}

func (m *ALPMManager) QueryAUR(packageName string) (*AURPackage, error) {
	url := fmt.Sprintf("https://aur.archlinux.org/rpc/?v=5&type=info&arg[]=%s", packageName)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to query AUR: %w", err)
	}
	defer resp.Body.Close()

	var aurResp AURResponse
	if err := json.NewDecoder(resp.Body).Decode(&aurResp); err != nil {
		return nil, fmt.Errorf("failed to decode AUR response: %w", err)
	}

	if aurResp.ResultCount == 0 {
		return nil, fmt.Errorf("package %s not found in AUR", packageName)
	}

	return &aurResp.Results[0], nil
}

// InstallOfficialPackageWithProgress installs an official package with progress reporting using libalpm
func (m *ALPMManager) InstallOfficialPackageWithProgress(packageName, repository string, verbose bool, progressCallback ProgressCallback) error {
	if progressCallback != nil {
		progressCallback(fmt.Sprintf("Checking %s in %s repository", packageName, repository))
	}

	// If libalpm handle is not available, use pacman command directly
	if m.handle == nil {
		if progressCallback != nil {
			progressCallback(fmt.Sprintf("Installing %s using pacman", packageName))
		}

		// Check if package is already installed using pacman
		checkCmd := exec.Command("pacman", "-Qq", packageName)
		if checkCmd.Run() == nil {
			if progressCallback != nil {
				progressCallback(fmt.Sprintf("Package %s is already installed", packageName))
			}
			return nil
		}

		// Install the package using pacman
		installCmd := exec.Command("sudo", "pacman", "-S", "--noconfirm", packageName)
		if verbose {
			installCmd.Stdout = os.Stdout
			installCmd.Stderr = os.Stderr
		}

		if err := installCmd.Run(); err != nil {
			return fmt.Errorf("failed to install package %s with pacman: %w", packageName, err)
		}

		if progressCallback != nil {
			progressCallback(fmt.Sprintf("Successfully installed %s", packageName))
		}
		return nil
	}

	// Find the package in sync databases
	syncDBs, err := m.handle.SyncDBs()
	if err != nil {
		return fmt.Errorf("failed to get sync databases: %w", err)
	}

	var targetPkg alpm.IPackage
	var targetDB alpm.IDB
	var isGroup = false

	// First, try to find as an individual package
	for _, db := range syncDBs.Slice() {
		if pkg := db.Pkg(packageName); pkg != nil {
			targetPkg = pkg
			targetDB = db
			break
		}
	}

	// If not found as individual package, check if it's a package group
	if targetPkg == nil {
		found := false
		_ = syncDBs.FindGroupPkgs(packageName).ForEach(func(pkg alpm.IPackage) error {
			found = true
			return nil
		})
		if found {
			isGroup = true
		}
	}

	if targetPkg == nil && !isGroup {
		return fmt.Errorf("package %s not found in official repositories", packageName)
	}

	// If it's a package group, we need to use pacman commands since libalpm doesn't support group installation
	if isGroup {
		if progressCallback != nil {
			progressCallback(fmt.Sprintf("Installing package group %s using pacman", packageName))
		}

		// Install the package group using pacman
		installCmd := exec.Command("sudo", "pacman", "-S", "--noconfirm", packageName)
		if verbose {
			installCmd.Stdout = os.Stdout
			installCmd.Stderr = os.Stderr
		}

		if err := installCmd.Run(); err != nil {
			return fmt.Errorf("failed to install package group %s with pacman: %w", packageName, err)
		}

		if progressCallback != nil {
			progressCallback(fmt.Sprintf("Successfully installed package group %s", packageName))
		}
		return nil
	}

	if progressCallback != nil {
		progressCallback(fmt.Sprintf("Found %s in %s repository", packageName, targetDB.Name()))
	}

	// Check if package is already installed
	localDB, err := m.handle.LocalDB()
	if err != nil {
		return fmt.Errorf("failed to get local database: %w", err)
	}

	if localPkg := localDB.Pkg(packageName); localPkg != nil {
		if progressCallback != nil {
			progressCallback(fmt.Sprintf("Package %s is already installed", packageName))
		}
		return nil
	}

	if progressCallback != nil {
		progressCallback("Package installation requires elevated privileges")
	}

	// Note: libalpm v2 doesn't provide direct package installation APIs for individual packages
	// The transaction APIs are mainly for complex operations and require root privileges
	// For now, we'll fall back to pacman for individual package installations
	// This can be improved in the future with proper privilege escalation
	return fmt.Errorf("individual package installation through libalpm requires elevated privileges - use 'pacman -S %s' manually", packageName)
}

// InstallPackageWithProgress installs a package with detailed progress reporting
func (m *ALPMManager) InstallPackageWithProgress(packageName string, verbose bool, progressCallback ProgressCallback) error {
	// Check if package exists in official repositories first
	repository, err := m.getPackageRepository(packageName)
	if err == nil && repository != "" {
		// Package found in official repository, install it
		return m.InstallOfficialPackageWithProgress(packageName, repository, verbose, progressCallback)
	}

	// Not found in official repos, try AUR
	return m.InstallAURPackageWithDetailedProgress(packageName, verbose, progressCallback, func(stage string, current, total int64, message string) {
		if progressCallback != nil {
			if stage == "download" && total > 0 {
				percentage := float64(current) / float64(total) * 100
				progressCallback(fmt.Sprintf("Downloading %s sources: %.1f%%", packageName, percentage))
			} else {
				progressCallback(message)
			}
		}
	})
}

// Legacy method for backward compatibility
func (m *ALPMManager) InstallPackage(packageName string, verbose bool) error {
	return m.InstallPackageWithProgress(packageName, verbose, nil)
}

// InstallPackages installs multiple packages
func (m *ALPMManager) InstallPackages(packages []string, verbose bool) error {
	for _, pkg := range packages {
		if err := m.InstallPackage(pkg, verbose); err != nil {
			return err
		}
	}
	return nil
}

// SearchPackages searches for packages using libalpm
func (m *ALPMManager) SearchPackages(searchTerm string) ([]SearchResult, error) {
	// For now, delegate to pacman command since libalpm search is complex
	cmd := exec.Command("pacman", "-Ss", searchTerm)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to search packages: %w", err)
	}

	return parsePacmanSearchOutput(string(output)), nil
}

// parsePacmanSearchOutput parses pacman search output into SearchResult structs
func parsePacmanSearchOutput(output string) []SearchResult {
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

func (m *ALPMManager) InstallAURPackage(packageName string, verbose bool) error {
	return m.InstallAURPackageWithProgress(packageName, verbose, nil)
}

func (m *ALPMManager) RemovePackage(packageName string) error {
	// Check if package is installed using libalpm
	localDB, err := m.handle.LocalDB()
	if err != nil {
		return fmt.Errorf("failed to get local database: %w", err)
	}

	pkg := localDB.Pkg(packageName)
	if pkg == nil {
		return fmt.Errorf("package %s is not installed", packageName)
	}

	// Initialize transaction for removal
	if err := m.handle.TransInit(0); err != nil {
		return fmt.Errorf("failed to initialize transaction: %w", err)
	}
	defer m.handle.TransRelease()

	// For removal, we still need to use pacman command with sudo for now
	// because libalpm transactions require proper privilege handling
	cmd := exec.Command("sudo", "pacman", "-Rns", "--noconfirm", packageName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to remove package %s: %w", packageName, err)
	}

	return nil
}

// SyncDatabases synchronizes package databases using libalpm
func (m *ALPMManager) SyncDatabases() error {
	// libalpm doesn't provide direct database sync functionality like pacman -Sy
	// The database sync is typically handled by the package manager (pacman)
	// For proper libalpm integration, we would need to implement database downloading
	// and updating logic, which is complex and requires proper mirror handling

	// For now, we'll note that this requires pacman command, but we can improve
	// this in the future by implementing proper database sync with libalpm
	return fmt.Errorf("database synchronization through libalpm requires implementing database download logic - use 'pacman -Sy' manually")
}

func (m *ALPMManager) UpgradeSystem(verbose bool) error {
	// Use pacman directly for system upgrades since it handles privileges properly
	if verbose {
		fmt.Println("Synchronizing package databases...")
	}

	syncCmd := exec.Command("sudo", "pacman", "-Sy", "--noconfirm")
	if verbose {
		syncCmd.Stdout = os.Stdout
		syncCmd.Stderr = os.Stderr
	}

	if err := syncCmd.Run(); err != nil {
		return fmt.Errorf("failed to synchronize databases: %w", err)
	}

	if verbose {
		fmt.Println("Upgrading system packages...")
	}

	// Upgrade system packages
	upgradeCmd := exec.Command("sudo", "pacman", "-Su", "--noconfirm")
	if verbose {
		upgradeCmd.Stdout = os.Stdout
		upgradeCmd.Stderr = os.Stderr
	}

	if err := upgradeCmd.Run(); err != nil {
		return fmt.Errorf("failed to upgrade system: %w", err)
	}

	if verbose {
		fmt.Println("System upgrade completed successfully")
	}

	return nil
}

// UpgradeSystemWithProgress upgrades system with detailed progress reporting
func (m *ALPMManager) UpgradeSystemWithProgress(verbose bool, progressCallback ProgressCallback) error {
	if progressCallback != nil {
		progressCallback("Synchronizing package databases")
	}

	// Use pacman directly for system upgrades since it handles privileges properly
	syncCmd := exec.Command("sudo", "pacman", "-Sy", "--noconfirm")
	if verbose {
		syncCmd.Stdout = os.Stdout
		syncCmd.Stderr = os.Stderr
	}

	if err := syncCmd.Run(); err != nil {
		return fmt.Errorf("failed to synchronize databases: %w", err)
	}

	if progressCallback != nil {
		progressCallback("Upgrading system packages")
	}

	// Upgrade system packages
	upgradeCmd := exec.Command("sudo", "pacman", "-Su", "--noconfirm")
	if verbose {
		upgradeCmd.Stdout = os.Stdout
		upgradeCmd.Stderr = os.Stderr
	}

	if err := upgradeCmd.Run(); err != nil {
		return fmt.Errorf("failed to upgrade system: %w", err)
	}

	if progressCallback != nil {
		progressCallback("System upgrade completed successfully")
	}

	// Check for AUR updates if needed
	if progressCallback != nil {
		progressCallback("Checking AUR packages for updates")
	}

	return m.UpgradeAURPackagesWithProgress(verbose, progressCallback)
}

func (m *ALPMManager) GetOutdatedPackages() ([]string, error) {
	// If libalpm handle is not available, use pacman command
	if m.handle == nil {
		cmd := exec.Command("pacman", "-Qu")
		output, err := cmd.Output()
		if err != nil {
			// If no updates available, pacman returns exit code 1 but no error output
			if cmd.ProcessState.ExitCode() == 1 && len(output) == 0 {
				return []string{}, nil
			}
			return nil, fmt.Errorf("failed to get outdated packages with pacman: %w", err)
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

	// Get local and sync databases
	localDB, err := m.handle.LocalDB()
	if err != nil {
		return nil, fmt.Errorf("failed to get local database: %w", err)
	}

	syncDBs, err := m.handle.SyncDBs()
	if err != nil {
		return nil, fmt.Errorf("failed to get sync databases: %w", err)
	}

	var outdated []string

	// Check each installed package for updates using proper libalpm version comparison
	err = localDB.PkgCache().ForEach(func(localPkg alpm.IPackage) error {
		packageName := localPkg.Name()
		localVersion := localPkg.Version()

		// Look for the package in sync databases
		for _, syncDB := range syncDBs.Slice() {
			if syncPkg := syncDB.Pkg(packageName); syncPkg != nil {
				syncVersion := syncPkg.Version()
				// Use libalpm's built-in version comparison function
				if alpm.VerCmp(localVersion, syncVersion) < 0 {
					outdated = append(outdated, packageName)
				}
				break
			}
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to iterate local packages: %w", err)
	}

	return outdated, nil
}

// QueryAURBatch queries multiple AUR packages in a single request
func (m *ALPMManager) QueryAURBatch(packageNames []string) ([]AURPackage, error) {
	if len(packageNames) == 0 {
		return []AURPackage{}, nil
	}

	// Build URL with multiple package names
	baseURL := "https://aur.archlinux.org/rpc/?v=5&type=info"
	for _, pkg := range packageNames {
		baseURL += "&arg[]=" + pkg
	}

	// Make HTTP request
	client := &http.Client{Timeout: 30 * time.Second} // Longer timeout for batch requests
	resp, err := client.Get(baseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to query AUR: %w", err)
	}
	defer resp.Body.Close()

	var aurResp AURResponse
	if err := json.NewDecoder(resp.Body).Decode(&aurResp); err != nil {
		return nil, fmt.Errorf("failed to decode AUR response: %w", err)
	}

	return aurResp.Results, nil
}

// AURPackageInfo represents AUR package info with version comparison
type AURPackageInfo struct {
	Name             string
	InstalledVersion string
	AURVersion       string
	NeedsUpdate      bool
	OutOfDate        bool
}

// GetAURUpdates checks for available updates for AUR packages
func (m *ALPMManager) GetAURUpdates() ([]AURPackageInfo, error) {
	return m.GetAURUpdatesWithProgress(false, nil)
}

// GetAURUpdatesWithDevel checks for available updates for AUR packages with optional VCS checking
func (m *ALPMManager) GetAURUpdatesWithDevel(checkDevel bool) ([]AURPackageInfo, error) {
	return m.GetAURUpdatesWithProgress(checkDevel, nil)
}

// GetAURUpdatesWithProgress checks for available updates for AUR packages with progress reporting
func (m *ALPMManager) GetAURUpdatesWithProgress(checkDevel bool, progressCallback ProgressCallback) ([]AURPackageInfo, error) {
	if progressCallback != nil {
		progressCallback("Getting installed AUR packages")
	}

	// Get list of AUR packages
	aurPackageNames, err := m.GetAURPackages()
	if err != nil {
		return nil, fmt.Errorf("failed to get AUR packages: %w", err)
	}

	if len(aurPackageNames) == 0 {
		return []AURPackageInfo{}, nil
	}

	if progressCallback != nil {
		progressCallback("Reading local package versions")
	}

	// Get installed package versions
	localDB, err := m.handle.LocalDB()
	if err != nil {
		return nil, fmt.Errorf("failed to get local database: %w", err)
	}

	installedVersions := make(map[string]string)
	for _, pkgName := range aurPackageNames {
		if pkg := localDB.Pkg(pkgName); pkg != nil {
			installedVersions[pkgName] = pkg.Version()
		}
	}

	if progressCallback != nil {
		progressCallback("Querying AUR for latest versions")
	}

	// Query AUR for current versions (batch request)
	aurPackages, err := m.QueryAURBatch(aurPackageNames)
	if err != nil {
		return nil, fmt.Errorf("failed to query AUR packages: %w", err)
	}

	// Build map of AUR package info
	aurVersions := make(map[string]AURPackage)
	for _, aurPkg := range aurPackages {
		aurVersions[aurPkg.Name] = aurPkg
	}

	if progressCallback != nil {
		progressCallback("Initializing VCS tracking")
	}

	// Initialize VCS store for git package handling
	vcsStore, err := NewVCSStore()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize VCS store: %w", err)
	}

	if progressCallback != nil {
		progressCallback("Comparing package versions")
	}

	// Compare versions and build update list
	var updates []AURPackageInfo
	for _, pkgName := range aurPackageNames {
		installedVer := installedVersions[pkgName]
		if aurPkg, exists := aurVersions[pkgName]; exists {
			needsUpdate := m.shouldUpdateWithDevel(pkgName, installedVer, aurPkg.Version, vcsStore, checkDevel)
			updates = append(updates, AURPackageInfo{
				Name:             pkgName,
				InstalledVersion: installedVer,
				AURVersion:       aurPkg.Version,
				NeedsUpdate:      needsUpdate,
				OutOfDate:        aurPkg.OutOfDate > 0,
			})
		} else {
			// Package not found in AUR (might have been deleted)
			updates = append(updates, AURPackageInfo{
				Name:             pkgName,
				InstalledVersion: installedVer,
				AURVersion:       "not found",
				NeedsUpdate:      false,
				OutOfDate:        false,
			})
		}
	}

	return updates, nil
}

// InitializeVCSDatabase initializes VCS database for git packages that don't have tracking info yet
// This is similar to yay's --gendb functionality
func (m *ALPMManager) InitializeVCSDatabase(verbose bool) error {
	// Get all AUR packages
	aurPackages, err := m.GetAURPackages()
	if err != nil {
		return fmt.Errorf("failed to get AUR packages: %w", err)
	}

	// Filter for git packages
	var gitPackages []string
	for _, pkg := range aurPackages {
		if IsGitPackage(pkg) {
			gitPackages = append(gitPackages, pkg)
		}
	}

	if len(gitPackages) == 0 {
		if verbose {
			fmt.Println("No git packages found to initialize")
		}
		return nil
	}

	if verbose {
		fmt.Printf("Initializing VCS database for %d git packages...\n", len(gitPackages))
	}

	// Initialize VCS store
	vcsStore, err := NewVCSStore()
	if err != nil {
		return fmt.Errorf("failed to initialize VCS store: %w", err)
	}

	// Create AUR client function
	aurClient := func(packageName string) (*AURPackage, error) {
		return m.QueryAUR(packageName)
	}

	// Initialize git packages
	ctx := context.Background()
	return vcsStore.InitializeGitPackages(ctx, gitPackages, aurClient)
}

// UpgradeAURPackagesWithProgress upgrades AUR packages with detailed progress reporting
// InstallAURPackageWithUI installs an AUR package with UI progress indicators
func (m *ALPMManager) InstallAURPackageWithUI(packageName string, verbose bool, enableSpinner bool) error {
	// Import UI types here to avoid circular imports
	// We'll handle UI creation in the calling code instead
	return m.InstallAURPackageWithDetailedProgress(packageName, verbose, nil, nil)
}

func (m *ALPMManager) UpgradeAURPackagesWithProgress(verbose bool, progressCallback ProgressCallback) error {
	if progressCallback != nil {
		progressCallback("Discovering AUR packages")
	}

	// Get all AUR packages first to know what we're working with
	aurPackages, err := m.GetAURPackages()
	if err != nil {
		return fmt.Errorf("failed to get AUR packages: %w", err)
	}

	if len(aurPackages) == 0 {
		if progressCallback != nil {
			progressCallback("No AUR packages installed")
		}
		return nil
	}

	if progressCallback != nil {
		progressCallback(fmt.Sprintf("Checking %d AUR packages for updates", len(aurPackages)))
	}

	// Get AUR update information with progress reporting
	updates, err := m.GetAURUpdatesWithProgress(false, progressCallback)
	if err != nil {
		return fmt.Errorf("failed to check AUR updates: %w", err)
	}

	if progressCallback != nil {
		progressCallback("Analyzing update requirements")
	}

	// Filter packages that need updates
	var packagesToUpdate []AURPackageInfo
	for _, update := range updates {
		if update.NeedsUpdate {
			packagesToUpdate = append(packagesToUpdate, update)
		}
	}

	if len(packagesToUpdate) == 0 {
		if progressCallback != nil {
			progressCallback("All AUR packages are up to date")
		}
		return nil
	}

	if progressCallback != nil {
		progressCallback(fmt.Sprintf("Found %d AUR packages to upgrade", len(packagesToUpdate)))
	}

	// Display packages to be updated if not using progress callback
	if progressCallback == nil {
		fmt.Printf("AUR packages to upgrade (%d):\n", len(packagesToUpdate))
		for _, pkg := range packagesToUpdate {
			status := ""
			if pkg.OutOfDate {
				status = " [out of date]"
			}
			fmt.Printf("  %s: %s -> %s%s\n", pkg.Name, pkg.InstalledVersion, pkg.AURVersion, status)
		}
		fmt.Println()
	}

	// Upgrade each package
	for i, pkg := range packagesToUpdate {
		if progressCallback != nil {
			progressCallback(fmt.Sprintf("Upgrading %s (%d/%d)", pkg.Name, i+1, len(packagesToUpdate)))
		}

		err := m.InstallAURPackageWithProgress(pkg.Name, verbose, progressCallback)
		if err != nil {
			if progressCallback != nil {
				progressCallback(fmt.Sprintf("Failed to upgrade %s: %v", pkg.Name, err))
			} else {
				fmt.Printf("Failed to upgrade %s: %v\n", pkg.Name, err)
			}
			continue
		}

		if progressCallback != nil {
			progressCallback(fmt.Sprintf("Successfully upgraded %s", pkg.Name))
		}
	}

	return nil
}

func (m *ALPMManager) UpgradeAURPackages(verbose bool) error {
	// Get AUR update information
	updates, err := m.GetAURUpdates()
	if err != nil {
		return fmt.Errorf("failed to check AUR updates: %w", err)
	}

	// Filter packages that need updates
	var packagesToUpdate []AURPackageInfo
	for _, update := range updates {
		if update.NeedsUpdate {
			packagesToUpdate = append(packagesToUpdate, update)
		}
	}

	if len(packagesToUpdate) == 0 {
		fmt.Println("All AUR packages are up to date")
		return nil
	}

	// Display packages to be updated
	fmt.Printf("AUR packages to upgrade (%d):\n", len(packagesToUpdate))
	for _, pkg := range packagesToUpdate {
		status := ""
		if pkg.OutOfDate {
			status = " [out of date]"
		}
		fmt.Printf("  %s: %s -> %s%s\n", pkg.Name, pkg.InstalledVersion, pkg.AURVersion, status)
	}
	fmt.Println()

	// Upgrade each package
	for _, pkg := range packagesToUpdate {
		if verbose {
			fmt.Printf("Upgrading %s...\n", pkg.Name)
		}

		err := m.InstallAURPackage(pkg.Name, verbose)
		if err != nil {
			fmt.Printf("Failed to upgrade %s: %v\n", pkg.Name, err)
			continue
		}

		if verbose {
			fmt.Printf("Successfully upgraded %s\n", pkg.Name)
		}
	}

	return nil
}

// CheckAURUpdates displays available AUR updates without installing them
func (m *ALPMManager) CheckAURUpdates() error {
	updates, err := m.GetAURUpdates()
	if err != nil {
		return fmt.Errorf("failed to check AUR updates: %w", err)
	}

	// Filter and display packages that have updates
	var hasUpdates bool
	for _, update := range updates {
		if update.NeedsUpdate {
			if !hasUpdates {
				fmt.Println("Available AUR updates:")
				hasUpdates = true
			}

			status := ""
			if update.OutOfDate {
				status = " [out of date]"
			}
			if update.AURVersion == "not found" {
				status = " [not found in AUR]"
			}

			fmt.Printf("  %s: %s -> %s%s\n", update.Name, update.InstalledVersion, update.AURVersion, status)
		}
	}

	if !hasUpdates {
		fmt.Println("All AUR packages are up to date")
	}

	return nil
}

func (m *ALPMManager) GetAURPackages() ([]string, error) {
	// Get installed packages
	installed, err := m.GetInstalledPackages()
	if err != nil {
		return nil, err
	}

	// Get sync databases to check against official repos
	syncDBs, err := m.handle.SyncDBs()
	if err != nil {
		return nil, fmt.Errorf("failed to get sync databases: %w", err)
	}

	var aurPackages []string
	for _, pkg := range installed {
		// Check if package exists in any official repository
		foundInOfficial := false
		for _, db := range syncDBs.Slice() {
			if db.Pkg(pkg) != nil {
				foundInOfficial = true
				break
			}
		}

		// If not found in official repos, it's likely from AUR
		if !foundInOfficial {
			aurPackages = append(aurPackages, pkg)
		}
	}

	return aurPackages, nil
}

// shouldUpdateWithDevel determines if a package needs updating with optional explicit VCS checking
func (m *ALPMManager) shouldUpdateWithDevel(packageName, installedVer, aurVer string, vcsStore *VCSStore, checkDevel bool) bool {
	// For git packages, use VCS-aware comparison
	if IsGitPackage(packageName) {
		// If explicit devel checking is disabled, still use VCS logic but be more conservative
		needsUpdate, err := vcsStore.CheckGitUpdate(context.Background(), packageName)
		if err != nil {
			if checkDevel {
				// If devel checking is explicitly enabled, fall back to version comparison on VCS error
				return alpm.VerCmp(installedVer, aurVer) < 0
			} else {
				// If devel checking is not explicit, be conservative - assume no update needed
				return false
			}
		}
		return needsUpdate
	}

	// For regular packages, use libalpm's proper version comparison
	return alpm.VerCmp(installedVer, aurVer) < 0
}
