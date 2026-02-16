use anyhow::{Result, anyhow};
use std::collections::HashMap;
use std::collections::HashSet;
use std::io::Write;
use std::process::{Command, Stdio};
use std::sync::{Mutex, OnceLock};
use std::thread;
use std::time::Duration;

/// Retry a command with exponential backoff for network-related failures
fn retry_command<F, T>(mut operation: F, max_retries: usize) -> Result<T>
where
    F: FnMut() -> Result<T>,
{
    let mut last_error = None;

    for attempt in 0..=max_retries {
        match operation() {
            Ok(result) => return Ok(result),
            Err(err) => {
                last_error = Some(err);

                // Check if this is a network-related error that we should retry
                let err_msg = last_error.as_ref().unwrap().to_string();
                let should_retry = err_msg.contains("Connection reset by peer")
                    || err_msg.contains("error sending request")
                    || err_msg.contains("error trying to connect")
                    || err_msg.contains("os error 104");

                if !should_retry || attempt == max_retries {
                    return Err(last_error.unwrap_or_else(|| anyhow::anyhow!("Unknown error")));
                }

                // Exponential backoff: 1s, 2s, 4s, 8s, 16s
                let delay = Duration::from_secs(1 << attempt);

                // Clear the current line and show retry status
                print!("\r\x1b[2K");
                std::io::stdout().flush().ok();
                print!(
                    "Retrying due to network errors... ({}/{})",
                    attempt + 1,
                    max_retries + 1
                );
                std::io::stdout().flush().ok();
                thread::sleep(delay);

                // Clear the line for the next operation
                print!("\r\x1b[2K");
                std::io::stdout().flush().ok();
            }
        }
    }

    Err(last_error.unwrap_or_else(|| anyhow!("Unknown error")))
}

#[derive(Debug, Clone, PartialEq)]
pub enum PackageSource {
    Repo,
    Aur,
}

#[derive(Debug, Clone)]
pub struct SearchResult {
    pub name: String,
    pub ver: String,
    pub source: PackageSource,
    pub repo: String,
    pub description: String,
    pub installed: bool,
}

pub trait PackageManager {
    fn list_installed(&self) -> Result<HashSet<String>>;
    fn batch_repo_available(&self, packages: &[String]) -> Result<HashSet<String>>;
    fn upgrade_count(&self) -> Result<usize>;
    fn get_aur_updates(&self) -> Result<Vec<String>>;
    fn install_repo(&self, packages: &[String]) -> Result<()>;
    fn install_aur(&self, packages: &[String]) -> Result<()>;
    fn update_repo(&self) -> Result<()>;
    fn update_aur(&self, packages: &[String]) -> Result<()>;
    fn remove_packages(&self, packages: &[String], quiet: bool) -> Result<()>;
    fn search_packages(&self, terms: &[String]) -> Result<Vec<SearchResult>>;
    fn is_package_group(&self, package_name: &str) -> Result<bool>;
    fn get_group_packages(&self, group_name: &str) -> Result<Vec<String>>;
}

pub struct ParuPacman;
impl ParuPacman {
    pub fn new() -> Self {
        Self
    }

    pub fn install_repo_with_mode(&self, packages: &[String], non_interactive: bool) -> Result<()> {
        if packages.is_empty() {
            return Ok(());
        }

        let manager = package_manager_command();
        let mut args: Vec<String> = if aur_helper_command().is_some() {
            vec!["--repo".to_string(), "-S".to_string()]
        } else {
            vec!["-S".to_string()]
        };
        if non_interactive {
            args.push("--noconfirm".to_string());
        }
        args.extend(packages.iter().cloned());
        let arg_refs: Vec<&str> = args.iter().map(String::as_str).collect();

        let status = if non_interactive {
            crate::internal::util::execute_command_with_spinner(
                manager,
                &arg_refs,
                &format!("Installing {} repo packages", packages.len()),
            )?
        } else {
            crate::internal::util::execute_command_interactive(
                manager,
                &arg_refs,
                &format!("Installing {} repo packages", packages.len()),
            )?
        };

        if !status.success() {
            return Err(anyhow::anyhow!("Repository install failed"));
        }
        Ok(())
    }

