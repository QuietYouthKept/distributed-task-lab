[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]$TestDatabaseURL,
    [string]$EvidenceRoot = $(Join-Path $(if (-not [string]::IsNullOrWhiteSpace($env:TASKLAB_ARTIFACT_ROOT)) { $env:TASKLAB_ARTIFACT_ROOT } else { Join-Path ([System.IO.Path]::GetTempPath()) 'distributed-task-lab' }) 'production-readiness\evidence\database-tests')
)
if ([string]::IsNullOrWhiteSpace($env:TASKLAB_ARTIFACT_ROOT)) { $env:TASKLAB_ARTIFACT_ROOT = Join-Path ([System.IO.Path]::GetTempPath()) 'distributed-task-lab' }
$TaskLabArtifactRoot = [System.IO.Path]::GetFullPath($env:TASKLAB_ARTIFACT_ROOT)


$ErrorActionPreference = 'Stop'
$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
Import-Module (Join-Path $PSScriptRoot 'database-safety.psm1') -Force -ErrorAction Stop
$databaseTarget = Assert-DisposablePostgresURL -ConnectionString $TestDatabaseURL
$databaseName = $databaseTarget.database_name
$evidenceRoot = [System.IO.Path]::GetFullPath($EvidenceRoot)
if (-not $evidenceRoot.StartsWith('E:\', [System.StringComparison]::OrdinalIgnoreCase)) { throw 'Evidence must be stored under E:\.' }
$runId = [DateTimeOffset]::Now.ToString('yyyyMMddTHHmmssfffZ')
$runDir = Join-Path $evidenceRoot "privileges-$runId"
New-Item -ItemType Directory -Path $runDir -Force | Out-Null
$logPath = Join-Path $runDir 'go-test.log'
$toolchain = Resolve-ModuleGoToolchain -RepoRoot $repoRoot
$go = $toolchain.path
$previousDatabaseURL = [Environment]::GetEnvironmentVariable('TEST_DATABASE_URL','Process')
$previousRequireDatabase = [Environment]::GetEnvironmentVariable('REQUIRE_TEST_DATABASE','Process')
$previousGoToolchain = [Environment]::GetEnvironmentVariable('GOTOOLCHAIN','Process')
$env:TEST_DATABASE_URL = $TestDatabaseURL
$env:REQUIRE_TEST_DATABASE = '1'
$env:GOTOOLCHAIN = 'local'

Push-Location $repoRoot
try {
    $command = '"' + $go + '" test ./internal/storage/postgres -run ''^TestRuntimeRoleLeastPrivilege$'' -count=1 -v'
    "COMMAND=$command" | Set-Content -LiteralPath $logPath -Encoding utf8
    $testOutput = & $go test ./internal/storage/postgres -run '^TestRuntimeRoleLeastPrivilege$' -count=1 -v 2>&1
    $exitCode = $LASTEXITCODE
    $safeOutput = Protect-DatabaseTestLog -Text ($testOutput -join "`n") -ConnectionString $TestDatabaseURL
    [System.IO.File]::AppendAllText($logPath,$safeOutput + "`n",[System.Text.UTF8Encoding]::new($false))
} finally {
    Pop-Location
    [Environment]::SetEnvironmentVariable('TEST_DATABASE_URL',$previousDatabaseURL,'Process')
    [Environment]::SetEnvironmentVariable('REQUIRE_TEST_DATABASE',$previousRequireDatabase,'Process')
    [Environment]::SetEnvironmentVariable('GOTOOLCHAIN',$previousGoToolchain,'Process')
}
$manifest = [ordered]@{
    status = if ($exitCode -eq 0) { 'PASS' } else { 'FAIL' }
    level = 'postgres-integration'
    run_id = $runId
    database_name = $databaseName
    database_host = $databaseTarget.host
    database_port = $databaseTarget.port
    maintenance_role = $databaseTarget.maintenance_role
    go_tool_version = $toolchain.version
    command = $command
    exit_code = $exitCode
    log = 'go-test.log'
    note = 'A generated temporary database and generated role are used; current privilege checks prove no DDL, DELETE, TRUNCATE, role creation, or audit/event rewriting. Column-level fencing isolation is not implemented.'
}
$manifestPath = Join-Path $runDir 'manifest.json'
$manifest | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath $manifestPath -Encoding utf8
Get-FileHash -Algorithm SHA256 -LiteralPath $logPath, $manifestPath | ForEach-Object { '{0}  {1}' -f $_.Hash.ToLowerInvariant(), [System.IO.Path]::GetFileName($_.Path) } | Set-Content -LiteralPath (Join-Path $runDir 'SHA256SUMS') -Encoding ascii
Write-Output "Evidence directory: $runDir"
if ($exitCode -ne 0) { exit $exitCode }
