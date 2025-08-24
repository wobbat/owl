package ui

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"owl/internal/constants"
	"owl/internal/types"

	"github.com/fatih/color"
)

// Color functions
var (
	primary   = color.New(color.FgBlue).SprintFunc()
	secondary = color.New(color.Faint).SprintFunc()
	success   = color.New(color.FgGreen).SprintFunc()
	errColor  = color.New(color.FgRed).SprintFunc()
	warning   = color.New(color.FgYellow).SprintFunc()
	info      = color.New(color.Faint).SprintFunc()
	muted     = color.New(color.Faint).SprintFunc()
	accent    = color.New(color.FgWhite).SprintFunc()
	cyan      = color.New(color.FgCyan).SprintFunc()
	white     = color.New(color.FgWhite).SprintFunc()
)

// Exported color functions for legacy compatibility
func Muted(text string) string {
	return muted(text)
}

func Success(text string) string {
	return success(text)
}

// Icons
var Icon = struct {
	Ok      string
	Err     string
	Info    string
	Warn    string
	Bullet  string
	Link    string
	Script  string
	Upgrade string
	Install string
	Remove  string
	Skip    string
	Sync    string
	Owl     string
}{
	Ok:      success("+"),
	Err:     errColor("-"),
	Info:    primary("i"),
	Warn:    warning("!"),
	Bullet:  muted("•"),
	Link:    primary("link"),
	Script:  primary("script"),
	Upgrade: warning("upgrade"),
	Install: success("install"),
	Remove:  errColor("remove"),
	Skip:    muted("skip"),
	Sync:    primary("sync"),
	Owl:     "",
}

// UI provides terminal user interface functions
type UI struct{}

// NewUI creates a new UI instance
func NewUI() *UI {
	return &UI{}
}

// Header displays the application header with optional mode
func (u *UI) Header(mode string) {
	fmt.Println()
	if mode != "" {
		var badge string
		if mode == "dry-run" {
			badge = color.New(color.BgYellow, color.FgBlack).Sprint(" Dry run ")
		} else {
			badge = color.New(color.BgBlue, color.FgWhite).Sprintf(" %s ", mode)
		}
		fmt.Printf(" %s \n", badge)
	}
	fmt.Println()
}

// Overview displays system overview information
func (u *UI) Overview(stats types.HostStats) {
	fmt.Printf("%s     %s\n", muted("host:"), stats.Host)
	fmt.Printf("%s %d\n", muted("packages:"), stats.Packages)
	fmt.Println()
	fmt.Println(warning(":::::::::::::::"))
	fmt.Println()
}

// InstallHeader displays the installation section header
func (u *UI) InstallHeader() {
	fmt.Println(primary("Installing:"))
}

// Info displays an info message
func (u *UI) Info(text string) {
	fmt.Printf("%s %s\n", Icon.Info, info(text))
}

// Ok displays a success message
func (u *UI) Ok(text string) {
	fmt.Printf("%s %s\n", Icon.Ok, success(text))
}

// Err displays an error message
func (u *UI) Err(text string) {
	fmt.Fprintf(os.Stderr, "%s %s\n", Icon.Err, errColor(text))
}

// Warn displays a warning message
func (u *UI) Warn(text string) {
	fmt.Printf("%s %s\n", Icon.Warn, warning(text))
}

// List displays a formatted list of items
func (u *UI) List(items []string, options types.ListOptions) {
	prefix := ""
	if options.Indent {
		prefix = "  "
	}

	colorFunc := func(s string) string { return accent(s) }
	if options.Color != nil {
		colorFunc = options.Color
	}

	for i, item := range items {
		var marker string
		if options.Numbered {
			marker = muted(fmt.Sprintf("%d.", i+1))
		} else {
			marker = Icon.Bullet
		}
		fmt.Printf("%s%s %s\n", prefix, marker, colorFunc(item))
	}
}

