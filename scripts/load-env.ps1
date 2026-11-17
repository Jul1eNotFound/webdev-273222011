# Loads .env into the current PowerShell session, so go run and go test see its variables.
# Dot-source it from the repository root:  . .\scripts\load-env.ps1
# It never prints a value. Variables already set in the session are kept, as on a real host, and listed
# as kept, so a stale value is easy to spot. After editing .env, reload with -Force to take every value
# from the file:  . .\scripts\load-env.ps1 -Force
param(
    [string]$Path = (Join-Path $PSScriptRoot '..\.env'),
    [switch]$Force
)

if (-not (Test-Path $Path)) {
    Write-Error "No .env at $Path. Copy .env.example to .env and fill in the values."
    return
}

$loaded = @()
$kept = @()
foreach ($line in Get-Content $Path) {
    $line = $line.Trim()
    if ($line -eq '' -or $line.StartsWith('#')) { continue }
    $key, $value = $line -split '=', 2
    $key = $key.Trim()
    if ($null -eq $value -or $key -eq '') { continue }
    $value = $value.Trim().Trim('"').Trim("'")
    if (-not $Force -and [Environment]::GetEnvironmentVariable($key, 'Process')) {
        $kept += $key
        continue
    }
    [Environment]::SetEnvironmentVariable($key, $value, 'Process')
    $loaded += $key
}
"Loaded from .env: " + ($loaded -join ', ')
if ($kept.Count -gt 0) {
    "Kept from this session, not reloaded: " + ($kept -join ', ') + "  (use -Force to reload them)"
}
