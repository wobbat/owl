package packages

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
	"strings"
	"time"
)

// AURPackageManager handles all AUR-related operations
// with clear, descriptive method names following domain language
type AURPackageManager struct {
	httpClient       *http.Client
	buildDirectory   string
	vcsStore         *VCSStore
	databaseExecutor *PacmanDatabaseExecutor
}

// AURPackageMetadata represents comprehensive AUR package information
type AURPackageMetadata struct {
	Name             string   `json:"Name"`
	Version          string   `json:"Version"`
	Description      string   `json:"Description"`
	URL              string   `json:"URL"`
	URLPath          string   `json:"URLPath"`
	PackageBase      string   `json:"PackageBase"`
	Maintainer       string   `json:"Maintainer"`
	OutOfDate        int64    `json:"OutOfDate"`
	NumVotes         int      `json:"NumVotes"`
	Popularity       float64  `json:"Popularity"`
	FirstSubmitted   int64    `json:"FirstSubmitted"`
	LastModified     int64    `json:"LastModified"`
	Dependencies     []string `json:"Depends"`
	MakeDependencies []string `json:"MakeDepends"`
	OptionalDeps     []string `json:"OptDepends"`
	Conflicts        []string `json:"Conflicts"`
	Provides         []string `json:"Provides"`
	Replaces         []string `json:"Replaces"`
	Groups           []string `json:"Groups"`
	License          []string `json:"License"`
	Keywords         []string `json:"Keywords"`
}

// AURSearchResponse represents the response from AUR search API
type AURSearchResponse struct {
	Version     int                  `json:"version"`
	Type        string               `json:"type"`
	ResultCount int                  `json:"resultcount"`
	Results     []AURPackageMetadata `json:"results"`
}

// AURPackageStatus represents the current state of an AUR package
type AURPackageStatus struct {
	Name             string
	InstalledVersion string
	AURVersion       string
	IsInstalled      bool
	HasUpdate        bool
	IsOutOfDate      bool
	IsVCSPackage     bool
	LastBuildTime    time.Time
}

// BuildProgress represents the progress of package building
type BuildProgress struct {
	PackageName string
	Stage       string  // "downloading", "extracting", "building", "installing"
	Progress    float64 // 0.0 to 1.0
	Message     string
	IsError     bool
}

// BuildProgressCallback is called during package building to report progress
type BuildProgressCallback func(progress BuildProgress)

// NewAURPackageManager creates a new AUR package manager with sensible defaults
func NewAURPackageManager() (*AURPackageManager, error) {
	executor, err := NewPacmanDatabaseExecutor()
	if err != nil {
		return nil, fmt.Errorf("failed to create database executor: %w", err)
	}

	vcsStore, err := NewVCSStore()
	if err != nil {
		return nil, fmt.Errorf("failed to create VCS store: %w", err)
	}

	return &AURPackageManager{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		buildDirectory:   "/tmp/owl-aur-builds",
		vcsStore:         vcsStore,
		databaseExecutor: executor,
	}, nil
}

// NewAURPackageManagerWithExecutor creates AUR manager with existing database executor
func NewAURPackageManagerWithExecutor(executor *PacmanDatabaseExecutor) (*AURPackageManager, error) {
	vcsStore, err := NewVCSStore()
	if err != nil {
		return nil, fmt.Errorf("failed to create VCS store: %w", err)
	}

	return &AURPackageManager{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		buildDirectory:   "/tmp/owl-aur-builds",
		vcsStore:         vcsStore,
		databaseExecutor: executor,
	}, nil
}

// Package Discovery and Search Methods

func (apm *AURPackageManager) SearchAURPackages(searchTerms []string) ([]AURPackageMetadata, error) {
	if len(searchTerms) == 0 {
		return nil, fmt.Errorf("search terms cannot be empty")
	}

	query := strings.Join(searchTerms, " ")
	url := fmt.Sprintf("https://aur.archlinux.org/rpc/?v=5&type=search&by=name-desc&arg=%s",
		strings.ReplaceAll(query, " ", "+"))

	response, err := apm.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to search AUR: %w", err)
	}
	defer response.Body.Close()

	var searchResponse AURSearchResponse
	if err := json.NewDecoder(response.Body).Decode(&searchResponse); err != nil {
		return nil, fmt.Errorf("failed to decode AUR search response: %w", err)
	}

	return searchResponse.Results, nil
}

func (apm *AURPackageManager) GetAURPackageMetadata(packageName string) (*AURPackageMetadata, error) {
	url := fmt.Sprintf("https://aur.archlinux.org/rpc/?v=5&type=info&arg[]=%s", packageName)

	response, err := apm.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to query AUR for package %s: %w", packageName, err)
	}
	defer response.Body.Close()

	var aurResponse AURSearchResponse
	if err := json.NewDecoder(response.Body).Decode(&aurResponse); err != nil {
		return nil, fmt.Errorf("failed to decode AUR response: %w", err)
	}

	if aurResponse.ResultCount == 0 {
		return nil, fmt.Errorf("package %s not found in AUR", packageName)
	}

	return &aurResponse.Results[0], nil
}

