package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"owl/internal/constants"
	"owl/internal/types"
)

// ConfigParseError represents a configuration parsing error
type ConfigParseError struct {
	FilePath   string
	LineNumber int
	Line       string
	Message    string
}

func (e *ConfigParseError) Error() string {
	return fmt.Sprintf("%s:%d: %s\n  → %s", e.FilePath, e.LineNumber, e.Message, strings.TrimSpace(e.Line))
}

// ParseOptions represents options for configuration parsing
type ParseOptions struct {
	Strict              bool
	SourcePath          string
	AllowInlineComments bool
	SourceType          string // main, host, group
	GroupName           string
}

// ConfigResult represents the result of parsing configuration files
type ConfigResult struct {
	Entries    []types.ConfigEntry
	GlobalEnvs []types.EnvVar
}

// LoadConfigForHost loads and merges configuration files for a specific host
func LoadConfigForHost(hostname string) (*ConfigResult, error) {
	// Check for custom config file path via environment variable
	if customConfigPath := os.Getenv("CONFIG_FILE"); customConfigPath != "" {
		// Use custom config file path
		if _, err := os.Stat(customConfigPath); os.IsNotExist(err) {
			return nil, &ConfigParseError{
				FilePath: customConfigPath,
				Line:     "",
				Message:  "Custom config file not found",
			}
		}

		result, err := parseOwlConfigFile(customConfigPath, ParseOptions{
			SourcePath: customConfigPath,
			SourceType: "main",
			Strict:     true,
		})
		if err != nil {
			return nil, err
		}

		return result, nil
	}

	// Default behavior: load from ~/.owl/
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user home directory: %w", err)
	}

	owlRoot := filepath.Join(homeDir, constants.OwlRootDir)
	globalPath := filepath.Join(owlRoot, "main.owl")

	// Load global configuration
	if _, err := os.Stat(globalPath); os.IsNotExist(err) {
		return nil, &ConfigParseError{
			FilePath: globalPath,
			Line:     "",
			Message:  "Global config file not found",
		}
	}

	globalResult, err := parseOwlConfigFile(globalPath, ParseOptions{
		SourcePath: globalPath,
		SourceType: "main",
		Strict:     true,
	})
	if err != nil {
		return nil, err
	}

	// Load host-specific configuration if it exists
	hostPath := filepath.Join(owlRoot, constants.OwlHostsDir, hostname+".owl")
	var hostResult *ConfigResult

	if _, err := os.Stat(hostPath); err == nil {
		hostResult, err = parseOwlConfigFile(hostPath, ParseOptions{
			SourcePath: hostPath,
			SourceType: "host",
			Strict:     true,
		})
		if err != nil {
			return nil, err
		}
	}

	// Merge configurations
	merged := mergeConfigurations(globalResult, hostResult)
	return merged, nil
}

