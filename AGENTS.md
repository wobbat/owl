# OWL

A dotfile and package manager with declarative configuration files. The CLI supports adding, adopting, finding, and applying configuration with dry-run and verbose modes.

## Quick Commands

- Build: `cargo build`
- Test: `cargo test`
- Run (apply): `cargo run --`
- Install locally: `cargo install --path .`

## Project Structure

- `src/main.rs` - Entry point
- `src/cli/` - CLI parsing and UI
- `src/commands/` - Command implementations (add, adopt, apply, dots, edit, find, clean)
- `src/core/` - Core logic (config, dotfiles, package management, state)
- `src/domain/` - Domain models and types
- `src/infrastructure/` - Infrastructure code
- `src/internal/` - Utilities, constants, and helpers
- `src/error.rs` - Error types

## CLI Commands

Primary subcommands (see `src/cli/handler.rs` for flags):
- `apply` (default)
- `dots`
- `add`
- `adopt`
- `find`
- `edit`
- `config-check`
- `config-host`
- `clean`

## Global Flags

- `-v, --verbose` - Verbose output
- `--dry-run` - Do not make changes
- `-y, --non-interactive` - Non-interactive mode
