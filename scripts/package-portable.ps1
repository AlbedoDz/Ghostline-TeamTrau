[CmdletBinding()]
param(
    [string]$Version,
    [string]$OutputDir = "E:\antigravity",
    [string]$SourceDir = "E:\antigravity\Ghostline_Portable_Client"
)

$ErrorActionPreference = "Stop"

if ([string]::IsNullOrWhiteSpace($Version)) {
    $vPath = Join-Path $PSScriptRoot "..\VERSION"
    if (Test-Path $vPath) {
        $Version = (Get-Content $vPath -Raw).Trim()
    } elseif (Test-Path "VERSION") {
        $Version = (Get-Content "VERSION" -Raw).Trim()
    } else {
        $Version = "1.0.2"
    }
}

if (-not (Test-Path $SourceDir)) {
    throw "Source portable directory does not exist: $SourceDir"
}

$zipFileName = "Ghostline_Portable_Client_v$Version.zip"
$targetZip = Join-Path $OutputDir $zipFileName

Write-Host "Packaging Portable Client Version: $Version" -ForegroundColor Cyan
Write-Host "Source: $SourceDir"
Write-Host "Target: $targetZip"

if (Test-Path $targetZip) {
    Remove-Item $targetZip -Force
}

Add-Type -AssemblyName System.IO.Compression.FileSystem
[System.IO.Compression.ZipFile]::CreateFromDirectory($SourceDir, $targetZip, [System.IO.Compression.CompressionLevel]::Optimal, $false)

$sizeMB = [math]::Round(((Get-Item $targetZip).Length / 1MB), 2)
Write-Host "Successfully generated: $targetZip ($sizeMB MB)" -ForegroundColor Green
