use crate::core::pm::PackageManager;
use crate::error::{handle_error, handle_error_with_context};

/// Parameters for package operations
#[derive(Debug)]
pub struct PackageOperationParams {
    pub dry_run: bool,
    pub non_interactive: bool,
    pub had_uninstalled: bool,
}

pub fn handle_removals(
    to_remove: &[String],
    dry_run: bool,
    state: &mut crate::core::state::PackageState,
) {
    if to_remove.is_empty() {
        return;
    }

    if dry_run {
        println!("Package cleanup (would remove conflicting packages):");
        for package in to_remove {
            println!(
                "  {} Would remove: {}",
                crate::internal::color::red("remove"),
                crate::internal::color::yellow(package)
            );
        }
        println!(
            "  {} Would remove {} package(s)",
            crate::internal::color::blue("info:"),
            to_remove.len()
        );
        return;
    }

    // Ask for explicit confirmation before removing packages
    if !crate::cli::ui::confirm_remove_operation(to_remove) {
        println!(
            "  {}",
            crate::internal::color::blue("Package removal cancelled")
        );
        return;
    }

    if let Err(e) = crate::core::package::remove_unmanaged_packages(to_remove, true) {
        eprintln!(
            "{}",
            crate::internal::color::red(&format!("Failed to remove packages: {}", e))
        );
        return;
    }

    // Remove successfully removed packages from managed list
    for package in to_remove {
        state.remove_managed(package);
    }

    if let Err(e) = state.save() {
        eprintln!(
            "{}",
            crate::internal::color::red(&format!("Failed to update package state: {}", e))
        );
    }
}

/// Install missing packages and update all packages
pub fn install_and_update_packages(
    to_install: &[String],
    params: &PackageOperationParams,
    config: &crate::core::config::Config,
) {
    // First, handle uninstalled packages
    let (repo_to_install, aur_to_install) = categorize_install_sets(to_install);

    // Get AUR packages that need updates
    let aur_to_update = compute_aur_updates(params.dry_run);

    // Install repo packages first (no confirmation needed)
    install_repo_packages(&repo_to_install, params.dry_run, params.non_interactive);

    // Handle all AUR packages together if there are any
    if !aur_to_install.is_empty() || !aur_to_update.is_empty() {
        // Show detailed breakdown of what will happen
        if !aur_to_install.is_empty() {
            println!(
                "  {} AUR packages to install: {}",
                crate::internal::color::yellow(&aur_to_install.len().to_string()),
                aur_to_install.join(", ")
            );
        }
        if !aur_to_update.is_empty() {
            println!(
                "  {} AUR packages to update: {}",
                crate::internal::color::yellow(&aur_to_update.len().to_string()),
                aur_to_update.join(", ")
            );
        }

        handle_aur_operations(
            &aur_to_install,
            &aur_to_update,
            params.dry_run,
            params.non_interactive,
        );
    }

    // Add blank line if we installed packages before this
    if params.had_uninstalled {
        println!();
    }

    // Update repo packages
    update_repo_packages(params.dry_run, params.non_interactive);

    // Apply dotfile synchronization
    super::dotfiles::apply_dotfiles_with_config(config, params.dry_run);

    // Handle system section (services + environment)
    super::system::handle_system_section_with_config(config, params.dry_run);
}

pub fn categorize_install_sets(to_install: &[String]) -> (Vec<String>, Vec<String>) {
    if to_install.is_empty() {
        return (Vec::new(), Vec::new());
    }
    match crate::core::package::categorize_packages(to_install) {
        Ok(result) => result,
        Err(e) => {
            handle_error_with_context("categorize packages", Err(e));
            (Vec::new(), Vec::new())
        }
    }
}

pub fn compute_aur_updates(dry_run: bool) -> Vec<String> {
    if dry_run {
        return Vec::new();
    }
    match super::analysis::get_aur_updates() {
        Ok(packages) => packages,
        Err(e) => {
            handle_error_with_context("check AUR updates", Err(e));
            Vec::new()
        }
    }
}

fn use_pm_passthrough(non_interactive: bool) -> bool {
    if non_interactive {
        return false;
    }

    std::env::var("OWL_PM_PASSTHROUGH")
        .map(|value| {
            matches!(
                value.trim().to_ascii_lowercase().as_str(),
                "1" | "true" | "yes" | "on"
            )
        })
        .unwrap_or(false)
}

pub fn install_repo_packages(repo_to_install: &[String], dry_run: bool, non_interactive: bool) {
    if repo_to_install.is_empty() {
        return;
    }
    println!(
        "  {} repo packages found: {}",
        crate::internal::color::yellow(&repo_to_install.len().to_string()),
        repo_to_install.join(", ")
    );
    if dry_run {
        println!(
            "  {} Would install {} from official repositories",
            crate::internal::color::blue("info:"),
            repo_to_install.join(", ")
        );
    } else {
        let pm = crate::core::pm::ParuPacman::new();
        if use_pm_passthrough(non_interactive) {
            println!(
                "  {} Package manager passthrough enabled",
                crate::internal::color::blue("info:")
            );
            handle_error(pm.install_repo_with_mode(repo_to_install, false));
        } else {
            handle_error(pm.install_repo(repo_to_install));
        }
    }
}

pub fn handle_aur_operations(
    aur_to_install: &[String],
    aur_to_update: &[String],
    dry_run: bool,
    non_interactive: bool,
) {
    // Create combined list only when needed for confirmation/display
    let all_aur_packages: Vec<String> = aur_to_install
        .iter()
        .chain(aur_to_update.iter())
        .cloned()
        .collect();

    if dry_run
        || non_interactive
        || crate::cli::ui::confirm_aur_operation(&all_aur_packages, "installing/updating")
    {
        if dry_run {
            println!(
                "  {} Would install/update {} from AUR",
                crate::internal::color::blue("info:"),
                all_aur_packages.join(", ")
            );
            return;
        }
        if !aur_to_install.is_empty() {
            let pm = crate::core::pm::ParuPacman::new();
            if use_pm_passthrough(non_interactive) {
                println!(
                    "  {} Package manager passthrough enabled",
                    crate::internal::color::blue("info:")
                );
                handle_error(pm.install_aur_with_mode(aur_to_install, false));
            } else {
                handle_error(pm.install_aur(aur_to_install));
            }
        }
        if !aur_to_update.is_empty() {
            let pm = crate::core::pm::ParuPacman::new();
            if use_pm_passthrough(non_interactive) {
                println!(
                    "  {} Package manager passthrough enabled",
                    crate::internal::color::blue("info:")
                );
                handle_error(pm.update_aur_with_mode(aur_to_update, false));
            } else {
                handle_error(pm.update_aur(aur_to_update));
            }
        }
    } else {
        println!(
            "  {}",
            crate::internal::color::blue("AUR package operations cancelled")
        );
    }
}

pub fn update_repo_packages(dry_run: bool, non_interactive: bool) {
    if dry_run {
        println!(
            "  {} Would update official repository packages",
            crate::internal::color::blue("info:")
        );
        return;
    }
    let pm = crate::core::pm::ParuPacman::new();
    if use_pm_passthrough(non_interactive) {
        println!(
            "  {} Package manager passthrough enabled",
            crate::internal::color::blue("info:")
        );
        handle_error_with_context("update repo packages", pm.update_repo_with_mode(false));
    } else {
        handle_error_with_context("update repo packages", pm.update_repo());
    }
}
