# 🦉 Owl Package Manager

A modern, config-driven package manager for Arch Linux that simplifies system management through declarative configuration files.

## Features

- **📦 Unified Package Management**: Handles both official repositories and AUR packages
- **⚙️ Configuration-Driven**: Declarative YAML-like configuration files
- **🏠 Dotfiles Management**: Automatic dotfile synchronization and symlinking
- **🔧 System Services**: Manage systemd services alongside packages
- **🌍 Environment Variables**: Set system and user environment variables
- **🖥️ Host-Specific Configs**: Different configurations per machine
- **👥 Group Configurations**: Shared configs across multiple machines
- **🔍 Intelligent Tracking**: Track existing packages into configurations
- **🚀 Fast and Reliable**: Built in D for performance and safety

## Installation

### From Source

```bash
# Clone the repository
git clone <repository-url>
cd owl

# Build and install
make build
sudo make install

# Or install to custom prefix
make install PREFIX=/usr
```

### Build Requirements

- **D Compiler**: `dmd` or `ldc2`
- **DUB**: D package manager
- **dfmt**: D code formatter (optional, for development)

On Arch Linux:
```bash
sudo pacman -S dmd dub dfmt
```

## Quick Start

### 1. Initialize Configuration

Create your main configuration file:

```bash
mkdir -p ~/.owl
$EDITOR ~/.owl/main.owl
```

### 2. Basic Configuration

```yaml
# ~/.owl/main.owl
packages:
  - git
  - vim
  - firefox
  - code

@service:
  - tailscaled: { enable: true, start: true }

@env:
  EDITOR: vim
  BROWSER: firefox

# Dotfiles
git:
  configs:
    - source: gitconfig
      dest: ~/.gitconfig

vim:
  configs:
    - source: vimrc
      dest: ~/.vimrc
```

### 3. Apply Configuration

```bash
# Preview changes (dry run)
owl dry-run

# Apply changes
owl apply
```

## Configuration Format

Owl uses a simple, declarative configuration format:

### Package Declaration

```yaml
packages:
  - package-name
  - another-package
```

### Service Management

```yaml
@service:
  - service-name: { enable: true, start: true }
  - another-service: { enable: false }
```

### Environment Variables

```yaml
@env:
  VARIABLE_NAME: value
  EDITOR: vim
  PATH_ADDITION: /custom/bin
```

### Dotfiles Configuration

```yaml
package-name:
  configs:
    - source: config-file
      dest: ~/.config/app/config
    - source: another-file
      dest: ~/another-location
```

### Setup Scripts

```yaml
package-name:
  setup:
    - echo "Post-install script"
    - systemctl --user enable some-service
```

## Host-Specific Configuration

Create host-specific configurations:

```bash
# For hostname 'workstation'
$EDITOR ~/.owl/workstation.owl

# Or in hosts directory
mkdir -p ~/.owl/hosts
$EDITOR ~/.owl/hosts/workstation.owl
```

Example host-specific config:
```yaml
# ~/.owl/workstation.owl
packages:
  - docker
  - kubectl
  - terraform

@service:
  - docker: { enable: true, start: true }
```

## Commands

### Core Commands

- **`owl apply`** - Install packages and apply configuration
- **`owl dry-run`** - Preview what would be done without making changes
- **`owl upgrade`** - Upgrade all packages to latest versions

### Configuration Management

- **`owl configedit [target]`** - Edit configuration files
- **`owl dotedit [target]`** - Edit dotfiles

### Package Management

- **`owl add [search-terms]`** - Search and add packages to configuration
- **`owl track`** - Track existing packages into configuration
- **`owl hide`** - Hide packages from track suggestions

### Dotfiles

- **`owl dots`** - Check and sync only dotfiles configurations

### Debugging

- **`owl check`** - Display parsed configuration for debugging

### Options

- **`--no-aur`** - Skip AUR packages
- **`--dev`** - Include VCS packages (-git, -hg, etc.)
- **`--dry-run`** - Preview mode for any command
- **`--verbose`** - Show detailed output

## Examples

### Basic Usage

```bash
# Apply configuration
owl apply

# Preview changes
owl dry-run

# Apply without AUR packages
owl apply --no-aur

# Upgrade everything including VCS packages
owl upgrade --dev
```

### Configuration Management

```bash
# Edit main configuration
owl configedit

# Edit host-specific configuration
owl configedit hostname

# Edit dotfiles
owl dotedit

# Edit specific dotfile
owl dotedit vimrc
```

### Package Management

```bash
# Search and add a package
owl add firefox

# Track existing packages
owl track

# Hide packages from tracking
owl hide
```

## Directory Structure

```
~/.owl/
├── main.owl              # Main configuration
├── hostname.owl          # Host-specific config
├── hosts/                # Alternative host configs location
│   └── hostname.owl
├── groups/               # Group configurations
│   └── group.owl
├── dotfiles/             # Dotfile sources
│   ├── vimrc
│   ├── gitconfig
│   └── config/
└── .state/               # Internal state files
    ├── untracked.json
    └── hidden.txt
```

## Advanced Features

### Group Configurations

Create shared configurations for multiple machines:

```bash
mkdir -p ~/.owl/groups
$EDITOR ~/.owl/groups/development.owl
```

### Complex Dotfile Management

```yaml
neovim:
  configs:
    - source: nvim/
      dest: ~/.config/nvim/
    - source: nvim-local.lua
      dest: ~/.config/nvim/lua/local.lua
```

### Conditional Package Installation

Use host-specific configurations for different setups:

```yaml
# ~/.owl/laptop.owl
packages:
  - tlp              # Power management for laptops
  - brightnessctl    # Screen brightness control

# ~/.owl/desktop.owl  
packages:
  - nvidia-utils     # Desktop GPU drivers
```

## Development

### Building from Source

```bash
# Clone and build
git clone <repository-url>
cd owl
make build

# Development build with debug symbols
make debug

# Format code
make format

# Run tests
make test

# Install development build
make dev-install
```

### Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Format code with `make format`
5. Test your changes
6. Submit a pull request

## Troubleshooting

### Common Issues

**Build fails with "dmd not found"**
```bash
sudo pacman -S dmd dub
```

**Permission denied during install**
```bash
sudo make install
```

**AUR packages fail to build**
```bash
# Skip AUR packages temporarily
owl apply --no-aur
```

### Debug Mode

```bash
# Check configuration parsing
owl check

# Verbose output
owl apply --verbose

# Preview mode
owl dry-run
```

## License

Copyright © 2025, wobbat

## Support

- Check the configuration with `owl check`
- Use `owl --help` for command-specific help
- Run with `--verbose` for detailed output
- Use `dry-run` to preview changes safely