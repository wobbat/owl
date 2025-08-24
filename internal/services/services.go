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

// ServiceAction represents what action was taken on a service
type ServiceAction struct {
	Name       string
	WasEnabled bool
	WasActive  bool
	Enabled    bool
	Started    bool
	NoAction   bool
	Error      error
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

// enableAndStartService enables and starts a systemd service if needed
func enableAndStartService(serviceName string) ServiceAction {
	action := ServiceAction{
		Name: serviceName,
	}

	status, err := checkServiceStatus(serviceName)
	if err != nil {
		action.Error = fmt.Errorf("failed to check service status: %w", err)
		return action
	}

	action.WasEnabled = status.Enabled
	action.WasActive = status.Active

	// If service is already enabled and active, no action needed
	if status.Enabled && status.Active {
		action.NoAction = true
		return action
	}

	if !status.Enabled {
		if err := enableService(serviceName); err != nil {
			action.Error = fmt.Errorf("failed to enable service %s: %w", serviceName, err)
			return action
		}
		action.Enabled = true
	}

	if !status.Active {
		if err := startService(serviceName); err != nil {
			action.Error = fmt.Errorf("failed to start service %s: %w", serviceName, err)
			return action
		}
		action.Started = true
	}

	return action
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

	// Check all services and collect their actions
	var actions []ServiceAction
	var servicesNeedingAction []ServiceAction

	for _, serviceName := range services {
		action := enableAndStartService(serviceName)
		actions = append(actions, action)
		if action.Error != nil || !action.NoAction {
			servicesNeedingAction = append(servicesNeedingAction, action)
		}
	}

	// If no services need action, don't show anything
	if len(servicesNeedingAction) == 0 {
		return nil
	}

	fmt.Println("Service management:")

	spinner := ui.NewSpinner(fmt.Sprintf("Managing %d services", len(servicesNeedingAction)), types.SpinnerOptions{Enabled: true})

	successCount := 0
	errorCount := 0

	for _, action := range servicesNeedingAction {
		if action.Error != nil {
			errorCount++
			globalUI.Err(fmt.Sprintf("Failed to manage service %s: %v", action.Name, action.Error))
		} else {
			successCount++
			actionMsg := ""
			if action.Enabled && action.Started {
				actionMsg = "enabled and started"
			} else if action.Enabled {
				actionMsg = "enabled"
			} else if action.Started {
				actionMsg = "started"
			}
			spinner.Update(fmt.Sprintf("Service %s %s", action.Name, actionMsg))
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
