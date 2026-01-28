package vpn

import (
	"fmt"
	"strings"

	"github.com/vailcody/IKEv2TunnelManager/internal/ssh"
)

// Logger interface for logging operations
type Logger interface {
	Log(message string)
	Logf(format string, args ...interface{})
	Error(message string)
	Errorf(format string, args ...interface{})
}

// SetupConfig holds configuration for VPN setup
type SetupConfig struct {
	Server1 *ssh.ServerConfig
	Server2 *ssh.ServerConfig

	// VPN network settings
	VPNSubnet     string // e.g., "10.10.10.0/24" for client connections
	TunnelSubnet  string // e.g., "10.10.20.0/24" for tunnel between servers
	Server1Domain string // Domain/hostname for Server 1
	Server2Domain string // Domain/hostname for Server 2
}

// Manager handles VPN setup and management
type Manager struct {
	config  *SetupConfig
	logger  Logger
	client1 *ssh.Client
	client2 *ssh.Client
}

// NewManager creates a new VPN manager
func NewManager(config *SetupConfig, logger Logger) *Manager {
	return &Manager{
		config: config,
		logger: logger,
	}
}

// SetupAll configures both servers
func (m *Manager) SetupAll() error {
	m.logger.Log("Starting VPN chain setup...")

	// Connect to both servers
	if err := m.connectServers(); err != nil {
		return err
	}
	defer m.disconnectServers()

	// Step 1: Setup VPN server on Server 2 (exit node)
	m.logger.Log("Setting up Server 2 (exit node)...")
	if err := m.setupServer2(); err != nil {
		return fmt.Errorf("failed to setup Server 2: %w", err)
	}

	// Step 2: Setup VPN server + client on Server 1
	m.logger.Log("Setting up Server 1 (entry point + tunnel client)...")
	if err := m.setupServer1(); err != nil {
		return fmt.Errorf("failed to setup Server 1: %w", err)
	}

	// Step 3: Configure tunnel between servers
	m.logger.Log("Configuring tunnel between servers...")
	if err := m.setupTunnel(); err != nil {
		return fmt.Errorf("failed to setup tunnel: %w", err)
	}

	// Step 4: Configure routing
	m.logger.Log("Configuring routing...")
	if err := m.setupRouting(); err != nil {
		return fmt.Errorf("failed to setup routing: %w", err)
	}

	m.logger.Log("VPN chain setup completed successfully!")
	return nil
}

func (m *Manager) connectServers() error {
	m.logger.Log("Connecting to servers...")

	m.client1 = ssh.NewClient(m.config.Server1)
	if err := m.client1.Connect(); err != nil {
		return fmt.Errorf("failed to connect to Server 1: %w", err)
	}
	m.logger.Logf("Connected to Server 1: %s", m.config.Server1.Host)

	m.client2 = ssh.NewClient(m.config.Server2)
	if err := m.client2.Connect(); err != nil {
		m.client1.Close()
		return fmt.Errorf("failed to connect to Server 2: %w", err)
	}
	m.logger.Logf("Connected to Server 2: %s", m.config.Server2.Host)

	return nil
}

func (m *Manager) disconnectServers() {
	if m.client1 != nil {
		m.client1.Close()
	}
	if m.client2 != nil {
		m.client2.Close()
	}
}

