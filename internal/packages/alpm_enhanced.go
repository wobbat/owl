package packages

import (
	"errors"
	"fmt"
	"time"

	alpm "github.com/Jguer/go-alpm/v2"
	pacmanconf "github.com/Morganamilo/go-pacmanconf"
)

// EnhancedALPMManager provides a more mature interface to libalpm
// based on patterns from yay's implementation
type EnhancedALPMManager struct {
	handle           *alpm.Handle
	localDB          alpm.IDB
	syncDB           alpm.IDBList
	syncDBsCache     []alpm.IDB
	conf             *pacmanconf.Config
	logCallback      func(level alpm.LogLevel, msg string)
	questionCallback func(question alpm.QuestionAny)
}

// SyncUpgrade represents a package that can be upgraded
type SyncUpgrade struct {
	Package      alpm.IPackage
	LocalVersion string
	Reason       alpm.PkgReason
}

// NewEnhancedALPMManager creates a new enhanced ALPM manager
func NewEnhancedALPMManager() (*EnhancedALPMManager, error) {
	// Parse pacman configuration using proper library
	conf, _, err := pacmanconf.ParseFile("/etc/pacman.conf")
	if err != nil {
		return nil, fmt.Errorf("failed to parse pacman config: %w", err)
	}

	manager := &EnhancedALPMManager{
		conf: conf,
	}

	if err := manager.RefreshHandle(); err != nil {
		return nil, fmt.Errorf("failed to initialize ALPM handle: %w", err)
	}

	return manager, nil
}

// toUsage converts pacman config usage strings to alpm.Usage flags
func toUsage(usages []string) alpm.Usage {
	if len(usages) == 0 {
		return alpm.UsageAll
	}

	var ret alpm.Usage
	for _, usage := range usages {
		switch usage {
		case "Sync":
			ret |= alpm.UsageSync
		case "Search":
			ret |= alpm.UsageSearch
		case "Install":
			ret |= alpm.UsageInstall
		case "Upgrade":
			ret |= alpm.UsageUpgrade
		case "All":
			ret |= alpm.UsageAll
		}
	}

	return ret
}

// configureALPM sets up the ALPM handle with pacman configuration
func (e *EnhancedALPMManager) configureALPM() error {
	for _, repo := range e.conf.Repos {
		alpmDB, err := e.handle.RegisterSyncDB(repo.Name, 0)
		if err != nil {
			return fmt.Errorf("failed to register sync database %s: %w", repo.Name, err)
		}

		alpmDB.SetServers(repo.Servers)
		alpmDB.SetUsage(toUsage(repo.Usage))
	}

	if err := e.handle.SetCacheDirs(e.conf.CacheDir); err != nil {
		return fmt.Errorf("failed to set cache dirs: %w", err)
	}

	// Add hook directories one by one to avoid overwriting system directory
	for _, dir := range e.conf.HookDir {
		if err := e.handle.AddHookDir(dir); err != nil {
			return fmt.Errorf("failed to add hook dir %s: %w", dir, err)
		}
	}

	if err := e.handle.SetGPGDir(e.conf.GPGDir); err != nil {
		return fmt.Errorf("failed to set GPG dir: %w", err)
	}

	if err := e.handle.SetLogFile(e.conf.LogFile); err != nil {
		return fmt.Errorf("failed to set log file: %w", err)
	}

	if err := e.handle.SetIgnorePkgs(e.conf.IgnorePkg); err != nil {
		return fmt.Errorf("failed to set ignore packages: %w", err)
	}

	if err := e.handle.SetIgnoreGroups(e.conf.IgnoreGroup); err != nil {
		return fmt.Errorf("failed to set ignore groups: %w", err)
	}

	if err := e.handle.SetArchitectures(e.conf.Architecture); err != nil {
		return fmt.Errorf("failed to set architectures: %w", err)
	}

	if err := e.handle.SetNoUpgrades(e.conf.NoUpgrade); err != nil {
		return fmt.Errorf("failed to set no upgrades: %w", err)
	}

	if err := e.handle.SetNoExtracts(e.conf.NoExtract); err != nil {
		return fmt.Errorf("failed to set no extracts: %w", err)
	}

	if err := e.handle.SetUseSyslog(e.conf.UseSyslog); err != nil {
		return fmt.Errorf("failed to set use syslog: %w", err)
	}

	return e.handle.SetCheckSpace(e.conf.CheckSpace)
}

