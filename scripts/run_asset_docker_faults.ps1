param(
    [Parameter(Mandatory = $true)]
    [string]$ResultDir
)

$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path -Parent $PSScriptRoot
$composeFile = Join-Path $repoRoot 'deployments/docker-compose.yml'
$overrideFile = Join-Path $ResultDir 'docker-compose.asset-faults.yml'
$apiToken = 'assetlab-api-token-20260823'
$adminToken = 'assetlab-admin-token-20260823'
$env:API_TOKEN = $apiToken
$env:ADMIN_TOKEN = $adminToken

[IO.Directory]::CreateDirectory($ResultDir) | Out-Null
[IO.File]::WriteAllText($overrideFile, @'
services:
  api:
    environment:
      DB_OPERATION_TIMEOUT: "1s"
  worker:
    environment:
      WORKER_CONCURRENCY: "1"
      WORKER_POLL_INTERVAL: "100ms"
      LEASE_TIMEOUT: "2s"
      CLAIM_BATCH_SIZE: "1"
      SHUTDOWN_TIMEOUT: "10s"
      DB_OPERATION_TIMEOUT: "1s"
'@)

function Compose([string]$Project, [string[]]$Arguments) {
    & docker compose -p $Project -f $composeFile -f $overrideFile @Arguments
    if ($LASTEXITCODE -ne 0) { throw "docker compose failed: $Project $Arguments" }
}

function Write-Json([string]$Path, $Value) {
    [IO.File]::WriteAllText($Path, (($Value | ConvertTo-Json -Depth 20) + "`n"))
}

function Add-Event([string]$Path, [datetime]$Started, [string]$Process, [string]$Task, [string]$Operation, [string]$Phase, $Metadata) {
    $event = [ordered]@{
        timestamp = [datetime]::UtcNow.ToString('o')
        elapsed_ms = [int64](([datetime]::UtcNow - $Started.ToUniversalTime()).TotalMilliseconds)
        process = $Process
        task_id = $Task
        operation = $Operation
        phase = $Phase
        metadata = $Metadata
    }
    [IO.File]::AppendAllText($Path, (($event | ConvertTo-Json -Compress -Depth 10) + "`n"))
}

function Wait-Ready([int]$Seconds = 90) {
    $deadline = [datetime]::UtcNow.AddSeconds($Seconds)
    while ([datetime]::UtcNow -lt $deadline) {
        try {
            $response = Invoke-WebRequest -UseBasicParsing -Uri 'http://localhost:8080/ready' -TimeoutSec 2
            if ($response.StatusCode -eq 200) { return }
        } catch {}
        Start-Sleep -Milliseconds 500
    }
    throw 'API readiness timeout'
}

function Submit-Task([string]$DocumentID, [int]$DurationMS) {
    $body = @{
        type = 'rag_ingestion'
        queue = 'rag_ingestion'
        payload = @{ version = '1'; kind = 'rag_ingestion'; data = @{ document_id = $DocumentID; mock_duration_ms = $DurationMS } }
        max_attempts = 5
        timeout_seconds = 30
        retry_delay_seconds = 0
    } | ConvertTo-Json -Depth 8
    return Invoke-RestMethod -Method Post -Uri 'http://localhost:8080/tasks' -Headers @{ Authorization = "Bearer $apiToken" } -ContentType 'application/json' -Body $body
}

function Get-Task([string]$TaskID) {
    return Invoke-RestMethod -Uri "http://localhost:8080/tasks/$TaskID" -Headers @{ Authorization = "Bearer $apiToken" } -TimeoutSec 3
}

function Wait-Task([string]$TaskID, [string]$Status, [int]$Seconds = 45) {
    $deadline = [datetime]::UtcNow.AddSeconds($Seconds)
    while ([datetime]::UtcNow -lt $deadline) {
        try {
            $task = Get-Task $TaskID
            if ($task.status -eq $Status) { return $task }
            if ($task.status -eq 'dlq') { throw "task $TaskID reached dlq at attempt $($task.attempt)" }
        } catch {}
        Start-Sleep -Milliseconds 150
    }
    throw "task $TaskID did not reach $Status"
}

function Capture([string]$Project, [string]$Directory, [string]$TaskID) {
    $logs = (& docker compose -p $Project -f $composeFile -f $overrideFile logs --no-color --timestamps 2>&1) -join "`n"
    [IO.File]::WriteAllText((Join-Path $Directory 'docker.log'), $logs + "`n")
    $postgres = "$Project-postgres-1"
    $sql = "SELECT jsonb_build_object('task',(SELECT to_jsonb(t) FROM tasks t WHERE id='$TaskID'),'events',(SELECT jsonb_agg(to_jsonb(e) ORDER BY id) FROM task_events e WHERE task_id='$TaskID'),'workers',(SELECT jsonb_agg(to_jsonb(w) ORDER BY worker_id) FROM workers w))::text;"
    $snapshot = (& docker exec $postgres psql -U postgres -d tasklab -At -c $sql 2>&1) -join "`n"
    [IO.File]::WriteAllText((Join-Path $Directory 'db-after.json'), $snapshot + "`n")
}

function Start-Stack([string]$Project) {
    Compose $Project @('up', '-d', '--build', '--scale', 'worker=1')
    Wait-Ready
}

function Stop-Stack([string]$Project) {
    try { Compose $Project @('down', '-v', '--remove-orphans') } catch { Write-Warning $_ }
}

$summary = [ordered]@{}

# A real container SIGKILL: worker A owns and renews, worker B starts, A is
# killed, then B recovers only after the database lease expires.
$project = 'tlassethardcrash'
$dir = Join-Path $ResultDir 'TL-CRASH-002'
[IO.Directory]::CreateDirectory($dir) | Out-Null
$history = Join-Path $dir 'operations.jsonl'
$started = [datetime]::UtcNow
try {
    Start-Stack $project
    $task = Submit-Task 'asset-hard-crash' 10000
    Add-Event $history $started 'client' $task.id 'enqueue' 'ok' @{ duration_ms = 10000 }
    $running = Wait-Task $task.id 'running' 15
    Add-Event $history $started $running.leased_by $task.id 'claim' 'ok' @{ attempt = $running.attempt; lease_version = $running.lease_version }
    Compose $project @('up', '-d', '--scale', 'worker=2')
    Add-Event $history $started 'nemesis' $task.id 'docker_kill' 'invoke' @{ container = "$project-worker-1" }
    & docker kill "$project-worker-1" | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'docker kill failed' }
    Add-Event $history $started 'nemesis' $task.id 'docker_kill' 'ok' @{ signal = 'SIGKILL' }
    $done = Wait-Task $task.id 'success' 60
    Add-Event $history $started 'recovery-scan' $task.id 'terminal_read' 'ok' @{ status = $done.status; attempt = $done.attempt; lease_version = $done.lease_version }
    Capture $project $dir $task.id
    $passed = $done.status -eq 'success' -and $done.attempt -ge 2 -and $done.lease_version -ge 2
    Write-Json (Join-Path $dir 'result.json') @{ experiment_id='TL-CRASH-002'; passed=$passed; assertions=@(@{name='sigkill_reclaimed_after_lease';passed=$passed;observed=@{status=$done.status;attempt=$done.attempt;lease_version=$done.lease_version};expected='success, attempt>=2, lease_version>=2'}) }
    $summary['TL-CRASH-002'] = $passed
} catch {
    if ($null -ne $task) {
        try { Capture $project $dir $task.id } catch {}
        try { Write-Json (Join-Path $dir 'failure-debug.json') @{ error=$_.Exception.Message; task=(Get-Task $task.id) } } catch {}
    }
    throw
} finally { Stop-Stack $project }

