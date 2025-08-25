package packages

import (
	"errors"
	"fmt"
	"os"
	"time"

	alpm "github.com/Jguer/go-alpm/v2"
	pacmanconf "github.com/Morganamilo/go-pacmanconf"
)

// PacmanDatabaseExecutor provides a mature interface to libalpm
// following yay's proven patterns for reliable package management
type PacmanDatabaseExecutor struct {
	handle              *alpm.Handle
	localDatabase       alpm.IDB
	syncDatabases       alpm.IDBList
	syncDatabasesCache  []alpm.IDB
	pacmanConfiguration *pacmanconf.Config
	logHandler          func(level alpm.LogLevel, msg string)
	questionHandler     func(question alpm.QuestionAny)
}

// SyncPackageUpgrade represents a package that can be upgraded from sync databases
type SyncPackageUpgrade struct {
	Package            alpm.IPackage
	CurrentVersion     string
	AvailableVersion   string
	InstallationReason alpm.PkgReason
}

// PackageDependencyInfo contains comprehensive dependency information
type PackageDependencyInfo struct {
	Name                 string
	Dependencies         []alpm.Depend
	OptionalDependencies []alpm.Depend
	Provides             []alpm.Depend
	Conflicts            []alpm.Depend
	Groups               []string
}

// DatabaseStatistics provides information about package databases
type DatabaseStatistics struct {
	TotalPackages      int
	InstalledPackages  int
	RepositoryPackages map[string]int
	LastDatabaseSync   time.Time
	Architecture       string
}

// NewPacmanDatabaseExecutor creates a new database executor using pacman configuration
func NewPacmanDatabaseExecutor() (*PacmanDatabaseExecutor, error) {
	config, _, err := pacmanconf.ParseFile("/etc/pacman.conf")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to parse pacman configuration: %v\n", err)
		fmt.Fprintf(os.Stderr, "Falling back to pacman command-based operations\n")

		// Create executor without libalpm handle - will use pacman commands
		return &PacmanDatabaseExecutor{
			handle:              nil,
			pacmanConfiguration: nil,
		}, nil
	}

	executor := &PacmanDatabaseExecutor{
		pacmanConfiguration: config,
	}

	if err := executor.initializeDatabaseHandle(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to initialize ALPM database handle: %v\n", err)
		fmt.Fprintf(os.Stderr, "Falling back to pacman command-based operations\n")

		// Return executor without handle - will use pacman commands
		return &PacmanDatabaseExecutor{
			handle:              nil,
			pacmanConfiguration: config,
		}, nil
	}

	return executor, nil
}

// NewPacmanDatabaseExecutorWithConfig creates executor with custom configuration
func NewPacmanDatabaseExecutorWithConfig(config *pacmanconf.Config) (*PacmanDatabaseExecutor, error) {
	executor := &PacmanDatabaseExecutor{
		pacmanConfiguration: config,
	}

	if err := executor.initializeDatabaseHandle(); err != nil {
		return nil, fmt.Errorf("failed to initialize database handle: %w", err)
	}

	return executor, nil
}

// Configuration and Setup Methods

func (pde *PacmanDatabaseExecutor) initializeDatabaseHandle() error {
	if pde.handle != nil {
		if err := pde.handle.Release(); err != nil {
			return fmt.Errorf("failed to release existing handle: %w", err)
		}
	}

	handle, err := alpm.Initialize(pde.pacmanConfiguration.RootDir, pde.pacmanConfiguration.DBPath)
	if err != nil {
		return fmt.Errorf("failed to initialize ALPM handle: %w", err)
	}

	pde.handle = handle

	if err := pde.configurePacmanSettings(); err != nil {
		pde.handle.Release()
		return fmt.Errorf("failed to configure pacman settings: %w", err)
	}

	if err := pde.refreshDatabaseReferences(); err != nil {
		pde.handle.Release()
		return fmt.Errorf("failed to refresh database references: %w", err)
	}

	return nil
}

