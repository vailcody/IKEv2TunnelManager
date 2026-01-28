package ssh

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// ServerConfig holds SSH connection parameters
type ServerConfig struct {
	Host       string
	Port       int
	User       string
	Password   string
	KeyPath    string
	KeyContent []byte
}

// Client wraps SSH client functionality
type Client struct {
	config     *ServerConfig
	connection *ssh.Client
}

// NewClient creates a new SSH client
func NewClient(config *ServerConfig) *Client {
	if config.Port == 0 {
		config.Port = 22
	}
	return &Client{config: config}
}

// Connect establishes SSH connection
func (c *Client) Connect() error {
	authMethods := []ssh.AuthMethod{}

	// Try key authentication first
	if c.config.KeyPath != "" {
		key, err := os.ReadFile(c.config.KeyPath)
		if err != nil {
			return fmt.Errorf("failed to read key file: %w", err)
		}
		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			return fmt.Errorf("failed to parse private key: %w", err)
		}
		authMethods = append(authMethods, ssh.PublicKeys(signer))
	} else if len(c.config.KeyContent) > 0 {
		signer, err := ssh.ParsePrivateKey(c.config.KeyContent)
		if err != nil {
			return fmt.Errorf("failed to parse private key: %w", err)
		}
		authMethods = append(authMethods, ssh.PublicKeys(signer))
	}

	// Add password authentication
	if c.config.Password != "" {
		authMethods = append(authMethods, ssh.Password(c.config.Password))
	}

	if len(authMethods) == 0 {
		return fmt.Errorf("no authentication method provided")
	}

	sshConfig := &ssh.ClientConfig{
		User:            c.config.User,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // TODO: implement proper host key verification
		Timeout:         10 * time.Second,
	}

	addr := net.JoinHostPort(c.config.Host, fmt.Sprintf("%d", c.config.Port))
	conn, err := ssh.Dial("tcp", addr, sshConfig)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	c.connection = conn
	return nil
}

// Close closes the SSH connection
func (c *Client) Close() error {
	if c.connection != nil {
		return c.connection.Close()
	}
	return nil
}

// IsConnected returns true if connected
func (c *Client) IsConnected() bool {
	return c.connection != nil
}

// Run executes a command and returns output
func (c *Client) Run(command string) (string, error) {
	if c.connection == nil {
		return "", fmt.Errorf("not connected")
	}

	session, err := c.connection.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	if err := session.Run(command); err != nil {
		return "", fmt.Errorf("command failed: %w, stderr: %s", err, stderr.String())
	}

	return stdout.String(), nil
}

// RunWithOutput executes a command and streams output to writer
func (c *Client) RunWithOutput(command string, stdout, stderr io.Writer) error {
	if c.connection == nil {
		return fmt.Errorf("not connected")
	}

	session, err := c.connection.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	session.Stdout = stdout
	session.Stderr = stderr

	return session.Run(command)
}

// RunSudo executes a command with sudo
func (c *Client) RunSudo(command string) (string, error) {
	if c.connection == nil {
		return "", fmt.Errorf("not connected")
	}

	session, err := c.connection.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	stdin, err := session.StdinPipe()
	if err != nil {
		return "", fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	// Run sudo with -S to read password from stdin
	// -p '' removes the password prompt
	// Escape single quotes to prevent breaking the bash command
	escapedCmd := strings.ReplaceAll(command, "'", "'\\''")
	fullCmd := fmt.Sprintf("sudo -S -p '' bash -c '%s'", escapedCmd)

	if err := session.Start(fullCmd); err != nil {
		return "", fmt.Errorf("failed to start command: %w", err)
	}

	// Write password to stdin
	if c.config.Password != "" {
		fmt.Fprintln(stdin, c.config.Password)
	}
	stdin.Close() // Close stdin to signal EOF

	if err := session.Wait(); err != nil {
		return "", fmt.Errorf("command failed: %w, stderr: %s", err, stderr.String())
	}

	return stdout.String(), nil
}

// CopyFile copies a local file to remote server
func (c *Client) CopyFile(localPath, remotePath string, mode os.FileMode) error {
	if c.connection == nil {
		return fmt.Errorf("not connected")
	}

	content, err := os.ReadFile(localPath)
	if err != nil {
		return fmt.Errorf("failed to read local file: %w", err)
	}

	return c.WriteFile(remotePath, content, mode)
}

// WriteFile writes content to a remote file
func (c *Client) WriteFile(remotePath string, content []byte, mode os.FileMode) error {
	if c.connection == nil {
		return fmt.Errorf("not connected")
	}

	session, err := c.connection.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	go func() {
		w, _ := session.StdinPipe()
		defer w.Close()
		fmt.Fprintf(w, "C%04o %d %s\n", mode, len(content), "file")
		w.Write(content)
		fmt.Fprint(w, "\x00")
	}()

	return session.Run(fmt.Sprintf("scp -t %s", remotePath))
}

// ReadFile reads content from a remote file
func (c *Client) ReadFile(remotePath string) ([]byte, error) {
	output, err := c.Run(fmt.Sprintf("cat %s", remotePath))
	if err != nil {
		return nil, err
	}
	return []byte(output), nil
}

// TestConnection tests the SSH connection
func (c *Client) TestConnection() error {
	if err := c.Connect(); err != nil {
		return err
	}
	defer c.Close()

	output, err := c.Run("echo 'Connection successful' && hostname")
	if err != nil {
		return err
	}

	if output == "" {
		return fmt.Errorf("empty response from server")
	}

	return nil
}
