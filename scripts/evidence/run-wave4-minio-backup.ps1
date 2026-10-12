[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$MinIOImage,
    [Parameter(Mandatory = $true)][string]$EvidenceDir
)
if ([string]::IsNullOrWhiteSpace($env:TASKLAB_ARTIFACT_ROOT)) { $env:TASKLAB_ARTIFACT_ROOT = Join-Path ([System.IO.Path]::GetTempPath()) 'distributed-task-lab' }
$TaskLabArtifactRoot = [System.IO.Path]::GetFullPath($env:TASKLAB_ARTIFACT_ROOT)


$ErrorActionPreference = 'Stop'
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$EvidenceDir = [System.IO.Path]::GetFullPath($EvidenceDir)
if (-not $EvidenceDir.StartsWith($TaskLabArtifactRoot, [StringComparison]::OrdinalIgnoreCase)) {
    throw 'EvidenceDir must be inside configured TASKLAB_ARTIFACT_ROOT.'
}
if (Test-Path -LiteralPath $EvidenceDir) { throw "Evidence directory already exists: $EvidenceDir" }
New-Item -ItemType Directory -Path $EvidenceDir | Out-Null

$suffix = [Guid]::NewGuid().ToString('N').Substring(0, 10)
$minioName = "tasklab-w4-minio-$suffix"
$postgresName = "tasklab-w4-postgres-$suffix"
$createdContainers = [System.Collections.Generic.List[string]]::new()
$temporaryCredentials = Join-Path $EvidenceDir 'minio.env'
$randomBytes = [System.Security.Cryptography.RandomNumberGenerator]::GetBytes(32)
$accessKey = [Convert]::ToHexString($randomBytes[0..19]).ToLowerInvariant()
$secretKey = [Convert]::ToHexString($randomBytes).ToLowerInvariant()
$postgresPassword = [Convert]::ToHexString([System.Security.Cryptography.RandomNumberGenerator]::GetBytes(24)).ToLowerInvariant()
$envNames = @('MINIO_ENDPOINT', 'MINIO_BUCKET', 'MINIO_ACCESS_KEY', 'MINIO_SECRET_KEY', 'REQUIRE_MINIO', 'REQUIRE_TEST_DATABASE', 'TEST_DATABASE_URL', 'PG_BACKUP_CONTAINER', 'RUN_BACKUP_RESTORE_DRILL', 'GOTOOLCHAIN', 'GOCACHE', 'GOMODCACHE')
$previousEnv = @{}
foreach ($name in $envNames) { $previousEnv[$name] = [Environment]::GetEnvironmentVariable($name, 'Process') }
$results = [System.Collections.Generic.List[object]]::new()

function Invoke-LoggedTest([string]$Name, [string[]]$Arguments) {
    $logPath = Join-Path $EvidenceDir "$Name.log"
    $manifestPath = Join-Path $EvidenceDir "$Name.manifest.txt"
    "command=go $($Arguments -join ' ')`nsource_commit=$(& git -C $repoRoot rev-parse HEAD)`nstarted_utc=$([DateTime]::UtcNow.ToString('o'))" | Set-Content -LiteralPath $manifestPath
    & go @Arguments *> $logPath
    $exitCode = $LASTEXITCODE
    "exit_code=$exitCode`ncompleted_utc=$([DateTime]::UtcNow.ToString('o'))" | Add-Content -LiteralPath $manifestPath
    $results.Add([ordered]@{ name = $Name; exit_code = $exitCode })
    if ($exitCode -eq 0) { Get-Content -LiteralPath $logPath -Tail 20 }
    else { Get-Content -LiteralPath $logPath -Tail 80 }
}

