#!/usr/bin/env pwsh
# Packages the Windows desktop build: console.exe -> Console-windows-x64.zip.
# Mirrors scripts/build.sh for macOS. Run from anywhere:
#   ./apps/desktop/scripts/build.ps1
#   ./apps/desktop/scripts/build.ps1 -OutDir C:\temp\dist -Locked
param(
    [string]$OutDir = "",
    [string]$Target = "",
    [string]$ZipName = "Console-windows-x64.zip",
    [switch]$Locked,
    [switch]$DebugBuild
)

$ErrorActionPreference = "Stop"

$DesktopDir = Split-Path -Parent $PSScriptRoot
$WorkspaceRoot = Split-Path -Parent (Split-Path -Parent $DesktopDir)

if ([string]::IsNullOrWhiteSpace($OutDir)) {
    $OutDir = Join-Path $DesktopDir "dist"
}

$cargoArgs = @("build", "-p", "console-app")
if (-not $DebugBuild) {
    $cargoArgs += "--release"
}
if (-not [string]::IsNullOrWhiteSpace($Target)) {
    $cargoArgs += @("--target", $Target)
}
if ($Locked) {
    $cargoArgs += "--locked"
}

Write-Host "==> cargo $($cargoArgs -join ' ')"
Push-Location $DesktopDir
try {
    & cargo @cargoArgs
    if ($LASTEXITCODE -ne 0) {
        throw "cargo build failed with exit code $LASTEXITCODE"
    }
}
finally {
    Pop-Location
}

$profile = if ($DebugBuild) { "debug" } else { "release" }
$relative = if ([string]::IsNullOrWhiteSpace($Target)) {
    Join-Path "target" $profile
}
else {
    Join-Path (Join-Path "target" $Target) $profile
}

$candidates = @(
    (Join-Path $DesktopDir (Join-Path $relative "console.exe")),
    (Join-Path $WorkspaceRoot (Join-Path $relative "console.exe"))
)
$binary = $candidates | Where-Object { Test-Path $_ } | Select-Object -First 1
if (-not $binary) {
    $looked = $candidates -join [Environment]::NewLine
    throw "Could not find console.exe. Looked in:`n$looked"
}

New-Item -ItemType Directory -Force -Path $OutDir | Out-Null
$staged = Join-Path $OutDir "console.exe"
Copy-Item -Force $binary $staged

$zipPath = Join-Path $OutDir $ZipName
if (Test-Path $zipPath) {
    Remove-Item -Force $zipPath
}
Compress-Archive -Path $staged -DestinationPath $zipPath -CompressionLevel Optimal

Write-Host "==> Packaged $zipPath"
