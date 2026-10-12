[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]$ComposeEnvFile,
    [Parameter(Mandatory)][string]$ComposeProject,
    [Parameter(Mandatory)][ValidatePattern('^[a-z0-9-]{8,80}$')][string]$RunId,
    [Parameter(Mandatory)][string]$OutputDirectory,
    [switch]$Execute
)

$ErrorActionPreference = 'Stop'
$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
$composeFile = Join-Path $repoRoot 'deployments/local-first/compose.core.yml'
$envPath = [System.IO.Path]::GetFullPath($ComposeEnvFile)
$output = [System.IO.Path]::GetFullPath($OutputDirectory)
if (-not $repoRoot.StartsWith('E:\',[System.StringComparison]::OrdinalIgnoreCase)) { throw 'Backup runner is restricted to the E: worktree.' }
if (-not $output.StartsWith('E:\',[System.StringComparison]::OrdinalIgnoreCase)) { throw 'Backup output is restricted to E:.' }
if ($ComposeProject -notmatch '^tasklab-local-first-[0-9]{8}t[0-9]{6}[pm][0-9]{4}$') { throw 'Compose project is not a unique local-first run ID.' }
if (-not (Test-Path -LiteralPath $envPath -PathType Leaf)) { throw 'Compose env file is missing.' }
if (Test-Path -LiteralPath $output) { throw 'Refusing to overwrite an existing backup directory.' }
$parent = Split-Path -Parent $output
if (Test-Path -LiteralPath $parent -PathType Leaf) { throw 'Backup parent path is a file; refusing to continue.' }
$expectedLeaf = "pgdump-$RunId"
if ((Split-Path -Leaf $output) -ne $expectedLeaf) { throw 'Output directory must be named pgdump-<RunId>.' }

$plan = [ordered]@{
    status = if ($Execute) { 'REQUESTED' } else { 'PLAN_ONLY' }
    run_id = $RunId
    source = 'owned Compose postgres service / database tasklab'
    project = $ComposeProject
    output = $output
    command = 'pg_dump --format=custom --no-owner --no-acl (password read inside the owned container from its mounted secret file)'
    executed = $false
    deletes_or_overwrites = $false
}
if (-not $Execute) { $plan | ConvertTo-Json -Depth 4; exit 0 }

$resourceOutput = & (Join-Path $PSScriptRoot 'check-resources.ps1') -Workload container -ReportOnly 2>$null
try { $resource = ($resourceOutput -join "`n") | ConvertFrom-Json } catch { throw 'Resource preflight did not return valid JSON.' }
if (-not $resource.workload_allowed) { throw 'Refusing database backup: container workload resource gate is not satisfied.' }
$owned = @(& docker ps -a --filter "label=com.docker.compose.project=$ComposeProject" --filter 'label=com.docker.compose.service=postgres' --format '{{.ID}}|{{.Label "com.docker.compose.project"}}|{{.Label "com.docker.compose.service"}}')
if ($LASTEXITCODE -ne 0 -or $owned.Count -ne 1 -or $owned[0] -notmatch "\|$([regex]::Escape($ComposeProject))\|postgres$") {
    throw 'Refusing backup: exactly one PostgreSQL container with matching Compose project/service labels is required.'
}

New-Item -ItemType Directory -Path $parent -Force | Out-Null
New-Item -ItemType Directory -Path $output | Out-Null
$identity = [System.Security.Principal.WindowsIdentity]::GetCurrent().Name
& icacls.exe $output /inheritance:r /grant:r "${identity}:(OI)(CI)F" | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Could not restrict backup directory ACL.' }
$dumpPath = Join-Path $output 'tasklab.dump'
$logPath = Join-Path $output 'pg_dump.stderr.log'
$manifestPath = Join-Path $output 'manifest.json'
$dumpFile = [System.IO.File]::Open($dumpPath,[System.IO.FileMode]::CreateNew,[System.IO.FileAccess]::Write,[System.IO.FileShare]::None)
$process = [System.Diagnostics.Process]::new()
$process.StartInfo = [System.Diagnostics.ProcessStartInfo]::new()
$argumentListProperty = $process.StartInfo.GetType().GetProperty('ArgumentList')
if (-not $argumentListProperty) { $dumpFile.Dispose(); throw 'PowerShell/.NET does not support safe process argument-list construction.' }
$process.StartInfo.FileName = (Get-Command docker -ErrorAction Stop).Source
$process.StartInfo.UseShellExecute = $false
$process.StartInfo.CreateNoWindow = $true
$process.StartInfo.RedirectStandardOutput = $true
$process.StartInfo.RedirectStandardError = $true
foreach ($argument in @('compose','--project-name',$ComposeProject,'--env-file',$envPath,'-f',$composeFile,'exec','-T','postgres','sh','-c','export PGPASSWORD="$(cat /run/secrets/postgres_superuser_password)"; exec pg_dump --host=127.0.0.1 --username=postgres --dbname=tasklab --format=custom --no-owner --no-acl')) {
    $process.StartInfo.ArgumentList.Add($argument)
}
$null = $process.Start()
$copyTask = $process.StandardOutput.BaseStream.CopyToAsync($dumpFile)
$stderrTask = $process.StandardError.ReadToEndAsync()
$process.WaitForExit()
$copyTask.GetAwaiter().GetResult()
$stderr = $stderrTask.GetAwaiter().GetResult()
$dumpFile.Flush($true)
$dumpFile.Dispose()
[System.IO.File]::WriteAllText($logPath,$stderr,[System.Text.UTF8Encoding]::new($false))
if ($process.ExitCode -ne 0) { throw "pg_dump failed with exit code $($process.ExitCode); preserve this run directory for diagnosis." }
$stream = [System.IO.File]::OpenRead($dumpPath)
try { $signature = [byte[]]::new(5); $read = $stream.Read($signature,0,5) } finally { $stream.Dispose() }
if ($read -ne 5 -or [System.Text.Encoding]::ASCII.GetString($signature) -ne 'PGDMP') { throw 'Backup output does not have the PostgreSQL custom-dump signature.' }
$commit = (& git -C $repoRoot rev-parse HEAD).Trim()
$tree = (& git -C $repoRoot rev-parse 'HEAD^{tree}').Trim()
$manifest = [ordered]@{
    run_id=$RunId; status='PASS'; source_commit=$commit; source_tree=$tree
    source_project=$ComposeProject; source_database='tasklab'; dump_file='tasklab.dump'
    dump_bytes=(Get-Item -LiteralPath $dumpPath).Length
    dump_sha256=(Get-FileHash -Algorithm SHA256 -LiteralPath $dumpPath).Hash.ToLowerInvariant()
    stderr_log='pg_dump.stderr.log'; log_closed_before_hash=$true
    consistency_limit='PostgreSQL-only point-in-time dump; not atomic with MinIO objects or external effects.'
}
[System.IO.File]::WriteAllText($manifestPath,($manifest|ConvertTo-Json -Depth 5),[System.Text.UTF8Encoding]::new($false))
$files=@((Get-FileHash -Algorithm SHA256 -LiteralPath $dumpPath,$logPath,$manifestPath)|ForEach-Object{'{0}  {1}'-f $_.Hash.ToLowerInvariant(),[System.IO.Path]::GetFileName($_.Path)})
[System.IO.File]::WriteAllText((Join-Path $output 'SHA256SUMS'),($files -join "`n")+"`n",[System.Text.UTF8Encoding]::new($false))
$plan.status='PASS'; $plan.executed=$true; $plan.dump_bytes=$manifest.dump_bytes; $plan.dump_sha256=$manifest.dump_sha256
$plan | ConvertTo-Json -Depth 4
