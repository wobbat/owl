use anyhow::{Result, anyhow};
use std::fs;

use crate::core::config::Config;
use crate::internal::color;

pub fn handle_clean(filename: &str) -> Result<()> {
    // Read and parse the config file
    let config =
        Config::parse_file(filename).map_err(|e| anyhow!("Failed to parse {}: {}", filename, e))?;

    // Optimize the config
    let optimized_content = optimize_config(&config);

    // Write back to the file
    fs::write(filename, optimized_content)
        .map_err(|e| anyhow!("Failed to write {}: {}", filename, e))?;

    Ok(())
}

pub fn handle_clean_all() -> Result<()> {
    let config_files =
        get_all_config_files().map_err(|e| anyhow!("Failed to discover config files: {}", e))?;

    if config_files.is_empty() {
        println!("[{}]", color::blue("clean"));
        println!(
            "  {} {}",
            color::green("➔"),
            color::dim("no .owl config files found in ~/.owl directory")
        );
        return Ok(());
    }

    println!("[{}]", color::blue("clean"));
    println!(
        "  {} config files cleaned",
        color::yellow(&config_files.len().to_string())
    );

    let mut failed_count = 0;

    for filename in config_files {
        match handle_clean(&filename) {
            Ok(()) => {
                // File cleaned successfully
            }
            Err(e) => {
                failed_count += 1;
                eprintln!("  {} {}: {}", color::red("✗"), color::dim(&filename), e);
            }
        }
    }

    if failed_count > 0 {
        println!();
        println!(
            "  {} {}",
            color::red("failed"),
            color::bold(&failed_count.to_string())
        );
    }

    Ok(())
}

fn get_all_config_files() -> Result<Vec<String>> {
    crate::internal::files::get_all_config_files()
}

fn optimize_config(config: &Config) -> String {
    let mut sections: Vec<String> = Vec::new();

    // Collect packages without directives
    let mut loose_packages: Vec<String> = Vec::new();
    let mut packages_with_directives: Vec<String> = Vec::new();

    for (name, pkg) in &config.packages {
        if pkg.config.is_empty() && pkg.service.is_none() && pkg.env_vars.is_empty() {
            loose_packages.push(name.clone());
        } else {
            let mut block = format!("@pkg {}\n", name);
            // Output :cfg directives
            for cfg in &pkg.config {
                block.push_str(&format!(":cfg {}\n", cfg));
            }
            // Output :service
            if let Some(service) = &pkg.service {
                block.push_str(&format!(":service {}\n", service));
            }
            // Output :env
            for (key, value) in &pkg.env_vars {
                block.push_str(&format!(":env {}={}\n", key, value));
            }
            packages_with_directives.push(block.trim_end().to_string());
        }
    }

    // Sort loose packages for consistency
    loose_packages.sort();

    // Sort packages with directives by package name
    packages_with_directives.sort_by(|a, b| {
        let name_a = a
            .lines()
            .next()
            .unwrap_or("")
            .trim_start_matches("@pkg ")
            .to_string();
        let name_b = b
            .lines()
            .next()
            .unwrap_or("")
            .trim_start_matches("@pkg ")
            .to_string();
        name_a.cmp(&name_b)
    });

    // Reorganize output order: @group -> @env -> @pkg blocks -> @pkgs list

    // Add groups as the first section (sorted alphabetically)
    if !config.groups.is_empty() {
        let mut sorted_groups = config.groups.clone();
        sorted_groups.sort();
        let mut group_block = String::new();
        for group in sorted_groups {
            group_block.push_str(&format!("@group {}\n", group));
        }
        sections.push(group_block.trim_end().to_string());
    }

    // Add global env vars as the second section
    if !config.env_vars.is_empty() {
        let mut env_block = String::new();
        for (key, value) in &config.env_vars {
            env_block.push_str(&format!("@env {}={}\n", key, value));
        }
        sections.push(env_block.trim_end().to_string());
    }

    // Add packages with directives as the third section
    sections.extend(packages_with_directives);

    // Add @pkgs section at the end
    if !loose_packages.is_empty() {
        let mut pkgs_block = "@pkgs\n".to_string();
        for pkg in loose_packages {
            pkgs_block.push_str(&format!("{}\n", pkg));
        }
        sections.push(pkgs_block.trim_end().to_string());
    }

    // Join sections and remove trailing spaces from each line
    sections
        .join("\n\n")
        .lines()
        .map(str::trim_end)
        .collect::<Vec<_>>()
        .join("\n")
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::core::config::Config;

    #[test]
    fn test_optimize_config() {
        let content = r"@package loose1

@package withconfig
:config test -> ~/.config/test

@packages
loose2
loose3

@env GLOBAL=val
@group testgroup";

        let config = Config::parse(content).unwrap();
        let optimized = optimize_config(&config);

        let expected = r"@group testgroup

@env GLOBAL=val

@pkg withconfig
:cfg test -> ~/.config/test

@pkgs
loose1
loose2
loose3";

        assert_eq!(optimized, expected);
    }
}
