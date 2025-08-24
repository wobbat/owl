# Owl AUR Helper Makefile
export GO111MODULE=on
GOPROXY ?= https://proxy.golang.org,direct
export GOPROXY

# Build configuration
BUILD_TAG = devel
ARCH ?= $(shell uname -m)
BIN := owl
DESTDIR :=
GO ?= go
PKGNAME := owl
PREFIX := /usr/local

# Version information
MAJORVERSION := 0
MINORVERSION := 1
PATCHVERSION := 0
VERSION ?= ${MAJORVERSION}.${MINORVERSION}.${PATCHVERSION}

# Build flags
FLAGS ?= -trimpath -mod=readonly -modcacherw
EXTRA_FLAGS ?= -buildmode=pie
LDFLAGS := -X "main.version=${VERSION}" -linkmode=external -compressdwarf=false

# Release configuration
RELEASE_DIR := ${PKGNAME}_${VERSION}_${ARCH}
PACKAGE := $(RELEASE_DIR).tar.gz
SOURCES ?= $(shell find . -name "*.go" -type f ! -path "./example-yay-codebase/*")

# Default target
.PHONY: default
default: build

.PHONY: all
all: | clean build test

# Clean build artifacts
.PHONY: clean
clean:
	$(GO) clean $(FLAGS) -i ./...
	rm -rf $(BIN) $(PKGNAME)_* dist/

# Build the binary
.PHONY: build
build: $(BIN)

$(BIN): $(SOURCES)
	$(GO) build $(FLAGS) -ldflags '$(LDFLAGS)' $(EXTRA_FLAGS) -o $@ ./main.go

# Development build (faster, no optimization)
.PHONY: dev
dev:
	$(GO) build -o $(BIN) ./main.go

# Run tests
.PHONY: test
test:
	$(GO) test -race -covermode=atomic $(FLAGS) ./...

.PHONY: test-verbose
test-verbose:
	$(GO) test -race -covermode=atomic -v $(FLAGS) ./...

# Integration tests (if any)
.PHONY: test-integration
test-integration:
	$(GO) test -tags=integration $(FLAGS) ./...

# Code quality checks
.PHONY: lint
lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		GOFLAGS="$(FLAGS)" golangci-lint run ./...; \
	else \
		echo "golangci-lint not found, install it with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
	fi

.PHONY: fmt
fmt:
	go fmt ./...

.PHONY: vet
vet:
	go vet ./...

.PHONY: check
check: fmt vet lint test

# Install the binary and supporting files
.PHONY: install
install: build
	@echo "Installing owl to $(DESTDIR)$(PREFIX)/bin/$(BIN)"
	install -Dm755 $(BIN) $(DESTDIR)$(PREFIX)/bin/$(BIN)
	@if [ -f "completions/bash" ]; then \
		echo "Installing bash completion"; \
		install -Dm644 completions/bash $(DESTDIR)$(PREFIX)/share/bash-completion/completions/$(PKGNAME); \
	fi
	@if [ -f "completions/zsh" ]; then \
		echo "Installing zsh completion"; \
		install -Dm644 completions/zsh $(DESTDIR)$(PREFIX)/share/zsh/site-functions/_$(PKGNAME); \
	fi
	@if [ -f "completions/fish" ]; then \
		echo "Installing fish completion"; \
		install -Dm644 completions/fish $(DESTDIR)$(PREFIX)/share/fish/vendor_completions.d/$(PKGNAME).fish; \
	fi
	@if [ -f "doc/$(PKGNAME).8" ]; then \
		echo "Installing man page"; \
		install -Dm644 doc/$(PKGNAME).8 $(DESTDIR)$(PREFIX)/share/man/man8/$(PKGNAME).8; \
	fi
	@echo "Installation complete!"

