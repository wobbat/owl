package handlers

import (
	"fmt"
	"os"

	"owl/internal/config"
	"owl/internal/dotfiles"
	"owl/internal/environment"
	"owl/internal/packages"
	"owl/internal/services"
	"owl/internal/setup"
	"owl/internal/types"
	"owl/internal/ui"
)

// HandleDotsCommand handles the dots command (dotfiles only)
func HandleDotsCommand(dryRun bool, options *types.CommandOptions) error {
	globalUI := ui.NewUI()

	// Get hostname
	hostname, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("failed to get hostname: %w", err)
	}

	// Load and parse all configuration files for this host
	configResult, err := config.LoadConfigForHost(hostname)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Extract only dotfile configs
	var allDotfileConfigs []types.ConfigMapping

	for _, entry := range configResult.Entries {
		allDotfileConfigs = append(allDotfileConfigs, entry.Configs...)
	}

	globalUI.Header(func() string {
		if dryRun {
			return "Dotfiles dry run"
		}
		return "Dotfiles sync"
	}())

	// Process dotfiles configurations
	if len(allDotfileConfigs) > 0 {
		err := processConfigs(allDotfileConfigs, configResult.Entries, dryRun, globalUI)
		if err != nil {
			return err
		}
	} else {
		globalUI.Info("No dotfiles configurations found")
	}

	// Show completion message
	if dryRun {
		globalUI.Success("Dotfiles dry run completed successfully - no changes made")
	} else {
		globalUI.SystemMessage("Dotfiles sync complete")
	}

	return nil
}

// HandleApplyCommand handles the apply command (both normal and dry-run modes)
func HandleApplyCommand(dryRun bool, options *types.CommandOptions) error {
	globalUI := ui.NewUI()

	// Get hostname
	hostname, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("failed to get hostname: %w", err)
	}

	// Load and parse all configuration files for this host
	configResult, err := config.LoadConfigForHost(hostname)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Extract all packages, dotfile configs, setup scripts, services, and environment variables
	var allPackages []string
	var allDotfileConfigs []types.ConfigMapping
	var allSetupScripts []string
	var allServices []string
	var allEnvironmentVariables []types.EnvVar

	for _, entry := range configResult.Entries {
		allPackages = append(allPackages, entry.Package)
		allDotfileConfigs = append(allDotfileConfigs, entry.Configs...)
		allSetupScripts = append(allSetupScripts, entry.Setups...)
		allServices = append(allServices, entry.Services...)
		allEnvironmentVariables = append(allEnvironmentVariables, entry.Envs...)
	}

	globalUI.Header(func() string {
		if dryRun {
			return "Dry run"
		}
		return "Sync"
	}())

	// Remove duplicate packages
	uniquePackages := removeDuplicates(allPackages)

	// Process packages if any are configured
	if len(uniquePackages) > 0 {
		err := processPackages(uniquePackages, configResult.Entries, allDotfileConfigs, dryRun, options, globalUI)
		if err != nil {
			return err
		}
	}

	// Process other configurations
	if len(allDotfileConfigs) > 0 {
		err := processConfigs(allDotfileConfigs, configResult.Entries, dryRun, globalUI)
		if err != nil {
			return err
		}
	}

	if len(allSetupScripts) > 0 {
		err := processSetupScripts(allSetupScripts, dryRun, globalUI)
		if err != nil {
			return err
		}
	}

	if len(allServices) > 0 {
		err := processServices(allServices, dryRun, globalUI)
		if err != nil {
			return err
		}
	}

	if len(allEnvironmentVariables) > 0 {
		err := processEnvironmentVariables(allEnvironmentVariables, dryRun, options.Debug, globalUI)
		if err != nil {
			return err
		}
	}

	if len(configResult.GlobalEnvs) > 0 {
		err := processGlobalEnvironmentVariables(configResult.GlobalEnvs, dryRun, options.Debug, globalUI)
		if err != nil {
			return err
		}
	}

	// Show completion message
	if dryRun {
		globalUI.Success("Dry run completed successfully - no changes made")
	} else {
		globalUI.SystemMessage("System sync complete")
	}

	return nil
}

