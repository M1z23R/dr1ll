package server

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Message struct {
	Type              string            `json:"type"`
	ID                string            `json:"id,omitempty"`
	Subdomain         string            `json:"subdomain,omitempty"`
	RequestedSubdomain string           `json:"requested_subdomain,omitempty"`
	Method            string            `json:"method,omitempty"`
	Path              string            `json:"path,omitempty"`
	Headers           map[string]string `json:"headers,omitempty"`
	Body              string            `json:"body,omitempty"`
	Status            int               `json:"status,omitempty"`
	Error             string            `json:"error,omitempty"`
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
	token             string
	domain            string
	port              string
}

func NewServer(token, domain, port string) *Server {
	return &Server{
		clients: make(map[string]*Client),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
		pendingRequests: make(map[string]chan Message),
		token:           token,
		domain:          domain,
		port:            port,
	}
}

func (s *Server) generateSubdomain() string {
	bytes := make([]byte, 4)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

func (s *Server) isSubdomainAvailable(subdomain string) bool {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	_, exists := s.clients[subdomain]
	return !exists
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

func (s *Server) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	token := r.Header.Get("Authorization")
	if token != "Bearer "+s.token {
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

	assignMsg := Message{
		Type:      "subdomain_assigned",
		Subdomain: fmt.Sprintf("%s.%s", subdomain, s.domain),
	}

	if err := conn.WriteJSON(assignMsg); err != nil {
		log.Printf("Failed to send subdomain assignment: %v", err)
		return
	}

	log.Printf("Client connected with subdomain: %s.%s", subdomain, s.domain)

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

		switch msg.Type {
		case "http_response":
			s.handleHTTPResponse(msg)
		case "subdomain_request":
			s.handleSubdomainRequest(client, msg)
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

func (s *Server) handleSubdomainRequest(client *Client, msg Message) {
	if s.isSubdomainAvailable(msg.RequestedSubdomain) {
		s.unregisterClient(client.subdomain)
		client.subdomain = msg.RequestedSubdomain
		s.registerClient(msg.RequestedSubdomain, client)
		
		assignMsg := Message{
			Type:      "subdomain_assigned",
			Subdomain: fmt.Sprintf("%s.%s", msg.RequestedSubdomain, s.domain),
		}
		
		if err := client.conn.WriteJSON(assignMsg); err != nil {
			log.Printf("Failed to send subdomain assignment: %v", err)
		} else {
			log.Printf("Client reassigned to requested subdomain: %s.%s", msg.RequestedSubdomain, s.domain)
		}
	} else {
		errorMsg := Message{
			Type:  "error",
			Error: fmt.Sprintf("Subdomain '%s' is not available", msg.RequestedSubdomain),
		}
		client.conn.WriteJSON(errorMsg)
	}
}

func (s *Server) HandleHTTPRequest(w http.ResponseWriter, r *http.Request) {
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

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusInternalServerError)
		return
	}

	headers := make(map[string]string)
	for name, values := range r.Header {
		if len(values) > 0 {
			headers[name] = values[0]
		}
	}

	requestID := s.generateSubdomain()
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

	select {
	case client.send <- msg:
		select {
		case resp := <-responseChan:
			for key, value := range resp.Headers {
				w.Header().Set(key, value)
			}
			w.WriteHeader(resp.Status)
			w.Write([]byte(resp.Body))
		case <-time.After(30 * time.Second):
			http.Error(w, "Client response timeout", http.StatusGatewayTimeout)
		case <-r.Context().Done():
			http.Error(w, "Request cancelled", http.StatusRequestTimeout)
		}
	default:
		http.Error(w, "Tunnel is busy", http.StatusServiceUnavailable)
	}
}

func (s *Server) Start() error {
	http.HandleFunc("/ws", s.HandleWebSocket)
	http.HandleFunc("/", s.HandleHTTPRequest)

	log.Printf("Tunnel server starting on :%s", s.port)
	log.Printf("WebSocket endpoint: wss://%s:%s/ws", s.domain, s.port)
	log.Printf("HTTP tunnels: https://*.%s:%s", s.domain, s.port)

	return http.ListenAndServe(":"+s.port, nil)
}