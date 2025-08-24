package packages

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Jguer/go-alpm/v2"
	"owl/internal/types"
)

// DependencyResolver handles dependency resolution for packages
type DependencyResolver struct {
	alpm        *ALPMManager
	enhanced    *EnhancedALPMManager
	searcher    *PackageSearcher
	transaction *TransactionManager
}

// NewDependencyResolver creates a new dependency resolver
func NewDependencyResolver() (*DependencyResolver, error) {
	alpm, err := NewALPMManager()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize ALPM: %w", err)
	}

	enhanced, err := NewEnhancedALPMManager()
	if err != nil {
		alpm.Release()
		return nil, fmt.Errorf("failed to initialize enhanced ALPM: %w", err)
	}

	searcher, err := NewPackageSearcher()
	if err != nil {
		alpm.Release()
		enhanced.Cleanup()
		return nil, fmt.Errorf("failed to initialize searcher: %w", err)
	}

	transaction := NewTransactionManager(enhanced.handle)

	return &DependencyResolver{
		alpm:        alpm,
		enhanced:    enhanced,
		searcher:    searcher,
		transaction: transaction,
	}, nil
}

// Release releases resources
func (dr *DependencyResolver) Release() {
	if dr.alpm != nil {
		dr.alpm.Release()
	}
	if dr.enhanced != nil {
		dr.enhanced.Cleanup()
	}
	if dr.searcher != nil {
		dr.searcher.Release()
	}
}

// DependencyNode represents a node in the dependency tree
type DependencyNode struct {
	Name         string
	Repository   string
	Version      string
	Dependencies []*DependencyNode
	Installed    bool
	Needed       bool
	Optional     bool
	MakeDep      bool
	Conflicts    []string
	Provides     []string
}

// ResolutionPlan represents a complete package installation plan
type ResolutionPlan struct {
	Targets      []*DependencyNode
	Dependencies []*DependencyNode
	MakeDeps     []*DependencyNode
	Conflicts    []string
	InstallOrder []string
	ToRemove     []string
}

// ResolvePackages resolves dependencies for a list of packages
func (dr *DependencyResolver) ResolvePackages(packageNames []string, options types.ResolveOptions) (*ResolutionPlan, error) {
	plan := &ResolutionPlan{
		Targets:      make([]*DependencyNode, 0),
		Dependencies: make([]*DependencyNode, 0),
		MakeDeps:     make([]*DependencyNode, 0),
		Conflicts:    make([]string, 0),
		InstallOrder: make([]string, 0),
		ToRemove:     make([]string, 0),
	}

	visited := make(map[string]*DependencyNode)

	// Resolve each target package
	for _, pkgName := range packageNames {
		node, err := dr.resolvePackageTree(pkgName, visited, false, false)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve package %s: %w", pkgName, err)
		}
		plan.Targets = append(plan.Targets, node)
	}

	// Separate dependencies and make dependencies
	dr.categorizeDependencies(visited, plan)

	// Check for conflicts using enhanced ALPM
	dr.checkConflicts(plan)

	// Determine installation order
	dr.calculateInstallOrder(plan)

	return plan, nil
}

// resolvePackageTree recursively resolves a package and its dependencies
func (dr *DependencyResolver) resolvePackageTree(pkgName string, visited map[string]*DependencyNode, isMakeDep, isOptional bool) (*DependencyNode, error) {
	// Check if already visited
	if node, exists := visited[pkgName]; exists {
		return node, nil
	}

	// Check if package is already installed using enhanced ALPM
	localPkg := dr.enhanced.LocalPackage(pkgName)
	installed := localPkg != nil

	// Get package information
	pkgInfo, err := dr.searcher.GetPackageInfo(pkgName)
	if err != nil {
		return nil, fmt.Errorf("package %s not found: %w", pkgName, err)
	}

	node := &DependencyNode{
		Name:         pkgInfo.Name,
		Repository:   pkgInfo.Repository,
		Version:      pkgInfo.Version,
		Dependencies: make([]*DependencyNode, 0),
		Installed:    installed,
		Needed:       !installed,
		Optional:     isOptional,
		MakeDep:      isMakeDep,
		Conflicts:    make([]string, 0),
		Provides:     make([]string, 0),
	}

	visited[pkgName] = node

	// If package is already installed and we're not forcing reinstall, skip dependency resolution
	if installed {
		return node, nil
	}

	// Resolve dependencies using enhanced ALPM for better dependency handling
	dependencies, err := dr.getPackageDependenciesEnhanced(pkgName)
	if err != nil {
		return nil, err
	}

	for _, dep := range dependencies {
		depNode, err := dr.resolvePackageTree(dep.Name, visited, dep.MakeDep, dep.Optional)
		if err != nil {
			if dep.Optional {
				// Skip optional dependencies that can't be resolved
				continue
			}
			return nil, err
		}
		node.Dependencies = append(node.Dependencies, depNode)
	}

	return node, nil
}

