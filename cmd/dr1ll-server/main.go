package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/M1z23R/dr1ll/internal/config"
	"github.com/M1z23R/dr1ll/internal/server"
)

func main() {
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
	fmt.Println("dr1ll-server - HTTP tunnel server")
	fmt.Println("")
	fmt.Println("Usage:")
	fmt.Println("  dr1ll-server start [options]    Start the tunnel server")
	fmt.Println("  dr1ll-server config <command>   Manage configuration")
	fmt.Println("  dr1ll-server help              Show this help message")
	fmt.Println("")
	fmt.Println("Start options:")
	fmt.Println("  -port <port>            Server port")
	fmt.Println("  -domain <domain>        Server domain")
	fmt.Println("  -token <token>          Auth token")
	fmt.Println("")
	fmt.Println("Configuration priority (highest to lowest):")
	fmt.Println("  1. Command line flags")
	fmt.Println("  2. Environment variables")
	fmt.Println("  3. Config file (~/.config/dr1ll/config.json)")
	fmt.Println("  4. Built-in defaults")
	fmt.Println("")
	fmt.Println("Environment variables:")
	fmt.Println("  TUNNEL_PORT             Server port")
	fmt.Println("  TUNNEL_DOMAIN           Server domain")
	fmt.Println("  TUNNEL_TOKEN            Authentication token")
	fmt.Println("")
	fmt.Println("Config commands:")
	fmt.Println("  dr1ll-server config set-domain <domain>    Set server domain")
	fmt.Println("  dr1ll-server config set-port <port>        Set server port")
	fmt.Println("  dr1ll-server config set-token <token>      Set authentication token")
	fmt.Println("  dr1ll-server config show                   Show current configuration")
	fmt.Println("")
	fmt.Println("Config file format:")
	fmt.Println("  {")
	fmt.Println("    \"server_port\": \"9090\",")
	fmt.Println("    \"server_domain\": \"yourdomain.com\",")
	fmt.Println("    \"server_token\": \"your-secret-token\"")
	fmt.Println("  }")
}

func startCommand() {
	startArgs := os.Args[2:]
	
	fs := flag.NewFlagSet("start", flag.ExitOnError)
	
	cfg, err := config.Load()
	if err != nil {
		log.Printf("Failed to load config: %v, using defaults", err)
		cfg = &config.Config{
			ServerPort:   "9090",
			ServerDomain: "mydomain.com",
			ServerToken:  "some-hard-coded-token",
		}
	}
	
	defaultPort := getEnvWithConfigFallback("TUNNEL_PORT", cfg.ServerPort, "9090")
	defaultDomain := getEnvWithConfigFallback("TUNNEL_DOMAIN", cfg.ServerDomain, "mydomain.com")
	defaultToken := getEnvWithConfigFallback("TUNNEL_TOKEN", cfg.ServerToken, "some-hard-coded-token")
	
	port := fs.String("port", defaultPort, "Server port")
	domain := fs.String("domain", defaultDomain, "Server domain")
	token := fs.String("token", defaultToken, "Authentication token")
	
	fs.Parse(startArgs)

	if *token == "some-hard-coded-token" {
		log.Println("⚠️  Using default token. Set TUNNEL_TOKEN env var or use -token flag for production")
	}
	
	if *domain == "mydomain.com" {
		log.Println("⚠️  Using default domain. Set TUNNEL_DOMAIN env var or use -domain flag")
	}

	fmt.Printf("🚀 Starting tunnel server\n")
	fmt.Printf("🌐 Domain: %s\n", *domain)
	fmt.Printf("🔌 Port: %s\n", *port)
	fmt.Printf("🔑 Token: %s***\n", (*token)[:min(len(*token), 8)])

	srv := server.NewServer(*token, *domain, *port)
	if err := srv.Start(); err != nil {
		log.Fatal("Server failed to start:", err)
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvWithConfigFallback(envKey, configValue, defaultValue string) string {
	if value := os.Getenv(envKey); value != "" {
		return value
	}
	if configValue != "" {
		return configValue
	}
	return defaultValue
}

func configCommand() {
	if len(os.Args) < 3 {
		fmt.Println("Config command required. Available commands:")
		fmt.Println("  set-domain <domain>    Set server domain")
		fmt.Println("  set-port <port>        Set server port")
		fmt.Println("  set-token <token>      Set authentication token")
		fmt.Println("  show                   Show current configuration")
		os.Exit(1)
	}

	subcommand := os.Args[2]

	switch subcommand {
	case "set-domain":
		if len(os.Args) < 4 {
			fmt.Println("Usage: dr1ll-server config set-domain <domain>")
			os.Exit(1)
		}
		domain := os.Args[3]
		if err := config.SetServerDomain(domain); err != nil {
			log.Fatalf("Failed to set server domain: %v", err)
		}
		fmt.Printf("✅ Server domain set to: %s\n", domain)

	case "set-port":
		if len(os.Args) < 4 {
			fmt.Println("Usage: dr1ll-server config set-port <port>")
			os.Exit(1)
		}
		port := os.Args[3]
		if err := config.SetServerPort(port); err != nil {
			log.Fatalf("Failed to set server port: %v", err)
		}
		fmt.Printf("✅ Server port set to: %s\n", port)

	case "set-token":
		if len(os.Args) < 4 {
			fmt.Println("Usage: dr1ll-server config set-token <token>")
			os.Exit(1)
		}
		token := os.Args[3]
		if err := config.SetServerToken(token); err != nil {
			log.Fatalf("Failed to set server token: %v", err)
		}
		fmt.Println("✅ Server authentication token updated")

	case "show":
		cfg, err := config.Load()
		if err != nil {
			log.Fatalf("Failed to load configuration: %v", err)
		}
		
		configPath, _ := config.GetConfigPath()
		fmt.Printf("Configuration file: %s\n", configPath)
		fmt.Printf("Server domain: %s\n", cfg.ServerDomain)
		fmt.Printf("Server port: %s\n", cfg.ServerPort)
		if cfg.ServerToken != "" {
			fmt.Printf("Server token: %s***\n", cfg.ServerToken[:min(len(cfg.ServerToken), 8)])
		} else {
			fmt.Println("Server token: (not set)")
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