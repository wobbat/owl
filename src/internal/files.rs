//! File operations utilities

use anyhow::{Result, anyhow};
use std::env;
use std::path::{Path, PathBuf};
use std::process::Command;

use crate::internal::constants;

/// Get the owl root directory (~/.owl)
fn owl_dir() -> Result<PathBuf> {
    let home = env::var("HOME").map_err(|_| anyhow!("HOME environment variable not set"))?;
    Ok(PathBuf::from(home).join(constants::OWL_DIR))
}

/// Scan a directory for .owl files and add them to the files vector
pub fn scan_directory_for_owl_files(directory: &Path, files: &mut Vec<String>) {
    if let Ok(entries) = std::fs::read_dir(directory) {
        for entry in entries.flatten() {
            let path = entry.path();
            if path.extension().is_some_and(|ext| ext == "owl")
                && let Some(path_str) = path.to_str()
            {
                files.push(path_str.to_string());
            }
        }
    }
}

/// Open a file in the user's preferred editor
pub fn open_editor(path: &str) -> Result<()> {
    let editor = env::var("EDITOR").unwrap_or_else(|_| constants::DEFAULT_EDITOR.to_string());

    Command::new(&editor)
        .arg(path)
        .status()
        .map_err(|e| anyhow!("Failed to open editor '{}': {}", editor, e))
        .and_then(|status| {
            if status.success() {
                Ok(())
            } else {
                Err(anyhow!("Editor '{}' exited with error", editor))
            }
        })
}

/// Find a config file in the standard locations
pub fn find_config_file(arg: &str) -> Result<String> {
    let base_dir = owl_dir()?;
    let arg_with_ext = format!("{}{}", arg, constants::OWL_EXT);

    let search_paths = [
        base_dir.join(&arg_with_ext),
        base_dir.join(arg),
        base_dir.join(constants::HOSTS_DIR).join(&arg_with_ext),
        base_dir.join(constants::HOSTS_DIR).join(arg),
        base_dir.join(constants::GROUPS_DIR).join(&arg_with_ext),
        base_dir.join(constants::GROUPS_DIR).join(arg),
    ];

    for path in &search_paths {
        if path.exists() {
            return path
                .to_str()
                .map(|s| s.to_string())
                .ok_or_else(|| anyhow!("Invalid path encoding"));
        }
    }

    Err(anyhow!("config file not found"))
}

/// Get the path for a dotfile
pub fn get_dotfile_path(filename: &str) -> Result<String> {
    let path = owl_dir()?.join(constants::DOTFILES_DIR).join(filename);
    path.to_str()
        .map(|s| s.to_string())
        .ok_or_else(|| anyhow!("Invalid path encoding"))
}

/// Get all config files from the owl directory (main, hosts, and groups)
pub fn get_all_config_files() -> Result<Vec<String>> {
    let owl = owl_dir()?;
    let mut files = Vec::new();

    // Check main config
    let main_config = owl.join(constants::MAIN_CONFIG_FILE);
    if main_config.exists()
        && let Some(path_str) = main_config.to_str()
    {
        files.push(path_str.to_string());
    }

    // Scan hosts directory
    scan_directory_for_owl_files(&owl.join(constants::HOSTS_DIR), &mut files);

    // Scan groups directory
    scan_directory_for_owl_files(&owl.join(constants::GROUPS_DIR), &mut files);

    Ok(files)
}
