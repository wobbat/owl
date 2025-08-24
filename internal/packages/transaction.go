package packages

import (
	"fmt"
	"os/exec"
	"strings"

	alpm "github.com/Jguer/go-alpm/v2"
)

// TransactionManager handles package operations using pacman commands
// This follows yay's approach of using pacman for actual installations
// while using libalpm for queries and dependency resolution
type TransactionManager struct {
	handle     *alpm.Handle
	pacmanPath string
	configPath string
	sudoPath   string
}

// TransactionOptions represents options for package operations
type TransactionOptions struct {
	NoConfirm      bool     // Skip confirmation prompts
	AsExplicit     bool     // Mark packages as explicitly installed
	AsDeps         bool     // Mark packages as dependencies
	Needed         bool     // Don't reinstall up-to-date packages
	OverwriteFiles []string // Files to overwrite during installation
	IgnorePackages []string // Packages to ignore during operations
	Force          bool     // Force installation
	DownloadOnly   bool     // Only download packages
	PrintOnly      bool     // Only print what would be done
	Recursive      bool     // Remove dependencies (for removal)
	Unneeded       bool     // Remove unneeded packages
	Nosave         bool     // Don't backup configuration files
}

// NewTransactionManager creates a new transaction manager
func NewTransactionManager(handle *alpm.Handle) *TransactionManager {
	return &TransactionManager{
		handle:     handle,
		pacmanPath: "pacman",           // Default pacman path
		configPath: "/etc/pacman.conf", // Default config path
		sudoPath:   "sudo",             // Default sudo path
	}
}

// SetPacmanPath sets the path to the pacman binary
func (tm *TransactionManager) SetPacmanPath(path string) {
	tm.pacmanPath = path
}

// SetConfigPath sets the path to the pacman configuration
func (tm *TransactionManager) SetConfigPath(path string) {
	tm.configPath = path
}

// SetSudoPath sets the path to the sudo binary
func (tm *TransactionManager) SetSudoPath(path string) {
	tm.sudoPath = path
}

// buildPacmanArgs builds common pacman arguments
func (tm *TransactionManager) buildPacmanArgs(options TransactionOptions) []string {
	args := []string{tm.pacmanPath}

	if tm.configPath != "" {
		args = append(args, "--config", tm.configPath)
	}

	if options.NoConfirm {
		args = append(args, "--noconfirm")
	}

	if options.AsExplicit {
		args = append(args, "--asexplicit")
	}

	if options.AsDeps {
		args = append(args, "--asdeps")
	}

	if options.Needed {
		args = append(args, "--needed")
	}

	if options.Force {
		args = append(args, "--force")
	}

	if options.DownloadOnly {
		args = append(args, "-w")
	}

	if options.PrintOnly {
		args = append(args, "--print")
	}

	if options.Recursive {
		args = append(args, "-s")
	}

	if options.Unneeded {
		args = append(args, "-u")
	}

	if options.Nosave {
		args = append(args, "-n")
	}

	for _, file := range options.OverwriteFiles {
		args = append(args, "--overwrite", file)
	}

	for _, pkg := range options.IgnorePackages {
		args = append(args, "--ignore", pkg)
	}

	return args
}

// InstallPackages installs packages using pacman
func (tm *TransactionManager) InstallPackages(packages []string, options TransactionOptions) error {
	if len(packages) == 0 {
		return fmt.Errorf("no packages specified")
	}

	args := tm.buildPacmanArgs(options)
	args = append(args, "-S")
	args = append(args, "--")
	args = append(args, packages...)

	cmd := exec.Command(tm.sudoPath, args...)
	cmd.Stdout = nil // Inherit from parent
	cmd.Stderr = nil // Inherit from parent
	cmd.Stdin = nil  // Inherit from parent

	return cmd.Run()
}

// RemovePackages removes packages using pacman
func (tm *TransactionManager) RemovePackages(packages []string, options TransactionOptions) error {
	if len(packages) == 0 {
		return fmt.Errorf("no packages specified")
	}

	args := tm.buildPacmanArgs(options)
	args = append(args, "-R")
	args = append(args, "--")
	args = append(args, packages...)

	cmd := exec.Command(tm.sudoPath, args...)
	cmd.Stdout = nil // Inherit from parent
	cmd.Stderr = nil // Inherit from parent
	cmd.Stdin = nil  // Inherit from parent

	return cmd.Run()
}

// UpgradeSystem performs a system upgrade using pacman
func (tm *TransactionManager) UpgradeSystem(options TransactionOptions) error {
	args := tm.buildPacmanArgs(options)
	args = append(args, "-Syu")

	cmd := exec.Command(tm.sudoPath, args...)
	cmd.Stdout = nil // Inherit from parent
	cmd.Stderr = nil // Inherit from parent
	cmd.Stdin = nil  // Inherit from parent

	return cmd.Run()
}

