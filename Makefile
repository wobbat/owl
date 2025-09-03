# Owl Package Manager Makefile
# Config driven package manager for Arch Linux

# Project configuration
PROJECT_NAME = owl
VERSION = 0.1.0
AUTHOR = wobbat

# Build configuration
DUB = dub
DFMT = dfmt
BUILD_TYPE = release
BUILD_FLAGS = --build=$(BUILD_TYPE) --quiet
SOURCE_DIR = src
BUILD_DIR = .dub/build/$(BUILD_TYPE)

# Installation paths
PREFIX ?= /usr/local
BINDIR = $(PREFIX)/bin
MANDIR = $(PREFIX)/share/man/man1
DATADIR = $(PREFIX)/share
DOCDIR = $(DATADIR)/doc/$(PROJECT_NAME)

# Installation configuration
INSTALL = install
INSTALL_PROGRAM = $(INSTALL) -D -m 755
INSTALL_DATA = $(INSTALL) -D -m 644

# Build targets
.PHONY: all build release debug clean install uninstall format test help

# Default target
all: build

# Build the project
build:
	@echo "Building $(PROJECT_NAME)..."
	$(DUB) build $(BUILD_FLAGS)
	@echo "Build complete: ./$(PROJECT_NAME)"

# Release build (optimized)
release:
	@echo "Building $(PROJECT_NAME) (release)..."
	$(DUB) build --build=release --quiet
	@echo "Release build complete: ./$(PROJECT_NAME)"

# Debug build
debug:
	@echo "Building $(PROJECT_NAME) (debug)..."
	$(DUB) build --build=debug --quiet
	@echo "Debug build complete: ./$(PROJECT_NAME)"

# Format source code
format:
	@echo "Formatting source code..."
	@if command -v $(DFMT) >/dev/null 2>&1; then \
		$(DFMT) -i $(SOURCE_DIR)/; \
		echo "Code formatted successfully"; \
	else \
		echo "Warning: dfmt not found, skipping format"; \
	fi

# Run tests
test: build
	@echo "Running tests..."
	$(DUB) test
	@echo "Tests completed"

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	$(DUB) clean
	@if [ -f $(PROJECT_NAME) ]; then rm $(PROJECT_NAME); fi
	@echo "Clean complete"

# Install the binary and documentation
install: build
	@echo "Installing $(PROJECT_NAME) to $(PREFIX)..."
	$(INSTALL_PROGRAM) $(PROJECT_NAME) $(DESTDIR)$(BINDIR)/$(PROJECT_NAME)
	@if [ -f README.md ]; then \
		$(INSTALL_DATA) README.md $(DESTDIR)$(DOCDIR)/README.md; \
	fi
	@echo "Installation complete"
	@echo "  Binary: $(BINDIR)/$(PROJECT_NAME)"
	@echo "  Documentation: $(DOCDIR)/"

# Uninstall the binary and documentation
uninstall:
	@echo "Uninstalling $(PROJECT_NAME)..."
	@if [ -f $(DESTDIR)$(BINDIR)/$(PROJECT_NAME) ]; then \
		rm -f $(DESTDIR)$(BINDIR)/$(PROJECT_NAME); \
		echo "Removed: $(BINDIR)/$(PROJECT_NAME)"; \
	fi
	@if [ -d $(DESTDIR)$(DOCDIR) ]; then \
		rm -rf $(DESTDIR)$(DOCDIR); \
		echo "Removed: $(DOCDIR)/"; \
	fi
	@echo "Uninstallation complete"

# Development helpers
dev-install: debug
	@echo "Installing development build..."
	$(INSTALL_PROGRAM) $(PROJECT_NAME) $(DESTDIR)$(BINDIR)/$(PROJECT_NAME)

# Package for distribution (creates a simple tarball)
package: clean release
	@echo "Creating package..."
	@mkdir -p $(PROJECT_NAME)-$(VERSION)
	@cp -r src dub.sdl Makefile README.md $(PROJECT_NAME)-$(VERSION)/
	@cp $(PROJECT_NAME) $(PROJECT_NAME)-$(VERSION)/
	@tar czf $(PROJECT_NAME)-$(VERSION).tar.gz $(PROJECT_NAME)-$(VERSION)
	@rm -rf $(PROJECT_NAME)-$(VERSION)
	@echo "Package created: $(PROJECT_NAME)-$(VERSION).tar.gz"

# Show help
help:
	@echo "Owl Package Manager - Available targets:"
	@echo ""
	@echo "Build targets:"
	@echo "  build          Build the project (default)"
	@echo "  release        Build optimized release version"
	@echo "  debug          Build debug version with symbols"
	@echo "  clean          Remove build artifacts"
	@echo ""
	@echo "Development targets:"
	@echo "  format         Format source code with dfmt"
	@echo "  test           Run unit tests"
	@echo "  dev-install    Install debug build for development"
	@echo ""
	@echo "Installation targets:"
	@echo "  install        Install to system (default: /usr/local)"
	@echo "  uninstall      Remove from system"
	@echo ""
	@echo "Distribution targets:"
	@echo "  package        Create distribution package"
	@echo ""
	@echo "Configuration:"
	@echo "  PREFIX=$(PREFIX)"
	@echo "  DESTDIR=$(DESTDIR)"
	@echo ""
	@echo "Examples:"
	@echo "  make build                    # Build the project"
	@echo "  make install PREFIX=/usr     # Install to /usr instead of /usr/local"
	@echo "  sudo make install            # Install with default prefix"
	@echo "  make dev-install             # Install debug build for development"

# Check if required tools are available
check-deps:
	@echo "Checking dependencies..."
	@command -v $(DUB) >/dev/null 2>&1 || { echo "Error: dub not found. Please install dub (D package manager)"; exit 1; }
	@command -v dmd >/dev/null 2>&1 || command -v ldc2 >/dev/null 2>&1 || { echo "Error: D compiler not found. Please install dmd or ldc2"; exit 1; }
	@echo "Dependencies OK"

# Show project information
info:
	@echo "Project: $(PROJECT_NAME)"
	@echo "Version: $(VERSION)"
	@echo "Author:  $(AUTHOR)"
	@echo "Build:   $(BUILD_TYPE)"
	@echo "Source:  $(SOURCE_DIR)"
	@echo "Prefix:  $(PREFIX)"
