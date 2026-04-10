use anyhow::{Result, anyhow};
use std::collections::HashMap;
use std::fs;

use crate::internal::files::owl_dir;

/// Get bash environment file path
fn env_file_bash() -> Result<std::path::PathBuf> {
    Ok(owl_dir()?.join(crate::internal::constants::ENV_BASH_FILE))
}

/// Get fish environment file path
fn env_file_fish() -> Result<std::path::PathBuf> {
    Ok(owl_dir()?.join(crate::internal::constants::ENV_FISH_FILE))
}

pub fn collect_all_env_vars(config: &crate::core::config::Config) -> Vec<(String, String)> {
    let mut vars: HashMap<String, String> = HashMap::new();
    // Global first
    for (k, v) in &config.env_vars {
        vars.insert(k.clone(), v.clone());
    }
    // Package-level, override globals
    for pkg in config.packages.values() {
        for (k, v) in &pkg.env_vars {
            vars.insert(k.clone(), v.clone());
        }
    }
    let mut sorted_environment_vars: Vec<(String, String)> = vars.into_iter().collect();
    sorted_environment_vars.sort_by(|a, b| a.0.cmp(&b.0));
    sorted_environment_vars
}

pub fn apply_environment_variables(
    config: &crate::core::config::Config,
    dry_run: bool,
) -> Result<()> {
    let vars = collect_all_env_vars(config);
    if vars.is_empty() {
        return Ok(());
    }

    if dry_run {
        println!("  {} Plan:", crate::internal::color::blue("info:"));
        for (k, v) in &vars {
            println!(
                "    ✓ Would export {}={} (shells)",
                crate::internal::color::yellow(k),
                crate::internal::color::green(v)
            );
        }
        return Ok(());
    }

    // Write bash
    let bash_path = env_file_bash()?;
    let mut bash = String::new();
    for (k, v) in &vars {
        bash.push_str(&format!("export {}=\"{}\"\n", k, v));
    }
    fs::write(&bash_path, bash)
        .map_err(|e| anyhow!("Failed to write {}: {}", bash_path.display(), e))?;

    // Write fish
    let fish_path = env_file_fish()?;
    let mut fish = String::new();
    for (k, v) in &vars {
        fish.push_str(&format!("set -x {} \"{}\"\n", k, v));
    }
    fs::write(&fish_path, fish)
        .map_err(|e| anyhow!("Failed to write {}: {}", fish_path.display(), e))?;

    println!(
        "  {} Environment exported (bash, fish)",
        crate::internal::color::green("⸎")
    );
    Ok(())
}