func (m *Manager) setupServer2() error {
	// Install StrongSwan
	m.logger.Log("[Server 2] Checking StrongSwan installation...")
	_, err := m.client2.Run("which ipsec")
	if err == nil {
		m.logger.Log("[Server 2] StrongSwan already installed.")
	} else {
		m.logger.Log("[Server 2] Installing StrongSwan...")
		_, err := m.client2.Run("sudo apt-get update && sudo DEBIAN_FRONTEND=noninteractive apt-get install -y strongswan strongswan-pki libcharon-extra-plugins libcharon-extauth-plugins")
		if err != nil {
			return fmt.Errorf("failed to install StrongSwan: %w", err)
		}
	}

	// Disable kernel-libipsec on Server 2 (Native kernel IPsec is preferred)
	_, _ = m.client2.Run(`echo 'kernel-libipsec { load = no }' | sudo tee /etc/strongswan.d/charon/kernel-libipsec.conf`)

	// Enable IP forwarding
	m.logger.Log("[Server 2] Enabling IP forwarding...")
	_, err = m.client2.Run(`
		sudo sysctl -w net.ipv4.ip_forward=1
		sudo sysctl -w net.ipv4.conf.all.accept_redirects=0
		sudo sysctl -w net.ipv4.conf.all.send_redirects=0
		echo 'net.ipv4.ip_forward=1' | sudo tee -a /etc/sysctl.conf
	`)
	if err != nil {
		return fmt.Errorf("failed to enable IP forwarding: %w", err)
	}

	// Generate certificates
	m.logger.Log("[Server 2] Checking certificates...")
	checkCertCmd := "test -f /etc/ipsec.d/certs/server-cert.pem && test -f /etc/ipsec.d/private/server-key.pem"
	if _, err := m.client2.Run(checkCertCmd); err == nil {
		m.logger.Log("[Server 2] Certificates already exist.")
	} else {
		m.logger.Log("[Server 2] Generating certificates...")
		if err := m.generateCertificates(m.client2, m.config.Server2Domain, m.config.Server2.Host, "VPN CA Server 2"); err != nil {
			return fmt.Errorf("failed to generate certificates: %w", err)
		}
	}

	// Configure IPsec
	m.logger.Log("[Server 2] Configuring IPsec...")
	if err := m.configureIPsec(m.client2, m.config.Server2.Host, m.config.TunnelSubnet, true); err != nil {
		return fmt.Errorf("failed to configure IPsec: %w", err)
	}

	// Configure firewall
	m.logger.Log("[Server 2] Configuring firewall...")
	if err := m.configureFirewall(m.client2, true); err != nil {
		return fmt.Errorf("failed to configure firewall: %w", err)
	}

	// Restart StrongSwan
	m.logger.Log("[Server 2] Restarting StrongSwan...")
	_, err = m.client2.Run("sudo systemctl restart strongswan-starter")
	if err != nil {
		return fmt.Errorf("failed to restart StrongSwan: %w", err)
	}

	return nil
}

func (m *Manager) setupServer1() error {
	// Install StrongSwan
	m.logger.Log("[Server 1] Checking StrongSwan installation...")
	_, err := m.client1.Run("which ipsec")
	if err == nil {
		m.logger.Log("[Server 1] StrongSwan already installed.")
	} else {
		m.logger.Log("[Server 1] Installing StrongSwan...")
		installScript := `
		export DEBIAN_FRONTEND=noninteractive
		sudo apt-get update
		sudo apt-get install -y strongswan strongswan-pki libcharon-extra-plugins libcharon-extauth-plugins
		
		# Disable kernel-libipsec - native kernel IPsec is better to avoid routing lockouts
		echo 'kernel-libipsec { load = no }' | sudo tee /etc/strongswan.d/charon/kernel-libipsec.conf
	`
		if _, err := m.client1.Run(installScript); err != nil {
			return fmt.Errorf("failed to install StrongSwan: %w", err)
		}
	}

	// Enable IP forwarding
	m.logger.Log("[Server 1] Enabling IP forwarding...")
	_, err = m.client1.Run(`
		sudo sysctl -w net.ipv4.ip_forward=1
		sudo sysctl -w net.ipv4.conf.all.accept_redirects=0
		sudo sysctl -w net.ipv4.conf.all.send_redirects=0
		echo 'net.ipv4.ip_forward=1' | sudo tee -a /etc/sysctl.conf
	`)
	if err != nil {
		return fmt.Errorf("failed to enable IP forwarding: %w", err)
	}

	// Generate certificates
	m.logger.Log("[Server 1] Checking certificates...")
	checkCertCmd := "test -f /etc/ipsec.d/certs/server-cert.pem && test -f /etc/ipsec.d/private/server-key.pem"
	if _, err := m.client1.Run(checkCertCmd); err == nil {
		m.logger.Log("[Server 1] Certificates already exist.")
	} else {
		m.logger.Log("[Server 1] Generating certificates...")
		if err := m.generateCertificates(m.client1, m.config.Server1Domain, m.config.Server1.Host, "VPN CA Server 1"); err != nil {
			return fmt.Errorf("failed to generate certificates: %w", err)
		}
	}

	// Configure IPsec for VPN server (for clients)
	m.logger.Log("[Server 1] Configuring IPsec...")
	if err := m.configureIPsec(m.client1, m.config.Server1.Host, m.config.VPNSubnet, false); err != nil {
		return fmt.Errorf("failed to configure IPsec: %w", err)
	}

	// Configure firewall
	m.logger.Log("[Server 1] Configuring firewall...")
	if err := m.configureFirewall(m.client1, false); err != nil {
		return fmt.Errorf("failed to configure firewall: %w", err)
	}

	// Restart StrongSwan
	m.logger.Log("[Server 1] Restarting StrongSwan...")
	_, err = m.client1.Run("sudo systemctl restart strongswan-starter")
	if err != nil {
		return fmt.Errorf("failed to restart StrongSwan: %w", err)
	}

	return nil
}

