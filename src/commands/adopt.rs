use crate::core::config::Config;
use crate::core::state::PackageState;
use crate::internal::color;
use anyhow::{Result, anyhow};
use std::collections::HashSet;
use std::io::Write;
use std::path::{Path, PathBuf};
use std::process::Command;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum PackageAction {
    Adopt,
    Ignore,
    Skip,
    Quit,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum AddResult {
    Added,
    AlreadyPresent,
}

pub fn run(items: &[String], all: bool) {
    let mut state = match PackageState::load() {
        Ok(s) => s,
        Err(e) => {
            eprintln!("{}", color::red(&format!("Failed to load state: {}", e)));
            return;
        }
    };

    let config = match Config::load_all_relevant_config_files() {
        Ok(cfg) => cfg,
        Err(e) => {
            eprintln!("{}", color::red(&format!("Failed to load config: {}", e)));
            return;
        }
    };

    let installed = match crate::core::package::get_installed_packages() {
        Ok(installed) => installed,
        Err(e) => {
            eprintln!(
                "{}",
                color::red(&format!("Failed to list installed packages: {}", e))
            );
            return;
        }
    };
    let explicit_installed = match get_explicitly_installed_packages() {
        Ok(explicit) => explicit,
        Err(e) => {
            eprintln!(
                "{}",
                color::red(&format!("Failed to list explicit packages: {}", e))
            );
            return;
        }
    };

    let discover_mode = all || items.is_empty();
    let targets = if discover_mode {
        discover_candidates_from_explicit(&explicit_installed, &state, &config)
    } else {
        normalize_targets(items)
    };

    if targets.is_empty() {
        println!(
            "{}",
            color::yellow("No unmanaged installed packages available for adoption")
        );
        return;
    }

    println!(
        "{} {} package(s) available for adoption",
        color::blue("info:"),
        targets.len()
    );

    let mut adopted = Vec::new();
    let mut adopted_state_only = Vec::new();
    let mut ignored = Vec::new();
    let mut skipped = Vec::new();
    let mut skipped_not_installed = Vec::new();
    let mut skipped_already_managed = Vec::new();
    let mut state_changed = false;
    let mut selected_config: Option<String> = None;

    for pkg in targets {
        if state.is_managed(&pkg) {
            skipped_already_managed.push(pkg);
            continue;
        }

        if discover_mode && state.is_untracked(&pkg) {
            skipped.push(pkg);
            continue;
        }

        if !installed.contains(&pkg) {
            skipped_not_installed.push(pkg);
            continue;
        }

        if config.packages.contains_key(&pkg) {
            state.add_managed(pkg.clone());
            state_changed = true;
            adopted_state_only.push(pkg);
            continue;
        }

        let action = match prompt_package_action(&pkg) {
            Some(action) => action,
            None => {
                eprintln!("{}", color::red("Failed to read selection, stopping adopt"));
                break;
            }
        };

        match action {
            PackageAction::Adopt => {
                let config_path = if let Some(path) = &selected_config {
                    path.clone()
                } else {
                    match prompt_config_file_selection() {
                        Ok(Some(path)) => {
                            selected_config = Some(path.clone());
                            path
                        }
                        Ok(None) => {
                            println!("{}", color::yellow("Adopt cancelled by user"));
                            break;
                        }
                        Err(err) => {
                            eprintln!(
                                "{}",
                                color::red(&format!("Failed to select config: {}", err))
                            );
                            return;
                        }
                    }
                };

                match add_package_to_file(&pkg, &config_path) {
                    Ok(AddResult::Added) => {
                        state.remove_untracked(&pkg);
                        state.add_managed(pkg.clone());
                        state_changed = true;
                        adopted.push(pkg);
                    }
                    Ok(AddResult::AlreadyPresent) => {
                        state.remove_untracked(&pkg);
                        state.add_managed(pkg.clone());
                        state_changed = true;
                        adopted_state_only.push(pkg);
                    }
                    Err(err) => {
                        eprintln!(
                            "{}",
                            color::red(&format!("Failed to adopt {}: {}", pkg, err))
                        );
                    }
                }
            }
            PackageAction::Ignore => {
                state.add_untracked(pkg.clone());
                state.remove_managed(&pkg);
                state_changed = true;
                ignored.push(pkg);
            }
            PackageAction::Skip => skipped.push(pkg),
            PackageAction::Quit => break,
        }
    }

    if state_changed {
        if let Err(e) = state.save() {
            eprintln!("{}", color::red(&format!("Failed to save state: {}", e)));
            return;
        }
    }

    if let Some(file) = selected_config {
        println!(
            "{} Adopted packages were written to {}",
            color::blue("info:"),
            file
        );
    }
    if !adopted.is_empty() {
        println!(
            "{} Adopted {} package(s): {}",
            color::green("✓"),
            adopted.len(),
            adopted.join(", ")
        );
    }
    if !adopted_state_only.is_empty() {
        println!(
            "{} Marked as managed (already in config): {}",
            color::blue("info:"),
            adopted_state_only.join(", ")
        );
    }
    if !ignored.is_empty() {
        println!(
            "{} Ignored package(s): {}",
            color::yellow("!"),
            ignored.join(", ")
        );
    }
    if !skipped_already_managed.is_empty() {
        println!(
            "{} Already managed: {}",
            color::blue("info:"),
            skipped_already_managed.join(", ")
        );
    }
    if !skipped_not_installed.is_empty() {
        println!(
            "{} Not installed (skipped): {}",
            color::yellow("!"),
            skipped_not_installed.join(", ")
        );
    }
    if !skipped.is_empty() {
        println!("{} Skipped: {}", color::blue("info:"), skipped.join(", "));
    }
}

fn normalize_targets(items: &[String]) -> Vec<String> {
    let mut seen = HashSet::new();
    let mut targets = Vec::new();
    for item in items {
        let name = item.trim();
        if name.is_empty() {
            continue;
        }
        if seen.insert(name.to_string()) {
            targets.push(name.to_string());
        }
    }
    targets
}

fn discover_candidates_from_explicit(
    explicit_installed: &HashSet<String>,
    state: &PackageState,
    config: &Config,
) -> Vec<String> {
    let mut candidates: Vec<String> = explicit_installed
        .iter()
        .filter(|pkg| !state.is_managed(pkg))
        .filter(|pkg| !state.is_untracked(pkg))
        .filter(|pkg| !config.packages.contains_key(*pkg))
        .cloned()
        .collect();
    candidates.sort();
    candidates
}

fn get_explicitly_installed_packages() -> Result<HashSet<String>> {
    let output = match Command::new(crate::internal::constants::PACKAGE_MANAGER)
        .args(["-Qeq"])
        .output()
    {
        Ok(output) => output,
        Err(_) => Command::new("pacman")
            .args(["-Qeq"])
            .output()
            .map_err(|e| anyhow!("Failed to query explicit packages: {}", e))?,
    };

    if !output.status.success() {
        return Err(anyhow!(
            "{} -Qeq failed: {}",
            crate::internal::constants::PACKAGE_MANAGER,
            String::from_utf8_lossy(&output.stderr).trim()
        ));
    }

    Ok(String::from_utf8_lossy(&output.stdout)
        .lines()
        .map(str::trim)
        .filter(|line| !line.is_empty())
        .map(ToString::to_string)
        .collect())
}

fn prompt_package_action(package_name: &str) -> Option<PackageAction> {
    loop {
        print!(
            "Package '{}' -> [a]dopt / [i]gnore / [s]kip / [q]uit: ",
            package_name
        );
        std::io::stdout().flush().ok()?;

        let mut input = String::new();
        std::io::stdin().read_line(&mut input).ok()?;
        match input.trim().to_lowercase().as_str() {
            "a" | "adopt" => return Some(PackageAction::Adopt),
            "i" | "ignore" => return Some(PackageAction::Ignore),
            "s" | "skip" => return Some(PackageAction::Skip),
            "q" | "quit" => return Some(PackageAction::Quit),
            _ => println!("{}", color::red("Invalid choice, try again")),
        }
    }
}

fn prompt_config_file_selection() -> Result<Option<String>> {
    let mut config_files = crate::internal::files::get_all_config_files()?;

    if config_files.is_empty() {
        config_files.push(get_main_config_path()?);
    }

    println!();
    println!(
        "{}",
        color::bold("Select config file to write adopted packages:")
    );
    for (idx, path) in config_files.iter().enumerate() {
        let friendly = path.replace(&std::env::var("HOME").unwrap_or_default(), "~");
        println!("  [{}] {}", idx, color::highlight(&friendly));
    }

    loop {
        print!(
            "Config index (0-{}, or 'c' to cancel): ",
            config_files.len() - 1
        );
        std::io::stdout().flush().ok();

        let mut input = String::new();
        std::io::stdin()
            .read_line(&mut input)
            .map_err(|e| anyhow!("Failed to read selection: {}", e))?;

        let input = input.trim();
        if input.eq_ignore_ascii_case("c") || input.eq_ignore_ascii_case("cancel") {
            return Ok(None);
        }

        if let Ok(idx) = input.parse::<usize>() {
            if idx < config_files.len() {
                return Ok(Some(config_files[idx].clone()));
            }
        }
        println!("{}", color::red("Invalid selection, try again"));
    }
}

fn get_main_config_path() -> Result<String> {
    let home = std::env::var("HOME").map_err(|_| anyhow!("HOME environment variable not set"))?;
    let path = PathBuf::from(home)
        .join(crate::internal::constants::OWL_DIR)
        .join(crate::internal::constants::MAIN_CONFIG_FILE);
    Ok(path.to_string_lossy().into_owned())
}

fn add_package_to_file(package_name: &str, file_path: &str) -> Result<AddResult> {
    use std::fs;

    let path = Path::new(file_path);
    if let Some(parent) = path.parent() {
        fs::create_dir_all(parent).map_err(|e| {
            anyhow!(
                "Failed to create config directory '{}': {}",
                parent.display(),
                e
            )
        })?;
    }

    let content = if path.exists() {
        fs::read_to_string(path)
            .map_err(|e| anyhow!("Failed to read config file '{}': {}", file_path, e))?
    } else {
        String::new()
    };

    if config_contains_package(package_name, &content) {
        return Ok(AddResult::AlreadyPresent);
    }

    let mut lines: Vec<String> = content.lines().map(|s| s.to_string()).collect();
    let mut inserted = false;

    for i in 0..lines.len() {
        let trimmed = lines[i].trim();
        if trimmed == "@packages" || trimmed == "@pkgs" {
            lines.insert(i + 1, package_name.to_string());
            inserted = true;
            break;
        }
    }

    if !inserted {
        if !lines.is_empty() && !lines.last().map(|line| line.is_empty()).unwrap_or(false) {
            lines.push(String::new());
        }
        lines.push("@packages".to_string());
        lines.push(package_name.to_string());
    }

    let new_content = lines.join("\n") + "\n";
    fs::write(path, new_content)
        .map_err(|e| anyhow!("Failed to write config file '{}': {}", file_path, e))?;

    Ok(AddResult::Added)
}

fn config_contains_package(package_name: &str, content: &str) -> bool {
    if let Ok(parsed) = Config::parse(content) {
        return parsed.packages.contains_key(package_name);
    }
    content.lines().any(|line| line.trim() == package_name)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_discover_candidates_filters_state_and_config() {
        let mut state = PackageState {
            untracked: Vec::new(),
            hidden: Vec::new(),
            managed: Vec::new(),
        };
        state.add_managed("managed".to_string());
        state.add_untracked("ignored".to_string());

        let mut config = Config::new();
        config.packages.insert(
            "in-config".to_string(),
            crate::core::config::Package {
                config: Vec::new(),
                service: None,
                env_vars: std::collections::HashMap::new(),
            },
        );

        let explicit_installed = HashSet::from([
            "managed".to_string(),
            "ignored".to_string(),
            "in-config".to_string(),
            "candidate-a".to_string(),
            "candidate-b".to_string(),
        ]);

        let candidates = discover_candidates_from_explicit(&explicit_installed, &state, &config);
        assert_eq!(
            candidates,
            vec!["candidate-a".to_string(), "candidate-b".to_string()]
        );
    }

    #[test]
    fn test_add_package_to_file_creates_packages_section() {
        let temp = tempfile::tempdir().expect("failed to create temp dir");
        let path = temp.path().join("main.owl");
        let result = add_package_to_file("htop", path.to_str().expect("utf8 path"));
        assert!(matches!(result, Ok(AddResult::Added)));

        let content = std::fs::read_to_string(path).expect("failed to read file");
        assert!(content.contains("@packages\nhtop\n"));
    }
}
