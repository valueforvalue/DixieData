param(
    [string]$ArtifactRoot = ".\tests\goldmaster\artifacts",
    [int]$Soldiers = 7500,
    [int]$SeedSoldiers = 1500,
    [int]$PlaywrightPort = 4173
)

$ErrorActionPreference = "Stop"

$resolvedArtifactRoot = if ([System.IO.Path]::IsPathRooted($ArtifactRoot)) {
    $ArtifactRoot
} else {
    Join-Path (Get-Location) $ArtifactRoot
}

if (Test-Path $resolvedArtifactRoot) {
    Remove-Item $resolvedArtifactRoot -Recurse -Force
}
New-Item -ItemType Directory -Force -Path $resolvedArtifactRoot | Out-Null

Write-Host "==> Output integrity audit"
go run .\cmd\gold-master -mode output-audit -report-dir (Join-Path $resolvedArtifactRoot "output")

Write-Host "==> Gold-master Go workflow tests"
go test . -run 'TestGoldMaster|TestHandleVersionReturnsBuildMetadata|TestHandleInsightsDrilldownShowsFilteredRecords|TestStressBridgeConcurrentSearchAndSave|TestStressAppLoggingToFile' -count=1

Write-Host "==> Install Playwright dependencies"
Push-Location .\tests\goldmaster\playwright
if (-not (Test-Path ".\node_modules")) {
    npm install --no-fund --no-audit
}
npx playwright install chromium
Pop-Location

$staticArchiveDir = Join-Path $resolvedArtifactRoot "output\artifacts\static-archive"
$server = Start-Process -FilePath "python" -ArgumentList "-m", "http.server", "$PlaywrightPort", "--directory", $staticArchiveDir -PassThru -WindowStyle Hidden
try {
    Start-Sleep -Seconds 3
    Write-Host "==> Playwright static archive verification"
    Push-Location .\tests\goldmaster\playwright
    $env:GOLD_MASTER_ARCHIVE_URL = "http://127.0.0.1:$PlaywrightPort"
    npx playwright test
    Pop-Location
} finally {
    if ($server -and -not $server.HasExited) {
        Stop-Process -Id $server.Id
    }
    Remove-Item Env:\GOLD_MASTER_ARCHIVE_URL -ErrorAction SilentlyContinue
}

Write-Host "==> Stress and scale suite"
python .\tests\goldmaster\stress_suite.py --repo-root . --report-dir (Join-Path $resolvedArtifactRoot "stress") --data-dir (Join-Path $resolvedArtifactRoot "stress\data") --soldiers $Soldiers --seed-soldiers $SeedSoldiers

Write-Host "Gold-master suite complete."
