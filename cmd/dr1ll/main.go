package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/M1z23R/dr1ll/internal/client"
	"github.com/M1z23R/dr1ll/internal/config"
	"golang.org/x/sys/windows/svc"
)

type DrillService struct {
	RunFunc func()
}

func (m *DrillService) Execute(args []string, r <-chan svc.ChangeRequest, s chan<- svc.Status) (bool, uint32) {
	s <- svc.Status{State: svc.StartPending}
	s <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}

	go m.RunFunc()
	for c := range r {
		switch c.Cmd {
		case svc.Interrogate:
			s <- c.CurrentStatus
		case svc.Stop, svc.Shutdown:
			log.Println("Service stopping")
			return false, 0
		}
	}
	return false, 0
}

func main() {
	isService, err := svc.IsWindowsService()
	if err != nil {
		log.Fatal(err)
	}
	if isService {
		svc.Run("DrillService", &DrillService{RunFunc: startCommand})
		return
	}

	if len(os.Args) < 2 {
		showUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "start":
		startCommand()
	case "config":
		configCommand()
	case "help", "-h", "--help":
		showUsage()
	default:
		fmt.Printf("Unknown command: %s\n\n", command)
		showUsage()
		os.Exit(1)
	}
}

func showUsage() {
	fmt.Println("dr1ll - HTTP tunnel client")
	fmt.Println("")
	fmt.Println("Usage:")
	fmt.Println("  dr1ll start [options]    Start the tunnel client")
	fmt.Println("  dr1ll config <command>   Manage configuration")
	fmt.Println("  dr1ll help              Show this help message")
	fmt.Println("")
	fmt.Println("Start options:")
	fmt.Println("  -port <port>            Local port to forward (default: 3000)")
	fmt.Println("  -server <url>           Override tunnel server URL")
	fmt.Println("  -token <token>          Override authentication token")
	fmt.Println("  -subdomain <name>       Request specific subdomain")
	fmt.Println("")
	fmt.Println("Config commands:")
	fmt.Println("  dr1ll config set-server <url>    Set tunnel server URL")
	fmt.Println("  dr1ll config set-token <token>   Set authentication token")
	fmt.Println("  dr1ll config show                Show current configuration")
}

func startCommand() {
	startArgs := os.Args[2:]

	fs := flag.NewFlagSet("start", flag.ExitOnError)
	port := fs.Int("port", 3000, "Local port to forward requests to")
	serverURL := fs.String("server", "", "Tunnel server URL (overrides config)")
	token := fs.String("token", "", "Authentication token (overrides config)")
	subdomain := fs.String("subdomain", "", "Request specific subdomain")

	fs.Parse(startArgs)

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	finalServerURL := cfg.TunnelServer
	if *serverURL != "" {
		finalServerURL = *serverURL
	}

	finalToken := cfg.Token
	if *token != "" {
		finalToken = *token
	}

	if finalServerURL == "" {
		log.Fatal("No tunnel server URL configured. Use 'dr1ll config set-server <url>' to set one.")
	}

	if finalToken == "" {
		log.Fatal("No authentication token configured. Use 'dr1ll config set-token <token>' to set one.")
	}

	fmt.Printf("🏠 Starting tunnel client for localhost:%d\n", *port)
	fmt.Printf("🌐 Server: %s\n", finalServerURL)

	client := client.NewClient(finalServerURL, finalToken, *port)
	if *subdomain != "" {
		client.SetRequestedSubdomain(*subdomain)
		fmt.Printf("🎯 Requesting subdomain: %s\n", *subdomain)
	}
	if err := client.Run(); err != nil {
		log.Fatal(err)
	}

	fmt.Println("👋 Tunnel closed. Goodbye!")
}

func configCommand() {
	if len(os.Args) < 3 {
		fmt.Println("Config command required. Available commands:")
		fmt.Println("  set-server <url>    Set tunnel server URL")
		fmt.Println("  set-token <token>   Set authentication token")
		fmt.Println("  show               Show current configuration")
		os.Exit(1)
	}

	subcommand := os.Args[2]

	switch subcommand {
	case "set-server":
		if len(os.Args) < 4 {
			fmt.Println("Usage: dr1ll config set-server <url>")
			os.Exit(1)
		}
		serverURL := os.Args[3]
		if err := config.SetServer(serverURL); err != nil {
			log.Fatalf("Failed to set server URL: %v", err)
		}
		fmt.Printf("✅ Tunnel server set to: %s\n", serverURL)

	case "set-token":
		if len(os.Args) < 4 {
			fmt.Println("Usage: dr1ll config set-token <token>")
			os.Exit(1)
		}
		token := os.Args[3]
		if err := config.SetToken(token); err != nil {
			log.Fatalf("Failed to set token: %v", err)
		}
		fmt.Println("✅ Authentication token updated")

	case "show":
		cfg, err := config.Load()
		if err != nil {
			log.Fatalf("Failed to load configuration: %v", err)
		}

		configPath, _ := config.GetConfigPath()
		fmt.Printf("Configuration file: %s\n", configPath)
		fmt.Printf("Tunnel server: %s\n", cfg.TunnelServer)
		if cfg.Token != "" {
			fmt.Printf("Token: %s***\n", cfg.Token[:min(len(cfg.Token), 8)])
		} else {
			fmt.Println("Token: (not set)")
		}

	default:
		fmt.Printf("Unknown config command: %s\n", subcommand)
		os.Exit(1)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
