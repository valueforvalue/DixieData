<#
.SYNOPSIS
    Token-saver clean — removes generated artifacts so LLM context stays lean.

.DESCRIPTION
    Deletes:
      - build/bin/, build/release/, build/{darwin,windows,linux}/
      - frontend/dist/, frontend/app.css, frontend/wailsjs/
      - internal/templates/*_templ.go and *_templ.txt
      - DixieData.exe in repo root

    Preserves source files. Does NOT touch .dixiedata/ (user data).
    Safe to re-run.
#>

[CmdletBinding()]
param()

$ErrorActionPreference = "Continue"
$root = (Get-Location).Path

$targets = @(
    'build\bin',
    'build\release',
    'build\darwin',
    'build\windows',
    'build\linux',
    'frontend\dist',
    'frontend\wailsjs',
    'DixieData.exe'
)

Write-Host "Cleaning generated artifacts in $root..." -ForegroundColor Cyan

foreach ($rel in $targets) {
    $abs = Join-Path $root $rel
    if (Test-Path $abs) {
        Remove-Item -Path $abs -Recurse -Force -ErrorAction SilentlyContinue
        Write-Host "  removed $rel" -ForegroundColor DarkGray
    }
}

$frontendCss = Join-Path $root 'frontend\app.css'
if (Test-Path $frontendCss) {
    Remove-Item $frontendCss -Force -ErrorAction SilentlyContinue
    Write-Host "  removed frontend\app.css" -ForegroundColor DarkGray
}

$tplDir = Join-Path $root 'internal\templates'
if (Test-Path $tplDir) {
    Get-ChildItem -Path $tplDir -Filter '*_templ.go' -ErrorAction SilentlyContinue | Remove-Item -Force
    Get-ChildItem -Path $tplDir -Filter '*_templ.txt' -ErrorAction SilentlyContinue | Remove-Item -Force
    Write-Host "  removed internal\templates\*_templ.{go,txt}" -ForegroundColor DarkGray
}

$logDir = Join-Path $root 'build\log'
if (Test-Path $logDir) {
    Get-ChildItem -Path $logDir -Filter '*.log' -ErrorAction SilentlyContinue | Remove-Item -Force
    Write-Host "  cleared build\log\*.log" -ForegroundColor DarkGray
}

Write-Host "Clean complete." -ForegroundColor Green
