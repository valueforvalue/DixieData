param(
    [string]$ReleaseName = "DixieData-demo",
    [int]$Soldiers = 300,
    [long]$Seed = 1865
)

$root = Split-Path -Parent $MyInvocation.MyCommand.Path
. (Join-Path $root "build-common.ps1")

Set-DixieDataBuildLocation -Root $root

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

Invoke-DixieDataBuild -Root $root -AllowExampleOAuthDefaults

Copy-Item (Join-Path $root "build\\bin\\*") $stageDir -Recurse -Force

$demoDataDir = Join-Path $stageDir ".dixiedata"
go run .\cmd\seed-data -data-dir $demoDataDir -reset -soldiers $Soldiers -seed $Seed

Compress-Archive -Path (Join-Path $stageDir "*") -DestinationPath $archivePath -Force
Write-Host "Demo release ready:" $archivePath
