[CmdletBinding()]
param([string]$RunID = "")

$ErrorActionPreference = "Stop"
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
Set-Location $repoRoot

function Get-Text([string]$File, [string[]]$Arguments) {
    try {
        $output = & $File @Arguments 2>&1
        if ($LASTEXITCODE -ne 0) { return "unavailable" }
        return (($output -join "`n").Trim())
    } catch { return "unavailable" }
}

function Get-FileSHA256([string]$Path) {
    $algorithm = [System.Security.Cryptography.SHA256]::Create()
    $stream = [System.IO.File]::OpenRead($Path)
    try { return ([BitConverter]::ToString($algorithm.ComputeHash($stream))).Replace("-", "").ToLowerInvariant() }
    finally { $stream.Dispose(); $algorithm.Dispose() }
}

function Invoke-RecordedCheck([string]$Name, [string]$File, [string[]]$Arguments, [hashtable]$Environment, [string]$LogDir, [string]$WorkingDirectory = $repoRoot) {
    $stdout = Join-Path $LogDir ($Name + ".stdout.log")
    $stderr = Join-Path $LogDir ($Name + ".stderr.log")
    $old = @{}
    foreach ($key in $Environment.Keys) {
        $old[$key] = [Environment]::GetEnvironmentVariable($key, "Process")
        [Environment]::SetEnvironmentVariable($key, $Environment[$key], "Process")
    }
    $started = (Get-Date).ToUniversalTime()
    try {
        $process = Start-Process -FilePath $File -ArgumentList $Arguments -WorkingDirectory $WorkingDirectory -Wait -PassThru -NoNewWindow -RedirectStandardOutput $stdout -RedirectStandardError $stderr
        $code = $process.ExitCode
    } catch {
        $code = 127
        [System.IO.File]::WriteAllText($stderr, $_.Exception.Message)
    } finally {
        foreach ($key in $Environment.Keys) { [Environment]::SetEnvironmentVariable($key, $old[$key], "Process") }
    }
    return [ordered]@{
        name = $Name
        command = (@($File) + $Arguments) -join " "
        exit_code = $code
        status = if ($code -eq 0) { "PASS" } else { "FAIL" }
        started_at_utc = $started.ToString("o")
        finished_at_utc = (Get-Date).ToUniversalTime().ToString("o")
        stdout = $stdout.Substring($repoRoot.Length + 1).Replace('\', '/')
        stderr = $stderr.Substring($repoRoot.Length + 1).Replace('\', '/')
    }
}

$commit = (Get-Text "git" @("rev-parse", "HEAD"))
if ($commit -eq "unavailable") { throw "Cannot identify the Git commit." }
$initialStatus = @(& git status --porcelain=v1)
if ($LASTEXITCODE -ne 0) { throw "Cannot read Git worktree status." }
if ($initialStatus.Count -ne 0) { throw "Evidence collection requires a clean worktree; no files were changed." }

if (-not $RunID) { $RunID = "wave2-" + (Get-Date).ToUniversalTime().ToString("yyyyMMddTHHmmssfffZ") + "-" + $commit.Substring(0, 12) }
if ($RunID -notmatch '^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$') { throw "RunID has invalid characters." }
$phaseRoot = Join-Path $repoRoot "evidence\phase1"
$runRoot = Join-Path (Join-Path $phaseRoot "runs") $RunID
if (Test-Path -LiteralPath $runRoot) { throw "Evidence run exists; refusing to overwrite: $RunID" }
$logDir = Join-Path $runRoot "logs"
New-Item -ItemType Directory -Path $logDir -Force | Out-Null

$checks = @()
$requiredDB = @{ REQUIRE_TEST_DATABASE = "1" }
$checks += Invoke-RecordedCheck "unit-integration" "go" @("test", "./cmd/...", "./internal/...", "./pkg/...", "-count=1") $requiredDB $logDir
$checks += Invoke-RecordedCheck "full-race" "go" @("test", "-race", "./cmd/...", "./internal/...", "./pkg/...", "-count=1") $requiredDB $logDir
$checks += Invoke-RecordedCheck "postgres-race-five" "go" @("test", "-race", "./internal/storage/postgres", "-count=5") $requiredDB $logDir
$checks += Invoke-RecordedCheck "retry-backoff-race-twenty" "go" @("test", "-race", "./internal/storage/postgres", "-run", "^TestRetryBackoff$", "-count=20", "-v") $requiredDB $logDir
$checks += Invoke-RecordedCheck "failurelab" "go" @("run", "./cmd/failurelab") @{} $logDir
$chaosOutputRoot = [Environment]::GetEnvironmentVariable("CHAOS_OUTPUT_DIR", "Process")
if (-not $chaosOutputRoot) { $chaosOutputRoot = Join-Path $repoRoot "benchmarks\chaos" }
$existingChaosRuns = @((Get-ChildItem -LiteralPath $chaosOutputRoot -Directory -ErrorAction SilentlyContinue | ForEach-Object { $_.Name }))
$checks += Invoke-RecordedCheck "chaosmatrix" "go" @("run", "./cmd/chaosmatrix") @{ CHAOS_OUTPUT_DIR = $chaosOutputRoot } $logDir
$paginationOutputRoot = [Environment]::GetEnvironmentVariable("PAGINATION_BENCH_OUTPUT_DIR", "Process")
if (-not $paginationOutputRoot) { $paginationOutputRoot = Join-Path $repoRoot "benchmarks\pagination" }
$existingPaginationRuns = @((Get-ChildItem -LiteralPath $paginationOutputRoot -Directory -ErrorAction SilentlyContinue | ForEach-Object { $_.Name }))
$checks += Invoke-RecordedCheck "pagination-benchmark-100k" "go" @("run", "./cmd/paginationbench") @{ REQUIRE_TEST_DATABASE = "1"; PAGINATION_BENCH_OUTPUT_DIR = $paginationOutputRoot } $logDir
$checks += Invoke-RecordedCheck "vet" "go" @("vet", "./...") @{} $logDir
$checks += Invoke-RecordedCheck "external-client-race" "go" @("test", "-race", "./...", "-count=1", "-v") @{} $logDir (Join-Path $repoRoot "test\external-client")
$checks += Invoke-RecordedCheck "otlp-receiver-race" "go" @("test", "-race", "./internal/observability", "-run", "^TestSetupTracingExportsSpanToOTLPHTTPReceiver$", "-count=1", "-v") @{} $logDir

$chaosSource = $null
$chaosFiles = @()
$chaosState = "NOT VERIFIED"
$chaosRunID = $null
$newChaosRuns = @((Get-ChildItem -LiteralPath $chaosOutputRoot -Directory -ErrorAction SilentlyContinue | Where-Object { $_.Name -notin $existingChaosRuns } | Sort-Object LastWriteTimeUtc))
if ($newChaosRuns.Count -eq 1) {
    $chaosSource = $newChaosRuns[0].FullName
    $chaosRunID = $newChaosRuns[0].Name
    $chaosTarget = Join-Path (Join-Path $runRoot "artifacts\chaosmatrix") $chaosRunID
    New-Item -ItemType Directory -Path $chaosTarget -Force | Out-Null
    foreach ($sourceFile in Get-ChildItem -LiteralPath $chaosSource -File) {
        $targetFile = Join-Path $chaosTarget $sourceFile.Name
        Copy-Item -LiteralPath $sourceFile.FullName -Destination $targetFile
        $chaosFiles += [ordered]@{
            file = $targetFile.Substring($repoRoot.Length + 1).Replace('\', '/')
            sha256 = Get-FileSHA256 $targetFile
            bytes = (Get-Item -LiteralPath $targetFile).Length
        }
    }
    $chaosSummaryPath = Join-Path $chaosTarget "summary.json"
    if (Test-Path -LiteralPath $chaosSummaryPath) {
        $chaosSummary = Get-Content -LiteralPath $chaosSummaryPath -Raw | ConvertFrom-Json
        $chaosScenarios = @($chaosSummary.PSObject.Properties | ForEach-Object { $_.Value })
        if ($chaosScenarios.Count -gt 0 -and @($chaosScenarios | Where-Object { $_.Failed -ne 0 -or $_.Passed -ne $_.Trials }).Count -eq 0) { $chaosState = "PASS" } else { $chaosState = "FAIL" }
    } else { $chaosState = "FAIL" }
} elseif ($newChaosRuns.Count -gt 1) {
    $chaosState = "NOT VERIFIED"
}

$paginationFiles = @()
$paginationState = "NOT VERIFIED"
$paginationRunID = $null
$newPaginationRuns = @((Get-ChildItem -LiteralPath $paginationOutputRoot -Directory -ErrorAction SilentlyContinue | Where-Object { $_.Name -notin $existingPaginationRuns } | Sort-Object LastWriteTimeUtc))
if ($newPaginationRuns.Count -eq 1) {
    $paginationSource = $newPaginationRuns[0].FullName
    $paginationRunID = $newPaginationRuns[0].Name
    $paginationTarget = Join-Path (Join-Path $runRoot "artifacts\paginationbench") $paginationRunID
    New-Item -ItemType Directory -Path $paginationTarget -Force | Out-Null
    foreach ($fileName in @("manifest.json", "report.md")) {
        $sourceFile = Join-Path $paginationSource $fileName
        if (-not (Test-Path -LiteralPath $sourceFile -PathType Leaf)) { continue }
        $targetFile = Join-Path $paginationTarget $fileName
        Copy-Item -LiteralPath $sourceFile -Destination $targetFile
        $paginationFiles += [ordered]@{
            file = $targetFile.Substring($repoRoot.Length + 1).Replace('\', '/')
            sha256 = Get-FileSHA256 $targetFile
            bytes = (Get-Item -LiteralPath $targetFile).Length
        }
    }
    $paginationManifestPath = Join-Path $paginationTarget "manifest.json"
    if ((Test-Path -LiteralPath $paginationManifestPath) -and $paginationFiles.Count -eq 2) {
        $paginationManifest = Get-Content -LiteralPath $paginationManifestPath -Raw | ConvertFrom-Json
        if ($paginationManifest.result -eq "PASS" -and $paginationManifest.Generator.rows -eq 100000 -and $paginationManifest.keyset_walk.rows_seen -eq 100000 -and $paginationManifest.keyset_walk.duplicates -eq 0 -and $paginationManifest.concurrent_write_check.initial_rows_seen -eq 100000 -and $paginationManifest.concurrent_write_check.concurrent_rows_written -eq 50 -and $paginationManifest.concurrent_write_check.first_concurrent_insert_committed_before_continuation -eq $true -and $paginationManifest.concurrent_write_check.duplicates -eq 0) {
            $paginationState = "PASS"
        } else { $paginationState = "FAIL" }
    } else { $paginationState = "FAIL" }
} elseif ($newPaginationRuns.Count -gt 1) {
    $paginationState = "NOT VERIFIED"
}
$checks += [ordered]@{ name = "pagination-benchmark-evidence"; command = "100k real PostgreSQL rows, keyset walk, concurrent inserts, EXPLAIN ANALYZE BUFFERS"; exit_code = if ($paginationState -eq "PASS") { 0 } elseif ($paginationState -eq "FAIL") { 1 } else { $null }; status = $paginationState; run_id = $paginationRunID }

$minioRequired = @("MINIO_ENDPOINT", "MINIO_BUCKET", "MINIO_ACCESS_KEY", "MINIO_SECRET_KEY")
$missingMinio = @($minioRequired | Where-Object { -not [Environment]::GetEnvironmentVariable($_, "Process") })
if ($missingMinio.Count -eq 0) {
    $checks += Invoke-RecordedCheck "minio-contract-race" "go" @("test", "-race", "./internal/artifacts", "-run", "^TestMinIOStoreContractAndConcurrentConditionalWrite$", "-count=1", "-v") @{ REQUIRE_MINIO = "1" } $logDir
    $checks += Invoke-RecordedCheck "api-two-worker-artifact-recovery-race" "go" @("test", "-race", "./internal/e2e", "-run", "^TestAPITwoWorkersRecoverSharedArtifactAfterAckProcessCrash$", "-count=1", "-v") @{ REQUIRE_TEST_DATABASE = "1"; REQUIRE_MINIO = "1" } $logDir
    $backupContainer = [Environment]::GetEnvironmentVariable("PG_BACKUP_CONTAINER", "Process")
    if ($backupContainer) {
        $checks += Invoke-RecordedCheck "postgres-backup-restore-drill-race" "go" @("test", "-race", "./internal/storage/postgres", "-run", "^TestDatabaseBackupRestoreDrill$", "-count=1", "-v") @{ REQUIRE_TEST_DATABASE = "1"; REQUIRE_MINIO = "1"; RUN_BACKUP_RESTORE_DRILL = "1"; PG_BACKUP_CONTAINER = $backupContainer } $logDir
    } else {
        $checks += [ordered]@{ name = "postgres-backup-restore-drill-race"; command = "not run: PG_BACKUP_CONTAINER is not configured"; exit_code = $null; status = "NOT VERIFIED" }
    }
} else {
    $checks += [ordered]@{ name = "minio-contract-race"; command = "not run: required MinIO environment variables absent"; exit_code = $null; status = "NOT VERIFIED"; missing_variable_names = $missingMinio }
    $checks += [ordered]@{ name = "api-two-worker-artifact-recovery-race"; command = "not run: required PostgreSQL and MinIO environment variables absent"; exit_code = $null; status = "NOT VERIFIED"; missing_variable_names = $missingMinio }
    $checks += [ordered]@{ name = "postgres-backup-restore-drill-race"; command = "not run: required PostgreSQL and MinIO environment variables absent"; exit_code = $null; status = "NOT VERIFIED"; missing_variable_names = $missingMinio }
}

$dirtyAfter = @(& git status --porcelain=v1)
$otherDirtyAfter = @($dirtyAfter | Where-Object { $_ -notmatch '^\?\? evidence/phase1/' -and $_ -notmatch '^\s*[A-Z?]{1,2} evidence/phase1/' })
$failed = @($checks | Where-Object { $_.status -eq "FAIL" }).Count
$unverified = @($checks | Where-Object { $_.status -eq "NOT VERIFIED" }).Count
$configHash = Get-FileSHA256 (Join-Path $repoRoot "internal\config\config.go")
$fingerprintInput = @(
    $commit,
    $configHash,
    "REQUIRE_TEST_DATABASE=1",
    "go test ./cmd/... ./internal/... ./pkg/... -count=1",
    "go test -race ./cmd/... ./internal/... ./pkg/... -count=1",
    "go test -race ./internal/storage/postgres -count=5",
    "go test -race ./internal/storage/postgres -run ^TestRetryBackoff$ -count=20 -v",
    "go run ./cmd/failurelab",
    "go run ./cmd/chaosmatrix",
    "go run ./cmd/paginationbench",
    "go vet ./...",
    "go test -race ./... -count=1 (test/external-client)",
    "go test -race ./internal/observability -run ^TestSetupTracingExportsSpanToOTLPHTTPReceiver$ -count=1 -v",
    "go test -race ./internal/e2e -run ^TestAPITwoWorkersRecoverSharedArtifactAfterAckProcessCrash$ -count=1 -v",
    "go test -race ./internal/storage/postgres -run ^TestDatabaseBackupRestoreDrill$ -count=1 -v"
) -join "`n"
$sha = [System.Security.Cryptography.SHA256]::Create()
try { $configFingerprint = ([BitConverter]::ToString($sha.ComputeHash([Text.Encoding]::UTF8.GetBytes($fingerprintInput)))).Replace("-", "").ToLowerInvariant() } finally { $sha.Dispose() }
foreach ($check in $checks) {
    if ($check.stdout) {
        $file = Join-Path $repoRoot ($check.stdout.Replace('/', '\'))
        $check.stdout_sha256 = Get-FileSHA256 $file
        $check.stdout_bytes = (Get-Item -LiteralPath $file).Length
    }
    if ($check.stderr) {
        $file = Join-Path $repoRoot ($check.stderr.Replace('/', '\'))
        $check.stderr_sha256 = Get-FileSHA256 $file
        $check.stderr_bytes = (Get-Item -LiteralPath $file).Length
    }
}
$minioContainer = [Environment]::GetEnvironmentVariable("MINIO_CONTAINER", "Process")
if (-not $minioContainer) { $minioContainer = "tasklab-wave2-minio" }
$minioImageID = Get-Text "docker" @("inspect", $minioContainer, "--format", "{{.Image}}")
$minioHealth = "NOT VERIFIED"
try {
    $healthEndpoint = [Environment]::GetEnvironmentVariable("MINIO_ENDPOINT", "Process")
    if (-not $healthEndpoint) { $healthEndpoint = "http://127.0.0.1:19000" }
    $healthResponse = Invoke-WebRequest -Uri ($healthEndpoint.TrimEnd('/') + "/minio/health/live") -TimeoutSec 3 -UseBasicParsing
    $minioHealth = "HTTP " + $healthResponse.StatusCode
} catch { $minioHealth = "unreachable" }
$manifest = [ordered]@{
    schema_version = 1
    run_id = $RunID
    collected_at_utc = (Get-Date).ToUniversalTime().ToString("o")
    implementation_commit = $commit
    worktree_clean_at_start = $true
    worktree_clean_after_excluding_run_artifacts = ($otherDirtyAfter.Count -eq 0)
    environment = [ordered]@{
        go = Get-Text "go" @("version")
        os = [System.Environment]::OSVersion.VersionString
        docker_server = Get-Text "docker" @("info", "--format", "{{.ServerVersion}}")
        postgres_container = Get-Text "docker" @("inspect", "tasklab-test-postgres", "--format", "{{.Config.Image}}")
        postgres_server = Get-Text "docker" @("exec", "tasklab-test-postgres", "pg_config", "--version")
        minio_container = $minioContainer
        minio_image_id = $minioImageID
        minio_health = $minioHealth
        minio_endpoint_configured = [bool]([Environment]::GetEnvironmentVariable("MINIO_ENDPOINT", "Process"))
        minio_required_variables_present = ($missingMinio.Count -eq 0)
    }
    configuration_fingerprint_sha256 = $configFingerprint
    configuration_source_sha256 = $configHash
    test_fixture_commit_sha = $commit
    checks = $checks
    chaos_matrix = [ordered]@{ status = $chaosState; run_id = $chaosRunID; files = $chaosFiles }
    pagination_benchmark = [ordered]@{ status = $paginationState; run_id = $paginationRunID; files = $paginationFiles }
    result = if ($failed -gt 0) { "FAIL" } elseif ($unverified -gt 0 -or $otherDirtyAfter.Count -gt 0) { "NOT VERIFIED" } else { "PASS" }
    limitations = @("This run does not by itself prove dual-worker E2E, external IdP/provider, OTLP staging receipt, DR, or production readiness.", "Credential values are never recorded.")
}
$manifestPath = Join-Path $runRoot "manifest.json"
$manifest | ConvertTo-Json -Depth 10 | Set-Content -LiteralPath $manifestPath -Encoding utf8

$indexPath = Join-Path $phaseRoot "index.json"
$records = @()
if (Test-Path -LiteralPath $indexPath) {
    $existing = Get-Content -LiteralPath $indexPath -Raw | ConvertFrom-Json
    $records = @($existing.records)
}
$records += [ordered]@{ evidence_id = $RunID; status = $manifest.result; implementation_commit = $commit; manifest = "evidence/phase1/runs/$RunID/manifest.json" }
[ordered]@{ schema_version = 1; updated_at_utc = (Get-Date).ToUniversalTime().ToString("o"); records = $records } |
    ConvertTo-Json -Depth 10 | Set-Content -LiteralPath $indexPath -Encoding utf8

Write-Output ("Evidence run: {0}" -f $RunID)
Write-Output ("Result: {0}; checks: {1}; failed: {2}; not verified: {3}" -f $manifest.result, $checks.Count, $failed, $unverified)
Write-Output ("Manifest: {0}" -f $manifestPath)
if ($failed -gt 0) { exit 1 }
