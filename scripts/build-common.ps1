Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Get-DixieDataRoot {
    param(
        [string]$StartPath = $PSScriptRoot
    )

    $current = (Resolve-Path $StartPath).Path
    while ($true) {
        if (Test-Path (Join-Path $current "wails.json")) {
            return $current
        }

        $parent = Split-Path -Parent $current
        if ([string]::IsNullOrWhiteSpace($parent) -or $parent -eq $current) {
            throw "Failed to locate repository root from $StartPath"
        }
        $current = $parent
    }
}

function Set-DixieDataBuildLocation {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Root
    )

    Set-Location $Root
}

function Get-DixieDataBuildBinDir {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Root
    )

    return Join-Path $Root "build\bin"
}

function Get-DixieDataTailwindMarkerPath {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Root
    )

    return Join-Path $Root "node_modules\tailwindcss\package.json"
}

function Get-DixieDataAppVersion {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Root
    )

    $versionInfoPath = Join-Path $Root "internal\versioninfo\versioninfo.go"
    $content = Get-Content -Path $versionInfoPath -Raw
    $match = [regex]::Match($content, "CurrentSchemaVersion\s*=\s*(\d+)")
    if (-not $match.Success) {
        throw "Failed to determine CurrentSchemaVersion from $versionInfoPath"
    }

    return "v1.2.{0}" -f $match.Groups[1].Value
}

function Get-DixieDataOAuthDefaultsBuildPath {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Root
    )

    return Join-Path (Get-DixieDataBuildBinDir -Root $Root) "google-oauth-defaults.json"
}

function Get-DixieDataPdfiumVersion {
    return "chromium/7857"
}

function Get-DixieDataPdfiumAssetName {
    return "pdfium-win-x64.tgz"
}

function Get-DixieDataPdfiumBuildPath {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Root
    )

    return Join-Path (Get-DixieDataBuildBinDir -Root $Root) "pdfium.dll"
}

function Get-DixieDataTypstBinaryBuildPath {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Root
    )

    # The release archive places the Typst binary at <bin>/bin/
    # (a subdirectory of build/bin/) so the appshell's
    # findTypstBinary walker can locate it from the cwd.
    $innerBin = Join-Path (Get-DixieDataBuildBinDir -Root $Root) "bin"
    New-Item -ItemType Directory -Path $innerBin -Force | Out-Null
    return Join-Path $innerBin "typst-windows.exe"
}

function Get-DixieDataTypstSourceBinaryPath {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Root
    )

    return Join-Path $Root "bin\typst-windows.exe"
}

function Get-DixieDataTypstTemplatesBuildDir {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Root
    )

    # Templates go at the root of build/bin/ so the appshell's
    # findTemplatesDir walker finds templates/ next to DixieData.exe.
    return Get-DixieDataBuildBinDir -Root $Root
}

function Get-DixieDataTypstTemplatesSourceDir {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Root
    )

    return Join-Path $Root "templates"
}

function Restore-DixieDataTypstAssets {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Root
    )

    # Restore the Typst binary from bin/ in the source tree.
    $sourceBinary = Get-DixieDataTypstSourceBinaryPath -Root $Root
    $targetBinary = Get-DixieDataTypstBinaryBuildPath -Root $Root
    if (-not (Test-Path $sourceBinary)) {
        throw "Typst binary not found at $sourceBinary. The Typst-migration release requires bin\typst-windows.exe to be present in the source tree."
    }
    Copy-Item $sourceBinary $targetBinary -Force
    Write-Host "Bundled Typst binary: $targetBinary"

    # Copy the templates directory wholesale. The appshell's
    # findTemplatesDir walker looks for a `templates/` directory
    # adjacent to DixieData.exe.
    $sourceTemplates = Get-DixieDataTypstTemplatesSourceDir -Root $Root
    $targetTemplates = Get-DixieDataTypstTemplatesBuildDir -Root $Root
    if (-not (Test-Path $sourceTemplates)) {
        throw "Templates directory not found at $sourceTemplates."
    }
    $targetTemplatesPath = Join-Path $targetTemplates "templates"
    if (Test-Path $targetTemplatesPath) {
        Remove-Item $targetTemplatesPath -Recurse -Force
    }
    Copy-Item $sourceTemplates $targetTemplatesPath -Recurse -Force
    Write-Host "Bundled Typst templates: $targetTemplatesPath"
}

