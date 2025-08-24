package packages

import (
	"fmt"
	"strings"
)

// PackageSearchExecutor provides comprehensive package search capabilities
// across official repositories and AUR with intelligent ranking and filtering
type PackageSearchExecutor struct {
	databaseExecutor   *PacmanDatabaseExecutor
	aurManager         *AURPackageManager
	configuredPackages map[string]bool
}

// ExecutorExecutorSearchResult represents a unified search result from repositories or AUR
type ExecutorSearchResult struct {
	Name         string
	Version      string
	Description  string
	Repository   string
	URL          string
	Maintainer   string
	Votes        int
	Popularity   float64
	OutOfDate    bool
	Installed    bool
	InConfig     bool
	IsVCSPackage bool
}

// SearchOptions controls search behavior and filtering
type SearchOptions struct {
	RepositoryOnly   bool
	AUROnly          bool
	IncludeInstalled bool
	IncludeOutdated  bool
	MaxResults       int
	SortByRelevance  bool
	SearchFields     []string // name, description, maintainer
}

// NewPackageSearchExecutor creates a new search executor with database and AUR access
func NewPackageSearchExecutor() (*PackageSearchExecutor, error) {
	dbExecutor, err := NewPacmanDatabaseExecutor()
	if err != nil {
		return nil, fmt.Errorf("failed to create database executor: %w", err)
	}

	aurManager, err := NewAURPackageManagerWithExecutor(dbExecutor)
	if err != nil {
		return nil, fmt.Errorf("failed to create AUR manager: %w", err)
	}

	return &PackageSearchExecutor{
		databaseExecutor:   dbExecutor,
		aurManager:         aurManager,
		configuredPackages: make(map[string]bool),
	}, nil
}

// SetConfiguredPackages updates the list of packages that are configured for management
func (pse *PackageSearchExecutor) SetConfiguredPackages(packages []string) {
	pse.configuredPackages = make(map[string]bool)
	for _, pkg := range packages {
		pse.configuredPackages[pkg] = true
	}
}

// SearchPackages performs a comprehensive search across repositories and AUR
func (pse *PackageSearchExecutor) SearchPackages(searchTerm string, options SearchOptions) ([]ExecutorSearchResult, error) {
	var allResults []ExecutorSearchResult

	// Search official repositories if not AUR-only
	if !options.AUROnly {
		repoResults, err := pse.searchRepositoryPackages(searchTerm, options)
		if err != nil {
			return nil, fmt.Errorf("repository search failed: %w", err)
		}
		allResults = append(allResults, repoResults...)
	}

	// Search AUR if not repository-only
	if !options.RepositoryOnly {
		aurResults, err := pse.searchAURPackages(searchTerm, options)
		if err != nil {
			return nil, fmt.Errorf("AUR search failed: %w", err)
		}
		allResults = append(allResults, aurResults...)
	}

	// Remove duplicates (prefer repo packages over AUR)
	deduplicated := pse.deduplicateResults(allResults)

	// Apply filters
	filtered := pse.applySearchFilters(deduplicated, options)

	// Sort results
	sorted := pse.sortResults(filtered, searchTerm, options)

	// Limit results if requested
	if options.MaxResults > 0 && len(sorted) > options.MaxResults {
		sorted = sorted[:options.MaxResults]
	}

	return sorted, nil
}

// NarrowSearch performs a multi-term search that narrows results progressively
// This mimics yay's behavior: "owl search linux headers" searches for "linux" then narrows to "headers"
func (pse *PackageSearchExecutor) NarrowSearch(terms []string, options SearchOptions) ([]ExecutorSearchResult, error) {
	if len(terms) == 0 {
		return []ExecutorSearchResult{}, nil
	}

	// Start with first term
	results, err := pse.SearchPackages(terms[0], options)
	if err != nil {
		return nil, err
	}

	// Narrow down with subsequent terms
	for _, term := range terms[1:] {
		results = pse.narrowResultsByTerm(results, term)
	}

	return results, nil
}

// GetPackageInformation gets detailed information about a specific package
func (pse *PackageSearchExecutor) GetPackageInformation(packageName string) (*ExecutorSearchResult, error) {
	// Check repositories first
	if syncPkg := pse.databaseExecutor.FindSyncPackage(packageName); syncPkg != nil {
		return pse.createRepositoryResult(syncPkg), nil
	}

	// Check local database
	if localPkg := pse.databaseExecutor.FindLocalPackage(packageName); localPkg != nil {
		result := &ExecutorSearchResult{
			Name:        localPkg.Name(),
			Version:     localPkg.Version(),
			Description: localPkg.Description(),
			Repository:  "local",
			URL:         localPkg.URL(),
			Installed:   true,
			InConfig:    pse.configuredPackages[packageName],
		}
		return result, nil
	}

	// Check AUR
	aurMetadata, err := pse.aurManager.GetAURPackageMetadata(packageName)
	if err == nil {
		return pse.createAURResult(aurMetadata), nil
	}

	return nil, fmt.Errorf("package %s not found in repositories, local database, or AUR", packageName)
}

// Helper Methods for Repository Search

