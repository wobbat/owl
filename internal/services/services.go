package services

import (
	"fmt"
	"os/exec"
	"strings"

	"owl/internal/types"
	"owl/internal/ui"
)

// ServiceStatus represents the status of a systemd service
type ServiceStatus struct {
	Name    string
	Enabled bool
	Active  bool
}

// checkServiceStatus checks if a systemd service is enabled and active
func checkServiceStatus(serviceName string) (ServiceStatus, error) {
	status := ServiceStatus{
		Name:    serviceName,
		Enabled: false,
		Active:  false,
	}

	// Check if service is enabled
	cmd := exec.Command("systemctl", "is-enabled", serviceName)
	output, err := cmd.Output()
	if err == nil {
		status.Enabled = strings.TrimSpace(string(output)) == "enabled"
	}

	// Check if service is active
	cmd = exec.Command("systemctl", "is-active", serviceName)
	output, err = cmd.Output()
	if err == nil {
		status.Active = strings.TrimSpace(string(output)) == "active"
	}

	return status, nil
}

// enableService enables a systemd service
func enableService(serviceName string) error {
	cmd := exec.Command("sudo", "systemctl", "enable", serviceName)
	return cmd.Run()
}

// startService starts a systemd service
func startService(serviceName string) error {
	cmd := exec.Command("sudo", "systemctl", "start", serviceName)
	return cmd.Run()
}

// enableAndStartService enables and starts a systemd service
func enableAndStartService(serviceName string) error {
	status, err := checkServiceStatus(serviceName)
	if err != nil {
		return fmt.Errorf("failed to check service status: %w", err)
	}

	if !status.Enabled {
		if err := enableService(serviceName); err != nil {
			return fmt.Errorf("failed to enable service %s: %w", serviceName, err)
		}
	}

	if !status.Active {
		if err := startService(serviceName); err != nil {
			return fmt.Errorf("failed to start service %s: %w", serviceName, err)
		}
	}

	return nil
}

// ProcessServices manages services
func ProcessServices(services []string, dryRun bool, globalUI *ui.UI) error {
	if len(services) == 0 {
		return nil
	}

	if dryRun {
		fmt.Println("Services to manage:")
		for _, serviceName := range services {
			fmt.Printf("  %s Would manage service: %s\n", ui.Icon.Ok, serviceName)
		}
		fmt.Println()
		return nil
	}

	fmt.Println("Service management:")

	spinner := ui.NewSpinner(fmt.Sprintf("Managing %d services", len(services)), types.SpinnerOptions{Enabled: true})

	successCount := 0
	errorCount := 0

	for _, serviceName := range services {
		spinner.Update(fmt.Sprintf("Managing service %s...", serviceName))

		err := enableAndStartService(serviceName)
		if err != nil {
			errorCount++
			globalUI.Err(fmt.Sprintf("Failed to manage service %s: %v", serviceName, err))
		} else {
			successCount++
		}
	}

	if errorCount == 0 {
		spinner.Stop(fmt.Sprintf("%d services managed successfully", successCount))
	} else {
		spinner.Fail(fmt.Sprintf("%d failed, %d succeeded", errorCount, successCount))
	}

	fmt.Println()
	return nil
}