func (apm *AURPackageManager) GetBatchAURPackageMetadata(packageNames []string) (map[string]*AURPackageMetadata, error) {
	if len(packageNames) == 0 {
		return make(map[string]*AURPackageMetadata), nil
	}

	url := "https://aur.archlinux.org/rpc/?v=5&type=info"
	for _, pkg := range packageNames {
		url += "&arg[]=" + pkg
	}

	response, err := apm.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to batch query AUR: %w", err)
	}
	defer response.Body.Close()

	var aurResponse AURSearchResponse
	if err := json.NewDecoder(response.Body).Decode(&aurResponse); err != nil {
		return nil, fmt.Errorf("failed to decode AUR batch response: %w", err)
	}

	result := make(map[string]*AURPackageMetadata)
	for i := range aurResponse.Results {
		pkg := &aurResponse.Results[i]
		result[pkg.Name] = pkg
	}

	return result, nil
}

// Package Status and Version Methods

func (apm *AURPackageManager) GetAURPackageStatus(packageName string) (*AURPackageStatus, error) {
	aurMetadata, err := apm.GetAURPackageMetadata(packageName)
	if err != nil {
		return nil, fmt.Errorf("failed to get AUR metadata: %w", err)
	}

	status := &AURPackageStatus{
		Name:         packageName,
		AURVersion:   aurMetadata.Version,
		IsOutOfDate:  aurMetadata.OutOfDate > 0,
		IsVCSPackage: IsGitPackage(packageName),
	}

	// Check if package is installed
	if localPkg := apm.databaseExecutor.FindLocalPackage(packageName); localPkg != nil {
		status.IsInstalled = true
		status.InstalledVersion = localPkg.Version()
		status.LastBuildTime = localPkg.BuildDate()
	}

	// Determine if update is needed
	if status.IsInstalled {
		if status.IsVCSPackage {
			// For VCS packages, check using VCS store
			hasUpdate, err := apm.vcsStore.CheckGitUpdate(context.Background(), packageName)
			if err == nil {
				status.HasUpdate = hasUpdate
			}
		} else {
			// For regular packages, use version comparison
			status.HasUpdate = apm.databaseExecutor.ComparePackageVersions(
				status.InstalledVersion, status.AURVersion) < 0
		}
	}

	return status, nil
}

func (apm *AURPackageManager) CheckAllAURUpdates() ([]AURPackageStatus, error) {
	foreignPackages, err := apm.GetInstalledForeignPackages()
	if err != nil {
		return nil, fmt.Errorf("failed to get foreign packages: %w", err)
	}

	if len(foreignPackages) == 0 {
		return []AURPackageStatus{}, nil
	}

	// Batch query AUR for metadata
	aurMetadata, err := apm.GetBatchAURPackageMetadata(foreignPackages)
	if err != nil {
		return nil, fmt.Errorf("failed to batch query AUR: %w", err)
	}

	var statusList []AURPackageStatus
	for _, packageName := range foreignPackages {
		metadata, exists := aurMetadata[packageName]
		if !exists {
			// Package not found in AUR (might be deleted)
			localPkg := apm.databaseExecutor.FindLocalPackage(packageName)
			status := AURPackageStatus{
				Name:             packageName,
				InstalledVersion: localPkg.Version(),
				AURVersion:       "not found",
				IsInstalled:      true,
				HasUpdate:        false,
				IsVCSPackage:     IsGitPackage(packageName),
				LastBuildTime:    localPkg.BuildDate(),
			}
			statusList = append(statusList, status)
			continue
		}

		status, err := apm.createStatusFromMetadata(packageName, metadata)
		if err != nil {
			continue // Skip packages with errors
		}

		statusList = append(statusList, *status)
	}

	return statusList, nil
}

func (apm *AURPackageManager) createStatusFromMetadata(packageName string, metadata *AURPackageMetadata) (*AURPackageStatus, error) {
	status := &AURPackageStatus{
		Name:         packageName,
		AURVersion:   metadata.Version,
		IsOutOfDate:  metadata.OutOfDate > 0,
		IsVCSPackage: IsGitPackage(packageName),
	}

	if localPkg := apm.databaseExecutor.FindLocalPackage(packageName); localPkg != nil {
		status.IsInstalled = true
		status.InstalledVersion = localPkg.Version()
		status.LastBuildTime = localPkg.BuildDate()

		// Determine update necessity
		if status.IsVCSPackage {
			hasUpdate, err := apm.vcsStore.CheckGitUpdate(context.Background(), packageName)
			if err == nil {
				status.HasUpdate = hasUpdate
			}
		} else {
			status.HasUpdate = apm.databaseExecutor.ComparePackageVersions(
				status.InstalledVersion, status.AURVersion) < 0
		}
	}

	return status, nil
}

