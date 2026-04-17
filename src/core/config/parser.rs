use anyhow::{Context, Result, anyhow};
use std::collections::HashMap;
use std::path::Path;

use super::{Config, Package};

impl Config {
    pub fn parse_file<P: AsRef<Path>>(path: P) -> Result<Self> {
        let content = std::fs::read_to_string(path.as_ref())
            .with_context(|| format!("Failed to read config file: {}", path.as_ref().display()))?;
        Self::parse(&content)
    }

    pub fn parse(content: &str) -> Result<Self> {
        let mut config = Config::new();
        let mut current_package: Option<String> = None;
        let mut in_packages_section = false;

        for (idx, line) in content.lines().enumerate() {
            let line_number = idx + 1;
            let trimmed = line.trim();

            if trimmed.is_empty() || trimmed.starts_with('#') {
                continue;
            }

            Self::parse_line(
                &mut config,
                &mut current_package,
                &mut in_packages_section,
                trimmed,
                line_number,
            )?;
        }

        Ok(config)
    }

    fn parse_line(
        config: &mut Config,
        current_package: &mut Option<String>,
        in_packages_section: &mut bool,
        line: &str,
        line_number: usize,
    ) -> Result<()> {
        if line == "@package"
            || line.starts_with("@package ")
            || line == "@pkg"
            || line.starts_with("@pkg ")
        {
            Self::parse_package_declaration(
                config,
                current_package,
                in_packages_section,
                line,
                line_number,
            )?;
        } else if line == "@packages" || line == "@pkgs" {
            Self::parse_packages_section(in_packages_section, current_package);
        } else if line == "@env" || line.starts_with("@env ") {
            Self::parse_global_env_directive(config, line, line_number)?;
        } else if line == "@group" || line.starts_with("@group ") {
            Self::parse_group_declaration(config, current_package, line, line_number)?;
        } else if line == ":config" || line.starts_with(":config ") {
            Self::parse_config_directive(config, current_package, line, ":config ", line_number)?;
        } else if line == ":cfg" || line.starts_with(":cfg ") {
            Self::parse_config_directive(config, current_package, line, ":cfg ", line_number)?;
        } else if line == ":service" || line.starts_with(":service ") {
            Self::parse_service_directive(config, current_package, line, line_number)?;
        } else if line == ":env" || line.starts_with(":env ") {
            Self::parse_package_env_directive(config, current_package, line, line_number)?;
        } else if !line.starts_with('@') && !line.starts_with(':') && *in_packages_section {
            Self::parse_package_in_section(config, line);
        } else if line.starts_with('@') || line.starts_with(':') {
            // Ignore unknown directives for forward compatibility.
        }

        Ok(())
    }

    fn parse_package_declaration(
        config: &mut Config,
        current_package: &mut Option<String>,
        in_packages_section: &mut bool,
        line: &str,
        line_number: usize,
    ) -> Result<()> {
        *in_packages_section = false;
        let name = if let Some(name) = line
            .strip_prefix("@package ")
            .or_else(|| line.strip_prefix("@package"))
        {
            name.trim()
        } else if let Some(name) = line.strip_prefix("@pkg ").or_else(|| line.strip_prefix("@pkg"))
        {
            name.trim()
        } else {
            line.trim()
        };

        if name.is_empty() {
            return Err(anyhow!(
                "Line {}: package directive requires a package name",
                line_number
            ));
        }

        *current_package = Some(name.to_string());
        config.packages.insert(
            name.to_string(),
            Package {
                config: Vec::new(),
                service: None,
                env_vars: HashMap::new(),
            },
        );

        Ok(())
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
        line_number: usize,
    ) -> Result<()> {
        let name = line
            .strip_prefix("@group ")
            .or_else(|| line.strip_prefix("@group"))
            .ok_or_else(|| anyhow!("Invalid @group directive format"))?
            .trim();

        if name.is_empty() {
            return Err(anyhow!(
                "Line {}: @group directive requires a group name",
                line_number
            ));
        }

        config.groups.push(name.to_string());
        *current_package = None;
        Ok(())
    }

    fn parse_package_in_section(config: &mut Config, line: &str) {
        let package_name = line.trim();
        if !package_name.is_empty() && !package_name.starts_with('#') {
            config.packages.insert(
                package_name.to_string(),
                Package {
                    config: Vec::new(),
                    service: None,
                    env_vars: HashMap::new(),
                },
            );
        }
    }