func (m *Manager) generateCertificates(client *ssh.Client, domain, ip string, caName string) error {
	script := fmt.Sprintf(`
		sudo mkdir -p /etc/ipsec.d/{cacerts,certs,private}
		
		# Generate CA key and certificate
		if [ ! -f /etc/ipsec.d/private/ca-key.pem ]; then
			sudo ipsec pki --gen --type rsa --size 4096 --outform pem > /tmp/ca-key.pem
			sudo mv /tmp/ca-key.pem /etc/ipsec.d/private/ca-key.pem
			
			sudo ipsec pki --self --ca --lifetime 3650 \
				--in /etc/ipsec.d/private/ca-key.pem \
				--type rsa --dn "CN=%s" \
				--outform pem > /tmp/ca-cert.pem
			sudo mv /tmp/ca-cert.pem /etc/ipsec.d/cacerts/ca-cert.pem
		fi
		
		# Generate server key and certificate
		sudo ipsec pki --gen --type rsa --size 4096 --outform pem > /tmp/server-key.pem
		sudo mv /tmp/server-key.pem /etc/ipsec.d/private/server-key.pem
		
		sudo ipsec pki --pub --in /etc/ipsec.d/private/server-key.pem --type rsa \
			| sudo ipsec pki --issue --lifetime 1825 \
			--cacert /etc/ipsec.d/cacerts/ca-cert.pem \
			--cakey /etc/ipsec.d/private/ca-key.pem \
			--dn "CN=%s" --san "%s" \
			--flag serverAuth --flag ikeIntermediate \
			--outform pem > /tmp/server-cert.pem
		sudo mv /tmp/server-cert.pem /etc/ipsec.d/certs/server-cert.pem

		# Add server key to ipsec.secrets if not present
		grep -q ": RSA server-key.pem" /etc/ipsec.secrets || echo ": RSA server-key.pem" | sudo tee -a /etc/ipsec.secrets
	`, caName, ip, ip)

	_, err := client.Run(script)
	return err
}

