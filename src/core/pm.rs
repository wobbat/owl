use anyhow::{Result, anyhow};
use std::collections::HashMap;
use std::collections::HashSet;
use std::io::Write;
use std::process::{Command, ExitStatus, Stdio};
use std::sync::{Mutex, OnceLock};
use std::thread;
use std::time::Duration;

pub use super::search::{PackageSource, SearchResult};

fn retry_command<F, T>(mut operation: F, max_retries: usize) -> Result<T>
where
    F: FnMut() -> Result<T>,
{
    let mut last_error = None;

    for attempt in 0..=max_retries {
        match operation() {
            Ok(result) => return Ok(result),
            Err(err) => {
                let err_msg = err.to_string();
                let should_retry = err_msg.contains("Connection reset by peer")
                    || err_msg.contains("error sending request")
                    || err_msg.contains("error trying to connect")
                    || err_msg.contains("os error 104");

                if !should_retry || attempt == max_retries {
                    return Err(err);
                }
                last_error = Some(err);

                let delay = Duration::from_secs(1 << attempt);
                print!("\r\x1b[2K");
                std::io::stdout().flush().ok();
                print!(
                    "Retrying due to network errors... ({}/{})",
                    attempt + 1,
                    max_retries + 1
                );
                std::io::stdout().flush().ok();
                thread::sleep(delay);
                print!("\r\x1b[2K");
                std::io::stdout().flush().ok();
            }
        }
    }

    Err(last_error.unwrap_or_else(|| anyhow!("Unknown error")))
}

pub struct ParuPacman;

impl ParuPacman {
    pub fn new() -> Self {
        Self
    }

    pub fn list_installed(&self) -> Result<HashSet<String>> {
        let output = Command::new("pacman")
            .arg("-Qq")
            .output()
            .map_err(|e| anyhow!("Failed to get installed packages: {}", e))?;

        if !output.status.success() {
            return Err(anyhow!(
                "Package manager failed: {}",
                String::from_utf8_lossy(&output.stderr)
            ));
        }

        Ok(String::from_utf8_lossy(&output.stdout)
            .lines()
            .map(str::trim)
            .filter(|name| !name.is_empty())
            .map(ToOwned::to_owned)
            .collect())
    }

    pub fn batch_repo_available(&self, packages: &[String]) -> Result<HashSet<String>> {
        if packages.is_empty() {
            return Ok(HashSet::new());
        }

        let output = Command::new("pacman")
            .arg("-Si")
            .args(packages)
            .output()
            .map_err(|e| anyhow!("Failed to check package info: {}", e))?;

        Ok(String::from_utf8_lossy(&output.stdout)
            .lines()
            .filter_map(|line| line.strip_prefix("Name"))
            .filter_map(|rest| rest.split_once(':'))
            .map(|(_, value)| value.trim())
            .filter(|value| !value.is_empty())
            .map(ToOwned::to_owned)
            .collect())
    }

    pub fn upgrade_count(&self) -> Result<usize> {
        let output = Command::new("pacman")
            .args(["-Qu", "-q"])
            .output()
            .map_err(|e| anyhow!("Failed to run pacman -Qu: {}", e))?;

        if output.status.success() {
            return Ok(String::from_utf8_lossy(&output.stdout).lines().count());
        }

        let stderr = String::from_utf8_lossy(&output.stderr);
        if output.status.code() == Some(1) && stderr.trim().is_empty() {
            Ok(0)
        } else {
            Err(anyhow!("pacman -Qu failed: {}", stderr))
        }
    }

