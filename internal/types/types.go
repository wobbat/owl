package types

import "time"

// ConfigEntry represents a single package configuration entry
type ConfigEntry struct {
	Package    string          `json:"package"`
	Configs    []ConfigMapping `json:"configs"`
	Setups     []string        `json:"setups"`
	Services   []string        `json:"services,omitempty"`
	Envs       []EnvVar        `json:"envs,omitempty"`
	SourceFile string          `json:"source_file,omitempty"`
	SourceType string          `json:"source_type,omitempty"` // main, host, group
	GroupName  string          `json:"group_name,omitempty"`
}

// ConfigMapping represents a dotfile mapping from source to destination
type ConfigMapping struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
}

// EnvVar represents an environment variable key-value pair
type EnvVar struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// PackageInfo represents detailed information about a package
type PackageInfo struct {
	Name             string  `json:"name"`
	Version          string  `json:"version"`
	InstalledVersion string  `json:"installed_version,omitempty"`
	AvailableVersion string  `json:"available_version,omitempty"`
	Description      string  `json:"description"`
	URL              string  `json:"url"`
	Repository       string  `json:"repository"`
	PackageBase      string  `json:"package_base,omitempty"`
	Maintainer       string  `json:"maintainer,omitempty"`
	Votes            int     `json:"votes,omitempty"`
	Popularity       float64 `json:"popularity,omitempty"`
	OutOfDate        bool    `json:"out_of_date"`
	Installed        bool    `json:"installed"`
	Status           string  `json:"status"` // not_installed, up_to_date, outdated
}

// PackageAction represents an action to be taken on a package
type PackageAction struct {
	Name    string `json:"name"`
	Status  string `json:"status"` // install, skip, remove
	Version string `json:"version,omitempty"`
}

// ManagedPackage represents a package tracked by Owl
type ManagedPackage struct {
	FirstManaged     time.Time `json:"first_managed"`
	LastSeen         time.Time `json:"last_seen"`
	InstalledVersion string    `json:"installed_version,omitempty"`
	AutoInstalled    bool      `json:"auto_installed"`
}

// ManagedLock represents the managed packages lock file
type ManagedLock struct {
	SchemaVersion     string                    `json:"schema_version"`
	Packages          map[string]ManagedPackage `json:"packages"`
	ProtectedPackages []string                  `json:"protected_packages"`
}

// HostStats represents statistics about the host configuration
type HostStats struct {
	Host     string `json:"host"`
	Packages int    `json:"packages"`
}

// SpinnerOptions represents options for spinner configuration
type SpinnerOptions struct {
	Enabled bool
	Color   func(string) string
}

// ListOptions represents options for list formatting
type ListOptions struct {
	Indent   bool
	Numbered bool
	Color    func(string) string
}

// CommandOptions represents common command-line options
type CommandOptions struct {
	NoSpinner  bool
	Verbose    bool
	Debug      bool
	Devel      bool // Check development packages for updates
	UseLibALPM bool // Use libalpm for package operations (default: use yay)
}

// SearchOptions represents options for package searching
type SearchOptions struct {
	AUROnly        bool
	RepoOnly       bool
	AURSearchLimit int
}

// ResolveOptions represents options for dependency resolution
type ResolveOptions struct {
	IncludeOptional bool
	SkipMakeDeps    bool
	ForceReinstall  bool
}

// InstallOptions represents options for package installation
type InstallOptions struct {
	AsDeps     bool
	AsExplicit bool
	NoConfirm  bool
	Needed     bool
}

// UpgradeOptions represents options for system upgrade
type UpgradeOptions struct {
	Devel      bool
	TimeUpdate bool
	NoConfirm  bool
}

// QueryOptions represents options for package querying
type QueryOptions struct {
	Foreign    bool
	Explicit   bool
	Deps       bool
	Unrequired bool
	Search     []string
}

// ProgressBarOptions represents options for progress bar configuration
type ProgressBarOptions struct {
	Enabled bool
	Width   int
	Color   func(string) string
}
