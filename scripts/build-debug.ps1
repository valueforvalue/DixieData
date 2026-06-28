param(
    [switch]$Run,
    [Parameter(ValueFromRemainingArguments = $true)]
    [string[]]$AppArgs
)

$scriptRoot = Split-Path -Parent $MyInvocation.MyCommand.Path
. (Join-Path $scriptRoot "build-common.ps1")

$root = Get-DixieDataRoot -StartPath $scriptRoot

Set-DixieDataBuildLocation -Root $root
# -Debug toggles on the Wails -debug flag (DevTools + context menu
# in the WebView2) AND adds -gcflags="all=-N -l" to the Go ldflags so
# the binary has symbols and unoptimised frames suitable for
# attaching a debugger (dlv, VS Code, etc.). Without this, make
# debug was indistinguishable from make release.
Invoke-DixieDataBuild -Root $root -DebugBuild

$launcherPath = Write-DixieDataDebugLauncher -Root $root
Write-Host "Debug build ready:" (Join-Path (Get-DixieDataBuildBinDir -Root $root) "DixieData.exe")
Write-Host "Debug launcher ready:" $launcherPath

if ($Run) {
    & $launcherPath @AppArgs
    exit $LASTEXITCODE
}