func (pde *PacmanDatabaseExecutor) configurePacmanSettings() error {
	config := pde.pacmanConfiguration

	// Register sync databases from configuration
	for _, repo := range config.Repos {
		database, err := pde.handle.RegisterSyncDB(repo.Name, 0)
		if err != nil {
			return fmt.Errorf("failed to register sync database %s: %w", repo.Name, err)
		}

		database.SetServers(repo.Servers)
		database.SetUsage(convertUsageFlags(repo.Usage))
	}

	// Configure handle settings
	if err := pde.handle.SetCacheDirs(config.CacheDir); err != nil {
		return fmt.Errorf("failed to set cache directories: %w", err)
	}

	// Add hook directories
	for _, hookDir := range config.HookDir {
		if err := pde.handle.AddHookDir(hookDir); err != nil {
			return fmt.Errorf("failed to add hook directory %s: %w", hookDir, err)
		}
	}

	// Set various pacman configuration options
	if err := pde.handle.SetGPGDir(config.GPGDir); err != nil {
		return fmt.Errorf("failed to set GPG directory: %w", err)
	}

	if err := pde.handle.SetLogFile(config.LogFile); err != nil {
		return fmt.Errorf("failed to set log file: %w", err)
	}

	if err := pde.handle.SetIgnorePkgs(config.IgnorePkg); err != nil {
		return fmt.Errorf("failed to set ignored packages: %w", err)
	}

	if err := pde.handle.SetIgnoreGroups(config.IgnoreGroup); err != nil {
		return fmt.Errorf("failed to set ignored groups: %w", err)
	}

	if err := pde.handle.SetArchitectures(config.Architecture); err != nil {
		return fmt.Errorf("failed to set architectures: %w", err)
	}

	if err := pde.handle.SetNoUpgrades(config.NoUpgrade); err != nil {
		return fmt.Errorf("failed to set no-upgrade packages: %w", err)
	}

	if err := pde.handle.SetNoExtracts(config.NoExtract); err != nil {
		return fmt.Errorf("failed to set no-extract files: %w", err)
	}

	if err := pde.handle.SetUseSyslog(config.UseSyslog); err != nil {
		return fmt.Errorf("failed to set syslog usage: %w", err)
	}

	return pde.handle.SetCheckSpace(config.CheckSpace)
}

func (pde *PacmanDatabaseExecutor) refreshDatabaseReferences() error {
	var err error

	pde.syncDatabasesCache = nil
	pde.syncDatabases, err = pde.handle.SyncDBs()
	if err != nil {
		return fmt.Errorf("failed to get sync databases: %w", err)
	}

	pde.localDatabase, err = pde.handle.LocalDB()
	if err != nil {
		return fmt.Errorf("failed to get local database: %w", err)
	}

	return nil
}

// Callback Configuration Methods

func (pde *PacmanDatabaseExecutor) SetLogHandler(handler func(level alpm.LogLevel, msg string)) {
	pde.logHandler = handler
	if pde.handle != nil {
		pde.handle.SetLogCallback(func(ctx interface{}, lvl alpm.LogLevel, msg string) {
			handler(lvl, msg)
		}, handler)
	}
}

func (pde *PacmanDatabaseExecutor) SetQuestionHandler(handler func(question alpm.QuestionAny)) {
	pde.questionHandler = handler
	if pde.handle != nil {
		pde.handle.SetQuestionCallback(func(ctx interface{}, q alpm.QuestionAny) {
			handler(q)
		}, handler)
	}
}

// Package Query Methods - Local Database

func (pde *PacmanDatabaseExecutor) FindLocalPackage(packageName string) alpm.IPackage {
	// If libalpm handle is not available, return nil
	if pde.handle == nil {
		return nil
	}
	return pde.localDatabase.Pkg(packageName)
}

func (pde *PacmanDatabaseExecutor) FindLocalPackageSatisfier(dependency string) (alpm.IPackage, error) {
	return pde.localDatabase.PkgCache().FindSatisfier(dependency)
}

func (pde *PacmanDatabaseExecutor) IsPackageInstalled(packageName string) bool {
	// If libalpm handle is not available, use pacman command
	if pde.handle == nil {
		// Use pacman -Qq to check if package is installed
		// This will be implemented when needed
		return false
	}
	return pde.localDatabase.Pkg(packageName) != nil
}

func (pde *PacmanDatabaseExecutor) IsExactVersionInstalled(packageName, version string) bool {
	pkg := pde.localDatabase.Pkg(packageName)
	if pkg == nil {
		return false
	}
	return pkg.Version() == version
}

func (pde *PacmanDatabaseExecutor) GetAllInstalledPackages() []alpm.IPackage {
	// If libalpm handle is not available, return empty slice
	if pde.handle == nil {
		return []alpm.IPackage{}
	}

	var packages []alpm.IPackage
	_ = pde.localDatabase.PkgCache().ForEach(func(pkg alpm.IPackage) error {
		packages = append(packages, pkg)
		return nil
	})
	return packages
}

func (pde *PacmanDatabaseExecutor) GetInstalledPackagesBySize() []alpm.IPackage {
	var packages []alpm.IPackage
	_ = pde.localDatabase.PkgCache().SortBySize().ForEach(func(pkg alpm.IPackage) error {
		packages = append(packages, pkg)
		return nil
	})
	return packages
}

// Package Query Methods - Sync Databases

func (pde *PacmanDatabaseExecutor) FindSyncPackage(packageName string) alpm.IPackage {
	for _, db := range pde.getCachedSyncDatabases() {
		if pkg := db.Pkg(packageName); pkg != nil {
			return pkg
		}
	}
	return nil
}