try {
    @("MINIO_ROOT_USER=$accessKey", "MINIO_ROOT_PASSWORD=$secretKey") | Set-Content -LiteralPath $temporaryCredentials -Encoding ascii
    $minioID = (& docker run --detach --name $minioName --env-file $temporaryCredentials --publish '127.0.0.1::9000' $MinIOImage server /data --console-address ':9001').Trim()
    if ($LASTEXITCODE -ne 0 -or -not $minioID) { throw 'Could not start the MinIO container for this run.' }
    $createdContainers.Add($minioID)
    Remove-Item -LiteralPath $temporaryCredentials -Force

    $postgresID = (& docker run --detach --name $postgresName --env "POSTGRES_PASSWORD=$postgresPassword" --publish '127.0.0.1::5432' postgres:16-alpine).Trim()
    if ($LASTEXITCODE -ne 0 -or -not $postgresID) { throw 'Could not start the PostgreSQL container for this run.' }
    $createdContainers.Add($postgresID)

    $minioPort = ((& docker port $minioID '9000/tcp').Trim() -split ':')[-1]
    $postgresPort = ((& docker port $postgresID '5432/tcp').Trim() -split ':')[-1]
    $postgresHealthy = $false
    for ($attempt = 0; $attempt -lt 60; $attempt++) {
        & docker exec $postgresID pg_isready -U postgres -d postgres *> $null
        if ($LASTEXITCODE -eq 0) { $postgresHealthy = $true; break }
        Start-Sleep -Seconds 1
    }
    if (-not $postgresHealthy) { throw 'PostgreSQL readiness check timed out.' }
    $minioHealthy = $false
    for ($attempt = 0; $attempt -lt 60; $attempt++) {
        try {
            $response = Invoke-WebRequest -UseBasicParsing -Uri "http://127.0.0.1:$minioPort/minio/health/live" -TimeoutSec 2
            if ($response.StatusCode -eq 200) { $minioHealthy = $true; break }
        } catch { }
        Start-Sleep -Seconds 1
    }
    if (-not $minioHealthy) { throw 'MinIO readiness check timed out.' }

    $env:MINIO_ENDPOINT = "http://127.0.0.1:$minioPort"
    $env:MINIO_BUCKET = "tasklab-w4-$suffix"
    $env:MINIO_ACCESS_KEY = $accessKey
    $env:MINIO_SECRET_KEY = $secretKey
    $env:REQUIRE_MINIO = '1'
    $env:REQUIRE_TEST_DATABASE = '1'
    $env:TEST_DATABASE_URL = "postgres://postgres:$postgresPassword@127.0.0.1:$postgresPort/postgres?sslmode=disable"
    $env:PG_BACKUP_CONTAINER = $postgresID
    $env:RUN_BACKUP_RESTORE_DRILL = '1'
    $env:GOTOOLCHAIN = 'auto'
    $env:GOCACHE = $(Join-Path $TaskLabArtifactRoot 'wave4-artifacts\gocache')
    $env:GOMODCACHE = $(Join-Path $TaskLabArtifactRoot 'wave4-artifacts\gomodcache')
    $env:NO_PROXY = 'localhost,127.0.0.1,::1,host.docker.internal'
    $env:no_proxy = $env:NO_PROXY

    [ordered]@{
        run_id = $suffix
        source_commit = (& git -C $repoRoot rev-parse HEAD).Trim()
        source_tree = (& git -C $repoRoot show -s --format=%T HEAD).Trim()
        started_utc = [DateTime]::UtcNow.ToString('o')
        minio_image = $MinIOImage
        minio_image_id = (& docker image inspect $MinIOImage --format '{{.Id}}').Trim()
        minio_endpoint = $env:MINIO_ENDPOINT
        minio_access = 'random per-run, redacted'
        minio_authenticated = $true
        postgres_image = 'postgres:16-alpine'
        postgres_endpoint = "127.0.0.1:$postgresPort"
        credentials = 'random per-run; removed from environment at exit'
    } | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath (Join-Path $EvidenceDir 'runtime-manifest.json')

    Invoke-LoggedTest 'minio-s3-contract' @('test', '-race', './internal/artifacts', '-run', '^TestMinIOStoreContractAndConcurrentConditionalWrite$', '-count=1', '-v')
    Invoke-LoggedTest 'two-worker-crash-recovery' @('test', '-race', './internal/e2e', '-run', '^TestAPITwoWorkersRecoverSharedArtifactAfterAckProcessCrash$', '-count=1', '-v')
    Invoke-LoggedTest 'postgres-backup-restore' @('test', '-race', './internal/storage/postgres', '-run', '^TestDatabaseBackupRestoreDrill$', '-count=1', '-v')
    $results | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath (Join-Path $EvidenceDir 'test-results.json')
    [DateTime]::UtcNow.ToString('o') | Set-Content -LiteralPath (Join-Path $EvidenceDir 'completed_utc.txt')
}
catch {
    $_ | Out-String | Set-Content -LiteralPath (Join-Path $EvidenceDir 'failure.txt')
    throw
}
finally {
    if (Test-Path -LiteralPath $temporaryCredentials) { Remove-Item -LiteralPath $temporaryCredentials -Force }
    foreach ($containerID in $createdContainers) {
        $logs = (& docker logs $containerID 2>&1 | Out-String).Replace($accessKey, '[REDACTED]').Replace($secretKey, '[REDACTED]').Replace($postgresPassword, '[REDACTED]')
        $name = if ($containerID -eq $createdContainers[0]) { 'minio-container.log' } else { 'postgres-container.log' }
        $logs | Set-Content -LiteralPath (Join-Path $EvidenceDir $name)
        & docker rm --force $containerID *> $null
    }
    foreach ($name in $envNames) { [Environment]::SetEnvironmentVariable($name, $previousEnv[$name], 'Process') }
    if (Test-Path -LiteralPath $EvidenceDir) {
        Get-ChildItem -LiteralPath $EvidenceDir -File | Where-Object Name -ne 'SHA256SUMS.txt' | ForEach-Object {
            $hash = (Get-FileHash -Algorithm SHA256 -LiteralPath $_.FullName).Hash.ToLowerInvariant()
            "$hash  $($_.Name)"
        } | Set-Content -LiteralPath (Join-Path $EvidenceDir 'SHA256SUMS.txt')
    }
}

$failed = @($results | Where-Object { $_.exit_code -ne 0 })
if ($failed.Count -gt 0) { throw "$($failed.Count) required verification test(s) failed; see the retained logs in $EvidenceDir" }
"All required MinIO/backup tests passed. Evidence: $EvidenceDir"