// Package Installation Methods

func (apm *AURPackageManager) InstallAURPackage(packageName string, progressCallback BuildProgressCallback) error {
	return apm.InstallAURPackageWithOptions(packageName, AURInstallOptions{}, progressCallback)
}

func (apm *AURPackageManager) InstallAURPackageWithOptions(packageName string, options AURInstallOptions, progressCallback BuildProgressCallback) error {
	if progressCallback != nil {
		progressCallback(BuildProgress{
			PackageName: packageName,
			Stage:       "initializing",
			Message:     fmt.Sprintf("Starting installation of %s", packageName),
		})
	}

	// Get package metadata
	metadata, err := apm.GetAURPackageMetadata(packageName)
	if err != nil {
		return fmt.Errorf("failed to get package metadata: %w", err)
	}

	if progressCallback != nil {
		progressCallback(BuildProgress{
			PackageName: packageName,
			Stage:       "downloading",
			Message:     fmt.Sprintf("Found %s v%s in AUR", packageName, metadata.Version),
		})
	}

	// Prepare build directory
	packageBuildDir := filepath.Join(apm.buildDirectory, packageName)
	if err := apm.prepareBuildDirectory(packageBuildDir); err != nil {
		return fmt.Errorf("failed to prepare build directory: %w", err)
	}
	defer func() {
		if !options.KeepBuildDir {
			os.RemoveAll(packageBuildDir)
		}
	}()

	// Clone AUR repository
	if err := apm.cloneAURRepository(packageName, packageBuildDir, progressCallback); err != nil {
		return fmt.Errorf("failed to clone AUR repository: %w", err)
	}

	// Build and install package
	if err := apm.buildAndInstallPackage(packageName, packageBuildDir, options, progressCallback); err != nil {
		return fmt.Errorf("failed to build and install package: %w", err)
	}

	// Update VCS tracking for git packages
	if IsGitPackage(packageName) {
		if err := apm.updateVCSTracking(packageName, packageBuildDir); err != nil {
			// Don't fail installation if VCS tracking fails
			if progressCallback != nil {
				progressCallback(BuildProgress{
					PackageName: packageName,
					Stage:       "finalizing",
					Message:     fmt.Sprintf("Warning: Failed to update VCS tracking: %v", err),
				})
			}
		}
	}

	if progressCallback != nil {
		progressCallback(BuildProgress{
			PackageName: packageName,
			Stage:       "completed",
			Progress:    1.0,
			Message:     fmt.Sprintf("Successfully installed %s", packageName),
		})
	}

	return nil
}

// Helper Methods for Installation

func (apm *AURPackageManager) prepareBuildDirectory(buildDir string) error {
	if err := os.RemoveAll(buildDir); err != nil {
		return fmt.Errorf("failed to clean existing build directory: %w", err)
	}

	if err := os.MkdirAll(buildDir, 0755); err != nil {
		return fmt.Errorf("failed to create build directory: %w", err)
	}

	return nil
}

func (apm *AURPackageManager) cloneAURRepository(packageName, buildDir string, progressCallback BuildProgressCallback) error {
	if progressCallback != nil {
		progressCallback(BuildProgress{
			PackageName: packageName,
			Stage:       "downloading",
			Message:     fmt.Sprintf("Cloning %s repository", packageName),
		})
	}

	gitURL := fmt.Sprintf("https://aur.archlinux.org/%s.git", packageName)
	cmd := exec.Command("git", "clone", gitURL, buildDir)

	// Monitor git clone progress
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start git clone: %w", err)
	}

	// Monitor clone progress in background
	go apm.monitorGitCloneProgress(stderr, packageName, progressCallback)

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("git clone failed: %w", err)
	}

	return nil
}

func (apm *AURPackageManager) buildAndInstallPackage(packageName, buildDir string, options AURInstallOptions, progressCallback BuildProgressCallback) error {
	if progressCallback != nil {
		progressCallback(BuildProgress{
			PackageName: packageName,
			Stage:       "building",
			Message:     fmt.Sprintf("Building %s package", packageName),
		})
	}

	// Prepare makepkg command
	makepkgArgs := []string{"-si"}
	if options.NoConfirm {
		makepkgArgs = append(makepkgArgs, "--noconfirm")
	}
	if options.Force {
		makepkgArgs = append(makepkgArgs, "--force")
	}
	if options.AsDeps {
		makepkgArgs = append(makepkgArgs, "--asdeps")
	}

	cmd := exec.Command("makepkg", makepkgArgs...)
	cmd.Dir = buildDir

	if options.ShowOutput {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	// Monitor build progress
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start makepkg: %w", err)
	}

	// Monitor build progress
	go apm.monitorMakepkgProgress(stdout, stderr, packageName, progressCallback)

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("makepkg failed: %w", err)
	}

	return nil
}

