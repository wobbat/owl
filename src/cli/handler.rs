use crate::commands::{add, adopt, apply, dots, edit, find};
use crate::internal::color;
use crate::internal::constants;
use clap::{Parser, Subcommand};

/// Global options for the CLI
#[derive(Debug, Clone, Parser)]
#[command(name = "owl", about = "Dotfile and package manager")]
pub struct Cli {
    /// Enable verbose output
    #[arg(short, long)]
    pub verbose: bool,

    /// Perform a dry run without making changes
    #[arg(long)]
    pub dry_run: bool,

    /// Run in non-interactive mode
    #[arg(short = 'y', long)]
    pub non_interactive: bool,

    #[command(subcommand)]
    pub command: Option<Commands>,
}

/// Edit target types for better type safety
#[derive(Debug, Clone, PartialEq, clap::ValueEnum)]
pub enum EditTarget {
    Dots,
    Config,
}

/// Available commands for the CLI
#[derive(Debug, Clone, Subcommand)]
pub enum Commands {
    /// Apply configuration (default command)
    Apply,
    /// Edit dotfiles or config
    Edit {
        /// Type to edit (dots or config)
        target: EditTarget,
        /// Argument for edit
        argument: String,
    },
    /// List dotfiles
    Dots,
    /// Add packages
    Add {
        /// Packages to add
        items: Vec<String>,
        /// Search mode
        #[arg(long)]
        search: bool,
    },
    /// Adopt existing packages
    Adopt {
        /// Packages to adopt
        items: Vec<String>,
        /// Discover explicitly installed unmanaged packages for adoption
        #[arg(long)]
        all: bool,
    },
    /// Find packages or files
    Find {
        /// Query terms
        query: Vec<String>,
    },
    /// Check configuration
    ConfigCheck {
        /// Specific config file to check
        file: Option<String>,
    },
    /// Show host configuration
    ConfigHost,
    /// Clean up files
    Clean {
        /// Specific filename to clean
        filename: Option<String>,
    },
    /// Alias for edit dots
    #[command(alias = "de")]
    EditDots {
        /// Argument for edit
        argument: String,
    },
    /// Alias for edit config
    #[command(alias = "ce")]
    EditConfig {
        /// Argument for edit
        argument: String,
    },
}

/// Global flags extracted from CLI for passing to commands
#[derive(Debug, Clone)]
pub struct GlobalFlags {
    pub verbose: bool,
    pub dry_run: bool,
    pub non_interactive: bool,
}

impl From<&Cli> for GlobalFlags {
    fn from(cli: &Cli) -> Self {
        Self {
            verbose: cli.verbose,
            dry_run: cli.dry_run,
            non_interactive: cli.non_interactive,
        }
    }
}

/// Execute the parsed command
fn execute_command(cli: &Cli) {
    let flags = GlobalFlags::from(cli);

    if flags.verbose {
        println!("{}", color::dim("[verbose] args parsed"));
    }

    // Normalize command aliases to their canonical form
    let command = match &cli.command {
        Some(Commands::EditDots { argument }) => Some(Commands::Edit {
            target: EditTarget::Dots,
            argument: argument.clone(),
        }),
        Some(Commands::EditConfig { argument }) => Some(Commands::Edit {
            target: EditTarget::Config,
            argument: argument.clone(),
        }),
        other => other.clone(),
    };

    match command {
        Some(Commands::Apply) | None => apply::run(&flags),
        Some(Commands::Edit { target, argument }) => {
            let typ = match target {
                EditTarget::Dots => constants::EDIT_TYPE_DOTS,
                EditTarget::Config => constants::EDIT_TYPE_CONFIG,
            };
            if let Err(err) = edit::run(typ, &argument) {
                eprintln!("{}", color::red(&err.to_string()));
                std::process::exit(1);
            }
        }
        Some(Commands::Dots) => dots::run(&flags),
        Some(Commands::Add { items, search }) => add::run(&items, search),
        Some(Commands::Adopt { items, all }) => adopt::run(&items, all),
        Some(Commands::Find { query }) => find::run(&query),
        Some(Commands::ConfigCheck { file }) => {
            if let Some(f) = file {
                if let Err(err) = crate::core::config::validator::run_configcheck(&f) {
                    eprintln!("{}", color::red(&err.to_string()));
                    std::process::exit(1);
                }
            } else if let Err(err) = crate::core::config::validator::run_full_configcheck() {
                eprintln!("{}", color::red(&err.to_string()));
                std::process::exit(1);
            }
        }
        Some(Commands::ConfigHost) => {
            if let Err(err) = crate::core::config::validator::run_confighost() {
                eprintln!("{}", color::red(&err.to_string()));
                std::process::exit(1);
            }
        }
        Some(Commands::Clean { filename }) => {
            let result = match filename {
                Some(fname) => {
                    let result = crate::commands::clean::handle_clean(&fname);
                    if result.is_ok() {
                        println!("[{}]", color::blue("clean"));
                        println!("  {} {}", color::green("✓"), color::dim(&fname));
                    }
                    result
                }
                None => crate::commands::clean::handle_clean_all(),
            };
            if let Err(err) = result {
                eprintln!("{}", color::red(&err.to_string()));
                std::process::exit(1);
            }
        }
        // These are normalized above, so they should never match here
        Some(Commands::EditDots { .. }) | Some(Commands::EditConfig { .. }) => unreachable!(),
    }
}

/// Parse command line arguments and execute the corresponding command
pub fn parse_and_execute() {
    let cli = match Cli::try_parse() {
        Ok(cli) => cli,
        Err(err) => {
            eprintln!("{}", color::red(&err.to_string()));
            std::process::exit(1);
        }
    };
    execute_command(&cli);
}
