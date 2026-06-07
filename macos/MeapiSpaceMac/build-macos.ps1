$ErrorActionPreference = "Stop"

$env:Path = "$env:USERPROFILE\go\bin;$env:Path"

if (-not (Get-Command wails3 -ErrorAction SilentlyContinue)) {
  go install github.com/wailsapp/wails/v3/cmd/wails3@latest
}

Write-Host "Preparing Wails Docker cross compiler..."
wails3 task setup:docker

Write-Host "Building universal macOS app bundle..."
wails3 task darwin:package:universal

Write-Host "Built: $PSScriptRoot\bin\MeapiSpace.app"
