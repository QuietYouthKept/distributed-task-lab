[CmdletBinding()]
param()

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
$temporaryDirectory = Join-Path ([IO.Path]::GetTempPath()) $project
New-Item -ItemType Directory -Path $temporaryDirectory | Out-Null
$composeLog = Join-Path $temporaryDirectory "compose.log"
$testLog = Join-Path $temporaryDirectory "test.log"
$environmentNames = @(
    "GITHUB_RUN_ID", "GITHUB_RUN_ATTEMPT", "GITHUB_WORKSPACE", "GITHUB_ENV",
    "MINIO_CI_IMAGE", "E2E_POSTGRES_PASSWORD", "E2E_POSTGRES_PORT", "E2E_MINIO_PORT",
    "E2E_API_PORT", "E2E_S3_ACCESS_KEY", "E2E_S3_SECRET_KEY", "E2E_API_TOKEN",
    "E2E_ADMIN_TOKEN", "E2E_ARTIFACT_PREFIX", "E2E_COMPOSE_PROJECT", "E2E_COMPOSE_FILE",
    "REQUIRE_CONTAINER_E2E", "E2E_API_URL", "E2E_DATABASE_URL", "E2E_MINIO_ENDPOINT",
    "E2E_MINIO_BUCKET", "REQUIRE_MINIO", "MINIO_ENDPOINT", "MINIO_BUCKET", "MINIO_ACCESS_KEY",
    "MINIO_SECRET_KEY", "E2E_COMPOSE_FILE", "GOTOOLCHAIN"
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
    $output = & docker compose --project-name $project --file $composeFile @Arguments 2>&1
    $code = $LASTEXITCODE
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

    [Environment]::SetEnvironmentVariable("GITHUB_RUN_ID", $runID, "Process")
    [Environment]::SetEnvironmentVariable("GITHUB_RUN_ATTEMPT", "1", "Process")
    [Environment]::SetEnvironmentVariable("GITHUB_WORKSPACE", $repoRoot, "Process")
    [Environment]::SetEnvironmentVariable("GITHUB_ENV", $null, "Process")
    [Environment]::SetEnvironmentVariable("MINIO_CI_IMAGE", $minioImage, "Process")
    Push-Location $repoRoot
    try {
        & $gitBash (Join-Path $repoRoot "scripts\evidence\build-minio-ci-image.sh") *> $composeLog
        if ($LASTEXITCODE -ne 0) { throw "Pinned MinIO source build failed (exit=$LASTEXITCODE); see temporary diagnostics." }
    } finally { Pop-Location }
    $imageID = & docker image inspect $minioImage --format "{{.Id}}"
    if ($LASTEXITCODE -ne 0 -or -not $imageID) { throw "Pinned MinIO source image was not created." }
    $createdImage = $true

    $postgresPort = Get-FreeLoopbackPort
    $minioPort = Get-FreeLoopbackPort
    $apiPort = Get-FreeLoopbackPort
    $secret = [guid]::NewGuid().ToString("N") + [guid]::NewGuid().ToString("N")
    $values = @{
        E2E_POSTGRES_PASSWORD = $secret
        E2E_POSTGRES_PORT = [string]$postgresPort
        E2E_MINIO_PORT = [string]$minioPort
        E2E_API_PORT = [string]$apiPort
        E2E_S3_ACCESS_KEY = "e2e" + [guid]::NewGuid().ToString("N")
        E2E_S3_SECRET_KEY = [guid]::NewGuid().ToString("N") + [guid]::NewGuid().ToString("N")
        E2E_API_TOKEN = [guid]::NewGuid().ToString("N") + [guid]::NewGuid().ToString("N")
        E2E_ADMIN_TOKEN = [guid]::NewGuid().ToString("N") + [guid]::NewGuid().ToString("N")
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
        try {
            $healthResponse = Invoke-WebRequest -Uri "http://127.0.0.1:$minioPort/minio/health/live" -TimeoutSec 2 -UseBasicParsing
            if ($healthResponse.StatusCode -eq 200) { $minioHealthy = $true; break }
        } catch { }
        Start-Sleep -Seconds 1
    }
    if (-not $minioHealthy) { throw "MinIO HTTP health endpoint did not return 200 within 90 seconds." }
    Invoke-Compose @("up", "--detach", "--build", "--wait", "--wait-timeout", "240", "api", "worker-a")
    Invoke-Compose @("up", "--detach", "--build", "--wait", "--wait-timeout", "120", "worker-b")

    [Environment]::SetEnvironmentVariable("REQUIRE_CONTAINER_E2E", "1", "Process")
    [Environment]::SetEnvironmentVariable("E2E_API_URL", "http://127.0.0.1:$apiPort", "Process")
    [Environment]::SetEnvironmentVariable("E2E_DATABASE_URL", "postgres://postgres:$secret@127.0.0.1:$postgresPort/tasklab?sslmode=disable", "Process")
    [Environment]::SetEnvironmentVariable("E2E_MINIO_ENDPOINT", "http://127.0.0.1:$minioPort", "Process")
    [Environment]::SetEnvironmentVariable("E2E_MINIO_BUCKET", "tasklab-e2e", "Process")
    [Environment]::SetEnvironmentVariable("REQUIRE_MINIO", "1", "Process")
    [Environment]::SetEnvironmentVariable("MINIO_ENDPOINT", "http://127.0.0.1:$minioPort", "Process")
    [Environment]::SetEnvironmentVariable("MINIO_BUCKET", "tasklab-e2e", "Process")
    [Environment]::SetEnvironmentVariable("MINIO_ACCESS_KEY", $values.E2E_S3_ACCESS_KEY, "Process")
    [Environment]::SetEnvironmentVariable("MINIO_SECRET_KEY", $values.E2E_S3_SECRET_KEY, "Process")
    $dirtyAtTest = [bool](& git status --porcelain)

    Push-Location $repoRoot
    try {
        & go test -race ./internal/artifacts -run '^TestMinIOStoreContractAndConcurrentConditionalWrite$' -count=1 -v *> $testLog
        $artifactExit = $LASTEXITCODE
        & go test -race ./internal/e2e -run '^TestAPIWorkerContainersRecoverSharedArtifact$' -count=1 -v *> (Join-Path $temporaryDirectory "container-recovery.log")
        $containerExit = $LASTEXITCODE
        $testExit = if ($artifactExit -eq 0 -and $containerExit -eq 0) { 0 } else { 1 }
    } finally { Pop-Location }
    Get-Content -LiteralPath $testLog
    Get-Content -LiteralPath (Join-Path $temporaryDirectory "container-recovery.log")
    if ($testExit -ne 0) { throw "Cross-container E2E failed with exit code $testExit." }
    $successDirectory = Join-Path $repoRoot ("evidence\phase1\runs\wave3-container-e2e\" + $project)
    New-Item -ItemType Directory -Path (Join-Path $successDirectory "logs") -Force | Out-Null
    Copy-Item -LiteralPath $testLog -Destination (Join-Path $successDirectory "logs\minio-response-loss-contract.log")
    Copy-Item -LiteralPath (Join-Path $temporaryDirectory "container-recovery.log") -Destination (Join-Path $successDirectory "logs\container-recovery.log")
    $serviceLogs = & docker compose --project-name $project --file $composeFile logs --no-color --timestamps 2>&1
    $redactedServiceLogs = ($serviceLogs -join "`n")
    foreach ($name in @("E2E_POSTGRES_PASSWORD", "E2E_S3_ACCESS_KEY", "E2E_S3_SECRET_KEY", "E2E_API_TOKEN", "E2E_ADMIN_TOKEN")) {
        $value = [Environment]::GetEnvironmentVariable($name, "Process")
        if ($value) { $redactedServiceLogs = $redactedServiceLogs.Replace($value, "[REDACTED]") }
    }
    $redactedServiceLogs | Set-Content -LiteralPath (Join-Path $successDirectory "logs\services.redacted.log") -Encoding utf8
    $implementationCommit = (& git log -1 --format=%H -- internal/artifacts/minio_integration_test.go internal/e2e/container_recovery_test.go internal/e2e/worker_recovery_test.go deployments/e2e/compose.yaml scripts/e2e/run-cross-container.ps1).Trim()
    if (-not $implementationCommit) { throw "Could not resolve the commit that introduced the cross-container E2E implementation." }
    $files = @()
    foreach ($relativePath in @("logs/minio-response-loss-contract.log", "logs/container-recovery.log", "logs/services.redacted.log")) {
        $fullPath = Join-Path $successDirectory ($relativePath -replace '/', '\')
        $hash = Get-FileHash -LiteralPath $fullPath -Algorithm SHA256
        $files += [ordered]@{ path = $relativePath; sha256 = $hash.Hash.ToLowerInvariant(); bytes = (Get-Item -LiteralPath $fullPath).Length }
    }
    $manifest = [ordered]@{
        run_id = $project
        result = "PASS"
        implementation_commit = $implementationCommit
        worktree_dirty_at_test = $dirtyAtTest
        docker_compose_project = $project
        minio_source_commit = "9e49d5e7a648f00e26f2246f4dc28e6b07f8c84a"
        minio_ephemeral_local_image_id = $imageID
        postgres_image = "postgres:16-alpine"
        credentials_recorded = $false
        checks = @(
            [ordered]@{ name = "minio-response-loss-contract"; exit_code = $artifactExit; status = $(if ($artifactExit -eq 0) { "PASS" } else { "FAIL" }) },
            [ordered]@{ name = "api-two-worker-container-recovery"; exit_code = $containerExit; status = $(if ($containerExit -eq 0) { "PASS" } else { "FAIL" }) }
        )
        files = $files
    }
    $manifest | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath (Join-Path $successDirectory "manifest.json") -Encoding utf8
    Write-Output "Cross-container E2E passed; Compose project was $project; MinIO source image was $imageID. Credentials were ephemeral and are not recorded."
} catch {
    Write-Output "Cross-container E2E did not pass: $($_.Exception.Message)"
    $evidenceDirectory = Join-Path $repoRoot ("evidence\phase1\runs\wave3-container-e2e-attempts\" + $project)
    New-Item -ItemType Directory -Path (Join-Path $evidenceDirectory "logs") -Force | Out-Null
    if (Test-Path -LiteralPath $composeLog) { Copy-Item -LiteralPath $composeLog -Destination (Join-Path $evidenceDirectory "logs\compose.log") -Force }
    if (Test-Path -LiteralPath $testLog) { Copy-Item -LiteralPath $testLog -Destination (Join-Path $evidenceDirectory "logs\test.log") -Force }
    if (Test-Path -LiteralPath (Join-Path $temporaryDirectory "container-recovery.log")) { Copy-Item -LiteralPath (Join-Path $temporaryDirectory "container-recovery.log") -Destination (Join-Path $evidenceDirectory "logs\container-recovery.log") -Force }
    $redacted = "No Compose service was started."
    if ($composeAttempted) {
        $diagnostics = & docker compose --project-name $project --file $composeFile logs --no-color --timestamps 2>&1
        $redacted = ($diagnostics -join "`n")
        foreach ($name in @("E2E_POSTGRES_PASSWORD", "E2E_S3_ACCESS_KEY", "E2E_S3_SECRET_KEY", "E2E_API_TOKEN", "E2E_ADMIN_TOKEN")) {
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
        & docker compose --project-name $project --file $composeFile down --rmi local --volumes --remove-orphans *> $null
    }
    foreach ($name in $environmentNames) { [Environment]::SetEnvironmentVariable($name, $previous[$name], "Process") }
    if ($createdImage) { & docker image rm $minioImage *> $null }
    $resolvedTemporary = (Resolve-Path -LiteralPath $temporaryDirectory -ErrorAction SilentlyContinue).Path
    $resolvedTempRoot = (Resolve-Path -LiteralPath ([IO.Path]::GetTempPath())).Path
    if ($resolvedTemporary -and $resolvedTemporary.StartsWith($resolvedTempRoot, [StringComparison]::OrdinalIgnoreCase) -and (Split-Path $resolvedTemporary -Leaf) -eq $project) {
        Remove-Item -LiteralPath $resolvedTemporary -Recurse -Force -ErrorAction SilentlyContinue
    }
}
