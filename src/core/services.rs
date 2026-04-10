use anyhow::{Result, anyhow};
use std::process::Command;

/// Result of service configuration operations
#[derive(Debug)]
pub struct ServiceResult {
    pub changed: bool,
    pub enabled_services: Vec<String>,
    pub started_services: Vec<String>,
    pub failed_services: Vec<String>,
}

/// Ensure all specified services are configured (enabled and started)
fn check_enabled(service: &str) -> Result<bool> {
    let status = Command::new("sudo")
        .arg("systemctl")
        .arg("is-enabled")
        .arg("--quiet")
        .arg(service)
        .status()
        .map_err(|e| anyhow!("Failed to run systemctl is-enabled for {}: {}", service, e))?;
    Ok(status.success())
}

fn check_active(service: &str) -> Result<bool> {
    let status = Command::new("sudo")
        .arg("systemctl")
        .arg("is-active")
        .arg("--quiet")
        .arg(service)
        .status()
        .map_err(|e| anyhow!("Failed to run systemctl is-active for {}: {}", service, e))?;
    Ok(status.success())
}

pub fn ensure_services_configured(services: &[String]) -> Result<ServiceResult> {
    if services.is_empty() {
        return Ok(ServiceResult {
            changed: false,
            enabled_services: Vec::new(),
            started_services: Vec::new(),
            failed_services: Vec::new(),
        });
    }

    let mut result = ServiceResult {
        changed: false,
        enabled_services: Vec::new(),
        started_services: Vec::new(),
        failed_services: Vec::new(),
    };
    for service in services {
        // Enable only if not enabled
        match check_enabled(service) {
            Ok(true) => {}
            Ok(false) => {
                match Command::new("sudo")
                    .arg("systemctl")
                    .arg("enable")
                    .arg(service)
                    .status()
                {
                    Ok(status) if status.success() => {
                        result.changed = true;
                        result.enabled_services.push(service.clone());
                    }
                    Ok(_) | Err(_) => {
                        result.failed_services.push(service.clone());
                        eprintln!(
                            "{}",
                            crate::internal::color::red(&format!(
                                "Failed to enable service {}",
                                service
                            ))
                        );
                        continue;
                    }
                }
            }
            Err(e) => {
                result.failed_services.push(service.clone());
                eprintln!(
                    "{}",
                    crate::internal::color::red(&format!(
                        "Service {} status check failed (enabled): {}",
                        service, e
                    ))
                );
                continue;
            }
        }

        // Start only if not running
        match check_active(service) {
            Ok(true) => {}
            Ok(false) => {
                match Command::new("sudo")
                    .arg("systemctl")
                    .arg("start")
                    .arg(service)
                    .status()
                {
                    Ok(status) if status.success() => {
                        result.changed = true;
                        result.started_services.push(service.clone());
                    }
                    Ok(_) | Err(_) => {
                        result.failed_services.push(service.clone());
                        eprintln!(
                            "{}",
                            crate::internal::color::red(&format!(
                                "Failed to start service {}",
                                service
                            ))
                        );
                    }
                }
            }
            Err(e) => {
                result.failed_services.push(service.clone());
                eprintln!(
                    "{}",
                    crate::internal::color::red(&format!(
                        "Service {} status check failed (active): {}",
                        service, e
                    ))
                );
            }
        }
    }
    Ok(result)
}

/// Get configured services from config
pub fn get_configured_services(config: &crate::core::config::Config) -> Vec<String> {
    let mut services = Vec::new();
    for pkg in config.packages.values() {
        if let Some(ref svc) = pkg.service {
            services.push(svc.clone());
        }
    }
    services.sort();
    services.dedup();
    services
}
