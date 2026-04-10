use anyhow::{Result, anyhow};
use std::collections::HashMap;
use std::path::Path;

use super::{Config, Package};

impl Config {
    pub fn parse_file<P: AsRef<Path>>(path: P) -> Result<Self> {
        let content = std::fs::read_to_string(path)
            .map_err(|e| anyhow!("Failed to read config file: {}", e))?;
        Self::parse(&content)
    }

    pub fn parse(content: &str) -> Result<Self> {
        let mut config = Config::new();
        let mut current_package: Option<String> = None;
        let mut in_packages_section = false;

        for line in content.lines() {
            let line = line.trim();

            // Skip empty lines and comments
            if line.is_empty() || line.starts_with('#') {
                continue;
            }

            Self::parse_line(
                &mut config,
                &mut current_package,
                &mut in_packages_section,
                line,
            )?;
        }

        Ok(config)
    }

    fn parse_line(
        config: &mut Config,
        current_package: &mut Option<String>,
        in_packages_section: &mut bool,
        line: &str,
    ) -> Result<()> {
        if line.starts_with("@package ") || line.starts_with("@pkg ") {
            Self::parse_package_declaration(config, current_package, in_packages_section, line);
        } else if line == "@packages" || line == "@pkgs" {
            Self::parse_packages_section(in_packages_section, current_package);
        } else if line.starts_with(":config ") {
            Self::parse_config_directive(config, current_package, line, ":config ")?;
        } else if line.starts_with(":cfg ") {
            Self::parse_config_directive(config, current_package, line, ":cfg ")?;
        } else if line.starts_with(":service ") {
            Self::parse_service_directive(config, current_package, line)?;
        } else if line.starts_with(":env ") {
            Self::parse_package_env_directive(config, current_package, line)?;
        } else if line.starts_with("@env ") {
            Self::parse_global_env_directive(config, line)?;
        } else if line.starts_with("@group ") {
            Self::parse_group_declaration(config, current_package, line);
        } else if !line.starts_with('@') && !line.starts_with(':') && *in_packages_section {
            Self::parse_package_in_section(config, line);
        }
        // Ignore unknown lines
        Ok(())
    }

    fn parse_package_declaration(
        config: &mut Config,
        current_package: &mut Option<String>,
        in_packages_section: &mut bool,
        line: &str,
    ) {
        *in_packages_section = false;
        let name = if let Some(name) = line.strip_prefix("@package ") {
            name.trim().to_string()
        } else if let Some(name) = line.strip_prefix("@pkg ") {
            name.trim().to_string()
        } else {
            // This shouldn't happen since we check the prefix in parse_line
            line.trim().to_string()
        };
        *current_package = Some(name.clone());
        config.packages.insert(
            name.clone(),
            Package {
                config: Vec::new(),
                service: None,
                env_vars: HashMap::new(),
            },
        );
    }

    fn parse_packages_section(
        in_packages_section: &mut bool,
        current_package: &mut Option<String>,
    ) {
        *in_packages_section = true;
        *current_package = None;
    }

    fn parse_group_declaration(
        config: &mut Config,
        current_package: &mut Option<String>,
        line: &str,
    ) {
        config
            .groups
            .push(line.strip_prefix("@group ").unwrap().trim().to_string());
        *current_package = None;
    }

    fn parse_package_in_section(config: &mut Config, line: &str) {
        let package_name = line.trim().to_string();
        config.packages.insert(
            package_name.clone(),
            Package {
                config: Vec::new(),
                service: None,
                env_vars: HashMap::new(),
            },
        );
    }

    fn parse_config_directive(
        config: &mut Config,
        current_package: &Option<String>,
        line: &str,
        prefix: &str,
    ) -> Result<()> {
        let rest = line.strip_prefix(prefix).unwrap();
        if let Some((source, sink)) = rest.split_once(" -> ") {
            if let Some(pkg_name) = current_package
                && let Some(package) = config.packages.get_mut(pkg_name)
            {
                package
                    .config
                    .push(format!("{} -> {}", source.trim(), sink.trim()));
            }
        } else if let Some(pkg_name) = current_package
            && let Some(package) = config.packages.get_mut(pkg_name)
        {
            package.config.push(rest.trim().to_string());
        }
        Ok(())
    }

    fn parse_service_directive(
        config: &mut Config,
        current_package: &Option<String>,
        line: &str,
    ) -> Result<()> {
        let service_part = line.strip_prefix(":service ").unwrap();
        let service_name = service_part
            .split('[')
            .next()
            .unwrap_or(service_part)
            .trim();
        if let Some(pkg_name) = current_package
            && let Some(package) = config.packages.get_mut(pkg_name)
        {
            package.service = Some(service_name.to_string());
        }
        Ok(())
    }

    fn parse_package_env_directive(
        config: &mut Config,
        current_package: &Option<String>,
        line: &str,
    ) -> Result<()> {
        let env_part = line.strip_prefix(":env ").unwrap();
        if let Some((key, value)) = env_part.split_once('=')
            && let Some(pkg_name) = current_package
            && let Some(package) = config.packages.get_mut(pkg_name)
        {
            package
                .env_vars
                .insert(key.trim().to_string(), value.trim().to_string());
        }
        Ok(())
    }

    fn parse_global_env_directive(config: &mut Config, line: &str) -> Result<()> {
        let env_part = line.strip_prefix("@env ").unwrap();
        if let Some((key, value)) = env_part.split_once('=') {
            config
                .env_vars
                .insert(key.trim().to_string(), value.trim().to_string());
        }
        Ok(())
    }
}
