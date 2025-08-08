package main

import (
	"crypto/rand"
	"encoding/hex"
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
	clients           map[string]*Client
	mutex             sync.RWMutex
	upgrader          websocket.Upgrader
	pendingRequests   map[string]chan Message
	pendingRequestsMu sync.RWMutex
}

func NewServer() *Server {
	return &Server{
		clients: make(map[string]*Client),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all origins for simplicity
			},
		},
		pendingRequests: make(map[string]chan Message),
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
	s.pendingRequestsMu.RLock()
	ch, ok := s.pendingRequests[msg.ID]
	s.pendingRequestsMu.RUnlock()
	if ok {
		ch <- msg
	} else {
		log.Printf("No pending request for ID %s", msg.ID)
	}
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

	// Prepare a channel to receive the response
	responseChan := make(chan Message, 1)

	s.pendingRequestsMu.Lock()
	s.pendingRequests[requestID] = responseChan
	s.pendingRequestsMu.Unlock()

	defer func() {
		s.pendingRequestsMu.Lock()
		delete(s.pendingRequests, requestID)
		s.pendingRequestsMu.Unlock()
	}()

	msg := Message{
		Type:    "http_request",
		ID:      requestID,
		Method:  r.Method,
		Path:    r.URL.Path,
		Headers: headers,
		Body:    string(body),
	}

	// Try sending the request
	select {
	case client.send <- msg:
		// Wait for response or timeout
		select {
		case resp := <-responseChan:
			// Write response from client
			for key, value := range resp.Headers {
				w.Header().Set(key, value)
			}
			w.WriteHeader(resp.Status)
			w.Write([]byte(resp.Body))
		case <-r.Context().Done():
			http.Error(w, "Client response timeout", http.StatusGatewayTimeout)
		}
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

	http.HandleFunc("/ws", server.handleWebSocket)

	http.HandleFunc("/", server.handleHTTPRequest)

	log.Printf("Tunnel server starting on :9090")
	log.Printf("WebSocket endpoint: ws://%s:9090/ws", DOMAIN)
	log.Printf("HTTP tunnels: http://*.%s:9090", DOMAIN)

	if err := http.ListenAndServe(":9090", nil); err != nil {
		log.Fatal("Server failed to start:", err)
	}
}
