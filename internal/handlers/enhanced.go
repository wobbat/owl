package handlers

import (
	"context"
	"fmt"
	"strings"

	"owl/internal/packages"
	"owl/internal/types"
	"owl/internal/ui"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// Enhanced search command inspired by yay
func SearchCommand() *cobra.Command {
	var aurOnly bool
	var repoOnly bool
	var limit int

	cmd := &cobra.Command{
		Use:   "search [search terms...]",
		Short: "Search for packages in repositories and AUR",
		Long: `Search for packages across official repositories and AUR.
Supports narrow search like yay: 'owl search linux header' will search for 'linux' then narrow to 'header'.`,
		Aliases: []string{"s"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("search term required")
			}

			uiInstance := ui.NewUI()
			searcher, err := packages.NewPackageSearcher()
			if err != nil {
				return fmt.Errorf("failed to initialize searcher: %w", err)
			}
			defer searcher.Release()

			options := types.SearchOptions{
				AUROnly:        aurOnly,
				RepoOnly:       repoOnly,
				AURSearchLimit: limit,
			}

			var results []packages.SearchResult
			var searchWarnings []string

			if len(args) == 1 {
				// Single search term
				results, err = searcher.SearchPackages(args[0], options)
			} else {
				// Narrow search with multiple terms
				results, err = searcher.NarrowSearch(args, options)
			}

			if err != nil {
				// Check if this is a partial failure (AUR failed but repos worked)
				if strings.Contains(err.Error(), "AUR:") && len(results) == 0 {
					// Try with repo-only as fallback
					fallbackOptions := options
					fallbackOptions.RepoOnly = true
					fallbackOptions.AUROnly = false

					if len(args) == 1 {
						results, err = searcher.SearchPackages(args[0], fallbackOptions)
					} else {
						results, err = searcher.NarrowSearch(args, fallbackOptions)
					}

					if err == nil && len(results) > 0 {
						searchWarnings = append(searchWarnings, "AUR search unavailable, showing official repository results only")
					}
				}

				if err != nil {
					return fmt.Errorf("search failed: %w", err)
				}
			}

			if len(searchWarnings) > 0 {
				fmt.Printf("%s %s\n\n", ui.Icon.Warn, searchWarnings[0])
			}

			if len(results) == 0 {
				fmt.Printf("%s No packages found matching '%s'\n", ui.Icon.Err, strings.Join(args, " "))
				return nil
			}

			printSearchResults(results, uiInstance)
			return nil
		},
	}

	cmd.Flags().BoolVar(&aurOnly, "aur", false, "Search AUR only")
	cmd.Flags().BoolVar(&repoOnly, "repo", false, "Search official repositories only")
	cmd.Flags().IntVar(&limit, "limit", 50, "Limit AUR search results")

	return cmd
}

// Enhanced install command
func InstallCommand() *cobra.Command {
	var asdeps bool
	var asexplicit bool
	var noconfirm bool
	var needed bool

	cmd := &cobra.Command{
		Use:     "install [packages...]",
		Short:   "Install packages from repositories or AUR",
		Aliases: []string{"i", "S"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("package name required")
			}

			return installPackagesWithOptions(args, types.InstallOptions{
				AsDeps:     asdeps,
				AsExplicit: asexplicit,
				NoConfirm:  noconfirm,
				Needed:     needed,
			})
		},
	}

	cmd.Flags().BoolVar(&asdeps, "asdeps", false, "Install as dependencies")
	cmd.Flags().BoolVar(&asexplicit, "asexplicit", false, "Install as explicitly installed")
	cmd.Flags().BoolVar(&noconfirm, "noconfirm", false, "Do not ask for confirmation")
	cmd.Flags().BoolVar(&needed, "needed", false, "Skip already installed packages")

	return cmd
}

// System upgrade command
func UpgradeCommand() *cobra.Command {
	var devel bool
	var timeupdate bool
	var noconfirm bool

	cmd := &cobra.Command{
		Use:     "upgrade",
		Short:   "Upgrade all packages",
		Aliases: []string{"u", "Syu"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return upgradeSystem(types.UpgradeOptions{
				Devel:      devel,
				TimeUpdate: timeupdate,
				NoConfirm:  noconfirm,
			})
		},
	}

	cmd.Flags().BoolVar(&devel, "devel", false, "Check development packages for updates")
	cmd.Flags().BoolVar(&timeupdate, "timeupdate", false, "Use PKGBUILD modification time for updates")
	cmd.Flags().BoolVar(&noconfirm, "noconfirm", false, "Do not ask for confirmation")

	return cmd
}

// Package info command
func InfoCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "info [package]",
		Short:   "Show detailed information about a package",
		Aliases: []string{"Si"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("package name required")
			}

			return showPackageInfo(args[0])
		},
	}

	return cmd
}

