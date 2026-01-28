package main

import (
	"github.com/vailcody/IKEv2TunnelManager/internal/ui"
)

// Version is set at build time via -ldflags
var Version = "dev"

func main() {
	app := ui.NewApp()
	app.SetVersion(Version)
	app.Run()
}
