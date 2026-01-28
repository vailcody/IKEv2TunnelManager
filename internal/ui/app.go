package ui

import (
	"fmt"
	"image/color"
	"os/exec"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/vailcody/IKEv2TunnelManager/internal/logging"
	"github.com/vailcody/IKEv2TunnelManager/internal/ssh"
	"github.com/vailcody/IKEv2TunnelManager/internal/storage"
	"github.com/vailcody/IKEv2TunnelManager/internal/vpn"
)

// App is the main application
type App struct {
	fyneApp    fyne.App
	mainWindow fyne.Window

	// Storage and Config
	store  *storage.Storage
	config *storage.AppConfig
	logger *logging.Logger

	// Server configs
	server1Config *ssh.ServerConfig
	server2Config *ssh.ServerConfig

	// SSH clients
	client1 *ssh.Client
	client2 *ssh.Client

	// UI components
	logWidget    *widget.RichText
	logBinding   binding.String
	statusWidget *widget.Label
	tabs         *container.AppTabs

	// State
	mu        sync.Mutex
	isRunning bool
	version   string
}

// NewApp creates the application
func NewApp() *App {
	// Initialize storage
	store, err := storage.New()
	if err != nil {
		fmt.Printf("Warning: failed to initialize storage: %v\n", err)
	}

	// Load or create config
	config := storage.NewAppConfig()
	if store != nil {
		if loaded, err := store.Load(); err == nil {
			config = loaded
		} else {
			fmt.Printf("Warning: failed to load config: %v\n", err)
		}
	}

	// Initialize logger
	var logger *logging.Logger
	if store != nil {
		logger, err = logging.New(store.GetLogDir())
		if err != nil {
			fmt.Printf("Warning: failed to create logger: %v\n", err)
		}
	}

	a := &App{
		fyneApp:       app.New(),
		store:         store,
		config:        config,
		logger:        logger,
		server1Config: &ssh.ServerConfig{Port: 22},
		server2Config: &ssh.ServerConfig{Port: 22},
	}

	// Apply loaded config to runtime configs
	if len(config.Servers) >= 2 {
		s1 := config.Servers[0]
		a.server1Config.Host = s1.Host
		if s1.Port > 0 {
			a.server1Config.Port = s1.Port
		}
		a.server1Config.User = s1.User
		a.server1Config.Password = s1.Password
		a.server1Config.KeyPath = s1.KeyPath

		s2 := config.Servers[1]
		a.server2Config.Host = s2.Host
		if s2.Port > 0 {
			a.server2Config.Port = s2.Port
		}
		a.server2Config.User = s2.User
		a.server2Config.Password = s2.Password
		a.server2Config.KeyPath = s2.KeyPath
	}

	a.mainWindow = a.fyneApp.NewWindow("IKEv2 Tunnel Manager")
	a.mainWindow.Resize(fyne.NewSize(900, 700))

	return a
}

// SetVersion sets the application version and updates the window title
func (a *App) SetVersion(version string) {
	a.version = version
	if version != "" && version != "dev" {
		a.mainWindow.SetTitle(fmt.Sprintf("IKEv2 Tunnel Manager %s", version))
	}
}

// Run starts the application
func (a *App) Run() {
	a.buildUI()
	a.mainWindow.ShowAndRun()
}

func (a *App) buildUI() {
	// Create tabs
	a.tabs = container.NewAppTabs(
		container.NewTabItemWithIcon("Connection", theme.ComputerIcon(), a.createConnectionTab()),
		container.NewTabItemWithIcon("Status", theme.InfoIcon(), a.createStatusTab()),
		container.NewTabItemWithIcon("Users", theme.AccountIcon(), a.createUsersTab()),
		container.NewTabItemWithIcon("Logs", theme.DocumentIcon(), a.createLogsTab()),
	)

	a.tabs.SetTabLocation(container.TabLocationTop)

	a.mainWindow.SetContent(a.tabs)
}