// parseOwlConfigFile parses a single .owl configuration file
func parseOwlConfigFile(filePath string, options ParseOptions) (*ConfigResult, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file: %w", err)
	}
	defer file.Close()

	var entries []types.ConfigEntry
	var globalEnvs []types.EnvVar
	var current *types.ConfigEntry
	var configs []types.ConfigMapping
	var setups []string
	var services []string
	var envs []types.EnvVar
	var packagesMode bool
	var pendingPackages []string

	visited := make(map[string]bool)
	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		rawLine := scanner.Text()
		line := rawLine

		// Handle inline comments if enabled
		if options.AllowInlineComments {
			line = parseInlineComments(line)
		}

		line = strings.TrimSpace(line)

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Handle group includes
		if strings.HasPrefix(line, "@group ") {
			groupName := strings.TrimSpace(line[7:])
			if err := validateDirective("@group", groupName, lineNum, filePath, rawLine); err != nil {
				if options.Strict {
					return nil, err
				}
				continue
			}

			groupEntries, err := loadGroup(groupName, visited, filepath.Dir(filePath))
			if err != nil {
				if options.Strict {
					return nil, err
				}
				continue
			}
			entries = append(entries, groupEntries...)
			continue
		}

		// Handle global environment variables
		if strings.HasPrefix(line, "@env ") {
			envArgs := strings.TrimSpace(line[5:])
			if err := validateDirective("@env", envArgs, lineNum, filePath, rawLine); err != nil {
				if options.Strict {
					return nil, err
				}
				continue
			}

			if match := regexp.MustCompile(`^(\S+)\s*=\s*(.+)$`).FindStringSubmatch(envArgs); match != nil && len(match) == 3 {
				globalEnvs = append(globalEnvs, types.EnvVar{
					Key:   match[1],
					Value: match[2],
				})
			}
			continue
		}

		// Handle package mode
		if line == "@packages" {
			packagesMode = true
			pendingPackages = []string{}
			continue
		}

		// Handle packages in package mode
		if packagesMode && !strings.HasPrefix(line, "@") && !strings.HasPrefix(line, ":") && !strings.HasPrefix(line, "!") {
			if err := validateDirective("@package", line, lineNum, filePath, rawLine); err == nil {
				pendingPackages = append(pendingPackages, line)
			}
			continue
		}

		// Exit package mode for other directives
		if packagesMode {
			packagesMode = false
			for _, pkg := range pendingPackages {
				entries = append(entries, createEntry(pkg, nil, nil, nil, nil, options))
			}
			pendingPackages = []string{}
		}

		// Handle package directive
		if strings.HasPrefix(line, "@package ") {
			if current != nil {
				current.Configs = configs
				current.Setups = setups
				current.Services = services
				current.Envs = envs
				entries = append(entries, *current)
			}

			pkgName := strings.TrimSpace(line[9:])
			if err := validateDirective("@package", pkgName, lineNum, filePath, rawLine); err != nil {
				if options.Strict {
					return nil, err
				}
				continue
			}

			current = &types.ConfigEntry{Package: pkgName}
			configs = []types.ConfigMapping{}
			setups = []string{}
			services = []string{}
			envs = []types.EnvVar{}
			continue
		}

		// Handle config directive
		if strings.HasPrefix(line, ":config ") {
			configArgs := strings.TrimSpace(line[8:])
			if err := validateDirective(":config", configArgs, lineNum, filePath, rawLine); err != nil {
				if options.Strict {
					return nil, err
				}
				continue
			}

			if match := regexp.MustCompile(`^(\S+)\s*->\s*(\S+)$`).FindStringSubmatch(configArgs); match != nil && len(match) == 3 {
				source := match[1]
				destination := match[2]

				// If source is absolute or relative path, use as-is
				// Otherwise, assume it's in the dotfiles directory
				var sourcePath string
				if filepath.IsAbs(source) || strings.HasPrefix(source, "./") || strings.HasPrefix(source, "../") {
					sourcePath = source
				} else {
					homeDir, _ := os.UserHomeDir()
					sourcePath = filepath.Join(homeDir, constants.OwlRootDir, constants.OwlDotfilesDir, source)
				}

				configs = append(configs, types.ConfigMapping{
					Source:      sourcePath,
					Destination: destination,
				})
			}
			continue
		}

		// Handle service directive
		if strings.HasPrefix(line, ":service ") {
			serviceName := strings.TrimSpace(line[9:])
			if err := validateDirective(":service", serviceName, lineNum, filePath, rawLine); err != nil {
				if options.Strict {
					return nil, err
				}
				continue
			}
			services = append(services, serviceName)
			continue
		}

		// Handle environment variable directive
		if strings.HasPrefix(line, ":env ") {
			envArgs := strings.TrimSpace(line[5:])
			if err := validateDirective(":env", envArgs, lineNum, filePath, rawLine); err != nil {
				if options.Strict {
					return nil, err
				}
				continue
			}

			if match := regexp.MustCompile(`^(\S+)\s*=\s*(.+)$`).FindStringSubmatch(envArgs); match != nil && len(match) == 3 {
				envs = append(envs, types.EnvVar{
					Key:   match[1],
					Value: match[2],
				})
			}
			continue
		}

		// Handle setup directive
		if strings.HasPrefix(line, "!setup ") {
			setupScript := strings.TrimSpace(line[7:])
			if err := validateDirective("!setup", setupScript, lineNum, filePath, rawLine); err != nil {
				if options.Strict {
					return nil, err
				}
				continue
			}
			setups = append(setups, setupScript)
			continue
		}

		// Handle unknown directives
		if strings.HasPrefix(line, "@") || strings.HasPrefix(line, ":") || strings.HasPrefix(line, "!") {
			parts := strings.Fields(line)
			if len(parts) > 0 {
				directive := parts[0]
				args := strings.Join(parts[1:], " ")
				if err := validateDirective(directive, args, lineNum, filePath, rawLine); err != nil {
					if options.Strict {
						return nil, err
					}
				}
			}
			continue
		}

		// Unknown line
		if options.Strict {
			return nil, &ConfigParseError{
				FilePath:   filePath,
				LineNumber: lineNum,
				Line:       rawLine,
				Message:    fmt.Sprintf("Unrecognized line: \"%s\". Expected a directive (@package, @group, :config, !setup) or package name in @packages block", line),
			}
		}
	}

	// Handle pending packages from package mode
	if packagesMode && len(pendingPackages) > 0 {
		for _, pkg := range pendingPackages {
			entries = append(entries, createEntry(pkg, nil, nil, nil, nil, options))
		}
	}

	// Add the last package entry
	if current != nil {
		current.Configs = configs
		current.Setups = setups
		current.Services = services
		current.Envs = envs
		entries = append(entries, *current)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading config file: %w", err)
	}

	return &ConfigResult{
		Entries:    entries,
		GlobalEnvs: globalEnvs,
	}, nil
}