function Get-DixieDataPdfiumMarkerPath {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Root
    )

    return Join-Path (Get-DixieDataBuildBinDir -Root $Root) "pdfium.version"
}

function Restore-DixieDataPdfiumBinary {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Root
    )

    $binDir = Get-DixieDataBuildBinDir -Root $Root
    New-Item -ItemType Directory -Path $binDir -Force | Out-Null

    $expectedVersion = Get-DixieDataPdfiumVersion
    $dllPath = Get-DixieDataPdfiumBuildPath -Root $Root
    $markerPath = Get-DixieDataPdfiumMarkerPath -Root $Root
    $currentVersion = if (Test-Path $markerPath) { (Get-Content -Path $markerPath -Raw).Trim() } else { "" }
    if ((Test-Path $dllPath) -and $currentVersion -eq $expectedVersion) {
        return
    }

    $tempDir = Join-Path ([System.IO.Path]::GetTempPath()) ("DixieData-pdfium-" + [guid]::NewGuid().ToString())
    New-Item -ItemType Directory -Path $tempDir -Force | Out-Null

    try {
        $assetName = Get-DixieDataPdfiumAssetName
        $archivePath = Join-Path $tempDir $assetName
        $escapedTag = [System.Uri]::EscapeDataString($expectedVersion)
        $downloadUrl = "https://github.com/bblanchon/pdfium-binaries/releases/download/$escapedTag/$assetName"

        Write-Host "Downloading PDFium runtime $expectedVersion..."
        Invoke-WebRequest -Uri $downloadUrl -OutFile $archivePath

        # Use Windows native tar (System32) to avoid git-bash /usr/bin/tar
        # shadowing pwsh's lookup and failing on 'C:' path resolution.
        & "$env:SystemRoot\System32\tar.exe" -xzf $archivePath -C $tempDir
        if ($LASTEXITCODE -ne 0) {
            throw "Failed to extract $assetName"
        }

        $extractedDll = Join-Path $tempDir "bin\pdfium.dll"
        if (-not (Test-Path $extractedDll)) {
            throw "Extracted archive did not contain bin\pdfium.dll"
        }

        Copy-Item $extractedDll $dllPath -Force
        Set-Content -Path $markerPath -Value $expectedVersion -Encoding ASCII
    } finally {
        if (Test-Path $tempDir) {
            Remove-Item $tempDir -Recurse -Force
        }
    }
}

function Save-DixieDataOAuthDefaults {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Root
    )

    $existing = Get-DixieDataOAuthDefaultsBuildPath -Root $Root
    if (-not (Test-Path $existing)) {
        return $null
    }

    $tempDir = Join-Path ([System.IO.Path]::GetTempPath()) ("DixieData-build-" + [guid]::NewGuid().ToString())
    New-Item -ItemType Directory -Path $tempDir -Force | Out-Null

    $preserved = Join-Path $tempDir "google-oauth-defaults.json"
    Copy-Item $existing $preserved -Force
    return $preserved
}

function Resolve-DixieDataOAuthDefaultsSource {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Root,
        [string]$PreservedPath,
        [switch]$AllowExampleOAuthDefaults
    )

    $projectOAuth = Join-Path $Root "google-oauth-defaults.json"
    if (Test-Path $projectOAuth) {
        return @{
            Path   = $projectOAuth
            Source = "project root google-oauth-defaults.json"
        }
    }

    if ($PreservedPath -and (Test-Path $PreservedPath)) {
        return @{
            Path   = $PreservedPath
            Source = "preserved build\bin\google-oauth-defaults.json"
        }
    }

    if ($AllowExampleOAuthDefaults) {
        $exampleOAuth = Join-Path $Root "google-oauth-defaults.example.json"
        if (Test-Path $exampleOAuth) {
            return @{
                Path   = $exampleOAuth
                Source = "google-oauth-defaults.example.json"
            }
        }
    }

    return $null
}

function Restore-DixieDataOAuthDefaults {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Root,
        [Parameter(Mandatory = $true)]
        [string]$SourcePath
    )

    $binDir = Get-DixieDataBuildBinDir -Root $Root
    New-Item -ItemType Directory -Path $binDir -Force | Out-Null
    Copy-Item $SourcePath (Get-DixieDataOAuthDefaultsBuildPath -Root $Root) -Force
}

