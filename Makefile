.PHONY: build clean run-server run-client test

# Build both server and client
build:
	go mod tidy
	go build -o bin/tunnel-server server.go
	go build -o bin/tunnel-client client.go

# Clean build artifacts
clean:
	rm -rf bin/

# Run the server
run-server:
	go run server.go

# Run the client (default port 3000)
run-client:
	go run client.go --port 3000

# Run client with custom port
run-client-port:
	@read -p "Enter port: " port; \
	go run client.go --port $$port

# Test with a simple HTTP server on port 3000
test-server:
	@echo "Starting test HTTP server on port 3000..."
	@echo "Visit http://localhost:3000 to test"
	python3 -m http.server 3000 2>/dev/null || python -m SimpleHTTPServer 3000

# Install dependencies
deps:
	go mod tidy
	go mod download

# Show help
help:
	@echo "Available commands:"
	@echo "  build         - Build server and client binaries"
	@echo "  clean         - Clean build artifacts"
	@echo "  run-server    - Run the tunnel server"
	@echo "  run-client    - Run the tunnel client (port 3000)"
	@echo "  test-server   - Start a test HTTP server on port 3000"
	@echo "  deps          - Install dependencies"
	@echo "  help          - Show this help message"