func (m *Manager) configureIPsec(client *ssh.Client, serverIP, subnet string, isExitNode bool) error {
	var ipsecConf string

	if isExitNode {
		ipsecConf = fmt.Sprintf(`
config setup
    charondebug="ike 1, knl 1, cfg 0"
    uniqueids=no

conn ikev2-tunnel
    auto=add
    compress=no
    type=tunnel
    keyexchange=ikev2
    fragmentation=yes
    forceencaps=yes
    dpdaction=clear
    dpddelay=300s
    rekey=no
    left=%%any
    leftid=%s
    leftcert=server-cert.pem
    leftsendcert=always
    leftsubnet=0.0.0.0/0
    right=%%any
    rightid=%%any
    rightauth=eap-mschapv2
    rightsourceip=%s
    rightdns=8.8.8.8,8.8.4.4
    rightsendcert=never
    eap_identity=%%identity
`, serverIP, subnet)
	} else {
		ipsecConf = fmt.Sprintf(`
config setup
    charondebug="ike 1, knl 1, cfg 0"
    uniqueids=no

conn ikev2-vpn
    auto=add
    compress=no
    type=tunnel
    keyexchange=ikev2
    fragmentation=yes
    forceencaps=yes
    dpdaction=clear
    dpddelay=300s
    rekey=no
    left=%%any
    leftid=%s
    leftcert=server-cert.pem
    leftsendcert=always
    leftsubnet=0.0.0.0/0
    right=%%any
    rightid=%%any
    rightauth=eap-mschapv2
    rightsourceip=%s
    rightdns=8.8.8.8,8.8.4.4
    rightsendcert=never
    eap_identity=%%identity
`, serverIP, subnet)
	}

	// Write ipsec.conf (overwrite to avoid duplicates)
	cmd := fmt.Sprintf(`echo '%s' | sudo tee /etc/ipsec.conf`, strings.ReplaceAll(ipsecConf, "'", "'\\''"))
	if _, err := client.Run(cmd); err != nil {
		return err
	}

	// DISABLE automatic route installation to prevent SSH lockout
	// We will handle routing manually for vpn clients only.
	charon_conf := `
charon {
    install_routes = no
    fragment_size = 1200
}
`
	_, _ = client.Run(fmt.Sprintf(`echo '%s' | sudo tee /etc/strongswan.d/charon-prio.conf`, charon_conf))

	return nil
}

func (m *Manager) configureFirewall(client *ssh.Client, isExitNode bool) error {
	ifaceCmd := "ip route | grep default | awk '{print $5}' | head -1"
	iface, err := client.Run(ifaceCmd)
	if err != nil {
		return err
	}
	iface = strings.TrimSpace(iface)

	script := fmt.Sprintf(`
		# Always ensure SSH is allowed first
		sudo iptables -I INPUT 1 -p tcp --dport 22 -j ACCEPT
		sudo iptables -I INPUT 1 -p udp --dport 500 -j ACCEPT
		sudo iptables -I INPUT 1 -p udp --dport 4500 -j ACCEPT
		sudo iptables -I INPUT 1 -p esp -j ACCEPT

		# Skip NAT for traffic going through IPsec tunnel (critical for VPN chain)
		sudo iptables -t nat -I POSTROUTING -s 10.10.0.0/16 -m policy --pol ipsec --dir out -j ACCEPT
		
		# Enable NAT for traffic NOT going through IPsec (fallback)
		sudo iptables -t nat -A POSTROUTING -s 10.10.0.0/16 -o %s -j MASQUERADE
		
		# Allow forwarding
		sudo iptables -A FORWARD -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT
		sudo iptables -A FORWARD -s 10.10.0.0/16 -j ACCEPT
		
		# Persistent rules
		if command -v netfilter-persistent >/dev/null; then
			sudo netfilter-persistent save
		fi
	`, iface)

	_, err = client.Run(script)
	return err
}

