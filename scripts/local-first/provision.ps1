[CmdletBinding()]
param(
    [string]$SecretRoot = $(Join-Path $(if (-not [string]::IsNullOrWhiteSpace($env:TASKLAB_ARTIFACT_ROOT)) { $env:TASKLAB_ARTIFACT_ROOT } else { Join-Path ([System.IO.Path]::GetTempPath()) 'distributed-task-lab' }) 'production-readiness\secrets'),
    [string]$OpenSSL = 'openssl'
)
if ([string]::IsNullOrWhiteSpace($env:TASKLAB_ARTIFACT_ROOT)) { $env:TASKLAB_ARTIFACT_ROOT = Join-Path ([System.IO.Path]::GetTempPath()) 'distributed-task-lab' }
$TaskLabArtifactRoot = [System.IO.Path]::GetFullPath($env:TASKLAB_ARTIFACT_ROOT)


$ErrorActionPreference = 'Stop'
$runId = [DateTimeOffset]::Now.ToString('yyyyMMddTHHmmsszzz').Replace(':', '')
$composeRunId = $runId.Replace('+', 'p').Replace('-', 'm').ToLowerInvariant()
$runDirectory = Join-Path ([System.IO.Path]::GetFullPath($SecretRoot)) "p0-runtime-$runId"
$tlsDirectory = Join-Path $runDirectory 'tls'
$identityTLSDirectory = Join-Path $tlsDirectory 'identity'
$apiTLSDirectory = Join-Path $tlsDirectory 'api'
if (-not $runDirectory.StartsWith('E:\', [System.StringComparison]::OrdinalIgnoreCase)) {
    throw 'Secrets and local TLS keys must be provisioned under E:\.'
}
if (Test-Path -LiteralPath $runDirectory) {
    throw 'The per-run secret directory already exists; refusing to overwrite it.'
}

New-Item -ItemType Directory -Path $runDirectory -Force | Out-Null
$identity = [System.Security.Principal.WindowsIdentity]::GetCurrent().Name
& icacls.exe $runDirectory /inheritance:r /grant:r "${identity}:(OI)(CI)F" | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Could not restrict access to the per-run secret directory.' }

function New-LocalSecret {
    param([Parameter(Mandatory)][string]$Name, [int]$Length = 48)
    $alphabet = 'abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789'
    $bytes = [byte[]]::new($Length)
    [System.Security.Cryptography.RandomNumberGenerator]::Fill($bytes)
    $chars = foreach ($byte in $bytes) { $alphabet[$byte % $alphabet.Length] }
    $value = -join $chars
    $path = Join-Path $runDirectory $Name
    [System.IO.File]::WriteAllText($path, $value + "`n", [System.Text.UTF8Encoding]::new($false))
    & icacls.exe $path /inheritance:r /grant:r "${identity}:F" | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "Could not restrict ACL for generated secret file $Name." }
    return $value
}

$null = New-LocalSecret 'postgres-superuser-password'
$migratorPassword = New-LocalSecret 'tasklab-migrator-password'
$apiPassword = New-LocalSecret 'tasklab-api-password'
$workerAPassword = New-LocalSecret 'tasklab-worker-a-password'
$null = New-LocalSecret 'minio-root-user' 18
$null = New-LocalSecret 'minio-root-password'
$null = New-LocalSecret 'artifact-s3-access-key' 20
$null = New-LocalSecret 'artifact-s3-secret-key'
$null = New-LocalSecret 'gateway-api-key'
$null = New-LocalSecret 'api-cursor-signing-key'

$database = 'tasklab'
$migratorUrl = "postgres://tasklab_migrator:$migratorPassword@postgres:5432/$database?sslmode=disable"
$apiUrl = "postgres://tasklab_api:$apiPassword@postgres:5432/$database?sslmode=disable"
$workerAUrl = "postgres://tasklab_worker_a:$workerAPassword@postgres:5432/$database?sslmode=disable"
foreach ($entry in @(
    @{ Name = 'migrator-database-url'; Value = $migratorUrl },
    @{ Name = 'api-database-url'; Value = $apiUrl },
    @{ Name = 'worker-a-database-url'; Value = $workerAUrl }
)) {
    $path = Join-Path $runDirectory $entry.Name
    [System.IO.File]::WriteAllText($path, $entry.Value + "`n", [System.Text.UTF8Encoding]::new($false))
    & icacls.exe $path /inheritance:r /grant:r "${identity}:F" | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "Could not restrict ACL for generated connection file $($entry.Name)." }
}

& (Join-Path $PSScriptRoot 'new-test-certs.ps1') -OutputDirectory $tlsDirectory -OpenSSL $OpenSSL
New-Item -ItemType Directory -Path $identityTLSDirectory, $apiTLSDirectory | Out-Null
Copy-Item -LiteralPath (Join-Path $tlsDirectory 'identity.crt'), (Join-Path $tlsDirectory 'identity.key') -Destination $identityTLSDirectory
Copy-Item -LiteralPath (Join-Path $tlsDirectory 'api.crt'), (Join-Path $tlsDirectory 'api.key'), (Join-Path $tlsDirectory 'test-ca.crt') -Destination $apiTLSDirectory
foreach ($bundle in @($identityTLSDirectory, $apiTLSDirectory)) {
    & icacls.exe $bundle /inheritance:r /grant:r "${identity}:(OI)(CI)F" | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Could not restrict ACL for a service-specific test certificate bundle.' }
}

$envFile = Join-Path $runDirectory 'compose.env'
$envText = @(
    "COMPOSE_PROJECT_NAME=tasklab-local-first-$composeRunId",
    "TASKLAB_NETWORK_NAME=tasklab-local-first-$composeRunId-core",
    "TASKLAB_POSTGRES_VOLUME=tasklab-local-first-$composeRunId-postgres",
    "TASKLAB_MINIO_VOLUME=tasklab-local-first-$composeRunId-minio",
    "TASKLAB_SECRET_DIR=$($runDirectory -replace '\\', '/')",
    "TASKLAB_IDENTITY_TLS_DIR=$($identityTLSDirectory -replace '\\', '/')",
    "TASKLAB_API_TLS_DIR=$($apiTLSDirectory -replace '\\', '/')",
    'POSTGRES_IMAGE=postgres:16-alpine@sha256:721873c34ceb9f8d8fc265984940dc982404c105f19ad51be9fdc5970a6080ea',
    'MINIO_IMAGE=REPLACE_WITH_VERIFIED_OFFICIAL_MINIO_IMAGE_AND_REGISTRY_DIGEST',
    'API_HOST_PORT=18443',
    'IDENTITY_HOST_PORT=18444'
) -join "`n"
[System.IO.File]::WriteAllText($envFile, $envText + "`n", [System.Text.UTF8Encoding]::new($false))
& icacls.exe $envFile /inheritance:r /grant:r "${identity}:F" | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Could not restrict ACL for the Compose environment file.' }

Write-Output "Provisioned isolated local credentials and short-lived TLS files under $runDirectory."
Write-Output "Compose env file: $envFile"
Write-Output 'Secrets are not printed. Verify the MinIO official image registry digest before Compose validation or runtime use.'
