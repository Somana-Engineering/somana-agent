package main

import (
	"flag"
	"log"
	"sync"
	"time"

	"sprinter-agent/internal/config"
	"sprinter-agent/internal/services"
)

func main() {
	// Parse command-line flags
	configPath := flag.String("config", "config/config.yaml", "Path to configuration file")
	flag.Parse()

	// Load configuration
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatal("Failed to load configuration:", err)
	}

	// Create host registration service
	hostRegService := services.NewHostRegistrationService(cfg)

	// Start host registration and heartbeat (runs in background, retries until successful)
	if err := hostRegService.Start(); err != nil {
		log.Printf("Warning: Failed to start host registration: %v", err)
	}

	// Wait a moment for host registration to potentially complete
	time.Sleep(2 * time.Second)

	// Start systemd monitoring service (will start once host is registered)
	// Check periodically if host is registered
	var systemdStarted sync.Once
	go func() {
		for {
			hostRid := hostRegService.GetHostRid()
			if hostRid != "" {
				systemdStarted.Do(func() {
					apiClient := hostRegService.GetClient()
					if apiClient != nil {
						systemdMonitor := services.NewSystemdMonitorService(cfg, apiClient, hostRid)
						if err := systemdMonitor.Start(); err != nil {
							log.Printf("Warning: Failed to start systemd monitoring: %v", err)
						} else {
							log.Printf("Systemd monitoring started for host RID: %s", hostRid)
						}
					}
				})
				return // Exit goroutine once monitoring is started
			}
			// Wait before checking again
			time.Sleep(5 * time.Second)
		}
	}()

	// Comment out Gin server for now - focus on host registration debugging
	/*
	// Create Gin router
	r := gin.Default()

	// Create host service that implements the generated ServerInterface
	hostService := services.NewHostService()

	// Register handlers using the generated code
	generated.RegisterHandlers(r, hostService)

	// Start server
	log.Println("Starting Sprinter Agent server on :9000")
	if err := r.Run(":9000"); err != nil {
		log.Fatal("Failed to start server:", err)
	}
	*/

	// Keep the process running for debugging
	log.Println("Host registration service started. Press Ctrl+C to exit.")
	select {}
} 