function Remove-DixieDataPreservedOAuthDefaults {
    param(
        [string]$PreservedPath
    )

    if (-not $PreservedPath) {
        return
    }

    $parent = Split-Path -Parent $PreservedPath
    if ($parent -and (Test-Path $parent)) {
        Remove-Item $parent -Recurse -Force
    }
}

function Write-DixieDataDebugLauncher {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Root
    )

    $launcherPath = Join-Path (Get-DixieDataBuildBinDir -Root $Root) "Run-DixieData-Debug.ps1"
    $script = @'
param(
    [Parameter(ValueFromRemainingArguments = $true)]
    [string[]]$AppArgs
)

$ErrorActionPreference = "Stop"

$binDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$exePath = Join-Path $binDir "DixieData.exe"
if (-not (Test-Path $exePath)) {
    throw "DixieData.exe was not found at $exePath"
}

$previousDebugSetting = [System.Environment]::GetEnvironmentVariable("DIXIEDATA_DEBUG_UI_IDS", "Process")
try {
    [System.Environment]::SetEnvironmentVariable("DIXIEDATA_DEBUG_UI_IDS", "1", "Process")
    & $exePath "--debug-ui-ids" @AppArgs
    $exitCode = $LASTEXITCODE
} finally {
    if ([string]::IsNullOrEmpty($previousDebugSetting)) {
        Remove-Item Env:DIXIEDATA_DEBUG_UI_IDS -ErrorAction SilentlyContinue
    } else {
        [System.Environment]::SetEnvironmentVariable("DIXIEDATA_DEBUG_UI_IDS", $previousDebugSetting, "Process")
    }
}

exit $exitCode
'@

    Set-Content -Path $launcherPath -Value $script -Encoding UTF8
    return $launcherPath
}

function Invoke-DixieDataFrontendAssetBuild {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Root
    )

    Set-DixieDataBuildLocation -Root $Root

    if (-not (Test-Path (Get-DixieDataTailwindMarkerPath -Root $Root))) {
        Write-Host "Installing frontend build dependencies..."
        & npm install
        if ($LASTEXITCODE -ne 0) {
            throw "npm install failed with exit code $LASTEXITCODE"
        }
    }

    Write-Host "Regenerating frontend CSS bundle..."
    & npm run build:css
    if ($LASTEXITCODE -ne 0) {
        throw "npm run build:css failed with exit code $LASTEXITCODE"
    }
}

function Invoke-DixieDataBuild {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Root,
        [string[]]$WailsArguments = @("build", "-clean", "-trimpath"),
        [switch]$AllowExampleOAuthDefaults
    )

    $preservedOAuth = Save-DixieDataOAuthDefaults -Root $Root
    try {
        Set-DixieDataBuildLocation -Root $Root

        Invoke-DixieDataFrontendAssetBuild -Root $Root

        go run github.com/a-h/templ/cmd/templ@v0.3.1001 generate
        if ($LASTEXITCODE -ne 0) {
            throw "templ generate failed with exit code $LASTEXITCODE"
        }

        $gitCommit = (& git rev-parse --short HEAD).Trim()
        if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace($gitCommit)) {
            throw "failed to resolve git commit for build metadata"
        }
        $buildTimestamp = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
        $buildLdFlags = "-X github.com/valueforvalue/DixieData/internal/buildinfo.GitCommit=$gitCommit -X github.com/valueforvalue/DixieData/internal/buildinfo.BuildTimestamp=$buildTimestamp"
        $effectiveWailsArguments = @($WailsArguments) + @("-ldflags", $buildLdFlags)

        & wails @effectiveWailsArguments
        if ($LASTEXITCODE -ne 0) {
            throw "wails $($effectiveWailsArguments -join ' ') failed with exit code $LASTEXITCODE"
        }

        $oauthSource = Resolve-DixieDataOAuthDefaultsSource -Root $Root -PreservedPath $preservedOAuth -AllowExampleOAuthDefaults:$AllowExampleOAuthDefaults
        if ($oauthSource) {
            Restore-DixieDataOAuthDefaults -Root $Root -SourcePath $oauthSource.Path
            Write-Host "Restored google-oauth-defaults.json from $($oauthSource.Source)."
        } else {
            Write-Warning "No google-oauth-defaults.json source was found. The build output will not include shared Google OAuth defaults."
        }

        Restore-DixieDataPdfiumBinary -Root $Root
        Restore-DixieDataTypstAssets -Root $Root
    } finally {
        Remove-DixieDataPreservedOAuthDefaults -PreservedPath $preservedOAuth
    }
}
