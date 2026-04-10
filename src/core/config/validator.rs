use super::Config;
use anyhow::{Result, anyhow};

/// Validate a provided .owl config file can be parsed
pub fn run_configcheck(path: &str) -> Result<()> {
    let p = std::path::Path::new(path);
    if !p.exists() {
        return Err(anyhow!("Config file not found: {}", path));
    }
    match Config::parse_file(p) {
        Ok(_) => {
            println!(
                "{} {}",
                crate::internal::color::green("✓"),
                crate::internal::color::bold(&format!("Config valid: {}", path))
            );
            Ok(())
        }
        Err(e) => Err(anyhow!("Failed to parse {}: {}", path, e)),
    }
}

/// Validate and print the full config chain (main, hostname, groups)
pub fn run_full_configcheck() -> Result<()> {
    let home = std::env::var("HOME").map_err(|_| anyhow!("HOME environment variable not set"))?;
    let owl_root = std::path::Path::new(&home).join(crate::internal::constants::OWL_DIR);
    println!("Loading config from: {}", owl_root.display());

    // Check main config
    let main_config_path = owl_root.join(crate::internal::constants::MAIN_CONFIG_FILE);
    println!(
        "Main config: {} (exists: {})",
        main_config_path.display(),
        main_config_path.exists()
    );

    // Check host config
    let hostname =
        crate::internal::constants::get_host_name().unwrap_or_else(|_| "unknown".to_string());
    let host_config_path = owl_root
        .join(crate::internal::constants::HOSTS_DIR)
        .join(format!(
            "{}{}",
            hostname,
            crate::internal::constants::OWL_EXT
        ));
    println!(
        "Host config: {} (exists: {})",
        host_config_path.display(),
        host_config_path.exists()
    );

    // Check groups
    let groups_path = owl_root.join(crate::internal::constants::GROUPS_DIR);
    println!(
        "Groups dir: {} (exists: {})",
        groups_path.display(),
        groups_path.exists()
    );
    if groups_path.exists()
        && let Ok(entries) = std::fs::read_dir(&groups_path)
    {
        for entry in entries.flatten() {
            println!(
                "  Group file: {} (exists: {})",
                entry.path().display(),
                entry.path().exists()
            );
        }
    }

    match Config::load_all_relevant_config_files() {
        Ok(config) => {
            println!(
                "{}",
                crate::internal::color::green("✓ Full config chain loaded successfully")
            );
            println!(
                "{}",
                serde_json::to_string_pretty(&config)
                    .map_err(|e| anyhow!("Failed to serialize config: {}", e))?
            );

            // Print summary
            let package_count = config.packages.len();
            let dotfile_count = config
                .packages
                .values()
                .filter(|pkg| !pkg.config.is_empty())
                .count();
            let service_count = config
                .packages
                .values()
                .filter(|pkg| pkg.service.is_some())
                .count();
            let env_var_count = config
                .packages
                .values()
                .map(|pkg| pkg.env_vars.len())
                .sum::<usize>()
                + config.env_vars.len();
            let group_count = config.groups.len();

            println!();
            println!("Summary:");
            println!("  Packages: {}", package_count);
            println!("  Dotfiles: {}", dotfile_count);
            println!("  Services: {}", service_count);
            println!("  Environment variables: {}", env_var_count);
            println!("  Groups: {}", group_count);

            Ok(())
        }
        Err(e) => Err(anyhow!("Failed to load full config: {}", e)),
    }
}

/// Show the host-specific config path for this machine
pub fn run_confighost() -> Result<()> {
    let hostname =
        crate::internal::constants::get_host_name().unwrap_or_else(|_| "unknown".to_string());
    let home = std::env::var("HOME").map_err(|_| anyhow!("HOME environment variable not set"))?;
    let path = std::path::Path::new(&home)
        .join(crate::internal::constants::OWL_DIR)
        .join("hosts")
        .join(format!("{}.owl", hostname));
    println!(
        "Host config: {}",
        crate::internal::color::bold(&path.to_string_lossy())
    );
    Ok(())
}

/// Return list of packages declared in config that are not installed
#[cfg(test)]
pub fn get_uninstalled_packages(config: &Config) -> Result<Vec<String>> {
    let installed = crate::core::package::get_installed_packages()?;
    let mut missing = Vec::new();
    for name in config.packages.keys() {
        if !installed.contains(name) {
            missing.push(name.clone());
        }
    }
    Ok(missing)
}