// PackageInstallProgress displays package installation progress
func (u *UI) PackageInstallProgress(packageName string, hasDotfiles bool, streamMode bool, packageEntry *types.ConfigEntry) {
	sourcePrefix := ""
	if packageEntry != nil {
		sourcePrefix = u.formatPackageSource(packageEntry)
	}

	fmt.Printf("%s%s %s\n", sourcePrefix, primary(packageName), muted("->"))

	if !streamMode {
		// Show package installation progress
		fmt.Printf("  Package - %s", muted("installing..."))
		time.Sleep(time.Duration(constants.PackageInstallDelay) * time.Millisecond)
		fmt.Printf("\r\033[K  Package - %s\n", success("installed"))

		// Show dotfiles installation if needed
		if hasDotfiles {
			fmt.Printf("  Dotfiles - %s", muted("installing..."))
			time.Sleep(time.Duration(constants.DotfilesInstallDelay) * time.Millisecond)
			fmt.Printf("\r\033[K  Dotfiles - %s\n", success("installed"))
		}

		fmt.Println()
	}
}

// PackageInstallComplete marks package installation as complete
func (u *UI) PackageInstallComplete(packageName string, hasDotfiles bool) {
	if hasDotfiles {
		fmt.Printf("\r\033[K  Dotfiles - %s\n", success("installed"))
	}
	fmt.Println()
}

// Success displays a success message with spacing
func (u *UI) Success(text string) {
	fmt.Println()
	fmt.Println(success(text))
	fmt.Println()
}

// Error displays an error message with spacing
func (u *UI) Error(text string) {
	fmt.Println()
	fmt.Fprintf(os.Stderr, "%s\n", errColor(text))
	fmt.Println()
}

// Celebration displays a celebration message
func (u *UI) Celebration(text string) {
	fmt.Println()
	fmt.Println(success(text))
	fmt.Println()
}

// FormatPackageSource formats the package source information (exported version)
func (u *UI) FormatPackageSource(entry *types.ConfigEntry) string {
	return u.formatPackageSource(entry)
}

// formatPackageSource formats the package source information
func (u *UI) formatPackageSource(entry *types.ConfigEntry) string {
	if entry.SourceType == "" {
		return ""
	}

	switch entry.SourceType {
	case "host":
		// Extract hostname from path like ~/.owl/hosts/hostname.owl
		hostMatch := ""
		if entry.SourceFile != "" {
			parts := strings.Split(entry.SourceFile, "/")
			for i, part := range parts {
				if part == "hosts" && i+1 < len(parts) {
					hostMatch = strings.TrimSuffix(parts[i+1], ".owl")
					break
				}
			}
		}
		if hostMatch == "" {
			hostMatch = "unknown"
		}
		return fmt.Sprintf("%s %s: ", info("@host"), warning(hostMatch))
	case "group":
		groupName := entry.GroupName
		if groupName == "" {
			groupName = "unknown"
		}
		return fmt.Sprintf("%s %s: ", info("@group"), warning(groupName))
	case "main":
		return ""
	default:
		return ""
	}
}

// Spinner represents a loading spinner
type Spinner struct {
	text      string
	enabled   bool
	color     func(string) string
	stopped   bool
	startTime time.Time
	mu        sync.Mutex
	done      chan bool
	finalizer func(duration time.Duration)
}

// NewSpinner creates a new spinner instance
func NewSpinner(text string, options types.SpinnerOptions) *Spinner {
	colorFunc := func(s string) string { return primary(s) }
	if options.Color != nil {
		colorFunc = options.Color
	}

	s := &Spinner{
		text:      text,
		enabled:   options.Enabled,
		color:     colorFunc,
		startTime: time.Now(),
		done:      make(chan bool),
	}

	if s.enabled {
		go s.animate()
	}

	return s
}

