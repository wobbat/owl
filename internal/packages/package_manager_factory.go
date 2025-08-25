package packages

import (
	"fmt"
	"os/exec"
)

// PackageManager defines the interface for package management operations
type PackageManager interface {
	InstallPackage(packageName string, verbose bool) error
	InstallPackages(packages []string, verbose bool) error
	RemovePackage(packageName string) error
	UpgradeSystem(verbose bool) error
	SearchPackages(searchTerm string) ([]SearchResult, error)
	GetInstalledPackages() ([]string, error)
	GetOutdatedPackages() ([]string, error)
	IsPackageInstalled(packageName string) (bool, error)
	Release()
}

// NewPackageManager creates the appropriate package manager based on the useLibALPM flag
func NewPackageManager(useLibALPM bool) (PackageManager, error) {
	if useLibALPM {
		// Use libalpm-based package manager
		return NewALPMManager()
	} else {
		// Use yay-based package manager (default)
		return NewYayPackageManager(), nil
	}
}

// EnsurePackageManagerReady ensures the package manager is ready for use
func EnsurePackageManagerReadyWithFlag(useLibALPM bool) error {
	if useLibALPM {
		// For libalpm, we need to ensure it's properly initialized
		manager, err := NewALPMManager()
		if err != nil {
			return err
		}
		manager.Release()
		return nil
	} else {
		// For yay, just check if it's available
		return checkYayAvailable()
	}
}

// checkYayAvailable checks if yay is available on the system
func checkYayAvailable() error {
	cmd := exec.Command("which", "yay")
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("yay is not available on this system. Please install yay or use --alpm flag to use libalpm")
	}
	return nil
}