func (apm *AURPackageManager) updateVCSTracking(packageName, buildDir string) error {
	pkgbuildPath := filepath.Join(buildDir, "PKGBUILD")
	pkgbuildContent, err := os.ReadFile(pkgbuildPath)
	if err != nil {
		return fmt.Errorf("failed to read PKGBUILD: %w", err)
	}

	return apm.vcsStore.UpdatePackageInfo(packageName, string(pkgbuildContent))
}

// Progress Monitoring Methods

func (apm *AURPackageManager) monitorGitCloneProgress(stderr io.ReadCloser, packageName string, progressCallback BuildProgressCallback) {
	if progressCallback == nil {
		return
	}

	scanner := bufio.NewScanner(stderr)
	for scanner.Scan() {
		line := scanner.Text()

		var message string
		var progress float64

		if strings.Contains(line, "Cloning into") {
			message = fmt.Sprintf("Initializing clone for %s", packageName)
			progress = 0.1
		} else if strings.Contains(line, "Receiving objects") {
			message = fmt.Sprintf("Downloading %s source", packageName)
			progress = 0.5
		} else if strings.Contains(line, "Resolving deltas") {
			message = fmt.Sprintf("Processing %s source", packageName)
			progress = 0.9
		} else {
			continue
		}

		progressCallback(BuildProgress{
			PackageName: packageName,
			Stage:       "downloading",
			Progress:    progress,
			Message:     message,
		})
	}
}

func (apm *AURPackageManager) monitorMakepkgProgress(stdout, stderr io.ReadCloser, packageName string, progressCallback BuildProgressCallback) {
	if progressCallback == nil {
		return
	}

	// Monitor stdout for main progress
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()

			var stage string
			var message string
			var progress float64

			if strings.Contains(line, "Retrieving sources") {
				stage = "downloading"
				message = fmt.Sprintf("Downloading %s sources", packageName)
				progress = 0.1
			} else if strings.Contains(line, "Validating source files") {
				stage = "building"
				message = fmt.Sprintf("Validating %s sources", packageName)
				progress = 0.2
			} else if strings.Contains(line, "Extracting sources") {
				stage = "building"
				message = fmt.Sprintf("Extracting %s sources", packageName)
				progress = 0.3
			} else if strings.Contains(line, "Starting build") {
				stage = "building"
				message = fmt.Sprintf("Compiling %s", packageName)
				progress = 0.5
			} else if strings.Contains(line, "Installing package") {
				stage = "installing"
				message = fmt.Sprintf("Installing compiled %s", packageName)
				progress = 0.9
			} else if strings.Contains(line, "Finished making") {
				stage = "completed"
				message = fmt.Sprintf("Finished building %s", packageName)
				progress = 1.0
			} else {
				continue
			}

			progressCallback(BuildProgress{
				PackageName: packageName,
				Stage:       stage,
				Progress:    progress,
				Message:     message,
			})
		}
	}()

	// Monitor stderr for errors
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.Contains(line, "error") || strings.Contains(line, "ERROR") {
				progressCallback(BuildProgress{
					PackageName: packageName,
					Stage:       "error",
					Message:     fmt.Sprintf("Build error: %s", line),
					IsError:     true,
				})
			}
		}
	}()
}

// Foreign Package Management

func (apm *AURPackageManager) GetInstalledForeignPackages() ([]string, error) {
	allPackages := apm.databaseExecutor.GetAllInstalledPackages()
	repositories := apm.databaseExecutor.GetConfiguredRepositories()

	var foreignPackages []string
	for _, pkg := range allPackages {
		isOfficial := false
		for _, repo := range repositories {
			if syncPkg := apm.databaseExecutor.FindSyncPackageInRepository(pkg.Name(), repo); syncPkg != nil {
				isOfficial = true
				break
			}
		}

		if !isOfficial {
			foreignPackages = append(foreignPackages, pkg.Name())
		}
	}

	return foreignPackages, nil
}

// Configuration and Options

type AURInstallOptions struct {
	NoConfirm    bool
	Force        bool
	AsDeps       bool
	ShowOutput   bool
	KeepBuildDir bool
}

// Resource Management

func (apm *AURPackageManager) SetBuildDirectory(directory string) {
	apm.buildDirectory = directory
}

func (apm *AURPackageManager) SetHTTPTimeout(timeout time.Duration) {
	apm.httpClient.Timeout = timeout
}

func (apm *AURPackageManager) Release() {
	if apm.databaseExecutor != nil {
		apm.databaseExecutor.Release()
	}
	if apm.vcsStore != nil {
		apm.vcsStore.Save()
	}
}
