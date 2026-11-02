#!/usr/bin/env pwsh
# Packages the Windows desktop build: console.exe -> NSIS installer ->
# Console-windows-x64.zip. The zip ships the installer, not the bare exe.
# Mirrors scripts/build.sh for macOS. Run from anywhere:
#   ./apps/desktop/scripts/build.ps1
#   ./apps/desktop/scripts/build.ps1 -OutDir C:\temp\dist -Locked
# Needs NSIS (makensis) on PATH or in its default install location.
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

# DisplayVersion for the uninstaller entry: the first `version = "..."` in the
# desktop workspace manifest is [workspace.package]'s.
$versionMatch = Select-String -Path (Join-Path $DesktopDir "Cargo.toml") `
    -Pattern '^version\s*=\s*"([^"]+)"' | Select-Object -First 1
$version = if ($versionMatch) { $versionMatch.Matches[0].Groups[1].Value } else { "0.0.0" }

# NSIS puts makensis on PATH when installed normally; the explicit paths cover
# a default install whose bin directory never made it there.
$makensisPath = (Get-Command makensis.exe -ErrorAction SilentlyContinue).Source
if (-not $makensisPath) {
    $fallbacks = @(
        $(if ($env:ProgramFiles) { Join-Path $env:ProgramFiles "NSIS\makensis.exe" }),
        $(if (${env:ProgramFiles(x86)}) { Join-Path ${env:ProgramFiles(x86)} "NSIS\makensis.exe" }),
        $(if ($env:LOCALAPPDATA) { Join-Path $env:LOCALAPPDATA "Programs\NSIS\makensis.exe" })
    ) | Where-Object { $_ -and (Test-Path $_) }
    $makensisPath = $fallbacks | Select-Object -First 1
}
if (-not $makensisPath) {
    throw "Could not find makensis.exe. Install NSIS from https://nsis.sourceforge.io/ (or run 'choco install nsis') and try again."
}

$installerName = "Console-Setup-x64.exe"
$installerPath = Join-Path $OutDir $installerName
if (Test-Path $installerPath) {
    Remove-Item -Force $installerPath
}

$nsi = Join-Path $PSScriptRoot "installer.nsi"
$icon = Join-Path $DesktopDir "assets\Console.ico"
Write-Host "==> makensis $installerName"
& $makensisPath "/DAPP_EXE=$staged" "/DAPP_ICON=$icon" "/DVERSION=$version" "/DOUTFILE=$installerPath" $nsi
if ($LASTEXITCODE -ne 0) {
    throw "makensis failed with exit code $LASTEXITCODE"
}
if (-not (Test-Path $installerPath)) {
    throw "makensis did not produce $installerPath"
}

$zipPath = Join-Path $OutDir $ZipName
if (Test-Path $zipPath) {
    Remove-Item -Force $zipPath
}
Compress-Archive -Path $installerPath -DestinationPath $zipPath -CompressionLevel Optimal

Write-Host "==> Packaged $zipPath (contains $installerName)"
