[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$Wave3Source,
    [Parameter(Mandatory = $true)][string]$Wave3Commit,
    [Parameter(Mandatory = $true)][string]$EvidenceDir
)
if ([string]::IsNullOrWhiteSpace($env:TASKLAB_ARTIFACT_ROOT)) { $env:TASKLAB_ARTIFACT_ROOT = Join-Path ([System.IO.Path]::GetTempPath()) 'distributed-task-lab' }
$TaskLabArtifactRoot = [System.IO.Path]::GetFullPath($env:TASKLAB_ARTIFACT_ROOT)


$ErrorActionPreference = 'Stop'
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$Wave3Source = (Resolve-Path -LiteralPath $Wave3Source).Path
$EvidenceDir = [System.IO.Path]::GetFullPath($EvidenceDir)
if (-not $EvidenceDir.StartsWith($TaskLabArtifactRoot, [StringComparison]::OrdinalIgnoreCase)) { throw 'EvidenceDir must be inside configured TASKLAB_ARTIFACT_ROOT.' }
if (Test-Path -LiteralPath $EvidenceDir) { throw "Evidence directory already exists: $EvidenceDir" }
New-Item -ItemType Directory -Path $EvidenceDir | Out-Null
$binaryDir = Join-Path $EvidenceDir 'binaries'
New-Item -ItemType Directory -Path $binaryDir | Out-Null

$suffix = [Guid]::NewGuid().ToString('N').Substring(0, 10)
$containerName = "tasklab-w4-migration-pg-$suffix"
$containerID = $null
$processes = [System.Collections.Generic.List[System.Diagnostics.Process]]::new()
$password = [Convert]::ToHexString([System.Security.Cryptography.RandomNumberGenerator]::GetBytes(24)).ToLowerInvariant()
$token = [Convert]::ToHexString([System.Security.Cryptography.RandomNumberGenerator]::GetBytes(32)).ToLowerInvariant()
$results = [System.Collections.Generic.List[object]]::new()

function Get-FreePort {
    $listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, 0)
    $listener.Start()
    $port = $listener.LocalEndpoint.Port
    $listener.Stop()
    return $port
}

function New-TestDatabase([string]$Name, [string]$Port) {
    & docker exec $containerID psql -v ON_ERROR_STOP=1 -U postgres -d postgres -c "CREATE DATABASE $Name" *> $null
    if ($LASTEXITCODE -ne 0) { throw "Could not create isolated database $Name." }
    return "postgres://postgres:${password}@127.0.0.1:${Port}/${Name}?sslmode=disable"
}

function Start-App([string]$Kind, [string]$Binary, [string]$DatabaseURL, [string]$Tag) {
    $isAPI = $Kind -eq 'api'
    $port = Get-FreePort
    $logOut = Join-Path $EvidenceDir "$Tag.stdout.log"
    $logErr = Join-Path $EvidenceDir "$Tag.stderr.log"
    $env:APP_PROFILE = 'development'
    $env:DATABASE_URL = $DatabaseURL
    $env:LOG_LEVEL = 'error'
    if ($isAPI) {
        $env:API_PORT = "$port"
        $env:API_TOKEN = $token
        $env:ADMIN_TOKEN = $token
        $uri = "http://127.0.0.1:$port/ready"
        $envNames = @('API_PORT', 'API_TOKEN', 'ADMIN_TOKEN')
        foreach ($name in @('WORKER_HEALTH_PORT', 'WORKER_ID')) { Remove-Item "Env:$name" -ErrorAction SilentlyContinue }
    } else {
        $env:WORKER_HEALTH_PORT = "$port"
        $env:WORKER_ID = "matrix-$Tag"
        $env:WORKER_CONCURRENCY = '1'
        $env:WORKER_POLL_INTERVAL = '100ms'
        $uri = "http://127.0.0.1:$port/ready"
        $envNames = @('WORKER_HEALTH_PORT', 'WORKER_ID', 'WORKER_CONCURRENCY', 'WORKER_POLL_INTERVAL')
        foreach ($name in @('API_PORT', 'API_TOKEN', 'ADMIN_TOKEN')) { Remove-Item "Env:$name" -ErrorAction SilentlyContinue }
    }
    $process = Start-Process -FilePath $Binary -WorkingDirectory $repoRoot -PassThru -WindowStyle Hidden -RedirectStandardOutput $logOut -RedirectStandardError $logErr
    $process | Add-Member -NotePropertyName 'EvidenceOut' -NotePropertyValue $logOut
    $process | Add-Member -NotePropertyName 'EvidenceErr' -NotePropertyValue $logErr
    $process | Add-Member -NotePropertyName 'ProbeURL' -NotePropertyValue $uri
    $process | Add-Member -NotePropertyName 'ServiceKind' -NotePropertyValue $Kind
    $processes.Add($process)
    return $process
}