// Query command for listing packages
func QueryCommand() *cobra.Command {
	var foreign bool
	var explicit bool
	var deps bool
	var unrequired bool

	cmd := &cobra.Command{
		Use:     "query",
		Short:   "Query installed packages",
		Aliases: []string{"q", "Q"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return queryPackages(types.QueryOptions{
				Foreign:    foreign,
				Explicit:   explicit,
				Deps:       deps,
				Unrequired: unrequired,
				Search:     args,
			})
		},
	}

	cmd.Flags().BoolVar(&foreign, "foreign", false, "List foreign packages (AUR)")
	cmd.Flags().BoolVar(&explicit, "explicit", false, "List explicitly installed packages")
	cmd.Flags().BoolVar(&deps, "deps", false, "List packages installed as dependencies")
	cmd.Flags().BoolVar(&unrequired, "unrequired", false, "List unrequired packages")

	return cmd
}

// Helper functions

func printSearchResults(results []packages.SearchResult, ui *ui.UI) {
	for i, result := range results {
		// Format: number) name version [repository] status (votes, popularity)
		line := fmt.Sprintf("%d) %s %s",
			i+1,
			formatPackageName(result.Name, result.Installed, result.InConfig),
			color.New(color.Faint).Sprint(result.Version))

		if result.Repository != "" {
			line += fmt.Sprintf(" [%s]", formatRepository(result.Repository))
		}

		// Add status indicators
		status := formatStatus(result.Installed, result.InConfig)
		if status != "" {
			line += " " + status
		}

		if result.Repository == "aur" {
			line += fmt.Sprintf(" (%d, %.2f)", result.Votes, result.Popularity)
		}

		if result.OutOfDate {
			line += color.New(color.FgRed).Sprint(" (out of date)")
		}

		fmt.Println(line)

		if result.Description != "" {
			fmt.Printf("    %s\n", color.New(color.Faint).Sprint(result.Description))
		}
	}
}

func formatPackageName(name string, installed bool, inConfig bool) string {
	if installed && inConfig {
		return color.New(color.FgGreen, color.Bold).Sprint(name)
	} else if installed {
		return color.New(color.FgGreen).Sprint(name)
	} else if inConfig {
		return color.New(color.FgBlue).Sprint(name)
	}
	return color.New(color.FgWhite).Sprint(name)
}

func formatStatus(installed bool, inConfig bool) string {
	var parts []string

	if installed {
		parts = append(parts, color.New(color.FgGreen).Sprint("[installed]"))
	}

	if inConfig {
		parts = append(parts, color.New(color.FgBlue).Sprint("[config]"))
	}

	return strings.Join(parts, " ")
}

func formatRepository(repo string) string {
	switch repo {
	case "aur":
		return color.New(color.FgMagenta).Sprint(repo)
	case "core", "extra", "multilib":
		return color.New(color.FgBlue).Sprint(repo)
	default:
		return color.New(color.Faint).Sprint(repo)
	}
}

func installPackagesWithOptions(packageNames []string, options types.InstallOptions) error {
	ui := ui.NewUI()

	// Create dependency resolver
	resolver, err := packages.NewDependencyResolver()
	if err != nil {
		return fmt.Errorf("failed to initialize dependency resolver: %w", err)
	}
	defer resolver.Release()

	// Resolve dependencies
	plan, err := resolver.ResolvePackages(packageNames, types.ResolveOptions{})
	if err != nil {
		return fmt.Errorf("dependency resolution failed: %w", err)
	}

	// Show installation plan
	fmt.Println()
	resolver.PrintResolutionPlan(plan)

	// Confirm installation
	if !options.NoConfirm {
		confirmed, err := ui.ConfirmAction("Proceed with installation?")
		if err != nil {
			return err
		}
		if !confirmed {
			ui.Info("Installation cancelled")
			return nil
		}
	}

	// Install packages
	if err := packages.InstallPackages(plan.InstallOrder, false, false); err != nil {
		return fmt.Errorf("installation failed: %w", err)
	}

	ui.Success("Installation completed successfully")
	return nil
}

