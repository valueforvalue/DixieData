param(
    [string]$ReleaseName = "DixieData-demo",
    [int]$Soldiers = 300,
    [long]$Seed = 1865
)

$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $root

$releaseDir = Join-Path $root "release"
$stageDir = Join-Path $releaseDir $ReleaseName
$archivePath = Join-Path $releaseDir ("{0}-{1}.zip" -f $ReleaseName, (Get-Date -Format "yyyy-MM-dd"))

if (Test-Path $stageDir) {
    Remove-Item $stageDir -Recurse -Force
}
if (Test-Path $archivePath) {
    Remove-Item $archivePath -Force
}

New-Item -ItemType Directory -Path $stageDir -Force | Out-Null

wails build -clean

Copy-Item (Join-Path $root "build\\bin\\*") $stageDir -Recurse -Force

$oauthSource = Join-Path $root "google-oauth-defaults.json"
if (-not (Test-Path $oauthSource)) {
    $oauthSource = Join-Path $root "google-oauth-defaults.example.json"
    Write-Warning "google-oauth-defaults.json was not found in the project root. Bundling the example file instead."
}
Copy-Item $oauthSource (Join-Path $stageDir "google-oauth-defaults.json") -Force

$demoDataDir = Join-Path $stageDir ".dixiedata"
go run .\cmd\seed-data -data-dir $demoDataDir -reset -soldiers $Soldiers -seed $Seed

Compress-Archive -Path (Join-Path $stageDir "*") -DestinationPath $archivePath -Force
Write-Host "Demo release ready:" $archivePath
