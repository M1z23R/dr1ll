package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"

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
	conn      *websocket.Conn
	subdomain string
	send      chan Message
}

type Server struct {
	clients  map[string]*Client
	mutex    sync.RWMutex
	upgrader websocket.Upgrader
}

func NewServer() *Server {
	return &Server{
		clients: make(map[string]*Client),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all origins for simplicity
			},
		},
	}
}

func (s *Server) generateSubdomain() string {
	bytes := make([]byte, 4)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

func (s *Server) registerClient(subdomain string, client *Client) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.clients[subdomain] = client
}

func (s *Server) unregisterClient(subdomain string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if client, ok := s.clients[subdomain]; ok {
		close(client.send)
		delete(s.clients, subdomain)
	}
}

func (s *Server) getClient(subdomain string) (*Client, bool) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	client, ok := s.clients[subdomain]
	return client, ok
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Check token
	token := r.Header.Get("Authorization")
	if token != "Bearer "+TOKEN {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}

	subdomain := s.generateSubdomain()
	client := &Client{
		conn:      conn,
		subdomain: subdomain,
		send:      make(chan Message, 256),
	}

	s.registerClient(subdomain, client)
	defer s.unregisterClient(subdomain)

	// Send subdomain to client
	assignMsg := Message{
		Type:      "subdomain_assigned",
		Subdomain: fmt.Sprintf("%s.%s", subdomain, DOMAIN),
	}

	if err := conn.WriteJSON(assignMsg); err != nil {
		log.Printf("Failed to send subdomain assignment: %v", err)
		return
	}

	log.Printf("Client connected with subdomain: %s.%s", subdomain, DOMAIN)

	// Start goroutines for reading and writing
	go s.writePump(client)
	s.readPump(client)
}

func (s *Server) readPump(client *Client) {
	defer client.conn.Close()

	for {
		var msg Message
		if err := client.conn.ReadJSON(&msg); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}

		// Handle response from client
		if msg.Type == "http_response" {
			// Store response for HTTP handler (in real implementation, use channels or callback)
			s.handleHTTPResponse(msg)
		}
	}
}

func (s *Server) writePump(client *Client) {
	defer client.conn.Close()

	for {
		select {
		case msg, ok := <-client.send:
			if !ok {
				client.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := client.conn.WriteJSON(msg); err != nil {
				log.Printf("Failed to write message: %v", err)
				return
			}
		}
	}
}

func (s *Server) handleHTTPResponse(msg Message) {
	// In a real implementation, you'd use channels or callbacks to send responses
	// back to the waiting HTTP handler
	log.Printf("Received response for request %s: status %d", msg.ID, msg.Status)
}

func (s *Server) handleHTTPRequest(w http.ResponseWriter, r *http.Request) {
	// Extract subdomain from Host header
	host := r.Host
	if !strings.Contains(host, ".") {
		http.Error(w, "Invalid subdomain", http.StatusBadRequest)
		return
	}

	subdomain := strings.Split(host, ".")[0]

	client, exists := s.getClient(subdomain)
	if !exists {
		http.Error(w, "Tunnel not found", http.StatusNotFound)
		return
	}

	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusInternalServerError)
		return
	}

	// Convert headers to map
	headers := make(map[string]string)
	for name, values := range r.Header {
		if len(values) > 0 {
			headers[name] = values[0] // Take first value for simplicity
		}
	}

	// Generate request ID
	requestID := s.generateSubdomain()

	// Create message for client
	msg := Message{
		Type:    "http_request",
		ID:      requestID,
		Method:  r.Method,
		Path:    r.URL.Path,
		Headers: headers,
		Body:    string(body),
	}

	// Send to client (non-blocking)
	select {
	case client.send <- msg:
		// For simplicity, return a basic response
		// In a real implementation, you'd wait for the client's response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		response := map[string]any{
			"message":    "Request forwarded to tunnel",
			"tunnel":     subdomain + "." + DOMAIN,
			"request_id": requestID,
		}
		json.NewEncoder(w).Encode(response)
	default:
		http.Error(w, "Tunnel is busy", http.StatusServiceUnavailable)
	}
}

func init() {
	TOKEN = os.Getenv("TUNNEL_TOKEN")
	if TOKEN == "" {
		TOKEN = "some-hard-coded-token"
		log.Println("⚠️  TUNNEL_TOKEN not set, using default token")
	}

	DOMAIN = os.Getenv("TUNNEL_DOMAIN")
	if DOMAIN == "" {
		DOMAIN = "mydomain.com"
		log.Println("⚠️  TUNNEL_DOMAIN not set, using default domain")
	}
}

func main() {
	server := NewServer()

	// WebSocket endpoint for tunnel clients
	http.HandleFunc("/ws", server.handleWebSocket)

	// Catch-all handler for HTTP requests to tunnels
	http.HandleFunc("/", server.handleHTTPRequest)

	log.Printf("Tunnel server starting on :8080")
	log.Printf("WebSocket endpoint: ws://localhost:8080/ws")
	log.Printf("HTTP tunnels: http://*.%s:8080", DOMAIN)

	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal("Server failed to start:", err)
	}
}