// animate runs the spinner animation
func (s *Spinner) animate() {
	frameIndex := 0
	ticker := time.NewTicker(time.Duration(constants.SpinnerFrameInterval) * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			s.mu.Lock()
			if s.stopped {
				s.mu.Unlock()
				return
			}

			frame := constants.SpinnerFrames[frameIndex]
			frameIndex = (frameIndex + 1) % len(constants.SpinnerFrames)

			// Clear the line first, then print new content
			fmt.Print("\r\033[K")

			// Check if this is a package installation or dotfiles spinner for proper indentation
			if strings.Contains(s.text, "Package - installing") ||
				strings.Contains(s.text, "Dotfiles - checking") ||
				strings.Contains(s.text, "Dotfiles - syncing") {
				fmt.Printf("  %s %s", s.color(frame), s.text)
			} else {
				fmt.Printf("%s %s", s.color(frame), s.color(s.text))
			}
			s.mu.Unlock()
		}
	}
}

// SetFinalizer sets a custom finalizer function for the spinner
func (s *Spinner) SetFinalizer(finalizer func(duration time.Duration)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.finalizer = finalizer
}

// Stop stops the spinner with a success message
func (s *Spinner) Stop(suffix string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.stopped {
		return
	}

	s.stopped = true
	if s.enabled {
		close(s.done)
	}

	duration := time.Since(s.startTime)

	// Use custom finalizer if set
	if s.finalizer != nil {
		s.finalizer(duration)
		return
	}

	timing := muted(fmt.Sprintf("(%dms)", duration.Milliseconds()))
	message := ""
	if suffix != "" {
		message = fmt.Sprintf(" %s", suffix)
	}

	if !s.enabled {
		finalMessage := s.text
		if suffix != "" {
			finalMessage = fmt.Sprintf("%s %s", s.text, info(suffix))
		}
		fmt.Printf("%s %s\n", Icon.Ok, success(finalMessage))
		return
	}

	// For package installs, show "Package - installed" format
	if strings.Contains(s.text, "Package - installing") {
		fmt.Printf("\r\033[K  Package - %s %s%s\n", success("installed"), timing, message)
	} else if strings.Contains(s.text, "Dotfiles - checking") {
		fmt.Printf("\r\033[K  Dotfiles - %s %s%s\n", success("up to date"), timing, message)
	} else if strings.Contains(s.text, "Dotfiles - syncing") {
		fmt.Printf("\r\033[K  Dotfiles - %s %s%s\n", success("synced"), timing, message)
	} else {
		fmt.Printf("\r\033[K%s %s %s%s\n", Icon.Ok, success(s.text), timing, message)
	}
}

// Fail stops the spinner with an error message
func (s *Spinner) Fail(reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.stopped {
		return
	}

	s.stopped = true
	if s.enabled {
		close(s.done)
	}

	message := ""
	if reason != "" {
		message = fmt.Sprintf(" %s", muted(reason))
	}

	if !s.enabled {
		finalMessage := s.text
		if reason != "" {
			finalMessage = fmt.Sprintf("%s %s", s.text, info(reason))
		}
		fmt.Fprintf(os.Stderr, "%s %s\n", Icon.Err, errColor(finalMessage))
		return
	}

	// For package installs, show "Package - failed" format
	if strings.Contains(s.text, "Package - installing") {
		fmt.Printf("\r\033[K  Package - %s%s\n", errColor("failed"), message)
	} else if strings.Contains(s.text, "Dotfiles - checking") || strings.Contains(s.text, "Dotfiles - syncing") {
		fmt.Printf("\r\033[K  Dotfiles - %s%s\n", errColor("failed"), message)
	} else {
		fmt.Printf("\r\033[K%s %s%s\n", Icon.Err, errColor(s.text), message)
	}
}

// ProgressBar represents a visual progress bar
type ProgressBar struct {
	text    string
	total   int64
	current int64
	width   int
	enabled bool
	color   func(string) string
	mu      sync.Mutex
}

