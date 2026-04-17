use anyhow::{Result, anyhow};
use serde::Deserialize;
use std::collections::HashSet;
use std::fs::{self, File};
use std::io::Write;
use std::path::Path;
use std::process::Command;
use std::thread;
use std::time::Duration;
use tar::Archive;

const PACMAN_SYNC_DIR: &str = "/var/lib/pacman/sync";
const AUR_RPC_URL: &str = "https://aur.archlinux.org/rpc/v5/search";

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

#[derive(Debug, Clone)]
struct RepoPackageRecord {
    name: String,
    version: String,
    repo: String,
    description: String,
}

#[derive(Debug, Deserialize)]
struct AurSearchResponse {
    #[serde(default, rename = "results")]
    results: Vec<AurPackage>,
}

#[derive(Debug, Deserialize)]
struct AurPackage {
    #[serde(rename = "Name")]
    name: String,
    #[serde(rename = "Version")]
    version: String,
    #[serde(rename = "Description")]
    description: Option<String>,
}

pub fn search_packages(terms: &[String]) -> Result<Vec<SearchResult>> {
    if terms.is_empty() {
        return Ok(Vec::new());
    }

    let normalized_terms: Vec<String> = terms
        .iter()
        .map(|term| term.trim().to_ascii_lowercase())
        .filter(|term| !term.is_empty())
        .collect();

    if normalized_terms.is_empty() {
        return Ok(Vec::new());
    }

    let installed = installed_packages()?;
    let mut results = search_repo_packages(&normalized_terms, &installed)?;
    results.extend(search_aur_packages(&normalized_terms, &installed)?);
    sort_results(&mut results, &normalized_terms);
    Ok(results)
}

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
                    "Retrying package search due to network errors... ({}/{})",
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

fn installed_packages() -> Result<HashSet<String>> {
    let output = Command::new("pacman")
        .arg("-Qq")
        .output()
        .map_err(|e| anyhow!("Failed to get installed packages: {}", e))?;

    if !output.status.success() {
        return Err(anyhow!(
            "Failed to get installed packages: {}",
            String::from_utf8_lossy(&output.stderr)
        ));
    }

    Ok(String::from_utf8_lossy(&output.stdout)
        .lines()
        .map(str::trim)
        .filter(|line| !line.is_empty())
        .map(ToOwned::to_owned)
        .collect())
}

fn search_repo_packages(
    terms: &[String],
    installed: &HashSet<String>,
) -> Result<Vec<SearchResult>> {
    let mut results = Vec::new();

    for entry in fs::read_dir(PACMAN_SYNC_DIR)
        .map_err(|e| anyhow!("Failed to read pacman sync database directory: {}", e))?
    {
        let entry = entry?;
        let path = entry.path();

        if path.extension().and_then(|ext| ext.to_str()) != Some("db") {
            continue;
        }

        let repo = match path.file_stem().and_then(|name| name.to_str()) {
            Some(repo) if !repo.is_empty() => repo.to_string(),
            _ => continue,
        };

        results.extend(search_repo_archive(&path, &repo, terms, installed)?);
    }

    Ok(results)
}

fn search_repo_archive(
    path: &Path,
    repo: &str,
    terms: &[String],
    installed: &HashSet<String>,
) -> Result<Vec<SearchResult>> {
    let file = File::open(path).map_err(|e| {
        anyhow!(
            "Failed to open pacman sync database {}: {}",
            path.display(),
            e
        )
    })?;
    let mut archive = Archive::new(file);
    let mut results = Vec::new();

    for entry in archive.entries()? {
        let mut entry = entry?;
        let path = entry.path()?;
        let path_str = path.to_string_lossy();

        if !path_str.ends_with("/desc") {
            continue;
        }

        let mut desc = String::new();
        std::io::Read::read_to_string(&mut entry, &mut desc)?;

        let Some(record) = parse_sync_desc(&desc, repo) else {
            continue;
        };

        if !matches_terms(&record.name, &record.description, terms) {
            continue;
        }

        results.push(SearchResult {
            name: record.name.clone(),
            ver: record.version,
            source: PackageSource::Repo,
            repo: record.repo,
            description: record.description,
            installed: installed.contains(&record.name),
        });
    }

    Ok(results)
}