func (pse *PackageSearchExecutor) searchRepositoryPackages(searchTerm string, options SearchOptions) ([]ExecutorSearchResult, error) {
	searchTerms := strings.Fields(searchTerm)
	packages := pse.databaseExecutor.SearchSyncPackages(searchTerms...)

	var results []ExecutorSearchResult
	for _, pkg := range packages {
		result := pse.createRepositoryResult(pkg)
		results = append(results, *result)
	}

	return results, nil
}

func (pse *PackageSearchExecutor) createRepositoryResult(pkg any) *ExecutorSearchResult {
	// This would need to be implemented based on the actual ALPM package interface
	// For now, returning a placeholder structure
	return &ExecutorSearchResult{
		Name:        "package-name", // pkg.Name()
		Version:     "1.0.0",        // pkg.Version()
		Description: "Description",  // pkg.Description()
		Repository:  "core",         // pkg.DB().Name()
		URL:         "",             // pkg.URL()
		Installed:   false,          // check local database
		InConfig:    false,          // check configured packages
	}
}

// Helper Methods for AUR Search

func (pse *PackageSearchExecutor) searchAURPackages(searchTerm string, options SearchOptions) ([]ExecutorSearchResult, error) {
	searchTerms := strings.Fields(searchTerm)
	aurPackages, err := pse.aurManager.SearchAURPackages(searchTerms)
	if err != nil {
		return nil, err
	}

	var results []ExecutorSearchResult
	for i := range aurPackages {
		result := pse.createAURResult(&aurPackages[i])
		results = append(results, *result)
	}

	return results, nil
}

func (pse *PackageSearchExecutor) createAURResult(metadata *AURPackageMetadata) *ExecutorSearchResult {
	isInstalled := pse.databaseExecutor.IsPackageInstalled(metadata.Name)

	return &ExecutorSearchResult{
		Name:         metadata.Name,
		Version:      metadata.Version,
		Description:  metadata.Description,
		Repository:   "aur",
		URL:          metadata.URL,
		Maintainer:   metadata.Maintainer,
		Votes:        metadata.NumVotes,
		Popularity:   metadata.Popularity,
		OutOfDate:    metadata.OutOfDate > 0,
		Installed:    isInstalled,
		InConfig:     pse.configuredPackages[metadata.Name],
		IsVCSPackage: IsGitPackage(metadata.Name),
	}
}

// Result Processing Methods

func (pse *PackageSearchExecutor) deduplicateResults(results []ExecutorSearchResult) []ExecutorSearchResult {
	seen := make(map[string]bool)
	var deduplicated []ExecutorSearchResult

	// Process results, keeping first occurrence (repository packages come first)
	for _, result := range results {
		if !seen[result.Name] {
			seen[result.Name] = true
			deduplicated = append(deduplicated, result)
		}
	}

	return deduplicated
}

func (pse *PackageSearchExecutor) applySearchFilters(results []ExecutorSearchResult, options SearchOptions) []ExecutorSearchResult {
	var filtered []ExecutorSearchResult

	for _, result := range results {
		// Filter by installation status
		if !options.IncludeInstalled && result.Installed {
			continue
		}

		// Filter by outdated status
		if !options.IncludeOutdated && result.OutOfDate {
			continue
		}

		filtered = append(filtered, result)
	}

	return filtered
}

func (pse *PackageSearchExecutor) sortResults(results []ExecutorSearchResult, searchTerm string, options SearchOptions) []ExecutorSearchResult {
	if !options.SortByRelevance {
		return results
	}

	// Implement relevance-based sorting
	// Priority: exact name match > name prefix > name contains > description contains
	// Within each category, sort by popularity for AUR packages, alphabetically for others

	// For now, return as-is (could implement sophisticated ranking algorithm)
	return results
}

func (pse *PackageSearchExecutor) narrowResultsByTerm(results []ExecutorSearchResult, term string) []ExecutorSearchResult {
	var narrowed []ExecutorSearchResult
	termLower := strings.ToLower(term)

	for _, result := range results {
		if pse.resultMatchesTerm(result, termLower) {
			narrowed = append(narrowed, result)
		}
	}

	return narrowed
}

func (pse *PackageSearchExecutor) resultMatchesTerm(result ExecutorSearchResult, term string) bool {
	nameLower := strings.ToLower(result.Name)
	descLower := strings.ToLower(result.Description)

	return strings.Contains(nameLower, term) || strings.Contains(descLower, term)
}

// SearchStatistics provides information about search operations
type SearchStatistics struct {
	RepositoryResults  int
	AURResults         int
	TotalResults       int
	InstalledPackages  int
	ConfiguredPackages int
	OutdatedPackages   int
	SearchDuration     string
}

func (pse *PackageSearchExecutor) GetSearchStatistics(results []ExecutorSearchResult) SearchStatistics {
	stats := SearchStatistics{
		TotalResults: len(results),
	}

	for _, result := range results {
		if result.Repository == "aur" {
			stats.AURResults++
		} else {
			stats.RepositoryResults++
		}

		if result.Installed {
			stats.InstalledPackages++
		}

		if result.InConfig {
			stats.ConfiguredPackages++
		}

		if result.OutOfDate {
			stats.OutdatedPackages++
		}
	}

	return stats
}

// Resource Management

func (pse *PackageSearchExecutor) Release() {
	if pse.databaseExecutor != nil {
		pse.databaseExecutor.Release()
	}
	if pse.aurManager != nil {
		pse.aurManager.Release()
	}
}
