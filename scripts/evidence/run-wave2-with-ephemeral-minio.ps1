[CmdletBinding()]
param([string]$RunID = "")

$ErrorActionPreference = "Stop"
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
$containerName = "tasklab-wave2-evidence-minio"
$image = "minio/minio@sha256:1dce27c494a16bae114774f1cec295493f3613142713130c2d22dd5696be6ad3"
$endpoint = "http://127.0.0.1:19002"
$bucket = "tasklab-wave2-evidence"
$portInUse = Test-NetConnection 127.0.0.1 -Port 19002 -InformationLevel Quiet
if ($portInUse) { throw "Port 19002 is already in use; refusing to alter an existing service." }
if (@(& docker ps -a --filter "name=^$containerName$" --format "{{.Names}}") -contains $containerName) {
    throw "Container name $containerName already exists; refusing to alter it."
}
$imageID = & docker image inspect $image --format "{{.Id}}"
if ($LASTEXITCODE -ne 0 -or -not $imageID) { throw "Pinned MinIO image is not available locally; this script does not pull images." }

$random = [guid]::NewGuid().ToString("N")
$accessKey = $random
$secretKey = [guid]::NewGuid().ToString("N") + [guid]::NewGuid().ToString("N")
$temporaryDirectory = Join-Path ([IO.Path]::GetTempPath()) ("tasklab-evidence-" + [guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $temporaryDirectory | Out-Null
$environmentFile = Join-Path $temporaryDirectory "minio.env"
$collectorOutput = Join-Path $temporaryDirectory "collector.stdout.log"
$createdContainer = $false
$names = @("MINIO_ENDPOINT", "MINIO_BUCKET", "MINIO_ACCESS_KEY", "MINIO_SECRET_KEY", "MINIO_CONTAINER", "REQUIRE_TEST_DATABASE", "TEST_DATABASE_URL")
$previous = @{}
foreach ($name in $names) { $previous[$name] = [Environment]::GetEnvironmentVariable($name, "Process") }

try {
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
        try {
            $response = Invoke-WebRequest -Uri ($endpoint + "/minio/health/live") -TimeoutSec 2 -UseBasicParsing
            if ($response.StatusCode -eq 200) { $healthy = $true; break }
        } catch { }
        Start-Sleep -Seconds 1
    }
    if (-not $healthy) { throw "Temporary MinIO did not become healthy within 60 seconds." }

    [Environment]::SetEnvironmentVariable("MINIO_ENDPOINT", $endpoint, "Process")
    [Environment]::SetEnvironmentVariable("MINIO_BUCKET", $bucket, "Process")
    [Environment]::SetEnvironmentVariable("MINIO_ACCESS_KEY", $accessKey, "Process")
    [Environment]::SetEnvironmentVariable("MINIO_SECRET_KEY", $secretKey, "Process")
    [Environment]::SetEnvironmentVariable("MINIO_CONTAINER", $containerName, "Process")
    if (-not [Environment]::GetEnvironmentVariable("TEST_DATABASE_URL", "Process")) {
        [Environment]::SetEnvironmentVariable("TEST_DATABASE_URL", "postgres://postgres:postgres@127.0.0.1:5433/postgres?sslmode=disable", "Process")
    }

    $powershell = (Get-Command powershell.exe -ErrorAction Stop).Source
    $collector = Join-Path $repoRoot "scripts\evidence\collect-wave2.ps1"
    $arguments = @("-NoProfile", "-ExecutionPolicy", "Bypass", "-File", $collector)
    if ($RunID) { $arguments += @("-RunID", $RunID) }
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
    if (Test-Path -LiteralPath $collectorOutput) { Remove-Item -LiteralPath $collectorOutput -Force }
    if (Test-Path -LiteralPath $environmentFile) { Remove-Item -LiteralPath $environmentFile -Force }
    if (Test-Path -LiteralPath $temporaryDirectory) { Remove-Item -LiteralPath $temporaryDirectory -Force -ErrorAction SilentlyContinue }
}