// SetLogCallback sets the logging callback for ALPM
func (e *EnhancedALPMManager) SetLogCallback(callback func(level alpm.LogLevel, msg string)) {
	e.logCallback = callback
	if e.handle != nil {
		e.handle.SetLogCallback(func(ctx interface{}, lvl alpm.LogLevel, msg string) {
			callback(lvl, msg)
		}, callback)
	}
}

// SetQuestionCallback sets the question callback for ALPM
func (e *EnhancedALPMManager) SetQuestionCallback(callback func(question alpm.QuestionAny)) {
	e.questionCallback = callback
	if e.handle != nil {
		e.handle.SetQuestionCallback(func(ctx interface{}, q alpm.QuestionAny) {
			callback(q)
		}, callback)
	}
}

// RefreshHandle refreshes the ALPM handle (useful after config changes)
func (e *EnhancedALPMManager) RefreshHandle() error {
	if e.handle != nil {
		if err := e.handle.Release(); err != nil {
			return fmt.Errorf("failed to release existing handle: %w", err)
		}
	}

	handle, err := alpm.Initialize(e.conf.RootDir, e.conf.DBPath)
	if err != nil {
		return fmt.Errorf("failed to initialize ALPM: %w", err)
	}

	e.handle = handle

	if err := e.configureALPM(); err != nil {
		e.handle.Release()
		return fmt.Errorf("failed to configure ALPM: %w", err)
	}

	// Set callbacks if they exist
	if e.logCallback != nil {
		e.SetLogCallback(e.logCallback)
	}
	if e.questionCallback != nil {
		e.SetQuestionCallback(e.questionCallback)
	}

	// Refresh database references
	e.syncDBsCache = nil
	e.syncDB, err = e.handle.SyncDBs()
	if err != nil {
		return fmt.Errorf("failed to get sync databases: %w", err)
	}

	e.localDB, err = e.handle.LocalDB()
	if err != nil {
		return fmt.Errorf("failed to get local database: %w", err)
	}

	return nil
}

// LocalSatisfierExists checks if a local package satisfies the given dependency
func (e *EnhancedALPMManager) LocalSatisfierExists(pkgName string) bool {
	_, err := e.localDB.PkgCache().FindSatisfier(pkgName)
	return err == nil
}

// SyncSatisfierExists checks if a sync package satisfies the given dependency
func (e *EnhancedALPMManager) SyncSatisfierExists(pkgName string) bool {
	_, err := e.syncDB.FindSatisfier(pkgName)
	return err == nil
}

// SyncSatisfier finds a package in sync databases that satisfies the dependency
func (e *EnhancedALPMManager) SyncSatisfier(pkgName string) alpm.IPackage {
	pkg, err := e.syncDB.FindSatisfier(pkgName)
	if err != nil {
		return nil
	}
	return pkg
}

// LocalPackage gets a local package by name
func (e *EnhancedALPMManager) LocalPackage(pkgName string) alpm.IPackage {
	return e.localDB.Pkg(pkgName)
}

// SyncPackage gets a sync package by name from any sync database
func (e *EnhancedALPMManager) SyncPackage(pkgName string) alpm.IPackage {
	for _, db := range e.syncDBs() {
		if pkg := db.Pkg(pkgName); pkg != nil {
			return pkg
		}
	}
	return nil
}

// SyncPackageFromDB gets a sync package by name from a specific database
func (e *EnhancedALPMManager) SyncPackageFromDB(pkgName, dbName string) alpm.IPackage {
	db, err := e.handle.SyncDBByName(dbName)
	if err != nil {
		return nil
	}
	return db.Pkg(pkgName)
}

