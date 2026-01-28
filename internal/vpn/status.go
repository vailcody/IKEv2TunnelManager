package vpn

import (
	"fmt"
	"strings"

	"github.com/vailcody/IKEv2TunnelManager/internal/ssh"
)

// Status represents VPN connection status
type Status struct {
	Connected     bool
	TunnelActive  bool
	ActiveClients int
	Uptime        string
	ServerIP      string
	Connections   []ConnectionInfo
}

// ConnectionInfo holds info about a VPN connection
type ConnectionInfo struct {
	Name       string
	RemoteAddr string
	State      string
	Uptime     string
}

// GetStatus retrieves VPN status from a server
func GetStatus(client *ssh.Client) (*Status, error) {
	if !client.IsConnected() {
		if err := client.Connect(); err != nil {
			return nil, err
		}
	}

	status := &Status{}

	// Check if StrongSwan is running by checking if charon process exists
	// or if ipsec status returns meaningful output
	output, err := client.Run("pgrep -x charon >/dev/null && echo 'running' || echo 'stopped'")
	if err != nil {
		output = "stopped"
	}
	status.Connected = strings.TrimSpace(output) == "running"

	if !status.Connected {
		return status, nil
	}

	// Get connection status - use statusall for complete output
	ipsecOutput, err := client.Run("sudo ipsec statusall 2>/dev/null || echo 'No connections'")
	if err != nil {
		return status, nil
	}

	// Parse connections from Security Associations section
	lines := strings.Split(ipsecOutput, "\n")
	inSecurityAssociations := false
	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Detect Security Associations section
		if strings.HasPrefix(line, "Security Associations") {
			inSecurityAssociations = true
			continue
		}

		// Only parse ESTABLISHED connections in the Security Associations section
		if inSecurityAssociations && strings.Contains(line, "ESTABLISHED") {
			// Count VPN clients (ikev2-vpn connections)
			if strings.Contains(line, "ikev2-vpn[") {
				status.ActiveClients++
			}

			// Check tunnel status
			if strings.Contains(line, "tunnel-to-server2[") {
				status.TunnelActive = true
			}

			parts := strings.Fields(line)
			if len(parts) >= 2 {
				conn := ConnectionInfo{
					Name:  parts[0],
					State: "ESTABLISHED",
				}
				status.Connections = append(status.Connections, conn)
			}
		}
	}

	// Get uptime
	output, err = client.Run("systemctl show strongswan-starter --property=ActiveEnterTimestamp 2>/dev/null | cut -d= -f2")
	if err == nil {
		status.Uptime = strings.TrimSpace(output)
	}

	// Get server IP (force IPv4)
	output, err = client.Run("curl -4 -s --max-time 5 ifconfig.me 2>/dev/null || echo 'unknown'")
	if err == nil {
		status.ServerIP = strings.TrimSpace(output)
	}

	return status, nil
}

// GetDetailedLogs retrieves StrongSwan logs
func GetDetailedLogs(client *ssh.Client, lines int) (string, error) {
	if !client.IsConnected() {
		if err := client.Connect(); err != nil {
			return "", err
		}
	}

	// Get journal logs
	logs, err := client.Run(fmt.Sprintf("sudo journalctl -u strongswan-starter -n %d --no-pager 2>/dev/null", lines))
	if err != nil {
		logs = fmt.Sprintf("Failed to get journal logs: %v", err)
	}

	// Get IPsec status
	status, _ := client.Run("sudo ipsec statusall 2>/dev/null")

	// Get Interfaces
	ifaces, _ := client.Run("ip addr")

	// Get Routes
	routes, _ := client.Run("ip route")

	// Get Tables
	tables, _ := client.Run("ip rule list && ip route show table vpnclients")

	return fmt.Sprintf("=== Journal ===\n%s\n\n=== IPsec Status ===\n%s\n\n=== Interfaces ===\n%s\n\n=== Routes ===\n%s\n\n=== Policy Routes ===\n%s", logs, status, ifaces, routes, tables), nil
}

// RestartVPN restarts StrongSwan service
func RestartVPN(client *ssh.Client) error {
	if !client.IsConnected() {
		if err := client.Connect(); err != nil {
			return err
		}
	}

	_, err := client.Run("sudo systemctl restart strongswan-starter")
	return err
}

// StopVPN stops StrongSwan service
func StopVPN(client *ssh.Client) error {
	if !client.IsConnected() {
		if err := client.Connect(); err != nil {
			return err
		}
	}

	_, err := client.Run("sudo systemctl stop strongswan-starter")
	return err
}
