package storage

// ServerConfig represents a saved server configuration
type ServerConfig struct {
	Name     string `json:"name"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password,omitempty"` // Not saved if using key auth
	KeyPath  string `json:"key_path,omitempty"`
}

// AppConfig holds the application configuration
type AppConfig struct {
	Servers    []ServerConfig `json:"servers"`
	SSHKeyPath string         `json:"ssh_key_path"`
}

// NewAppConfig creates a new config with defaults
func NewAppConfig() *AppConfig {
	return &AppConfig{
		Servers: []ServerConfig{
			{Name: "Server 1", Port: 22},
			{Name: "Server 2", Port: 22},
		},
	}
}
