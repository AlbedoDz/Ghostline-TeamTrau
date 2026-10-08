[CmdletBinding()]
param(
    [string]$Version = $(if (Test-Path "$PSScriptRoot\..\VERSION") { (Get-Content "$PSScriptRoot\..\VERSION").Trim() } else { "1.0.1" }),
    [string]$OutputDir = "E:\antigravity",
    [string]$SourceDir = "E:\antigravity\Ghostline_Portable_Client"
)

$ErrorActionPreference = "Stop"

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