func (a *App) createConnectionTab() fyne.CanvasObject {
	// SSH Key Management
	keyPathEntry := widget.NewEntry()
	if a.store != nil {
		keyPathEntry.SetText(a.store.GetDefaultKeyPath())
	} else {
		keyPathEntry.SetPlaceHolder("Path to SSH key")
	}

	generateKeyBtn := widget.NewButton("🔑 Generate SSH Key", func() {
		go a.generateKey(keyPathEntry.Text)
	})

	copyKeysBtn := widget.NewButton("📤 Copy Key to Servers", func() {
		go a.copyKeyToAllServers(keyPathEntry.Text)
	})

	// Server 1 section
	server1Title := widget.NewLabel("Server 1 (Entry Point)")
	server1Title.TextStyle = fyne.TextStyle{Bold: true}
	server1PingLabel := widget.NewLabel("⚫ Not configured")

	server1Host := widget.NewEntry()
	server1Host.SetPlaceHolder("IP address or hostname")
	server1Host.SetText(a.server1Config.Host)
	server1Host.OnChanged = func(s string) {
		a.server1Config.Host = s
		a.saveConfig()
		if s != "" {
			go a.updatePingStatus(s, server1PingLabel)
		} else {
			server1PingLabel.SetText("⚫ Not configured")
		}
	}
	// Initial ping if host is set
	if a.server1Config.Host != "" {
		go a.updatePingStatus(a.server1Config.Host, server1PingLabel)
	}

	server1User := widget.NewEntry()
	server1User.SetPlaceHolder("root")
	server1User.SetText(a.server1Config.User)
	server1User.OnChanged = func(s string) {
		a.server1Config.User = s
		a.saveConfig()
	}

	server1Pass := widget.NewPasswordEntry()
	server1Pass.SetPlaceHolder("Password (optional if using key)")
	server1Pass.SetText(a.server1Config.Password)
	server1Pass.OnChanged = func(s string) {
		a.server1Config.Password = s
		a.saveConfig()
	}

	server1Form := container.NewVBox(
		container.NewHBox(server1Title, server1PingLabel),
		container.NewGridWithColumns(2,
			widget.NewLabel("Host:"), server1Host,
			widget.NewLabel("User:"), server1User,
			widget.NewLabel("Password:"), server1Pass,
		),
	)

	// Server 2 section
	server2Title := widget.NewLabel("Server 2 (Exit Node)")
	server2Title.TextStyle = fyne.TextStyle{Bold: true}
	server2PingLabel := widget.NewLabel("⚫ Not configured")

	server2Host := widget.NewEntry()
	server2Host.SetPlaceHolder("IP address or hostname")
	server2Host.SetText(a.server2Config.Host)
	server2Host.OnChanged = func(s string) {
		a.server2Config.Host = s
		a.saveConfig()
		if s != "" {
			go a.updatePingStatus(s, server2PingLabel)
		} else {
			server2PingLabel.SetText("⚫ Not configured")
		}
	}
	// Initial ping if host is set
	if a.server2Config.Host != "" {
		go a.updatePingStatus(a.server2Config.Host, server2PingLabel)
	}

	server2User := widget.NewEntry()
	server2User.SetPlaceHolder("root")
	server2User.SetText(a.server2Config.User)
	server2User.OnChanged = func(s string) {
		a.server2Config.User = s
		a.saveConfig()
	}

	server2Pass := widget.NewPasswordEntry()
	server2Pass.SetPlaceHolder("Password (optional if using key)")
	server2Pass.SetText(a.server2Config.Password)
	server2Pass.OnChanged = func(s string) {
		a.server2Config.Password = s
		a.saveConfig()
	}

	server2Form := container.NewVBox(
		container.NewHBox(server2Title, server2PingLabel),
		container.NewGridWithColumns(2,
			widget.NewLabel("Host:"), server2Host,
			widget.NewLabel("User:"), server2User,
			widget.NewLabel("Password:"), server2Pass,
		),
	)

	// Buttons
	testBtn := widget.NewButton("Test Connections", func() {
		go a.testConnections()
	})
	testBtn.Importance = widget.MediumImportance

	setupBtn := widget.NewButton("Setup IKEv2 Tunnel", func() {
		go a.setupVPN()
	})
	setupBtn.Importance = widget.HighImportance

	buttons := container.NewHBox(testBtn, setupBtn)

	// Status
	a.statusWidget = widget.NewLabel("Ready")

	// Global Key Management Section
	keyPathRow := container.NewBorder(nil, nil, widget.NewLabel("Default Key Path:"), nil, keyPathEntry)
	keyButtons := container.NewGridWithColumns(2, generateKeyBtn, copyKeysBtn)

	keyMgmt := container.NewVBox(
		widget.NewLabelWithStyle("SSH Key Management", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		keyPathRow,
		keyButtons,
		widget.NewSeparator(),
	)

	return container.NewVBox(
		keyMgmt,
		server1Form,
		widget.NewSeparator(),
		server2Form,
		widget.NewSeparator(),
		buttons,
		a.statusWidget,
	)
}

func (a *App) updatePingStatus(host string, label *widget.Label) {
	fyne.Do(func() {
		label.SetText("🟡 Pinging...")
	})

	start := time.Now()
	cmd := fmt.Sprintf("ping -c 1 -W 2 %s", host)
	output, err := runLocalCommand(cmd)
	elapsed := time.Since(start)

	fyne.Do(func() {
		if err != nil || !strings.Contains(output, "1 packets received") && !strings.Contains(output, "1 received") {
			label.SetText("🔴 Unreachable")
			return
		}

		pingMs := elapsed.Milliseconds()
		label.SetText(fmt.Sprintf("🟢 %dms", pingMs))
	})
}

func runLocalCommand(cmd string) (string, error) {
	out, err := exec.Command("sh", "-c", cmd).CombinedOutput()
	return string(out), err
}

func (a *App) createStatusTab() fyne.CanvasObject {
	server1Status := widget.NewLabel("Server 1: Not connected")
	server2Status := widget.NewLabel("Server 2: Not connected")
	tunnelStatus := widget.NewLabel("Tunnel: Unknown")
	clientsStatus := widget.NewLabel("Active clients: 0")

	refreshBtn := widget.NewButton("Refresh Status", func() {
		go func() {
			a.Log("Refreshing status...")

			if a.client1 == nil {
				a.client1 = ssh.NewClient(a.server1Config)
			}
			// vpn.GetStatus handles connection internally if needed
			status, err := vpn.GetStatus(a.client1)
			fyne.Do(func() {
				if err != nil {
					server1Status.SetText(fmt.Sprintf("Server 1: Error - %v", err))
				} else if status.Connected {
					server1Status.SetText(fmt.Sprintf("Server 1: Running (IP: %s)", status.ServerIP))
					clientsStatus.SetText(fmt.Sprintf("Active clients: %d", status.ActiveClients))
					if status.TunnelActive {
						tunnelStatus.SetText("Tunnel: Active")
					} else {
						tunnelStatus.SetText("Tunnel: Not active")
					}
				} else {
					server1Status.SetText("Server 1: StrongSwan not running")
				}
			})

			if a.client2 == nil {
				a.client2 = ssh.NewClient(a.server2Config)
			}
			// vpn.GetStatus handles connection internally if needed
			status2, err2 := vpn.GetStatus(a.client2)
			fyne.Do(func() {
				if err2 != nil {
					server2Status.SetText(fmt.Sprintf("Server 2: Error - %v", err2))
				} else if status2.Connected {
					server2Status.SetText(fmt.Sprintf("Server 2: Running (IP: %s)", status2.ServerIP))
				} else {
					server2Status.SetText("Server 2: StrongSwan not running")
				}
			})

			a.Log("Status refreshed")
		}()
	})

	restartBtn := widget.NewButton("Restart Tunnel", func() {
		go func() {
			a.Log("Restarting tunnel on both servers...")
			if a.client1 != nil {
				vpn.RestartVPN(a.client1)
			}
			if a.client2 != nil {
				vpn.RestartVPN(a.client2)
			}
			a.Log("Tunnel restarted")
		}()
	})
	restartBtn.Importance = widget.MediumImportance

	return container.NewVBox(
		widget.NewLabel("Tunnel Status"),
		widget.NewSeparator(),
		server1Status,
		server2Status,
		tunnelStatus,
		clientsStatus,
		widget.NewSeparator(),
		container.NewHBox(refreshBtn, restartBtn),
	)
}

func (a *App) createUsersTab() fyne.CanvasObject {
	var users []vpn.User
	var selectedUser string
	usersContainer := container.NewVBox()

	var refreshList func()
	refreshList = func() {
		if a.client1 == nil {
			a.client1 = ssh.NewClient(a.server1Config)
		}
		if !a.client1.IsConnected() {
			a.Log("Connecting to Server 1 to list users...")
			if err := a.client1.Connect(); err != nil {
				a.Errorf("Failed to connect to Server 1: %v", err)
				return
			}
		}
		um := vpn.NewUserManager(a.client1, a)
		var err error
		users, err = um.ListUsers()
		fyne.Do(func() {
			if err != nil {
				a.Errorf("Failed to list users: %v", err)
				return
			}

			usersContainer.RemoveAll()
			for _, u := range users {
				username := u.Username
				userRow := a.createUserRow(username, &selectedUser, refreshList)
				usersContainer.Add(userRow)
			}
			usersContainer.Refresh()
		})
	}

	usernameEntry := widget.NewEntry()
	usernameEntry.SetPlaceHolder("Username")

	passwordEntry := widget.NewPasswordEntry()
	passwordEntry.SetPlaceHolder("Password (leave empty for auto)")

	addBtn := widget.NewButton("Add User", func() {
		go func() {
			if a.client1 == nil {
				a.client1 = ssh.NewClient(a.server1Config)
			}
			if !a.client1.IsConnected() {
				a.Log("Connecting to Server 1 to add user...")
				if err := a.client1.Connect(); err != nil {
					a.Errorf("Failed to connect to Server 1: %v", err)
					return
				}
			}
			um := vpn.NewUserManager(a.client1, a)
			_, err := um.AddUser(usernameEntry.Text, passwordEntry.Text)
			if err != nil {
				a.Errorf("Failed to add user: %v", err)
				return
			}
			usernameEntry.SetText("")
			passwordEntry.SetText("")
			refreshList()
		}()
	})
	addBtn.Importance = widget.HighImportance

	deleteBtn := widget.NewButton("Delete Selected", func() {
		if selectedUser == "" {
			a.Log("Select a user to delete")
			return
		}
		go func() {
			um := vpn.NewUserManager(a.client1, a)
			if err := um.RemoveUser(selectedUser); err != nil {
				a.Errorf("Failed to delete user: %v", err)
				return
			}
			selectedUser = ""
			refreshList()
		}()
	})
	deleteBtn.Importance = widget.DangerImportance

	refreshBtn := widget.NewButton("Refresh", func() {
		go refreshList()
	})

	return container.NewBorder(
		container.NewVBox(
			widget.NewLabel("Tunnel Users"),
			widget.NewSeparator(),
			container.NewGridWithColumns(2,
				usernameEntry,
				passwordEntry,
			),
			container.NewHBox(addBtn, deleteBtn, refreshBtn),
		),
		nil, nil, nil,
		container.NewScroll(usersContainer),
	)
}

func (a *App) createUserRow(username string, selectedUser *string, refreshList func()) fyne.CanvasObject {
	userLabel := widget.NewLabel(username)
	userLabel.TextStyle = fyne.TextStyle{Bold: true}

	selectBtn := widget.NewButton("Select", func() {
		*selectedUser = username
		a.Log(fmt.Sprintf("Selected user: %s", username))
	})

	configBtn := widget.NewButton("📱 .mobileconfig", func() {
		go a.downloadMobileConfig(username)
	})
	configBtn.Importance = widget.HighImportance

	instructionsBtn := widget.NewButton("📋 Instructions", func() {
		go a.showInstructions(username)
	})

	return container.NewHBox(
		userLabel,
		selectBtn,
		configBtn,
		instructionsBtn,
	)
}

func (a *App) downloadMobileConfig(username string) {
	if a.client1 == nil || !a.client1.IsConnected() {
		a.Log("Not connected to Server 1")
		return
	}

	um := vpn.NewUserManager(a.client1, a)
	password, err := um.GetUserPassword(username)
	if err != nil {
		a.Errorf("Failed to get user password: %v", err)
		return
	}

	// Get CA certificate from server
	caCert, err := a.client1.ReadFile("/etc/ipsec.d/cacerts/ca-cert.pem")
	if err != nil {
		a.Errorf("Failed to read CA certificate: %v", err)
		return
	}

	// Get server IP
	serverIP := a.server1Config.Host

	config := vpn.GenerateMobileConfig(username, password, serverIP, string(caCert))

	// Save file dialog
	fyne.Do(func() {
		saveDialog := dialog.NewFileSave(func(writer fyne.URIWriteCloser, err error) {
			if err != nil {
				a.Errorf("Error saving file: %v", err)
				return
			}
			if writer == nil {
				return
			}
			defer writer.Close()
			writer.Write(config)
			a.Log(fmt.Sprintf("Saved .mobileconfig for %s", username))
		}, a.mainWindow)
		saveDialog.SetFileName(fmt.Sprintf("%s.mobileconfig", username))
		saveDialog.Show()
	})
}

func (a *App) showInstructions(username string) {
	if a.client1 == nil || !a.client1.IsConnected() {
		a.Log("Not connected to Server 1")
		return
	}

	um := vpn.NewUserManager(a.client1, a)
	password, err := um.GetUserPassword(username)
	if err != nil {
		a.Errorf("Failed to get user password: %v", err)
		return
	}

	serverIP := a.server1Config.Host

	windowsInstructions := vpn.GetWindowsInstructions(serverIP, username, password)
	androidInstructions := vpn.GetAndroidInstructions(serverIP, username, password)

	windowsText := widget.NewMultiLineEntry()
	windowsText.SetText(windowsInstructions)
	windowsText.Wrapping = fyne.TextWrapWord
	windowsText.Disable()

	androidText := widget.NewMultiLineEntry()
	androidText.SetText(androidInstructions)
	androidText.Wrapping = fyne.TextWrapWord
	androidText.Disable()

	tabs := container.NewAppTabs(
		container.NewTabItem("Windows", container.NewScroll(windowsText)),
		container.NewTabItem("Android", container.NewScroll(androidText)),
	)

	fyne.Do(func() {
		instructionsWindow := a.fyneApp.NewWindow(fmt.Sprintf("Tunnel Instructions - %s", username))
		instructionsWindow.SetContent(tabs)
		instructionsWindow.Resize(fyne.NewSize(500, 400))
		instructionsWindow.Show()
	})
}

func (a *App) createLogsTab() fyne.CanvasObject {
	a.logWidget = widget.NewRichText()
	a.logWidget.Wrapping = fyne.TextWrapWord

	// Connect logger to UI widget
	if a.logger != nil {
		a.logger.AddWriter(&logging.UIWriter{
			WriteFunc: func(s string) {
				if a.logWidget == nil {
					return
				}

				// Parse the log line for level
				var color fyne.CanvasObject
				_ = color // unused but following pattern if needed

				segments := []widget.RichTextSegment{}

				// Split into lines if multiple lines come at once
				lines := strings.Split(s, "\n")
				for _, line := range lines {
					if line == "" {
						continue
					}

					if strings.Contains(line, "ERROR") || strings.Contains(line, "failed") || strings.Contains(line, "Error") {
						// We'll use a custom segment for errors
						segments = append(segments, &widget.TextSegment{
							Text: line + "\n",
							Style: widget.RichTextStyle{
								ColorName: theme.ColorNameError,
								TextStyle: fyne.TextStyle{Bold: true, Monospace: true},
							},
						})
					} else if strings.Contains(line, "OK") || strings.Contains(line, "success") || strings.Contains(line, "ready") {
						segments = append(segments, &widget.TextSegment{
							Text: line + "\n",
							Style: widget.RichTextStyle{
								ColorName: theme.ColorNameSuccess,
								TextStyle: fyne.TextStyle{Bold: true, Monospace: true},
							},
						})
					} else {
						segments = append(segments, &widget.TextSegment{
							Text: line + "\n",
							Style: widget.RichTextStyle{
								TextStyle: fyne.TextStyle{Monospace: true},
							},
						})
					}
				}

				// Update widget in main thread
				fyne.Do(func() {
					a.logWidget.Segments = append(a.logWidget.Segments, segments...)

					// Truncate if too many segments
					if len(a.logWidget.Segments) > 2000 {
						a.logWidget.Segments = a.logWidget.Segments[1000:]
					}

					a.logWidget.Refresh()
				})
			},
		})
	}

	clearBtn := widget.NewButtonWithIcon("Clear Logs", theme.DeleteIcon(), func() {
		a.logWidget.Segments = nil
		a.logWidget.Refresh()
	})

	openLogsDirBtn := widget.NewButton("Open Logs Folder", func() {
		if a.store != nil {
			exec.Command("open", a.store.GetLogDir()).Start()
		}
	})

	fetchServer1Logs := widget.NewButton("Fetch Server 1 Logs", func() {
		go func() {
			if a.client1 == nil || !a.client1.IsConnected() {
				a.Log("Not connected to Server 1")
				return
			}
			logs, err := vpn.GetDetailedLogs(a.client1, 50)
			if err != nil {
				a.Errorf("Failed to fetch logs: %v", err)
				return
			}
			a.Log("=== Server 1 Logs ===")
			a.Log(logs)
		}()
	})

	fetchServer2Logs := widget.NewButton("Fetch Server 2 Logs", func() {
		go func() {
			if a.client2 == nil || !a.client2.IsConnected() {
				a.Log("Not connected to Server 2")
				return
			}
			logs, err := vpn.GetDetailedLogs(a.client2, 50)
			if err != nil {
				a.Errorf("Failed to fetch logs: %v", err)
				return
			}
			a.Log("=== Server 2 Logs ===")
			a.Log(logs)
		}()
	})

	buttons := container.NewVBox(
		container.NewHBox(clearBtn, openLogsDirBtn),
		container.NewHBox(fetchServer1Logs, fetchServer2Logs),
	)

	// Wrap log widget in a container with a dark background for better contrast
	logContent := container.NewScroll(a.logWidget)

	// Create a dark "terminal" background
	bg := canvas.NewRectangle(color.NRGBA{R: 15, G: 15, B: 20, A: 255})
	bg.StrokeColor = color.NRGBA{R: 40, G: 40, B: 50, A: 255}
	bg.StrokeWidth = 1

	terminalContainer := container.NewStack(bg, container.NewPadded(logContent))

	return container.NewBorder(
		container.NewPadded(buttons),
		nil, nil, nil,
		container.NewPadded(terminalContainer),
	)
}

func (a *App) testConnections() {
	a.setStatus("Testing connections...")
	a.Log("Testing connection to Server 1...")

	client1 := ssh.NewClient(a.server1Config)
	if err := client1.TestConnection(); err != nil {
		a.Errorf("Server 1 connection failed: %v", err)
		a.setStatus("Server 1 connection failed")
		return
	}
	a.Log("Server 1: OK")

	a.Log("Testing connection to Server 2...")
	client2 := ssh.NewClient(a.server2Config)
	if err := client2.TestConnection(); err != nil {
		a.Errorf("Server 2 connection failed: %v", err)
		a.setStatus("Server 2 connection failed")
		return
	}
	a.Log("Server 2: OK")

	a.setStatus("Both connections successful!")
	a.Log("All connections tested successfully")
}

func (a *App) setupVPN() {
	a.mu.Lock()
	if a.isRunning {
		a.mu.Unlock()
		a.Log("Setup is already running")
		return
	}
	a.isRunning = true
	a.mu.Unlock()

	defer func() {
		a.mu.Lock()
		a.isRunning = false
		a.mu.Unlock()
	}()

	a.setStatus("Setting up IKEv2 tunnel...")
	a.Log("Starting IKEv2 tunnel setup...")

	config := &vpn.SetupConfig{
		Server1:       a.server1Config,
		Server2:       a.server2Config,
		VPNSubnet:     "10.10.10.0/24",
		TunnelSubnet:  "10.10.20.0/24",
		Server1Domain: a.server1Config.Host,
		Server2Domain: a.server2Config.Host,
	}

	manager := vpn.NewManager(config, a)

	if err := manager.SetupAll(); err != nil {
		a.Errorf("Setup failed: %v", err)
		a.setStatus("Setup failed!")
		return
	}

	// Store clients for later use
	a.client1 = ssh.NewClient(a.server1Config)
	a.client1.Connect()
	a.client2 = ssh.NewClient(a.server2Config)
	a.client2.Connect()

	a.setStatus("IKEv2 tunnel setup completed!")
	a.Log("IKEv2 tunnel is ready!")
}

func (a *App) setStatus(status string) {
	if a.statusWidget != nil {
		fyne.Do(func() {
			a.statusWidget.SetText(status)
		})
	}
}

// Log implements vpn.Logger
func (a *App) Log(message string) {
	if a.logger != nil {
		a.logger.Log(message)
	} else {
		// Fallback if logger is not initialized
		timestamp := time.Now().Format("15:04:05")
		text := fmt.Sprintf("[%s] %s\n", timestamp, message)
		fmt.Print(text)

		if a.logWidget != nil {
			fyne.Do(func() {
				a.logWidget.Segments = append(a.logWidget.Segments, &widget.TextSegment{
					Text:  text,
					Style: widget.RichTextStyleParagraph,
				})
				a.logWidget.Refresh()
			})
		}
	}
}

// Logf implements vpn.Logger
func (a *App) Logf(format string, args ...interface{}) {
	a.Log(fmt.Sprintf(format, args...))
}

// Error implements vpn.Logger
func (a *App) Error(message string) {
	if a.logger != nil {
		a.logger.Error(message)
	} else {
		a.Log("ERROR: " + message)
	}
}

// Errorf implements vpn.Logger
func (a *App) Errorf(format string, args ...interface{}) {
	if a.logger != nil {
		a.logger.Errorf(format, args...)
	} else {
		a.Error(fmt.Sprintf(format, args...))
	}
}

// --- Helper Methods ---

func (a *App) saveConfig() {
	if a.store == nil || a.config == nil {
		return
	}

	if len(a.config.Servers) >= 2 {
		a.config.Servers[0].Host = a.server1Config.Host
		a.config.Servers[0].Port = a.server1Config.Port
		a.config.Servers[0].User = a.server1Config.User
		a.config.Servers[0].Password = a.server1Config.Password
		a.config.Servers[0].KeyPath = a.server1Config.KeyPath

		a.config.Servers[1].Host = a.server2Config.Host
		a.config.Servers[1].Port = a.server2Config.Port
		a.config.Servers[1].User = a.server2Config.User
		a.config.Servers[1].Password = a.server2Config.Password
		a.config.Servers[1].KeyPath = a.server2Config.KeyPath
	}

	if err := a.store.Save(a.config); err != nil {
		fmt.Printf("Error saving config: %v\n", err)
	}
}

func (a *App) generateKey(path string) {
	if path == "" {
		a.Error("Please specify a path for the SSH key")
		return
	}

	a.Log("Generating SSH key...")
	keygen := ssh.NewKeyGenerator()

	if keygen.KeyExists(path) {
		fyne.Do(func() {
			dialog.ShowConfirm("Key already exists", "Overwrite existing key?", func(overwrite bool) {
				if overwrite {
					go func() {
						if err := keygen.GenerateKey(path); err != nil {
							a.Errorf("Failed to generate key: %v", err)
						} else {
							a.Log("SSH key generated successfully!")
						}
					}()
				} else {
					a.Log("Key generation cancelled")
				}
			}, a.mainWindow)
		})
	} else {
		if err := keygen.GenerateKey(path); err != nil {
			a.Errorf("Failed to generate key: %v", err)
		} else {
			a.Log("SSH key generated successfully!")
		}
	}
}

func (a *App) copyKeyToAllServers(keyPath string) {
	if keyPath == "" {
		a.Error("Key path is required")
		return
	}

	a.Log("Starting SSH key distribution...")

	keygen := ssh.NewKeyGenerator()
	if !keygen.KeyExists(keyPath) {
		a.Errorf("SSH key not found at %s. Please generate one first.", keyPath)
		fyne.Do(func() {
			dialog.ShowError(fmt.Errorf("SSH key not found at %s. Please generate one first.", keyPath), a.mainWindow)
		})
		return
	}

	pubKey, err := keygen.GetPublicKey(keyPath)
	if err != nil {
		a.Errorf("Failed to read public key: %v", err)
		return
	}

	configs := []*ssh.ServerConfig{a.server1Config, a.server2Config}
	successCount := 0
	attemptCount := 0

	for i, config := range configs {
		serverName := fmt.Sprintf("Server %d", i+1)
		if config.Host == "" {
			a.Logf("%s not configured, skipping key copy.", serverName)
			continue
		}

		attemptCount++
		a.Logf("Copying key to %s (%s)...", serverName, config.Host)

		client := ssh.NewClient(config)
		if err := client.Connect(); err != nil {
			a.Errorf("Failed to connect to %s: %v", serverName, err)
			continue
		}

		if err := keygen.CopyKeyToServer(client, pubKey); err != nil {
			client.Close()
			a.Errorf("Failed to copy key to %s: %v", serverName, err)
			continue
		}
		client.Close()

		config.Password = "" // Clear password as we have key now
		config.KeyPath = keyPath
		successCount++
		a.Logf("Key installed on %s", serverName)
	}

	if attemptCount > 0 {
		a.saveConfig()
		fyne.Do(func() {
			if successCount == attemptCount {
				msg := fmt.Sprintf("Successfully copied SSH key to %d configured servers!", successCount)
				a.Log(msg)
				dialog.ShowInformation("Success", msg, a.mainWindow)
			} else {
				msg := fmt.Sprintf("Copied SSH key to %d/%d servers. Check logs for errors.", successCount, attemptCount)
				a.Log(msg)
				dialog.ShowInformation("Partial Success", msg, a.mainWindow)
			}
		})
	} else {
		fyne.Do(func() {
			a.Log("No servers configured to copy keys to.")
			dialog.ShowInformation("Info", "No servers configured.", a.mainWindow)
		})
	}
}
