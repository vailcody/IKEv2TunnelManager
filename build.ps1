# Build script for IKEv2 Tunnel Manager (Windows)
# Requires: Go 1.21+, MSYS2 with mingw-w64-ucrt-x86_64-gcc
#
# If script execution is disabled:
# powershell -ExecutionPolicy Bypass -File .\build.ps1

$ErrorActionPreference = "Stop"

# Add MSYS2 UCRT64 GCC to PATH (if exists)
$msys2Gcc = "C:\msys64\ucrt64\bin"
if (Test-Path "$msys2Gcc\gcc.exe") {
    $env:PATH = "$msys2Gcc;$env:PATH"
    Write-Host "Using GCC from: $msys2Gcc"
} else {
    Write-Warning "GCC not found at $msys2Gcc"
    Write-Host "Install: winget install MSYS2.MSYS2"
    Write-Host "Then run in MSYS2 UCRT64: pacman -S mingw-w64-ucrt-x86_64-gcc"
    exit 1
}

# Enable CGO (required for Fyne)
go env -w CGO_ENABLED=1 | Out-Null

Write-Host "Building..."
go build -ldflags="-H windowsgui -s -w" -o tunnelmanager.exe ./cmd/vpnmanager

if ($LASTEXITCODE -eq 0) {
    Write-Host "Done! Run: .\tunnelmanager.exe"
} else {
    exit $LASTEXITCODE
}
