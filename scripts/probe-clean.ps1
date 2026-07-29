# probe-clean.ps1 -- kill any straggler dixiedata-* processes from a
# previous probe run, then verify the file handles are released so
# the next `just debug` doesn't fail with:
#
#   unlinkat ... dixiedata-web.exe: The process cannot access the
#   file because it is being used by another process.
#
# The previous Makefile recipe used bare `taskkill /F` with output
# redirected to nul and make's `-@` "ignore errors" prefix. That
# silently swallowed two failure modes:
#
#   1. Antivirus or protected-process hold: taskkill reports success
#      (exit 0) but the process re-spawns or stays alive, and the
#      file handle is never released. Downstream `go build` fails
#      with the unlinkat error above and the user has no clue.
#   2. Mixed-case process names: Wails occasionally launches
#      `DixieData.exe` vs `dixiedata.exe`. The old list only
#      matched `DixieData.exe`, missing the lowercase variant.
#
# This script kills, then re-queries via `tasklist` to verify
# nothing survived. If anything is still running after the kill,
# it prints a clear error listing the surviving PIDs and exits 1,
# which propagates through `make` so the build halts with the
# right context. Caller can then either re-run probe-clean (the
# retry often works after a few hundred ms) or check AV.
#
# Idempotent: no error if nothing is running. Safe to invoke from
# CI / pre-build hooks / post-failure cleanup.

$ErrorActionPreference = 'Stop'

# Process names to terminate. Lowercase comparison is intentional
# -- Windows process names are case-insensitive but the tasklist
# output preserves the original image name. Match both casings
# via the matching helper below.
$targets = @(
    'dixiedata-web.exe',
    'DixieData.exe',
    'dixiedata.exe',
    'seed-data.exe',
    'gold-master.exe'
)

function Test-ProcessAlive([string]$name) {
    $proc = Get-Process -Name ([System.IO.Path]::GetFileNameWithoutExtension($name)) `
        -ErrorAction SilentlyContinue
    if ($null -eq $proc) { return @() }
    # Filter to processes whose MainModule filename matches the
    # target (Get-Process -Name is a prefix match -- `dixiedata`
    # would also match `dixiedata-web`).
    return @($proc | Where-Object {
        try {
            $_.MainModule.FileName -like "*\$name"
        } catch {
            # Some processes (e.g. system) deny MainModule access.
            # Fall back to ProcessName match.
            $_.ProcessName -eq [System.IO.Path]::GetFileNameWithoutExtension($name)
        }
    })
}

$killed = @()
$survivors = @()

foreach ($name in $targets) {
    $before = Test-ProcessAlive $name
    if ($before.Count -eq 0) { continue }

    Write-Host "  killing $name (PIDs: $($before.Id -join ', '))"
    # /F = force, /T = tree (children too). Exit 128 = no match
    # (which shouldn't happen here since we just confirmed alive),
    # exit 1 = access denied. We treat both as a retry signal --
    # see verify loop below.
    & taskkill.exe /F /IM $name /T 2>&1 | Out-Null
    $killed += $name
}

# Verify loop. Single retry after a short delay -- the second pass
# almost always succeeds when the first didn't (handle release
# latency, AV scan completion, etc.). If it still fails after the
# retry, surface the survivors so the operator knows what's stuck.
$maxAttempts = 2
for ($attempt = 1; $attempt -le $maxAttempts; $attempt++) {
    $stillAlive = @()
    foreach ($name in $targets) {
        $alive = Test-ProcessAlive $name
        if ($alive.Count -gt 0) {
            $stillAlive += [PSCustomObject]@{
                Name = $name
                PIds = $alive.Id
            }
        }
    }
    if ($stillAlive.Count -eq 0) { break }
    if ($attempt -lt $maxAttempts) {
        Start-Sleep -Milliseconds 500
        foreach ($entry in $stillAlive) {
            & taskkill.exe /F /IM $entry.Name /T 2>&1 | Out-Null
        }
        Start-Sleep -Milliseconds 200
    } else {
        $survivors = $stillAlive
    }
}

if ($killed.Count -gt 0) {
    Write-Host ("probe-clean: killed {0} process(es): {1}" -f `
        $killed.Count, ($killed -join ', '))
} else {
    Write-Host "probe-clean: nothing to clean"
}

if ($survivors.Count -gt 0) {
    Write-Host ""
    Write-Host "probe-clean: FAILED -- these processes survived the kill:" -ForegroundColor Red
    foreach ($entry in $survivors) {
        Write-Host ("  {0} (PIDs: {1})" -f $entry.Name, ($entry.PIds -join ', ')) -ForegroundColor Red
    }
    Write-Host ""
    Write-Host "Likely causes:" -ForegroundColor Yellow
    Write-Host "  - Antivirus quarantined the binary and is holding the handle"
    Write-Host '  - A debugger (VS, dlv) is attached to the process'
    Write-Host "  - The process re-spawns immediately (a watcher service)"
    Write-Host ""
    Write-Host "Try: wait a few seconds and re-run, or kill manually via Task Manager."
    exit 1
}

exit 0