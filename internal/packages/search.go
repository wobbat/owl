package packages

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/Jguer/go-alpm/v2"
	"owl/internal/config"
	"owl/internal/types"
)

// SearchResult represents a package search result
type SearchResult struct {
	Name        string
	Repository  string
	Version     string
	Description string
	Installed   bool
	InConfig    bool
	OutOfDate   bool
	Votes       int
	Popularity  float64
}

// PackageSearcher handles package searching across repositories
type PackageSearcher struct {
	alpm *ALPMManager
}

// NewPackageSearcher creates a new package searcher
func NewPackageSearcher() (*PackageSearcher, error) {
	alpm, err := NewALPMManager()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize ALPM: %w", err)
	}

	return &PackageSearcher{alpm: alpm}, nil
}

// Release releases resources
func (ps *PackageSearcher) Release() {
	if ps.alpm != nil {
		ps.alpm.Release()
	}
}

// getConfiguredPackages gets packages from the current configuration
func (ps *PackageSearcher) getConfiguredPackages() []string {
	// Get hostname
	hostname, err := os.Hostname()
	if err != nil {
		return nil
	}

	// Load configuration
	configResult, err := config.LoadConfigForHost(hostname)
	if err != nil {
		return nil
	}

	// Extract all packages from configuration entries
	var configuredPackages []string
	for _, entry := range configResult.Entries {
		configuredPackages = append(configuredPackages, entry.Package)
	}

	return configuredPackages
}

// SearchPackages searches for packages in both official repos and AUR
func (ps *PackageSearcher) SearchPackages(query string, options types.SearchOptions) ([]SearchResult, error) {
	var results []SearchResult
	var searchErrors []string

	// Get installed packages for comparison
	installedPackages, err := ps.alpm.GetInstalledPackages()
	if err != nil {
		return nil, fmt.Errorf("failed to get installed packages: %w", err)
	}

	installedMap := make(map[string]bool)
	for _, pkg := range installedPackages {
		installedMap[pkg] = true
	}

	// Get configured packages for comparison
	configuredMap := make(map[string]bool)
	if configuredPackages := ps.getConfiguredPackages(); configuredPackages != nil {
		for _, pkg := range configuredPackages {
			configuredMap[pkg] = true
		}
	}

	// Search official repositories if not AUR-only
	if !options.AUROnly {
		repoResults, err := ps.searchOfficialRepos(query)
		if err != nil {
			searchErrors = append(searchErrors, fmt.Sprintf("official repos: %v", err))
		} else {
			for _, result := range repoResults {
				result.Installed = installedMap[result.Name]
				result.InConfig = configuredMap[result.Name]
				results = append(results, result)
			}
		}
	}

	// Search AUR if not repo-only
	if !options.RepoOnly {
		aurResults, err := ps.searchAUR(query, options.AURSearchLimit)
		if err != nil {
			searchErrors = append(searchErrors, fmt.Sprintf("AUR: %v", err))
			// Don't fail completely if AUR search fails but official repos succeeded
			if len(results) == 0 {
				return nil, fmt.Errorf("search failed - %s", strings.Join(searchErrors, "; "))
			}
		} else {
			for _, result := range aurResults {
				result.Installed = installedMap[result.Name]
				result.InConfig = configuredMap[result.Name]
				results = append(results, result)
			}
		}
	}

	// If we have any results, sort them and return
	if len(results) > 0 {
		ps.sortResults(results, query)
		return results, nil
	}

	// If no results and we have errors, return the errors
	if len(searchErrors) > 0 {
		return nil, fmt.Errorf("search failed - %s", strings.Join(searchErrors, "; "))
	}

	// No results but no errors either
	return results, nil
}

// searchOfficialRepos searches official repositories
func (ps *PackageSearcher) searchOfficialRepos(query string) ([]SearchResult, error) {
	syncDBs, err := ps.alpm.handle.SyncDBs()
	if err != nil {
		return nil, fmt.Errorf("failed to get sync databases: %w", err)
	}

	var results []SearchResult
	queryLower := strings.ToLower(query)

	// Search through each sync database
	for _, syncDB := range syncDBs.Slice() {
		repoName := syncDB.Name()

		// Search through packages in this database
		err = syncDB.PkgCache().ForEach(func(pkg alpm.IPackage) error {
			name := pkg.Name()
			desc := pkg.Description()

			// Check if package name or description matches the query
			if strings.Contains(strings.ToLower(name), queryLower) ||
				strings.Contains(strings.ToLower(desc), queryLower) {

				results = append(results, SearchResult{
					Name:        name,
					Repository:  repoName,
					Version:     pkg.Version(),
					Description: desc,
				})
			}
			return nil
		})

		if err != nil {
			return nil, fmt.Errorf("failed to search database %s: %w", repoName, err)
		}
	}

	return results, nil
}

