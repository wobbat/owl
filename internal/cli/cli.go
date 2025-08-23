package cli

import (
	"fmt"
	"os"

	"owl/internal/handlers"
	"owl/internal/packages"
	"owl/internal/types"
	"owl/internal/ui"
	"owl/internal/utils"

	"github.com/spf13/cobra"
)

var (
	globalUI = ui.NewUI()
	options  = &types.CommandOptions{}
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "owl",
	Short: "A modern package manager for Arch Linux",
	Long:  "", // We'll handle help manually
	Run: func(cmd *cobra.Command, args []string) {
		// Check for help flag
		if help, _ := cmd.Flags().GetBool("help"); help {
			showHelp()
			return
		}
		// Default command is apply
		handleApplyCommand(false)
	},
	// Disable default help to implement custom help
	DisableAutoGenTag: true,
}

// applyCmd represents the apply command
var applyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Install packages, copy configs, and run setup scripts",
	Long:  `Apply all configurations (install packages, link dotfiles, run setup scripts)`,
	Run: func(cmd *cobra.Command, args []string) {
		handleApplyCommand(false)
	},
}

// dryRunCmd represents the dry-run command
var dryRunCmd = &cobra.Command{
	Use:     "dry-run",
	Aliases: []string{"dr"},
	Short:   "Preview what would be done without making changes",
	Long:    `Preview changes without making them`,
	Run: func(cmd *cobra.Command, args []string) {
		handleApplyCommand(true)
	},
}

// upgradeCmd represents the upgrade command
var upgradeCmd = &cobra.Command{
	Use:     "upgrade",
	Aliases: []string{"up"},
	Short:   "Upgrade all packages to latest versions",
	Long:    `Upgrade all managed packages to their latest versions`,
	Run: func(cmd *cobra.Command, args []string) {
		handleUpgradeCommand()
	},
}

// uninstallCmd represents the uninstall command
var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove all managed packages and configs",
	Long:  `Remove all packages and configurations managed by Owl`,
	Run: func(cmd *cobra.Command, args []string) {
		handleUninstallCommand()
	},
}

// helpCmd represents a custom help command
var helpCmd = &cobra.Command{
	Use:     "help",
	Aliases: []string{"--help", "-h"},
	Short:   "Show this help message",
	Long:    `Display help information for Owl Package Manager`,
	Run: func(cmd *cobra.Command, args []string) {
		showHelp()
	},
}

// versionCmd represents the version command
var versionCmd = &cobra.Command{
	Use:     "version",
	Aliases: []string{"-v"},
	Short:   "Show version information",
	Long:    `Display version information for Owl Package Manager`,
	Run: func(cmd *cobra.Command, args []string) {
		showVersion()
	},
}

func init() {
	rootCmd.AddCommand(applyCmd)
	rootCmd.AddCommand(dryRunCmd)
	rootCmd.AddCommand(upgradeCmd)
	rootCmd.AddCommand(uninstallCmd)
	rootCmd.AddCommand(helpCmd)
	rootCmd.AddCommand(versionCmd)

	// Global flags
	rootCmd.PersistentFlags().BoolVar(&options.NoSpinner, "no-spinner", false, "Disable loading animations")
	rootCmd.PersistentFlags().BoolVar(&options.Verbose, "verbose", false, "Show full command output instead of progress spinners")
	rootCmd.PersistentFlags().BoolVar(&options.Debug, "debug", false, "Enable debug output")

	// Override default help behavior
	rootCmd.SetHelpCommand(&cobra.Command{Hidden: true})
	rootCmd.PersistentFlags().BoolP("help", "h", false, "Show help")
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() error {
	// Override help handling before execution
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--help", "-h":
			showHelp()
			return nil
		case "--version", "-v":
			showVersion()
			return nil
		}
	}
	return rootCmd.Execute()
}