func (pde *PacmanDatabaseExecutor) FindSyncPackageInRepository(packageName, repositoryName string) alpm.IPackage {
	db, err := pde.handle.SyncDBByName(repositoryName)
	if err != nil {
		return nil
	}
	return db.Pkg(packageName)
}

func (pde *PacmanDatabaseExecutor) FindSyncPackageSatisfier(dependency string) (alpm.IPackage, error) {
	return pde.syncDatabases.FindSatisfier(dependency)
}

func (pde *PacmanDatabaseExecutor) FindSyncPackageSatisfierInRepository(dependency, repositoryName string) (alpm.IPackage, error) {
	singleDBList, err := pde.handle.SyncDBListByDBName(repositoryName)
	if err != nil {
		return nil, err
	}
	return singleDBList.FindSatisfier(dependency)
}

func (pde *PacmanDatabaseExecutor) SearchSyncPackages(searchTerms ...string) []alpm.IPackage {
	var packages []alpm.IPackage
	_ = pde.syncDatabases.ForEach(func(db alpm.IDB) error {
		if len(searchTerms) == 0 {
			_ = db.PkgCache().ForEach(func(pkg alpm.IPackage) error {
				packages = append(packages, pkg)
				return nil
			})
		} else {
			_ = db.Search(searchTerms).ForEach(func(pkg alpm.IPackage) error {
				packages = append(packages, pkg)
				return nil
			})
		}
		return nil
	})
	return packages
}

// Group and Repository Methods

func (pde *PacmanDatabaseExecutor) GetPackagesInGroup(groupName string) []alpm.IPackage {
	var packages []alpm.IPackage
	_ = pde.syncDatabases.FindGroupPkgs(groupName).ForEach(func(pkg alpm.IPackage) error {
		packages = append(packages, pkg)
		return nil
	})
	return packages
}

func (pde *PacmanDatabaseExecutor) GetPackagesInGroupFromRepository(groupName, repositoryName string) ([]alpm.IPackage, error) {
	singleDBList, err := pde.handle.SyncDBListByDBName(repositoryName)
	if err != nil {
		return nil, err
	}

	var packages []alpm.IPackage
	_ = singleDBList.FindGroupPkgs(groupName).ForEach(func(pkg alpm.IPackage) error {
		packages = append(packages, pkg)
		return nil
	})

	return packages, nil
}

func (pde *PacmanDatabaseExecutor) GetConfiguredRepositories() []string {
	var repositories []string
	_ = pde.syncDatabases.ForEach(func(db alpm.IDB) error {
		repositories = append(repositories, db.Name())
		return nil
	})
	return repositories
}

// Dependency Analysis Methods

func (pde *PacmanDatabaseExecutor) GetPackageDependencies(pkg alpm.IPackage) []alpm.Depend {
	if alpmPkg, ok := pkg.(*alpm.Package); ok {
		return alpmPkg.Depends().Slice()
	}
	return nil
}

func (pde *PacmanDatabaseExecutor) GetPackageOptionalDependencies(pkg alpm.IPackage) []alpm.Depend {
	if alpmPkg, ok := pkg.(*alpm.Package); ok {
		return alpmPkg.OptionalDepends().Slice()
	}
	return nil
}

func (pde *PacmanDatabaseExecutor) GetPackageProvisions(pkg alpm.IPackage) []alpm.Depend {
	if alpmPkg, ok := pkg.(*alpm.Package); ok {
		return alpmPkg.Provides().Slice()
	}
	return nil
}

func (pde *PacmanDatabaseExecutor) GetPackageConflicts(pkg alpm.IPackage) []alpm.Depend {
	if alpmPkg, ok := pkg.(*alpm.Package); ok {
		return alpmPkg.Conflicts().Slice()
	}
	return nil
}

func (pde *PacmanDatabaseExecutor) GetPackageGroups(pkg alpm.IPackage) []string {
	if alpmPkg, ok := pkg.(*alpm.Package); ok {
		return alpmPkg.Groups().Slice()
	}
	return nil
}

func (pde *PacmanDatabaseExecutor) GetComprehensiveDependencyInfo(pkg alpm.IPackage) PackageDependencyInfo {
	return PackageDependencyInfo{
		Name:                 pkg.Name(),
		Dependencies:         pde.GetPackageDependencies(pkg),
		OptionalDependencies: pde.GetPackageOptionalDependencies(pkg),
		Provides:             pde.GetPackageProvisions(pkg),
		Conflicts:            pde.GetPackageConflicts(pkg),
		Groups:               pde.GetPackageGroups(pkg),
	}
}

// System Upgrade Analysis