// processPackages handles package analysis and installation/removal
func processPackages(uniquePackages []string, configEntries []types.ConfigEntry, allConfigs []types.ConfigMapping, dryRun bool, options *types.CommandOptions, globalUI *ui.UI) error {
	// Analyze what packages need to be installed or removed
	spinner := ui.NewSpinner("Analyzing package status...", types.SpinnerOptions{Enabled: !options.NoSpinner})
	packageActions, err := packages.AnalyzePackages(uniquePackages, options.Verbose)
	if err != nil {
		spinner.Fail("Analysis failed")
		return err
	}
	spinner.Stop("Analysis complete")

	// Separate packages into install and remove lists
	var toInstall []types.PackageAction
	var toRemove []types.PackageAction

	for _, action := range packageActions {
		switch action.Status {
		case "install":
			toInstall = append(toInstall, action)
		case "remove":
			toRemove = append(toRemove, action)
		}
	}

	// Get hostname for overview
	hostname, _ := os.Hostname()

	// Show overview of what will be done
	globalUI.Overview(types.HostStats{
		Host:     hostname,
		Packages: len(uniquePackages),
	})

	// Show packages that will be removed
	if len(toRemove) > 0 {
		showPackagesToRemove(toRemove, globalUI)
	}

	// Execute the appropriate action based on dry-run mode
	if dryRun {
		return showDryRunResults(toInstall, toRemove, configEntries, allConfigs, globalUI)
	}

	return performPackageInstallation(toInstall, toRemove, configEntries, allConfigs, uniquePackages, options, globalUI)
}

// showPackagesToRemove displays packages that will be removed
func showPackagesToRemove(toRemove []types.PackageAction, globalUI *ui.UI) {
	fmt.Println("Packages to remove (no longer in config):")
	for _, pkg := range toRemove {
		fmt.Printf("  %s %s\n", ui.Icon.Remove, pkg.Name)
	}
	fmt.Println()
}

// showDryRunResults shows dry run results without making changes
func showDryRunResults(toInstall, toRemove []types.PackageAction, configEntries []types.ConfigEntry, allConfigs []types.ConfigMapping, globalUI *ui.UI) error {
	if len(toInstall) > 0 || len(toRemove) > 0 {
		globalUI.InstallHeader()

		for _, pkg := range toInstall {
			var packageEntry *types.ConfigEntry
			for i := range configEntries {
				if configEntries[i].Package == pkg.Name {
					packageEntry = &configEntries[i]
					break
				}
			}

			hasConfigs := false
			for _, cf := range allConfigs {
				if contains(cf.Source, pkg.Name) {
					hasConfigs = true
					break
				}
			}

			globalUI.PackageInstallProgress(pkg.Name, hasConfigs, false, packageEntry)
		}

		if len(toRemove) > 0 {
			fmt.Println("Package removal simulation:")
			for _, pkg := range toRemove {
				fmt.Printf("  %s Would remove: %s\n", ui.Icon.Remove, pkg.Name)
			}
		}

		globalUI.Success("Package analysis completed (dry-run mode)")
	}

	return nil
}

// performPackageInstallation performs actual package installation
func performPackageInstallation(toInstall, toRemove []types.PackageAction, configEntries []types.ConfigEntry, allConfigs []types.ConfigMapping, uniquePackages []string, options *types.CommandOptions, globalUI *ui.UI) error {
	// Remove packages that are no longer in config
	if len(toRemove) > 0 {
		err := removePackages(toRemove, options, globalUI)
		if err != nil {
			return err
		}
	}

	// Upgrade system packages
	err := upgradeSystemPackages(options, globalUI)
	if err != nil {
		return err
	}

	// Install new packages
	if len(toInstall) > 0 {
		err := installNewPackages(toInstall, configEntries, allConfigs, options, globalUI)
		if err != nil {
			return err
		}
	}

	// Update managed packages tracking
	return packages.UpdateManagedPackages(uniquePackages)
}

// removePackages removes packages that are no longer in config
func removePackages(toRemove []types.PackageAction, options *types.CommandOptions, globalUI *ui.UI) error {
	fmt.Println("Package cleanup (removing conflicting packages):")
	for _, pkg := range toRemove {
		fmt.Printf("  %s Removing: %s\n", ui.Icon.Remove, pkg.Name)
	}

	var packageNames []string
	for _, pkg := range toRemove {
		packageNames = append(packageNames, pkg.Name)
	}

	// Get package manager based on flag
	packageManager, err := packages.NewPackageManager(options.UseLibALPM)
	if err != nil {
		return fmt.Errorf("failed to initialize package manager: %w", err)
	}
	defer packageManager.Release()

	// Remove packages one by one using the selected package manager
	var successfullyRemoved []string
	for _, pkgName := range packageNames {
		if err := packageManager.RemovePackage(pkgName); err != nil {
			return fmt.Errorf("failed to remove package %s: %w", pkgName, err)
		}
		successfullyRemoved = append(successfullyRemoved, pkgName)
	}

	// Update managed packages state to remove the successfully removed packages
	if len(successfullyRemoved) > 0 {
		if err := packages.RemoveFromManagedPackages(successfullyRemoved); err != nil {
			// Log the error but don't fail the operation
			fmt.Printf("  %s Warning: Failed to update managed packages state: %v\n", ui.Icon.Warn, err)
		}
	}

	fmt.Printf("  %s Removed %d packages\n", ui.Icon.Ok, len(toRemove))
	fmt.Println()
	return nil
}

