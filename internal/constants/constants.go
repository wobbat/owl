package constants

// File paths and directories
const (
	OwlRootDir      = ".owl"
	OwlStateDir     = ".state"
	OwlDotfilesDir  = "dotfiles"
	OwlSetupDir     = "setup"
	OwlHostsDir     = "hosts"
	OwlGroupsDir    = "groups"
	ManagedLockFile = "managed.lock"
)

// Configuration file extensions
var ConfigExtensions = []string{".owl"}

// Setup script extensions
var SetupExtensions = []string{".sh", ".js", ".ts"}

// Timing constants (in milliseconds)
const (
	SpinnerFrameInterval = 80
	PackageInstallDelay  = 150
	DotfilesInstallDelay = 100
)

// Exit codes
const (
	ExitSuccess = 0
	ExitFailure = 1
)

// Package manager commands
const (
	Yay    = "yay"
	Pacman = "pacman"
)

// Default protected packages that should never be auto-removed
var DefaultProtectedPackages = []string{
	"base", "base-devel", "linux", "linux-firmware", "linux-headers",
	"systemd", "systemd-sysvcompat", "dbus", "dbus-broker",
	"grub", "systemd-boot", "refind", "bootctl",
	"bash", "zsh", "fish", "coreutils", "util-linux", "filesystem",
	"pacman", "pacman-contrib", "archlinux-keyring", "ca-certificates",
	"networkmanager", "dhcpcd", "iwd", "wpa_supplicant",
	"sudo", "polkit", "glibc", "gcc-libs", "binutils", "gawk", "sed", "grep",
}

// Schema version for managed lock file
const SchemaVersion = "1.0"

// Spinner frames
var SpinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Error messages
const (
	ErrorUnknownCommand       = "Unknown command"
	ErrorConfigNotFound       = "Configuration file not found"
	ErrorPackageInstallFailed = "Package installation failed"
	ErrorPackageRemovalFailed = "Package removal failed"
	ErrorYayNotFound          = "yay not found. Installing yay..."
	ErrorInvalidConfig        = "Invalid configuration"
	ErrorCircularDependency   = "Circular dependency detected"
)
