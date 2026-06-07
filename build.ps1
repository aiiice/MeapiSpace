$ErrorActionPreference = "Stop"

$outDir = Join-Path $PSScriptRoot "dist"
New-Item -ItemType Directory -Force $outDir | Out-Null
Remove-Item (Join-Path $outDir "sub2api-quota.exe") -ErrorAction SilentlyContinue

function New-IcoFromPng {
  param(
    [Parameter(Mandatory = $true)][string]$SourcePng,
    [Parameter(Mandatory = $true)][string]$OutIco
  )

  Add-Type -AssemblyName System.Drawing
  $sizes = @(16, 24, 32, 48, 64, 128, 256)
  $source = [System.Drawing.Image]::FromFile($SourcePng)
  $frames = @()
  try {
    foreach ($size in $sizes) {
      $bitmap = [System.Drawing.Bitmap]::new($size, $size, [System.Drawing.Imaging.PixelFormat]::Format32bppArgb)
      $graphics = [System.Drawing.Graphics]::FromImage($bitmap)
      $stream = [System.IO.MemoryStream]::new()
      try {
        $graphics.Clear([System.Drawing.Color]::Transparent)
        $graphics.InterpolationMode = [System.Drawing.Drawing2D.InterpolationMode]::HighQualityBicubic
        $graphics.SmoothingMode = [System.Drawing.Drawing2D.SmoothingMode]::AntiAlias
        $graphics.PixelOffsetMode = [System.Drawing.Drawing2D.PixelOffsetMode]::HighQuality
        $graphics.DrawImage($source, 0, 0, $size, $size)
        $bitmap.Save($stream, [System.Drawing.Imaging.ImageFormat]::Png)
        $frames += [pscustomobject]@{
          Size = $size
          Bytes = $stream.ToArray()
        }
      } finally {
        $stream.Dispose()
        $graphics.Dispose()
        $bitmap.Dispose()
      }
    }
  } finally {
    $source.Dispose()
  }

  $file = [System.IO.File]::Open($OutIco, [System.IO.FileMode]::Create, [System.IO.FileAccess]::Write, [System.IO.FileShare]::None)
  $writer = [System.IO.BinaryWriter]::new($file)
  try {
    $writer.Write([uint16]0)
    $writer.Write([uint16]1)
    $writer.Write([uint16]$frames.Count)
    $offset = 6 + (16 * $frames.Count)
    foreach ($frame in $frames) {
      $entrySize = if ($frame.Size -ge 256) { 0 } else { $frame.Size }
      $writer.Write([byte]$entrySize)
      $writer.Write([byte]$entrySize)
      $writer.Write([byte]0)
      $writer.Write([byte]0)
      $writer.Write([uint16]1)
      $writer.Write([uint16]32)
      $writer.Write([uint32]$frame.Bytes.Length)
      $writer.Write([uint32]$offset)
      $offset += $frame.Bytes.Length
    }
    foreach ($frame in $frames) {
      $writer.Write([byte[]]$frame.Bytes)
    }
  } finally {
    $writer.Dispose()
    $file.Dispose()
  }
}

$iconPng = Join-Path $PSScriptRoot "ico.png"
$appIcon = Join-Path $PSScriptRoot "app.ico"
New-IcoFromPng -SourcePng $iconPng -OutIco $appIcon

$rsrc = Join-Path (go env GOPATH) "bin\rsrc.exe"
if (-not (Test-Path $rsrc)) {
  go install github.com/akavel/rsrc@latest
}

& $rsrc -manifest (Join-Path $PSScriptRoot "app.manifest") -ico $appIcon -o (Join-Path $PSScriptRoot "rsrc.syso")

$env:CGO_ENABLED = "0"
go build -trimpath -ldflags "-H windowsgui -s -w" -o (Join-Path $outDir "MeapiSpace.exe") .

Write-Host "Built: $(Join-Path $outDir 'MeapiSpace.exe')"
