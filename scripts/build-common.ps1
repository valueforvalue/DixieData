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

function Get-DixieDataPdfiumExpectedHash {
    # Pinned hash of the canonical pdfium.dll from upstream tag chromium/7857.
    # Verified 2026-06-24. To upgrade: bump Get-DixieDataPdfiumVersion, download
    # the new archive, extract bin\pdfium.dll, replace this string with its
    # `certutil -hashfile ... SHA256` value, update bin\MANIFEST.md.
    return "ebddbc781afbffb6f76c8e674e5900665a8676e778a91c4130b9afcb4a8a812a"
}

function Get-DixieDataTypstVersion {
    return "v0.15.0"
}

function Get-DixieDataTypstAssetName {
    return "typst-x86_64-pc-windows-msvc.zip"
}

function Get-DixieDataTypstExpectedHash {
    # Pinned hash of typst-windows.exe from upstream release v0.15.0.
    # Verified 2026-06-24 against the binary shipped in bin/.
    # To upgrade: bump Get-DixieDataTypstVersion, download + extract the
    # new typst-x86_64-pc-windows-msvc.zip, replace this string with the
    # `certutil -hashfile typst-windows.exe SHA256` value, update bin\MANIFEST.md.
    return "b561e8bbcccb0caaa665831d9fe08136eb47761b8ea5c2d8209ad64e76db5963"
}

function Test-DixieDataSha256 {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path,
        [Parameter(Mandatory = $true)]
        [string]$ExpectedHash
    )
    $actual = (certutil -hashfile $Path SHA256 2>$null | Select-String -Pattern '^[0-9a-fA-F]{64}$').Line
    return ($actual -eq $expectedHash.ToLower())
}

function Get-DixieDataPdfiumAssetName {
    return "pdfium-win-x64.tgz"
}

# Get-DixieDataLocalPdfiumCandidate searches release/ for a
# previously-shipped pdfium.dll matching the expected version.
# Returns the full path of the first match, or "" if none.
# We don't recurse — release archives live at
# release/DixieData-debug-v*/pdfium.dll by convention.
function Get-DixieDataLocalPdfiumCandidate {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Root,
        [Parameter(Mandatory = $true)]
        [string]$Version
    )

    $releaseRoot = Join-Path $Root "release"
    if (-not (Test-Path $releaseRoot)) {
        return ""
    }
    $matches = Get-ChildItem -Path $releaseRoot -Recurse -Filter "pdfium.dll" -ErrorAction SilentlyContinue
    foreach ($match in $matches) {
        $marker = Join-Path (Split-Path -Parent $match.FullName) "pdfium.version"
        if (Test-Path $marker) {
            $markerText = (Get-Content -Path $marker -Raw).Trim()
            if ($markerText -eq $Version) {
                return $match.FullName
            }
        }
    }
    return ""
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

    # Fast path: if a prior release archive under release/ ships
    # the same version's pdfium.dll, copy it locally. Saves the
    # 30s+ download round-trip on every debug build. We only use
    # the local copy when the SHA256 matches the pinned hash; a
    # mismatch falls through to the network download (and the
    # download's own SHA256 check catches tampering upstream).
    $localCandidate = Get-DixieDataLocalPdfiumCandidate -Root $Root -Version $expectedVersion
    if ($localCandidate) {
        $expectedHash = Get-DixieDataPdfiumExpectedHash
        if (Test-DixieDataSha256 -Path $localCandidate -ExpectedHash $expectedHash) {
            Copy-Item $localCandidate $dllPath -Force
            Set-Content -Path $markerPath -Value $expectedVersion -Encoding ASCII
            Write-Host "Restored pdfium.dll from local cache: $localCandidate"
            return
        }
        Write-Warning "Local pdfium.dll at $localCandidate has wrong hash; falling through to network download."
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

        # Verify SHA256 against the pinned hash. A mismatch means the
        # upstream tag was renamed or replaced — fail loud rather than
        # ship a silently-different binary.
        $expectedHash = Get-DixieDataPdfiumExpectedHash
        if (-not (Test-DixieDataSha256 -Path $dllPath -ExpectedHash $expectedHash)) {
            Remove-Item $dllPath -Force -ErrorAction SilentlyContinue
            throw "PDFium SHA256 mismatch for $dllPath. Upstream tag $expectedVersion may have been replaced. Update Get-DixieDataPdfiumVersion + Get-DixieDataPdfiumExpectedHash in scripts/build-common.ps1."
        }

        Set-Content -Path $markerPath -Value $expectedVersion -Encoding ASCII
    } finally {
        if (Test-Path $tempDir) {
            Remove-Item $tempDir -Recurse -Force
        }
    }
}