func (m *Manager) setupTunnel() error {
	// Sync CA certs
	caCert2, err := m.client2.ReadFile("/etc/ipsec.d/cacerts/ca-cert.pem")
	if err != nil {
		return fmt.Errorf("failed to read CA cert from Server 2: %w", err)
	}
	_, _ = m.client1.Run(fmt.Sprintf(`echo '%s' | sudo tee /etc/ipsec.d/cacerts/server2-ca.pem`, string(caCert2)))

	caCert1, err := m.client1.ReadFile("/etc/ipsec.d/cacerts/ca-cert.pem")
	if err != nil {
		return fmt.Errorf("failed to read CA cert from Server 1: %w", err)
	}
	_, _ = m.client2.Run(fmt.Sprintf(`echo '%s' | sudo tee /etc/ipsec.d/cacerts/server1-ca.pem`, string(caCert1)))

	// Server 1 ipsec.conf (VPN for clients + Tunnel to Server 2)
	ipsecConf1 := fmt.Sprintf(`
config setup
    charondebug="ike 1, knl 1, cfg 0"
    uniqueids=no

conn ikev2-vpn
    auto=add
    compress=no
    type=tunnel
    keyexchange=ikev2
    fragmentation=yes
    forceencaps=yes
    dpdaction=clear
    dpddelay=300s
    rekey=no
    left=%%any
    leftid=%s
    leftcert=server-cert.pem
    leftsendcert=always
    leftsubnet=0.0.0.0/0
    right=%%any
    rightid=%%any
    rightauth=eap-mschapv2
    rightsourceip=%s
    rightdns=8.8.8.8,8.8.4.4
    rightsendcert=never
    eap_identity=%%identity

conn tunnel-to-server2
    auto=start
    type=tunnel
    keyexchange=ikev2
    left=%%defaultroute
    leftid=%s
    leftauth=pubkey
    leftsendcert=always
    leftcert=server-cert.pem
    leftsubnet=%s
    right=%s
    rightid=%s
    rightauth=pubkey
    rightsubnet=0.0.0.0/0
`, m.config.Server1.Host, m.config.VPNSubnet, m.config.Server1.Host, m.config.VPNSubnet, m.config.Server2.Host, m.config.Server2.Host)

	_, _ = m.client1.Run(fmt.Sprintf(`echo '%s' | sudo tee /etc/ipsec.conf`, strings.ReplaceAll(ipsecConf1, "'", "'\\''")))

	// Server 2 ipsec.conf (Receiving tunnel from Server 1)
	ipsecConf2 := fmt.Sprintf(`
config setup
    charondebug="ike 1, knl 1, cfg 0"
    uniqueids=no

conn tunnel-from-server1
    auto=add
    type=tunnel
    keyexchange=ikev2
    left=%%defaultroute
    leftid=%s
    leftauth=pubkey
    leftsendcert=always
    leftcert=server-cert.pem
    leftsubnet=0.0.0.0/0
    right=%s
    rightid=%s
    rightauth=pubkey
    rightsubnet=%s
    rightsendcert=never
`, m.config.Server2.Host, m.config.Server1.Host, m.config.Server1.Host, m.config.VPNSubnet)

	_, _ = m.client2.Run(fmt.Sprintf(`echo '%s' | sudo tee /etc/ipsec.conf`, strings.ReplaceAll(ipsecConf2, "'", "'\\''")))

	// Restart
	m.client1.Run("sudo systemctl restart strongswan-starter")
	m.client2.Run("sudo systemctl restart strongswan-starter")

	return nil
}

func (m *Manager) setupRouting() error {
	m.logger.Log("Configuring policy routing and fixing potential lockouts...")

	// Detect default interface and gateway on Server 1
	ifaceCmd := "ip route | grep default | awk '{print $5}' | head -1"
	iface, _ := m.client1.Run(ifaceCmd)
	iface = strings.TrimSpace(iface)

	gwCmd := fmt.Sprintf("ip route show default dev %s | awk '{print $3}' | head -1", iface)
	gw, _ := m.client1.Run(gwCmd)
	gw = strings.TrimSpace(gw)

	script := fmt.Sprintf(`
		# 1. Prevent lockout: Traffic FROM server IP always goes via main table
		sudo ip rule add from %s lookup main pref 100 2>/dev/null || true

		# 2. Ensure route to Server 2 is always via direct gateway
		sudo ip route add %s via %s dev %s 2>/dev/null || true

		# 3. Handle VPN client routing: ONLY traffic from VPNSubnet follows IPsec table 220
		# First, remove existing generic 'from all' rule if any
		sudo ip rule del from all lookup 220 2>/dev/null || true
		
		# Add specific rule for our clients
		sudo ip rule add from %s lookup 220 pref 220 2>/dev/null || true

		# Ensure table 220 has a default route to trigger the SPD (even if dummy)
		# If kernel-libipsec created ipsec0, use it. Otherwise use eth0.
		if ip link show ipsec0 >/dev/null 2>&1; then
			sudo ip route add default dev ipsec0 table 220 2>/dev/null || true
		else
			sudo ip route add default dev %s table 220 2>/dev/null || true
		fi
	`, m.config.Server1.Host, m.config.Server2.Host, gw, iface, m.config.VPNSubnet, iface)

	_, err := m.client1.Run(script)
	return err
}
