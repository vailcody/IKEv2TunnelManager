package vpn

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// GenerateMobileConfig creates an Apple .mobileconfig profile for IKEv2 VPN
func GenerateMobileConfig(username, password, serverIP, caCertPEM string) []byte {
	profileUUID := uuid.New().String()
	payloadUUID := uuid.New().String()
	certUUID := uuid.New().String()

	// Base64 encode the CA certificate (PEM format without headers)
	caCertData := cleanPEMCert(caCertPEM)

	config := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>PayloadContent</key>
	<array>
		<dict>
			<key>IKEv2</key>
			<dict>
				<key>AuthenticationMethod</key>
				<string>None</string>
				<key>ChildSecurityAssociationParameters</key>
				<dict>
					<key>DiffieHellmanGroup</key>
					<integer>14</integer>
					<key>EncryptionAlgorithm</key>
					<string>AES-256</string>
					<key>IntegrityAlgorithm</key>
					<string>SHA2-256</string>
					<key>LifeTimeInMinutes</key>
					<integer>1440</integer>
				</dict>
				<key>DeadPeerDetectionRate</key>
				<string>Medium</string>
				<key>DisableMOBIKE</key>
				<integer>0</integer>
				<key>DisableRedirect</key>
				<integer>0</integer>
				<key>EnableCertificateRevocationCheck</key>
				<integer>0</integer>
				<key>EnablePFS</key>
				<true/>
				<key>ExtendedAuthEnabled</key>
				<true/>
				<key>IKESecurityAssociationParameters</key>
				<dict>
					<key>DiffieHellmanGroup</key>
					<integer>14</integer>
					<key>EncryptionAlgorithm</key>
					<string>AES-256</string>
					<key>IntegrityAlgorithm</key>
					<string>SHA2-256</string>
					<key>LifeTimeInMinutes</key>
					<integer>1440</integer>
				</dict>
				<key>LocalIdentifier</key>
				<string>%s</string>
				<key>RemoteAddress</key>
				<string>%s</string>
				<key>RemoteIdentifier</key>
				<string>%s</string>
				<key>UseConfigurationAttributeInternalIPSubnet</key>
				<integer>0</integer>
				<key>AuthName</key>
				<string>%s</string>
				<key>AuthPassword</key>
				<string>%s</string>
			</dict>
			<key>OnDemandEnabled</key>
			<integer>0</integer>
			<key>PayloadDescription</key>
			<string>IKEv2 Tunnel Configuration</string>
			<key>PayloadDisplayName</key>
			<string>IKEv2 Tunnel (%s)</string>
			<key>PayloadIdentifier</key>
			<string>com.vpn.ikev2.%s</string>
			<key>PayloadType</key>
			<string>com.apple.vpn.managed</string>
			<key>PayloadUUID</key>
			<string>%s</string>
			<key>PayloadVersion</key>
			<integer>1</integer>
			<key>UserDefinedName</key>
			<string>IKEv2 Tunnel %s</string>
			<key>VPNType</key>
			<string>IKEv2</string>
		</dict>
		<dict>
			<key>PayloadCertificateFileName</key>
			<string>ca-cert.pem</string>
			<key>PayloadContent</key>
			<data>%s</data>
			<key>PayloadDescription</key>
			<string>CA Certificate</string>
			<key>PayloadDisplayName</key>
			<string>Tunnel CA Certificate</string>
			<key>PayloadIdentifier</key>
			<string>com.vpn.ca.%s</string>
			<key>PayloadType</key>
			<string>com.apple.security.root</string>
			<key>PayloadUUID</key>
			<string>%s</string>
			<key>PayloadVersion</key>
			<integer>1</integer>
		</dict>
	</array>
	<key>PayloadDescription</key>
	<string>IKEv2 Tunnel Profile for %s</string>
	<key>PayloadDisplayName</key>
	<string>IKEv2 Tunnel %s</string>
	<key>PayloadIdentifier</key>
	<string>com.vpn.profile.%s</string>
	<key>PayloadOrganization</key>
	<string>IKEv2 Tunnel</string>
	<key>PayloadRemovalDisallowed</key>
	<false/>
	<key>PayloadType</key>
	<string>Configuration</string>
	<key>PayloadUUID</key>
	<string>%s</string>
	<key>PayloadVersion</key>
	<integer>1</integer>
</dict>
</plist>`,
		username, serverIP, serverIP, username, password, // IKEv2 section with auth
		serverIP, payloadUUID, payloadUUID, serverIP, // VPN payload metadata
		caCertData, certUUID, certUUID, // Cert payload
		username, serverIP, profileUUID, profileUUID) // Profile

	return []byte(config)
}

// cleanPEMCert removes PEM headers and newlines
func cleanPEMCert(pem string) string {
	pem = strings.ReplaceAll(pem, "-----BEGIN CERTIFICATE-----", "")
	pem = strings.ReplaceAll(pem, "-----END CERTIFICATE-----", "")
	pem = strings.ReplaceAll(pem, "\n", "")
	return strings.TrimSpace(pem)
}

// GetWindowsInstructions returns setup instructions for Windows
func GetWindowsInstructions(serverIP, username, password string) string {
	return fmt.Sprintf(`# Windows IKEv2 Tunnel Setup

## Шаги настройки:

1. **Откройте настройки VPN**
   - Перейдите в Настройки → Сеть и Интернет → VPN
   - Нажмите "Добавить VPN-подключение"

2. **Заполните данные:**
   - Поставщик VPN: **Windows (встроенный)**
   - Имя подключения: **IKEv2 Tunnel %s**
   - Имя или адрес сервера: **%s**
   - Тип подключения: **IKEv2**
   - Тип данных для входа: **Имя пользователя и пароль**
   - Имя пользователя: **%s**
   - Пароль: **%s**

3. **Нажмите "Сохранить"**

4. **Подключитесь:**
   - Выберите созданное подключение
   - Нажмите "Подключиться"

## Важно:
Если подключение не работает, может потребоваться импорт CA сертификата.
`, serverIP, serverIP, username, password)
}

// GetAndroidInstructions returns setup instructions for Android
func GetAndroidInstructions(serverIP, username, password string) string {
	return fmt.Sprintf(`# Android IKEv2 Tunnel Setup

## Рекомендуемое приложение:
Установите **strongSwan VPN Client** из Google Play Store.

## Шаги настройки:

1. **Откройте strongSwan VPN Client**

2. **Добавьте новый профиль:**
   - Нажмите "ADD VPN PROFILE"

3. **Заполните данные:**
   - Server: **%s**
   - VPN Type: **IKEv2 EAP (Username/Password)**
   - Username: **%s**
   - Password: **%s**
   - CA Certificate: **Select automatically**

4. **Сохраните и подключитесь**

## Альтернатива (встроенный VPN):
Некоторые Android устройства поддерживают IKEv2 нативно:
- Настройки → Сеть → Tunnel → Добавить Tunnel
- Тип: IKEv2/IPSec PSK или IKEv2/IPSec MSCHAPv2
`, serverIP, username, password)
}
