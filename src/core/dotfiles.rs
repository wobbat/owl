//! Dotfile synchronization functionality
//!
//! This module handles the synchronization of dotfiles from the dotfiles directory
//! to their target locations in the user's home directory.

use anyhow::{Result, anyhow};
use sha2::{Digest, Sha256};
use std::fs;
use std::path::{Path, PathBuf};

/// Represents a dotfile mapping from source to destination
#[derive(Debug, Clone)]
pub struct DotfileMapping {
    pub source: String,
    pub destination: String,
}

/// Status of a dotfile operation
#[derive(Debug, Clone, PartialEq)]
pub enum DotfileStatus {
    Create,
    Update,
    UpToDate,
}

/// Represents a dotfile operation to be performed
#[derive(Debug, Clone)]
pub struct DotfileAction {
    pub mapping: DotfileMapping,
    pub status: DotfileStatus,
}

fn owl_dotfiles_dir() -> Result<PathBuf> {
    let home = std::env::var("HOME").map_err(|_| anyhow!("HOME environment variable not set"))?;
    Ok(Path::new(&home)
        .join(crate::internal::constants::OWL_DIR)
        .join(crate::internal::constants::DOTFILES_DIR))
}

fn expand_tilde(path: &str) -> String {
    if let Some(rest) = path.strip_prefix("~/") {
        if let Ok(home) = std::env::var("HOME") {
            return Path::new(&home).join(rest).to_string_lossy().into_owned();
        }
    } else if path == "~"
        && let Ok(home) = std::env::var("HOME")
    {
        return home;
    }
    path.to_string()
}

fn collect_files_recursively(root: &Path, rels: &mut Vec<PathBuf>, base: &Path) -> Result<()> {
    for entry in
        fs::read_dir(root).map_err(|e| anyhow!("Failed to read dir {}: {}", root.display(), e))?
    {
        let entry =
            entry.map_err(|e| anyhow!("Failed to read entry in {}: {}", root.display(), e))?;
        let ty = entry
            .file_type()
            .map_err(|e| anyhow!("Failed to stat {}: {}", entry.path().display(), e))?;
        let path = entry.path();
        if ty.is_dir() {
            collect_files_recursively(&path, rels, base)?;
        } else if ty.is_file() {
            let rel = path.strip_prefix(base).unwrap_or(&path).to_path_buf();
            rels.push(rel);
        }
    }
    Ok(())
}

fn dirs_in_sync(src: &Path, dst: &Path) -> Result<bool> {
    if !dst.exists() || !dst.is_dir() {
        return Ok(false);
    }

    // Collect all files in source
    let mut src_files: Vec<PathBuf> = Vec::new();
    collect_files_recursively(src, &mut src_files, src)?;

    // Collect all files in destination
    let mut dst_files: Vec<PathBuf> = Vec::new();
    collect_files_recursively(dst, &mut dst_files, dst)?;

    // Check if file counts match
    if src_files.len() != dst_files.len() {
        return Ok(false);
    }

    // Check if all source files exist in destination with same content
    for rel in &src_files {
        let s = src.join(rel);
        let d = dst.join(rel);
        if !d.exists() || !d.is_file() {
            return Ok(false);
        }
        if sha256_file(&s)? != sha256_file(&d)? {
            return Ok(false);
        }
    }

    // Check if destination has no extra files (should be covered by count check, but being explicit)
    for rel in &dst_files {
        if !src_files.contains(rel) {
            return Ok(false);
        }
    }

    Ok(true)
}

fn sha256_file(path: &Path) -> Result<String> {
    let data = fs::read(path).map_err(|e| anyhow!("Failed to read {}: {}", path.display(), e))?;
    let mut hasher = Sha256::new();
    hasher.update(&data);
    Ok(format!("{:x}", hasher.finalize()))
}

fn ensure_parent_dir(dest: &Path) -> Result<()> {
    if let Some(parent) = dest.parent() {
        fs::create_dir_all(parent)
            .map_err(|e| anyhow!("Failed to create directory {}: {}", parent.display(), e))?;
    }
    Ok(())
}

fn copy_dir_all(src: &Path, dst: &Path) -> Result<()> {
    if src == dst {
        return Ok(());
    }
    // Create destination directory
    fs::create_dir_all(dst)
        .map_err(|e| anyhow!("Failed to create directory {}: {}", dst.display(), e))?;

    for entry in
        fs::read_dir(src).map_err(|e| anyhow!("Failed to read dir {}: {}", src.display(), e))?
    {
        let entry =
            entry.map_err(|e| anyhow!("Failed to read entry in {}: {}", src.display(), e))?;
        let ty = entry
            .file_type()
            .map_err(|e| anyhow!("Failed to stat {}: {}", entry.path().display(), e))?;
        let src_path = entry.path();
        let dst_path = dst.join(entry.file_name());
        if ty.is_dir() {
            copy_dir_all(&src_path, &dst_path)?;
        } else if ty.is_file() {
            let data = fs::read(&src_path)
                .map_err(|e| anyhow!("Failed to read {}: {}", src_path.display(), e))?;
            fs::write(&dst_path, &data)
                .map_err(|e| anyhow!("Failed to write {}: {}", dst_path.display(), e))?;
        }
    }
    Ok(())
}

