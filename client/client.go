package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

var (
	TOKEN  string
	DOMAIN string
)

type Message struct {
	Type      string            `json:"type"`
	ID        string            `json:"id,omitempty"`
	Subdomain string            `json:"subdomain,omitempty"`
	Method    string            `json:"method,omitempty"`
	Path      string            `json:"path,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	Body      string            `json:"body,omitempty"`
	Status    int               `json:"status,omitempty"`
	Error     string            `json:"error,omitempty"`
}

type Client struct {
	conn            *websocket.Conn
	localPort       int
	serverURL       string
	done            chan struct{}
	pendingRequests map[string]chan Message
}

func NewClient(serverURL string, localPort int) *Client {
	return &Client{
		serverURL:       serverURL,
		localPort:       localPort,
		done:            make(chan struct{}),
		pendingRequests: make(map[string]chan Message),
	}
}

func (c *Client) connect() error {
	// Parse server URL and convert to WebSocket URL
	u, err := url.Parse(c.serverURL)
	if err != nil {
		return fmt.Errorf("invalid server URL: %v", err)
	}

	scheme := "ws"
	if u.Scheme == "https" {
		scheme = "wss"
	}

	wsURL := fmt.Sprintf("%s://%s/ws", scheme, u.Host)

	// Set up headers with authorization token
	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+TOKEN)

	// Connect to WebSocket
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, headers)
	if err != nil {
		return fmt.Errorf("failed to connect to server: %v", err)
	}

	c.conn = conn
	return nil
}

func (c *Client) handleMessages() {
	defer c.conn.Close()

	for {
		var msg Message
		if err := c.conn.ReadJSON(&msg); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			close(c.done)
			return
		}

		switch msg.Type {
		case "subdomain_assigned":
			fmt.Printf("🚀 Tunnel active! Your URL is: http://%s\n", msg.Subdomain)
			fmt.Printf("💡 Forwarding requests to localhost:%d\n", c.localPort)
			fmt.Println("📝 Press Ctrl+C to stop the tunnel")

		case "http_request":
			go c.forwardRequest(msg)

		case "http_response":
			if ch, ok := c.pendingRequests[msg.ID]; ok {
				ch <- msg
				delete(c.pendingRequests, msg.ID)
			}

		default:
			log.Printf("Unknown message type: %s", msg.Type)
		}
	}
}

func (c *Client) forwardRequest(msg Message) {
	// Construct local URL
	localURL := fmt.Sprintf("http://localhost:%d%s", c.localPort, msg.Path)

	var bodyReader io.Reader
	if msg.Body != "" {
		bodyReader = strings.NewReader(msg.Body)
	}

	req, err := http.NewRequest(msg.Method, localURL, bodyReader)
	if err != nil {
		c.sendErrorResponse(msg.ID, fmt.Sprintf("Failed to create request: %v", err))
		return
	}

	for name, value := range msg.Headers {
		if name != "Host" {
			req.Header.Set(name, value)
		}
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		c.sendErrorResponse(msg.ID, fmt.Sprintf("Request failed: %v", err))
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		c.sendErrorResponse(msg.ID, fmt.Sprintf("Failed to read response: %v", err))
		return
	}

	respHeaders := make(map[string]string)
	for name, values := range resp.Header {
		if len(values) > 0 {
			respHeaders[name] = values[0]
		}
	}

	response := Message{
		Type:    "http_response",
		ID:      msg.ID,
		Status:  resp.StatusCode,
		Headers: respHeaders,
		Body:    string(respBody),
	}

	// Send back the HTTP response to the server via WebSocket
	if err := c.conn.WriteJSON(response); err != nil {
		log.Printf("Failed to send response: %v", err)
	}

	log.Printf("✅ %s %s -> %d", msg.Method, msg.Path, resp.StatusCode)
}

func (c *Client) sendErrorResponse(requestID, errorMsg string) {
	response := Message{
		Type:   "http_response",
		ID:     requestID,
		Status: 500,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: fmt.Sprintf(`{"error": "%s"}`, errorMsg),
	}

	if err := c.conn.WriteJSON(response); err != nil {
		log.Printf("Failed to send error response: %v", err)
	}

	log.Printf("❌ Request %s failed: %s", requestID, errorMsg)
}

func (c *Client) run() error {
	if err := c.connect(); err != nil {
		return err
	}

	fmt.Println("🔌 Connecting to tunnel server...")

	// Handle messages in a separate goroutine
	go c.handleMessages()

	// Wait for interrupt signal
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)

	select {
	case <-c.done:
		log.Println("Connection closed")
	case <-interrupt:
		log.Println("Interrupt received, closing connection...")

		// Send close message to server
		err := c.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		if err != nil {
			log.Printf("Error sending close message: %v", err)
		}

		// Wait for connection to close or timeout
		select {
		case <-c.done:
		case <-time.After(time.Second):
		}
	}

	return nil
}

func main() {
	var (
		port      = flag.Int("port", 3000, "Local port to forward requests to")
		serverURL = flag.String("server", DOMAIN, "Tunnel server URL")
		token     = flag.String("token", TOKEN, "Authentication token")
	)
	flag.Parse()

	if *token != TOKEN {
		log.Fatal("Invalid token provided")
	}

	fmt.Printf("🏠 Starting tunnel client for localhost:%d\n", *port)
	fmt.Printf("🌐 Server: %s\n", *serverURL)

	client := NewClient(*serverURL, *port)
	if err := client.run(); err != nil {
		log.Fatal(err)
	}

	fmt.Println("👋 Tunnel closed. Goodbye!")
}

func init() {
	TOKEN = os.Getenv("TUNNEL_TOKEN")
	if TOKEN == "" {
		TOKEN = "some-hard-coded-token"
		log.Println("⚠️  TUNNEL_TOKEN not set, using default token")
	}

	DOMAIN = os.Getenv("TUNNEL_DOMAIN")
	if DOMAIN == "" {
		DOMAIN = "http://localhost:9090"
		log.Println("⚠️  TUNNEL_DOMAIN not set, using default domain")
	}
}
