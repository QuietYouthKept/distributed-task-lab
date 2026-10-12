[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]$ComposeEnvFile,
    [Parameter(Mandatory)][string]$ComposeProject,
    [Parameter(Mandatory)][ValidateSet('worker-stop','worker-kill','worker-start','gateway-stop','gateway-start','gateway-restart','gateway-pause','gateway-resume','gateway-request-failure','minio-stop','minio-start','minio-network-loss','postgres-stop','postgres-start','postgres-short-disconnect')][string]$Action,
    [string]$ConfirmRunId,
    [switch]$Apply
)

$ErrorActionPreference = 'Stop'
$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
$composeFile = Join-Path $repoRoot 'deployments/local-first/compose.core.yml'
$envPath = [System.IO.Path]::GetFullPath($ComposeEnvFile)
if (-not $repoRoot.StartsWith('E:\', [System.StringComparison]::OrdinalIgnoreCase)) { throw 'Recovery harness only operates from the E: worktree.' }
if (-not (Test-Path -LiteralPath $envPath -PathType Leaf)) { throw 'Compose env file does not exist.' }
Import-Module (Join-Path $PSScriptRoot 'compose-cleanup.psm1') -Force -ErrorAction Stop
$scopePlan = Get-ScopedComposeCleanupPlan -ComposeProject $ComposeProject -ComposeEnvFile $envPath -ComposeFile $composeFile
if ($Apply -and $ConfirmRunId -cne [string]$scopePlan.run_id) { throw 'Fault injection requires -ConfirmRunId to exactly match the Compose Run ID.' }

$service = switch -Regex ($Action) {
    '^worker-' { 'worker'; break }
    '^gateway-' { 'gateway-simulator'; break }
    '^minio-' { 'minio'; break }
    '^postgres-' { 'postgres'; break }
}
$containers = @(& docker ps -a --filter "label=com.docker.compose.project=$ComposeProject" --filter "label=com.docker.compose.service=$service" --format '{{.ID}}|{{.Label "com.docker.compose.project"}}|{{.Label "com.docker.compose.service"}}')
if ($LASTEXITCODE -ne 0) { throw 'Could not verify Docker resource ownership.' }
if ($Apply -and $containers.Count -eq 0) { throw 'Refusing action: no container with the exact project and service ownership labels exists.' }
$expectedCompose = "docker compose --project-name $ComposeProject --env-file <redacted-path> -f <core-compose>"
$actionArgs = switch ($Action) {
    'worker-stop' { @('stop','worker') }
    'worker-kill' { @('kill','worker') }
    'worker-start' { @('start','worker') }
    'gateway-stop' { @('stop','gateway-simulator') }
    'gateway-start' { @('start','gateway-simulator') }
    'gateway-restart' { @('restart','gateway-simulator') }
    'gateway-pause' { @('pause','gateway-simulator') }
    'gateway-resume' { @('unpause','gateway-simulator') }
    'gateway-request-failure' { @('stop','gateway-simulator') }
    'minio-stop' { @('stop','minio') }
    'minio-start' { @('start','minio') }
    'minio-network-loss' { @('stop','minio') }
    'postgres-stop' { @('stop','postgres') }
    'postgres-start' { @('start','postgres') }
    'postgres-short-disconnect' { @('stop','postgres') }
}
$result = [ordered]@{
    status = if ($Apply) { 'REQUESTED' } else { 'PLAN_ONLY' }
    action = $Action
    project = $ComposeProject
    service = $service
    owned_container_count = $containers.Count
    command = "$expectedCompose $($actionArgs -join ' ')"
    fault_semantics = switch ($Action) {
        'gateway-request-failure' { 'Service-stop approximation only; this does not inject an HTTP response or prove provider request semantics.' }
        'minio-network-loss' { 'Service-stop approximation only; this does not inject packet loss or isolate only the test network.' }
        'postgres-short-disconnect' { 'Service-stop approximation only; this is not a timed connection fault proxy.' }
        default { 'Exact service lifecycle action scoped to the verified Compose Run ID.' }
    }
    executed = $false
    note = 'This harness is intentionally not invoked during offline acceptance. It never removes volumes, networks, or containers.'
}
if ($Apply) {
    $resourceOutput = & (Join-Path $PSScriptRoot 'check-resources.ps1') -Workload container 2>$null
    if ($LASTEXITCODE -ne 0) { throw 'Refusing fault injection because the container resource gate is not satisfied.' }
    if (-not $containers -or @($containers | Where-Object { $_ -notmatch "\|$([regex]::Escape($ComposeProject))\|$([regex]::Escape($service))$" }).Count -gt 0) { throw 'Docker labels do not prove exact resource ownership.' }
    & docker compose --project-name $ComposeProject --env-file $envPath -f $composeFile @actionArgs
    $exitCode = $LASTEXITCODE
    $result.executed = $true
    $result.status = if ($exitCode -eq 0) { 'PASS' } else { 'FAIL' }
    $result.exit_code = $exitCode
    $result | ConvertTo-Json -Depth 5
    exit $exitCode
}
$result | ConvertTo-Json -Depth 5
