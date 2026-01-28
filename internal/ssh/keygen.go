package ssh

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"
)

const (
	keyBits = 4096
)

// KeyGenerator handles SSH key generation and management
type KeyGenerator struct{}

// NewKeyGenerator creates a new key generator
func NewKeyGenerator() *KeyGenerator {
	return &KeyGenerator{}
}

// GenerateKey generates a new RSA SSH key pair
// Returns the path to the private key
func (kg *KeyGenerator) GenerateKey(keyPath string) error {
	// Ensure directory exists
	dir := filepath.Dir(keyPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create key directory: %w", err)
	}

	// Generate RSA key
	privateKey, err := rsa.GenerateKey(rand.Reader, keyBits)
	if err != nil {
		return fmt.Errorf("failed to generate RSA key: %w", err)
	}

	// Save private key in PEM format
	privateKeyPEM := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}

	privateFile, err := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to create private key file: %w", err)
	}
	defer privateFile.Close()

	if err := pem.Encode(privateFile, privateKeyPEM); err != nil {
		return fmt.Errorf("failed to write private key: %w", err)
	}

	// Generate public key
	publicKey, err := ssh.NewPublicKey(&privateKey.PublicKey)
	if err != nil {
		return fmt.Errorf("failed to generate public key: %w", err)
	}

	// Save public key
	publicKeyPath := keyPath + ".pub"
	publicKeyBytes := ssh.MarshalAuthorizedKey(publicKey)

	if err := os.WriteFile(publicKeyPath, publicKeyBytes, 0644); err != nil {
		return fmt.Errorf("failed to write public key: %w", err)
	}

	return nil
}

// KeyExists checks if SSH key already exists
func (kg *KeyGenerator) KeyExists(keyPath string) bool {
	_, err := os.Stat(keyPath)
	return err == nil
}

// GetPublicKey reads the public key from file
func (kg *KeyGenerator) GetPublicKey(keyPath string) (string, error) {
	publicKeyPath := keyPath + ".pub"
	data, err := os.ReadFile(publicKeyPath)
	if err != nil {
		return "", fmt.Errorf("failed to read public key: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

// CopyKeyToServer adds the public key to the server's authorized_keys
func (kg *KeyGenerator) CopyKeyToServer(client *Client, publicKey string) error {
	if !client.IsConnected() {
		if err := client.Connect(); err != nil {
			return fmt.Errorf("failed to connect: %w", err)
		}
	}

	// Get the remote user's home directory
	homeDir, err := client.Run("echo $HOME")
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}
	homeDir = strings.TrimSpace(homeDir)

	// Create .ssh directory if it doesn't exist
	sshDir := homeDir + "/.ssh"
	_, err = client.Run(fmt.Sprintf("mkdir -p %s && chmod 700 %s", sshDir, sshDir))
	if err != nil {
		return fmt.Errorf("failed to create .ssh directory: %w", err)
	}

	// Check if key already exists in authorized_keys
	authKeysPath := sshDir + "/authorized_keys"
	existingKeys, _ := client.Run(fmt.Sprintf("cat %s 2>/dev/null || echo ''", authKeysPath))

	if strings.Contains(existingKeys, publicKey) {
		return nil // Key already exists
	}

	// Append public key to authorized_keys
	escapedKey := strings.ReplaceAll(publicKey, "'", "'\\''")
	_, err = client.Run(fmt.Sprintf("echo '%s' >> %s && chmod 600 %s", escapedKey, authKeysPath, authKeysPath))
	if err != nil {
		return fmt.Errorf("failed to add key to authorized_keys: %w", err)
	}

	return nil
}

// RemoveKeyFromServer removes the public key from the server's authorized_keys
func (kg *KeyGenerator) RemoveKeyFromServer(client *Client, publicKey string) error {
	if !client.IsConnected() {
		if err := client.Connect(); err != nil {
			return fmt.Errorf("failed to connect: %w", err)
		}
	}

	// Get the remote user's home directory
	homeDir, err := client.Run("echo $HOME")
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}
	homeDir = strings.TrimSpace(homeDir)

	authKeysPath := homeDir + "/.ssh/authorized_keys"

	// Create a temp file without the key and replace
	escapedKey := strings.ReplaceAll(publicKey, "/", "\\/")
	_, err = client.Run(fmt.Sprintf("sed -i '/%s/d' %s 2>/dev/null || true", escapedKey[:50], authKeysPath))
	if err != nil {
		return fmt.Errorf("failed to remove key from authorized_keys: %w", err)
	}

	return nil
}