func upgradeSystem(options types.UpgradeOptions) error {
	ui := ui.NewUI()
	ui.Info("Upgrading system packages...")

	if err := packages.UpgradeAllPackages(!options.NoConfirm); err != nil {
		return fmt.Errorf("system upgrade failed: %w", err)
	}

	if options.Devel {
		ui.Info("Checking development packages...")

		// Get all installed AUR packages
		alpmMgr, err := packages.NewALPMManager()
		if err != nil {
			return fmt.Errorf("failed to initialize ALPM manager: %w", err)
		}
		defer alpmMgr.Release()

		aurPackages, err := alpmMgr.GetAURPackages()
		if err != nil {
			return fmt.Errorf("failed to get AUR packages: %w", err)
		}

		// Filter for VCS packages (git, hg, svn, etc.)
		var vcsPackages []string
		for _, pkg := range aurPackages {
			if packages.IsGitPackage(pkg) {
				vcsPackages = append(vcsPackages, pkg)
			}
		}

		if len(vcsPackages) > 0 {
			ui.Info(fmt.Sprintf("Found %d VCS packages to check", len(vcsPackages)))

			// Initialize VCS store for checking updates
			vcsStore, err := packages.NewVCSStore()
			if err != nil {
				ui.Info("Warning: Could not initialize VCS store, skipping VCS package checks")
			} else {
				defer vcsStore.Save()

				var toUpdate []string
				for _, pkg := range vcsPackages {
					ui.Info(fmt.Sprintf("Checking %s for updates...", pkg))
					needsUpdate, err := vcsStore.CheckGitUpdate(context.Background(), pkg)
					if err != nil {
						ui.Info(fmt.Sprintf("Warning: Could not check %s: %v", pkg, err))
						continue
					}
					if needsUpdate {
						toUpdate = append(toUpdate, pkg)
					}
				}

				if len(toUpdate) > 0 {
					ui.Info(fmt.Sprintf("Found %d VCS packages with updates: %s", len(toUpdate), strings.Join(toUpdate, ", ")))

					if !options.NoConfirm {
						confirmed, err := ui.ConfirmAction(fmt.Sprintf("Update %d VCS packages?", len(toUpdate)))
						if err != nil {
							return err
						}
						if !confirmed {
							ui.Info("VCS package updates cancelled")
							return nil
						}
					}

					// Install updated VCS packages
					if err := packages.InstallPackages(toUpdate, false, false); err != nil {
						return fmt.Errorf("failed to update VCS packages: %w", err)
					}

					ui.Success(fmt.Sprintf("Updated %d VCS packages", len(toUpdate)))
				} else {
					ui.Info("All VCS packages are up to date")
				}
			}
		} else {
			ui.Info("No VCS packages found")
		}
	}

	ui.Success("System upgrade completed")
	return nil
}

func showPackageInfo(packageName string) error {
	searcher, err := packages.NewPackageSearcher()
	if err != nil {
		return fmt.Errorf("failed to initialize searcher: %w", err)
	}
	defer searcher.Release()

	info, err := searcher.GetPackageInfo(packageName)
	if err != nil {
		return fmt.Errorf("package %s not found: %w", packageName, err)
	}

	// Display package information
	fmt.Printf("Name         : %s\n", info.Name)
	fmt.Printf("Version      : %s\n", info.Version)
	fmt.Printf("Description  : %s\n", info.Description)
	fmt.Printf("Repository   : %s\n", formatRepository(info.Repository))
	fmt.Printf("URL          : %s\n", info.URL)
	fmt.Printf("Installed    : %v\n", info.Installed)

	if info.Repository == "aur" {
		if info.Maintainer != "" {
			fmt.Printf("Maintainer   : %s\n", info.Maintainer)
		}
		fmt.Printf("Votes        : %d\n", info.Votes)
		fmt.Printf("Popularity   : %.2f\n", info.Popularity)
		if info.OutOfDate {
			fmt.Printf("Out of Date  : %s\n", color.New(color.FgRed).Sprint("Yes"))
		}
	}

	return nil
}

func queryPackages(options types.QueryOptions) error {
	alpmMgr, err := packages.NewALPMManager()
	if err != nil {
		return fmt.Errorf("failed to initialize ALPM manager: %w", err)
	}
	defer alpmMgr.Release()

	var results []string

	if options.Foreign {
		// List foreign (AUR) packages
		foreignPkgs, err := alpmMgr.GetForeignPackages()
		if err != nil {
			return fmt.Errorf("failed to get foreign packages: %w", err)
		}
		results = append(results, foreignPkgs...)
	} else if options.Explicit {
		// List explicitly installed packages
		explicitPkgs, err := alpmMgr.GetExplicitPackages()
		if err != nil {
			return fmt.Errorf("failed to get explicit packages: %w", err)
		}
		results = append(results, explicitPkgs...)
	} else if options.Deps {
		// List packages installed as dependencies
		depPkgs, err := alpmMgr.GetDependencyPackages()
		if err != nil {
			return fmt.Errorf("failed to get dependency packages: %w", err)
		}
		results = append(results, depPkgs...)
	} else if options.Unrequired {
		// List unrequired packages (orphans)
		orphanPkgs, err := alpmMgr.GetOrphanPackages()
		if err != nil {
			return fmt.Errorf("failed to get orphan packages: %w", err)
		}
		results = append(results, orphanPkgs...)
	} else {
		// List all installed packages
		allPkgs, err := alpmMgr.GetInstalledPackages()
		if err != nil {
			return fmt.Errorf("failed to get installed packages: %w", err)
		}
		results = append(results, allPkgs...)
	}

	// Filter by search terms if provided
	if len(options.Search) > 0 {
		var filtered []string
		searchTerm := strings.ToLower(strings.Join(options.Search, " "))
		for _, pkg := range results {
			if strings.Contains(strings.ToLower(pkg), searchTerm) {
				filtered = append(filtered, pkg)
			}
		}
		results = filtered
	}

	// Display results
	for _, pkg := range results {
		fmt.Println(pkg)
	}

	return nil
}