// loadGroup loads a group configuration file
func loadGroup(groupName string, visited map[string]bool, basePath string) ([]types.ConfigEntry, error) {
	if visited[groupName] {
		return nil, &ConfigParseError{
			FilePath: filepath.Join(basePath, constants.OwlGroupsDir, groupName+".owl"),
			Message:  fmt.Sprintf("Circular dependency detected for group \"%s\"", groupName),
		}
	}

	visited[groupName] = true
	defer delete(visited, groupName)

	homeDir, _ := os.UserHomeDir()
	groupPath := filepath.Join(homeDir, constants.OwlRootDir, constants.OwlGroupsDir, groupName+".owl")

	if _, err := os.Stat(groupPath); os.IsNotExist(err) {
		return nil, &ConfigParseError{
			FilePath: groupPath,
			Message:  fmt.Sprintf("Group file not found: %s", groupPath),
		}
	}

	result, err := parseOwlConfigFile(groupPath, ParseOptions{
		SourcePath: groupPath,
		SourceType: "group",
		GroupName:  groupName,
		Strict:     true,
	})
	if err != nil {
		return nil, err
	}

	return result.Entries, nil
}

// createEntry creates a new ConfigEntry
func createEntry(packageName string, configs []types.ConfigMapping, setups []string, services []string, envs []types.EnvVar, options ParseOptions) types.ConfigEntry {
	entry := types.ConfigEntry{
		Package:    packageName,
		Configs:    configs,
		Setups:     setups,
		Services:   services,
		Envs:       envs,
		SourceFile: options.SourcePath,
		SourceType: options.SourceType,
		GroupName:  options.GroupName,
	}

	if configs == nil {
		entry.Configs = []types.ConfigMapping{}
	}
	if setups == nil {
		entry.Setups = []string{}
	}
	if services == nil {
		entry.Services = []string{}
	}
	if envs == nil {
		entry.Envs = []types.EnvVar{}
	}

	return entry
}

// parseInlineComments removes inline comments from a line
func parseInlineComments(line string) string {
	var result strings.Builder
	escaped := false
	inQuotes := false
	var quoteChar rune

	for _, char := range line {
		if escaped {
			result.WriteRune(char)
			escaped = false
			continue
		}

		if char == '\\' {
			result.WriteRune(char)
			escaped = true
			continue
		}

		if !inQuotes && (char == '"' || char == '\'') {
			inQuotes = true
			quoteChar = char
			result.WriteRune(char)
			continue
		}

		if inQuotes && char == quoteChar {
			inQuotes = false
			quoteChar = 0
			result.WriteRune(char)
			continue
		}

		if !inQuotes && char == '#' {
			break
		}

		result.WriteRune(char)
	}

	return strings.TrimSpace(result.String())
}

