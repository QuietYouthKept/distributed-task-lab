[CmdletBinding()]
param(
    [string]$OutputDirectory = "",
    [switch]$SkipSlowChecks
)

$ErrorActionPreference = "Stop"

function Get-RepositoryRoot {
    $scriptDirectory = Split-Path -Parent $PSCommandPath
    return (Resolve-Path (Join-Path $scriptDirectory "..\\..")).Path
}

function Get-Sha256([string]$Path) {
    return (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLowerInvariant()
}

function Get-CommandVersion([string]$FilePath, [string[]]$Arguments) {
    try {
        $value = & $FilePath @Arguments 2>&1
        if ($LASTEXITCODE -ne 0) {
            return @{ available = $false; value = ($value -join "`n") }
        }
        return @{ available = $true; value = ($value -join "`n").Trim() }
    } catch {
        return @{ available = $false; value = $_.Exception.Message }
    }
}

function Invoke-RecordedCommand(
    [string]$Name,
    [string]$FilePath,
    [string[]]$Arguments,
    [hashtable]$Environment,
    [string]$LogDirectory
) {
    $safeName = $Name -replace "[^A-Za-z0-9_.-]", "_"
    $stdout = Join-Path $LogDirectory "$safeName.stdout.log"
    $stderr = Join-Path $LogDirectory "$safeName.stderr.log"
    $startedAt = (Get-Date).ToUniversalTime()

    $savedEnvironment = @{}
    foreach ($key in $Environment.Keys) {
        $savedEnvironment[$key] = [Environment]::GetEnvironmentVariable($key, "Process")
        [Environment]::SetEnvironmentVariable($key, $Environment[$key], "Process")
    }

    try {
        try {
            $process = Start-Process -FilePath $FilePath -ArgumentList $Arguments -WorkingDirectory $repoRoot -Wait -PassThru -NoNewWindow -RedirectStandardOutput $stdout -RedirectStandardError $stderr
            $exitCode = $process.ExitCode
            $error = $null
        } catch {
            $exitCode = 127
            $error = $_.Exception.Message
            Set-Content -LiteralPath $stderr -Value $error
        }
    } finally {
        foreach ($key in $Environment.Keys) {
            [Environment]::SetEnvironmentVariable($key, $savedEnvironment[$key], "Process")
        }
    }

    return [ordered]@{
        name = $Name
        command = (@($FilePath) + $Arguments) -join " "
        exit_code = $exitCode
        started_at_utc = $startedAt.ToString("o")
        finished_at_utc = (Get-Date).ToUniversalTime().ToString("o")
        stdout = (Resolve-Path -LiteralPath $stdout).Path.Substring($repoRoot.Length + 1).Replace("\\", "/")
        stderr = (Resolve-Path -LiteralPath $stderr).Path.Substring($repoRoot.Length + 1).Replace("\\", "/")
        error = $error
    }
}

$repoRoot = Get-RepositoryRoot
if ([string]::IsNullOrWhiteSpace($OutputDirectory)) {
    $OutputDirectory = Join-Path $repoRoot "evidence\\baseline"
}
if (-not [System.IO.Path]::IsPathRooted($OutputDirectory)) {
    $OutputDirectory = Join-Path $repoRoot $OutputDirectory
}
$OutputDirectory = [System.IO.Path]::GetFullPath($OutputDirectory)
$logDirectory = Join-Path $OutputDirectory "logs"
New-Item -ItemType Directory -Path $logDirectory -Force | Out-Null

$gitCommit = (& git -C $repoRoot rev-parse HEAD 2>$null).Trim()
if ($LASTEXITCODE -ne 0) { $gitCommit = "unavailable" }
$gitStatus = (& git -C $repoRoot status --porcelain=v1 2>$null)
if ($LASTEXITCODE -ne 0) { $gitStatus = @("git status unavailable") }

$routes = @(
    Select-String -LiteralPath (Join-Path $repoRoot "internal\\api\\router.go") -Pattern 'mux\.Handle(?:Func)?\("([^"]+)"' |
        ForEach-Object { $_.Matches[0].Groups[1].Value }
)
$migrations = @(
    Get-ChildItem -LiteralPath (Join-Path $repoRoot "internal\\storage\\postgres\\migrations") -Filter "*.sql" |
        Sort-Object Name |
        ForEach-Object {
            [ordered]@{
                file = "internal/storage/postgres/migrations/$($_.Name)"
                sha256 = Get-Sha256 $_.FullName
            }
        }
)
$benchmarks = @(
    Get-ChildItem -LiteralPath (Join-Path $repoRoot "benchmarks") -File -Recurse -ErrorAction SilentlyContinue |
        Sort-Object FullName |
        ForEach-Object {
            [ordered]@{
                file = $_.FullName.Substring($repoRoot.Length + 1).Replace("\\", "/")
                sha256 = Get-Sha256 $_.FullName
                bytes = $_.Length
            }
        }
)

$checks = @()
$checks += Invoke-RecordedCommand -Name "unit" -FilePath "go" -Arguments @("test", "./cmd/...", "./internal/...", "./pkg/...", "-count=1") -Environment @{} -LogDirectory $logDirectory
if (-not $SkipSlowChecks) {
    $checks += Invoke-RecordedCommand -Name "integration" -FilePath "go" -Arguments @("test", "./internal/storage/postgres", "-count=1", "-v") -Environment @{ REQUIRE_TEST_DATABASE = "1" } -LogDirectory $logDirectory
    $checks += Invoke-RecordedCommand -Name "race" -FilePath "go" -Arguments @("test", "-race", "./cmd/...", "./internal/...", "./pkg/...", "-count=1") -Environment @{} -LogDirectory $logDirectory
    $checks += Invoke-RecordedCommand -Name "failurelab_smoke" -FilePath "go" -Arguments @("run", "./cmd/failurelab") -Environment @{} -LogDirectory $logDirectory
}

$manifest = [ordered]@{
    schema_version = 1
    baseline_id = "baseline-" + (Get-Date).ToUniversalTime().ToString("yyyyMMddTHHmmssZ") + "-" + $gitCommit.Substring(0, [Math]::Min(12, $gitCommit.Length))
    collected_at_utc = (Get-Date).ToUniversalTime().ToString("o")
    repository = [ordered]@{
        commit_sha = $gitCommit
        dirty = @($gitStatus).Count -gt 0
        dirty_entries = @($gitStatus)
    }
    environment = [ordered]@{
        go = Get-CommandVersion "go" @("version")
        docker = Get-CommandVersion "docker" @("version", "--format", "{{.Server.Version}}")
        postgres_container_image = Get-CommandVersion "docker" @("image", "inspect", "postgres:16-alpine", "--format", "{{index .RepoDigests 0}}")
        os = [System.Environment]::OSVersion.VersionString
        config_source_sha256 = Get-Sha256 (Join-Path $repoRoot "internal\\config\\config.go")
        database_url_redacted = ((Get-ChildItem Env:DATABASE_URL -ErrorAction SilentlyContinue).Value -replace "//[^@/]+@", "//***@")
    }
    api_routes = $routes
    migrations = $migrations
    benchmark_inventory = $benchmarks
    verification = $checks
    notes = @(
        "A non-zero verification exit_code means the baseline is unverified for that check.",
        "Benchmark files are inventoried only; this manifest does not promote their values to measured claims.",
        "config_source_sha256 fingerprints the configuration implementation, not secret runtime values."
    )
}

$manifestPath = Join-Path $OutputDirectory "manifest.json"
$manifest | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $manifestPath -Encoding utf8
Write-Host "Baseline manifest written to $manifestPath"

if (@($checks | Where-Object { $_.exit_code -ne 0 }).Count -gt 0) {
    Write-Error "One or more baseline verification commands failed; inspect $manifestPath."
    exit 1
}