// showHelp displays custom help information that matches the TypeScript version
func showHelp() {
	fmt.Println("Owl Package Manager")
	fmt.Println("A modern package manager for Arch Linux with config management and setup script automation.\n")

	fmt.Println("\x1b[1mUsage:\x1b[0m")
	fmt.Println("  owl <command> [options]\n")

	fmt.Println("\x1b[1mCommands:\x1b[0m")
	globalUI.List([]string{
		"apply          Install packages, copy configs, and run setup scripts",
		"dry-run, dr    Preview what would be done without making changes",
		"upgrade, up    Upgrade all packages to latest versions",
		"uninstall      Remove all managed packages and configs",
		"help, --help   Show this help message",
		"version, -v    Show version information",
	}, types.ListOptions{
		Indent: true,
		Color:  func(s string) string { return "\x1b[34m" + s + "\x1b[0m" },
	})

	fmt.Println("\x1b[1m\nOptions:\x1b[0m")
	globalUI.List([]string{
		"--no-spinner   Disable loading animations",
		"--verbose      Show full command output instead of progress spinners",
	}, types.ListOptions{
		Indent: true,
		Color:  func(s string) string { return "\x1b[37m" + s + "\x1b[0m" },
	})

	fmt.Println("\x1b[1m\nExamples:\x1b[0m")
	globalUI.List([]string{
		"owl                      # Apply all configurations (default)",
		"owl apply                # Apply all configurations",
		"owl dry-run              # Preview changes",
		"owl upgrade              # Upgrade all packages",
		"owl apply --no-spinner   # Apply without animations",
		"owl upgrade --verbose    # Upgrade with full command output",
	}, types.ListOptions{
		Indent: true,
		Color:  func(s string) string { return "\x1b[32m" + s + "\x1b[0m" },
	})

	fmt.Println("\x1b[1m\nConfiguration:\x1b[0m")
	fmt.Println("  Place configuration files in ~/.owl/")
	globalUI.List([]string{
		"~/.owl/main.owl           # Global configuration",
		"~/.owl/hosts/{host}.owl   # Host-specific overrides",
	}, types.ListOptions{
		Indent: true,
		Color:  func(s string) string { return "\x1b[2m" + s + "\x1b[0m" },
	})

	fmt.Println()
}

// showVersion displays version information
func showVersion() {
	fmt.Println("Owl v1.0.0")
	fmt.Println("\x1b[2mA modern package manager for Arch Linux\x1b[0m")
}

// handleApplyCommand handles the apply command (both normal and dry-run)
func handleApplyCommand(dryRun bool) {
	// Setup Owl environment first
	if err := utils.EnsureOwlDirectories(); err != nil {
		globalUI.Error(fmt.Sprintf("Failed to setup directories: %v", err))
		os.Exit(1)
	}

	if err := packages.EnsureYayInstalled(); err != nil {
		globalUI.Error(fmt.Sprintf("Failed to ensure yay is installed: %v", err))
		os.Exit(1)
	}

	// Execute the apply command
	if err := handlers.HandleApplyCommand(dryRun, options); err != nil {
		globalUI.Error(fmt.Sprintf("Apply command failed: %v", err))
		os.Exit(1)
	}
}

// handleUpgradeCommand handles the upgrade command
func handleUpgradeCommand() {
	// Setup Owl environment first
	if err := utils.EnsureOwlDirectories(); err != nil {
		globalUI.Error(fmt.Sprintf("Failed to setup directories: %v", err))
		os.Exit(1)
	}

	if err := packages.EnsureYayInstalled(); err != nil {
		globalUI.Error(fmt.Sprintf("Failed to ensure yay is installed: %v", err))
		os.Exit(1)
	}

	// Execute the upgrade command
	if err := handlers.HandleUpgradeCommand(options); err != nil {
		globalUI.Error(fmt.Sprintf("Upgrade command failed: %v", err))
		os.Exit(1)
	}
}

// handleUninstallCommand handles the uninstall command
func handleUninstallCommand() {
	// Setup Owl environment first
	if err := utils.EnsureOwlDirectories(); err != nil {
		globalUI.Error(fmt.Sprintf("Failed to setup directories: %v", err))
		os.Exit(1)
	}

	// Execute the uninstall command
	if err := handlers.HandleUninstallCommand(options); err != nil {
		globalUI.Error(fmt.Sprintf("Uninstall command failed: %v", err))
		os.Exit(1)
	}
}