function Wait-Ready($Process, [string]$Label) {
    $deadline = [DateTime]::UtcNow.AddSeconds(25)
    while ([DateTime]::UtcNow -lt $deadline) {
        $Process.Refresh()
        if ($Process.HasExited) { throw "$Label exited early with code $($Process.ExitCode)." }
        try {
            $response = Invoke-WebRequest -UseBasicParsing -Uri $Process.ProbeURL -TimeoutSec 2
            if ($response.StatusCode -eq 200) {
                $results.Add([ordered]@{ case = $Label; outcome = 'PASS'; readiness = 200 })
                return
            }
        } catch { }
        Start-Sleep -Milliseconds 250
    }
    throw "$Label readiness deadline expired."
}

function Stop-App($Process) {
    if (-not $Process.HasExited) { $Process.Kill(); $Process.WaitForExit(5000) | Out-Null }
}

try {
    $oldAPI = Join-Path $binaryDir 'wave3-api.exe'
    $oldWorker = Join-Path $binaryDir 'wave3-worker.exe'
    $newAPI = Join-Path $binaryDir 'wave4-api.exe'
    $newWorker = Join-Path $binaryDir 'wave4-worker.exe'
    Push-Location $Wave3Source
    try {
        & go build -trimpath -o $oldAPI ./cmd/api
        if ($LASTEXITCODE -ne 0) { throw 'Wave 3 API build failed.' }
        & go build -trimpath -o $oldWorker ./cmd/worker
        if ($LASTEXITCODE -ne 0) { throw 'Wave 3 Worker build failed.' }
    } finally { Pop-Location }
    Push-Location $repoRoot
    try {
        & go build -trimpath -o $newAPI ./cmd/api
        if ($LASTEXITCODE -ne 0) { throw 'Wave 4 API build failed.' }
        & go build -trimpath -o $newWorker ./cmd/worker
        if ($LASTEXITCODE -ne 0) { throw 'Wave 4 Worker build failed.' }
    } finally { Pop-Location }

    $containerID = (& docker run --detach --name $containerName --env "POSTGRES_PASSWORD=$password" --publish '127.0.0.1::5432' postgres:16-alpine).Trim()
    if ($LASTEXITCODE -ne 0 -or -not $containerID) { throw 'Could not start isolated PostgreSQL 16.' }
    $port = ((& docker port $containerID '5432/tcp').Trim() -split ':')[-1]
    $healthy = $false
    for ($attempt = 0; $attempt -lt 60; $attempt++) {
        & docker exec $containerID pg_isready -U postgres -d postgres *> $null
        if ($LASTEXITCODE -eq 0) { $healthy = $true; break }
        Start-Sleep -Seconds 1
    }
    if (-not $healthy) { throw 'Isolated PostgreSQL health check timed out.' }

    # Old binaries create and verify schema 11. New binaries must safely upgrade 11 to 12.
    $oldDB = New-TestDatabase "w4_old11_$suffix" $port
    $old11API = Start-App 'api' $oldAPI $oldDB 'old-api-schema11'
    Wait-Ready $old11API 'Wave3 API / schema11'
    $old11Worker = Start-App 'worker' $oldWorker $oldDB 'old-worker-schema11'
    Wait-Ready $old11Worker 'Wave3 Worker / schema11'
    Stop-App $old11Worker; Stop-App $old11API

    $newAPI11DB = New-TestDatabase "w4_newapi11_$suffix" $port
    $seedNewAPI11 = Start-App 'api' $oldAPI $newAPI11DB 'seed-new-api-schema11'
    Wait-Ready $seedNewAPI11 'Wave3 seed API / new API schema11 fixture'
    Stop-App $seedNewAPI11
    $newAPI11 = Start-App 'api' $newAPI $newAPI11DB 'new-api-schema11-upgrade'
    Wait-Ready $newAPI11 'Wave4 API auto-upgrade schema11→12'
    Stop-App $newAPI11

    $newWorker11DB = New-TestDatabase "w4_newworker11_$suffix" $port
    $seedNewWorker11 = Start-App 'api' $oldAPI $newWorker11DB 'seed-new-worker-schema11'
    Wait-Ready $seedNewWorker11 'Wave3 seed API / new Worker schema11 fixture'
    Stop-App $seedNewWorker11
    $newWorker11 = Start-App 'worker' $newWorker $newWorker11DB 'new-worker-schema11-upgrade'
    Wait-Ready $newWorker11 'Wave4 Worker auto-upgrade schema11→12'
    Stop-App $newWorker11

    $newDB = New-TestDatabase "w4_schema12_$suffix" $port
    $seedNew = Start-App 'api' $newAPI $newDB 'seed-schema12'
    Wait-Ready $seedNew 'Wave4 seed API / schema12 fixture'
    Stop-App $seedNew
    $new12API = Start-App 'api' $newAPI $newDB 'new-api-schema12'
    Wait-Ready $new12API 'Wave4 API / schema12'
    $new12Worker = Start-App 'worker' $newWorker $newDB 'new-worker-schema12'
    Wait-Ready $new12Worker 'Wave4 Worker / schema12'
    Stop-App $new12Worker; Stop-App $new12API

    foreach ($service in @(@{ kind = 'api'; binary = $oldAPI }, @{ kind = 'worker'; binary = $oldWorker })) {
        $oldOn12 = Start-App $service.kind $service.binary $newDB "old-$($service.kind)-schema12-reject"
        $deadline = [DateTime]::UtcNow.AddSeconds(20)
        while ([DateTime]::UtcNow -lt $deadline -and -not $oldOn12.HasExited) { Start-Sleep -Milliseconds 100; $oldOn12.Refresh() }
        if (-not $oldOn12.HasExited -or $oldOn12.ExitCode -eq 0) { Stop-App $oldOn12; throw "Wave3 $($service.kind) unexpectedly started on schema12." }
        $results.Add([ordered]@{ case = "Wave3 $($service.kind) / schema12"; outcome = 'BLOCKED_AS_EXPECTED'; exit_code = $oldOn12.ExitCode; expected = 'database schema version 12 is newer than embedded version 11' })
    }

    $matrix = [ordered]@{
        source_wave3 = $Wave3Commit
        source_wave4 = (& git -C $repoRoot rev-parse HEAD).Trim()
        source_wave4_tree = (& git -C $repoRoot show -s --format=%T HEAD).Trim()
        postgres = '16.x isolated container'
        results = $results
        interpretation = 'Wave3 API/Worker work on schema11; Wave3 API/Worker fail closed on schema12 during RunMigrations. Wave4 API/Worker auto-upgrade schema11 to 12 and run on schema12.'
        rollback = 'BLOCKED: not safe to roll Wave4 migration 012 back to Wave3 binaries.'
    }
    $matrix | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath (Join-Path $EvidenceDir 'matrix.json')
}
catch {
    $_ | Out-String | Set-Content -LiteralPath (Join-Path $EvidenceDir 'failure.txt')
    throw
}
finally {
    foreach ($process in $processes) {
        if ($null -eq $process) { continue }
        try { if (-not $process.HasExited) { Stop-App $process } }
        catch { "process cleanup: $($_.Exception.Message)" | Add-Content -LiteralPath (Join-Path $EvidenceDir 'cleanup-errors.log') }
    }
    if ($containerID) {
        try { & docker logs $containerID 2>&1 | Set-Content -LiteralPath (Join-Path $EvidenceDir 'postgres-container.log') }
        catch { "postgres log collection: $($_.Exception.Message)" | Add-Content -LiteralPath (Join-Path $EvidenceDir 'cleanup-errors.log') }
        try { & docker rm --force $containerID *> $null }
        catch { "postgres container cleanup: $($_.Exception.Message)" | Add-Content -LiteralPath (Join-Path $EvidenceDir 'cleanup-errors.log') }
    }
    try { Remove-Item Env:APP_PROFILE,Env:DATABASE_URL,Env:LOG_LEVEL,Env:API_PORT,Env:API_TOKEN,Env:ADMIN_TOKEN,Env:WORKER_HEALTH_PORT,Env:WORKER_ID,Env:WORKER_CONCURRENCY,Env:WORKER_POLL_INTERVAL -ErrorAction SilentlyContinue }
    catch { "environment cleanup: $($_.Exception.Message)" | Add-Content -LiteralPath (Join-Path $EvidenceDir 'cleanup-errors.log') }
    try {
        $files = Get-ChildItem -LiteralPath $EvidenceDir -File -Recurse | Where-Object Name -ne 'SHA256SUMS.txt' | ForEach-Object {
            $relative = [System.IO.Path]::GetRelativePath($EvidenceDir, $_.FullName)
            $hash = (Get-FileHash -Algorithm SHA256 -LiteralPath $_.FullName).Hash.ToLowerInvariant()
            "$hash  $relative"
        }
        $files | Set-Content -LiteralPath (Join-Path $EvidenceDir 'SHA256SUMS.txt')
    }
    catch { "checksum generation: $($_.Exception.Message)" | Add-Content -LiteralPath (Join-Path $EvidenceDir 'cleanup-errors.log') }
}
"Migration matrix evidence: $EvidenceDir"
