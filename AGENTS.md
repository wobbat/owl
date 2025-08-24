# AGENTS.md

## Project Overview

This repository contains the codebase for **owl**, a Go-based AUR (Arch User Repository) helper tool. Owl is designed to assist with managing packages from the Arch User Repository, providing functionalities similar to tools like `yay`. It simplifies installing, updating, and querying AUR packages while ensuring security and ease of use.

## Key Features

- **Package Management**: Install, update, and remove AUR packages.
- **Query and Search**: Search for packages and query installed packages.
- **Dependency Resolution**: Automatically handle dependencies and conflicts.
- **Security Checks**: Verify package integrity and handle GPG signatures.
- **CLI Interface**: Command-line driven with support for various operations.

## Codebase Structure

- **Root Directory**:
  - `main.go`: Entry point for the application.
  - `go.mod` and `go.sum`: Go module files for dependency management.

- **internal/**: Core application logic.
  - `cli/`: Command-line interface handling.
  - `config/`: Configuration management.
  - `handlers/`: Request and operation handlers.
  - `packages/`: Package-related operations (e.g., querying, resolving).
  - `services/`: Core services for AUR interactions.
  - `ui/`: User interface components.

- **pkg/**: Reusable packages.
  - `owl/`: Main package for owl-specific functionality, including manager logic.

VERY IMPORTANT:
- **example-yay-codebase/**: Contains the actual codebase of the `yay` AUR helper for reference and inspiration only.
  - **Important**: Do not copy code directly from this directory to avoid plagiarism. Use it solely to understand best practices for implementing an AUR helper.
    * Only for how to interact with libalpm and other package related stuff


IMPORTANT: Do not take over pacman or yay ui. I like my current ui and that should stay!
Important: We should not rely on pacman or another command line tool (just like yay does not)

## Usage Notes

- The project is built using Go modules.
- Run `go mod tidy` to manage dependencies.
