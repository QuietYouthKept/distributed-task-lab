param(
    [string]$EvidenceRoot = $(Join-Path $(if (-not [string]::IsNullOrWhiteSpace($env:TASKLAB_ARTIFACT_ROOT)) { $env:TASKLAB_ARTIFACT_ROOT } else { Join-Path ([System.IO.Path]::GetTempPath()) 'distributed-task-lab' }) 'wave4-artifacts\shared-admission-e2e-runs')
)
if ([string]::IsNullOrWhiteSpace($env:TASKLAB_ARTIFACT_ROOT)) { $env:TASKLAB_ARTIFACT_ROOT = Join-Path ([System.IO.Path]::GetTempPath()) 'distributed-task-lab' }
$TaskLabArtifactRoot = [System.IO.Path]::GetFullPath($env:TASKLAB_ARTIFACT_ROOT)


$ErrorActionPreference = 'Stop'
$repo = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$composeFile = Join-Path $repo 'deployments\e2e\admission-compose.yaml'
$runID = 'admission-' + [guid]::NewGuid().ToString('N').Substring(0, 12)
$evidence = Join-Path $EvidenceRoot $runID
New-Item -ItemType Directory -Path $evidence -Force | Out-Null

$names = @(
    'E2E_POSTGRES_PASSWORD', 'E2E_API_TOKEN', 'E2E_ADMIN_TOKEN',
    'E2E_SHARED_RATE_LIMIT_HASH_KEY', 'E2E_POSTGRES_PORT',
    'E2E_API_A_PORT', 'E2E_API_B_PORT', 'E2E_SHARED_RATE_LIMIT_RATE',
    'E2E_SHARED_RATE_LIMIT_BURST', 'E2E_API_A_URL', 'E2E_API_B_URL',
    'E2E_SHARED_RATE_LIMIT_BURST', 'E2E_REQUIRE_SHARED_ADMISSION',
    'GOCACHE', 'GOMODCACHE', 'GOTOOLCHAIN'
)
$saved = @{}
foreach ($name in $names) { $saved[$name] = [Environment]::GetEnvironmentVariable($name, 'Process') }
$project = 'tasklab-' + $runID.Replace('-', '')
$compose = @('compose', '--project-name', $project, '--file', $composeFile)
$exitCode = 1
$composeAttempted = $false
$logsCollected = $false

function Get-FreePort {
    $listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, 0)
    $listener.Start()
    try { return $listener.LocalEndpoint.Port } finally { $listener.Stop() }
}

try {
    Set-Location -LiteralPath $repo
    $env:E2E_POSTGRES_PASSWORD = [guid]::NewGuid().ToString('N')
    $env:E2E_API_TOKEN = [guid]::NewGuid().ToString('N') + [guid]::NewGuid().ToString('N')
    $env:E2E_ADMIN_TOKEN = [guid]::NewGuid().ToString('N') + [guid]::NewGuid().ToString('N')
    $env:E2E_SHARED_RATE_LIMIT_HASH_KEY = [guid]::NewGuid().ToString('N') + [guid]::NewGuid().ToString('N')
    $env:E2E_POSTGRES_PORT = [string](Get-FreePort)
    $env:E2E_API_A_PORT = [string](Get-FreePort)
    $env:E2E_API_B_PORT = [string](Get-FreePort)
    $env:E2E_SHARED_RATE_LIMIT_RATE = '0.001'
    $env:E2E_SHARED_RATE_LIMIT_BURST = '16'
    $env:E2E_API_A_URL = "http://127.0.0.1:$env:E2E_API_A_PORT"
    $env:E2E_API_B_URL = "http://127.0.0.1:$env:E2E_API_B_PORT"
    $env:E2E_REQUIRE_SHARED_ADMISSION = 'true'
    $env:GOTOOLCHAIN = 'auto'
    if (-not $env:GOCACHE) { $env:GOCACHE = $(Join-Path $TaskLabArtifactRoot 'wave4-artifacts\gocache') }
    if (-not $env:GOMODCACHE) { $env:GOMODCACHE = $(Join-Path $TaskLabArtifactRoot 'wave4-artifacts\gomodcache') }

    $source = git -C $repo rev-parse HEAD
    $tree = git -C $repo rev-parse 'HEAD^{tree}'
    @("source_commit=$source", "source_tree=$tree", "run_id=$runID", "compose_project=$project", "started_utc=$([DateTime]::UtcNow.ToString('o'))", 'credentials=ephemeral; values intentionally not recorded') |
        Set-Content -LiteralPath (Join-Path $evidence 'manifest.txt') -Encoding utf8

    $composeAttempted = $true
    & docker @compose up --detach --build --wait --wait-timeout 180 postgres api-a api-b 2>&1 |
        Tee-Object -FilePath (Join-Path $evidence 'compose-up.log')
    if ($LASTEXITCODE -ne 0) { throw "isolated Compose startup failed with exit code $LASTEXITCODE" }

    $deadline = [DateTime]::UtcNow.AddSeconds(60)
    foreach ($url in @($env:E2E_API_A_URL, $env:E2E_API_B_URL)) {
        $healthy = $false
        while ([DateTime]::UtcNow -lt $deadline) {
            try {
                $response = Invoke-WebRequest -Uri "$url/ready" -TimeoutSec 3
                if ($response.StatusCode -eq 200) { $healthy = $true; break }
            } catch { Start-Sleep -Milliseconds 500 }
        }
        if (-not $healthy) { throw "API readiness failed for one isolated replica" }
    }

    $testLog = Join-Path $evidence 'shared-admission-race.log'
    & go test -race ./internal/e2e -run '^TestSharedIngressAdmissionAcrossIndependentAPIContainers$' -count=1 -v 2>&1 |
        Tee-Object -FilePath $testLog
    $exitCode = $LASTEXITCODE
    if ($exitCode -ne 0) { throw "two-container shared admission E2E failed with exit code $exitCode" }
    Add-Content -LiteralPath (Join-Path $evidence 'manifest.txt') -Value "finished_utc=$([DateTime]::UtcNow.ToString('o'))`nexit_code=$exitCode"
} catch {
    $_ | Out-String | Set-Content -LiteralPath (Join-Path $evidence 'failure.txt') -Encoding utf8
    if ($LASTEXITCODE -ne 0) { $exitCode = $LASTEXITCODE }
} finally {
    if ($composeAttempted) {
        & docker @compose logs --no-color 2>&1 | Set-Content -LiteralPath (Join-Path $evidence 'container-logs.log') -Encoding utf8
        $logsCollected = $true
        & docker @compose down --volumes --remove-orphans 2>&1 | Set-Content -LiteralPath (Join-Path $evidence 'compose-cleanup.log') -Encoding utf8
    }
    if (-not $logsCollected) { 'Compose was not started; no container logs available.' | Set-Content -LiteralPath (Join-Path $evidence 'container-logs.log') }
    foreach ($name in $names) { [Environment]::SetEnvironmentVariable($name, $saved[$name], 'Process') }
    if (Test-Path (Join-Path $evidence 'manifest.txt')) {
        Get-ChildItem -LiteralPath $evidence -File | Where-Object Name -ne 'sha256.txt' |
            Get-FileHash -Algorithm SHA256 | ForEach-Object { "$($_.Hash.ToLower())  $([IO.Path]::GetFileName($_.Path))" } |
            Set-Content -LiteralPath (Join-Path $evidence 'sha256.txt') -Encoding ascii
    }
    Write-Host "Evidence: $evidence"
}

if ($exitCode -ne 0) { exit $exitCode }