    pub fn install_aur_with_mode(&self, packages: &[String], non_interactive: bool) -> Result<()> {
        if packages.is_empty() {
            return Ok(());
        }

        let aur_helper = require_aur_helper()?;
        let mut args = vec!["--aur".to_string(), "-S".to_string()];
        if non_interactive {
            args.push("--noconfirm".to_string());
            args.push("--skipreview".to_string());
            args.push("--noprovides".to_string());
            args.push("--noupgrademenu".to_string());
        }
        args.extend(packages.iter().cloned());

        let status = if non_interactive {
            crate::internal::util::execute_command_with_retry(
                aur_helper,
                &args,
                &format!("Installing {} AUR packages", packages.len()),
                3,
            )?
        } else {
            let arg_refs: Vec<&str> = args.iter().map(String::as_str).collect();
            crate::internal::util::execute_command_interactive(
                aur_helper,
                &arg_refs,
                &format!("Installing {} AUR packages", packages.len()),
            )?
        };

        if !status.success() {
            return Err(anyhow::anyhow!("AUR install failed"));
        }
        Ok(())
    }

    pub fn update_repo_with_mode(&self, non_interactive: bool) -> Result<()> {
        let manager = package_manager_command();
        let mut args: Vec<String> = if aur_helper_command().is_some() {
            vec!["--repo".to_string(), "-Syu".to_string()]
        } else {
            vec!["-Syu".to_string()]
        };
        if non_interactive {
            args.push("--noconfirm".to_string());
        }
        let arg_refs: Vec<&str> = args.iter().map(String::as_str).collect();

        let status = if non_interactive {
            let (status, _stderr_out) = crate::internal::util::execute_command_with_stderr_capture(
                manager,
                &arg_refs,
                "Updating official repository packages (syncing databases and upgrading packages)",
            )?;
            status
        } else {
            crate::internal::util::execute_command_interactive(
                manager,
                &arg_refs,
                "Updating official repository packages (syncing databases and upgrading packages)",
            )?
        };

        if status.success() {
            println!(
                "  {} Official repos synced",
                crate::internal::color::green("⸎")
            );
            Ok(())
        } else if aur_helper_command().is_some() && status.code() == Some(1) {
            println!(
                "  {} Packages from main repos have been updated",
                crate::internal::color::green("⸎")
            );
            Ok(())
        } else {
            Err(anyhow::anyhow!(
                "Repository update failed (exit code: {:?})",
                status.code()
            ))
        }
    }

    pub fn update_aur_with_mode(&self, packages: &[String], non_interactive: bool) -> Result<()> {
        if packages.is_empty() {
            return Ok(());
        }

        let aur_helper = require_aur_helper()?;
        let mut args = vec!["--aur".to_string(), "-Syu".to_string()];
        if non_interactive {
            args.push("--noconfirm".to_string());
        }
        args.extend(packages.iter().cloned());

        if non_interactive {
            let arg_refs: Vec<&str> = args.iter().map(String::as_str).collect();
            let (status, stderr_out) = crate::internal::util::execute_command_with_stderr_capture(
                aur_helper,
                &arg_refs,
                "Updating AUR packages",
            )
            .map_err(|e| anyhow::anyhow!(e))?;
            if status.success() {
                println!(
                    "\r\x1b[2K  {} AUR package updates completed",
                    crate::internal::color::green("⸎")
                );
                Ok(())
            } else {
                let err = stderr_out.trim();
                if !err.is_empty() {
                    let take = 30usize;
                    err.lines()
                        .rev()
                        .take(take)
                        .for_each(|line| eprintln!("  {}", line));
                }
                Err(anyhow::anyhow!("AUR package update failed"))
            }
        } else {
            let arg_refs: Vec<&str> = args.iter().map(String::as_str).collect();
            let status = crate::internal::util::execute_command_interactive(
                aur_helper,
                &arg_refs,
                "Updating AUR packages",
            )?;
            if status.success() {
                println!(
                    "  {} AUR package updates completed",
                    crate::internal::color::green("⸎")
                );
                Ok(())
            } else {
                Err(anyhow::anyhow!("AUR package update failed"))
            }
        }
    }
}

// Cache for package groups to avoid repeated pacman -Sg calls
static GROUP_CACHE: OnceLock<Mutex<HashMap<String, bool>>> = OnceLock::new();
static GROUP_PACKAGES_CACHE: OnceLock<Mutex<HashMap<String, Vec<String>>>> = OnceLock::new();
static AUR_HELPER: OnceLock<Option<String>> = OnceLock::new();
static PACKAGE_MANAGER_COMMAND: OnceLock<String> = OnceLock::new();