// LocalPackages returns all locally installed packages
func (e *EnhancedALPMManager) LocalPackages() []alpm.IPackage {
	var packages []alpm.IPackage
	_ = e.localDB.PkgCache().ForEach(func(pkg alpm.IPackage) error {
		packages = append(packages, pkg)
		return nil
	})
	return packages
}

// SyncPackages returns packages from sync databases
func (e *EnhancedALPMManager) SyncPackages(searchTerms ...string) []alpm.IPackage {
	var packages []alpm.IPackage
	_ = e.syncDB.ForEach(func(db alpm.IDB) error {
		if len(searchTerms) == 0 {
			// Return all packages if no search terms
			_ = db.PkgCache().ForEach(func(pkg alpm.IPackage) error {
				packages = append(packages, pkg)
				return nil
			})
		} else {
			// Search for packages matching terms
			_ = db.Search(searchTerms).ForEach(func(pkg alpm.IPackage) error {
				packages = append(packages, pkg)
				return nil
			})
		}
		return nil
	})
	return packages
}

// PackagesFromGroup returns all packages in a group
func (e *EnhancedALPMManager) PackagesFromGroup(groupName string) []alpm.IPackage {
	var packages []alpm.IPackage
	_ = e.syncDB.FindGroupPkgs(groupName).ForEach(func(pkg alpm.IPackage) error {
		packages = append(packages, pkg)
		return nil
	})
	return packages
}

// PackageDepends returns the dependencies of a package
func (e *EnhancedALPMManager) PackageDepends(pkg alpm.IPackage) []alpm.Depend {
	if alpmPkg, ok := pkg.(*alpm.Package); ok {
		return alpmPkg.Depends().Slice()
	}
	return nil
}

// PackageOptionalDepends returns the optional dependencies of a package
func (e *EnhancedALPMManager) PackageOptionalDepends(pkg alpm.IPackage) []alpm.Depend {
	if alpmPkg, ok := pkg.(*alpm.Package); ok {
		return alpmPkg.OptionalDepends().Slice()
	}
	return nil
}

// PackageProvides returns what a package provides
func (e *EnhancedALPMManager) PackageProvides(pkg alpm.IPackage) []alpm.Depend {
	if alpmPkg, ok := pkg.(*alpm.Package); ok {
		return alpmPkg.Provides().Slice()
	}
	return nil
}

// SyncUpgrades gets available upgrades using ALPM's system upgrade logic
func (e *EnhancedALPMManager) SyncUpgrades(enableDowngrade bool) (map[string]SyncUpgrade, error) {
	upgrades := make(map[string]SyncUpgrade)

	if err := e.handle.TransInit(alpm.TransFlagNoLock); err != nil {
		return upgrades, fmt.Errorf("failed to initialize transaction: %w", err)
	}

	defer func() {
		if err := e.handle.TransRelease(); err != nil {
			// Log error but don't return it since we're in defer
			if e.logCallback != nil {
				e.logCallback(alpm.LogError, fmt.Sprintf("failed to release transaction: %v", err))
			}
		}
	}()

	if err := e.handle.SyncSysupgrade(enableDowngrade); err != nil {
		return upgrades, fmt.Errorf("failed to prepare system upgrade: %w", err)
	}

	_ = e.handle.TransGetAdd().ForEach(func(pkg alpm.IPackage) error {
		localVer := "-"
		reason := alpm.PkgReasonExplicit

		if localPkg := e.localDB.Pkg(pkg.Name()); localPkg != nil {
			localVer = localPkg.Version()
			reason = localPkg.Reason()
		}

		upgrades[pkg.Name()] = SyncUpgrade{
			Package:      pkg,
			LocalVersion: localVer,
			Reason:       reason,
		}

		return nil
	})

	return upgrades, nil
}

// IsCorrectVersionInstalled checks if the exact version is installed
func (e *EnhancedALPMManager) IsCorrectVersionInstalled(pkgName, version string) bool {
	pkg := e.localDB.Pkg(pkgName)
	if pkg == nil {
		return false
	}
	return pkg.Version() == version
}

