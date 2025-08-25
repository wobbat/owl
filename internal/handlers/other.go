package handlers

import (
	"fmt"
	"os"
	"os/exec"

	"owl/internal/packages"
	"owl/internal/types"
	"owl/internal/ui"
)

// HandleUpgradeCommand handles the upgrade command
func HandleUpgradeCommand(options *types.CommandOptions) error {
	globalUI := ui.NewUI()

	// Show header like TypeScript version
	globalUI.Header("Upgrade")

	// Analyze packages first
	analysisSpinner := ui.NewSpinner("Analyzing system packages...", types.SpinnerOptions{
		Enabled: !options.NoSpinner,
	})

	// Get package manager based on flag
	packageManager, err := packages.NewPackageManager(options.UseLibALPM)
	if err != nil {
		analysisSpinner.Fail("Failed to initialize package manager")
		return fmt.Errorf("failed to initialize package manager: %w", err)
	}
	defer packageManager.Release()

	var outdatedPackages []string

	if options.UseLibALPM {
		// Use ALPM manager for detailed AUR and official package checking
		alpmMgr, err := packages.NewALPMManager()
		if err != nil {
			analysisSpinner.Fail("Failed to initialize ALPM manager")
			return fmt.Errorf("failed to initialize ALPM manager: %w", err)
		}
		defer alpmMgr.Release()

		// Get AUR updates with devel checking if enabled
		aurUpdates, err := alpmMgr.GetAURUpdatesWithDevel(options.Devel)
		if err != nil {
			analysisSpinner.Fail("Failed to check AUR packages")
			return fmt.Errorf("failed to check AUR updates: %w", err)
		}

		// Filter packages that need updates
		for _, update := range aurUpdates {
			if update.NeedsUpdate {
				outdatedPackages = append(outdatedPackages, update.Name)
			}
		}

		// Also get outdated official packages
		officialOutdated, err := alpmMgr.GetOutdatedPackages()
		if err != nil {
			analysisSpinner.Fail("Failed to check official packages")
			return fmt.Errorf("failed to check official packages: %w", err)
		}

		// Combine both lists
		outdatedPackages = append(outdatedPackages, officialOutdated...)
	} else {
		// Use generic package manager for basic functionality
		outdatedPackages, err = packageManager.GetOutdatedPackages()
		if err != nil {
			analysisSpinner.Fail("Failed to check for outdated packages")
			return fmt.Errorf("failed to check for outdated packages: %w", err)
		}
	}

	analysisSpinner.Stop(fmt.Sprintf("Found %d packages to upgrade", len(outdatedPackages)))

	if len(outdatedPackages) == 0 {
		globalUI.Ok("All packages are up to date")
		return nil
	}

	// Show overview like TypeScript version
	hostname, _ := os.Hostname()
	globalUI.Overview(types.HostStats{
		Host:     hostname,
		Packages: len(outdatedPackages),
	})

	// Show packages to upgrade
	fmt.Println("Packages to upgrade:")
	for _, pkg := range outdatedPackages {
		fmt.Printf("  %s %s\n", ui.Icon.Upgrade, fmt.Sprintf("\x1b[37m%s\x1b[0m", pkg))
	}
	fmt.Println()

	// Upgrade packages
	upgradeSpinner := ui.NewSpinner(fmt.Sprintf("Upgrading %d packages...", len(outdatedPackages)), types.SpinnerOptions{
		Enabled: !options.NoSpinner && !options.Verbose,
	})

	err = packageManager.UpgradeSystem(options.Verbose)
	if err != nil {
		if !options.NoSpinner && !options.Verbose {
			upgradeSpinner.Fail("System upgrade failed")
		}
		return fmt.Errorf("system upgrade failed: %w", err)
	}

	if !options.NoSpinner && !options.Verbose {
		upgradeSpinner.Stop("System upgrade completed successfully")
	}
	globalUI.Celebration("All packages upgraded!")
	return nil
}

