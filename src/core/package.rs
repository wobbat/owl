//! Package management utilities

use crate::core::config::Config;
use crate::core::pm::{ParuPacman, SearchResult};
use crate::core::state::PackageState;
use anyhow::Result;
use std::collections::HashSet;
use std::sync::OnceLock;

/// Package action types for planning installations and removals
#[derive(Debug, Clone, PartialEq)]
pub enum PackageAction {
    Install { name: String },
    Remove { name: String },
}

// Cache of installed packages for the current process run
static INSTALLED_CACHE: OnceLock<HashSet<String>> = OnceLock::new();
static PACKAGE_COUNT_CACHE: OnceLock<usize> = OnceLock::new();

fn query_installed_packages() -> Result<HashSet<String>> {
    ParuPacman::new().list_installed()
}

/// Plan package actions by comparing desired config with installed packages
pub fn plan_package_actions(config: &Config, state: &PackageState) -> Result<Vec<PackageAction>> {
    let installed = get_installed_packages()?;
    let desired: HashSet<String> = config.packages.keys().cloned().collect();

    let mut actions = Vec::new();

    for package in &desired {
        if !is_package_or_group_installed(package)? {
            actions.push(PackageAction::Install {
                name: package.clone(),
            });
        }
    }

    for package in &installed {
        if !desired.contains(package) && state.is_managed(package) {
            actions.push(PackageAction::Remove {
                name: package.clone(),
            });
        }
    }

    Ok(actions)
}

/// Get list of all installed packages
pub fn get_installed_packages() -> Result<HashSet<String>> {
    if let Some(cached) = INSTALLED_CACHE.get() {
        return Ok(cached.clone());
    }
    let installed = query_installed_packages()?;
    let _ = INSTALLED_CACHE.set(installed.clone());
    Ok(installed)
}

/// Remove unmanaged packages
pub fn remove_unmanaged_packages(packages: &[String], quiet: bool) -> Result<()> {
    if packages.is_empty() {
        return Ok(());
    }
    println!("Package cleanup (removing conflicting packages):");
    for package in packages {
        println!(
            "  {} Removing: {}",
            crate::internal::color::red("remove"),
            crate::internal::color::yellow(package)
        );
    }
    ParuPacman::new().remove_packages(packages, quiet)
}

/// Get the count of packages that can be upgraded
pub fn get_package_count() -> Result<usize> {
    if let Some(cached) = PACKAGE_COUNT_CACHE.get() {
        return Ok(*cached);
    }
    let count = ParuPacman::new().upgrade_count()?;
    let _ = PACKAGE_COUNT_CACHE.set(count);
    Ok(count)
}

/// Check if a package is installed
pub fn is_package_installed(package_name: &str) -> Result<bool> {
    if let Some(cached) = INSTALLED_CACHE.get() {
        return Ok(cached.contains(package_name));
    }
    let installed = query_installed_packages()?;
    let contains = installed.contains(package_name);
    let _ = INSTALLED_CACHE.set(installed.clone());
    Ok(contains)
}

/// Check if a package or group is effectively installed
/// For regular packages, checks if the package is installed
/// For groups, checks if all packages in the group are installed
pub fn is_package_or_group_installed(package_name: &str) -> Result<bool> {
    // First check if it's a regular package (fastest check)
    if is_package_installed(package_name)? {
        return Ok(true);
    }

    // Check if it's a group (cached to avoid repeated calls)
    let pm = ParuPacman::new();
    if pm.is_package_group(package_name)? {
        // It's a group, check if all packages in the group are installed
        let group_packages = pm.get_group_packages(package_name)?;
        if group_packages.is_empty() {
            return Ok(false);
        }

        // Use the cached installed packages list for faster lookups
        let installed = get_installed_packages()?;
        for pkg in group_packages {
            if !installed.contains(&pkg) {
                return Ok(false);
            }
        }
        return Ok(true);
    }

    // Not a package or group
    Ok(false)
}

/// Determine if a package is available in official repositories
#[cfg(test)]
pub fn is_repo_package(package_name: &str) -> Result<bool, String> {
    let set = ParuPacman::new()
        .batch_repo_available(&[package_name.to_string()])
        .map_err(|e| e.to_string())?;
    Ok(set.contains(package_name))
}

/// Categorize packages into repo and AUR lists
pub fn categorize_packages(packages: &[String]) -> Result<(Vec<String>, Vec<String>)> {
    if packages.is_empty() {
        return Ok((Vec::new(), Vec::new()));
    }
    let available = ParuPacman::new().batch_repo_available(packages)?;
    let repo_packages: Vec<String> = packages
        .iter()
        .filter(|p| available.contains(&**p))
        .cloned()
        .collect();
    let aur_packages: Vec<String> = packages
        .iter()
        .filter(|p| !available.contains(&**p))
        .cloned()
        .collect();
    Ok((repo_packages, aur_packages))
}

/// Search packages using the configured pacman/AUR backends
pub fn search_packages(terms: &[String]) -> Result<Vec<SearchResult>> {
    ParuPacman::new().search_packages(terms)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_is_package_installed() {
        let result = is_package_installed("bash");
        assert!(result.is_ok());
        assert!(result.unwrap());
        let result = is_package_installed("nonexistentpackage12345");
        assert!(result.is_ok());
        assert!(!result.unwrap());
    }

    #[test]
    fn test_is_repo_package() {
        let result = is_repo_package("bash");
        assert!(result.is_ok());
        assert!(result.unwrap());
        let result = is_repo_package("nonexistentpackage12345");
        assert!(result.is_ok());
        assert!(!result.unwrap());
    }

    #[test]
    fn test_categorize_packages() {
        let packages = vec!["bash".to_string(), "nonexistentpackage12345".to_string()];
        let result = categorize_packages(&packages);
        assert!(result.is_ok());
        let (repo_packages, aur_packages) = result.unwrap();
        assert!(repo_packages.contains(&"bash".to_string()));
        assert!(aur_packages.contains(&"nonexistentpackage12345".to_string()));
    }

    #[test]
    fn test_is_package_group() {
        let pm = ParuPacman::new();
        // Test with a known group (using pro-audio as it's in the list)
        let result = pm.is_package_group("pro-audio");
        assert!(result.is_ok());
        assert!(result.unwrap());

        // Test with a non-group package
        let result = pm.is_package_group("bash");
        assert!(result.is_ok());
        assert!(!result.unwrap());

        // Test with a nonexistent package/group
        let result = pm.is_package_group("nonexistentgroup12345");
        assert!(result.is_ok());
        assert!(!result.unwrap());
    }

    #[test]
    fn test_get_group_packages() {
        let pm = ParuPacman::new();
        // Test with a known group
        let result = pm.get_group_packages("pro-audio");
        assert!(result.is_ok());
        let packages = result.unwrap();
        // Should contain some packages
        assert!(!packages.is_empty());
    }

    #[test]
    fn test_is_package_or_group_installed() {
        // Test with a regular package
        let result = is_package_or_group_installed("bash");
        assert!(result.is_ok());
        assert!(result.unwrap());

        // Test with a group
        let result = is_package_or_group_installed("pro-audio");
        assert!(result.is_ok());
        // This might be true or false depending on the system, but shouldn't error
    }
}
