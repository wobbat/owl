package owl

import (
	"context"
)

// Reporter abstracts user-facing output. Library code should not print directly.
type Reporter interface {
	Info(msg string)
	Warn(msg string)
	Error(msg string)
	Success(msg string)
}

// Options configures a Manager instance.
type Options struct {
	HomeDir   string // override home directory (useful for tests)
	NoSpinner bool
	Verbose   bool
	Debug     bool
}

// ApplyOptions defines options for Apply.
type ApplyOptions struct {
	DryRun bool
	Devel  bool // include development packages when relevant
}

// UpgradeOptions defines options for Upgrade.
type UpgradeOptions struct {
	Devel      bool
	TimeUpdate bool
}

// DotfilesOptions defines options for syncing dotfiles.
type DotfilesOptions struct {
	DryRun bool
}

// InstallOptions defines options for installing packages.
type InstallOptions struct {
	AsDeps     bool
	AsExplicit bool
	NoConfirm  bool
	Needed     bool
}

// QueryOptions defines filters for querying packages.
type QueryOptions struct {
	Foreign    bool
	Explicit   bool
	Deps       bool
	Unrequired bool
	Search     []string
}

// SearchOptions defines options for searching packages.
type SearchOptions struct {
	AUROnly        bool
	RepoOnly       bool
	AURSearchLimit int
}

// Results structures (initial minimal versions; can expand)

type ApplyResult struct {
	PackagesInstalled int
	PackagesRemoved   int
	DotfilesProcessed int
}

type UpgradeResult struct {
	PackagesUpgraded int
}

type UninstallResult struct {
	PackagesRemoved int
}

type DotfilesResult struct {
	DotfilesProcessed int
}

type VCSResult struct {
	PackagesIndexed int
}

type InstallResult struct {
	PackagesInstalled int
}

type SearchResult struct {
	Name        string
	Version     string
	Description string
	Repository  string
	Votes       int
	Popularity  float64
	Installed   bool
	InConfig    bool
	OutOfDate   bool
}

type PackageInfo struct {
	Name        string
	Version     string
	Description string
	Repository  string
	URL         string
	Maintainer  string
	Votes       int
	Popularity  float64
	OutOfDate   bool
	Installed   bool
}

// Manager defines the high-level façade API.
type Manager interface {
	Apply(ctx context.Context, opt ApplyOptions) (*ApplyResult, error)
	Upgrade(ctx context.Context, opt UpgradeOptions) (*UpgradeResult, error)
	Uninstall(ctx context.Context) (*UninstallResult, error)
	GenerateVCS(ctx context.Context) (*VCSResult, error)
	SyncDotfiles(ctx context.Context, opt DotfilesOptions) (*DotfilesResult, error)
	Search(ctx context.Context, terms []string, opt SearchOptions) ([]SearchResult, error)
	Install(ctx context.Context, pkgs []string, opt InstallOptions) (*InstallResult, error)
	Query(ctx context.Context, opt QueryOptions) ([]string, error)
	Info(ctx context.Context, name string) (*PackageInfo, error)
}

// manager implements Manager.
type manager struct {
	opts     Options
	reporter Reporter
}

// NewManager creates a new Manager instance.
func NewManager(opts Options, r Reporter) Manager {
	return &manager{opts: opts, reporter: r}
}

// helper to emit debug messages
func (m *manager) debug(msg string) {
	if m.opts.Debug && m.reporter != nil {
		m.reporter.Info("[debug] " + msg)
	}
}

// Stub implementations (to be filled during refactor)

func (m *manager) Apply(ctx context.Context, opt ApplyOptions) (*ApplyResult, error) {
	// logic moved from handlers.HandleApplyCommand later
	return &ApplyResult{}, nil
}

func (m *manager) Upgrade(ctx context.Context, opt UpgradeOptions) (*UpgradeResult, error) {
	return &UpgradeResult{}, nil
}

func (m *manager) Uninstall(ctx context.Context) (*UninstallResult, error) {
	return &UninstallResult{}, nil
}

func (m *manager) GenerateVCS(ctx context.Context) (*VCSResult, error) {
	return &VCSResult{}, nil
}

func (m *manager) SyncDotfiles(ctx context.Context, opt DotfilesOptions) (*DotfilesResult, error) {
	return &DotfilesResult{}, nil
}

func (m *manager) Search(ctx context.Context, terms []string, opt SearchOptions) ([]SearchResult, error) {
	return nil, nil
}

func (m *manager) Install(ctx context.Context, pkgs []string, opt InstallOptions) (*InstallResult, error) {
	return &InstallResult{}, nil
}

func (m *manager) Query(ctx context.Context, opt QueryOptions) ([]string, error) {
	return nil, nil
}

func (m *manager) Info(ctx context.Context, name string) (*PackageInfo, error) {
	return nil, nil
}
