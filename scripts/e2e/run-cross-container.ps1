[CmdletBinding()]
param([switch]$Tracing)

$ErrorActionPreference = "Stop"
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
$composeFile = Join-Path $repoRoot "deployments\e2e\compose.yaml"
$project = "tasklab-e2e-" + [guid]::NewGuid().ToString("N").Substring(0, 12)
$runID = $project
$minioImage = "tasklab-minio-ci:$runID-1"
$createdImage = $false
$composeStarted = $false
$composeAttempted = $false
$testExit = $null
$temporaryRoot = if ($Tracing) { 'E:\distributed-task-lab-temp\wave4-artifacts\tracing-e2e-runs' } else { 'E:\distributed-task-lab-temp' }
New-Item -ItemType Directory -Path $temporaryRoot -Force | Out-Null
$temporaryDirectory = Join-Path $temporaryRoot $project
New-Item -ItemType Directory -Path $temporaryDirectory | Out-Null
$composeLog = Join-Path $temporaryDirectory "compose.log"
$testLog = Join-Path $temporaryDirectory "test.log"
$environmentNames = @(
    "GITHUB_RUN_ID", "GITHUB_RUN_ATTEMPT", "GITHUB_WORKSPACE", "GITHUB_ENV",
    "RUNNER_TEMP", "TMPDIR",
    "MINIO_CI_IMAGE", "E2E_POSTGRES_PASSWORD", "E2E_POSTGRES_PORT", "E2E_MINIO_PORT",
    "E2E_API_PORT", "E2E_S3_ACCESS_KEY", "E2E_S3_SECRET_KEY", "E2E_API_TOKEN",
    "E2E_ADMIN_TOKEN", "E2E_ARTIFACT_PREFIX", "E2E_COMPOSE_PROJECT", "E2E_COMPOSE_FILE",
    "E2E_PROXY_ADMIN_PORT", "E2E_S3_PROXY_TOKEN", "E2E_S3_PROXY_ADMIN_URL",
    "REQUIRE_CONTAINER_E2E", "E2E_API_URL", "E2E_DATABASE_URL", "E2E_MINIO_ENDPOINT",
    "E2E_MINIO_BUCKET", "REQUIRE_MINIO", "MINIO_ENDPOINT", "MINIO_BUCKET", "MINIO_ACCESS_KEY",
    "MINIO_SECRET_KEY", "E2E_COMPOSE_FILE", "E2E_TRACING_ENABLED", "E2E_OTEL_HEALTH_PORT",
    "E2E_DISTRIBUTED_TRACE_E2E", "GOTOOLCHAIN"
)
$previous = @{}
foreach ($name in $environmentNames) { $previous[$name] = [Environment]::GetEnvironmentVariable($name, "Process") }

function Get-FreeLoopbackPort {
    $listener = [Net.Sockets.TcpListener]::new([Net.IPAddress]::Loopback, 0)
    $listener.Start()
    try { return $listener.LocalEndpoint.Port }
    finally { $listener.Stop() }
}

function Invoke-Compose([string[]]$Arguments) {
	$savedErrorActionPreference = $ErrorActionPreference
	try {
		# Docker Compose writes normal lifecycle progress to stderr. Capture it
		# without treating it as a PowerShell exception; the process exit code is
	# the authoritative result.
		$ErrorActionPreference = "Continue"
		$output = & docker compose --project-name $project --file $composeFile @Arguments 2>&1
		$code = $LASTEXITCODE
	} finally {
		$ErrorActionPreference = $savedErrorActionPreference
	}
    $output | Out-File -LiteralPath $composeLog -Append -Encoding utf8
    if ($code -ne 0) { throw "Docker Compose command failed (exit=$code): docker compose $($Arguments -join ' ')" }
}