    pub fn get_aur_updates(&self) -> Result<Vec<String>> {
        retry_command(
            || {
                let aur_helper = require_aur_helper()?;
                let output = Command::new(aur_helper)
                    .args(["-Qua", "-q"])
                    .output()
                    .map_err(|e| anyhow!("Failed to check AUR updates: {}", e))?;

                if output.status.success() {
                    return Ok(String::from_utf8_lossy(&output.stdout)
                        .lines()
                        .map(str::trim)
                        .filter(|line| !line.is_empty())
                        .map(|line| line.split_whitespace().next().unwrap_or(line).to_string())
                        .collect());
                }

                let stderr = String::from_utf8_lossy(&output.stderr);
                if output.status.code() == Some(1) && stderr.trim().is_empty() {
                    Ok(Vec::new())
                } else {
                    Err(anyhow!("AUR update check failed: {}", stderr))
                }
            },
            3,
        )
    }

    pub fn install_repo(&self, packages: &[String]) -> Result<()> {
        self.install_repo_with_mode(packages, true)
    }

    pub fn install_repo_with_mode(&self, packages: &[String], non_interactive: bool) -> Result<()> {
        if packages.is_empty() {
            return Ok(());
        }

        let mut args = vec!["-S".to_string()];
        if non_interactive {
            args.push("--noconfirm".to_string());
        }
        args.extend(packages.iter().cloned());

        let outcome = run_command(
            "pacman",
            &args,
            mode_from_bool(non_interactive),
            "Installing repository packages",
            CaptureMode::Spinner,
        )?;
        ensure_success(outcome.status, "Repository install failed")
    }

    pub fn install_aur(&self, packages: &[String]) -> Result<()> {
        self.install_aur_with_mode(packages, true)
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
                "Installing AUR packages",
                3,
            )?
        } else {
            let arg_refs: Vec<&str> = args.iter().map(String::as_str).collect();
            crate::internal::util::execute_command_interactive(
                aur_helper,
                &arg_refs,
                "Installing AUR packages",
            )?
        };

        ensure_success(status, "AUR install failed")
    }

    pub fn update_repo(&self) -> Result<()> {
        self.update_repo_with_mode(true)
    }

    pub fn update_repo_with_mode(&self, non_interactive: bool) -> Result<()> {
        let mut args = vec!["-Syu".to_string()];
        if non_interactive {
            args.push("--noconfirm".to_string());
        }

        let outcome = run_command(
            "pacman",
            &args,
            mode_from_bool(non_interactive),
            "Updating official repository packages (syncing databases and upgrading packages)",
            CaptureMode::CaptureStderr,
        )?;

        if outcome.status.success() {
            println!(
                "  {} Official repos synced",
                crate::internal::color::green("⸎")
            );
            Ok(())
        } else {
            Err(anyhow!(
                "Repository update failed (exit code: {:?})",
                outcome.status.code()
            ))
        }
    }

    pub fn update_aur(&self, packages: &[String]) -> Result<()> {
        self.update_aur_with_mode(packages, true)
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
            let result = retry_command(
                || {
                    let (status, stderr) = crate::internal::util::execute_command_with_stderr_capture(
                        aur_helper,
                        &arg_refs,
                        "Updating AUR packages",
                    )?;

                    if status.success() {
                        Ok(())
                    } else {
                        Err(anyhow!("AUR package update failed: {}", stderr.trim()))
                    }
                },
                3,
            );

            match result {
                Ok(()) => {
                    println!(
                        "  {} AUR package updates completed",
                        crate::internal::color::green("⸎")
                    );
                    Ok(())
                }
                Err(err) => {
                    let detail = err.to_string();
                    if let Some((_, stderr)) = detail.split_once(": ") {
                        if !stderr.trim().is_empty() {
                            stderr
                                .lines()
                                .rev()
                                .take(30)
                                .for_each(|line| eprintln!("  {}", line));
                        }
                    }
                    Err(anyhow!("AUR package update failed"))
                }
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
                Err(anyhow!("AUR package update failed"))
            }
        }
    }

    pub fn remove_packages(&self, packages: &[String], quiet: bool) -> Result<()> {
        if packages.is_empty() {
            return Ok(());
        }

        let mut cmd = Command::new("pacman");
        cmd.arg("-Rns");
        if quiet {
            cmd.arg("--noconfirm");
        }
        cmd.args(packages);

        let status = cmd
            .status()
            .map_err(|e| anyhow!("Failed to remove packages: {}", e))?;

        if status.success() {
            println!(
                "  {} Removed {} package(s)",
                crate::internal::color::green("✓"),
                packages.len()
            );
            Ok(())
        } else {
            Err(anyhow!("Package removal failed"))
        }
    }

    pub fn search_packages(&self, terms: &[String]) -> Result<Vec<SearchResult>> {
        super::search::search_packages(terms)
    }

    pub fn is_package_group(&self, package_name: &str) -> Result<bool> {
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
            .map_err(|e| anyhow!("Failed to check if {} is a group: {}", package_name, e))?;

        let is_group =
            output.status.success() && !String::from_utf8_lossy(&output.stdout).trim().is_empty();

        {
            let mut cache_guard = cache.lock().unwrap();
            cache_guard.insert(package_name.to_string(), is_group);
        }

        Ok(is_group)
    }

    pub fn get_group_packages(&self, group_name: &str) -> Result<Vec<String>> {
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
            .map_err(|e| anyhow!("Failed to get packages for group {}: {}", group_name, e))?;

        if !output.status.success() {
            return Err(anyhow!("Failed to get packages for group {}", group_name));
        }

        let packages: Vec<String> = String::from_utf8_lossy(&output.stdout)
            .lines()
            .map(str::trim)
            .filter(|line| !line.is_empty())
            .filter_map(|line| line.split_once(' '))
            .map(|(_, package)| package.trim())
            .filter(|package| !package.is_empty())
            .map(ToOwned::to_owned)
            .collect();

        {
            let mut cache_guard = cache.lock().unwrap();
            cache_guard.insert(group_name.to_string(), packages.clone());
        }

        Ok(packages)
    }
}