// upgradeSystemPackages upgrades system packages
func upgradeSystemPackages(options *types.CommandOptions, globalUI *ui.UI) error {
	spinner := ui.NewSpinner("Synchronizing package databases", types.SpinnerOptions{
		Enabled: !options.NoSpinner && !options.Verbose,
	})

	fmt.Println("Performing system maintenance!")

	// Get package manager based on flag
	packageManager, err := packages.NewPackageManager(options.UseLibALPM)
	if err != nil {
		if spinner != nil {
			spinner.Fail(fmt.Sprintf("Failed to initialize package manager: %v", err))
		}
		return fmt.Errorf("failed to initialize package manager: %w", err)
	}
	defer packageManager.Release()

	err = packageManager.UpgradeSystem(options.Verbose)
	if err != nil {
		if spinner != nil {
			spinner.Fail("Upgrade failed")
		}
		return fmt.Errorf("failed to upgrade system: %w", err)
	}

	if options.Verbose {
		fmt.Printf("  %s All packages upgraded to latest versions\n", ui.Icon.Ok)
	} else if spinner != nil {
		spinner.Stop("-> done!")
	}

	fmt.Println()
	return nil
}

// installNewPackages installs new packages
func installNewPackages(toInstall []types.PackageAction, configEntries []types.ConfigEntry, allConfigs []types.ConfigMapping, options *types.CommandOptions, globalUI *ui.UI) error {
	globalUI.InstallHeader()

	for _, pkg := range toInstall {
		hasConfigs := false
		for _, cf := range allConfigs {
			if contains(cf.Source, pkg.Name) {
				hasConfigs = true
				break
			}
		}

		// Create a spinner that will be updated with progress
		spinner := ui.NewSpinner(fmt.Sprintf("Preparing %s", pkg.Name), types.SpinnerOptions{
			Enabled: !options.NoSpinner && !options.Verbose,
		})

		// Get package manager based on flag
		packageManager, err := packages.NewPackageManager(options.UseLibALPM)
		if err != nil {
			if !options.Verbose {
				spinner.Fail(fmt.Sprintf("Failed to initialize package manager: %v", err))
			}
			return fmt.Errorf("failed to initialize package manager: %w", err)
		}

		// Use the selected package manager for installation
		err = packageManager.InstallPackage(pkg.Name, options.Verbose)
		packageManager.Release()

		if err != nil {
			if !options.Verbose {
				spinner.Fail(fmt.Sprintf("Failed: %v", err))
			}
			return fmt.Errorf("failed to install %s: %w", pkg.Name, err)
		}

		if !options.Verbose {
			spinner.Stop("installed")
		}

		globalUI.PackageInstallComplete(pkg.Name, hasConfigs)
	}

	return nil
}

// processConfigs handles configuration management
func processConfigs(allConfigs []types.ConfigMapping, configEntries []types.ConfigEntry, dryRun bool, globalUI *ui.UI) error {
	if len(allConfigs) > 0 {
		return dotfiles.ProcessConfigsPerPackage(configEntries, dryRun, globalUI)
	}
	return nil
}

// processSetupScripts handles setup script execution
func processSetupScripts(allSetups []string, dryRun bool, globalUI *ui.UI) error {
	if len(allSetups) > 0 {
		return setup.ProcessSetupScripts(allSetups, dryRun, globalUI)
	}
	return nil
}

// processServices handles service management
func processServices(allServices []string, dryRun bool, globalUI *ui.UI) error {
	if len(allServices) > 0 {
		return services.ProcessServices(allServices, dryRun, globalUI)
	}
	return nil
}

// processEnvironmentVariables handles environment variable management
func processEnvironmentVariables(allEnvs []types.EnvVar, dryRun bool, debug bool, globalUI *ui.UI) error {
	if len(allEnvs) > 0 {
		return environment.ProcessEnvironmentVariables(allEnvs, dryRun, debug, globalUI)
	}
	return nil
}

// processGlobalEnvironmentVariables handles global environment variable management
func processGlobalEnvironmentVariables(globalEnvs []types.EnvVar, dryRun bool, debug bool, globalUI *ui.UI) error {
	if len(globalEnvs) > 0 {
		return environment.ProcessGlobalEnvironmentVariables(globalEnvs, dryRun, debug, globalUI)
	}
	return nil
}

// removeDuplicates removes duplicate strings from a slice
func removeDuplicates(slice []string) []string {
	keys := make(map[string]bool)
	var result []string

	for _, item := range slice {
		if !keys[item] {
			keys[item] = true
			result = append(result, item)
		}
	}

	return result
}

// contains checks if a string contains a substring
func contains(s, substr string) bool {
	return len(substr) > 0 && len(s) >= len(substr) && s[:len(substr)] == substr
}