/// Build dotfile mappings from config
pub fn get_dotfile_mappings(config: &crate::core::config::Config) -> Vec<DotfileMapping> {
    let mut mappings = Vec::new();
    for pkg in config.packages.values() {
        for cfg in &pkg.config {
            // formats: "a -> b" or "b" (same source name)
            if let Some((source, dest)) = cfg.split_once(" -> ") {
                mappings.push(DotfileMapping {
                    source: source.trim().to_string(),
                    destination: dest.trim().to_string(),
                });
            } else {
                mappings.push(DotfileMapping {
                    source: cfg.clone(),
                    destination: cfg.clone(),
                });
            }
        }
    }
    mappings
}

/// Return true if any mapping requires action
pub fn has_actionable_dotfiles(mappings: &[DotfileMapping]) -> Result<bool> {
    for m in mappings {
        let src = owl_dotfiles_dir()?.join(&m.source);
        let dst = expand_tilde(&m.destination);
        let dst_path = Path::new(&dst);
        if !src.exists() {
            continue;
        }
        if src.is_dir() {
            if !dirs_in_sync(&src, dst_path)? {
                return Ok(true);
            }
        } else {
            if !dst_path.exists() {
                return Ok(true);
            }
            if sha256_file(&src)? != sha256_file(dst_path)? {
                return Ok(true);
            }
        }
    }
    Ok(false)
}

/// Analyze and apply dotfiles
pub fn apply_dotfiles(mappings: &[DotfileMapping], dry_run: bool) -> Result<Vec<DotfileAction>> {
    let mut actions = Vec::new();
    for m in mappings {
        let src = owl_dotfiles_dir()?.join(&m.source);
        let dst = PathBuf::from(expand_tilde(&m.destination));
        let status = if src.is_dir() {
            if !dst.exists() {
                DotfileStatus::Create
            } else if dirs_in_sync(&src, &dst)? {
                DotfileStatus::UpToDate
            } else {
                DotfileStatus::Update
            }
        } else if !dst.exists() {
            DotfileStatus::Create
        } else if sha256_file(&src)? == sha256_file(&dst)? {
            DotfileStatus::UpToDate
        } else {
            DotfileStatus::Update
        };

        if !dry_run {
            if src.is_dir() {
                // Remove destination directory if it exists, then copy entire source
                if dst.exists() {
                    fs::remove_dir_all(&dst).map_err(|e| {
                        anyhow!("Failed to remove directory {}: {}", dst.display(), e)
                    })?;
                }
                copy_dir_all(&src, &dst)?;
            } else {
                // Remove destination file if it exists, then copy source file
                if dst.exists() {
                    fs::remove_file(&dst)
                        .map_err(|e| anyhow!("Failed to remove file {}: {}", dst.display(), e))?;
                }
                ensure_parent_dir(&dst)?;
                let data = fs::read(&src)
                    .map_err(|e| anyhow!("Failed to read {}: {}", src.display(), e))?;
                fs::write(&dst, &data)
                    .map_err(|e| anyhow!("Failed to write {}: {}", dst.display(), e))?;
            }
        }

        actions.push(DotfileAction {
            mapping: m.clone(),
            status,
        });
    }
    Ok(actions)
}

pub fn print_actions(actions: &[DotfileAction], dry_run: bool) {
    let mut _created = 0usize;
    let mut _updated = 0usize;
    let mut up_to_date = 0usize;
    for a in actions {
        match a.status {
            DotfileStatus::Create => {
                _created += 1;
                println!(
                    "  {} create {} -> {}",
                    crate::internal::color::green("➔"),
                    a.mapping.source,
                    a.mapping.destination
                );
            }
            DotfileStatus::Update => {
                _updated += 1;
                println!(
                    "  {} update {} -> {}",
                    crate::internal::color::green("➔"),
                    a.mapping.source,
                    a.mapping.destination
                );
            }
            DotfileStatus::UpToDate => {
                up_to_date += 1;
            }
        }
    }
    if !dry_run {
        println!(
            "  {} Up to date: {} dotfiles",
            crate::internal::color::green("➔"),
            up_to_date
        );
    }
}