fn command_exists(command: &str) -> bool {
    Command::new(command)
        .arg("--version")
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .status()
        .map(|status| status.success())
        .unwrap_or(false)
}

pub fn aur_helper_command() -> Option<&'static str> {
    AUR_HELPER
        .get_or_init(|| {
            if command_exists("paru") {
                Some("paru".to_string())
            } else if command_exists("yay") {
                Some("yay".to_string())
            } else {
                None
            }
        })
        .as_deref()
}

pub fn package_manager_command() -> &'static str {
    PACKAGE_MANAGER_COMMAND
        .get_or_init(|| aur_helper_command().unwrap_or("pacman").to_string())
        .as_str()
}

fn require_aur_helper() -> Result<&'static str> {
    aur_helper_command().ok_or_else(|| {
        anyhow!("No AUR helper found. Install either 'paru' or 'yay' to manage AUR packages.")
    })
}

impl PackageManager for ParuPacman {
    fn list_installed(&self) -> Result<HashSet<String>> {
        let manager = package_manager_command();
        let output = Command::new(manager)
            .arg("-Qq")
            .output()
            .map_err(|e| anyhow::anyhow!("Failed to get installed packages: {}", e))?;
        if !output.status.success() {
            return Err(anyhow::anyhow!(
                "Package manager failed: {}",
                String::from_utf8_lossy(&output.stderr)
            ));
        }
        let stdout = String::from_utf8_lossy(&output.stdout);
        let installed = stdout
            .lines()
            .map(|line| line.trim())
            .filter(|name| !name.is_empty())
            .map(|name| name.to_string())
            .collect::<HashSet<_>>();
        Ok(installed)
    }

    fn batch_repo_available(&self, packages: &[String]) -> Result<HashSet<String>> {
        if packages.is_empty() {
            return Ok(HashSet::new());
        }

        // Use a single pacman call for all packages to improve performance
        let mut cmd = Command::new("pacman");
        cmd.arg("-Si");
        cmd.args(packages);

        let output = cmd
            .output()
            .map_err(|e| anyhow::anyhow!("Failed to check package info: {}", e))?;

        let stdout = String::from_utf8_lossy(&output.stdout);
        let repo_names = stdout
            .lines()
            .filter_map(|line| {
                line.strip_prefix("Name")
                    .and_then(|rest| {
                        rest.find(':').map(|idx| {
                            let value = rest[idx + 1..].trim();
                            if !value.is_empty() {
                                Some(value.to_string())
                            } else {
                                None
                            }
                        })
                    })
                    .flatten()
            })
            .collect::<HashSet<_>>();
        Ok(repo_names)
    }

    fn upgrade_count(&self) -> Result<usize> {
        let manager = package_manager_command();
        retry_command(
            || {
                let output = Command::new(manager)
                    .args(["-Qu", "-q"])
                    .output()
                    .map_err(|e| anyhow::anyhow!("Failed to run {} -Qu: {}", manager, e))?;
                if output.status.success() {
                    let stdout = String::from_utf8_lossy(&output.stdout);
                    Ok(stdout.lines().count())
                } else {
                    let stderr = String::from_utf8_lossy(&output.stderr);
                    if output.status.code() == Some(1) && stderr.trim().is_empty() {
                        Ok(0)
                    } else {
                        Err(anyhow::anyhow!("{} -Qu failed: {}", manager, stderr))
                    }
                }
            },
            3, // Max 3 retries
        )
    }

    fn get_aur_updates(&self) -> Result<Vec<String>> {
        let aur_helper = require_aur_helper()?;
        retry_command(
            || {
                let output = Command::new(aur_helper)
                    .args(["-Qua", "-q"])
                    .output()
                    .map_err(|e| anyhow::anyhow!("Failed to check AUR updates: {}", e))?;
                if output.status.success() {
                    let stdout = String::from_utf8_lossy(&output.stdout);
                    let packages: Vec<String> = stdout
                        .lines()
                        .filter_map(|line| {
                            let l = line.trim();
                            if l.is_empty() {
                                return None;
                            }
                            Some(l.split_whitespace().next().unwrap_or(l).to_string())
                        })
                        .collect();
                    Ok(packages)
                } else {
                    let stderr = String::from_utf8_lossy(&output.stderr);
                    if output.status.code() == Some(1) && stderr.trim().is_empty() {
                        // Treat as no updates
                        Ok(Vec::new())
                    } else {
                        Err(anyhow::anyhow!("AUR update check failed: {}", stderr))
                    }
                }
            },
            3, // Max 3 retries
        )
    }

