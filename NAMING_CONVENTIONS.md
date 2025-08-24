# Owl Package Manager - Naming Conventions and Architecture Improvements

## Executive Summary

After analyzing your current codebase and studying yay's mature implementation, I've identified several areas for improvement in naming clarity and overall architecture. This document outlines a comprehensive naming scheme and architectural patterns that will make your AUR helper more maintainable and professional.

## Current Issues Identified

### 1. Vague and Generic Naming
- `EnhancedALPMManager` - "Enhanced" provides no semantic meaning
- `NewALPMManager()` vs `NewEnhancedALPMManager()` - unclear distinction
- Generic method names like `setupDatabases()`, `getPackageRepository()`

### 2. Inconsistent Naming Patterns
- Mixed usage of `ALPM` vs `Alpm` in type names
- Inconsistent verb patterns (`Get` vs `Check` vs `Is` vs `Find`)
- Missing domain context in function names

### 3. Missing Semantic Domain Language
- Functions don't clearly indicate scope (local DB vs sync DB vs AUR)
- No distinction between query operations and mutating operations
- Unclear ownership and lifecycle management

## Proposed Naming Conventions

### 1. Core Architecture Components

#### Before (Problematic):
```go
type ALPMManager struct {}
type EnhancedALPMManager struct {}
```

#### After (Clear & Descriptive):
```go
type PacmanDatabaseExecutor struct {}  // Handles libalpm database operations
type AURPackageManager struct {}       // Handles AUR-specific operations  
type PackageSearchExecutor struct {}   // Unified search across all sources
```

### 2. Method Naming Patterns

#### Database Query Methods - Use Domain-Specific Prefixes:
```go
// Local database operations
FindLocalPackage(name string) alpm.IPackage
FindLocalPackageSatisfier(dependency string) (alpm.IPackage, error)
GetAllInstalledPackages() []alpm.IPackage
IsPackageInstalled(name string) bool
IsExactVersionInstalled(name, version string) bool

// Sync database operations  
FindSyncPackage(name string) alpm.IPackage
FindSyncPackageInRepository(name, repo string) alpm.IPackage
SearchSyncPackages(terms ...string) []alpm.IPackage
GetConfiguredRepositories() []string

// System operations
CalculateSystemUpgrades(allowDowngrades bool) (map[string]SyncPackageUpgrade, error)
ValidatePacmanConfiguration() error
GetDatabaseStatistics() DatabaseStatistics
```

#### AUR Operations - Clear Scope and Intent:
```go
// Package discovery
SearchAURPackages(terms []string) ([]AURPackageMetadata, error)
GetAURPackageMetadata(name string) (*AURPackageMetadata, error)
GetBatchAURPackageMetadata(names []string) (map[string]*AURPackageMetadata, error)

// Status and version management
GetAURPackageStatus(name string) (*AURPackageStatus, error)
CheckAllAURUpdates() ([]AURPackageStatus, error)
GetInstalledForeignPackages() ([]string, error)

// Installation with progress tracking
InstallAURPackage(name string, callback BuildProgressCallback) error
InstallAURPackageWithOptions(name string, options AURInstallOptions, callback BuildProgressCallback) error
```

### 3. Type Naming - Descriptive and Domain-Focused

#### Replace Generic Types:
```go
// Before: Vague and unclear
type AURPackage struct {}
type SyncUpgrade struct {}

// After: Clear and descriptive  
type AURPackageMetadata struct {}      // Complete AUR package information
type SyncPackageUpgrade struct {}      // Represents available upgrade from sync repos
type AURPackageStatus struct {}        // Current installation/update status
type PackageDependencyInfo struct {}   // Comprehensive dependency information
type BuildProgress struct {}           // Progress tracking for AUR builds
```

### 4. Configuration and Options - Clear Intent