try {
    # Honor the module's Go version even when the caller has GOTOOLCHAIN=local
    # configured with an older installed toolchain.
    [Environment]::SetEnvironmentVariable("GOTOOLCHAIN", "auto", "Process")
    $gitPath = (Get-Command git -ErrorAction Stop).Source
    $gitRoot = Split-Path (Split-Path $gitPath -Parent) -Parent
    $gitBash = Join-Path $gitRoot "bin\bash.exe"
    if (-not (Test-Path -LiteralPath $gitBash)) { throw "Git Bash is required to build the pinned MinIO test image." }
    $gitTempRoot = (& $gitBash -lc "cygpath -u '$temporaryRoot'").Trim()
    if (-not $gitTempRoot.StartsWith("/")) { throw "Could not map the E: temporary directory into Git Bash." }
    [Environment]::SetEnvironmentVariable("RUNNER_TEMP", $gitTempRoot, "Process")
    [Environment]::SetEnvironmentVariable("TMPDIR", $gitTempRoot, "Process")

    [Environment]::SetEnvironmentVariable("GITHUB_RUN_ID", $runID, "Process")
    [Environment]::SetEnvironmentVariable("GITHUB_RUN_ATTEMPT", "1", "Process")
    [Environment]::SetEnvironmentVariable("GITHUB_WORKSPACE", $repoRoot, "Process")
    [Environment]::SetEnvironmentVariable("GITHUB_ENV", $null, "Process")
    [Environment]::SetEnvironmentVariable("MINIO_CI_IMAGE", $minioImage, "Process")
    Push-Location $repoRoot
    try {
        $savedErrorActionPreference = $ErrorActionPreference
        try {
            # BuildKit emits progress on stderr even on success. Capture both
            # streams, then trust only the shell process exit code.
            $ErrorActionPreference = "Continue"
            & $gitBash (Join-Path $repoRoot "scripts\evidence\build-minio-ci-image.sh") *> $composeLog
            $minioBuildExit = $LASTEXITCODE
        } finally {
            $ErrorActionPreference = $savedErrorActionPreference
        }
        if ($minioBuildExit -ne 0) { throw "Pinned MinIO source build failed (exit=$minioBuildExit); see E: temporary diagnostics." }
    } finally { Pop-Location }
    $imageID = & docker image inspect $minioImage --format "{{.Id}}"
    if ($LASTEXITCODE -ne 0 -or -not $imageID) { throw "Pinned MinIO source image was not created." }
    $createdImage = $true

    $postgresPort = Get-FreeLoopbackPort
    $minioPort = Get-FreeLoopbackPort
    $apiPort = Get-FreeLoopbackPort
    $proxyAdminPort = Get-FreeLoopbackPort
    $otelHealthPort = Get-FreeLoopbackPort
    $secret = [guid]::NewGuid().ToString("N") + [guid]::NewGuid().ToString("N")
    $values = @{
        E2E_POSTGRES_PASSWORD = $secret
        E2E_POSTGRES_PORT = [string]$postgresPort
        E2E_MINIO_PORT = [string]$minioPort
        E2E_API_PORT = [string]$apiPort
        E2E_PROXY_ADMIN_PORT = [string]$proxyAdminPort
        E2E_S3_ACCESS_KEY = "e2e" + [guid]::NewGuid().ToString("N")
        E2E_S3_SECRET_KEY = [guid]::NewGuid().ToString("N") + [guid]::NewGuid().ToString("N")
        E2E_API_TOKEN = [guid]::NewGuid().ToString("N") + [guid]::NewGuid().ToString("N")
        E2E_ADMIN_TOKEN = [guid]::NewGuid().ToString("N") + [guid]::NewGuid().ToString("N")
        E2E_S3_PROXY_TOKEN = [guid]::NewGuid().ToString("N") + [guid]::NewGuid().ToString("N")
        E2E_OTEL_HEALTH_PORT = [string]$otelHealthPort
        E2E_TRACING_ENABLED = [string]([bool]$Tracing).ToString().ToLowerInvariant()
        E2E_ARTIFACT_PREFIX = "container-e2e/" + $project
        E2E_COMPOSE_PROJECT = $project
        E2E_COMPOSE_FILE = $composeFile
    }
    foreach ($entry in $values.GetEnumerator()) { [Environment]::SetEnvironmentVariable($entry.Key, [string]$entry.Value, "Process") }

    $composeAttempted = $true
    Invoke-Compose @("up", "--detach", "--wait", "--wait-timeout", "120", "postgres", "minio")
    $composeStarted = $true
    $minioHealthy = $false
    for ($attempt = 0; $attempt -lt 90; $attempt++) {
        # This is a loopback probe, not an internet request. Explicitly bypass
        # the caller's HTTP proxy so local health checks cannot be sent to it.
        $healthStatus = & curl.exe --noproxy "*" --fail --silent --show-error --max-time 2 --output NUL --write-out "%{http_code}" "http://127.0.0.1:$minioPort/minio/health/live" 2>$null
        $healthExit = $LASTEXITCODE
        if ($healthExit -eq 0 -and $healthStatus -eq "200") { $minioHealthy = $true; break }
        Start-Sleep -Seconds 1
    }
    if (-not $minioHealthy) { throw "MinIO HTTP health endpoint did not return 200 within 90 seconds." }
    if ($Tracing) {
        Invoke-Compose @("up", "--detach", "--wait", "--wait-timeout", "60", "otel-collector")
        $collectorHealthy = $false
        for ($attempt = 0; $attempt -lt 60; $attempt++) {
            $healthStatus = & curl.exe --noproxy "*" --fail --silent --show-error --max-time 2 --output NUL --write-out "%{http_code}" "http://127.0.0.1:$otelHealthPort/" 2>$null
            $healthExit = $LASTEXITCODE
            if ($healthExit -eq 0 -and $healthStatus -eq "200") { $collectorHealthy = $true; break }
            Start-Sleep -Seconds 1
        }
        if (-not $collectorHealthy) { throw "OTLP Collector health endpoint did not return 200 within 60 seconds." }
    }
    Invoke-Compose @("up", "--detach", "--build", "--wait", "--wait-timeout", "240", "api", "worker-a")
    Invoke-Compose @("up", "--detach", "--build", "--wait", "--wait-timeout", "120", "worker-b")

    [Environment]::SetEnvironmentVariable("REQUIRE_CONTAINER_E2E", "1", "Process")
    [Environment]::SetEnvironmentVariable("E2E_API_URL", "http://127.0.0.1:$apiPort", "Process")
    [Environment]::SetEnvironmentVariable("E2E_DATABASE_URL", "postgres://postgres:$secret@127.0.0.1:$postgresPort/tasklab?sslmode=disable", "Process")
    [Environment]::SetEnvironmentVariable("E2E_MINIO_ENDPOINT", "http://127.0.0.1:$minioPort", "Process")
    [Environment]::SetEnvironmentVariable("E2E_S3_PROXY_ADMIN_URL", "http://127.0.0.1:$proxyAdminPort", "Process")
    [Environment]::SetEnvironmentVariable("E2E_MINIO_BUCKET", "tasklab-e2e", "Process")
    [Environment]::SetEnvironmentVariable("REQUIRE_MINIO", "1", "Process")
    [Environment]::SetEnvironmentVariable("MINIO_ENDPOINT", "http://127.0.0.1:$minioPort", "Process")
    [Environment]::SetEnvironmentVariable("MINIO_BUCKET", "tasklab-e2e", "Process")
    [Environment]::SetEnvironmentVariable("MINIO_ACCESS_KEY", $values.E2E_S3_ACCESS_KEY, "Process")
    [Environment]::SetEnvironmentVariable("MINIO_SECRET_KEY", $values.E2E_S3_SECRET_KEY, "Process")
    $dirtyAtTest = [bool](& git status --porcelain)

    Push-Location $repoRoot
    try {
        if ($Tracing) {
            [Environment]::SetEnvironmentVariable("E2E_DISTRIBUTED_TRACE_E2E", "1", "Process")
            & go test -race ./internal/e2e -run '^TestDistributedTracingContainersExportToCollector$' -count=1 -v *> $testLog
            $traceExit = $LASTEXITCODE
            [Environment]::SetEnvironmentVariable("REQUIRE_CONTAINER_E2E", "1", "Process")
            & go test -race ./internal/e2e -run '^TestAPIWorkerContainersRecoverSharedArtifact$' -count=1 -v *> (Join-Path $temporaryDirectory "container-recovery.log")
            $containerExit = $LASTEXITCODE
            $testExit = if ($traceExit -eq 0 -and $containerExit -eq 0) { 0 } else { 1 }
        } else {
            & go test -race ./internal/artifacts -run '^TestMinIOStoreContractAndConcurrentConditionalWrite$' -count=1 -v *> $testLog
            $artifactExit = $LASTEXITCODE
            & go test -race ./internal/e2e -run '^TestAPIWorkerContainersRecoverSharedArtifact$' -count=1 -v *> (Join-Path $temporaryDirectory "container-recovery.log")
            $containerExit = $LASTEXITCODE
            & go test -race ./internal/e2e -run '^TestAPIWorkerContainerRecoversLostPutResponse$' -count=1 -v *> (Join-Path $temporaryDirectory "lost-put-response.log")
            $faultExit = $LASTEXITCODE
            & go test -race ./internal/e2e -run '^TestTwoWorkerContainersRaceForSameArtifact$' -count=1 -v *> (Join-Path $temporaryDirectory "same-artifact-race.log")
            $sameCaseExit = $LASTEXITCODE
            $testExit = if ($artifactExit -eq 0 -and $containerExit -eq 0 -and $faultExit -eq 0 -and $sameCaseExit -eq 0) { 0 } else { 1 }
        }
    } finally { Pop-Location }
    Get-Content -LiteralPath $testLog
    if (-not $Tracing) {
        Get-Content -LiteralPath (Join-Path $temporaryDirectory "container-recovery.log")
        Get-Content -LiteralPath (Join-Path $temporaryDirectory "lost-put-response.log")
        Get-Content -LiteralPath (Join-Path $temporaryDirectory "same-artifact-race.log")
    } else {
        Get-Content -LiteralPath (Join-Path $temporaryDirectory "container-recovery.log")
    }
    if ($testExit -ne 0) { throw "Cross-container E2E failed with exit code $testExit." }
    if ($Tracing) {
        $traceLine = Select-String -LiteralPath $testLog -Pattern 'TRACE_E2E_TASK_ID=(\S+) TRACE_E2E_TRACE_ID=(\S+) TRACE_E2E_API_SPAN_ID=(\S+) TRACE_E2E_SENSITIVE_MARKER=(\S+)' | Select-Object -Last 1
        if (-not $traceLine) { throw "Trace E2E did not emit correlation identifiers." }
        $taskID, $traceID, $apiSpanID, $sensitiveMarker = $traceLine.Matches[0].Groups[1..4].Value
        $recoveryLine = Select-String -LiteralPath (Join-Path $temporaryDirectory "container-recovery.log") -Pattern 'PASS: task_id=(\S+);' | Select-Object -Last 1
        if (-not $recoveryLine) { throw "Cross-container crash/reclaim test did not emit a task correlation identifier." }
        $recoveryTaskID = $recoveryLine.Matches[0].Groups[1].Value
        $requiredTraceEvidence = @($taskID, $traceID, "Name           : POST", "url.path: Str(/v1/tasks)", "task.enqueue", "claim.transaction", "task.execute rag_ingestion", "artifact.head", "artifact.put", "task.mark_success", "task.id: Str($recoveryTaskID)", "task.attempt: Int(2)", "task.claim_reason: Str(expired_lease)")
        $collectorText = ""
        $traceDeadline = [DateTime]::UtcNow.AddSeconds(20)
        do {
            $collectorLogs = & docker compose --project-name $project --file $composeFile logs --no-color --timestamps otel-collector 2>&1
            $collectorText = ($collectorLogs -join "`n")
            $collectorText | Set-Content -LiteralPath (Join-Path $temporaryDirectory "collector.log") -Encoding utf8
            $missing = @($requiredTraceEvidence | Where-Object { -not $collectorText.Contains($_) })
            if ($missing.Count -eq 0) { break }
            Start-Sleep -Seconds 1
        } while ([DateTime]::UtcNow -lt $traceDeadline)
        if ($missing.Count -gt 0) {
            throw "Collector output did not contain required trace evidence before the bounded 20-second deadline: $($missing -join ', ')."
        }
        if (-not $collectorText.Contains($apiSpanID)) { throw "Collector did not include the persisted API span as a Worker link." }
        if ($collectorText.Contains($sensitiveMarker)) { throw "Sensitive payload marker appeared in Collector trace output." }
        foreach ($name in @("E2E_POSTGRES_PASSWORD", "E2E_S3_ACCESS_KEY", "E2E_S3_SECRET_KEY", "E2E_API_TOKEN", "E2E_ADMIN_TOKEN", "E2E_S3_PROXY_TOKEN")) {
            $value = [Environment]::GetEnvironmentVariable($name, "Process")
            if ($value -and $collectorText.Contains($value)) { throw "Collector trace output contains a configured credential value." }
        }
    }
    $successDirectory = if ($Tracing) { Join-Path $temporaryRoot ("evidence\" + $project) } else { Join-Path $repoRoot ("evidence\phase1\runs\wave3-container-e2e\" + $project) }
    New-Item -ItemType Directory -Path (Join-Path $successDirectory "logs") -Force | Out-Null
    if ($Tracing) {
        Copy-Item -LiteralPath $testLog -Destination (Join-Path $successDirectory "logs\distributed-tracing.log")
        Copy-Item -LiteralPath (Join-Path $temporaryDirectory "container-recovery.log") -Destination (Join-Path $successDirectory "logs\container-recovery.log")
        Copy-Item -LiteralPath (Join-Path $temporaryDirectory "collector.log") -Destination (Join-Path $successDirectory "logs\collector.log")
    } else {
        Copy-Item -LiteralPath $testLog -Destination (Join-Path $successDirectory "logs\minio-response-loss-contract.log")
        Copy-Item -LiteralPath (Join-Path $temporaryDirectory "container-recovery.log") -Destination (Join-Path $successDirectory "logs\container-recovery.log")
        Copy-Item -LiteralPath (Join-Path $temporaryDirectory "lost-put-response.log") -Destination (Join-Path $successDirectory "logs\container-put-response-loss.log")
        Copy-Item -LiteralPath (Join-Path $temporaryDirectory "same-artifact-race.log") -Destination (Join-Path $successDirectory "logs\two-worker-same-artifact-race.log")
    }
    $serviceLogs = & docker compose --project-name $project --file $composeFile logs --no-color --timestamps 2>&1
    $redactedServiceLogs = ($serviceLogs -join "`n")
    foreach ($name in @("E2E_POSTGRES_PASSWORD", "E2E_S3_ACCESS_KEY", "E2E_S3_SECRET_KEY", "E2E_API_TOKEN", "E2E_ADMIN_TOKEN", "E2E_S3_PROXY_TOKEN")) {
        $value = [Environment]::GetEnvironmentVariable($name, "Process")
        if ($value) { $redactedServiceLogs = $redactedServiceLogs.Replace($value, "[REDACTED]") }
    }
    $redactedServiceLogs | Set-Content -LiteralPath (Join-Path $successDirectory "logs\services.redacted.log") -Encoding utf8
    $implementationCommit = (& git rev-parse HEAD).Trim()
    $sourceTree = (& git rev-parse 'HEAD^{tree}').Trim()
    if (-not $implementationCommit -or -not $sourceTree) { throw "Could not resolve the exact tested Git source commit and tree." }
    $files = @()
    $evidencePaths = if ($Tracing) { @("logs/distributed-tracing.log", "logs/container-recovery.log", "logs/collector.log", "logs/services.redacted.log") } else { @("logs/minio-response-loss-contract.log", "logs/container-recovery.log", "logs/container-put-response-loss.log", "logs/two-worker-same-artifact-race.log", "logs/services.redacted.log") }
    foreach ($relativePath in $evidencePaths) {
        $fullPath = Join-Path $successDirectory ($relativePath -replace '/', '\')
        $hash = Get-FileHash -LiteralPath $fullPath -Algorithm SHA256
        $files += [ordered]@{ path = $relativePath; sha256 = $hash.Hash.ToLowerInvariant(); bytes = (Get-Item -LiteralPath $fullPath).Length }
    }
    $manifest = [ordered]@{
        run_id = $project
        result = "PASS"
        implementation_commit = $implementationCommit
        source_tree = $sourceTree
        worktree_dirty_at_test = $dirtyAtTest
        docker_compose_project = $project
        minio_source_commit = "9e49d5e7a648f00e26f2246f4dc28e6b07f8c84a"
        minio_ephemeral_local_image_id = $imageID
        postgres_image = "postgres:16-alpine"
        otel_collector_image = "otel/opentelemetry-collector-contrib@sha256:89107a3a8f4636a396927edf7025bb9614b8da2d92f4cc3f43109e8d115736e2"
        trace_e2e = if ($Tracing) { [ordered]@{ task_id = $taskID; trace_id = $traceID; api_span_id = $apiSpanID; collector_redaction_check = "PASS" } } else { $null }
        credentials_recorded = $false
        checks = if ($Tracing) { @(
            [ordered]@{ name = "distributed-tracing-collector-e2e"; exit_code = $traceExit; status = $(if ($traceExit -eq 0) { "PASS" } else { "FAIL" }) },
            [ordered]@{ name = "api-two-worker-crash-reclaim-with-tracing"; exit_code = $containerExit; status = $(if ($containerExit -eq 0) { "PASS" } else { "FAIL" }) }
        ) } else { @(
            [ordered]@{ name = "minio-response-loss-contract"; exit_code = $artifactExit; status = $(if ($artifactExit -eq 0) { "PASS" } else { "FAIL" }) },
            [ordered]@{ name = "api-two-worker-container-recovery"; exit_code = $containerExit; status = $(if ($containerExit -eq 0) { "PASS" } else { "FAIL" }) },
            [ordered]@{ name = "api-worker-put-response-loss-recovery"; exit_code = $faultExit; status = $(if ($faultExit -eq 0) { "PASS" } else { "FAIL" }) },
            [ordered]@{ name = "two-worker-same-artifact-race"; exit_code = $sameCaseExit; status = $(if ($sameCaseExit -eq 0) { "PASS" } else { "FAIL" }) }
        ) }
        files = $files
    }
    $manifest | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath (Join-Path $successDirectory "manifest.json") -Encoding utf8
    if ($Tracing) { Write-Output "Distributed tracing E2E passed; evidence=$successDirectory; Collector image is digest-pinned. Credentials were ephemeral and are not recorded." }
    else { Write-Output "Cross-container E2E passed; Compose project was $project; MinIO source image was $imageID. Credentials were ephemeral and are not recorded." }
} catch {
    Write-Output "Cross-container E2E did not pass: $($_.Exception.Message)"
    $evidenceDirectory = if ($Tracing) { Join-Path $temporaryRoot ("attempts\" + $project) } else { Join-Path $repoRoot ("evidence\phase1\runs\wave3-container-e2e-attempts\" + $project) }
    New-Item -ItemType Directory -Path (Join-Path $evidenceDirectory "logs") -Force | Out-Null
    if (Test-Path -LiteralPath $composeLog) { Copy-Item -LiteralPath $composeLog -Destination (Join-Path $evidenceDirectory "logs\compose.log") -Force }
    if (Test-Path -LiteralPath $testLog) { Copy-Item -LiteralPath $testLog -Destination (Join-Path $evidenceDirectory "logs\test.log") -Force }
    if (Test-Path -LiteralPath (Join-Path $temporaryDirectory "collector.log")) { Copy-Item -LiteralPath (Join-Path $temporaryDirectory "collector.log") -Destination (Join-Path $evidenceDirectory "logs\collector.log") -Force }
    if (Test-Path -LiteralPath (Join-Path $temporaryDirectory "container-recovery.log")) { Copy-Item -LiteralPath (Join-Path $temporaryDirectory "container-recovery.log") -Destination (Join-Path $evidenceDirectory "logs\container-recovery.log") -Force }
    if (Test-Path -LiteralPath (Join-Path $temporaryDirectory "lost-put-response.log")) { Copy-Item -LiteralPath (Join-Path $temporaryDirectory "lost-put-response.log") -Destination (Join-Path $evidenceDirectory "logs\container-put-response-loss.log") -Force }
    if (Test-Path -LiteralPath (Join-Path $temporaryDirectory "same-artifact-race.log")) { Copy-Item -LiteralPath (Join-Path $temporaryDirectory "same-artifact-race.log") -Destination (Join-Path $evidenceDirectory "logs\two-worker-same-artifact-race.log") -Force }
    $redacted = "No Compose service was started."
    if ($composeAttempted) {
        $diagnostics = & docker compose --project-name $project --file $composeFile logs --no-color --timestamps 2>&1
        $redacted = ($diagnostics -join "`n")
        foreach ($name in @("E2E_POSTGRES_PASSWORD", "E2E_S3_ACCESS_KEY", "E2E_S3_SECRET_KEY", "E2E_API_TOKEN", "E2E_ADMIN_TOKEN", "E2E_S3_PROXY_TOKEN")) {
            $value = [Environment]::GetEnvironmentVariable($name, "Process")
            if ($value) { $redacted = $redacted.Replace($value, "[REDACTED]") }
        }
        $redacted | Out-File -LiteralPath (Join-Path $temporaryDirectory "services.redacted.log") -Encoding utf8
        Copy-Item -LiteralPath (Join-Path $temporaryDirectory "services.redacted.log") -Destination (Join-Path $evidenceDirectory "logs\services.redacted.log") -Force
    }
    $failurePhase = if ($null -ne $testExit) { "E2E test command ran and did not pass; inspect test logs." } elseif ($composeAttempted -and $composeStarted) { "Compose services started but a later step did not pass." } elseif ($composeAttempted) { "Compose startup or health gate did not pass." } else { "Pinned MinIO source-image preflight did not complete; no test gate ran." }
    $result = if ($null -ne $testExit) { "FAIL" } else { "NOT VERIFIED" }
    @("run_id=$project", "result=$result", "reason=$failurePhase", "secrets_recorded=false") | Set-Content -LiteralPath (Join-Path $evidenceDirectory "status.txt") -Encoding utf8
    Get-Content -LiteralPath (Join-Path $evidenceDirectory "logs\compose.log") -Tail 100 -ErrorAction SilentlyContinue
    Get-Content -LiteralPath (Join-Path $evidenceDirectory "logs\services.redacted.log") -Tail 100 -ErrorAction SilentlyContinue
    if (Test-Path -LiteralPath $testLog) { Get-Content -LiteralPath $testLog -Tail 100 }
    throw
} finally {
    if ($composeAttempted) {
        try { Invoke-Compose @("down", "--rmi", "local", "--volumes", "--remove-orphans") }
        catch { Write-Warning "Could not fully clean this isolated Compose project; see the E: diagnostic log." }
    }
    foreach ($name in $environmentNames) { [Environment]::SetEnvironmentVariable($name, $previous[$name], "Process") }
    if ($createdImage) {
        $savedErrorActionPreference = $ErrorActionPreference
        try {
            $ErrorActionPreference = "Continue"
            & docker image rm $minioImage *> $composeLog
            if ($LASTEXITCODE -ne 0) { Write-Warning "Could not remove this run's MinIO image; inspect the E: diagnostic log." }
        } finally { $ErrorActionPreference = $savedErrorActionPreference }
    }
    $resolvedTemporary = (Resolve-Path -LiteralPath $temporaryDirectory -ErrorAction SilentlyContinue).Path
    $resolvedTempRoot = (Resolve-Path -LiteralPath $temporaryRoot).Path
    if ($resolvedTemporary -and $resolvedTemporary.StartsWith($resolvedTempRoot, [StringComparison]::OrdinalIgnoreCase) -and (Split-Path $resolvedTemporary -Leaf) -eq $project) {
        Remove-Item -LiteralPath $resolvedTemporary -Recurse -Force -ErrorAction SilentlyContinue
    }
}
