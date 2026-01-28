package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const (
	configDirName  = ".tunnelmanager"
	configFileName = "config.json"
	logsDirName    = "logs"
)

// Storage handles loading and saving configuration
type Storage struct {
	configDir string
}

// New creates a new Storage instance
func New() (*Storage, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	configDir := filepath.Join(homeDir, configDirName)
	return &Storage{configDir: configDir}, nil
}

// GetConfigDir returns the configuration directory path
func (s *Storage) GetConfigDir() string {
	return s.configDir
}

// GetLogDir returns the logs directory path
func (s *Storage) GetLogDir() string {
	return filepath.Join(s.configDir, logsDirName)
}

// GetSSHKeyDir returns the SSH keys directory path
func (s *Storage) GetSSHKeyDir() string {
	return filepath.Join(s.configDir, "ssh")
}

// GetDefaultKeyPath returns the default SSH key path
func (s *Storage) GetDefaultKeyPath() string {
	return filepath.Join(s.GetSSHKeyDir(), "tunnelmanager_rsa")
}

// EnsureDirs creates all necessary directories
func (s *Storage) EnsureDirs() error {
	dirs := []string{
		s.configDir,
		s.GetLogDir(),
		s.GetSSHKeyDir(),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
	}
	return nil
}

// Load reads configuration from file
func (s *Storage) Load() (*AppConfig, error) {
	configPath := filepath.Join(s.configDir, configFileName)

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Return default config if file doesn't exist
			return NewAppConfig(), nil
		}
		return nil, err
	}

	var config AppConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	// Ensure we have at least 2 servers
	for len(config.Servers) < 2 {
		config.Servers = append(config.Servers, ServerConfig{
			Name: "Server " + string(rune('1'+len(config.Servers))),
			Port: 22,
		})
	}

	return &config, nil
}

// Save writes configuration to file
func (s *Storage) Save(config *AppConfig) error {
	if err := s.EnsureDirs(); err != nil {
		return err
	}

	configPath := filepath.Join(s.configDir, configFileName)

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0600)
}