```go
// Installation behavior control
type AURInstallOptions struct {
    NoConfirm    bool
    Force        bool  
    AsDeps       bool
    ShowOutput   bool
    KeepBuildDir bool
}

// Search behavior control
type SearchOptions struct {
    RepositoryOnly   bool
    AUROnly         bool
    IncludeInstalled bool
    IncludeOutdated  bool
    MaxResults      int
    SortByRelevance bool
}
```

## Architectural Improvements

### 1. Clear Separation of Concerns

#### PacmanDatabaseExecutor
- Handles all libalpm database operations
- Provides read-only access to package databases
- Manages configuration and database lifecycle
- Follows yay's proven patterns for database interaction

#### AURPackageManager  
- Dedicated to AUR operations (search, install, status)
- Handles build process management
- Tracks VCS packages appropriately
- Provides progress reporting for long operations

#### PackageSearchExecutor
- Unified interface for searching across all package sources
- Intelligent result ranking and deduplication
- Support for yay-style narrow search
- Configurable search behavior

### 2. Improved Error Handling and Resource Management

```go
// Clear lifecycle management
func (pde *PacmanDatabaseExecutor) Release() {
    // Proper cleanup with error handling
}

// Context-aware operations where appropriate
func (apm *AURPackageManager) CheckAllAURUpdates(ctx context.Context) ([]AURPackageStatus, error) {
    // Cancellable long operations
}
```

### 3. Progress Reporting and User Experience

```go
// Rich progress information for AUR builds
type BuildProgress struct {
    PackageName    string
    Stage          string  // "downloading", "building", "installing" 
    Progress       float64 // 0.0 to 1.0
    Message        string
    IsError        bool
}

type BuildProgressCallback func(progress BuildProgress)
```

## Implementation Benefits

### 1. Self-Documenting Code
- Method names clearly indicate their scope and purpose
- Type names reflect their domain responsibility
- No ambiguity about what operations affect what components

### 2. Better IDE Support
- Clear autocomplete suggestions
- Easy navigation between related functionality
- Reduced cognitive load when reading code

### 3. Maintainable Architecture
- Clear boundaries between components
- Easy to test individual components
- Simple to extend with new functionality

### 4. Professional Quality
- Follows industry best practices for Go naming
- Consistent with mature package managers like yay
- Clear documentation through naming

## Migration Strategy

1. **Phase 1**: Implement new core types alongside existing ones
2. **Phase 2**: Update internal usage to new naming patterns  
3. **Phase 3**: Update public APIs and documentation
4. **Phase 4**: Remove deprecated legacy types

## Example Usage with New Naming

```go
// Clear, professional API usage
func upgradeSystem() error {
    // Initialize database access
    dbExecutor, err := packages.NewPacmanDatabaseExecutor()
    if err != nil {
        return fmt.Errorf("failed to initialize database: %w", err)
    }
    defer dbExecutor.Release()
    
    // Check for system upgrades
    upgrades, err := dbExecutor.CalculateSystemUpgrades(false)
    if err != nil {
        return fmt.Errorf("failed to calculate upgrades: %w", err)
    }
    
    // Handle AUR packages separately
    aurManager, err := packages.NewAURPackageManager()
    if err != nil {
        return fmt.Errorf("failed to initialize AUR manager: %w", err)
    }
    defer aurManager.Release()
    
    aurUpdates, err := aurManager.CheckAllAURUpdates()
    if err != nil {
        return fmt.Errorf("failed to check AUR updates: %w", err)
    }
    
    // Install updates with progress tracking
    for _, update := range aurUpdates {
        if update.HasUpdate {
            err := aurManager.InstallAURPackage(update.Name, func(progress packages.BuildProgress) {
                fmt.Printf("[%s] %s: %s\n", progress.Stage, progress.PackageName, progress.Message)
            })
            if err != nil {
                fmt.Printf("Failed to update %s: %v\n", update.Name, err)
            }
        }
    }
    
    return nil
}
```

This naming scheme provides clarity, follows Go conventions, and creates a professional-grade codebase that's easy to understand, maintain, and extend.