// HandleUninstallCommand handles the uninstall command
func HandleUninstallCommand(options *types.CommandOptions) error {
	globalUI := ui.NewUI()

	// Show header like TypeScript version
	globalUI.Header("Uninstall")

	// Load managed packages
	managedLock, err := packages.LoadManagedPackages()
	if err != nil {
		return fmt.Errorf("failed to load managed packages: %w", err)
	}

	if len(managedLock.Packages) == 0 {
		globalUI.Ok("No managed packages found to uninstall")
		return nil
	}

	// Collect package names
	var packageNames []string
	for packageName := range managedLock.Packages {
		packageNames = append(packageNames, packageName)
	}

	// Show packages that will be removed (like TypeScript version)
	fmt.Println("Managed packages to remove:")
	for _, pkg := range packageNames {
		fmt.Printf("  %s %s\n", ui.Icon.Remove, fmt.Sprintf("\x1b[37m%s\x1b[0m", pkg))
	}
	fmt.Println()
	fmt.Printf("This will remove %d packages managed by Owl.\n", len(packageNames))

	// Get user confirmation
	confirmed, err := globalUI.ConfirmAction("Continue?")
	if err != nil {
		return fmt.Errorf("failed to get user confirmation: %w", err)
	}

	if !confirmed {
		fmt.Println("Uninstall cancelled")
		return nil
	}

	fmt.Println("Removing managed packages...")

	// Get package manager based on flag
	packageManager, err := packages.NewPackageManager(options.UseLibALPM)
	if err != nil {
		return fmt.Errorf("failed to initialize package manager: %w", err)
	}
	defer packageManager.Release()

	// Remove packages one by one using the selected package manager
	for _, pkgName := range packageNames {
		if err := packageManager.RemovePackage(pkgName); err != nil {
			return fmt.Errorf("failed to remove package %s: %w", pkgName, err)
		}
	}

	// Clear the managed packages file
	emptyLock := &types.ManagedLock{
		SchemaVersion:     "1.0",
		Packages:          make(map[string]types.ManagedPackage),
		ProtectedPackages: []string{},
	}

	err = packages.SaveManagedPackages(emptyLock)
	if err != nil {
		return fmt.Errorf("failed to clear managed packages: %w", err)
	}

	globalUI.Celebration("All managed packages removed successfully!")
	return nil
}

// HandleGendbCommand handles the generate VCS database command
func HandleGendbCommand(globalUI *ui.UI) error {
	globalUI.Header("Generate VCS Database")

	// Create VCS store
	vcsStore, err := packages.NewVCSStore()
	if err != nil {
		return fmt.Errorf("failed to initialize VCS store: %w", err)
	}

	// Get ALPM manager
	alpmMgr, err := packages.NewALPMManager()
	if err != nil {
		return fmt.Errorf("failed to initialize ALPM manager: %w", err)
	}

	analysisSpinner := ui.NewSpinner("Analyzing installed AUR packages for VCS sources...", types.SpinnerOptions{
		Enabled: true,
	})

	// Get AUR packages
	aurPackages, err := alpmMgr.GetAURPackages()
	if err != nil {
		analysisSpinner.Fail("Failed to get AUR packages")
		return fmt.Errorf("failed to get AUR packages: %w", err)
	}

	var vcsPackages []string
	var processedCount int

	for _, pkgName := range aurPackages {
		if packages.IsGitPackage(pkgName) {
			vcsPackages = append(vcsPackages, pkgName)
		}
		processedCount++
	}

	analysisSpinner.Stop(fmt.Sprintf("Found %d VCS packages out of %d AUR packages", len(vcsPackages), len(aurPackages)))

	if len(vcsPackages) == 0 {
		globalUI.Ok("No VCS packages found")
		return nil
	}

	// Generate database for each VCS package
	generateSpinner := ui.NewSpinner("Generating VCS database...", types.SpinnerOptions{
		Enabled: true,
	})

	var generatedCount int
	for _, pkgName := range vcsPackages {
		// Download PKGBUILD and extract VCS info
		if err := generateVCSInfoForPackage(pkgName, vcsStore); err != nil {
			// Don't fail the whole operation for individual packages
			continue
		}
		generatedCount++
	}

	generateSpinner.Stop(fmt.Sprintf("Generated VCS database for %d packages", generatedCount))

	if err := vcsStore.Save(); err != nil {
		return fmt.Errorf("failed to save VCS database: %w", err)
	}

	globalUI.Success(fmt.Sprintf("VCS database generated for %d development packages", generatedCount))
	globalUI.Info("Development package updates can now be checked with future upgrade commands")
	return nil
}

// generateVCSInfoForPackage downloads PKGBUILD and extracts VCS info for a package
func generateVCSInfoForPackage(packageName string, vcsStore *packages.VCSStore) error {
	// Create temporary directory
	tmpDir := fmt.Sprintf("/tmp/gowl-gendb-%s", packageName)
	if err := os.RemoveAll(tmpDir); err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return err
	}

	// Clone AUR repository to get PKGBUILD
	gitURL := fmt.Sprintf("https://aur.archlinux.org/%s.git", packageName)
	gitCmd := exec.Command("git", "clone", "--depth=1", gitURL, tmpDir)

	if err := gitCmd.Run(); err != nil {
		return err
	}

	// Read PKGBUILD
	pkgbuildPath := fmt.Sprintf("%s/PKGBUILD", tmpDir)
	pkgbuildContent, err := os.ReadFile(pkgbuildPath)
	if err != nil {
		return err
	}

	// Update VCS info
	return vcsStore.UpdatePackageInfo(packageName, string(pkgbuildContent))
}