// searchAUR searches the AUR
func (ps *PackageSearcher) searchAUR(query string, limit int) ([]SearchResult, error) {
	if limit == 0 {
		limit = 50 // Default limit
	}

	// Build search URL
	baseURL := "https://aur.archlinux.org/rpc/?v=5&type=search"
	searchURL := fmt.Sprintf("%s&arg=%s", baseURL, url.QueryEscape(query))

	// Make HTTP request with better configuration
	client := &http.Client{
		Timeout: 30 * time.Second, // Increased timeout
		Transport: &http.Transport{
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}

	// Create request with proper headers
	req, err := http.NewRequest("GET", searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "owl/1.0")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to query AUR: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("AUR API returned status %d", resp.StatusCode)
	}

	var aurResp AURResponse
	if err := json.NewDecoder(resp.Body).Decode(&aurResp); err != nil {
		return nil, fmt.Errorf("failed to decode AUR response: %w", err)
	}

	var results []SearchResult
	for i, pkg := range aurResp.Results {
		if i >= limit {
			break
		}

		results = append(results, SearchResult{
			Name:        pkg.Name,
			Repository:  "aur",
			Version:     pkg.Version,
			Description: pkg.Description,
			OutOfDate:   pkg.OutOfDate > 0,
			Votes:       pkg.NumVotes,
			Popularity:  pkg.Popularity,
		})
	}

	return results, nil
}

// sortResults sorts search results by relevance
func (ps *PackageSearcher) sortResults(results []SearchResult, query string) {
	sort.Slice(results, func(i, j int) bool {
		a, b := results[i], results[j]

		// Exact name match comes first
		aExact := strings.EqualFold(a.Name, query)
		bExact := strings.EqualFold(b.Name, query)
		if aExact != bExact {
			return aExact
		}

		// Name starts with query
		aStarts := strings.HasPrefix(strings.ToLower(a.Name), strings.ToLower(query))
		bStarts := strings.HasPrefix(strings.ToLower(b.Name), strings.ToLower(query))
		if aStarts != bStarts {
			return aStarts
		}

		// Installed packages come before uninstalled
		if a.Installed != b.Installed {
			return a.Installed
		}

		// Configured packages come before unconfigured
		if a.InConfig != b.InConfig {
			return a.InConfig
		}

		// Official repos come before AUR
		aRepo := a.Repository != "aur"
		bRepo := b.Repository != "aur"
		if aRepo != bRepo {
			return aRepo
		}

		// For AUR packages, sort by popularity/votes
		if a.Repository == "aur" && b.Repository == "aur" {
			if a.Votes != b.Votes {
				return a.Votes > b.Votes
			}
			return a.Popularity > b.Popularity
		}

		// Finally, sort alphabetically
		return a.Name < b.Name
	})
}

// NarrowSearch performs a narrow search (yay-style)
func (ps *PackageSearcher) NarrowSearch(queries []string, options types.SearchOptions) ([]SearchResult, error) {
	if len(queries) == 0 {
		return nil, fmt.Errorf("no search queries provided")
	}

	// Start with the first query
	results, err := ps.SearchPackages(queries[0], options)
	if err != nil {
		return nil, err
	}

	// Narrow down with subsequent queries
	for _, query := range queries[1:] {
		results = ps.filterResults(results, query)
	}

	return results, nil
}

// filterResults filters existing results by a query
func (ps *PackageSearcher) filterResults(results []SearchResult, query string) []SearchResult {
	var filtered []SearchResult
	queryLower := strings.ToLower(query)

	for _, result := range results {
		if strings.Contains(strings.ToLower(result.Name), queryLower) ||
			strings.Contains(strings.ToLower(result.Description), queryLower) {
			filtered = append(filtered, result)
		}
	}

	return filtered
}

// GetPackageInfo gets detailed information about a package
func (ps *PackageSearcher) GetPackageInfo(packageName string) (*types.PackageInfo, error) {
	// Check if package is in official repos first
	info, err := ps.getOfficialPackageInfo(packageName)
	if err == nil {
		return info, nil
	}

	// Try AUR
	return ps.getAURPackageInfo(packageName)
}

// getOfficialPackageInfo gets package info from official repos
func (ps *PackageSearcher) getOfficialPackageInfo(packageName string) (*types.PackageInfo, error) {
	syncDBs, err := ps.alpm.handle.SyncDBs()
	if err != nil {
		return nil, fmt.Errorf("failed to get sync databases: %w", err)
	}

	// Search through each sync database
	for _, syncDB := range syncDBs.Slice() {
		if pkg := syncDB.Pkg(packageName); pkg != nil {
			// Get installed status
			installed, err := ps.alpm.IsPackageInstalled(packageName)
			if err != nil {
				installed = false
			}

			return &types.PackageInfo{
				Name:        pkg.Name(),
				Version:     pkg.Version(),
				Description: pkg.Description(),
				URL:         pkg.URL(),
				Repository:  syncDB.Name(),
				Installed:   installed,
			}, nil
		}
	}

	return nil, fmt.Errorf("package not found in official repositories")
}

// getAURPackageInfo gets package info from AUR
func (ps *PackageSearcher) getAURPackageInfo(packageName string) (*types.PackageInfo, error) {
	aurPkg, err := ps.alpm.QueryAUR(packageName)
	if err != nil {
		return nil, err
	}

	// Get installed status
	installed, err := ps.alpm.IsPackageInstalled(packageName)
	if err != nil {
		installed = false
	}

	return &types.PackageInfo{
		Name:        aurPkg.Name,
		Version:     aurPkg.Version,
		Description: aurPkg.Description,
		URL:         aurPkg.URL,
		Repository:  "aur",
		PackageBase: aurPkg.PackageBase,
		Maintainer:  aurPkg.Maintainer,
		Votes:       aurPkg.NumVotes,
		Popularity:  aurPkg.Popularity,
		OutOfDate:   aurPkg.OutOfDate > 0,
		Installed:   installed,
	}, nil
}