    fn install_repo(&self, packages: &[String]) -> Result<()> {
        self.install_repo_with_mode(packages, true)
    }

    fn install_aur(&self, packages: &[String]) -> Result<()> {
        self.install_aur_with_mode(packages, true)
    }

    fn update_repo(&self) -> Result<()> {
        self.update_repo_with_mode(true)
    }

    fn update_aur(&self, packages: &[String]) -> Result<()> {
        self.update_aur_with_mode(packages, true)
    }

    fn remove_packages(&self, packages: &[String], quiet: bool) -> Result<()> {
        if packages.is_empty() {
            return Ok(());
        }
        let mut cmd = Command::new(package_manager_command());
        cmd.arg("-Rns");
        if quiet {
            cmd.arg("--noconfirm");
        }
        cmd.args(packages);
        let status = cmd
            .status()
            .map_err(|e| anyhow::anyhow!("Failed to remove packages: {}", e))?;
        if status.success() {
            println!(
                "  {} Removed {} package(s)",
                crate::internal::color::green("✓"),
                packages.len()
            );
            Ok(())
        } else {
            Err(anyhow::anyhow!("Package removal failed"))
        }
    }

    fn search_packages(&self, terms: &[String]) -> Result<Vec<SearchResult>> {
        if terms.is_empty() {
            return Ok(Vec::new());
        }
        retry_command(
            || {
                let manager = aur_helper_command().unwrap_or("pacman");
                let mut cmd = Command::new(manager);
                cmd.arg("-Ss");
                if aur_helper_command().is_some() {
                    cmd.arg("--bottomup");
                }
                cmd.args(terms);
                let output = cmd
                    .output()
                    .map_err(|e| anyhow::anyhow!("Failed to run {} search: {}", manager, e))?;
                if !output.status.success() {
                    let stderr = String::from_utf8_lossy(&output.stderr);
                    return Err(anyhow::anyhow!("{} search failed: {}", manager, stderr));
                }
                let text = String::from_utf8_lossy(&output.stdout);
                parse_paru_search_output(&text)
            },
            3, // Max 3 retries
        )
    }

    fn is_package_group(&self, package_name: &str) -> Result<bool> {
        // Check cache first
        let cache = GROUP_CACHE.get_or_init(|| Mutex::new(HashMap::new()));

        {
            let cache_guard = cache.lock().unwrap();
            if let Some(&is_group) = cache_guard.get(package_name) {
                return Ok(is_group);
            }
        }

        let output = Command::new("pacman")
            .args(["-Sg", package_name])
            .output()
            .map_err(|e| {
                anyhow::anyhow!("Failed to check if {} is a group: {}", package_name, e)
            })?;

        // If pacman -Sg succeeds and returns output, it's a group
        let is_group =
            output.status.success() && !String::from_utf8_lossy(&output.stdout).trim().is_empty();

        // Cache the result
        {
            let mut cache_guard = cache.lock().unwrap();
            cache_guard.insert(package_name.to_string(), is_group);
        }

        Ok(is_group)
    }

    fn get_group_packages(&self, group_name: &str) -> Result<Vec<String>> {
        // Check cache first
        let cache = GROUP_PACKAGES_CACHE.get_or_init(|| Mutex::new(HashMap::new()));

        {
            let cache_guard = cache.lock().unwrap();
            if let Some(packages) = cache_guard.get(group_name) {
                return Ok(packages.clone());
            }
        }

        let output = Command::new("pacman")
            .args(["-Sg", group_name])
            .output()
            .map_err(|e| {
                anyhow::anyhow!("Failed to get packages for group {}: {}", group_name, e)
            })?;

        if !output.status.success() {
            return Err(anyhow::anyhow!(
                "Failed to get packages for group {}",
                group_name
            ));
        }

        let stdout = String::from_utf8_lossy(&output.stdout);
        let mut packages = Vec::new();

        for line in stdout.lines() {
            let line = line.trim();
            if !line.is_empty() {
                // pacman -Sg output format is "group package_name"
                // We want just the package name part
                if let Some(space_pos) = line.find(' ') {
                    let package = line[space_pos + 1..].trim();
                    if !package.is_empty() {
                        packages.push(package.to_string());
                    }
                }
            }
        }

        // Cache the result
        {
            let mut cache_guard = cache.lock().unwrap();
            cache_guard.insert(group_name.to_string(), packages.clone());
        }

        Ok(packages)
    }
}

