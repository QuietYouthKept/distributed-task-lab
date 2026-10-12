[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]$ComposeEnvFile,
    [Parameter(Mandatory)][string]$ComposeProject,
    [switch]$RequireImages
)

$ErrorActionPreference = 'Continue'
$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
$envPath = [System.IO.Path]::GetFullPath($ComposeEnvFile)
$composePath = Join-Path $repoRoot 'deployments/local-first/compose.core.yml'
$results = [System.Collections.Generic.List[object]]::new()
$failures = [System.Collections.Generic.List[string]]::new()
function Add-Check([string]$Name, [bool]$Passed, [string]$Detail) {
    $results.Add([pscustomobject]@{ name = $Name; status = $(if ($Passed) { 'PASS' } else { 'FAIL' }); detail = $Detail })
    if (-not $Passed) { $failures.Add($Name) }
}

try {
    $top = (& git -C $repoRoot rev-parse --show-toplevel 2>$null | Select-Object -First 1)
    $topOK = ([string]::IsNullOrWhiteSpace($top) -eq $false -and [System.IO.Path]::GetFullPath($top).TrimEnd('\') -ieq $repoRoot.TrimEnd('\') -and $repoRoot.StartsWith('E:\', [System.StringComparison]::OrdinalIgnoreCase))
    Add-Check 'worktree_identity' $topOK "root=$repoRoot"
} catch { Add-Check 'worktree_identity' $false 'git worktree identity unavailable' }

$envMap = @{}
if (Test-Path -LiteralPath $envPath -PathType Leaf) {
    foreach ($line in Get-Content -LiteralPath $envPath) {
        if ($line -match '^\s*([A-Za-z_][A-Za-z0-9_]*)=(.*)$') { $envMap[$Matches[1]] = $Matches[2].Trim().Trim('"').Trim("'") }
    }
}
Add-Check 'compose_env_file' ((Test-Path -LiteralPath $envPath -PathType Leaf) -and $envMap.Count -gt 0) 'env file exists and has key/value entries; values are not emitted'
Add-Check 'compose_project_run_scoped' ($ComposeProject -match '^tasklab-local-first-[0-9]{8}t[0-9]{6}[pm][0-9]{4}$' -and $envMap.COMPOSE_PROJECT_NAME -eq $ComposeProject) "project=$ComposeProject"
Add-Check 'compose_file' (Test-Path -LiteralPath $composePath -PathType Leaf) 'core compose file exists'
Add-Check 'e_drive' (Test-Path -LiteralPath 'E:\') 'E: mount is available'
$networkName=[string]$envMap.TASKLAB_NETWORK_NAME
$pgVolume=[string]$envMap.TASKLAB_POSTGRES_VOLUME
$s3Volume=[string]$envMap.TASKLAB_MINIO_VOLUME
$namesScoped=($networkName -eq "$ComposeProject-core" -and $pgVolume -eq "$ComposeProject-postgres" -and $s3Volume -eq "$ComposeProject-minio")
Add-Check 'network_and_volume_run_scope' $namesScoped 'network and both named volumes must be unique to this Compose run'
$resourcesAbsent=$false
if($namesScoped -and (Get-Command docker -ErrorAction SilentlyContinue)){
    $networkOutput=& docker network inspect $networkName 2>$null
    $networkMissing=$LASTEXITCODE -ne 0
    $null=& docker volume inspect $pgVolume 2>$null; $pgMissing=$LASTEXITCODE -ne 0
    $null=& docker volume inspect $s3Volume 2>$null; $s3Missing=$LASTEXITCODE -ne 0
    $resourcesAbsent=$networkMissing -and $pgMissing -and $s3Missing
}
Add-Check 'no_preexisting_owned_resource_collision' $resourcesAbsent 'refuses names already present; does not remove resources'

$requiredSecrets = @('postgres-superuser-password','tasklab-migrator-password','tasklab-api-password','tasklab-worker-a-password','migrator-database-url','api-database-url','worker-a-database-url','minio-root-user','minio-root-password','artifact-s3-access-key','artifact-s3-secret-key','gateway-api-key','api-cursor-signing-key')
$secretRoot = $envMap.TASKLAB_SECRET_DIR -replace '/', '\'
$missingSecrets = @($requiredSecrets | Where-Object { -not (Test-Path -LiteralPath (Join-Path $secretRoot $_) -PathType Leaf) })
Add-Check 'required_secret_files' ($secretRoot.StartsWith('E:\', [System.StringComparison]::OrdinalIgnoreCase) -and $missingSecrets.Count -eq 0) "missing_count=$($missingSecrets.Count); secret values omitted"

$tlsPairs = @(
    @{ Name='identity_tls'; Dir=($envMap.TASKLAB_IDENTITY_TLS_DIR -replace '/', '\'); Cert='identity.crt'; Key='identity.key'; CA=(Join-Path $secretRoot 'tls\test-ca.crt'); SAN='DNS:identity' },
    @{ Name='api_tls'; Dir=($envMap.TASKLAB_API_TLS_DIR -replace '/', '\'); Cert='api.crt'; Key='api.key'; CA=(Join-Path $secretRoot 'tls\test-ca.crt'); SAN='DNS:localhost' }
)
$openssl = Get-Command openssl -ErrorAction SilentlyContinue
foreach ($pair in $tlsPairs) {
    $cert = Join-Path $pair.Dir $pair.Cert
    $key = Join-Path $pair.Dir $pair.Key
    $present = (Test-Path -LiteralPath $cert -PathType Leaf) -and (Test-Path -LiteralPath $key -PathType Leaf) -and (Test-Path -LiteralPath $pair.CA -PathType Leaf)
    $valid = $false
    if ($present -and $openssl) {
        & $openssl.Source x509 -in $cert -noout -checkend 86400 *> $null
        $notExpiring = $LASTEXITCODE -eq 0
        $sanText = (& $openssl.Source x509 -in $cert -noout -ext subjectAltName 2>$null | Out-String)
        & $openssl.Source verify -CAfile $pair.CA $cert *> $null
        $trusted = $LASTEXITCODE -eq 0
        $sanOK = $sanText.Contains($pair.SAN)
        $valid = $notExpiring -and $trusted -and $sanOK
    }
    Add-Check $pair.Name $valid "present=$present openssl_available=$([bool]$openssl); expiry, SAN, CA checked when available"
}

$proxy = @($env:HTTPS_PROXY, $env:HTTP_PROXY, $env:ALL_PROXY) -join ' '
$noProxy = @($env:NO_PROXY, $env:no_proxy) -join ','
$proxyPresent = $proxy -match '\S'
$proxyConfigured = (-not $proxyPresent) -or ($proxy -match '127\.0\.0\.1:7890')
$proxyBypass = (-not $proxyPresent) -or (($noProxy -match 'localhost') -and ($noProxy -match '127\.0\.0\.1'))
Add-Check 'proxy_bypass' ($proxyConfigured -and $proxyBypass) $(if($proxyPresent){'configured proxy must use 127.0.0.1:7890 and bypass localhost/127.0.0.1; values and credentials are not emitted'}else{'no proxy environment configured; local service traffic uses direct routing'})

$resourceOutput = & (Join-Path $PSScriptRoot 'check-resources.ps1') -Workload container -ReportOnly 2>$null
$resource = $null
try { $resource = ($resourceOutput -join "`n") | ConvertFrom-Json } catch { }
$resourceOK = ($null -ne $resource -and $resource.workload_allowed -eq $true)
Add-Check 'resource_budget' $resourceOK $(if ($resource) { "ram_gib=$($resource.available_ram_gib); time_wait=$($resource.time_wait_count); e_free_gib=$($resource.e_free_gib); cpu=$($resource.cpu_load_percent); workload_allowed=$($resource.workload_allowed)" } else { 'resource snapshot failed' })

$portChecks = @()
foreach ($portName in @('API_HOST_PORT','IDENTITY_HOST_PORT')) {
    $port = 0
    $isNumber = [int]::TryParse([string]$envMap[$portName], [ref]$port)
    $busy = $false
    if ($isNumber) { $busy = @(Get-NetTCPConnection -State Listen -LocalPort $port -ErrorAction SilentlyContinue).Count -gt 0 }
    $portChecks += ($isNumber -and -not $busy)
}
Add-Check 'host_ports_free' (($portChecks.Count -eq 2) -and -not ($portChecks -contains $false)) 'API and identity host ports checked for listener collisions'

$minioImage = [string]$envMap.MINIO_IMAGE
$minioPinned = $minioImage -match '^minio/minio:[^@]+@sha256:[a-f0-9]{64}$'
Add-Check 'minio_registry_digest' $minioPinned 'requires official repository and immutable registry digest; this check does not authenticate registry provenance'
$postgresImage = [string]$envMap.POSTGRES_IMAGE
$postgresAllowed = $postgresImage -eq 'postgres:16-alpine@sha256:721873c34ceb9f8d8fc265984940dc982404c105f19ad51be9fdc5970a6080ea'
Add-Check 'postgres_digest_allowlist' $postgresAllowed 'exactly matches the previously verified PostgreSQL index digest'

$composeConfig = $false
if ((Test-Path -LiteralPath $envPath) -and (Test-Path -LiteralPath $composePath)) {
    $null = & docker compose --project-name $ComposeProject --env-file $envPath -f $composePath config --quiet 2>&1
    $composeConfig = $LASTEXITCODE -eq 0
}
Add-Check 'compose_config' $composeConfig 'docker compose config --quiet'

$imagesAvailable = $false
if ($RequireImages -and $composeConfig) {
    $modelRaw = & docker compose --project-name $ComposeProject --env-file $envPath -f $composePath config --format json 2>$null
    if ($LASTEXITCODE -eq 0) {
        $model = ($modelRaw -join "`n") | ConvertFrom-Json
        $images = @($model.services.PSObject.Properties | ForEach-Object { $_.Value.image } | Where-Object { $_ })
        $imagesAvailable = $images.Count -gt 0
        foreach ($image in $images) {
            $null = & docker image inspect $image 2>$null
            if ($LASTEXITCODE -ne 0) { $imagesAvailable = $false }
        }
    }
    Add-Check 'images_present_locally' $imagesAvailable 'inspect only; no pulls and no local Image ID accepted as registry provenance'
} else { Add-Check 'images_present_locally' $false 'NOT RUN: RequireImages not set; this is not an image verification PASS' }

$report = [ordered]@{ status = if ($failures.Count -eq 0) { 'PASS' } else { 'FAIL' }; project = $ComposeProject; checks = @($results); failure_reasons = @($failures) }
$report | ConvertTo-Json -Depth 6
if ($failures.Count -gt 0) { exit 4 }
