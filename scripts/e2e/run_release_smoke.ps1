param(
    [string]$EvidenceRoot = "E:\distributed-task-lab-temp\wave4-artifacts\release-smoke-runs"
)

$ErrorActionPreference = "Stop"
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
$composeFile = Join-Path $repoRoot "deployments\release-smoke-compose.yml"
$runId = "release-" + [Guid]::NewGuid().ToString("N").Substring(0, 12)
$project = "tasklab-$runId"
$evidenceDir = Join-Path $EvidenceRoot $runId
$rng = [System.Security.Cryptography.RandomNumberGenerator]::Create()
function New-Token([int]$Length = 32) {
    $data = New-Object byte[] $Length
    $rng.GetBytes($data)
    return [Convert]::ToHexString($data).ToLowerInvariant()
}
$listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, 0)
$listener.Start()
$apiPort = $listener.LocalEndpoint.Port
$listener.Stop()
New-Item -ItemType Directory -Force -Path $evidenceDir | Out-Null
$env:SMOKE_POSTGRES_PASSWORD = New-Token 24
$env:SMOKE_API_TOKEN = New-Token
$env:SMOKE_ADMIN_TOKEN = New-Token
$env:SMOKE_API_PORT = "$apiPort"
$env:GOPROXY = if ($env:GOPROXY) { $env:GOPROXY } else { "https://proxy.golang.org,direct" }
$apiBase = "http://127.0.0.1:$apiPort"

function Invoke-Compose {
    param([string[]]$ComposeArgs)
    & docker compose --project-name $project --file $composeFile @ComposeArgs
    if ($LASTEXITCODE -ne 0) { throw "docker compose failed ($LASTEXITCODE): $($ComposeArgs -join ' ')" }
}

function Wait-Task([string]$TaskID, [string]$Status, [int]$TimeoutSeconds = 30) {
    $deadline = [DateTime]::UtcNow.AddSeconds($TimeoutSeconds)
    while ([DateTime]::UtcNow -lt $deadline) {
        try {
            $task = Invoke-RestMethod -Uri "$apiBase/tasks/$TaskID" -Headers @{ Authorization = "Bearer $env:SMOKE_API_TOKEN" } -TimeoutSec 3
            if ($task.status -eq $Status) { return $task }
            if ($task.status -eq "dlq" -or $task.status -eq "failed") { throw "task reached terminal status $($task.status)" }
        } catch {
            if ($_.Exception.Message -like "task reached terminal status*") { throw }
        }
        Start-Sleep -Milliseconds 150
    }
    throw "task $TaskID did not reach $Status within ${TimeoutSeconds}s"
}

try {
    Invoke-Compose -ComposeArgs @("up", "--build", "--wait", "--wait-timeout", "240")
    $readyDeadline = [DateTime]::UtcNow.AddSeconds(60)
    while ([DateTime]::UtcNow -lt $readyDeadline) {
        try {
            $ready = Invoke-WebRequest -UseBasicParsing -Uri "$apiBase/ready" -TimeoutSec 3
            if ($ready.StatusCode -eq 200) { break }
        } catch { }
        Start-Sleep -Milliseconds 500
    }
    if ($null -eq $ready -or $ready.StatusCode -ne 200) { throw "API readiness failed" }

    $payload = @{
        type = "rag_ingestion"
        queue = "rag_ingestion"
        payload = @{ version = "1"; kind = "rag_ingestion"; data = @{ execution_mode = "mock"; document_id = "release-smoke-$runId"; mock_duration_ms = 5000 } }
        max_attempts = 3
        timeout_seconds = 30
        retry_delay_seconds = 0
    } | ConvertTo-Json -Depth 8
    $task = Invoke-RestMethod -Method Post -Uri "$apiBase/tasks" -Headers @{ Authorization = "Bearer $env:SMOKE_API_TOKEN" } -ContentType "application/json" -Body $payload -TimeoutSec 5
    $running = Wait-Task $task.id "running" 20
    $workerID = (& docker compose --project-name $project --file $composeFile ps -q worker).Trim()
    if (-not $workerID) { throw "Worker container ID was not available" }
    $started = [DateTime]::UtcNow
    & docker stop --time 15 $workerID | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "Worker SIGTERM stop failed ($LASTEXITCODE)" }
    $stoppedInSeconds = ([DateTime]::UtcNow - $started).TotalSeconds
    $completed = Wait-Task $task.id "success" 15
    $passed = $completed.status -eq "success" -and $completed.attempt -eq 1 -and $stoppedInSeconds -lt 15
    if (-not $passed) { throw "drain assertions failed: status=$($completed.status) attempt=$($completed.attempt) elapsed=$stoppedInSeconds" }

    $result = [ordered]@{
        result = "PASS"
        source_commit = (git -C $repoRoot rev-parse HEAD)
        task_id = $task.id
        initial_status = $running.status
        final_status = $completed.status
        attempt = $completed.attempt
        worker_sigterm_drain_seconds = [Math]::Round($stoppedInSeconds, 3)
        assertions = @("API became ready against a fresh PostgreSQL database", "Worker accepted task and reached running", "SIGTERM drained in-flight handler", "task succeeded at attempt 1", "worker stopped before 15 second container timeout")
        rollback_status = "BLOCKED: compatibility with the prior binary after migration 012 is not yet supported or verified"
    }
    $result | ConvertTo-Json -Depth 8 | Set-Content (Join-Path $evidenceDir "result.json")
    "Release smoke PASS. Evidence: $evidenceDir"
}
catch {
    $_ | Out-String | Set-Content (Join-Path $evidenceDir "failure.txt")
    throw
}
finally {
    & docker compose --project-name $project --file $composeFile logs --no-color api worker postgres 2>&1 |
        Set-Content (Join-Path $evidenceDir "container-logs.txt")
    & docker compose --project-name $project --file $composeFile down --volumes --remove-orphans 2>&1 |
        Set-Content (Join-Path $evidenceDir "cleanup.txt")
    Remove-Item Env:SMOKE_POSTGRES_PASSWORD,Env:SMOKE_API_TOKEN,Env:SMOKE_ADMIN_TOKEN,Env:SMOKE_API_PORT -ErrorAction SilentlyContinue
    $rng.Dispose()
}
