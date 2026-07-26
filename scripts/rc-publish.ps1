param(
    [string]$Zip = ""
)
if ([string]::IsNullOrWhiteSpace($Zip)) {
    throw "ZIP is required: pwsh -File scripts/rc-publish.ps1 -Zip release/DixieData-release-v1.1.4-rc1.zip"
}
if (-not (Test-Path $Zip)) {
    throw "zip not found: $Zip"
}
$sha = ([System.Security.Cryptography.SHA256]::Create()).ComputeHash([System.IO.File]::OpenRead($Zip)) | ForEach-Object { $_.ToString("x2") }; $sha = -join $sha
$leaf = Split-Path $Zip -Leaf
$ver = [regex]::Match($leaf, 'DixieData-release-v(.+?)\.zip').Groups[1].Value
if (-not $ver) {
    throw "could not extract version from $Zip"
}
$asset = "https://github.com/valueforvalue/DixieData/releases/download/v$ver/DixieData-release-$ver.zip"
$manifest = @{
    version       = $ver
    asset_url     = $asset
    sha256        = $sha
    release_notes = "$ver -- see DixieData CHANGELOG.md for the commits since the previous RC."
    published_at  = (Get-Date).ToUniversalTime().ToString("o")
} | ConvertTo-Json -Depth 4
Write-Host $manifest