fn parse_sync_desc(desc: &str, repo: &str) -> Option<RepoPackageRecord> {
    let mut current_field = None::<&str>;
    let mut name = None;
    let mut version = None;
    let mut description = String::new();

    for line in desc.lines() {
        if line.starts_with('%') && line.ends_with('%') && line.len() > 2 {
            current_field = Some(&line[1..line.len() - 1]);
            continue;
        }

        match current_field {
            Some("NAME") if !line.is_empty() => {
                name = Some(line.to_string());
            }
            Some("VERSION") if !line.is_empty() => {
                version = Some(line.to_string());
            }
            Some("DESC") if !line.is_empty() => {
                if !description.is_empty() {
                    description.push(' ');
                }
                description.push_str(line);
            }
            _ => {}
        }
    }

    Some(RepoPackageRecord {
        name: name?,
        version: version?,
        repo: repo.to_string(),
        description,
    })
}

fn search_aur_packages(terms: &[String], installed: &HashSet<String>) -> Result<Vec<SearchResult>> {
    let seed = match aur_search_seed(terms) {
        Some(seed) => seed,
        None => return Ok(Vec::new()),
    };

    retry_command(
        || {
            let response = ureq::get(AUR_RPC_URL)
                .query("arg", &seed)
                .query("by", "name-desc")
                .call()
                .map_err(|e| anyhow!("AUR search failed: {}", e))?;

            let payload: AurSearchResponse = response
                .into_json()
                .map_err(|e| anyhow!("Failed to parse AUR search response: {}", e))?;

            Ok(payload
                .results
                .into_iter()
                .filter(|pkg| {
                    matches_terms(
                        &pkg.name,
                        pkg.description.as_deref().unwrap_or_default(),
                        terms,
                    )
                })
                .map(|pkg| SearchResult {
                    installed: installed.contains(&pkg.name),
                    name: pkg.name,
                    ver: pkg.version,
                    source: PackageSource::Aur,
                    repo: "aur".to_string(),
                    description: pkg.description.unwrap_or_default(),
                })
                .collect())
        },
        3,
    )
}

fn aur_search_seed(terms: &[String]) -> Option<String> {
    let seed = terms
        .iter()
        .filter(|term| term.len() >= 2)
        .min_by_key(|term| term.len())?;
    Some(seed.to_string())
}

fn matches_terms(name: &str, description: &str, terms: &[String]) -> bool {
    let haystack = format!(
        "{} {}",
        name.to_ascii_lowercase(),
        description.to_ascii_lowercase()
    );
    terms.iter().all(|term| haystack.contains(term))
}

fn sort_results(results: &mut [SearchResult], terms: &[String]) {
    let query = terms.join(" ");

    results.sort_by(|left, right| {
        result_sort_key(left, &query)
            .cmp(&result_sort_key(right, &query))
            .then_with(|| left.name.cmp(&right.name))
    });
}

fn result_sort_key(result: &SearchResult, query: &str) -> (u8, u8, bool) {
    let name = result.name.to_ascii_lowercase();
    let source_rank = match result.source {
        PackageSource::Repo => 0,
        PackageSource::Aur => 1,
    };

    let match_rank = if name == query {
        0
    } else if name.starts_with(query) {
        1
    } else if name.contains(query) {
        2
    } else {
        3
    };

    (source_rank, match_rank, !result.installed)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_sync_desc() {
        let desc = r#"%NAME%
bash

%VERSION%
5.3.3-1

%DESC%
The GNU Bourne Again shell
"#;

        let package = parse_sync_desc(desc, "core").unwrap();
        assert_eq!(package.name, "bash");
        assert_eq!(package.version, "5.3.3-1");
        assert_eq!(package.repo, "core");
        assert_eq!(package.description, "The GNU Bourne Again shell");
    }

    #[test]
    fn test_matches_terms() {
        assert!(matches_terms(
            "bash-completion",
            "Programmable completion for Bash",
            &["bash".to_string(), "completion".to_string()]
        ));
        assert!(!matches_terms(
            "bash",
            "The GNU shell",
            &["bash".to_string(), "fish".to_string()]
        ));
    }
}