// NewProgressBar creates a new progress bar
func NewProgressBar(text string, total int64, options types.ProgressBarOptions) *ProgressBar {
	colorFunc := func(s string) string { return primary(s) }
	if options.Color != nil {
		colorFunc = options.Color
	}

	width := 40
	if options.Width > 0 {
		width = options.Width
	}

	return &ProgressBar{
		text:    text,
		total:   total,
		current: 0,
		width:   width,
		enabled: options.Enabled,
		color:   colorFunc,
	}
}

// Update updates the progress bar
func (pb *ProgressBar) Update(current int64) {
	pb.mu.Lock()
	defer pb.mu.Unlock()

	pb.current = current
	if !pb.enabled {
		return
	}

	percentage := float64(pb.current) / float64(pb.total) * 100
	if percentage > 100 {
		percentage = 100
	}

	// Calculate filled blocks
	filled := int(float64(pb.width) * percentage / 100)

	// Build progress bar
	bar := "["
	for i := 0; i < pb.width; i++ {
		if i < filled {
			bar += "="
		} else {
			bar += " "
		}
	}
	bar += "]"

	// Clear line and print progress
	fmt.Print("\r\033[K")
	if pb.total > 0 {
		fmt.Printf("  %s %s %.1f%%", pb.color(bar), pb.text, percentage)
	} else {
		fmt.Printf("  %s %s", pb.color(bar), pb.text)
	}
}

// UpdateWithText updates the progress bar with custom text
func (pb *ProgressBar) UpdateWithText(current int64, text string) {
	pb.mu.Lock()
	pb.text = text
	pb.mu.Unlock()
	pb.Update(current)
}

// Finish completes the progress bar
func (pb *ProgressBar) Finish(message string) {
	pb.mu.Lock()
	defer pb.mu.Unlock()

	if !pb.enabled {
		fmt.Printf("%s %s\n", Icon.Ok, success(message))
		return
	}

	// Show completed bar
	bar := "["
	for i := 0; i < pb.width; i++ {
		bar += "="
	}
	bar += "]"

	fmt.Print("\r\033[K")
	fmt.Printf("  %s %s\n", pb.color(bar), success(message))
}

// Fail shows the progress bar as failed
func (pb *ProgressBar) Fail(reason string) {
	pb.mu.Lock()
	defer pb.mu.Unlock()

	if !pb.enabled {
		fmt.Fprintf(os.Stderr, "%s %s\n", Icon.Err, errColor(reason))
		return
	}

	fmt.Print("\r\033[K")
	fmt.Printf("  %s %s\n", Icon.Err, errColor(reason))
}

// Update updates the spinner text
func (s *Spinner) Update(newText string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if newText != "" {
		s.text = newText
	}
}

// LegacyRenderer provides legacy TypeScript-style UI rendering
type LegacyRenderer struct {
	ui      *UI
	noColor bool
}

// NewLegacyRenderer creates a new legacy renderer instance
func NewLegacyRenderer() *LegacyRenderer {
	return &LegacyRenderer{
		ui:      NewUI(),
		noColor: false,
	}
}

// NoColor indicates if color output is disabled
func (r *LegacyRenderer) NoColor() bool {
	return r.noColor
}

// SetNoColor sets the no-color mode
func (r *LegacyRenderer) SetNoColor(noColor bool) {
	r.noColor = noColor
}

// Title displays the application title
func (r *LegacyRenderer) Title() {
	r.ui.Header("")
}

// StartSpinner starts a spinner with the given text
func (r *LegacyRenderer) StartSpinner(text string) *Spinner {
	return NewSpinner(text, types.SpinnerOptions{Enabled: true})
}

// StopSpinner stops the spinner with a message
func (r *LegacyRenderer) StopSpinner(spinner *Spinner, message string) {
	spinner.Stop(message)
}

