package packages

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// VCSInfo represents VCS package information
type VCSInfo struct {
	URL    string `json:"url"`
	Branch string `json:"branch"`
	SHA    string `json:"sha"`
}

// VCSStore manages VCS package cache
type VCSStore struct {
	OriginsByPackage map[string][]VCSInfo `json:"origins_by_package"`
	cachePath        string
}

// NewVCSStore creates a new VCS store
func NewVCSStore() (*VCSStore, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	cacheDir := filepath.Join(homeDir, ".cache", "gowl")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	cachePath := filepath.Join(cacheDir, "vcs.json")
	store := &VCSStore{
		OriginsByPackage: make(map[string][]VCSInfo),
		cachePath:        cachePath,
	}

	// Load existing cache
	if err := store.Load(); err != nil {
		// If file doesn't exist, that's fine - we'll create it
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to load VCS cache: %w", err)
		}
	}

	return store, nil
}

// Load loads the VCS cache from disk
func (v *VCSStore) Load() error {
	data, err := os.ReadFile(v.cachePath)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, v)
}

// Save saves the VCS cache to disk
func (v *VCSStore) Save() error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal VCS cache: %w", err)
	}

	return os.WriteFile(v.cachePath, data, 0644)
}

// IsGitPackage checks if a package name suggests it's a git package
func IsGitPackage(packageName string) bool {
	return strings.HasSuffix(packageName, "-git") ||
		strings.HasSuffix(packageName, "-hg") ||
		strings.HasSuffix(packageName, "-svn") ||
		strings.HasSuffix(packageName, "-bzr")
}

// CheckGitUpdate checks if a git package needs updating by comparing commit hashes
func (v *VCSStore) CheckGitUpdate(ctx context.Context, packageName string) (bool, error) {
	infos, exists := v.OriginsByPackage[packageName]
	if !exists || len(infos) == 0 {
		// Package not in cache - we need to check it once to establish baseline
		// But for now, assume no update to avoid unnecessary rebuilds
		return false, nil
	}

	// Check each git source for updates
	for _, info := range infos {
		needsUpdate, err := v.checkRemoteCommit(ctx, info)
		if err != nil {
			// If we can't check, assume no update needed to avoid constant rebuilds
			continue
		}
		if needsUpdate {
			return true, nil
		}
	}

	return false, nil
}

// checkRemoteCommit checks if the remote commit differs from cached commit
func (v *VCSStore) checkRemoteCommit(ctx context.Context, info VCSInfo) (bool, error) {
	// Create context with timeout
	ctxTimeout, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	// Use git ls-remote to get the latest commit without cloning
	branch := info.Branch
	if branch == "" {
		branch = "HEAD"
	}

	cmd := exec.CommandContext(ctxTimeout, "git", "ls-remote", info.URL, branch)
	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("failed to check remote commit for %s: %w", info.URL, err)
	}

	// Parse the commit SHA from output
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) == 0 {
		return false, fmt.Errorf("no output from git ls-remote")
	}

	// Extract commit SHA (first part before whitespace)
	parts := strings.Fields(lines[0])
	if len(parts) == 0 {
		return false, fmt.Errorf("invalid git ls-remote output")
	}

	remoteCommit := parts[0]

	// Compare with cached commit
	return remoteCommit != info.SHA, nil
}

// UpdatePackageInfo updates the VCS info for a package after installation
func (v *VCSStore) UpdatePackageInfo(packageName string, pkgbuildContent string) error {
	if !IsGitPackage(packageName) {
		return nil // Not a VCS package
	}

	sources := parseSourcesFromPKGBUILD(pkgbuildContent)
	var vcsInfos []VCSInfo

	for _, source := range sources {
		if info := parseVCSSource(source); info != nil {
			// Get current commit for this source
			sha, err := v.getCurrentCommit(context.Background(), *info)
			if err != nil {
				// If we can't get current commit, store without SHA
				vcsInfos = append(vcsInfos, *info)
				continue
			}
			info.SHA = sha
			vcsInfos = append(vcsInfos, *info)
		}
	}

	if len(vcsInfos) > 0 {
		v.OriginsByPackage[packageName] = vcsInfos
		return v.Save()
	}

	return nil
}

