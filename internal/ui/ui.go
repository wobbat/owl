package ui

import (
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
		fmt.Printf("\r  Package - %s     \n", success("installed"))

		// Show dotfiles installation if needed
		if hasDotfiles {
			fmt.Printf("  Dotfiles - %s", muted("installing..."))
			time.Sleep(time.Duration(constants.DotfilesInstallDelay) * time.Millisecond)
			fmt.Printf("\r  Dotfiles - %s     \n", success("installed"))
		}

		fmt.Println()
	}
}

// PackageInstallComplete marks package installation as complete
func (u *UI) PackageInstallComplete(packageName string, hasDotfiles bool) {
	if hasDotfiles {
		fmt.Printf("  Dotfiles - %s     \n", success("installed"))
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

			// Check if this is a package installation or dotfiles spinner for proper indentation
			if strings.Contains(s.text, "Package - installing") ||
				strings.Contains(s.text, "Dotfiles - checking") ||
				strings.Contains(s.text, "Dotfiles - syncing") {
				fmt.Printf("\r  %s %s  ", s.color(frame), s.text)
			} else {
				fmt.Printf("\r%s %s  ", s.color(frame), s.color(s.text))
			}
			s.mu.Unlock()
		}
	}
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
		fmt.Printf("\r  Package - %s %s%s     \n", success("installed"), timing, message)
	} else if strings.Contains(s.text, "Dotfiles - checking") {
		fmt.Printf("\r  Dotfiles - %s %s%s     \n", success("up to date"), timing, message)
	} else if strings.Contains(s.text, "Dotfiles - syncing") {
		fmt.Printf("\r  Dotfiles - %s %s%s     \n", success("synced"), timing, message)
	} else {
		fmt.Printf("\r%s %s %s%s\n", Icon.Ok, success(s.text), timing, message)
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
		fmt.Printf("\r  Package - %s%s\n", errColor("failed"), message)
	} else if strings.Contains(s.text, "Dotfiles - checking") || strings.Contains(s.text, "Dotfiles - syncing") {
		fmt.Printf("\r  Dotfiles - %s%s\n", errColor("failed"), message)
	} else {
		fmt.Printf("\r%s %s%s\n", Icon.Err, errColor(s.text), message)
	}
}

// Update updates the spinner text
func (s *Spinner) Update(newText string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if newText != "" {
		s.text = newText
	}
}