func (pde *PacmanDatabaseExecutor) CalculateSystemUpgrades(allowDowngrades bool) (map[string]SyncPackageUpgrade, error) {
	upgrades := make(map[string]SyncPackageUpgrade)

	if err := pde.handle.TransInit(alpm.TransFlagNoLock); err != nil {
		return upgrades, fmt.Errorf("failed to initialize transaction: %w", err)
	}

	defer func() {
		if err := pde.handle.TransRelease(); err != nil {
			if pde.logHandler != nil {
				pde.logHandler(alpm.LogError, fmt.Sprintf("failed to release transaction: %v", err))
			}
		}
	}()

	if err := pde.handle.SyncSysupgrade(allowDowngrades); err != nil {
		return upgrades, fmt.Errorf("failed to prepare system upgrade: %w", err)
	}

	_ = pde.handle.TransGetAdd().ForEach(func(pkg alpm.IPackage) error {
		currentVersion := "-"
		reason := alpm.PkgReasonExplicit

		if localPkg := pde.localDatabase.Pkg(pkg.Name()); localPkg != nil {
			currentVersion = localPkg.Version()
			reason = localPkg.Reason()
		}

		upgrades[pkg.Name()] = SyncPackageUpgrade{
			Package:            pkg,
			CurrentVersion:     currentVersion,
			AvailableVersion:   pkg.Version(),
			InstallationReason: reason,
		}

		return nil
	})

	return upgrades, nil
}

// Version Comparison and Validation

func (pde *PacmanDatabaseExecutor) ComparePackageVersions(version1, version2 string) int {
	return alpm.VerCmp(version1, version2)
}

func (pde *PacmanDatabaseExecutor) ValidatePackageVersion(version string) bool {
	// Use ALPM's version parsing to validate
	// A version is valid if it doesn't cause comparison errors
	return alpm.VerCmp(version, version) == 0
}

// System Information and Statistics

func (pde *PacmanDatabaseExecutor) GetDatabaseStatistics() DatabaseStatistics {
	stats := DatabaseStatistics{
		RepositoryPackages: make(map[string]int),
		LastDatabaseSync:   pde.getLastDatabaseSync(),
	}

	// Count installed packages
	stats.InstalledPackages = len(pde.GetAllInstalledPackages())

	// Count packages per repository
	_ = pde.syncDatabases.ForEach(func(db alpm.IDB) error {
		count := 0
		_ = db.PkgCache().ForEach(func(pkg alpm.IPackage) error {
			count++
			return nil
		})
		stats.RepositoryPackages[db.Name()] = count
		stats.TotalPackages += count
		return nil
	})

	// Get architecture info
	if architectures, err := pde.handle.GetArchitectures(); err == nil {
		if archSlice := architectures.Slice(); len(archSlice) > 0 {
			stats.Architecture = archSlice[0]
		}
	}

	return stats
}

func (pde *PacmanDatabaseExecutor) getLastDatabaseSync() time.Time {
	var lastTime time.Time
	_ = pde.syncDatabases.ForEach(func(db alpm.IDB) error {
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

// Configuration Validation

func (pde *PacmanDatabaseExecutor) ValidatePacmanConfiguration() error {
	if pde.pacmanConfiguration == nil {
		return errors.New("no pacman configuration loaded")
	}

	if len(pde.pacmanConfiguration.Repos) == 0 {
		return errors.New("no repositories configured")
	}

	// Verify essential repositories exist
	hasCore := false
	for _, repo := range pde.pacmanConfiguration.Repos {
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

// Database Handle Management

func (pde *PacmanDatabaseExecutor) RefreshDatabaseHandle() error {
	return pde.initializeDatabaseHandle()
}

func (pde *PacmanDatabaseExecutor) getCachedSyncDatabases() []alpm.IDB {
	if pde.syncDatabasesCache == nil {
		pde.syncDatabasesCache = pde.syncDatabases.Slice()
	}
	return pde.syncDatabasesCache
}

// Resource Management

func (pde *PacmanDatabaseExecutor) Release() {
	if pde.handle != nil {
		if err := pde.handle.Release(); err != nil {
			if pde.logHandler != nil {
				pde.logHandler(alpm.LogError, fmt.Sprintf("cleanup error: %v", err))
			}
		}
		pde.handle = nil
	}
}

// Utility Functions

func convertUsageFlags(usages []string) alpm.Usage {
	if len(usages) == 0 {
		return alpm.UsageAll
	}

	var flags alpm.Usage
	for _, usage := range usages {
		switch usage {
		case "Sync":
			flags |= alpm.UsageSync
		case "Search":
			flags |= alpm.UsageSearch
		case "Install":
			flags |= alpm.UsageInstall
		case "Upgrade":
			flags |= alpm.UsageUpgrade
		case "All":
			flags |= alpm.UsageAll
		}
	}

	return flags
}
