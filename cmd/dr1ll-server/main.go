package main

import (
	"flag"
	"fmt"
	"log"
	"os"

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
	fmt.Println("  dr1ll-server help              Show this help message")
	fmt.Println("")
	fmt.Println("Start options:")
	fmt.Println("  -port <port>            Server port (default: 9090, or TUNNEL_PORT env)")
	fmt.Println("  -domain <domain>        Server domain (default: mydomain.com, or TUNNEL_DOMAIN env)")
	fmt.Println("  -token <token>          Auth token (default: some-hard-coded-token, or TUNNEL_TOKEN env)")
	fmt.Println("")
	fmt.Println("Environment variables:")
	fmt.Println("  TUNNEL_PORT             Server port")
	fmt.Println("  TUNNEL_DOMAIN           Server domain")
	fmt.Println("  TUNNEL_TOKEN            Authentication token")
}

func startCommand() {
	startArgs := os.Args[2:]
	
	fs := flag.NewFlagSet("start", flag.ExitOnError)
	
	defaultPort := getEnv("TUNNEL_PORT", "9090")
	defaultDomain := getEnv("TUNNEL_DOMAIN", "mydomain.com")
	defaultToken := getEnv("TUNNEL_TOKEN", "some-hard-coded-token")
	
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}