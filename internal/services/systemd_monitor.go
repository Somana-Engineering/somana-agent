package services

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"sprinter-agent/internal/config"
	"sprinter-agent/internal/generated"
)

// SystemdMonitorService handles monitoring and reporting systemd services
type SystemdMonitorService struct {
	config   *config.Config
	client   *generated.ClientWithResponses
	hostRid  string
	stopChan chan bool
}

// NewSystemdMonitorService creates a new systemd monitor service
func NewSystemdMonitorService(cfg *config.Config, apiClient *generated.ClientWithResponses, hostRid string) *SystemdMonitorService {
	return &SystemdMonitorService{
		config:   cfg,
		client:   apiClient,
		hostRid:  hostRid,
		stopChan: make(chan bool),
	}
}

// Start begins monitoring systemd services and reporting them periodically
func (s *SystemdMonitorService) Start() error {
	if s.hostRid == "" {
		log.Println("Host RID not set - skipping systemd monitoring")
		return nil
	}

	// Start monitoring goroutine
	go s.monitorLoop()

	log.Printf("Systemd monitoring service started for host RID: %s", s.hostRid)
	return nil
}

// Stop stops the monitoring process
func (s *SystemdMonitorService) Stop() {
	if s.hostRid != "" {
		close(s.stopChan)
		log.Println("Systemd monitoring service stopped")
	}
}

// monitorLoop runs the periodic monitoring loop
func (s *SystemdMonitorService) monitorLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	// Run immediately on start
	s.reportSystemdServices()

	for {
		select {
		case <-ticker.C:
			s.reportSystemdServices()
		case <-s.stopChan:
			return
		}
	}
}

// reportSystemdServices reads systemd services and reports them to the API
func (s *SystemdMonitorService) reportSystemdServices() {
	services, err := s.getSystemdServices()
	if err != nil {
		log.Printf("ERROR: Failed to get systemd services: %v", err)
		// Check if it's a permission error
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			if exitError.ExitCode() == 1 {
				log.Printf("ERROR: systemctl command failed with exit code 1 - this may indicate permission issues")
				log.Printf("ERROR: Current user: %s, UID: %d, GID: %d", os.Getenv("USER"), os.Getuid(), os.Getgid())
				if stderr := string(exitError.Stderr); stderr != "" {
					log.Printf("ERROR: systemctl stderr: %s", stderr)
				}
				log.Printf("ERROR: Suggestion: Ensure the service is running with appropriate permissions (may need to run as root or add user to systemd-journal group)")
			}
		}
		// Check for permission denied errors
		if errors.Is(err, os.ErrPermission) || strings.Contains(err.Error(), "permission denied") {
			log.Printf("ERROR: Permission denied accessing systemd - ensure the process has appropriate permissions")
		}
		// Send empty list if systemd doesn't exist or fails
		services = []generated.SystemdUnit{}
	}

	reqBody := generated.SystemdServicesRequest{
		Services: services,
	}

	ctx := context.Background()
	resp, err := s.client.PutApiV1HostsHostRidSystemdServicesWithResponse(ctx, generated.HostRid(s.hostRid), reqBody)
	if err != nil {
		log.Printf("Failed to report systemd services: %v", err)
		return
	}

	if resp.StatusCode() != http.StatusOK {
		log.Printf("Failed to report systemd services: status %d", resp.StatusCode())
		return
	}

	log.Printf("Reported %d systemd services successfully", len(services))
}

// getSystemdServices reads systemd services from the system
func (s *SystemdMonitorService) getSystemdServices() ([]generated.SystemdUnit, error) {
	// Check if systemctl exists
	if _, err := exec.LookPath("systemctl"); err != nil {
		log.Println("systemctl not found - returning empty list")
		return []generated.SystemdUnit{}, nil
	}

	// Run systemctl list-units command
	cmd := exec.Command("systemctl", "list-units", "--type=service", "--no-pager", "--no-legend")
	
	// Capture both stdout and stderr for better error reporting
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	
	output, err := cmd.Output()
	if err != nil {
		// Get stderr output if available
		stderrStr := strings.TrimSpace(stderr.String())
		
		// Check for specific error types
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			exitCode := exitError.ExitCode()
			errMsg := fmt.Sprintf("systemctl command failed with exit code %d", exitCode)
			if stderrStr != "" {
				errMsg += fmt.Sprintf(": %s", stderrStr)
			}
			
			// Check for permission-related exit codes
			if exitCode == 1 {
				errMsg += " (likely permission issue - systemctl may require elevated privileges)"
			}
			
			return nil, fmt.Errorf("failed to run systemctl: %s: %w", errMsg, err)
		}
		
		// Check for permission denied
		if errors.Is(err, os.ErrPermission) || strings.Contains(err.Error(), "permission denied") {
			return nil, fmt.Errorf("permission denied running systemctl (current user: %s, UID: %d): %w", os.Getenv("USER"), os.Getuid(), err)
		}
		
		// Generic error
		if stderrStr != "" {
			return nil, fmt.Errorf("failed to run systemctl (stderr: %s): %w", stderrStr, err)
		}
		return nil, fmt.Errorf("failed to run systemctl: %w", err)
	}

	// Parse the output
	// systemctl output format: UNIT LOAD ACTIVE SUB DESCRIPTION
	// Fields are separated by multiple spaces
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	services := make([]generated.SystemdUnit, 0, len(lines))

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Split by multiple spaces (at least 2 spaces)
		// This handles the systemctl output format better
		parts := strings.Fields(line)
		if len(parts) < 5 {
			// Try to parse with at least 4 fields (description might be empty)
			if len(parts) >= 4 {
				services = append(services, generated.SystemdUnit{
					Unit:        parts[0],
					Load:        parts[1],
					Active:      parts[2],
					Sub:         parts[3],
					Description: strings.Join(parts[4:], " "),
				})
			}
			continue
		}

		// Parse the fields
		unit := parts[0]
		load := parts[1]
		active := parts[2]
		sub := parts[3]
		description := strings.Join(parts[4:], " ")

		services = append(services, generated.SystemdUnit{
			Unit:        unit,
			Load:        load,
			Active:      active,
			Sub:         sub,
			Description: description,
		})
	}

	return services, nil
}