# A real Docker SIGTERM must stop new work, keep the lease alive while the
# cooperative handler drains, persist success, and exit within the grace time.
$project = 'tlassetgraceful'
$dir = Join-Path $ResultDir 'TL-SHUT-002'
[IO.Directory]::CreateDirectory($dir) | Out-Null
$history = Join-Path $dir 'operations.jsonl'
$started = [datetime]::UtcNow
try {
    Start-Stack $project
    $task = Submit-Task 'asset-graceful' 3000
    $running = Wait-Task $task.id 'running' 15
    Add-Event $history $started $running.leased_by $task.id 'claim' 'ok' @{ attempt=$running.attempt; lease_version=$running.lease_version }
    Add-Event $history $started 'nemesis' $task.id 'docker_stop' 'invoke' @{ signal='SIGTERM'; timeout_seconds=10 }
    & docker stop --time 10 "$project-worker-1" | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'docker stop failed' }
    Add-Event $history $started 'nemesis' $task.id 'docker_stop' 'ok' @{ signal='SIGTERM' }
    $done = Wait-Task $task.id 'success' 15
    Capture $project $dir $task.id
    $passed = $done.status -eq 'success' -and $done.attempt -eq 1
    Write-Json (Join-Path $dir 'result.json') @{ experiment_id='TL-SHUT-002'; passed=$passed; assertions=@(@{name='sigterm_drained_inflight';passed=$passed;observed=@{status=$done.status;attempt=$done.attempt};expected='success at attempt 1'}) }
    $summary['TL-SHUT-002'] = $passed
} finally { Stop-Stack $project }

# PostgreSQL pause makes readiness fail and renewal lose its generation. After
# unpause, the disposable worker fleet must recover the expired task.
$project = 'tlassetdbfault'
$dir = Join-Path $ResultDir 'TL-DBFAULT-001'
[IO.Directory]::CreateDirectory($dir) | Out-Null
$history = Join-Path $dir 'operations.jsonl'
$started = [datetime]::UtcNow
try {
    Start-Stack $project
    $task = Submit-Task 'asset-db-pause' 6000
    $running = Wait-Task $task.id 'running' 15
    $postgres = "$project-postgres-1"
    Add-Event $history $started 'nemesis' $task.id 'docker_pause_postgres' 'invoke' @{ container=$postgres }
    & docker pause $postgres | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'docker pause failed' }
    Start-Sleep -Seconds 3
    $readyFailed = $false
    try { Invoke-WebRequest -UseBasicParsing -Uri 'http://localhost:8080/ready' -TimeoutSec 2 | Out-Null } catch { $readyFailed = $true }
    Add-Event $history $started 'api-ready' $task.id 'readiness_during_db_pause' 'ok' @{ unavailable=$readyFailed }
    & docker unpause $postgres | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'docker unpause failed' }
    Add-Event $history $started 'nemesis' $task.id 'docker_unpause_postgres' 'ok' @{}
    Wait-Ready 20
    $done = Wait-Task $task.id 'success' 45
    Capture $project $dir $task.id
    $passed = $readyFailed -and $done.status -eq 'success' -and $done.attempt -ge 2
    Write-Json (Join-Path $dir 'result.json') @{ experiment_id='TL-DBFAULT-001'; passed=$passed; assertions=@(@{name='readiness_failed_closed';passed=$readyFailed;observed=$readyFailed;expected=$true},@{name='task_recovered_after_database_unpause';passed=($done.status -eq 'success' -and $done.attempt -ge 2);observed=@{status=$done.status;attempt=$done.attempt};expected='success, attempt>=2'}) }
    $summary['TL-DBFAULT-001'] = $passed
} finally { Stop-Stack $project }

Write-Json (Join-Path $ResultDir 'docker-fault-summary.json') $summary
if ($summary.Values -contains $false) { exit 1 }
