# Loads .env into the current PowerShell session, so go run and go test see its variables.
# Dot-source it from the repository root:  . .\scripts\load-env.ps1
# It never prints a value. Variables already set in the session are kept, as on a real host.
param([string]$Path = (Join-Path $PSScriptRoot '..\.env'))

if (-not (Test-Path $Path)) {
    Write-Error "No .env at $Path. Copy .env.example to .env and fill in the values."
    return
}

$loaded = @()
foreach ($line in Get-Content $Path) {
    $line = $line.Trim()
    if ($line -eq '' -or $line.StartsWith('#')) { continue }
    $key, $value = $line -split '=', 2
    $key = $key.Trim()
    if ($null -eq $value -or $key -eq '') { continue }
    $value = $value.Trim().Trim('"').Trim("'")
    if ([Environment]::GetEnvironmentVariable($key, 'Process')) { continue }
    [Environment]::SetEnvironmentVariable($key, $value, 'Process')
    $loaded += $key
}
"Loaded from .env: " + ($loaded -join ', ')
