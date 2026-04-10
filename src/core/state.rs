//! Package state management for tracking untracked and hidden packages

use crate::internal::constants;
use anyhow::Result;
use serde::{Deserialize, Serialize};
use std::fs;
use std::path::Path;
use std::path::PathBuf;

/// Generic trait for state persistence operations
trait StatePersistence<T> {
    const FILE_NAME: &'static str;
    const DEFAULT_VALUE: fn() -> T;

    fn serialize(data: &T) -> Result<String>;
    fn deserialize(content: &str) -> Result<T>;

    fn load(state_dir: &Path) -> Result<T> {
        let file_path = state_dir.join(Self::FILE_NAME);
        if !file_path.exists() {
            let default = Self::DEFAULT_VALUE();
            Self::save(state_dir, &default)?;
            return Ok(default);
        }

        let content = fs::read_to_string(&file_path)
            .map_err(|e| anyhow::anyhow!("Failed to read {}: {}", Self::FILE_NAME, e))?;
        Self::deserialize(&content)
    }

    fn save(state_dir: &Path, data: &T) -> Result<()> {
        let file_path = state_dir.join(Self::FILE_NAME);
        let content = Self::serialize(data)
            .map_err(|e| anyhow::anyhow!("Failed to serialize {}: {}", Self::FILE_NAME, e))?;
        fs::write(&file_path, content)
            .map_err(|e| anyhow::anyhow!("Failed to write {}: {}", Self::FILE_NAME, e))?;
        Ok(())
    }
}

/// Default system packages that should not be tracked
fn default_untracked_packages() -> Vec<String> {
    vec![
        "linux".to_string(),
        "linux-firmware".to_string(),
        "intel-ucode".to_string(),
        "amd-ucode".to_string(),
        "base".to_string(),
        "base-devel".to_string(),
        "glibc".to_string(),
        "filesystem".to_string(),
        "bash".to_string(),
        "coreutils".to_string(),
        "findutils".to_string(),
        "grep".to_string(),
        "gawk".to_string(),
        "sed".to_string(),
        "less".to_string(),
        "util-linux".to_string(),
        "procps-ng".to_string(),
        "shadow".to_string(),
        "iproute2".to_string(),
        "iputils".to_string(),
        "pacman".to_string(),
        "pacman-contrib".to_string(),
        "gzip".to_string(),
        "xz".to_string(),
        "tar".to_string(),
        "openssl".to_string(),
        "ca-certificates".to_string(),
        "e2fsprogs".to_string(),
    ]
}

/// Package state information
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct PackageState {
    pub untracked: Vec<String>,
    pub hidden: Vec<String>,
    pub managed: Vec<String>,
}

/// Specific implementation for untracked packages (JSON format)
struct UntrackedPackages;

impl StatePersistence<Vec<String>> for UntrackedPackages {
    const FILE_NAME: &'static str = "untracked.json";
    const DEFAULT_VALUE: fn() -> Vec<String> = || default_untracked_packages();

    fn serialize(data: &Vec<String>) -> Result<String> {
        serde_json::to_string_pretty(data)
            .map_err(|e| anyhow::anyhow!("Failed to serialize untracked packages: {}", e))
    }

    fn deserialize(content: &str) -> Result<Vec<String>> {
        serde_json::from_str(content)
            .map_err(|e| anyhow::anyhow!("Failed to parse untracked packages JSON: {}", e))
    }
}

/// Specific implementation for hidden packages (plain text format)
struct HiddenPackages;

impl StatePersistence<Vec<String>> for HiddenPackages {
    const FILE_NAME: &'static str = "hidden.txt";
    const DEFAULT_VALUE: fn() -> Vec<String> = Vec::new;

    fn serialize(data: &Vec<String>) -> Result<String> {
        Ok(data.join("\n") + "\n")
    }

    fn deserialize(content: &str) -> Result<Vec<String>> {
        Ok(content
            .lines()
            .map(|line| line.trim().to_string())
            .filter(|line| !line.is_empty())
            .collect())
    }
}

/// Specific implementation for managed packages (JSON format)
struct ManagedPackages;

impl StatePersistence<Vec<String>> for ManagedPackages {
    const FILE_NAME: &'static str = "managed.json";
    const DEFAULT_VALUE: fn() -> Vec<String> = Vec::new;

    fn serialize(data: &Vec<String>) -> Result<String> {
        serde_json::to_string_pretty(data)
            .map_err(|e| anyhow::anyhow!("Failed to serialize managed packages: {}", e))
    }

