package vpn

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/vailcody/IKEv2TunnelManager/internal/ssh"
)

// User represents a VPN user
type User struct {
	Username string
	Password string
}

// UserManager handles VPN user operations
type UserManager struct {
	client *ssh.Client
	logger Logger
}

// NewUserManager creates a new user manager
func NewUserManager(client *ssh.Client, logger Logger) *UserManager {
	return &UserManager{
		client: client,
		logger: logger,
	}
}

// ListUsers returns list of VPN users
func (um *UserManager) ListUsers() ([]User, error) {
	if !um.client.IsConnected() {
		if err := um.client.Connect(); err != nil {
			return nil, err
		}
	}

	output, err := um.client.RunSudo("cat /etc/ipsec.secrets")
	if err != nil {
		return nil, err
	}

	var users []User
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, ": EAP") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) >= 1 {
				username := strings.TrimSpace(parts[0])
				if username != "" && username != "tunnel-user" {
					users = append(users, User{Username: username})
				}
			}
		}
	}

	return users, nil
}

// AddUser adds a new VPN user and returns the password (generated if not provided)
func (um *UserManager) AddUser(username, password string) (string, error) {
	if !um.client.IsConnected() {
		if err := um.client.Connect(); err != nil {
			return "", err
		}
	}

	// Validate username
	if strings.ContainsAny(username, " :\n'\"") {
		return "", fmt.Errorf("invalid username: contains forbidden characters")
	}

	// Generate password if not provided
	if password == "" {
		bytes := make([]byte, 16)
		rand.Read(bytes)
		password = hex.EncodeToString(bytes)
	}

	// Check if user already exists
	users, err := um.ListUsers()
	if err != nil {
		return "", err
	}
	for _, u := range users {
		if u.Username == username {
			return "", fmt.Errorf("user %s already exists", username)
		}
	}

	// Add user to ipsec.secrets
	secretLine := fmt.Sprintf(`%s : EAP "%s"`, username, password)
	cmd := fmt.Sprintf(`echo '%s' | tee -a /etc/ipsec.secrets`, secretLine)
	_, err = um.client.RunSudo(cmd)
	if err != nil {
		return "", fmt.Errorf("failed to add user: %w", err)
	}

	// Reload secrets
	_, err = um.client.RunSudo("ipsec rereadsecrets")
	if err != nil {
		um.logger.Errorf("Warning: failed to reload secrets: %v", err)
	}

	um.logger.Logf("Added user: %s", username)
	return password, nil
}

// GetUserPassword retrieves the password for a user from ipsec.secrets
func (um *UserManager) GetUserPassword(username string) (string, error) {
	if !um.client.IsConnected() {
		if err := um.client.Connect(); err != nil {
			return "", err
		}
	}

	output, err := um.client.RunSudo("cat /etc/ipsec.secrets")
	if err != nil {
		return "", err
	}

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, username+" : EAP") {
			// Format: username : EAP "password"
			start := strings.Index(line, "\"")
			end := strings.LastIndex(line, "\"")
			if start != -1 && end != -1 && end > start {
				return line[start+1 : end], nil
			}
		}
	}

	return "", fmt.Errorf("password not found for user %s", username)
}

// RemoveUser removes a VPN user
func (um *UserManager) RemoveUser(username string) error {
	if !um.client.IsConnected() {
		if err := um.client.Connect(); err != nil {
			return err
		}
	}

	// Remove user line from ipsec.secrets
	cmd := fmt.Sprintf(`sed -i '/^%s : EAP/d' /etc/ipsec.secrets`, username)
	_, err := um.client.RunSudo(cmd)
	if err != nil {
		return fmt.Errorf("failed to remove user: %w", err)
	}

	// Reload secrets
	_, err = um.client.RunSudo("ipsec rereadsecrets")
	if err != nil {
		um.logger.Errorf("Warning: failed to reload secrets: %v", err)
	}

	um.logger.Logf("Removed user: %s", username)
	return nil
}

// GeneratePassword generates a random password
func GeneratePassword(length int) string {
	bytes := make([]byte, length/2+1)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)[:length]
}
