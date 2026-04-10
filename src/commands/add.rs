use crate::core::pm::{PackageSource, SearchResult};
use anyhow::anyhow;

/// Add items (packages) to configuration files
///
/// # Arguments
/// * `items` - List of package names to search for and add
/// * `search_mode` - Whether to search for packages first (always true now)
pub fn run(items: &[String], _search_mode: bool) {
    run_search_mode(items);
}

/// Search and select mode - add to config instead of installing
fn run_search_mode(terms: &[String]) {
    match crate::core::package::search_packages(terms) {
        Ok(results) => {
            if results.is_empty() {
                println!(
                    "{}",
                    crate::internal::color::yellow("No packages found matching the search terms")
                );
                return;
            }

            display_search_results(&results);
            let selection = prompt_package_selection(&results);

            match selection {
                Some(package_name) => {
                    if let Err(err) = add_package_to_config(&package_name) {
                        crate::error::exit_with_error(anyhow::anyhow!(err));
                    }
                }
                None => {
                    println!("{}", crate::internal::color::yellow("No package selected"));
                }
            }
        }
        Err(e) => {
            crate::error::exit_with_error(anyhow::anyhow!("Search failed: {}", e));
        }
    }
}

fn display_search_results(results: &[SearchResult]) {
    println!(
        "\n{} {} package(s):\n",
        crate::internal::color::bold("Found"),
        results.len()
    );

    for (i, result) in results.iter().enumerate() {
        let num_str = number_brackets(results.len() - 1 - i);
        let name = crate::internal::color::highlight(&result.name);
        let version = crate::internal::color::success(&result.ver);

        let tag = match result.source {
            PackageSource::Aur => crate::internal::color::warning(&format!("[{}]", result.repo)),
            PackageSource::Repo => {
                crate::internal::color::repository(&format!("[{}]", result.repo))
            }
        };

        let status = if result.installed {
            format!(" {}", crate::internal::color::success("installed"))
        } else {
            String::new()
        };

        let desc = if result.description.is_empty() {
            String::new()
        } else {
            format!(
                " - {}",
                crate::internal::color::description(&result.description)
            )
        };

        println!("{}{} {}{} {}{}", num_str, name, version, tag, status, desc);
    }
    println!();
}

/// Prompt user to select a package from search results
fn prompt_package_selection(results: &[SearchResult]) -> Option<String> {
    if results.is_empty() {
        return None;
    }

    loop {
        print!(
            "Select package (0-{}, or 'c' to cancel): ",
            results.len() - 1
        );
        std::io::Write::flush(&mut std::io::stdout()).ok()?;

        let mut input = String::new();
        std::io::stdin().read_line(&mut input).ok()?;
        let input = input.trim();

        if input == "c" || input == "cancel" {
            return None;
        }

        match input.parse::<usize>() {
            Ok(num) if num < results.len() => {
                let index = results.len() - 1 - num;
                return Some(results[index].name.clone());
            }
            _ => {
                println!(
                    "{}",
                    crate::internal::color::red("Invalid selection. Please try again.")
                );
            }
        }
    }
}

/// Format a number in brackets like [1], [2], etc.
fn number_brackets(num: usize) -> String {
    format!("[{num}]")
}

/// Add a package to the appropriate configuration file
fn add_package_to_config(package_name: &str) -> anyhow::Result<()> {
    use crate::internal::files::{add_package_to_file, get_main_config_path, AddPackageResult};

    let mut config_files = get_relevant_config_files()?;

    if config_files.is_empty() {
        // Use main config if no relevant files found
        let main_config = get_main_config_path()?;
        match add_package_to_file(package_name, &main_config)? {
            AddPackageResult::Added => {
                println!(
                    "{}",
                    crate::internal::color::success(&format!(
                        "Added '{}' to {}",
                        package_name, main_config
                    ))
                );
            }
            AddPackageResult::AlreadyPresent => {
                println!(
                    "{}",
                    crate::internal::color::yellow(&format!(
                        "Package '{}' already exists in {}",
                        package_name, main_config
                    ))
                );
            }
        }
        return Ok(());
    }

    if config_files.len() == 1 {
        let file_path = &config_files[0];
        match add_package_to_file(package_name, file_path)? {
            AddPackageResult::Added => {
                println!(
                    "{}",
                    crate::internal::color::success(&format!(
                        "Added '{}' to {}",
                        package_name, file_path
                    ))
                );
            }
            AddPackageResult::AlreadyPresent => {
                println!(
                    "{}",
                    crate::internal::color::yellow(&format!(
                        "Package '{}' already exists in {}",
                        package_name, file_path
                    ))
                );
            }
        }
        return Ok(());
    }

    // Reverse the order so main appears at the bottom
    config_files.reverse();

    // Multiple files - prompt for selection
    println!(
        "\n{} {} config file(s):\n",
        crate::internal::color::bold("Found"),
        config_files.len()
    );

    for (i, file) in config_files.iter().enumerate() {
        let num_str = number_brackets(config_files.len() - 1 - i);
        let friendly = file.replace(&std::env::var("HOME").unwrap_or_default(), "~");
        println!(
            "{} {}",
            num_str,
            crate::internal::color::highlight(&friendly)
        );
    }
    println!();

    let selection = prompt_file_selection(config_files.len());
    match selection {
        Some(index) => {
            let file_path = &config_files[index];
            match add_package_to_file(package_name, file_path)? {
                AddPackageResult::Added => {
                    println!(
                        "{}",
                        crate::internal::color::success(&format!(
                            "Added '{}' to {}",
                            package_name, file_path
                        ))
                    );
                }
                AddPackageResult::AlreadyPresent => {
                    println!(
                        "{}",
                        crate::internal::color::yellow(&format!(
                            "Package '{}' already exists in {}",
                            package_name, file_path
                        ))
                    );
                }
            }
            Ok(())
        }
        None => {
            println!(
                "{}",
                crate::internal::color::yellow("No config file selected")
            );
            Ok(())
        }
    }
}

/// Get relevant config files for the current system
fn get_relevant_config_files() -> anyhow::Result<Vec<String>> {
    crate::internal::files::get_all_config_files()
}

/// Prompt user to select a config file from search results
fn prompt_file_selection(count: usize) -> Option<usize> {
    if count == 0 {
        return None;
    }

    loop {
        print!("Select config file (0-{}, or 'c' to cancel): ", count - 1);
        std::io::Write::flush(&mut std::io::stdout()).ok()?;

        let mut input = String::new();
        std::io::stdin().read_line(&mut input).ok()?;
        let input = input.trim();

        if input == "c" || input == "cancel" {
            return None;
        }

        match input.parse::<usize>() {
            Ok(num) if num < count => {
                let index = count - 1 - num;
                return Some(index);
            }
            _ => {
                println!(
                    "{}",
                    crate::internal::color::red("Invalid selection. Please try again.")
                );
            }
        }
    }
}