static GROUP_CACHE: OnceLock<Mutex<HashMap<String, bool>>> = OnceLock::new();
static GROUP_PACKAGES_CACHE: OnceLock<Mutex<HashMap<String, Vec<String>>>> = OnceLock::new();
static AUR_HELPER: OnceLock<Option<String>> = OnceLock::new();

#[derive(Copy, Clone)]
enum CommandMode {
    Managed,
    Interactive,
}

#[derive(Copy, Clone)]
enum CaptureMode {
    Spinner,
    CaptureStderr,
}

struct CommandOutcome {
    status: ExitStatus,
    _stderr: Option<String>,
}

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

fn require_aur_helper() -> Result<&'static str> {
    aur_helper_command().ok_or_else(|| {
        anyhow!("No AUR helper found. Install either 'paru' or 'yay' to manage AUR packages.")
    })
}

fn mode_from_bool(non_interactive: bool) -> CommandMode {
    if non_interactive {
        CommandMode::Managed
    } else {
        CommandMode::Interactive
    }
}

fn run_command(
    command: &str,
    args: &[String],
    mode: CommandMode,
    message: &str,
    capture: CaptureMode,
) -> Result<CommandOutcome> {
    let arg_refs: Vec<&str> = args.iter().map(String::as_str).collect();

    match mode {
        CommandMode::Interactive => Ok(CommandOutcome {
            status: crate::internal::util::execute_command_interactive(
                command, &arg_refs, message,
            )?,
            _stderr: None,
        }),
        CommandMode::Managed => match capture {
            CaptureMode::Spinner => Ok(CommandOutcome {
                status: crate::internal::util::execute_command_with_spinner(
                    command, &arg_refs, message,
                )?,
                _stderr: None,
            }),
            CaptureMode::CaptureStderr => {
                let (status, stderr) = crate::internal::util::execute_command_with_stderr_capture(
                    command, &arg_refs, message,
                )?;
                Ok(CommandOutcome {
                    status,
                    _stderr: Some(stderr),
                })
            }
        },
    }
}

fn ensure_success(status: ExitStatus, failure_message: &str) -> Result<()> {
    if status.success() {
        Ok(())
    } else {
        Err(anyhow!("{}", failure_message))
    }
}
