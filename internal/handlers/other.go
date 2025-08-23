package handlers

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"owl/internal/packages"
	"owl/internal/types"
	"owl/internal/ui"
	"owl/internal/utils"
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

	// Get outdated packages using yay -Qu
	cmd := exec.Command("yay", "-Qu")
	output, err := cmd.Output()

	var outdatedPackages []string
	if err == nil && len(output) > 0 {
		lines := strings.Split(string(output), "\n")
		var packageNames []string
		for _, line := range lines {
			if line != "" {
				parts := strings.Fields(line)
				if len(parts) > 0 {
					packageNames = append(packageNames, parts[0])
				}
			}
		}
		outdatedPackages = utils.Compact(packageNames)
	} else {
		// If command fails or no output, assume no packages to upgrade
		outdatedPackages = []string{}
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

	err = packages.UpgradeAllPackages(options.Verbose)
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
	confirmed, err := utils.ConfirmAction("Continue?")
	if err != nil {
		return fmt.Errorf("failed to get user confirmation: %w", err)
	}

	if !confirmed {
		fmt.Println("Uninstall cancelled")
		return nil
	}

	fmt.Println("Removing managed packages...")

	// Remove packages
	err = packages.RemoveUnmanagedPackages(packageNames, !options.Verbose)
	if err != nil {
		return fmt.Errorf("uninstall failed: %w", err)
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