// validateDirective validates a configuration directive
func validateDirective(directive, args string, lineNum int, sourcePath, rawLine string) error {
	switch directive {
	case "@package":
		if strings.TrimSpace(args) == "" {
			return &ConfigParseError{
				FilePath:   sourcePath,
				LineNumber: lineNum,
				Line:       rawLine,
				Message:    "Package name cannot be empty",
			}
		}
		if matched, _ := regexp.MatchString(`^[a-zA-Z0-9_\-\/\.]+$`, strings.TrimSpace(args)); !matched {
			return &ConfigParseError{
				FilePath:   sourcePath,
				LineNumber: lineNum,
				Line:       rawLine,
				Message:    "Package name contains invalid characters",
			}
		}

	case "@group":
		if strings.TrimSpace(args) == "" {
			return &ConfigParseError{
				FilePath:   sourcePath,
				LineNumber: lineNum,
				Line:       rawLine,
				Message:    "Group name cannot be empty",
			}
		}
		if matched, _ := regexp.MatchString(`^[a-zA-Z0-9_\-\/]+$`, strings.TrimSpace(args)); !matched {
			return &ConfigParseError{
				FilePath:   sourcePath,
				LineNumber: lineNum,
				Line:       rawLine,
				Message:    "Group name contains invalid characters",
			}
		}

	case ":config":
		configMatch := regexp.MustCompile(`^\s*(\S+)\s*->\s*(\S+)\s*$`).FindStringSubmatch(args)
		if configMatch == nil {
			return &ConfigParseError{
				FilePath:   sourcePath,
				LineNumber: lineNum,
				Line:       rawLine,
				Message:    "Config directive must follow format \":config <source> -> <destination>\"",
			}
		}

	case "!setup":
		if strings.TrimSpace(args) == "" {
			return &ConfigParseError{
				FilePath:   sourcePath,
				LineNumber: lineNum,
				Line:       rawLine,
				Message:    "Setup script cannot be empty",
			}
		}

	case ":service":
		if strings.TrimSpace(args) == "" {
			return &ConfigParseError{
				FilePath:   sourcePath,
				LineNumber: lineNum,
				Line:       rawLine,
				Message:    "Service name cannot be empty",
			}
		}
		if matched, _ := regexp.MatchString(`^[a-zA-Z0-9_\-\.]+$`, strings.TrimSpace(args)); !matched {
			return &ConfigParseError{
				FilePath:   sourcePath,
				LineNumber: lineNum,
				Line:       rawLine,
				Message:    "Service name contains invalid characters",
			}
		}

	case "@env":
		globalEnvMatch := regexp.MustCompile(`^\s*(\S+)\s*=\s*(.+)\s*$`).FindStringSubmatch(args)
		if globalEnvMatch == nil {
			return &ConfigParseError{
				FilePath:   sourcePath,
				LineNumber: lineNum,
				Line:       rawLine,
				Message:    "Global environment variable must follow format \"@env <KEY> = <VALUE>\"",
			}
		}
		if len(globalEnvMatch) < 3 || globalEnvMatch[1] == "" || globalEnvMatch[2] == "" {
			return &ConfigParseError{
				FilePath:   sourcePath,
				LineNumber: lineNum,
				Line:       rawLine,
				Message:    "Global environment variable key and value cannot be empty",
			}
		}

	case ":env":
		envMatch := regexp.MustCompile(`^\s*(\S+)\s*=\s*(.+)\s*$`).FindStringSubmatch(args)
		if envMatch == nil {
			return &ConfigParseError{
				FilePath:   sourcePath,
				LineNumber: lineNum,
				Line:       rawLine,
				Message:    "Environment variable must follow format \":env <KEY> = <VALUE>\"",
			}
		}
		if len(envMatch) < 3 || envMatch[1] == "" || envMatch[2] == "" {
			return &ConfigParseError{
				FilePath:   sourcePath,
				LineNumber: lineNum,
				Line:       rawLine,
				Message:    "Environment variable key and value cannot be empty",
			}
		}

	default:
		if strings.HasPrefix(directive, "@") || strings.HasPrefix(directive, ":") || strings.HasPrefix(directive, "!") {
			return &ConfigParseError{
				FilePath:   sourcePath,
				LineNumber: lineNum,
				Line:       rawLine,
				Message:    fmt.Sprintf("Unknown directive: %s", directive),
			}
		}
	}

	return nil
}

// mergeConfigurations merges global and host configurations
func mergeConfigurations(global, host *ConfigResult) *ConfigResult {
	if host == nil {
		return global
	}

	merged := make(map[string]types.ConfigEntry)

	// Add global entries
	for _, entry := range global.Entries {
		merged[entry.Package] = entry
	}

	// Override with host entries
	for _, entry := range host.Entries {
		if existing, exists := merged[entry.Package]; exists {
			// Package exists in both global and host configs
			// Host configs override global configs, but keep global setups if host has none
			mergedEntry := entry
			if len(entry.Configs) == 0 {
				mergedEntry.Configs = existing.Configs
			}
			if len(entry.Setups) == 0 {
				mergedEntry.Setups = existing.Setups
			}
			merged[entry.Package] = mergedEntry
		} else {
			// Package only exists in host config
			merged[entry.Package] = entry
		}
	}

	// Convert map back to slice
	var entries []types.ConfigEntry
	for _, entry := range merged {
		entries = append(entries, entry)
	}

	// Combine global environment variables
	var combinedGlobalEnvs []types.EnvVar
	combinedGlobalEnvs = append(combinedGlobalEnvs, global.GlobalEnvs...)
	if host != nil {
		combinedGlobalEnvs = append(combinedGlobalEnvs, host.GlobalEnvs...)
	}

	return &ConfigResult{
		Entries:    entries,
		GlobalEnvs: combinedGlobalEnvs,
	}
}