// getCurrentCommit gets the current commit SHA for a VCS source
func (v *VCSStore) getCurrentCommit(ctx context.Context, info VCSInfo) (string, error) {
	ctxTimeout, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	branch := info.Branch
	if branch == "" {
		branch = "HEAD"
	}

	cmd := exec.CommandContext(ctxTimeout, "git", "ls-remote", info.URL, branch)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) == 0 {
		return "", fmt.Errorf("no output from git ls-remote")
	}

	parts := strings.Fields(lines[0])
	if len(parts) == 0 {
		return "", fmt.Errorf("invalid git ls-remote output")
	}

	return parts[0], nil
}

// parseSourcesFromPKGBUILD extracts source URLs from PKGBUILD content
func parseSourcesFromPKGBUILD(content string) []string {
	var sources []string

	// Look for source array definition
	sourceRegex := regexp.MustCompile(`source\s*=\s*\([^)]*\)`)
	matches := sourceRegex.FindAllString(content, -1)

	for _, match := range matches {
		// Extract URLs from the source array
		urlRegex := regexp.MustCompile(`["']([^"']+)["']`)
		urlMatches := urlRegex.FindAllStringSubmatch(match, -1)

		for _, urlMatch := range urlMatches {
			if len(urlMatch) > 1 {
				sources = append(sources, urlMatch[1])
			}
		}
	}

	return sources
}

// parseVCSSource parses a VCS source URL and extracts relevant information
func parseVCSSource(source string) *VCSInfo {
	// Look for git+ protocol
	if !strings.HasPrefix(source, "git+") {
		return nil
	}

	// Remove git+ prefix
	url := strings.TrimPrefix(source, "git+")

	// Extract branch if specified
	branch := "HEAD"
	if strings.Contains(url, "#branch=") {
		parts := strings.Split(url, "#branch=")
		if len(parts) == 2 {
			url = parts[0]
			branch = parts[1]
		}
	} else if strings.Contains(url, "#") {
		// Skip sources with specific commit/tag references
		return nil
	}

	return &VCSInfo{
		URL:    url,
		Branch: branch,
		SHA:    "", // Will be filled later
	}
}

// CleanOrphans removes VCS info for packages that are no longer installed
func (v *VCSStore) CleanOrphans(installedPackages []string) {
	installedSet := make(map[string]bool)
	for _, pkg := range installedPackages {
		installedSet[pkg] = true
	}

	for packageName := range v.OriginsByPackage {
		if !installedSet[packageName] {
			delete(v.OriginsByPackage, packageName)
		}
	}
}

// InitializeGitPackages initializes VCS info for git packages that don't have it yet
// This is similar to yay's --gendb functionality
func (v *VCSStore) InitializeGitPackages(ctx context.Context, gitPackages []string, aurClient func(string) (*AURPackage, error)) error {
	for _, packageName := range gitPackages {
		if IsGitPackage(packageName) {
			// Skip if we already have VCS info for this package
			if _, exists := v.OriginsByPackage[packageName]; exists {
				continue
			}

			// Try to get AUR info to extract sources (we don't use the result but verify package exists)
			_, err := aurClient(packageName)
			if err != nil {
				// Skip packages not found in AUR or on error
				continue
			}

			// Create temporary directory to download PKGBUILD
			tmpDir := fmt.Sprintf("/tmp/vcs-init-%s", packageName)
			defer os.RemoveAll(tmpDir)

			if err := os.MkdirAll(tmpDir, 0755); err != nil {
				continue
			}

			// Clone AUR repository to get PKGBUILD
			gitURL := fmt.Sprintf("https://aur.archlinux.org/%s.git", packageName)
			cmd := exec.CommandContext(ctx, "git", "clone", "--depth=1", gitURL, tmpDir)
			if err := cmd.Run(); err != nil {
				continue
			}

			// Read PKGBUILD
			pkgbuildPath := filepath.Join(tmpDir, "PKGBUILD")
			pkgbuildContent, err := os.ReadFile(pkgbuildPath)
			if err != nil {
				continue
			}

			// Update VCS info
			if err := v.UpdatePackageInfo(packageName, string(pkgbuildContent)); err != nil {
				// Log error but continue with other packages
				fmt.Printf("Warning: Failed to initialize VCS info for %s: %v\n", packageName, err)
			}
		}
	}

	return v.Save()
}
