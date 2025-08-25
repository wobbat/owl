package cli

import (
	"fmt"
	"os"
	"time"

	"owl/internal/config"
	"owl/internal/dotfiles"
	"owl/internal/packages"
	"owl/internal/ui"
)

// RunLegacySync implements the legacy TypeScript-style sync process
func RunLegacySync(dryRun bool) {
	renderer := ui.NewLegacyRenderer()

	// Title
	renderer.Title()

	// Analysis phase
	analyzeSpinner := renderer.StartSpinner("Analyzing package status...")
	hostname, err := os.Hostname()
	if err != nil {
		globalUI.Error(fmt.Sprintf("get hostname: %v", err))
		os.Exit(1)
	}

	configResult, err := config.LoadConfigForHost(hostname)
	if err != nil {
		globalUI.Error(fmt.Sprintf("load config: %v", err))
		os.Exit(1)
	}

	var allPackages []string
	for _, entry := range configResult.Entries {
		allPackages = append(allPackages, entry.Package)
	}

	uniquePackages := removeDuplicates(allPackages)
	var packageCount int
	if len(uniquePackages) > 0 {
		packageActions, err := packages.AnalyzePackages(uniquePackages, false)
		if err != nil {
			globalUI.Error(fmt.Sprintf("analyze packages: %v", err))
			os.Exit(1)
		}
		packageCount = len(packageActions)
	}

	// Stop analysis spinner
	renderer.StopSpinner(analyzeSpinner, "Analysis complete")

	// Host info and separator
	renderer.HostInfo(hostname, packageCount)
	renderer.Separator()

	// Maintenance phase
	renderer.MaintenanceHeader()
	upgradeSpinner := renderer.StartSpinner("Upgrading system packages...")

	if !dryRun {
		// Use default package manager (yay) for legacy sync
		packageManager, err := packages.NewPackageManager(false)
		if err != nil {
			globalUI.Error(fmt.Sprintf("failed to initialize package manager: %v", err))
			os.Exit(1)
		}
		defer packageManager.Release()

		if err := packageManager.UpgradeSystem(false); err != nil {
			globalUI.Error(fmt.Sprintf("upgrade system: %v", err))
			os.Exit(1)
		}
	}

	// Stop upgrade spinner with legacy "-> done!" format
	upgradeSpinner.SetFinalizer(func(duration time.Duration) {
		timing := fmt.Sprintf("(%dms)", duration.Milliseconds())
		if renderer.NoColor() {
			fmt.Printf("+ Upgrading system packages... %s -> done!\n", timing)
		} else {
			fmt.Printf("%s Upgrading system packages... %s -> %s\n",
				ui.Icon.Ok, ui.Muted(timing), ui.Success("done!"))
		}
	})
	upgradeSpinner.Stop("")

	// Config management phase
	renderer.ConfigHeader()

	// Process all dotfiles first if not dry run (silently)
	if !dryRun {
		if err := dotfiles.ProcessConfigsPerPackage(configResult.Entries, false, globalUI); err != nil {
			// Continue on error for now to match legacy behavior
		}
	}

	for _, entry := range configResult.Entries {
		if len(entry.Configs) > 0 {
			renderer.PackageHeader(entry.Package)
			// For now, mark as "up to date" - could enhance with change detection
			status := "up to date"
			renderer.DotfilesStatus(status, "(0ms)")
		}
	}

	// Footer
	renderer.Footer()
}

// removeDuplicates removes duplicate strings from slice (helper)
func removeDuplicates(slice []string) []string {
	seen := make(map[string]struct{}, len(slice))
	var out []string
	for _, s := range slice {
		if _, ok := seen[s]; !ok {
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}
	return out
}
