package services

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"sprinter-agent/internal/client"
	"sprinter-agent/internal/config"
)

// HostRegistrationService handles registration with main Somana instance
type HostRegistrationService struct {
	config   *config.Config
	client   *client.ClientWithResponses
	hostRid  string
	stopChan chan bool
}

// NewHostRegistrationService creates a new host registration service
func NewHostRegistrationService(cfg *config.Config) *HostRegistrationService {
	log.Printf("Creating host registration service with URL: %s", cfg.HostRegistration.SprinterURL)
	
	httpClient := &http.Client{Timeout: 10 * time.Second}
	apiClient, err := client.NewClientWithResponses(cfg.HostRegistration.SprinterURL, client.WithHTTPClient(httpClient))
	if err != nil {
		log.Printf("Warning: failed to create client: %v", err)
	} else {
		log.Printf("Successfully created API client")
	}

	return &HostRegistrationService{
		config:   cfg,
		client:   apiClient,
		stopChan: make(chan bool),
	}
}

// Start begins the host registration and heartbeat process
func (s *HostRegistrationService) Start() error {
	if s.config.HostRegistration.SprinterURL == "" {
		log.Println("Host registration not configured - skipping")
		return nil
	}

	// Get system information
	hostname, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("failed to get hostname: %w", err)
	}

	ipAddress, err := s.getLocalIP()
	if err != nil {
		return fmt.Errorf("failed to get IP address: %w", err)
	}

	osVersion, err := s.getOSVersion()
	if err != nil {
		log.Printf("Warning: failed to get OS version: %v", err)
		osVersion = "Unknown"
	}

	// Register with main Somana instance
	if err := s.registerHost(hostname, ipAddress, osVersion); err != nil {
		return fmt.Errorf("failed to register host: %w", err)
	}

	// Start heartbeat goroutine
	go s.startHeartbeat()

	log.Printf("Host registration started - Host RID: %s", s.hostRid)
	return nil
}

// GetHostRid returns the host RID
func (s *HostRegistrationService) GetHostRid() string {
	return s.hostRid
}

// GetClient returns the API client
func (s *HostRegistrationService) GetClient() *client.ClientWithResponses {
	return s.client
}

// Stop stops the heartbeat process
func (s *HostRegistrationService) Stop() {
	if s.config.HostRegistration.SprinterURL != "" {
		close(s.stopChan)
		log.Println("Host registration stopped")
	}
}

// registerHost registers this host with the main Somana instance
func (s *HostRegistrationService) registerHost(hostname, ipAddress, osVersion string) error {
	ctx := context.Background()

	log.Printf("Attempting to register host: %s (%s) - %s", hostname, ipAddress, osVersion)

	// Check if we have a host RID in config
	if s.config.HostRegistration.HostRid != "" {
		hostRid := s.config.HostRegistration.HostRid

		log.Printf("Checking if host RID %s exists", hostRid)
		
		// Check if host exists with this RID
		resp, err := s.client.GetApiV1HostsHostRidWithResponse(ctx, client.HostRid(hostRid))
		if err != nil {
			log.Printf("Failed to check host existence: %v", err)
			return fmt.Errorf("failed to check host existence: %w", err)
		}

		if resp.StatusCode() == http.StatusOK && resp.JSON200 != nil {
			// Host exists with this RID, use it
			s.hostRid = hostRid
			log.Printf("Found existing host with RID: %s", s.hostRid)
			return nil
		} else {
			log.Printf("Host with RID %s does not exist, will create new host", hostRid)
		}
	}

	// Get OS name from runtime
	osName := runtime.GOOS
	if osName == "darwin" {
		osName = "macOS"
	}

	// Register new host
	reqBody := client.HostCreateRequest{
		Hostname:  hostname,
		IpAddress: ipAddress,
		OsName:    osName,
		OsVersion: osVersion,
	}

	log.Printf("Sending registration request to: %s/api/v1/hosts", s.config.HostRegistration.SprinterURL)
	resp, err := s.client.PostApiV1HostsWithResponse(ctx, reqBody)
	if err != nil {
		log.Printf("Registration request failed: %v", err)
		return fmt.Errorf("failed to register host: %w", err)
	}

	log.Printf("Registration response status: %d", resp.StatusCode())
	if resp.StatusCode() != http.StatusCreated {
		log.Printf("Registration failed with status: %d", resp.StatusCode())
		return fmt.Errorf("registration failed with status: %d", resp.StatusCode())
	}

	if resp.JSON201 == nil {
		log.Printf("No host data in response")
		return fmt.Errorf("no host data in response")
	}

	s.hostRid = string(resp.JSON201.HostRid)
	s.config.HostRegistration.HostRid = s.hostRid

	// Save updated config
	if err := config.SaveConfig(s.config, "config/config.yaml"); err != nil {
		log.Printf("Warning: failed to save host RID to config: %v", err)
	}

	log.Printf("Successfully registered host with RID: %s", s.hostRid)
	return nil
}

// startHeartbeat starts the heartbeat process
func (s *HostRegistrationService) startHeartbeat() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := s.sendHeartbeat(); err != nil {
				log.Printf("Failed to send heartbeat: %v", err)
			}
		case <-s.stopChan:
			return
		}
	}
}

// sendHeartbeat sends a heartbeat to the main Somana instance
func (s *HostRegistrationService) sendHeartbeat() error {
	ctx := context.Background()
	
	// API changed: status field removed, server tracks last_heartbeat automatically
	reqBody := client.HostHeartbeatRequest{}

	resp, err := s.client.PostApiV1HostsHostRidHeartbeatWithResponse(ctx, client.HostRid(s.hostRid), reqBody)
	if err != nil {
		return fmt.Errorf("failed to send heartbeat: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return fmt.Errorf("heartbeat failed with status: %d", resp.StatusCode())
	}

	log.Printf("Heartbeat sent successfully")
	return nil
}

// getLocalIP gets the local IP address
func (s *HostRegistrationService) getLocalIP() (string, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return "", err
	}

	addrs, err := net.LookupIP(hostname)
	if err != nil {
		return "", err
	}

	for _, addr := range addrs {
		if !addr.IsLoopback() && addr.To4() != nil {
			return addr.String(), nil
		}
	}

	return "127.0.0.1", nil
}

// getOSVersion gets the OS version information
func (s *HostRegistrationService) getOSVersion() (string, error) {
	switch runtime.GOOS {
	case "linux":
		// Try to read /etc/os-release
		if data, err := os.ReadFile("/etc/os-release"); err == nil {
			lines := strings.Split(string(data), "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "PRETTY_NAME=") {
					version := strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), "\"")
					return version, nil
				}
			}
		}

		// Fallback to uname
		if output, err := exec.Command("uname", "-r").Output(); err == nil {
			return "Linux " + strings.TrimSpace(string(output)), nil
		}

		return "Linux", nil
	case "darwin":
		if output, err := exec.Command("sw_vers", "-productVersion").Output(); err == nil {
			return "macOS " + strings.TrimSpace(string(output)), nil
		}
		return "macOS", nil
	default:
		return runtime.GOOS, nil
	}
} 