fn is_header_line(line: &str) -> bool {
    line.contains('/')
        && line.contains(' ')
        && !line.starts_with(' ')
        && !line.starts_with('[')
        && line.split_whitespace().next().unwrap_or("").contains('/')
}

fn parse_repo_name(repo_name: &str) -> Result<(&str, &str)> {
    if let Some(slash_pos) = repo_name.find('/') {
        let repo = &repo_name[..slash_pos];
        let name = &repo_name[slash_pos + 1..];
        Ok((repo, name))
    } else {
        Err(anyhow::anyhow!("Invalid repo/name format: {}", repo_name))
    }
}

fn parse_header_line(line: &str) -> Result<SearchResult> {
    let parts: Vec<&str> = line.split_whitespace().collect();
    if parts.is_empty() {
        return Err(anyhow::anyhow!("Empty header line"));
    }
    let repo_name_part = parts[0];
    let (repo, name) = parse_repo_name(repo_name_part)?;
    let version = parts
        .get(1)
        .ok_or_else(|| anyhow::anyhow!("Missing version in header line"))?;
    let installed = line.contains("[installed]");
    Ok(SearchResult {
        name: name.to_string(),
        ver: version.to_string(),
        source: if repo == "aur" {
            PackageSource::Aur
        } else {
            PackageSource::Repo
        },
        repo: repo.to_string(),
        description: String::new(),
        installed,
    })
}

fn parse_paru_search_output(output: &str) -> Result<Vec<SearchResult>> {
    let mut results = Vec::new();
    let mut current_result: Option<SearchResult> = None;
    for line in output.lines() {
        let original_line = line;
        let trimmed_line = line.trim();
        if trimmed_line.is_empty() {
            continue;
        }
        if is_header_line(trimmed_line) {
            if let Some(result) = current_result.take() {
                results.push(result);
            }
            current_result = Some(parse_header_line(trimmed_line)?);
        } else if original_line.starts_with("    ")
            && let Some(ref mut result) = current_result
        {
            let desc_part = trimmed_line;
            if result.description.is_empty() {
                result.description = desc_part.to_string();
            } else {
                result.description.push(' ');
                result.description.push_str(desc_part);
            }
        }
    }
    if let Some(result) = current_result {
        results.push(result);
    }
    Ok(results)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_paru_search_output() {
        let sample_output = r#"aur/jet-bin 0.7.27-1 [+5 ~0.00]
    CLI to transform between JSON, EDN and Transit, powered with a minimal query language.
aur/clang-opencl-headers-minimal-git 21.0.0_r537041.f2e62cfca5e5-1 [+5 ~0.00]
    clang headers & include files for OpenCL, trunk version
extra/texlive-latexextra 2025.2-2 [29.63 MiB 95.69 MiB] (texlive)
    TeX Live - LaTeX additional packages
extra/nim 2.0.8-1 [13.08 MiB 58.55 MiB]
    Imperative, multi-paradigm, compiled programming language"#;

        let results = parse_paru_search_output(sample_output).unwrap();
        assert_eq!(results.len(), 4);
        assert_eq!(results[0].name, "jet-bin");
        assert_eq!(results[0].repo, "aur");
        assert_eq!(results[0].source, PackageSource::Aur);
        assert_eq!(results[2].name, "texlive-latexextra");
        assert_eq!(results[2].repo, "extra");
        assert_eq!(results[2].source, PackageSource::Repo);
    }

    #[test]
    fn test_parse_repo_name() {
        assert_eq!(
            parse_repo_name("aur/package-name").unwrap(),
            ("aur", "package-name")
        );
        assert_eq!(parse_repo_name("extra/bash").unwrap(), ("extra", "bash"));
        assert!(parse_repo_name("invalid-format").is_err());
    }

    #[test]
    fn test_is_header_line() {
        assert!(is_header_line("aur/jet-bin 0.7.27-1 [+5 ~0.00]"));
        assert!(is_header_line(
            "extra/texlive-latexextra 2025.2-2 [29.63 MiB 95.69 MiB] (texlive)"
        ));
        assert!(!is_header_line("    Description line"));
        assert!(!is_header_line("[some other format]"));
    }
}
