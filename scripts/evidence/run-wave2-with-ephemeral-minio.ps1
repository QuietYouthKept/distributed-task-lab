[CmdletBinding()]
param([string]$RunID = "")

$ErrorActionPreference = "Stop"
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
$containerName = "tasklab-wave2-evidence-minio"
$endpoint = "http://127.0.0.1:19002"
$bucket = "tasklab-wave2-evidence"
$temporaryRoot = 'E:\distributed-task-lab-temp'
$artifactRoot = 'E:\distributed-task-lab-temp\wave3-artifacts'
New-Item -ItemType Directory -Path $temporaryRoot -Force | Out-Null
$portInUse = Test-NetConnection 127.0.0.1 -Port 19002 -InformationLevel Quiet
if ($portInUse) { throw "Port 19002 is already in use; refusing to alter an existing service." }
if (@(& docker ps -a --filter "name=^$containerName$" --format "{{.Names}}") -contains $containerName) {
    throw "Container name $containerName already exists; refusing to alter it."
}

$random = [guid]::NewGuid().ToString("N")
$image = "tasklab-minio-ci:wave2-$random-1"
$accessKey = $random
$secretKey = [guid]::NewGuid().ToString("N") + [guid]::NewGuid().ToString("N")
$shortCommit = (& git -C $repoRoot rev-parse --short=12 HEAD).Trim()
if (-not $RunID) { $RunID = "wave3-" + (Get-Date).ToUniversalTime().ToString("yyyyMMddTHHmmssfffZ") + "-" + $shortCommit }
if ($RunID -notmatch '^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$') { throw "RunID has invalid characters." }
$runArtifactRoot = Join-Path $artifactRoot $RunID
if (Test-Path -LiteralPath (Join-Path $repoRoot ("evidence\phase1\runs\" + $RunID))) { throw "Evidence run already exists; refusing to overwrite: $RunID" }
if (Test-Path -LiteralPath $runArtifactRoot) { throw "E: artifact run already exists; refusing to overwrite: $RunID" }
New-Item -ItemType Directory -Path $runArtifactRoot -Force | Out-Null
$chaosOutputRoot = Join-Path $runArtifactRoot "chaos"
$paginationOutputRoot = Join-Path $runArtifactRoot "pagination"
$temporaryDirectory = Join-Path $temporaryRoot ("tasklab-evidence-" + [guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $temporaryDirectory | Out-Null
$environmentFile = Join-Path $temporaryDirectory "minio.env"
$collectorOutput = Join-Path $temporaryDirectory "collector.stdout.log"
$createdContainer = $false
$createdImage = $false
$names = @("MINIO_ENDPOINT", "MINIO_BUCKET", "MINIO_ACCESS_KEY", "MINIO_SECRET_KEY", "MINIO_CONTAINER", "REQUIRE_TEST_DATABASE", "TEST_DATABASE_URL", "PG_BACKUP_CONTAINER", "GITHUB_RUN_ID", "GITHUB_RUN_ATTEMPT", "GITHUB_WORKSPACE", "GITHUB_ENV", "GOTOOLCHAIN", "RUNNER_TEMP", "TMPDIR", "CHAOS_OUTPUT_DIR", "PAGINATION_BENCH_OUTPUT_DIR")
$previous = @{}
foreach ($name in $names) { $previous[$name] = [Environment]::GetEnvironmentVariable($name, "Process") }

try {
    $gitPath = (Get-Command git -ErrorAction Stop).Source
    $gitRoot = Split-Path (Split-Path $gitPath -Parent) -Parent
    $gitBash = Join-Path $gitRoot "bin\bash.exe"
    if (-not (Test-Path -LiteralPath $gitBash)) { throw "Git Bash was not found at $gitBash; install Git for Windows to build pinned MinIO from source." }
    $gitTempRoot = (& $gitBash -lc "cygpath -u '$temporaryRoot'").Trim()
    if (-not $gitTempRoot.StartsWith("/")) { throw "Could not map the E: temporary directory into Git Bash." }
    [Environment]::SetEnvironmentVariable("RUNNER_TEMP", $gitTempRoot, "Process")
    [Environment]::SetEnvironmentVariable("TMPDIR", $gitTempRoot, "Process")
    [Environment]::SetEnvironmentVariable("CHAOS_OUTPUT_DIR", $chaosOutputRoot, "Process")
    [Environment]::SetEnvironmentVariable("PAGINATION_BENCH_OUTPUT_DIR", $paginationOutputRoot, "Process")
    [Environment]::SetEnvironmentVariable("GITHUB_RUN_ID", "wave2-$random", "Process")
    [Environment]::SetEnvironmentVariable("GITHUB_RUN_ATTEMPT", "1", "Process")
    [Environment]::SetEnvironmentVariable("GITHUB_WORKSPACE", $repoRoot, "Process")
    [Environment]::SetEnvironmentVariable("GITHUB_ENV", $null, "Process")
    # A missing random tag is the expected preflight result. Keep PowerShell
    # from promoting Docker's stderr for that probe into a terminating error.
    $savedErrorActionPreference = $ErrorActionPreference
    try {
        $ErrorActionPreference = "Continue"
        $existingImage = & docker image inspect $image --format "{{.Id}}" 2>$null
        $imageInspectExitCode = $LASTEXITCODE
    } finally { $ErrorActionPreference = $savedErrorActionPreference }
    if ($imageInspectExitCode -eq 0 -and $existingImage) { throw "Temporary MinIO image tag $image already exists; refusing to overwrite it." }
    Push-Location $repoRoot
    try {
        & $gitBash (Join-Path $repoRoot "scripts\evidence\build-minio-ci-image.sh")
        if ($LASTEXITCODE -ne 0) { throw "Pinned MinIO source build failed with exit code $LASTEXITCODE." }
    } finally { Pop-Location }
    $imageID = & docker image inspect $image --format "{{.Id}}"
    if ($LASTEXITCODE -ne 0 -or -not $imageID) { throw "Source-built MinIO image was not created." }
    $createdImage = $true

    Set-Content -LiteralPath $environmentFile -Value @(
        "MINIO_ROOT_USER=$accessKey",
        "MINIO_ROOT_PASSWORD=$secretKey"
    ) -Encoding ascii
    & docker run --detach --rm --name $containerName --env-file $environmentFile --publish "127.0.0.1:19002:9000" $image server /data --console-address ":9001" | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "Could not start the temporary MinIO container." }
    $createdContainer = $true
    Remove-Item -LiteralPath $environmentFile -Force

    $healthy = $false
    for ($attempt = 0; $attempt -lt 60; $attempt++) {
        # Health probes target our own loopback container, never the user's proxy.
        $healthStatus = & curl.exe --noproxy "*" --fail --silent --show-error --max-time 2 --output NUL --write-out "%{http_code}" ($endpoint + "/minio/health/live") 2>$null
        $healthExit = $LASTEXITCODE
        if ($healthExit -eq 0 -and $healthStatus -eq "200") { $healthy = $true; break }
        Start-Sleep -Seconds 1
    }
    if (-not $healthy) { throw "Temporary MinIO did not become healthy within 60 seconds." }

    [Environment]::SetEnvironmentVariable("MINIO_ENDPOINT", $endpoint, "Process")
    [Environment]::SetEnvironmentVariable("MINIO_BUCKET", $bucket, "Process")
    [Environment]::SetEnvironmentVariable("MINIO_ACCESS_KEY", $accessKey, "Process")
    [Environment]::SetEnvironmentVariable("MINIO_SECRET_KEY", $secretKey, "Process")
    [Environment]::SetEnvironmentVariable("MINIO_CONTAINER", $containerName, "Process")
    # Honor the go.mod toolchain directive even when the workstation's Go
    # installation is older and has GOTOOLCHAIN=local configured globally.
    [Environment]::SetEnvironmentVariable("GOTOOLCHAIN", "auto", "Process")
    if (-not [Environment]::GetEnvironmentVariable("TEST_DATABASE_URL", "Process")) {
        [Environment]::SetEnvironmentVariable("TEST_DATABASE_URL", "postgres://postgres:postgres@127.0.0.1:5433/postgres?sslmode=disable", "Process")
    }
    if (-not [Environment]::GetEnvironmentVariable("PG_BACKUP_CONTAINER", "Process")) {
        [Environment]::SetEnvironmentVariable("PG_BACKUP_CONTAINER", "tasklab-test-postgres", "Process")
    }

    $powershell = (Get-Command powershell.exe -ErrorAction Stop).Source
    $collector = Join-Path $repoRoot "scripts\evidence\collect-wave2.ps1"
    $arguments = @("-NoProfile", "-ExecutionPolicy", "Bypass", "-File", $collector)
    $arguments += @("-RunID", $RunID)
    $process = Start-Process -FilePath $powershell -ArgumentList $arguments -WorkingDirectory $repoRoot -WindowStyle Hidden -Wait -PassThru -RedirectStandardOutput $collectorOutput
    Get-Content -LiteralPath $collectorOutput
    if ($process.ExitCode -ne 0) { throw "Wave 2 evidence collector exited with code $($process.ExitCode)." }
} finally {
    if (Test-Path -LiteralPath $environmentFile) { Remove-Item -LiteralPath $environmentFile -Force }
    if ($createdContainer) {
        $runningContainer = & docker ps -q --filter "name=^$containerName$"
        if ($runningContainer) { & docker stop $containerName | Out-Null }
    }
    foreach ($name in $names) { [Environment]::SetEnvironmentVariable($name, $previous[$name], "Process") }
    if ($createdImage) { & docker image rm $image | Out-Null }
    if (Test-Path -LiteralPath $collectorOutput) { Remove-Item -LiteralPath $collectorOutput -Force }
    if (Test-Path -LiteralPath $environmentFile) { Remove-Item -LiteralPath $environmentFile -Force }
    $resolvedTemporary = (Resolve-Path -LiteralPath $temporaryDirectory -ErrorAction SilentlyContinue).Path
    $resolvedTemporaryRoot = (Resolve-Path -LiteralPath $temporaryRoot).Path
    if ($resolvedTemporary -and
        $resolvedTemporary.StartsWith($resolvedTemporaryRoot, [StringComparison]::OrdinalIgnoreCase) -and
        (Split-Path $resolvedTemporary -Leaf).StartsWith("tasklab-evidence-", [StringComparison]::OrdinalIgnoreCase)) {
        Remove-Item -LiteralPath $resolvedTemporary -Recurse -Force -ErrorAction SilentlyContinue
    }
}
