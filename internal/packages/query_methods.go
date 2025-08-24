package packages

import (
	"fmt"
	"strings"

	"github.com/Jguer/go-alpm/v2"
)

// GetForeignPackages returns packages not found in sync databases (usually AUR packages)
func (m *ALPMManager) GetForeignPackages() ([]string, error) {
	return m.GetAURPackages()
}

// GetExplicitPackages returns packages that were explicitly installed
func (m *ALPMManager) GetExplicitPackages() ([]string, error) {
	localDB, err := m.handle.LocalDB()
	if err != nil {
		return nil, fmt.Errorf("failed to get local database: %w", err)
	}

	var explicit []string

	err = localDB.PkgCache().ForEach(func(pkg alpm.IPackage) error {
		if pkg.Reason() == alpm.PkgReasonExplicit {
			explicit = append(explicit, pkg.Name())
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to iterate packages: %w", err)
	}

	return explicit, nil
}

// GetDependencyPackages returns packages that were installed as dependencies
func (m *ALPMManager) GetDependencyPackages() ([]string, error) {
	localDB, err := m.handle.LocalDB()
	if err != nil {
		return nil, fmt.Errorf("failed to get local database: %w", err)
	}

	var deps []string

	err = localDB.PkgCache().ForEach(func(pkg alpm.IPackage) error {
		if pkg.Reason() == alpm.PkgReasonDepend {
			deps = append(deps, pkg.Name())
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to iterate packages: %w", err)
	}

	return deps, nil
}

// GetOrphanPackages returns packages that are no longer required by any other package
func (m *ALPMManager) GetOrphanPackages() ([]string, error) {
	localDB, err := m.handle.LocalDB()
	if err != nil {
		return nil, fmt.Errorf("failed to get local database: %w", err)
	}

	var orphans []string

	err = localDB.PkgCache().ForEach(func(pkg alpm.IPackage) error {
		// A package is an orphan if it was installed as a dependency
		// and no other installed package requires it
		if pkg.Reason() == alpm.PkgReasonDepend {
			required := false

			// Check if any installed package depends on this one
			localDB.PkgCache().ForEach(func(otherPkg alpm.IPackage) error {
				if otherPkg.Name() == pkg.Name() {
					return nil // Skip self
				}

				// Check dependencies
				for _, dep := range otherPkg.Depends().Slice() {
					if strings.HasPrefix(dep.String(), pkg.Name()) {
						required = true
						return fmt.Errorf("found dependency") // Break out of inner loop
					}
				}

				return nil
			})

			if !required {
				orphans = append(orphans, pkg.Name())
			}
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to iterate packages: %w", err)
	}

	return orphans, nil
}
