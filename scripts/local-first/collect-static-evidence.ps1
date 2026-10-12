[CmdletBinding()]
param(
    [Parameter(Mandatory)][ValidateNotNullOrEmpty()][string]$ComposeEnvFile,
    [string]$EvidenceRoot = $(Join-Path $(if (-not [string]::IsNullOrWhiteSpace($env:TASKLAB_ARTIFACT_ROOT)) { $env:TASKLAB_ARTIFACT_ROOT } else { Join-Path ([System.IO.Path]::GetTempPath()) 'distributed-task-lab' }) 'production-readiness\evidence'),
    [string]$GoExe = $(if ($env:GO_EXE) { $env:GO_EXE } else { (Get-Command go -ErrorAction Stop).Source })
)
if ([string]::IsNullOrWhiteSpace($env:TASKLAB_ARTIFACT_ROOT)) { $env:TASKLAB_ARTIFACT_ROOT = Join-Path ([System.IO.Path]::GetTempPath()) 'distributed-task-lab' }
$TaskLabArtifactRoot = [System.IO.Path]::GetFullPath($env:TASKLAB_ARTIFACT_ROOT)


$ErrorActionPreference = 'Stop'
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$envFile = [System.IO.Path]::GetFullPath($ComposeEnvFile)
$evidencePath = [System.IO.Path]::GetFullPath($EvidenceRoot)
if (-not $evidencePath.StartsWith('E:\', [System.StringComparison]::OrdinalIgnoreCase)) {
    throw 'Evidence must be written under E:\.'
}
if (-not (Test-Path -LiteralPath $envFile -PathType Leaf)) { throw 'Compose env file does not exist.' }
if (-not (Test-Path -LiteralPath $GoExe -PathType Leaf)) { throw 'The pinned Go 1.26.9 toolchain executable was not found.' }

$runId = 'p0-runtime-static-' + [DateTimeOffset]::Now.ToString('yyyyMMddTHHmmsszzz').Replace(':', '')
$runDirectory = Join-Path $evidencePath $runId
if (Test-Path -LiteralPath $runDirectory) { throw 'Evidence run directory already exists; refusing to overwrite.' }
$logsDirectory = Join-Path $runDirectory 'logs'
New-Item -ItemType Directory -Path $logsDirectory -Force | Out-Null
$startedAt = [DateTimeOffset]::Now

function Invoke-Captured {
    param(
        [Parameter(Mandatory)][string]$Name,
        [Parameter(Mandatory)][string]$Executable,
        [Parameter(Mandatory)][string[]]$Arguments
    )
    $stdoutPath = Join-Path $logsDirectory "$Name.stdout.log"
    $stderrPath = Join-Path $logsDirectory "$Name.stderr.log"
    $startInfo = [System.Diagnostics.ProcessStartInfo]::new()
    $startInfo.FileName = $Executable
    $startInfo.WorkingDirectory = $repoRoot
    $startInfo.UseShellExecute = $false
    $startInfo.RedirectStandardOutput = $true
    $startInfo.RedirectStandardError = $true
    foreach ($argument in $Arguments) { [void]$startInfo.ArgumentList.Add($argument) }
    $process = [System.Diagnostics.Process]::Start($startInfo)
    $stdoutTask = $process.StandardOutput.ReadToEndAsync()
    $stderrTask = $process.StandardError.ReadToEndAsync()
    $process.WaitForExit()
    $stdout = $stdoutTask.GetAwaiter().GetResult()
    $stderr = $stderrTask.GetAwaiter().GetResult()
    [System.IO.File]::WriteAllText($stdoutPath, $stdout, [System.Text.UTF8Encoding]::new($false))
    [System.IO.File]::WriteAllText($stderrPath, $stderr, [System.Text.UTF8Encoding]::new($false))
    return [ordered]@{
        name = $Name
        command = @($Executable) + $Arguments
        exit_code = $process.ExitCode
        status = if ($process.ExitCode -eq 0) { 'PASS' } else { 'FAIL' }
        stdout = "logs/$Name.stdout.log"
        stderr = "logs/$Name.stderr.log"
    }
}

$docker = (Get-Command docker -ErrorAction Stop).Source
$composeResult = Invoke-Captured -Name 'compose-config' -Executable $docker -Arguments @(
    'compose', '--env-file', $envFile, '-f', 'deployments/local-first/compose.core.yml', 'config', '--quiet'
)
$envEntries = @{}
foreach ($line in Get-Content -LiteralPath $envFile) {
    if ($line -match '^([A-Z0-9_]+)=(.*)$') { $envEntries[$matches[1]] = $matches[2] }
}
$apiTLSDirectory = $envEntries['TASKLAB_API_TLS_DIR']
$identityTLSDirectory = $envEntries['TASKLAB_IDENTITY_TLS_DIR']
$tlsRoot = Split-Path -Parent $apiTLSDirectory
$openssl = (Get-Command openssl -ErrorAction Stop).Source
$tlsResults = @(
    (Invoke-Captured -Name 'tls-api-localhost' -Executable $openssl -Arguments @('verify', '-CAfile', (Join-Path $tlsRoot 'test-ca.crt'), '-verify_hostname', 'localhost', (Join-Path $apiTLSDirectory 'api.crt'))),
    (Invoke-Captured -Name 'tls-api-loopback-ip' -Executable $openssl -Arguments @('verify', '-CAfile', (Join-Path $tlsRoot 'test-ca.crt'), '-verify_ip', '127.0.0.1', (Join-Path $apiTLSDirectory 'api.crt'))),
    (Invoke-Captured -Name 'tls-identity-service-dns' -Executable $openssl -Arguments @('verify', '-CAfile', (Join-Path $tlsRoot 'test-ca.crt'), '-verify_hostname', 'identity', (Join-Path $identityTLSDirectory 'identity.crt')))
)
$env:GOTOOLCHAIN = 'local'
$env:GOPROXY = 'https://proxy.golang.org,direct'
$goResult = Invoke-Captured -Name 'targeted-go-tests' -Executable $GoExe -Arguments @(
    'test', './cmd/migrate', './cmd/oidc-fixture', './cmd/gateway-simulator',
    './internal/config', './internal/auth', './internal/domain/taskpayload', '-count=1'
)

$parseErrors = [System.Collections.Generic.List[string]]::new()
foreach ($scriptName in @('provision.ps1', 'new-test-certs.ps1', 'test-compose-contract.ps1', 'collect-static-evidence.ps1')) {
    $scriptPath = Join-Path $PSScriptRoot $scriptName
    $tokens = $null
    $errors = $null
    [System.Management.Automation.Language.Parser]::ParseFile($scriptPath, [ref]$tokens, [ref]$errors) | Out-Null
    if ($errors.Count -gt 0) { $parseErrors.AddRange(@($errors | ForEach-Object { "$scriptName`: $($_.Message)" })) }
}
$parseStatus = if ($parseErrors.Count -eq 0) { 'PASS' } else { 'FAIL' }
$parserOutput = if ($parseErrors.Count -eq 0) { 'PowerShell parser accepted all local-first provisioning, Compose contract and evidence scripts.' } else { $parseErrors -join "`n" }
[System.IO.File]::WriteAllText((Join-Path $logsDirectory 'powershell-parser.stdout.log'),
    $parserOutput,
    [System.Text.UTF8Encoding]::new($false))
[System.IO.File]::WriteAllText((Join-Path $logsDirectory 'powershell-parser.stderr.log'), '', [System.Text.UTF8Encoding]::new($false))
$parserResult = [ordered]@{ name = 'powershell-parser'; command = @('PowerShell AST parser', 'provision.ps1', 'new-test-certs.ps1', 'test-compose-contract.ps1', 'collect-static-evidence.ps1'); exit_code = if ($parseErrors.Count -eq 0) { 0 } else { 1 }; status = $parseStatus; stdout = 'logs/powershell-parser.stdout.log'; stderr = 'logs/powershell-parser.stderr.log' }
$contractResult = Invoke-Captured -Name 'compose-contract' -Executable 'powershell.exe' -Arguments @(
    '-NoProfile', '-File', (Join-Path $PSScriptRoot 'test-compose-contract.ps1'), '-ComposeEnvFile', $envFile
)
$bash = (Get-Command bash -ErrorAction Stop).Source
$shellResults = @()
foreach ($shellScript in @(
    'deployments/local-first/postgres/00-create-roles.sh',
    'deployments/local-first/postgres/apply-runtime-grants.sh',
    'deployments/local-first/minio/init-minio.sh'
)) {
    $name = 'shell-' + [System.IO.Path]::GetFileNameWithoutExtension($shellScript)
    $shellResults += Invoke-Captured -Name $name -Executable $bash -Arguments @('-n', $shellScript)
}
$gitResult = Invoke-Captured -Name 'git-diff-check' -Executable (Get-Command git -ErrorAction Stop).Source -Arguments @('-C', $repoRoot, 'diff', '--check')
$policyPath = Join-Path $repoRoot 'deployments/local-first/minio/artifact-policy.json'
$policyOk = $false
try {
    $policy = Get-Content -LiteralPath $policyPath -Raw | ConvertFrom-Json
    $policyOk = $policy.Version -eq '2012-10-17' -and @($policy.Statement).Count -eq 2
} catch { $policyOk = $false }
$policyStatus = if ($policyOk) { 'PASS' } else { 'FAIL' }
$policyOutput = if ($policyOk) { 'MinIO least-privilege artifact policy JSON parsed with two scoped statements.' } else { 'MinIO policy JSON is invalid or has an unexpected statement count.' }
[System.IO.File]::WriteAllText((Join-Path $logsDirectory 'minio-policy.stdout.log'), $policyOutput, [System.Text.UTF8Encoding]::new($false))
[System.IO.File]::WriteAllText((Join-Path $logsDirectory 'minio-policy.stderr.log'), '', [System.Text.UTF8Encoding]::new($false))
$policyResult = [ordered]@{ name = 'minio-policy-json'; command = @('PowerShell ConvertFrom-Json', 'deployments/local-first/minio/artifact-policy.json'); exit_code = if ($policyOk) { 0 } else { 1 }; status = $policyStatus; stdout = 'logs/minio-policy.stdout.log'; stderr = 'logs/minio-policy.stderr.log' }

$resourceResult = Invoke-Captured -Name 'resource-gate' -Executable 'powershell.exe' -Arguments @(
    '-NoProfile', '-File', (Join-Path $PSScriptRoot 'check-resources.ps1'), '-Workload', 'container', '-ReportOnly'
)
$resourceSnapshot = $null
$resourceStdout = Get-Content -LiteralPath (Join-Path $logsDirectory 'resource-gate.stdout.log') -Raw
try { $resourceSnapshot = $resourceStdout | ConvertFrom-Json } catch { throw 'Resource gate output was not valid JSON; evidence is incomplete.' }
$resourceResult.status = if ($resourceSnapshot.workload_allowed) { 'PASS' } else { 'RESOURCE_LIMITED' }
$resourceResult.exit_code = 0

$head = (& git -C $repoRoot rev-parse HEAD).Trim()
$tree = (& git -C $repoRoot rev-parse 'HEAD^{tree}').Trim()
$statusLines = @(& git -C $repoRoot status --porcelain=v1)
$finishedAt = [DateTimeOffset]::Now
$manifest = [ordered]@{
    schema_version = 1
    run_id = $runId
    started_at = $startedAt.ToString('o')
    finished_at = $finishedAt.ToString('o')
    source_commit = $head
    source_tree = $tree
    source_worktree_clean = ($statusLines.Count -eq 0)
    source_status = if ($statusLines.Count -eq 0) { 'clean' } else { 'dirty; source_commit and source_tree identify HEAD only, not uncommitted files' }
    scope = 'static Compose config and topology contract, shell/PowerShell syntax, certificate verification, policy JSON, targeted Go unit tests, git diff check, resource preflight only'
    container_e2e_started = $false
    minio_image_status = 'BLOCKED: no verified anonymously pullable official immutable registry digest'
    results = @($composeResult) + $tlsResults + @($goResult, $parserResult, $contractResult) + $shellResults + @($gitResult, $policyResult, $resourceResult)
    resource_snapshot = $resourceSnapshot
    production_status = 'PRODUCTION_NOT_APPROVED'
    credential_values_recorded = $false
}
$manifestPath = Join-Path $runDirectory 'manifest.json'
$manifestJson = $manifest | ConvertTo-Json -Depth 10
[System.IO.File]::WriteAllText($manifestPath, $manifestJson + "`n", [System.Text.UTF8Encoding]::new($false))

# All child processes have exited and all log files are closed before hashing.
$hashLines = [System.Collections.Generic.List[string]]::new()
Get-ChildItem -LiteralPath $runDirectory -File -Recurse | Sort-Object FullName | ForEach-Object {
    $relative = $_.FullName.Substring($runDirectory.Length + 1).Replace('\', '/')
    $hash = (Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
    $hashLines.Add("$hash  $relative")
}
[System.IO.File]::WriteAllText((Join-Path $runDirectory 'SHA256SUMS'), ($hashLines -join "`n") + "`n", [System.Text.UTF8Encoding]::new($false))
Write-Output "Evidence sealed: $runDirectory"
Write-Output "Compose config: $($composeResult.status); topology contract: $($contractResult.status); TLS verification: $(if (@($tlsResults | Where-Object status -ne 'PASS').Count -eq 0) { 'PASS' } else { 'CHECK EVIDENCE' }); Go tests: $($goResult.status); PowerShell/shell syntax: $($parserResult.status)/$(if (@($shellResults | Where-Object status -ne 'PASS').Count -eq 0) { 'PASS' } else { 'CHECK EVIDENCE' }); policy JSON: $($policyResult.status); diff check: $($gitResult.status); container gate: $($resourceResult.status)."
