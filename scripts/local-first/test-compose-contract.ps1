[CmdletBinding()]
param(
    [Parameter(Mandatory)][ValidateNotNullOrEmpty()][string]$ComposeEnvFile
)

$ErrorActionPreference = 'Stop'
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$envFile = [System.IO.Path]::GetFullPath($ComposeEnvFile)
$composeFile = Join-Path $repoRoot 'deployments/local-first/compose.core.yml'
if (-not (Test-Path -LiteralPath $envFile -PathType Leaf)) { throw 'Compose env file does not exist.' }
$docker = (Get-Command docker -ErrorAction Stop).Source
$raw = & $docker compose --env-file $envFile -f $composeFile config --format json
if ($LASTEXITCODE -ne 0) { throw 'Docker Compose could not render the local-first topology.' }
$model = ($raw -join "`n") | ConvertFrom-Json
$failures = [System.Collections.Generic.List[string]]::new()
$checks = [System.Collections.Generic.List[object]]::new()

function Assert-Contract {
    param([string]$Name, [bool]$Condition)
    $checks.Add([pscustomobject]@{ name = $Name; passed = $Condition })
    if (-not $Condition) { $failures.Add($Name) }
}

$expected = @('postgres', 'db-migrator', 'db-runtime-grants', 'minio', 'minio-init', 'gateway-simulator', 'identity', 'api', 'worker')
foreach ($serviceName in $expected) {
    Assert-Contract "service:$serviceName" ($null -ne $model.services.$serviceName)
}

$api = $model.services.api
$worker = $model.services.worker
$db = $model.services.postgres
$network = $model.networks.core
Assert-Contract 'postgres-16-immutable-official-index' ($db.image -eq 'postgres:16-alpine@sha256:721873c34ceb9f8d8fc265984940dc982404c105f19ad51be9fdc5970a6080ea')
Assert-Contract 'api-worker-separate-builds' ($api.build.dockerfile -ne $worker.build.dockerfile)
Assert-Contract 'api-staging-sim-auto-migrate-disabled' ($api.environment.APP_PROFILE -eq 'staging-sim' -and $api.environment.AUTO_MIGRATE -eq 'false')
Assert-Contract 'worker-staging-sim-auto-migrate-disabled' ($worker.environment.APP_PROFILE -eq 'staging-sim' -and $worker.environment.AUTO_MIGRATE -eq 'false')
Assert-Contract 'distinct-runtime-database-secret-files' ($api.environment.DATABASE_URL_FILE -ne $worker.environment.DATABASE_URL_FILE)
Assert-Contract 'api-has-no-static-token-auth' ($null -eq $api.environment.API_TOKEN -and $null -eq $api.environment.ADMIN_TOKEN)
Assert-Contract 'worker-only-eval-gateway-candidate' ($worker.environment.EVAL_BACKEND -eq 'gateway' -and $worker.environment.EVAL_GATEWAY_PROFILE -eq 'staging-sim')
Assert-Contract 'worker-shared-minio-artifact-store' ($worker.environment.ARTIFACT_STORE_BACKEND -eq 's3' -and $worker.environment.ARTIFACT_S3_ENDPOINT -eq 'http://minio:9000')
Assert-Contract 'worker-bucket-auto-create-disabled' ($worker.environment.ARTIFACT_ALLOW_BUCKET_CREATE -eq 'false')
Assert-Contract 'internal-private-network' ([bool]$network.internal)
Assert-Contract 'minio-runtime-healthcheck-is-direct-exec' ($model.services.minio.healthcheck.test[0] -eq 'CMD')
Assert-Contract 'minio-init-uses-same-explicit-image-source' ($model.services.'minio-init'.build.dockerfile -eq 'deployments/Dockerfile.minio-init' -and $model.services.'minio-init'.build.args.MINIO_IMAGE -eq $model.services.minio.image)
Assert-Contract 'api-waits-for-migrator-grants' ($api.depends_on.'db-runtime-grants'.condition -eq 'service_completed_successfully')
Assert-Contract 'migrator-waits-for-postgres-health' ($model.services.'db-migrator'.depends_on.postgres.condition -eq 'service_healthy')
Assert-Contract 'grants-wait-for-migration-success' ($model.services.'db-runtime-grants'.depends_on.'db-migrator'.condition -eq 'service_completed_successfully')
Assert-Contract 'identity-and-api-certs-isolated' ($model.services.identity.volumes[0].source -ne $api.volumes[0].source)

$onlyLoopback = $true
foreach ($serviceName in $model.services.PSObject.Properties.Name) {
    foreach ($port in @($model.services.$serviceName.ports)) {
        if ($null -ne $port -and $port.host_ip -ne '127.0.0.1') { $onlyLoopback = $false }
    }
}
Assert-Contract 'only-loopback-host-ports' $onlyLoopback
$sensitivePortsPresent = $false
foreach ($serviceName in @('postgres', 'minio', 'gateway-simulator')) {
    $servicePorts = @($model.services.$serviceName.ports | Where-Object { $null -ne $_ })
    if ($servicePorts.Count -gt 0) { $sensitivePortsPresent = $true }
}
Assert-Contract 'postgres-minio-gateway-unpublished' (-not $sensitivePortsPresent)

$capped = @($model.services.PSObject.Properties | Where-Object { -not $_.Value.mem_limit -or -not $_.Value.cpus })
Assert-Contract 'all-services-resource-capped' ($capped.Count -eq 0)

$roleBootstrap = Get-Content -LiteralPath (Join-Path $repoRoot 'deployments/local-first/postgres/00-create-roles.sh') -Raw
$runtimeGrantLines = @(Get-Content -LiteralPath (Join-Path $repoRoot 'deployments/local-first/postgres/runtime-grants.sql') | Where-Object { $_ -match '^\s*GRANT\b' })
$restrictedGrant = @($runtimeGrantLines | Where-Object { $_ -match '\b(ALL|DELETE|TRUNCATE|CREATE|GRANT\s+OPTION)\b' })
Assert-Contract 'three-runtime-logins-explicitly-nonsuperuser' ($roleBootstrap -match 'tasklab_migrator LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION' -and $roleBootstrap -match 'tasklab_api LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION' -and $roleBootstrap -match 'tasklab_worker LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION')
Assert-Contract 'runtime-grants-have-no-delete-or-ddl' ($runtimeGrantLines.Count -gt 0 -and $restrictedGrant.Count -eq 0)

$checks | ForEach-Object { '{0}: {1}' -f $_.name, $(if ($_.passed) { 'PASS' } else { 'FAIL' }) }
if ($failures.Count -gt 0) {
    throw "Compose contract failures: $($failures -join ', ')"
}
