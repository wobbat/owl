use crate::core::pm::PackageManager;
use anyhow::{Result, anyhow};

/// Get list of AUR packages that can be updated
pub fn get_aur_updates() -> Result<Vec<String>> {
    crate::core::pm::ParuPacman::new().get_aur_updates()
}

/// Count packages that have dotfile configurations
pub fn count_dotfile_packages(config: &crate::core::config::Config) -> usize {
    config
        .packages
        .values()
        .filter(|pkg| !pkg.config.is_empty())
        .count()
}

/// Count total environment variables (package + global)
pub fn count_environment_variables(config: &crate::core::config::Config) -> usize {
    let package_env_vars = config
        .packages
        .values()
        .map(|pkg| pkg.env_vars.len())
        .sum::<usize>();
    package_env_vars + config.env_vars.len()
}

/// Analysis result containing system configuration and package information
#[derive(Debug)]
pub struct Analysis {
    pub package_count: usize,
    pub config: crate::core::config::Config,
    pub state: crate::core::state::PackageState,
    pub actions: Vec<crate::core::package::PackageAction>,
    pub dotfile_count: usize,
    pub service_count: usize,
    pub config_package_count: usize,
}

pub fn analyze_system() -> anyhow::Result<Analysis> {
    use std::thread;

    // Run independent, potentially slow operations in parallel
    // 1) Count upgradable packages
    let count_handle = thread::spawn(crate::core::package::get_package_count);
    // 2) Load config files
    let config_handle = thread::spawn(crate::core::config::Config::load_all_relevant_config_files);
    // 3) Load package state from disk
    let state_handle = thread::spawn(crate::core::state::PackageState::load);
    // 4) Prewarm installed package cache to avoid repeated -Q calls later
    let installed_warm_handle = thread::spawn(|| {
        let _ = crate::core::package::get_installed_packages();
        Ok::<(), anyhow::Error>(())
    });

    // Join results
    let package_count = count_handle
        .join()
        .map_err(|_| anyhow!("Failed to join package count thread"))?
        .map_err(|e| anyhow!("Failed to get package count: {}", e))?;

    let mut state = state_handle
        .join()
        .map_err(|_| anyhow!("Failed to join state loader thread"))?
        .map_err(|e| anyhow!("Failed to load package state: {}", e))?;

    let config = config_handle
        .join()
        .map_err(|_| anyhow!("Failed to join config loader thread"))?
        .map_err(|e| anyhow!("Failed to load config: {}", e))?;

    // Ensure installed cache warm-up finished (best-effort)
    let _ = installed_warm_handle.join();

    // Seed managed state with currently installed packages that are present in config.
    // This ensures future removals are detected only for packages user explicitly managed via config.
    if seed_managed_with_desired_installed(&config, &mut state)? {
        // Best-effort save; don't fail analysis if saving state fails.
        if let Err(e) = state.save() {
            eprintln!(
                "{}",
                crate::internal::color::red(&format!("Failed to save seeded package state: {}", e))
            );
        }
    }

    // Plan package actions (installs and removals)
    let actions = crate::core::package::plan_package_actions(&config, &state)
        .map_err(|e| anyhow!("Failed to plan package actions: {}", e))?;

    // Calculate dynamic values (these are fast)
    let dotfile_count = count_dotfile_packages(&config);
    let service_count = crate::core::services::get_configured_services(&config).len();
    let config_package_count = config.packages.len();

    Ok(Analysis {
        package_count,
        config,
        state,
        actions,
        dotfile_count,
        service_count,
        config_package_count,
    })
}

/// Ensure packages that are currently in the config and installed are marked as managed
pub fn seed_managed_with_desired_installed(
    config: &crate::core::config::Config,
    state: &mut crate::core::state::PackageState,
) -> anyhow::Result<bool> {
    let mut changed = false;

    // Collect packages to check in batches
    let packages_to_check: Vec<&String> = config
        .packages
        .keys()
        .filter(|pkg| !state.is_managed(pkg))
        .collect();

    if packages_to_check.is_empty() {
        return Ok(false);
    }

    // Group packages by whether they might be groups or regular packages
    // to minimize redundant group checks
    for pkg in packages_to_check {
        match crate::core::package::is_package_or_group_installed(pkg) {
            Ok(true) => {
                state.add_managed(pkg.to_string());
                changed = true;
            }
            Ok(false) => {}
            Err(e) => {
                eprintln!(
                    "{}",
                    crate::internal::color::red(&format!(
                        "Failed to verify installation of {}: {}",
                        pkg, e
                    ))
                );
            }
        }
    }

    Ok(changed)
}