// PackageDependency represents a dependency relationship
type PackageDependency struct {
	Name     string
	Version  string
	Optional bool
	MakeDep  bool
}

// getPackageDependenciesEnhanced gets dependencies using the enhanced ALPM manager
func (dr *DependencyResolver) getPackageDependenciesEnhanced(pkgName string) ([]PackageDependency, error) {
	var dependencies []PackageDependency

	// Try to find package in sync databases using enhanced ALPM
	pkg := dr.enhanced.SyncPackage(pkgName)
	if pkg != nil {
		// Get dependencies using enhanced ALPM
		deps := dr.enhanced.PackageDepends(pkg)
		for _, dep := range deps {
			dependencies = append(dependencies, PackageDependency{
				Name:     dep.Name,
				Version:  dep.Version,
				Optional: false,
				MakeDep:  false,
			})
		}

		// Get optional dependencies
		optDeps := dr.enhanced.PackageOptionalDepends(pkg)
		for _, dep := range optDeps {
			dependencies = append(dependencies, PackageDependency{
				Name:     dep.Name,
				Version:  dep.Version,
				Optional: true,
				MakeDep:  false,
			})
		}

		return dependencies, nil
	}

	// Fallback to original ALPM implementation for AUR packages
	return dr.getPackageDependencies(pkgName)
}

// getPackageDependencies gets the dependencies for a package using libalpm (legacy)
func (dr *DependencyResolver) getPackageDependencies(pkgName string) ([]PackageDependency, error) {
	var dependencies []PackageDependency

	// First check official repositories using libalpm
	syncDBs, err := dr.alpm.handle.SyncDBs()
	if err != nil {
		return nil, fmt.Errorf("failed to get sync databases: %w", err)
	}

	// Look for package in sync databases
	for _, db := range syncDBs.Slice() {
		if pkg := db.Pkg(pkgName); pkg != nil {
			// Get dependencies using libalpm
			deps := pkg.Depends()
			if deps != nil {
				deps.ForEach(func(dep *alpm.Depend) error {
					dependencies = append(dependencies, PackageDependency{
						Name:     dep.Name,
						Version:  dep.Version,
						Optional: false,
						MakeDep:  false,
					})
					return nil
				})
			}

			// Get optional dependencies
			optDeps := pkg.OptionalDepends()
			if optDeps != nil {
				optDeps.ForEach(func(dep *alpm.Depend) error {
					dependencies = append(dependencies, PackageDependency{
						Name:     dep.Name,
						Version:  dep.Version,
						Optional: true,
						MakeDep:  false,
					})
					return nil
				})
			}

			// Get make dependencies (for completeness, though less common in binary packages)
			makeDeps := pkg.MakeDepends()
			if makeDeps != nil {
				makeDeps.ForEach(func(dep *alpm.Depend) error {
					dependencies = append(dependencies, PackageDependency{
						Name:     dep.Name,
						Version:  dep.Version,
						Optional: false,
						MakeDep:  true,
					})
					return nil
				})
			}

			return dependencies, nil
		}
	}

	// If not found in official repos, it might be an AUR package
	// For AUR packages, we'd need to download and parse PKGBUILD
	// This is more complex and would require additional implementation
	return dependencies, nil
}

// categorizeDependencies separates targets, dependencies, and make dependencies
func (dr *DependencyResolver) categorizeDependencies(visited map[string]*DependencyNode, plan *ResolutionPlan) {
	for _, node := range visited {
		if node.MakeDep {
			plan.MakeDeps = append(plan.MakeDeps, node)
		} else if !contains(getNodeNames(plan.Targets), node.Name) {
			plan.Dependencies = append(plan.Dependencies, node)
		}
	}
}