    fn parse_config_directive(
        config: &mut Config,
        current_package: &Option<String>,
        line: &str,
        prefix: &str,
        line_number: usize,
    ) -> Result<()> {
        let rest = line
            .strip_prefix(prefix)
            .or_else(|| line.strip_prefix(prefix.trim()))
            .ok_or_else(|| anyhow!("Invalid config directive format"))?
            .trim();

        if rest.is_empty() {
            return Err(anyhow!(
                "Line {}: {} directive requires a value",
                line_number,
                prefix.trim()
            ));
        }

        let Some(pkg_name) = current_package else {
            return Err(anyhow!(
                "Line {}: {} directive found outside of a package context",
                line_number,
                prefix.trim()
            ));
        };

        let Some(package) = config.packages.get_mut(pkg_name) else {
            return Err(anyhow!(
                "Line {}: Package '{}' not found in config",
                line_number,
                pkg_name
            ));
        };

        if let Some((source, sink)) = rest.split_once(" -> ") {
            let source = source.trim();
            let sink = sink.trim();

            if source.is_empty() {
                return Err(anyhow!(
                    "Line {}: Config source path cannot be empty",
                    line_number
                ));
            }
            if sink.is_empty() {
                return Err(anyhow!(
                    "Line {}: Config destination path cannot be empty",
                    line_number
                ));
            }

            package.config.push(format!("{} -> {}", source, sink));
        } else {
            package.config.push(rest.to_string());
        }

        Ok(())
    }

    fn parse_service_directive(
        config: &mut Config,
        current_package: &Option<String>,
        line: &str,
        line_number: usize,
    ) -> Result<()> {
        let service_part = line
            .strip_prefix(":service ")
            .or_else(|| line.strip_prefix(":service"))
            .ok_or_else(|| anyhow!("Invalid :service directive format"))?;
        let service_name = service_part
            .split('[')
            .next()
            .unwrap_or(service_part)
            .trim();

        if service_name.is_empty() {
            return Err(anyhow!(
                "Line {}: :service directive requires a service name",
                line_number
            ));
        }

        let Some(pkg_name) = current_package else {
            return Err(anyhow!(
                "Line {}: :service directive found outside of a package context",
                line_number
            ));
        };

        let Some(package) = config.packages.get_mut(pkg_name) else {
            return Err(anyhow!(
                "Line {}: Package '{}' not found in config",
                line_number,
                pkg_name
            ));
        };

        package.service = Some(service_name.to_string());
        Ok(())
    }

    fn parse_package_env_directive(
        config: &mut Config,
        current_package: &Option<String>,
        line: &str,
        line_number: usize,
    ) -> Result<()> {
        let env_part = line
            .strip_prefix(":env ")
            .or_else(|| line.strip_prefix(":env"))
            .ok_or_else(|| anyhow!("Invalid :env directive format"))?;

        let Some((key, value)) = env_part.split_once('=') else {
            return Err(anyhow!(
                "Line {}: :env directive must be in format 'KEY=value' (missing '=')",
                line_number
            ));
        };

        let key = key.trim();
        let value = value.trim();

        if key.is_empty() {
            return Err(anyhow!(
                "Line {}: Environment variable name cannot be empty",
                line_number
            ));
        }

        let Some(pkg_name) = current_package else {
            return Err(anyhow!(
                "Line {}: :env directive found outside of a package context",
                line_number
            ));
        };

        let Some(package) = config.packages.get_mut(pkg_name) else {
            return Err(anyhow!(
                "Line {}: Package '{}' not found in config",
                line_number,
                pkg_name
            ));
        };

        package.env_vars.insert(key.to_string(), value.to_string());
        Ok(())
    }

    fn parse_global_env_directive(
        config: &mut Config,
        line: &str,
        line_number: usize,
    ) -> Result<()> {
        let env_part = line
            .strip_prefix("@env ")
            .or_else(|| line.strip_prefix("@env"))
            .ok_or_else(|| anyhow!("Invalid @env directive format"))?;

        let Some((key, value)) = env_part.split_once('=') else {
            return Err(anyhow!(
                "Line {}: @env directive must be in format 'KEY=value' (missing '=')",
                line_number
            ));
        };

        let key = key.trim();
        let value = value.trim();

        if key.is_empty() {
            return Err(anyhow!(
                "Line {}: Environment variable name cannot be empty",
                line_number
            ));
        }

        config.env_vars.insert(key.to_string(), value.to_string());
        Ok(())
    }
}