# Uninstall the binary and supporting files
.PHONY: uninstall
uninstall:
	@echo "Removing owl from $(DESTDIR)$(PREFIX)/bin/$(BIN)"
	rm -f $(DESTDIR)$(PREFIX)/bin/$(BIN)
	rm -f $(DESTDIR)$(PREFIX)/share/bash-completion/completions/$(PKGNAME)
	rm -f $(DESTDIR)$(PREFIX)/share/zsh/site-functions/_$(PKGNAME)
	rm -f $(DESTDIR)$(PREFIX)/share/fish/vendor_completions.d/$(PKGNAME).fish
	rm -f $(DESTDIR)$(PREFIX)/share/man/man8/$(PKGNAME).8
	@echo "Uninstallation complete!"

# Install development dependencies
.PHONY: deps
deps:
	$(GO) mod download
	$(GO) mod tidy

# Update dependencies
.PHONY: update-deps
update-deps:
	$(GO) get -u ./...
	$(GO) mod tidy

# Create release artifacts
.PHONY: release
release: $(PACKAGE)

$(RELEASE_DIR):
	mkdir -p $(RELEASE_DIR)

$(PACKAGE): build $(RELEASE_DIR)
	@echo "Creating release package $(PACKAGE)"
	strip $(BIN)
	cp $(BIN) $(RELEASE_DIR)/
	@if [ -f "README.md" ]; then cp README.md $(RELEASE_DIR)/; fi
	@if [ -f "LICENSE" ]; then cp LICENSE $(RELEASE_DIR)/; fi
	@if [ -d "completions" ]; then cp -r completions $(RELEASE_DIR)/; fi
	@if [ -d "doc" ]; then cp -r doc $(RELEASE_DIR)/; fi
	tar -czf $(PACKAGE) $(RELEASE_DIR)
	@echo "Release package created: $(PACKAGE)"

# Docker build (if Dockerfile exists)
.PHONY: docker-build
docker-build:
	@if [ -f "Dockerfile" ]; then \
		docker build -t $(PKGNAME):$(VERSION) .; \
	else \
		echo "Dockerfile not found"; \
	fi

# Run the application
.PHONY: run
run: build
	./$(BIN)

# Run with arguments
.PHONY: run-args
run-args: build
	./$(BIN) $(ARGS)

# Show help
.PHONY: help
help:
	@echo "Owl AUR Helper - Available targets:"
	@echo ""
	@echo "Build targets:"
	@echo "  build       Build the owl binary (default)"
	@echo "  dev         Fast development build"
	@echo "  clean       Clean build artifacts"
	@echo ""
	@echo "Testing targets:"
	@echo "  test        Run tests"
	@echo "  test-verbose Run tests with verbose output"
	@echo "  test-integration Run integration tests"
	@echo ""
	@echo "Code quality targets:"
	@echo "  fmt         Format code"
	@echo "  vet         Run go vet"
	@echo "  lint        Run golangci-lint"
	@echo "  check       Run fmt, vet, lint, and test"
	@echo ""
	@echo "Installation targets:"
	@echo "  install     Install owl binary and supporting files"
	@echo "  uninstall   Remove owl binary and supporting files"
	@echo ""
	@echo "Dependency targets:"
	@echo "  deps        Install dependencies"
	@echo "  update-deps Update dependencies"
	@echo ""
	@echo "Release targets:"
	@echo "  release     Create release package"
	@echo ""
	@echo "Development targets:"
	@echo "  run         Build and run owl"
	@echo "  run-args    Build and run owl with ARGS='your-args'"
	@echo ""
	@echo "Docker targets:"
	@echo "  docker-build Build Docker image"
	@echo ""
	@echo "Configuration:"
	@echo "  PREFIX      Installation prefix (default: /usr/local)"
	@echo "  DESTDIR     Destination directory for packaging"
	@echo "  VERSION     Version number (default: $(VERSION))"
	@echo "  GO          Go compiler (default: go)"

# Show current configuration
.PHONY: info
info:
	@echo "Owl Build Configuration:"
	@echo "  Version:     $(VERSION)"
	@echo "  Binary:      $(BIN)"
	@echo "  Arch:        $(ARCH)"
	@echo "  Go:          $(GO)"
	@echo "  Prefix:      $(PREFIX)"
	@echo "  Destdir:     $(DESTDIR)"
	@echo "  Build flags: $(FLAGS)"
	@echo "  LDFLAGS:     $(LDFLAGS)"
