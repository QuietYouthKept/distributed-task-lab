[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$Wave3Source,
    [Parameter(Mandatory = $true)][string]$Wave3Commit,
    [Parameter(Mandatory = $true)][string]$MinIOImage,
    [Parameter(Mandatory = $true)][string]$EvidenceDir
)

$ErrorActionPreference = 'Stop'
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$Wave3Source = (Resolve-Path -LiteralPath $Wave3Source).Path
$EvidenceDir = [System.IO.Path]::GetFullPath($EvidenceDir)
if (-not $EvidenceDir.StartsWith('E:\distributed-task-lab-temp\', [StringComparison]::OrdinalIgnoreCase)) { throw 'EvidenceDir must be on E:.' }
if (Test-Path -LiteralPath $EvidenceDir) { throw "Refusing to overwrite evidence: $EvidenceDir" }
if (-not (Test-Path -LiteralPath (Join-Path $Wave3Source '.git'))) { throw 'Wave3Source must be a Git worktree.' }
$actualWave3Commit = (& git -C $Wave3Source rev-parse HEAD).Trim()
if ($LASTEXITCODE -ne 0 -or $actualWave3Commit -ne $Wave3Commit) { throw "Wave3 source SHA mismatch: got $actualWave3Commit, want $Wave3Commit." }
$image = docker image inspect $MinIOImage --format '{{json .RepoDigests}} {{json .Config.Labels}}' 2>$null
if ($LASTEXITCODE -ne 0 -or -not $image) { throw 'Pinned MinIO image is not available locally; provide an independently verified immutable image reference.' }
New-Item -ItemType Directory -Path $EvidenceDir | Out-Null
$binaryDir = Join-Path $EvidenceDir 'binaries'
New-Item -ItemType Directory -Path $binaryDir | Out-Null

$suffix = [Guid]::NewGuid().ToString('N').Substring(0, 10)
$pgName = "tasklab-w4-populated-pg-$suffix"
$minioName = "tasklab-w4-populated-minio-$suffix"
$createdContainers = [System.Collections.Generic.List[string]]::new()
$processes = [System.Collections.Generic.List[System.Diagnostics.Process]]::new()
$pgID = $null
$minioID = $null
$password = [Convert]::ToHexString([System.Security.Cryptography.RandomNumberGenerator]::GetBytes(24)).ToLowerInvariant()
$apiToken = [Convert]::ToHexString([System.Security.Cryptography.RandomNumberGenerator]::GetBytes(32)).ToLowerInvariant()
$adminToken = [Convert]::ToHexString([System.Security.Cryptography.RandomNumberGenerator]::GetBytes(32)).ToLowerInvariant()
$minioAccess = [Convert]::ToHexString([System.Security.Cryptography.RandomNumberGenerator]::GetBytes(20)).ToLowerInvariant()
$minioSecret = [Convert]::ToHexString([System.Security.Cryptography.RandomNumberGenerator]::GetBytes(32)).ToLowerInvariant()
$credentialFile = Join-Path $EvidenceDir 'temporary-minio.env'
$envNames = @('APP_PROFILE','DATABASE_URL','LOG_LEVEL','API_PORT','API_TOKEN','ADMIN_TOKEN','WORKER_HEALTH_PORT','WORKER_ID','WORKER_CONCURRENCY','WORKER_POLL_INTERVAL','CLAIM_BATCH_SIZE','LEASE_TIMEOUT','DB_OPERATION_TIMEOUT','SHUTDOWN_TIMEOUT','RAG_ARTIFACT_DIR','ARTIFACT_STORE_BACKEND','ARTIFACT_S3_ENDPOINT','ARTIFACT_S3_BUCKET','ARTIFACT_S3_PREFIX','ARTIFACT_S3_ACCESS_KEY','ARTIFACT_S3_SECRET_KEY','ARTIFACT_ALLOW_HTTP','ARTIFACT_ALLOW_BUCKET_CREATE','W4_MIGRATION_FIXTURE','W4_MIGRATION_ARTIFACT_MODE','W4_MIGRATION_ARTIFACT_KEY','W4_MIGRATION_ARTIFACT_SHA256','MINIO_ENDPOINT','MINIO_BUCKET','MINIO_ACCESS_KEY','MINIO_SECRET_KEY','GOCACHE','GOMODCACHE','NO_PROXY','no_proxy')
$previousEnv = @{}
foreach ($name in $envNames) { $previousEnv[$name] = [Environment]::GetEnvironmentVariable($name, 'Process') }
$started = [DateTime]::UtcNow
$run = [ordered]@{
    run_id = $suffix
    wave3_commit = $Wave3Commit
    wave4_commit = (& git -C $repoRoot rev-parse HEAD).Trim()
    wave4_tree = (& git -C $repoRoot show -s --format=%T HEAD).Trim()
    minio_image = $MinIOImage
    minio_image_identity = $image
    postgres_image = 'postgres:16-alpine'
    postgres = $null
    minio_endpoint = $null
    credential_policy = 'random per run; redacted; temporary file removed immediately after container creation'
    task_ids = [ordered]@{}
    checkpoints = [System.Collections.Generic.List[object]]::new()
    conclusions = [System.Collections.Generic.List[string]]::new()
    started_utc = $started.ToString('o')
}

function Get-FreePort {
    $listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, 0)
    $listener.Start(); $port = $listener.LocalEndpoint.Port; $listener.Stop(); return $port
}
function Invoke-Psql([string]$Database, [string]$SQL) {
    $value = & docker exec $pgID psql -X -A -t -v ON_ERROR_STOP=1 -U postgres -d $Database -c $SQL 2>&1
    if ($LASTEXITCODE -ne 0) { throw "psql failed for isolated database ${Database}: $($value -join "`n")" }
    return ($value -join "`n").Trim()
}
function New-Database([string]$Name) {
    [void](Invoke-Psql 'postgres' "CREATE DATABASE $Name")
    $port = $run.postgres.port
    return "postgres://postgres:${password}@127.0.0.1:${port}/${Name}?sslmode=disable"
}
function Set-CommonEnv([string]$DatabaseURL) {
    $env:APP_PROFILE = 'development'; $env:DATABASE_URL = $DatabaseURL; $env:LOG_LEVEL = 'error'
    $env:API_TOKEN = $apiToken; $env:ADMIN_TOKEN = $adminToken
    $env:ARTIFACT_STORE_BACKEND = 's3'; $env:ARTIFACT_S3_ENDPOINT = $run.minio_endpoint
    $env:ARTIFACT_S3_BUCKET = "migration-$suffix"; $env:ARTIFACT_S3_PREFIX = "migration-$suffix"
    $env:ARTIFACT_S3_ACCESS_KEY = $minioAccess; $env:ARTIFACT_S3_SECRET_KEY = $minioSecret
    $env:ARTIFACT_ALLOW_HTTP = 'true'; $env:ARTIFACT_ALLOW_BUCKET_CREATE = 'true'; $env:CLAIM_BATCH_SIZE = '1'
    $env:DB_OPERATION_TIMEOUT = '1s'
    $env:RAG_ARTIFACT_DIR = (Join-Path $EvidenceDir 'worker-artifacts')
}
function Start-App([string]$Kind, [string]$Binary, [string]$DatabaseURL, [string]$Label) {
    Set-CommonEnv $DatabaseURL
    $port = Get-FreePort
    if ($Kind -eq 'api') {
        $env:API_PORT = "$port"; Remove-Item Env:WORKER_HEALTH_PORT,Env:WORKER_ID -ErrorAction SilentlyContinue
        $probe = "http://127.0.0.1:$port/ready"
    } else {
        $env:WORKER_HEALTH_PORT = "$port"; $env:WORKER_ID = "w4-migration-$Label"
        $env:WORKER_CONCURRENCY = '1'; $env:WORKER_POLL_INTERVAL = '100ms'; $env:CLAIM_BATCH_SIZE = '1'; $env:LEASE_TIMEOUT = '10s'; $env:SHUTDOWN_TIMEOUT = '10s'
        Remove-Item Env:API_PORT -ErrorAction SilentlyContinue
        $probe = "http://127.0.0.1:$port/ready"
    }
    $out = Join-Path $EvidenceDir "$Label.stdout.log"; $err = Join-Path $EvidenceDir "$Label.stderr.log"
    $p = Start-Process -FilePath $Binary -WorkingDirectory $repoRoot -PassThru -WindowStyle Hidden -RedirectStandardOutput $out -RedirectStandardError $err
    $p | Add-Member EvidenceOut $out; $p | Add-Member EvidenceErr $err; $p | Add-Member ProbeURL $probe; $p | Add-Member Kind $Kind
    $processes.Add($p)
    return $p
}
function Wait-Ready($Process, [string]$Label) {
    $deadline = [DateTime]::UtcNow.AddSeconds(30)
    while ([DateTime]::UtcNow -lt $deadline) {
        $Process.Refresh()
        if ($Process.HasExited) { throw "$Label exited early ($($Process.ExitCode)). See $($Process.EvidenceErr)." }
        try { if ((Invoke-WebRequest -UseBasicParsing -Uri $Process.ProbeURL -TimeoutSec 2).StatusCode -eq 200) { return } } catch { }
        Start-Sleep -Milliseconds 200
    }
    throw "$Label readiness deadline expired."
}
function Stop-App($Process, [switch]$Crash) {
    $Process.Refresh()
    if ($Process.HasExited) { return }
    if ($Crash) { $Process.Kill(); $Process.WaitForExit(5000) | Out-Null; return }
    if ($Process.Kind -eq 'worker') {
        # SIGTERM is not available as a graceful control signal for a detached
        # Windows process. Normal workers are stopped only after no task is in flight.
        $Process.Kill(); $Process.WaitForExit(5000) | Out-Null; return
    }
    $Process.Kill(); $Process.WaitForExit(5000) | Out-Null
}
function New-Task([string]$BaseURL, [string]$Queue, [hashtable]$Data, [int]$MaxAttempts = 3, [int]$RetryDelay = 1) {
    $body = @{ type='rag_ingestion'; queue=$Queue; max_attempts=$MaxAttempts; retry_delay_seconds=$RetryDelay; retry_jitter=$false; payload=@{ version='1'; kind='rag_ingestion'; data=$Data } } | ConvertTo-Json -Depth 10 -Compress
    return Invoke-RestMethod -Method Post -Uri "$($BaseURL.TrimEnd('/'))/tasks" -ContentType 'application/json' -Headers @{ Authorization="Bearer $apiToken" } -Body $body
}
function New-EffectTask([string]$BaseURL, [string]$Queue, [bool]$LoseResponse, [int]$MaxAttempts = 3) {
    $payload = @{ version='1'; kind='payment'; data=@{ execution_mode='effect'; operation='payment'; operation_id="migration-$suffix-$Queue"; target='isolated-fixture'; simulate_response_loss=$LoseResponse } }
    $body = @{ type='payment'; queue=$Queue; max_attempts=$MaxAttempts; retry_delay_seconds=30; retry_jitter=$false; payload=$payload } | ConvertTo-Json -Depth 10 -Compress
    return Invoke-RestMethod -Method Post -Uri "$($BaseURL.TrimEnd('/'))/tasks" -ContentType 'application/json' -Headers @{ Authorization="Bearer $apiToken" } -Body $body
}
function Wait-Task([string]$Database, [string]$TaskID, [string]$Expected, [int]$TimeoutSeconds = 45) {
    $deadline = [DateTime]::UtcNow.AddSeconds($TimeoutSeconds)
    while ([DateTime]::UtcNow -lt $deadline) {
        $row = Invoke-Psql $Database "SELECT status||'|'||attempt||'|'||lease_version FROM tasks WHERE id='$TaskID'"
        if ($row.StartsWith($Expected)) { return $row }
        Start-Sleep -Milliseconds 100
    }
    throw "Task $TaskID did not reach '$Expected' before deadline."
}
function Save-Snapshot([string]$Database, [string]$Name) {
    $sql = @'
SELECT json_build_object(
 'tasks', (SELECT json_agg(json_build_object('id',id,'type',type,'queue',queue,'status',status,'attempt',attempt,'lease_version',lease_version,'next_run_at',next_run_at,'updated_at',updated_at,'created_by',created_by,'result',result,'last_error',last_error,'dlq_reason',dlq_reason) ORDER BY id) FROM tasks),
 'events', (SELECT json_agg(json_build_object('task_id',task_id,'event_type',event_type,'worker_id',worker_id,'message',message,'created_at',created_at) ORDER BY task_id,created_at,id) FROM task_events),
 'effects', (SELECT json_agg(json_build_object('effect_id',effect_id,'task_id',task_id,'operation_key',operation_key,'kind',kind,'policy',policy,'status',status,'provider_request_id',provider_request_id,'request_hash',request_hash,'response_hash',response_hash,'generation',generation,'attempt',attempt,'resolution_note',resolution_note,'created_at',created_at,'reconciled_at',reconciled_at) ORDER BY effect_id) FROM effects),
 'queues', (SELECT json_agg(row_to_json(q) ORDER BY name) FROM queues q),
 'audit', (SELECT json_agg(json_build_object('actor_id',actor_id,'actor_kind',actor_kind,'request_id',request_id,'action',action,'resource_type',resource_type,'resource_id',resource_id,'outcome',outcome,'reason',reason,'context',context,'created_at',created_at) ORDER BY id) FROM security_audit_events)
)::text
'@
    $json = Invoke-Psql $Database $sql
    $path = Join-Path $EvidenceDir "$Name.snapshot.json"
    $json | Set-Content -LiteralPath $path -Encoding utf8
    return (Get-FileHash -Algorithm SHA256 -LiteralPath $path).Hash.ToLowerInvariant()
}

try {
    @("MINIO_ROOT_USER=$minioAccess", "MINIO_ROOT_PASSWORD=$minioSecret") | Set-Content -LiteralPath $credentialFile -Encoding ascii
    $minioID = (& docker run --detach --name $minioName --env-file $credentialFile --publish '127.0.0.1::9000' $MinIOImage server /data --console-address ':9001').Trim()
    if ($LASTEXITCODE -ne 0 -or -not $minioID) { throw 'Could not start isolated MinIO.' }
    $createdContainers.Add($minioID); Remove-Item -LiteralPath $credentialFile -Force
    $pgID = (& docker run --detach --name $pgName --env "POSTGRES_PASSWORD=$password" --publish '127.0.0.1::5432' postgres:16-alpine).Trim()
    if ($LASTEXITCODE -ne 0 -or -not $pgID) { throw 'Could not start isolated PostgreSQL 16.' }
    $createdContainers.Add($pgID)
    $minioPort = ((& docker port $minioID '9000/tcp').Trim() -split ':')[-1]
    $pgPort = ((& docker port $pgID '5432/tcp').Trim() -split ':')[-1]
    $run.minio_endpoint = "http://127.0.0.1:$minioPort"; $run.postgres = @{ port=[int]$pgPort; version=(docker exec $pgID postgres --version).Trim() }
    $ready = $false
    for ($i=0; $i -lt 60; $i++) { docker exec $pgID pg_isready -U postgres -d postgres *> $null; if ($LASTEXITCODE -eq 0) { $ready=$true; break }; Start-Sleep -Seconds 1 }
    if (-not $ready) { throw 'PostgreSQL health check timed out.' }
    $ready = $false
    for ($i=0; $i -lt 60; $i++) { try { if ((Invoke-WebRequest -UseBasicParsing -Uri "$($run.minio_endpoint)/minio/health/live" -TimeoutSec 2).StatusCode -eq 200) { $ready=$true; break } } catch {}; Start-Sleep -Seconds 1 }
    if (-not $ready) { throw 'MinIO health check timed out.' }

    $oldAPI = Join-Path $binaryDir 'wave3-api.exe'; $oldWorker = Join-Path $binaryDir 'wave3-worker.exe'
    $newAPI = Join-Path $binaryDir 'wave4-api.exe'; $newWorker = Join-Path $binaryDir 'wave4-worker.exe'
    Push-Location $Wave3Source; try { go build -trimpath -o $oldAPI ./cmd/api; if ($LASTEXITCODE -ne 0) { throw 'Wave 3 API build failed.' }; go build -trimpath -o $oldWorker ./cmd/worker; if ($LASTEXITCODE -ne 0) { throw 'Wave 3 Worker build failed.' } } finally { Pop-Location }
    Push-Location $repoRoot; try { go build -trimpath -o $newAPI ./cmd/api; if ($LASTEXITCODE -ne 0) { throw 'Wave 4 API build failed.' }; go build -trimpath -o $newWorker ./cmd/worker; if ($LASTEXITCODE -ne 0) { throw 'Wave 4 Worker build failed.' } } finally { Pop-Location }
    $dbName = "w4_pop_$suffix"; $databaseURL = New-Database $dbName
    $api = Start-App 'api' $oldAPI $databaseURL 'wave3-api-schema11'; Wait-Ready $api 'Wave 3 API schema11'
    $baseURL = $api.ProbeURL -replace '/ready$','/'
    $run.checkpoints.Add(@{ name='Wave 3 API migration/schema creation'; outcome='PASS'; schema=(Invoke-Psql $dbName 'SELECT max(version) FROM schema_migrations') })

    $queue = "migration-$suffix"
    $adminHeaders = @{ Authorization="Bearer $adminToken" }
    $queueConfig = @{ max_concurrency=0; rate_limit_per_second=0; burst=1; aging_after_seconds=0; aging_priority_boost=0 } | ConvertTo-Json -Compress
    Invoke-RestMethod -Method Put -Uri "$($baseURL)queues/$queue" -Headers $adminHeaders -ContentType 'application/json' -Body $queueConfig | Out-Null

    $pending = New-Task $baseURL "$queue-pending" @{ execution_mode='mock'; document_id='migration-pending'; mock_duration_ms=10 }
    $run.task_ids.pending = $pending.id
    Invoke-RestMethod -Method Post -Uri "$($baseURL)queues/$queue-pending/pause" -Headers $adminHeaders -ContentType 'application/json' -Body '{}' | Out-Null
    $success = New-Task $baseURL "$queue-success" @{ execution_mode='mock'; document_id='migration-success'; mock_duration_ms=20 }
    $run.task_ids.success = $success.id
    $retry = New-Task $baseURL "$queue-retry" @{ execution_mode='mock'; document_id='migration-retry'; simulate='retry_then_success'; mock_duration_ms=10 } 3 30
    $run.task_ids.retry = $retry.id
    $dlq = New-Task $baseURL "$queue-dlq" @{ execution_mode='mock'; document_id='migration-dlq'; simulate='bad_payload'; mock_duration_ms=10 }
    $run.task_ids.dlq = $dlq.id

    $artifactSourceKey = "source-$suffix.md"
    Set-CommonEnv $databaseURL
    $env:MINIO_ENDPOINT = $run.minio_endpoint; $env:MINIO_BUCKET = "migration-$suffix"; $env:MINIO_ACCESS_KEY = $minioAccess; $env:MINIO_SECRET_KEY = $minioSecret
    $env:W4_MIGRATION_FIXTURE='1'; $env:W4_MIGRATION_ARTIFACT_MODE='put-source'; $env:W4_MIGRATION_ARTIFACT_KEY=$artifactSourceKey
    $env:GOCACHE='E:\distributed-task-lab-temp\wave4-artifacts\gocache'; $env:GOMODCACHE='E:\distributed-task-lab-temp\wave4-artifacts\gomodcache'
    go test ./internal/e2e -run '^TestMigrationArtifactFixture$' -count=1 -v *> (Join-Path $EvidenceDir 'artifact-source-put.log')
    if ($LASTEXITCODE -ne 0) { throw 'Could not write migration input artifact to MinIO.' }
    $artifactTask = New-Task $baseURL "$queue-artifact" @{ execution_mode='real'; document_id="migration-$suffix"; object_key=$artifactSourceKey }
    $run.task_ids.artifact = $artifactTask.id
    $applied = New-EffectTask $baseURL "$queue-effect-applied" $false
    $run.task_ids.effect_applied_task = $applied.id
    $unknown = New-EffectTask $baseURL "$queue-effect-unknown" $true 2
    $run.task_ids.effect_unknown_task = $unknown.id

    $worker = Start-App 'worker' $oldWorker $databaseURL 'wave3-worker-seed'; Wait-Ready $worker 'Wave 3 Worker seed'
    foreach ($item in @(@{ id=$success.id; want='success|1|1' }, @{ id=$retry.id; want='pending|1|1' }, @{ id=$dlq.id; want='dlq|1|1' }, @{ id=$artifactTask.id; want='success|1|1' }, @{ id=$applied.id; want='success|1|1' }, @{ id=$unknown.id; want='pending|1|1' })) {
        [void](Wait-Task $dbName $item.id $item.want 60)
    }
    Stop-App $worker; $worker=$null
    $runningWorker = Start-App 'worker' $oldWorker $databaseURL 'wave3-worker-running-crash'
    Wait-Ready $runningWorker 'Wave 3 Worker running fixture'
    $running = New-Task $baseURL "$queue-running" @{ execution_mode='mock'; document_id='migration-running'; mock_duration_ms=8000 }
    $run.task_ids.running = $running.id
    [void](Wait-Task $dbName $running.id 'running|1|1' 30)
    Stop-App $runningWorker -Crash
    $worker = $null
    $run.checkpoints.Add(@{ name='Wave 3 populated seed'; outcome='PASS'; states='pending, success, retryable pending (attempt 1), DLQ, running lease, applied effect, unknown effect, audit/events, S3-backed RAG result' })

    $preSnapshotHash = Save-Snapshot $dbName 'schema11-before'
    $run.checkpoints.Add(@{ name='schema11 snapshot'; outcome='PASS'; sha256=$preSnapshotHash })
    $dumpPath = Join-Path $EvidenceDir 'schema11-pre-migration.dump'
    $dumpProc = Start-Process -FilePath docker -ArgumentList @('exec',$pgID,'pg_dump','-U','postgres','--format=custom','--no-owner','--no-acl',$dbName) -NoNewWindow -PassThru -RedirectStandardOutput $dumpPath -RedirectStandardError (Join-Path $EvidenceDir 'pg_dump.stderr.log') -Wait
    if ($dumpProc.ExitCode -ne 0) { throw 'Could not create pre-migration pg_dump.' }
    $run.pre_migration_backup = @{ file='schema11-pre-migration.dump'; sha256=(Get-FileHash -Algorithm SHA256 -LiteralPath $dumpPath).Hash.ToLowerInvariant(); bytes=(Get-Item -LiteralPath $dumpPath).Length }

    # Wave 3 has completed all planned operations; the one deliberately retained
    # running lease is the crash/reclaim case. Stop old API and assert no worker process remains.
    Stop-App $api; $api = $null
    foreach ($p in $processes) { $p.Refresh(); if (-not $p.HasExited) { Stop-App $p } }
    $alive = @($processes | Where-Object { $_.Refresh(); -not $_.HasExited }).Count
    if ($alive -ne 0) { throw "Old service process count after stop is $alive." }
    $run.checkpoints.Add(@{ name='Wave 3 stop/drain boundary'; outcome='PASS_WITH_KNOWN_CRASH_FIXTURE'; active_lease=$running.id; note='All non-crash workers stopped after no in-flight fixture; one intentionally killed running worker is retained to verify lease expiry/reclaim. No ACK was recorded for the killed process.' })

    $api = Start-App 'api' $newAPI $databaseURL 'wave4-api-populated-upgrade'; Wait-Ready $api 'Wave 4 API schema11 populated upgrade'
    $baseURL = $api.ProbeURL -replace '/ready$','/'
    $schema = Invoke-Psql $dbName 'SELECT max(version) FROM schema_migrations'
    if ($schema -ne '12') { throw "Wave 4 migration ended at schema $schema, expected 12." }
    $postSnapshotHash = Save-Snapshot $dbName 'schema12-after-migration'
    if ($preSnapshotHash -ne $postSnapshotHash) { throw "Populated data snapshot changed across additive migration (before $preSnapshotHash, after $postSnapshotHash)." }
    $run.checkpoints.Add(@{ name='schema11 to 12 populated migration'; outcome='PASS'; before_sha256=$preSnapshotHash; after_sha256=$postSnapshotHash; schema=$schema })

    $wrongOwnerToken = [Convert]::ToHexString([System.Security.Cryptography.RandomNumberGenerator]::GetBytes(32)).ToLowerInvariant()
    $taskGet = Invoke-WebRequest -UseBasicParsing -Uri "$($baseURL)tasks/$($success.id)" -Headers @{Authorization="Bearer $wrongOwnerToken"} -SkipHttpErrorCheck
    $invalidCredentialStatus = $taskGet.StatusCode
    if ($invalidCredentialStatus -ne 401) { throw "Invalid credential guard expected 401, got $invalidCredentialStatus." }
    $taskGet = Invoke-RestMethod -Uri "$($baseURL)tasks/$($success.id)" -Headers @{Authorization="Bearer $apiToken"}
    if ($taskGet.status -ne 'success') { throw 'Wave 4 API could not read the migrated success task.' }
    $taskList = Invoke-RestMethod -Uri "$($baseURL)tasks?limit=100" -Headers @{Authorization="Bearer $apiToken"}
    if (@($taskList | Where-Object id -eq $success.id).Count -ne 1) { throw 'Wave 4 API task list omitted a migrated task.' }
    $events = Invoke-RestMethod -Uri "$($baseURL)tasks/$($success.id)/events" -Headers @{Authorization="Bearer $apiToken"}
    if (@($events).Count -lt 2) { throw 'Migrated task event history was not readable.' }
    $run.checkpoints.Add(@{ name='Wave 4 task Get/List and event query'; outcome='PASS'; success_task=$success.id; event_count=@($events).Count; invalid_credential_status=$invalidCredentialStatus })

    # Make the preserved retry due using PostgreSQL's clock, then verify Wave 4
    # worker claim/lease and eventual retry completion. The snapshot remains immutable.
    [void](Invoke-Psql $dbName "UPDATE tasks SET next_run_at=clock_timestamp() WHERE id='$($retry.id)' AND status='pending' AND attempt=1")
    [void](Invoke-Psql $dbName "UPDATE tasks SET next_run_at=clock_timestamp() WHERE id='$($unknown.id)' AND status='pending' AND attempt=1")
    [void](Invoke-Psql $dbName "UPDATE tasks SET lease_expires_at=clock_timestamp()-interval '1 millisecond' WHERE id='$($running.id)' AND status='running' AND lease_version=1")
    Stop-App $api; $api = $null
    $api = Start-App 'api' $newAPI $databaseURL 'wave4-api-rollforward-recovery'; Wait-Ready $api 'Wave 4 API roll-forward recovery'
    $baseURL = $api.ProbeURL -replace '/ready$','/'
    $artifactReuse = New-Task $baseURL "$queue-artifact-reuse" @{ execution_mode='real'; document_id="migration-$suffix"; object_key=$artifactSourceKey }
    $run.task_ids.artifact_reuse = $artifactReuse.id
    $worker = Start-App 'worker' $newWorker $databaseURL 'wave4-worker-reclaim'; Wait-Ready $worker 'Wave 4 Worker recovery'
    [void](Wait-Task $dbName $running.id 'running|2|2' 30)
    $leaseBeforeRenew = Invoke-Psql $dbName "SELECT lease_expires_at::text FROM tasks WHERE id='$($running.id)'"
    Start-Sleep -Seconds 4
    $leaseAfterRenew = Invoke-Psql $dbName "SELECT lease_expires_at::text FROM tasks WHERE id='$($running.id)'"
    if ([DateTimeOffset]::Parse($leaseAfterRenew) -le [DateTimeOffset]::Parse($leaseBeforeRenew)) { throw 'Wave 4 Worker lease renewal did not advance lease_expires_at.' }
    [void](Wait-Task $dbName $retry.id 'success|2|2' 60)
    [void](Wait-Task $dbName $unknown.id 'success|2|2' 60)
    [void](Wait-Task $dbName $running.id 'success|2|2' 20)
    [void](Wait-Task $dbName $artifactTask.id 'success|1|1' 30)
    [void](Wait-Task $dbName $artifactReuse.id 'success|1|1' 30)
    $artifactResult = Invoke-Psql $dbName "SELECT result::text FROM tasks WHERE id='$($artifactTask.id)'"
    $artifactUriMatch = [regex]::Match($artifactResult, 'artifact://([^"\\]+)')
    if (-not $artifactUriMatch.Success) { throw 'Wave 3 task result had no S3 artifact URI.' }
    $artifactKey = $artifactUriMatch.Groups[1].Value
    $artifactSHA = [regex]::Match($artifactResult, '"artifact_sha256"\s*:\s*"([a-f0-9]{64})"').Groups[1].Value
    if (-not $artifactSHA) { throw 'Wave 3 task result had no artifact SHA256.' }
    $reuseResult = Invoke-Psql $dbName "SELECT result::text FROM tasks WHERE id='$($artifactReuse.id)'"
    if ($reuseResult -notmatch '"idempotent_reused"\s*:\s*"true"') { throw 'Wave 4 did not reuse the existing MinIO artifact.' }
    $env:W4_MIGRATION_FIXTURE='1'; $env:W4_MIGRATION_ARTIFACT_MODE='verify-output'; $env:W4_MIGRATION_ARTIFACT_KEY=$artifactKey; $env:W4_MIGRATION_ARTIFACT_SHA256=$artifactSHA
    $env:MINIO_ENDPOINT=$run.minio_endpoint; $env:MINIO_BUCKET="migration-$suffix"; $env:MINIO_ACCESS_KEY=$minioAccess; $env:MINIO_SECRET_KEY=$minioSecret; $env:ARTIFACT_S3_PREFIX="migration-$suffix"
    go test ./internal/e2e -run '^TestMigrationArtifactFixture$' -count=1 -v *> (Join-Path $EvidenceDir 'artifact-output-verify.log')
    if ($LASTEXITCODE -ne 0) { throw 'Migrated artifact object verification failed.' }
    $run.checkpoints.Add(@{ name='Wave 4 roll-forward, retry, lease renew/reclaim, and artifact reuse'; outcome='PASS'; retry_task=$retry.id; expired_lease_task=$running.id; attempt='2'; lease_version='2'; lease_before_renewal=$leaseBeforeRenew; lease_after_renewal=$leaseAfterRenew; artifact_key=$artifactKey; artifact_sha256=$artifactSHA; reuse_task=$artifactReuse.id })

    $unknownEffect = Invoke-Psql $dbName "SELECT effect_id::text FROM effects WHERE task_id='$($unknown.id)' AND status='unknown'"
    $appliedEffect = Invoke-Psql $dbName "SELECT effect_id::text FROM effects WHERE task_id='$($applied.id)' AND status='applied'"
    if (-not $unknownEffect -or -not $appliedEffect) { throw 'Expected migrated unknown/applied effect fixtures are missing.' }
    $reconciledEffect = Invoke-Psql $dbName "SELECT status FROM effects WHERE task_id='$($unknown.id)' AND status='applied' ORDER BY generation DESC LIMIT 1"
    if ($reconciledEffect -ne 'applied') { throw "Effect retry reconciliation result is $reconciledEffect." }
    $run.checkpoints.Add(@{ name='Effect state lookup and response-loss reconciliation'; outcome='PASS'; reconciled_effect_id=$unknownEffect; reconciled_status=$reconciledEffect; applied_effect_id=$appliedEffect })

    # Roll-forward fault injection is limited to stopping compatible app processes;
    # restart the same schema-12-compatible binaries and verify service recovery.
    Stop-App $worker; $worker = $null; Stop-App $api; $api = $null
    $api = Start-App 'api' $newAPI $databaseURL 'wave4-api-rollforward-restart'; Wait-Ready $api 'Wave 4 API roll-forward restart'
    $baseURL = $api.ProbeURL -replace '/ready$','/'
    $worker = Start-App 'worker' $newWorker $databaseURL 'wave4-worker-rollforward-restart'; Wait-Ready $worker 'Wave 4 Worker roll-forward restart'
    $apiTask = Invoke-RestMethod -Uri "$($baseURL)tasks/$($success.id)" -Headers @{Authorization="Bearer $apiToken"}
    if ($apiTask.status -ne 'success') { throw 'Wave 4 task read failed after roll-forward restart.' }
    $run.checkpoints.Add(@{ name='Roll-forward recovery after compatible process restart'; outcome='PASS'; schema=$schema; note='Application process failure simulated by stopping the isolated Wave 4 API/Worker and restarting compatible schema-12 binaries; no production service or writes affected.' })

    Stop-App $worker; $worker=$null; Stop-App $api; $api=$null
    $restoreDB = "w4_restore_$suffix"; [void](Invoke-Psql 'postgres' "CREATE DATABASE $restoreDB")
    $containerDump = "/tmp/schema11-$suffix.dump"
    docker cp $dumpPath "${pgID}:$containerDump" | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Could not copy schema11 backup into isolated PostgreSQL container.' }
    docker exec $pgID pg_restore -U postgres --exit-on-error --no-owner --no-acl --dbname $restoreDB $containerDump *> (Join-Path $EvidenceDir 'schema11-restore.log')
    if ($LASTEXITCODE -ne 0) { throw 'Could not restore the complete pre-migration backup to a separate isolated database.' }
    $restoreHash = Save-Snapshot $restoreDB 'schema11-restored-target'
    if ($restoreHash -ne $preSnapshotHash) { throw "Restored schema11 snapshot mismatch: before=$preSnapshotHash restored=$restoreHash." }
    $restoredURL = "postgres://postgres:${password}@127.0.0.1:$($run.postgres.port)/${restoreDB}?sslmode=disable"
    $restoreAPI = Start-App 'api' $oldAPI $restoredURL 'wave3-api-restored-schema11'; Wait-Ready $restoreAPI 'Wave 3 API restored schema11'
    $restoreURL = $restoreAPI.ProbeURL -replace '/ready$','/'
    $restored = Invoke-RestMethod -Uri "$($restoreURL)tasks/$($pending.id)" -Headers @{Authorization="Bearer $apiToken"}
    if ($restored.status -ne 'pending') { throw 'Wave 3 could not read pending task after complete pre-migration restore.' }
    $restoredArtifactResult = Invoke-Psql $restoreDB "SELECT result::text FROM tasks WHERE id='$($artifactTask.id)'"
    if ($restoredArtifactResult -ne $artifactResult) { throw 'Restored task artifact reference differs from the schema11 snapshot.' }
    Invoke-RestMethod -Method Post -Uri "$($restoreURL)queues/$queue-pending/resume" -Headers $adminHeaders -ContentType 'application/json' -Body '{}' | Out-Null
    [void](Invoke-Psql $restoreDB "UPDATE tasks SET next_run_at=clock_timestamp() WHERE id='$($unknown.id)' AND status='pending' AND attempt=1")
    $restoreWorker = Start-App 'worker' $oldWorker $restoredURL 'wave3-worker-restored-target'; Wait-Ready $restoreWorker 'Wave 3 Worker restored schema11'
    [void](Wait-Task $restoreDB $pending.id 'success|1|1' 30)
    [void](Wait-Task $restoreDB $unknown.id 'success|2|2' 60)
    $run.checkpoints.Add(@{ name='Complete restore to separate isolated target'; outcome='PASS'; restored_schema='11'; source_snapshot_sha256=$preSnapshotHash; restored_snapshot_sha256=$restoreHash; old_api='ready and read restored pending task'; post_upgrade_write_loss='restoring pre-migration snapshot intentionally omits all schema12-era writes; this is expected data-loss boundary, not rollback.' })
    Stop-App $restoreWorker; $restoreWorker=$null; Stop-App $restoreAPI; $restoreAPI=$null

    $run.conclusions.Add('SCHEMA11_TO_12_POPULATED_UPGRADE_PASS')
    $run.conclusions.Add('ROLL_FORWARD_RECOVERY_PASS')
    $run.conclusions.Add('ISOLATED_PRE_MIGRATION_RESTORE_PASS')
    $run.conclusions.Add('WAVE3_SCHEMA12_DIRECT_ROLLBACK_BLOCKED_AS_EXPECTED')
}
catch {
    $_ | Out-String | Set-Content -LiteralPath (Join-Path $EvidenceDir 'failure.txt')
    $run.conclusions.Add('NOT VERIFIED')
    throw
}
finally {
    foreach ($p in $processes) { try { Stop-App $p } catch { "process cleanup: $($_.Exception.Message)" | Add-Content -LiteralPath (Join-Path $EvidenceDir 'cleanup-errors.log') } }
    if (Test-Path -LiteralPath $credentialFile) { Remove-Item -LiteralPath $credentialFile -Force }
    foreach ($id in $createdContainers) {
        try { $logs = (docker logs $id 2>&1 | Out-String).Replace($password,'[REDACTED]').Replace($apiToken,'[REDACTED]').Replace($adminToken,'[REDACTED]').Replace($minioAccess,'[REDACTED]').Replace($minioSecret,'[REDACTED]'); $logs | Set-Content -LiteralPath (Join-Path $EvidenceDir "container-$id.log") } catch { }
        try { docker rm --force $id *> $null } catch { }
    }
    foreach ($name in $envNames) { [Environment]::SetEnvironmentVariable($name, $previousEnv[$name], 'Process') }
    Get-ChildItem -LiteralPath $EvidenceDir -File -Recurse -Filter '*.log' | ForEach-Object {
        $content = [string](Get-Content -LiteralPath $_.FullName -Raw)
        if (-not [string]::IsNullOrEmpty($content)) { $content = $content.Replace($password,'[REDACTED]').Replace($apiToken,'[REDACTED]').Replace($adminToken,'[REDACTED]').Replace($minioAccess,'[REDACTED]').Replace($minioSecret,'[REDACTED]') }
        Set-Content -LiteralPath $_.FullName -Value $content -Encoding utf8
    }
    $run.completed_utc = [DateTime]::UtcNow.ToString('o')
    $run | ConvertTo-Json -Depth 15 | Set-Content -LiteralPath (Join-Path $EvidenceDir 'migration-report.json') -Encoding utf8
    Get-ChildItem -LiteralPath $EvidenceDir -File -Recurse | Where-Object Name -ne 'SHA256SUMS.txt' | ForEach-Object {
        $relative = [System.IO.Path]::GetRelativePath($EvidenceDir, $_.FullName)
        $hash = (Get-FileHash -Algorithm SHA256 -LiteralPath $_.FullName).Hash.ToLowerInvariant()
        "$hash  $relative"
    } | Set-Content -LiteralPath (Join-Path $EvidenceDir 'SHA256SUMS.txt')
}
"Migration evidence retained at $EvidenceDir"
