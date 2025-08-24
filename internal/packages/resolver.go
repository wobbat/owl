package packages

import (
	"fmt"
	"sort"
	"strings"

	"owl/internal/types"
)

// DependencyResolver handles dependency resolution for packages
type DependencyResolver struct {
	alpm     *ALPMManager
	searcher *PackageSearcher
}

// NewDependencyResolver creates a new dependency resolver
func NewDependencyResolver() (*DependencyResolver, error) {
	alpm, err := NewALPMManager()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize ALPM: %w", err)
	}

	searcher, err := NewPackageSearcher()
	if err != nil {
		alpm.Release()
		return nil, fmt.Errorf("failed to initialize searcher: %w", err)
	}

	return &DependencyResolver{
		alpm:     alpm,
		searcher: searcher,
	}, nil
}

// Release releases resources
func (dr *DependencyResolver) Release() {
	if dr.alpm != nil {
		dr.alpm.Release()
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

	// Check for conflicts
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

	// Check if package is already installed
	installed, err := dr.alpm.IsPackageInstalled(pkgName)
	if err != nil {
		return nil, err
	}

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

	// Resolve dependencies (this would need to be implemented based on PKGBUILD parsing)
	dependencies, err := dr.getPackageDependencies(pkgName)
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

// getPackageDependencies gets the dependencies for a package
func (dr *DependencyResolver) getPackageDependencies(pkgName string) ([]PackageDependency, error) {
	// This is a simplified implementation
	// In practice, you'd need to parse PKGBUILD files or use ALPM to get dependency information

	// For AUR packages, you'd download and parse the PKGBUILD
	// For official packages, you can use ALPM

	return []PackageDependency{}, nil
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

// checkConflicts checks for package conflicts
func (dr *DependencyResolver) checkConflicts(plan *ResolutionPlan) {
	// This would check for package conflicts and provides relationships
	// Implementation would involve checking installed packages and planned installations
}

// calculateInstallOrder determines the correct order to install packages
func (dr *DependencyResolver) calculateInstallOrder(plan *ResolutionPlan) {
	installed := make(map[string]bool)

	// Get currently installed packages
	installedPackages, err := dr.alpm.GetInstalledPackages()
	if err == nil {
		for _, pkg := range installedPackages {
			installed[pkg] = true
		}
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