// BiggestPackages returns packages sorted by size (largest first)
func (e *EnhancedALPMManager) BiggestPackages() []alpm.IPackage {
	var packages []alpm.IPackage
	_ = e.localDB.PkgCache().SortBySize().ForEach(func(pkg alpm.IPackage) error {
		packages = append(packages, pkg)
		return nil
	})
	return packages
}

// LastBuildTime returns the most recent build time across all sync packages
func (e *EnhancedALPMManager) LastBuildTime() time.Time {
	var lastTime time.Time
	_ = e.syncDB.ForEach(func(db alpm.IDB) error {
		_ = db.PkgCache().ForEach(func(pkg alpm.IPackage) error {
			if buildTime := pkg.BuildDate(); buildTime.After(lastTime) {
				lastTime = buildTime
			}
			return nil
		})
		return nil
	})
	return lastTime
}

// Repos returns a list of configured repository names
func (e *EnhancedALPMManager) Repos() []string {
	var repos []string
	_ = e.syncDB.ForEach(func(db alpm.IDB) error {
		repos = append(repos, db.Name())
		return nil
	})
	return repos
}

// AlpmArchitectures returns the configured architectures
func (e *EnhancedALPMManager) AlpmArchitectures() ([]string, error) {
	architectures, err := e.handle.GetArchitectures()
	if err != nil {
		return nil, err
	}
	return architectures.Slice(), nil
}

// syncDBs returns cached sync databases
func (e *EnhancedALPMManager) syncDBs() []alpm.IDB {
	if e.syncDBsCache == nil {
		e.syncDBsCache = e.syncDB.Slice()
	}
	return e.syncDBsCache
}

// Cleanup releases ALPM resources
func (e *EnhancedALPMManager) Cleanup() {
	if e.handle != nil {
		if err := e.handle.Release(); err != nil {
			if e.logCallback != nil {
				e.logCallback(alpm.LogError, fmt.Sprintf("cleanup error: %v", err))
			}
		}
		e.handle = nil
	}
}

// VerCmp performs version comparison using ALPM's version comparison
func (e *EnhancedALPMManager) VerCmp(v1, v2 string) int {
	return alpm.VerCmp(v1, v2)
}

// CheckDatabaseUpdate checks if databases need to be updated
func (e *EnhancedALPMManager) CheckDatabaseUpdate() (bool, error) {
	// This is a simplified check - in practice, you'd compare
	// database modification times or other indicators
	lastUpdate := e.LastBuildTime()
	if lastUpdate.IsZero() {
		return true, nil // Assume update needed if no build time found
	}

	// Consider databases stale if older than 24 hours
	return time.Since(lastUpdate) > 24*time.Hour, nil
}

// GetPackageInfo returns comprehensive package information
func (e *EnhancedALPMManager) GetPackageInfo(pkgName string) (*AURPackage, error) {
	// Try sync packages first
	if syncPkg := e.SyncPackage(pkgName); syncPkg != nil {
		info := &AURPackage{
			Name:        syncPkg.Name(),
			Version:     syncPkg.Version(),
			Description: syncPkg.Description(),
			URL:         syncPkg.URL(),
		}

		return info, nil
	}

	// Try local packages
	if localPkg := e.LocalPackage(pkgName); localPkg != nil {
		return &AURPackage{
			Name:        localPkg.Name(),
			Version:     localPkg.Version(),
			Description: localPkg.Description(),
			URL:         localPkg.URL(),
		}, nil
	}

	return nil, fmt.Errorf("package %s not found", pkgName)
}

// ValidatePacmanConfig validates the current pacman configuration
func (e *EnhancedALPMManager) ValidatePacmanConfig() error {
	if e.conf == nil {
		return errors.New("no pacman configuration loaded")
	}

	if len(e.conf.Repos) == 0 {
		return errors.New("no repositories configured")
	}

	// Check if essential repositories exist
	hasCore := false
	for _, repo := range e.conf.Repos {
		if repo.Name == "core" {
			hasCore = true
			break
		}
	}

	if !hasCore {
		return errors.New("core repository not found in configuration")
	}

	return nil
}