function Restore-DixieDataTypstBinary {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Root
    )

    # If a vendored binary already exists in bin/, prefer it (offline / dev flow).
    # Otherwise download the pinned upstream release and verify SHA256 before
    # installing into bin/. A version marker prevents redundant re-downloads.
    $binDir = Join-Path $Root "bin"
    New-Item -ItemType Directory -Path $binDir -Force | Out-Null

    $targetPath = Join-Path $binDir "typst-windows.exe"
    $markerPath = Join-Path $binDir ".typst.version"
    $expectedVersion = Get-DixieDataTypstVersion
    $currentVersion = if (Test-Path $markerPath) { (Get-Content -Path $markerPath -Raw).Trim() } else { "" }
    if ((Test-Path $targetPath) -and $currentVersion -eq $expectedVersion) {
        return
    }

    $tempDir = Join-Path ([System.IO.Path]::GetTempPath()) ("DixieData-typst-" + [guid]::NewGuid().ToString())
    New-Item -ItemType Directory -Path $tempDir -Force | Out-Null

    try {
        $assetName = Get-DixieDataTypstAssetName
        $archivePath = Join-Path $tempDir $assetName
        $downloadUrl = "https://github.com/typst/typst/releases/download/$expectedVersion/$assetName"

        Write-Host "Downloading Typst $expectedVersion..."
        Invoke-WebRequest -Uri $downloadUrl -OutFile $archivePath

        # Upstream typst-windows zip is a real .zip (multi-entry), not a single-
        # entry gzip stream. System32 tar's -xzf chokes on it. Expand-Archive
        # is pwsh-native and handles multi-entry zips cleanly. The earlier
        # Invoke-WebRequest is also a native cmdlet, so $LASTEXITCODE is not
        # reliably set in this function's scope; check $? (success-of-last-
        # command, always defined) instead.
        Expand-Archive -Path $archivePath -DestinationPath $tempDir -Force
        if (-not $?) {
            throw "Failed to extract $assetName"
        }

        # Upstream archive layout: typst-x86_64-pc-windows-msvc/typst.exe
        # (asset name carries the triple, so the nested dir matches).
        $nestedExe = Get-ChildItem -Path $tempDir -Recurse -Filter "typst.exe" | Select-Object -First 1
        if (-not $nestedExe) {
            throw "Extracted archive did not contain typst.exe (nested under asset name dir)"
        }
        $extractedExe = $nestedExe.FullName

        $expectedHash = Get-DixieDataTypstExpectedHash
        if (-not (Test-DixieDataSha256 -Path $extractedExe -ExpectedHash $expectedHash)) {
            throw "Typst SHA256 mismatch. Upstream tag $expectedVersion may have been replaced. Update Get-DixieDataTypstVersion + Get-DixieDataTypstExpectedHash in scripts/build-common.ps1."
        }

        Copy-Item $extractedExe $targetPath -Force
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
    # The launcher forwards args to DixieData.exe verbatim, but
    # also sets up a debug-friendly env so a debugger can attach.
    # In particular:
    #   - DIXIEDATA_DATA_DIR lets the user point at a scratch dir
    #     without rebuilding (handy when reproducing a crash).
    #   - GOTRACEBACK=all gives a full stack on panic.
    # The launcher does NOT enable the race detector; that lives
    # on the binary itself (make debug uses -gcflags=-N -l so a
    # debugger can attach; the race detector is added via
    # `make race` / wails build -race).
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

# Debug-friendly env defaults. Existing values win so a user
# can override per-invocation: `$env:GOTRACEBACK='panic';
# .\Run-DixieData-Debug.ps1`.
if (-not $env:GOTRACEBACK) { $env:GOTRACEBACK = "all" }
if (-not $env:DIXIEDATA_DEVTOOLS) { $env:DIXIEDATA_DEVTOOLS = "1" }
# Allow the user to attach a Go debugger on a fixed port. The
# binary's symbols are intact (make debug uses -gcflags=-N -l)
# so dlv attach --pid <pid> works after the process is up.
if (-not $env:DIXIEDATA_WAIT_FOR_DEBUGGER) { $env:DIXIEDATA_WAIT_FOR_DEBUGGER = "0" }

& $exePath @AppArgs
exit $LASTEXITCODE
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
        [switch]$AllowExampleOAuthDefaults,
        [switch]$DebugBuild
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
        # -X injects the build metadata. When -DebugBuild is set
        # we also pass -debug to Wails, which:
        #   * Drops -trimpath so the debugger can map addresses
        #     back to source paths.
        #   * Adds -gcflags=all=-N -l so Go's optimiser does not
        #     elide frames or inline past breakpoints.
        #   * Enables the WebView2 inspector and default context
        #     menu (F12 / Ctrl+Shift+I works in the running app).
        # The Go runtime symbol table is intact in both modes,
        # so `dlv attach $PID` works against any build.
        $buildLdFlags = "-X github.com/valueforvalue/DixieData/internal/buildinfo.GitCommit=$gitCommit -X github.com/valueforvalue/DixieData/internal/buildinfo.BuildTimestamp=$buildTimestamp"
        $effectiveWailsArguments = @($WailsArguments)
        if ($DebugBuild) {
            # Strip -trimpath from the caller-supplied args; the
            # Wails -debug flag is mutually exclusive with -trimpath
            # but the user may have left it in the default.
            $effectiveWailsArguments = $effectiveWailsArguments | Where-Object { $_ -ne "-trimpath" }
            $effectiveWailsArguments += "-debug"
            # -tags debug gates //go:build debug files (trace.Log,
            # debug-only tests) so release binaries carry zero
            # dead weight from debug instrumentation.
            $effectiveWailsArguments += @("-tags", "debug")
        }
        $effectiveWailsArguments += @("-ldflags", $buildLdFlags)

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
        Restore-DixieDataTypstBinary -Root $Root
        Restore-DixieDataTypstAssets -Root $Root
    } finally {
        Remove-DixieDataPreservedOAuthDefaults -PreservedPath $preservedOAuth
    }
}
