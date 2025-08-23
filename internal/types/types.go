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

// PackageInfo represents information about a package
type PackageInfo struct {
	Name             string `json:"name"`
	InstalledVersion string `json:"installed_version,omitempty"`
	AvailableVersion string `json:"available_version,omitempty"`
	Status           string `json:"status"` // not_installed, up_to_date, outdated
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
	NoSpinner bool
	Verbose   bool
	Debug     bool
}