    fn deserialize(content: &str) -> Result<Vec<String>> {
        serde_json::from_str(content)
            .map_err(|e| anyhow::anyhow!("Failed to parse managed packages JSON: {}", e))
    }
}

// Some methods are part of the public API for future use (e.g., CLI commands for managing
// hidden/untracked packages). They are tested but not yet used in the main application.
#[allow(dead_code)]
impl PackageState {
    /// Load package state from ~/.owl/.state directory
    pub fn load() -> Result<Self> {
        let state_dir = Self::get_state_dir()?;
        if !state_dir.exists() {
            fs::create_dir_all(&state_dir)
                .map_err(|e| anyhow::anyhow!("Failed to create state directory: {}", e))?;
        }

        // Use trait-based loading for each state type
        let untracked = UntrackedPackages::load(&state_dir)?;
        let hidden = HiddenPackages::load(&state_dir)?;
        let managed = ManagedPackages::load(&state_dir)?;

        Ok(PackageState {
            untracked,
            hidden,
            managed,
        })
    }

    /// Save package state to disk
    pub fn save(&self) -> Result<()> {
        let state_dir = Self::get_state_dir()?;
        if !state_dir.exists() {
            fs::create_dir_all(&state_dir)
                .map_err(|e| anyhow::anyhow!("Failed to create state directory: {}", e))?;
        }

        // Use trait-based saving for each state type
        UntrackedPackages::save(&state_dir, &self.untracked)?;
        HiddenPackages::save(&state_dir, &self.hidden)?;
        ManagedPackages::save(&state_dir, &self.managed)?;
        Ok(())
    }

    /// Check if a package is in the untracked list
    pub fn is_untracked(&self, package: &str) -> bool {
        self.untracked.contains(&package.to_string())
    }

    /// Check if a package is in the hidden list
    pub fn is_hidden(&self, package: &str) -> bool {
        self.hidden.contains(&package.to_string())
    }

    /// Check if a package is managed by owl
    pub fn is_managed(&self, package: &str) -> bool {
        self.managed.contains(&package.to_string())
    }

    /// Add a package to the untracked list
    pub fn add_untracked(&mut self, package: String) {
        if !self.untracked.contains(&package) {
            self.untracked.push(package);
            self.untracked.sort();
        }
    }

    /// Remove a package from the untracked list
    pub fn remove_untracked(&mut self, package: &str) {
        self.untracked.retain(|p| p != package);
    }

    /// Add a package to the hidden list
    pub fn add_hidden(&mut self, package: String) {
        if !self.hidden.contains(&package) {
            self.hidden.push(package);
            self.hidden.sort();
        }
    }

    /// Remove a package from the hidden list
    pub fn remove_hidden(&mut self, package: &str) {
        self.hidden.retain(|p| p != package);
    }

    /// Add a package to the managed list
    pub fn add_managed(&mut self, package: String) {
        if !self.managed.contains(&package) {
            self.managed.push(package);
            self.managed.sort();
        }
    }

    /// Remove a package from the managed list
    pub fn remove_managed(&mut self, package: &str) {
        self.managed.retain(|p| p != package);
    }

    fn get_state_dir() -> Result<PathBuf> {
        let home = std::env::var("HOME")
            .map_err(|_| anyhow::anyhow!("HOME environment variable not set"))?;
        Ok(PathBuf::from(home)
            .join(constants::OWL_DIR)
            .join(constants::STATE_DIR))
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::Mutex;
    use tempfile::tempdir;

    // Use a mutex to ensure tests don't interfere with each other
    static TEST_MUTEX: Mutex<()> = Mutex::new(());

    fn setup_test_home() -> tempfile::TempDir {
        let temp_dir = tempdir().expect("Failed to create temp directory");
        unsafe { std::env::set_var("HOME", temp_dir.path()) };
        temp_dir
    }

    #[test]
    fn test_load_initial_state() {
        let _guard = TEST_MUTEX.lock().unwrap();
        let _temp_dir = setup_test_home();

        let state = PackageState::load().expect("Failed to load package state");
        assert!(!state.untracked.is_empty());
        assert!(state.is_untracked("linux"));
        assert!(state.is_untracked("base"));
    }

    #[test]
    fn test_add_remove_untracked() {
        let _guard = TEST_MUTEX.lock().unwrap();
        let _temp_dir = setup_test_home();

        let mut state = PackageState::load().expect("Failed to load package state");
        state.add_untracked("test-package".to_string());
        assert!(state.is_untracked("test-package"));
        state.remove_untracked("test-package");
        assert!(!state.is_untracked("test-package"));
    }
}