// HostInfo displays host information
func (r *LegacyRenderer) HostInfo(hostname string, packageCount int) {
	fmt.Printf("%s     %s\n", muted("host:"), hostname)
	fmt.Printf("%s %d\n", muted("packages:"), packageCount)
	fmt.Println()
}

// Separator displays a separator line
func (r *LegacyRenderer) Separator() {
	fmt.Println(warning(":::::::::::::::"))
	fmt.Println()
}

// MaintenanceHeader displays the maintenance section header
func (r *LegacyRenderer) MaintenanceHeader() {
	fmt.Println(primary("Maintenance:"))
}

// ConfigHeader displays the configuration section header
func (r *LegacyRenderer) ConfigHeader() {
	fmt.Println(primary("Configuration:"))
}

// PackageHeader displays a package header
func (r *LegacyRenderer) PackageHeader(packageName string) {
	fmt.Printf("%s %s\n", primary(packageName), muted("->"))
}

// DotfilesStatus displays dotfiles status
func (r *LegacyRenderer) DotfilesStatus(status, timing string) {
	fmt.Printf("  Dotfiles - %s %s\n", success(status), muted(timing))
}

// Footer displays the footer
func (r *LegacyRenderer) Footer() {
	fmt.Println()
}

// GetUserConfirmation prompts the user for confirmation and returns their response
func (u *UI) GetUserConfirmation(prompt string) (string, error) {
	fmt.Print(prompt)

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(input), nil
}

// ConfirmAction asks the user to confirm an action with a yes/no prompt
func (u *UI) ConfirmAction(message string) (bool, error) {
	response, err := u.GetUserConfirmation(fmt.Sprintf("%s (y/N): ", message))
	if err != nil {
		return false, err
	}

	response = strings.ToLower(response)
	return response == "y" || response == "yes", nil
}

// ShowHelp displays the application help information
func (u *UI) ShowHelp() {
	fmt.Println("Owl Package Manager")
	fmt.Println("A modern AUR helper and package manager for Arch Linux with config management and setup script automation.")

	fmt.Printf("\n%s\n", color.New(color.Bold).Sprint("Usage:"))
	fmt.Println("  owl <command> [options]")

	fmt.Printf("\n%s\n", color.New(color.Bold).Sprint("Config Management Commands:"))
	u.List([]string{
		"apply          Install packages, copy configs, and run setup scripts",
		"dry-run, dr    Preview what would be done without making changes",
		"dots           Check and sync only dotfiles configurations",
		"uninstall      Remove all managed packages and configs",
	}, types.ListOptions{
		Indent: true,
		Color:  func(s string) string { return color.New(color.FgBlue).Sprint(s) },
	})

	fmt.Printf("\n%s\n", color.New(color.Bold).Sprint("Package Manager Commands:"))
	u.List([]string{
		"search, s      Search for packages in repositories and AUR",
		"install, i, S  Install packages from repositories or AUR",
		"upgrade, u     Upgrade all packages to latest versions",
		"info, Si       Show detailed information about a package",
		"query, q, Q    Query installed packages",
	}, types.ListOptions{
		Indent: true,
		Color:  func(s string) string { return color.New(color.FgMagenta).Sprint(s) },
	})

	fmt.Printf("\n%s\n", color.New(color.Bold).Sprint("General Commands:"))
	u.List([]string{
		"gendb          Generate VCS database for development packages",
		"help, --help   Show this help message",
		"version, -v    Show version information",
	}, types.ListOptions{
		Indent: true,
		Color:  func(s string) string { return color.New(color.FgWhite).Sprint(s) },
	})

	fmt.Println()
}

// ShowVersion displays the application version
func (u *UI) ShowVersion(version string) {
	fmt.Printf("owl version %s\n", version)
}

// SystemMessage displays a system message with green brackets and white text
func (u *UI) SystemMessage(text string) {
	fmt.Printf("%s %s %s\n", success("::"), white(text), success("::"))
}