// checkConflicts checks for package conflicts using enhanced ALPM
func (dr *DependencyResolver) checkConflicts(plan *ResolutionPlan) {
	// Use enhanced ALPM for better conflict detection
	var allPackages []alpm.IPackage

	// Collect all packages from the plan
	for _, node := range plan.Targets {
		if pkg := dr.enhanced.SyncPackage(node.Name); pkg != nil {
			allPackages = append(allPackages, pkg)
		}
	}
	for _, node := range plan.Dependencies {
		if pkg := dr.enhanced.SyncPackage(node.Name); pkg != nil {
			allPackages = append(allPackages, pkg)
		}
	}

	// Use the enhanced ALPM transaction manager to check for conflicts
	if upgrades, err := dr.transaction.CheckUpgrades(false); err == nil {
		for pkgName := range upgrades {
			// Check if any planned packages conflict with upgrade targets
			for _, node := range plan.Targets {
				if node.Name == pkgName {
					plan.Conflicts = append(plan.Conflicts, fmt.Sprintf("%s conflicts with system upgrade", pkgName))
				}
			}
		}
	}
}

// calculateInstallOrder determines the correct order to install packages
func (dr *DependencyResolver) calculateInstallOrder(plan *ResolutionPlan) {
	installed := make(map[string]bool)

	// Get currently installed packages using enhanced ALPM
	installedPackages := dr.enhanced.LocalPackages()
	for _, pkg := range installedPackages {
		installed[pkg.Name()] = true
	}

	var order []string

	// Add dependencies first
	for _, node := range plan.Dependencies {
		if !installed[node.Name] && node.Needed {
			order = append(order, node.Name)
			installed[node.Name] = true
		}
	}

	// Add make dependencies
	for _, node := range plan.MakeDeps {
		if !installed[node.Name] && node.Needed {
			order = append(order, node.Name)
			installed[node.Name] = true
		}
	}

	// Add targets last
	for _, node := range plan.Targets {
		if !installed[node.Name] && node.Needed {
			order = append(order, node.Name)
			installed[node.Name] = true
		}
	}

	plan.InstallOrder = order
}

// getNodeNames extracts names from dependency nodes
func getNodeNames(nodes []*DependencyNode) []string {
	names := make([]string, len(nodes))
	for i, node := range nodes {
		names[i] = node.Name
	}
	return names
}

// PrintResolutionPlan prints a human-readable resolution plan
func (dr *DependencyResolver) PrintResolutionPlan(plan *ResolutionPlan) {
	if len(plan.InstallOrder) == 0 {
		fmt.Println("Nothing to do.")
		return
	}

	fmt.Printf("Packages (%d) ", len(plan.InstallOrder))
	fmt.Println(strings.Join(plan.InstallOrder, " "))

	if len(plan.MakeDeps) > 0 {
		fmt.Printf("Make dependencies (%d) ", len(plan.MakeDeps))
		makeDepNames := getNodeNames(plan.MakeDeps)
		sort.Strings(makeDepNames)
		fmt.Println(strings.Join(makeDepNames, " "))
	}

	if len(plan.Conflicts) > 0 {
		fmt.Printf("Conflicts (%d) ", len(plan.Conflicts))
		fmt.Println(strings.Join(plan.Conflicts, " "))
	}

	if len(plan.ToRemove) > 0 {
		fmt.Printf("Remove (%d) ", len(plan.ToRemove))
		fmt.Println(strings.Join(plan.ToRemove, " "))
	}
}

// ExecuteInstallPlan executes an installation plan using the transaction manager
func (dr *DependencyResolver) ExecuteInstallPlan(plan *ResolutionPlan, options TransactionOptions) error {
	if len(plan.InstallOrder) == 0 {
		return nil // Nothing to install
	}

	// Install packages in the correct order
	return dr.transaction.InstallPackages(plan.InstallOrder, options)
}

// ValidatePackagesExist validates that all packages in the plan exist
func (dr *DependencyResolver) ValidatePackagesExist(plan *ResolutionPlan) error {
	var missing []string

	for _, node := range plan.Targets {
		if pkg := dr.enhanced.SyncPackage(node.Name); pkg == nil {
			missing = append(missing, node.Name)
		}
	}

	for _, node := range plan.Dependencies {
		if pkg := dr.enhanced.SyncPackage(node.Name); pkg == nil {
			missing = append(missing, node.Name)
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("packages not found: %s", strings.Join(missing, ", "))
	}

	return nil
}
