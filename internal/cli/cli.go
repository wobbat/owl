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
	Short: "A modern AUR helper and package manager for Arch Linux",
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
		// Check for --devel flag
		if devel, _ := cmd.Flags().GetBool("devel"); devel {
			options.Devel = true
		}
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

// gendbCmd represents the generate VCS database command
var gendbCmd = &cobra.Command{
	Use:   "gendb",
	Short: "Generate VCS database for development packages",
	Long:  `Generate a VCS database for development packages (-git, -hg, etc.) that were installed without Owl. This command should only be run once.`,
	Run: func(cmd *cobra.Command, args []string) {
		handleGendbCommand()
	},
}

// dotsCmd represents the dots command
var dotsCmd = &cobra.Command{
	Use:   "dots",
	Short: "Check and sync only dotfiles configurations",
	Long:  `Check and sync only dotfiles configurations without installing packages or running setup scripts`,
	Run: func(cmd *cobra.Command, args []string) {
		// Check for --dry-run flag
		if dryRun, _ := cmd.Flags().GetBool("dry-run"); dryRun {
			handleDotsCommand(true)
		} else {
			handleDotsCommand(false)
		}
	},
}

func init() {
	rootCmd.AddCommand(applyCmd)
	rootCmd.AddCommand(dryRunCmd)
	rootCmd.AddCommand(upgradeCmd)
	rootCmd.AddCommand(uninstallCmd)
	rootCmd.AddCommand(helpCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(gendbCmd)
	rootCmd.AddCommand(dotsCmd)

	// Add --devel flag to upgrade command
	upgradeCmd.Flags().Bool("devel", false, "Check development packages (-git, -hg, etc.) for updates")

	// Add --dry-run flag to dots command
	dotsCmd.Flags().Bool("dry-run", false, "Preview dotfiles changes without making them")

	// Add enhanced package manager commands
	rootCmd.AddCommand(handlers.SearchCommand())
	rootCmd.AddCommand(handlers.InstallCommand())
	rootCmd.AddCommand(handlers.UpgradeCommand())
	rootCmd.AddCommand(handlers.InfoCommand())
	rootCmd.AddCommand(handlers.QueryCommand())

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
	globalUI.ShowHelp()
}

// showVersion displays version information
func showVersion() {
	globalUI.ShowVersion("1.0.0")
}

// setupEnvironment performs common setup tasks for all commands
func setupEnvironment() error {
	if err := utils.EnsureOwlDirectories(); err != nil {
		return fmt.Errorf("failed to setup directories: %w", err)
	}
	return nil
}

// setupEnvironmentWithPackageManager performs setup including package manager verification
func setupEnvironmentWithPackageManager() error {
	if err := setupEnvironment(); err != nil {
		return err
	}
	if err := packages.EnsurePackageManagerReady(); err != nil {
		return fmt.Errorf("failed to verify package manager: %w", err)
	}
	return nil
}

// handleCommandError handles command execution errors consistently
func handleCommandError(err error, message string) {
	if err != nil {
		globalUI.Error(fmt.Sprintf("%s: %v", message, err))
		os.Exit(1)
	}
}

// handleApplyCommand handles the apply command (both normal and dry-run)
func handleApplyCommand(dryRun bool) {
	if err := setupEnvironmentWithPackageManager(); err != nil {
		globalUI.Error(err.Error())
		os.Exit(1)
	}

	handleCommandError(handlers.HandleApplyCommand(dryRun, options), "Apply command failed")
}

// handleUpgradeCommand handles the upgrade command
func handleUpgradeCommand() {
	if err := setupEnvironmentWithPackageManager(); err != nil {
		globalUI.Error(err.Error())
		os.Exit(1)
	}

	handleCommandError(handlers.HandleUpgradeCommand(options), "Upgrade command failed")
}

// handleUninstallCommand handles the uninstall command
func handleUninstallCommand() {
	if err := setupEnvironment(); err != nil {
		globalUI.Error(err.Error())
		os.Exit(1)
	}

	handleCommandError(handlers.HandleUninstallCommand(options), "Uninstall command failed")
}

// handleGendbCommand handles the generate VCS database command
func handleGendbCommand() {
	if err := setupEnvironmentWithPackageManager(); err != nil {
		globalUI.Error(err.Error())
		os.Exit(1)
	}

	handleCommandError(handlers.HandleGendbCommand(globalUI), "Generate VCS database command failed")
}

// handleDotsCommand handles the dots command (dotfiles only)
func handleDotsCommand(dryRun bool) {
	if err := setupEnvironment(); err != nil {
		globalUI.Error(err.Error())
		os.Exit(1)
	}

	handleCommandError(handlers.HandleDotsCommand(dryRun, options), "Dots command failed")
}
