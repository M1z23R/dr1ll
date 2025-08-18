package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	TunnelServer string `json:"tunnel_server"`
	Token        string `json:"token"`
	
	// Server configuration
	ServerPort   string `json:"server_port,omitempty"`
	ServerDomain string `json:"server_domain,omitempty"`
	ServerToken  string `json:"server_token,omitempty"`
}

func GetConfigDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}
	
	configDir := filepath.Join(homeDir, ".config", "dr1ll")
	return configDir, nil
}

func GetConfigPath() (string, error) {
	configDir, err := GetConfigDir()
	if err != nil {
		return "", err
	}
	
	return filepath.Join(configDir, "config.json"), nil
}

func EnsureConfigDir() error {
	configDir, err := GetConfigDir()
	if err != nil {
		return err
	}
	
	return os.MkdirAll(configDir, 0755)
}

func Load() (*Config, error) {
	configPath, err := GetConfigPath()
	if err != nil {
		return nil, err
	}
	
	// Return default config if file doesn't exist
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return &Config{
			TunnelServer: "http://localhost:9090",
			Token:        "some-hard-coded-token",
			ServerPort:   "9090",
			ServerDomain: "mydomain.com",
			ServerToken:  "some-hard-coded-token",
		}, nil
	}
	
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}
	
	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}
	
	return &config, nil
}

func (c *Config) Save() error {
	if err := EnsureConfigDir(); err != nil {
		return err
	}
	
	configPath, err := GetConfigPath()
	if err != nil {
		return err
	}
	
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}
	
	return nil
}

func SetServer(server string) error {
	config, err := Load()
	if err != nil {
		return err
	}
	
	config.TunnelServer = server
	return config.Save()
}

func SetToken(token string) error {
	config, err := Load()
	if err != nil {
		return err
	}
	
	config.Token = token
	return config.Save()
}

func SetServerDomain(domain string) error {
	config, err := Load()
	if err != nil {
		return err
	}
	
	config.ServerDomain = domain
	return config.Save()
}

func SetServerPort(port string) error {
	config, err := Load()
	if err != nil {
		return err
	}
	
	config.ServerPort = port
	return config.Save()
}

func SetServerToken(token string) error {
	config, err := Load()
	if err != nil {
		return err
	}
	
	config.ServerToken = token
	return config.Save()
}