// SyncDatabases synchronizes package databases
func (tm *TransactionManager) SyncDatabases(options TransactionOptions) error {
	args := tm.buildPacmanArgs(options)
	args = append(args, "-Sy")

	cmd := exec.Command(tm.sudoPath, args...)
	cmd.Stdout = nil // Inherit from parent
	cmd.Stderr = nil // Inherit from parent
	cmd.Stdin = nil  // Inherit from parent

	return cmd.Run()
}

// CheckUpgrades gets available upgrades using libalpm (read-only operation)
func (tm *TransactionManager) CheckUpgrades(enableDowngrade bool) (map[string]SyncUpgrade, error) {
	upgrades := make(map[string]SyncUpgrade)

	// Initialize a read-only transaction to check for upgrades
	if err := tm.handle.TransInit(alpm.TransFlagNoLock); err != nil {
		return upgrades, fmt.Errorf("failed to initialize transaction: %w", err)
	}

	defer func() {
		if err := tm.handle.TransRelease(); err != nil {
			// Log the error but don't return it since we're in defer
			fmt.Printf("Warning: failed to release transaction: %v\n", err)
		}
	}()

	if err := tm.handle.SyncSysupgrade(enableDowngrade); err != nil {
		return upgrades, fmt.Errorf("failed to prepare system upgrade: %w", err)
	}

	localDB, err := tm.handle.LocalDB()
	if err != nil {
		return upgrades, fmt.Errorf("failed to get local database: %w", err)
	}

	_ = tm.handle.TransGetAdd().ForEach(func(pkg alpm.IPackage) error {
		localVer := "-"
		reason := alpm.PkgReasonExplicit

		if localPkg := localDB.Pkg(pkg.Name()); localPkg != nil {
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

// ValidatePackages checks if packages exist in repositories
func (tm *TransactionManager) ValidatePackages(packages []string) (map[string]bool, error) {
	results := make(map[string]bool)

	syncDBs, err := tm.handle.SyncDBs()
	if err != nil {
		return results, fmt.Errorf("failed to get sync databases: %w", err)
	}

	for _, pkgName := range packages {
		found := false

		// Check if package exists in sync databases
		for _, db := range syncDBs.Slice() {
			if pkg := db.Pkg(pkgName); pkg != nil {
				found = true
				break
			}
		}

		// If not found in sync, check if it satisfies any dependency
		if !found {
			if _, err := syncDBs.FindSatisfier(pkgName); err == nil {
				found = true
			}
		}

		results[pkgName] = found
	}

	return results, nil
}

// GetPackageDependencies gets dependencies for packages
func (tm *TransactionManager) GetPackageDependencies(packages []string) (map[string][]string, error) {
	deps := make(map[string][]string)

	syncDBs, err := tm.handle.SyncDBs()
	if err != nil {
		return deps, fmt.Errorf("failed to get sync databases: %w", err)
	}

	for _, pkgName := range packages {
		var packageDeps []string

		// Find package in sync databases
		for _, db := range syncDBs.Slice() {
			if pkg := db.Pkg(pkgName); pkg != nil {
				if alpmPkg, ok := pkg.(*alpm.Package); ok {
					for _, dep := range alpmPkg.Depends().Slice() {
						packageDeps = append(packageDeps, dep.String())
					}
				}
				break
			}
		}

		deps[pkgName] = packageDeps
	}

	return deps, nil
}

// DryRunInstall shows what would be installed without actually installing
func (tm *TransactionManager) DryRunInstall(packages []string, options TransactionOptions) ([]string, error) {
	// Force print-only mode for dry run
	options.PrintOnly = true

	args := tm.buildPacmanArgs(options)
	args = append(args, "-S")
	args = append(args, "--")
	args = append(args, packages...)

	cmd := exec.Command(args[0], args[1:]...)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to run dry install: %w", err)
	}

	// Parse output to extract package names
	lines := strings.Split(string(output), "\n")
	var resultPackages []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "::") {
			resultPackages = append(resultPackages, line)
		}
	}

	return resultPackages, nil
}

// DryRunRemove shows what would be removed without actually removing
func (tm *TransactionManager) DryRunRemove(packages []string, options TransactionOptions) ([]string, error) {
	// Force print-only mode for dry run
	options.PrintOnly = true

	args := tm.buildPacmanArgs(options)
	args = append(args, "-R")
	args = append(args, "--")
	args = append(args, packages...)

	cmd := exec.Command(args[0], args[1:]...)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to run dry remove: %w", err)
	}

	// Parse output to extract package names
	lines := strings.Split(string(output), "\n")
	var resultPackages []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "::") {
			resultPackages = append(resultPackages, line)
		}
	}

	return resultPackages, nil
}

// InstallPackageFiles installs local package files
func (tm *TransactionManager) InstallPackageFiles(files []string, options TransactionOptions) error {
	if len(files) == 0 {
		return fmt.Errorf("no package files specified")
	}

	args := tm.buildPacmanArgs(options)
	args = append(args, "-U")
	args = append(args, "--")
	args = append(args, files...)

	cmd := exec.Command(tm.sudoPath, args...)
	cmd.Stdout = nil // Inherit from parent
	cmd.Stderr = nil // Inherit from parent
	cmd.Stdin = nil  // Inherit from parent

	return cmd.Run()
}
