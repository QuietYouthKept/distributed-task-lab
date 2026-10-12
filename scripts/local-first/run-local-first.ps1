[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]$ComposeEnvFile,
    [Parameter(Mandatory)][string]$ComposeProject,
    [switch]$Execute,
    [switch]$KeepRunning,
    [string]$EvidenceRoot=$(Join-Path $(if (-not [string]::IsNullOrWhiteSpace($env:TASKLAB_ARTIFACT_ROOT)) { $env:TASKLAB_ARTIFACT_ROOT } else { Join-Path ([System.IO.Path]::GetTempPath()) 'distributed-task-lab' }) 'production-readiness\evidence\compose-runs')
)
if ([string]::IsNullOrWhiteSpace($env:TASKLAB_ARTIFACT_ROOT)) { $env:TASKLAB_ARTIFACT_ROOT = Join-Path ([System.IO.Path]::GetTempPath()) 'distributed-task-lab' }
$TaskLabArtifactRoot = [System.IO.Path]::GetFullPath($env:TASKLAB_ARTIFACT_ROOT)


$ErrorActionPreference='Stop'
$repoRoot=[System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
$composeFile=Join-Path $repoRoot 'deployments/local-first/compose.core.yml'
$envPath=[System.IO.Path]::GetFullPath($ComposeEnvFile)
$evidenceRoot=[System.IO.Path]::GetFullPath($EvidenceRoot)
if(-not $repoRoot.StartsWith('E:\',[System.StringComparison]::OrdinalIgnoreCase) -or -not $evidenceRoot.StartsWith('E:\',[System.StringComparison]::OrdinalIgnoreCase)){throw 'Runner and evidence must remain under E:.'}
if($ComposeProject -notmatch '^tasklab-local-first-[0-9]{8}t[0-9]{6}[pm][0-9]{4}$'){throw 'Compose project must be a unique TaskLab run ID.'}
if(-not(Test-Path -LiteralPath $envPath -PathType Leaf)){throw 'Compose env file is missing.'}
Import-Module (Join-Path $PSScriptRoot 'evidence-integrity.psm1') -Force -ErrorAction Stop

$runId=$ComposeProject.Substring('tasklab-local-first-'.Length)
$runDir=Join-Path $evidenceRoot "run-$runId"
if(Test-Path -LiteralPath $runDir){throw 'Refusing to overwrite an existing Compose run evidence directory.'}
New-Item -ItemType Directory -Path $runDir -Force|Out-Null
$identity=[System.Security.Principal.WindowsIdentity]::GetCurrent().Name
& icacls.exe $runDir /inheritance:r /grant:r "${identity}:(OI)(CI)F"|Out-Null
if($LASTEXITCODE -ne 0){throw 'Could not restrict Compose run evidence directory ACL.'}
$sourceInventoryResult=Get-ChangedSourceInventory -RepoRoot $repoRoot -GitExe (Get-Command git -ErrorAction Stop).Source
$sourceInventory=@($sourceInventoryResult.files)
$inventoryPath=Join-Path $runDir 'source-inventory.json'
[System.IO.File]::WriteAllText($inventoryPath,(ConvertTo-Json -InputObject $sourceInventory -Depth 5),[System.Text.UTF8Encoding]::new($false))
$inventoryHash=(Get-FileHash -Algorithm SHA256 -LiteralPath $inventoryPath).Hash.ToLowerInvariant()

function Write-RunEvidence{
    param([Parameter(Mandatory)][string]$Status,[Parameter(Mandatory)][System.Collections.IDictionary]$Manifest)
    $manifestPath=Join-Path $runDir 'manifest.json'
    [System.IO.File]::WriteAllText($manifestPath,($Manifest|ConvertTo-Json -Depth 10),[System.Text.UTF8Encoding]::new($false))
    $files=@(Get-ChildItem -LiteralPath $runDir -File|Where-Object{$_.Name -ne 'SHA256SUMS'})
    $sums=@(($files|Get-FileHash -Algorithm SHA256)|ForEach-Object{'{0}  {1}'-f $_.Hash.ToLowerInvariant(),[System.IO.Path]::GetFileName($_.Path)})
    [System.IO.File]::WriteAllText((Join-Path $runDir 'SHA256SUMS'),($sums -join "`n")+"`n",[System.Text.UTF8Encoding]::new($false))
    Write-Output "STATUS=$Status EVIDENCE=$runDir"
}

$phases=@(
    [pscustomobject]@{name='postgres-health';args=@('up','-d','--wait','--wait-timeout','90','postgres')},
    [pscustomobject]@{name='migration';args=@('run','--rm','db-migrator')},
    [pscustomobject]@{name='runtime-grants';args=@('run','--rm','db-runtime-grants')},
    [pscustomobject]@{name='identity-gateway-minio-health';args=@('up','-d','--wait','--wait-timeout','120','identity','gateway-simulator','minio')},
    [pscustomobject]@{name='minio-bucket-init';args=@('run','--rm','minio-init')},
    [pscustomobject]@{name='api-worker-readiness';args=@('up','-d','--wait','--wait-timeout','120','api','worker')}
)
$preflightOutput=& (Join-Path $PSScriptRoot 'preflight.ps1') -ComposeEnvFile $envPath -ComposeProject $ComposeProject -RequireImages 2>&1
$preflightExit=$LASTEXITCODE
try{$preflight=($preflightOutput -join "`n")|ConvertFrom-Json}catch{
    $preflightFailure=$preflightOutput -join "`n"
    $preflightFailure=[regex]::Replace($preflightFailure,'(?i)(authorization\s*:\s*bearer\s+)[^\s"'']+','$1[REDACTED]')
    $preflightFailure=[regex]::Replace($preflightFailure,'(?i)(postgres(?:ql)?://[^:/\s]+:)[^@\s]+(@)','$1[REDACTED]$2')
    $preflightFailure=[regex]::Replace($preflightFailure,'(?i)(password|secret|token|api[_-]?key|database[_-]?url|dsn)(\s*[=:]\s*["'']?)[^\s,"'']+','$1$2[REDACTED]')
    $errorPath=Join-Path $runDir 'preflight-error.log'
    [System.IO.File]::WriteAllText($errorPath,$preflightFailure,[System.Text.UTF8Encoding]::new($false))
    $manifest=[ordered]@{run_id=$runId;status='FAIL';project=$ComposeProject;preflight_status='INVALID_OUTPUT';failure_reasons=@('preflight_output_invalid');phases=@();executed=$false;source_commit=(& git -C $repoRoot rev-parse HEAD).Trim();source_tree=(& git -C $repoRoot rev-parse 'HEAD^{tree}').Trim();dirty_at_start=$sourceInventoryResult.dirty;source_inventory='source-inventory.json';source_inventory_sha256=$inventoryHash;error_log='preflight-error.log';log_closed_before_hash=$true}
    Write-RunEvidence 'FAIL' $manifest
    exit 1
}
$preflightPath=Join-Path $runDir 'preflight.json'
[System.IO.File]::WriteAllText($preflightPath,($preflight|ConvertTo-Json -Depth 10),[System.Text.UTF8Encoding]::new($false))
if($preflightExit -ne 0 -or $preflight.status -ne 'PASS'){
    $reasons=@($preflight.failure_reasons)
    $status=if(@($preflight.checks|Where-Object{$_.name -eq 'resource_budget' -and $_.status -eq 'FAIL'}).Count -gt 0){'RESOURCE_LIMITED'}else{'DEPENDENCY_BLOCKED'}
    $manifest=[ordered]@{run_id=$runId;status=$status;project=$ComposeProject;preflight_status=$preflight.status;failure_reasons=$reasons;phases=@();executed=$false;note='No Compose mutation occurred.';source_commit=(& git -C $repoRoot rev-parse HEAD).Trim();source_tree=(& git -C $repoRoot rev-parse 'HEAD^{tree}').Trim();dirty_at_start=$sourceInventoryResult.dirty;source_inventory='source-inventory.json';source_inventory_sha256=$inventoryHash;preflight='preflight.json';log_closed_before_hash=$true}
    Write-RunEvidence $status $manifest
    $manifest|ConvertTo-Json -Depth 10
    exit 3
}
if(-not $Execute){
    $manifest=[ordered]@{run_id=$runId;status='PLAN_ONLY';project=$ComposeProject;preflight_status='PASS';failure_reasons=@();phases=$phases;executed=$false;source_commit=(& git -C $repoRoot rev-parse HEAD).Trim();source_tree=(& git -C $repoRoot rev-parse 'HEAD^{tree}').Trim();dirty_at_start=$sourceInventoryResult.dirty;source_inventory='source-inventory.json';source_inventory_sha256=$inventoryHash;preflight='preflight.json';log_closed_before_hash=$true}
    Write-RunEvidence 'PLAN_ONLY' $manifest
    $manifest|ConvertTo-Json -Depth 10
    exit 0
}

$logPath=Join-Path $runDir 'compose-stages.log'
$stageResults=[System.Collections.Generic.List[object]]::new()
$docker=(Get-Command docker -ErrorAction Stop).Source
$failed=$false
foreach($phase in $phases){
    $output=@(& $docker compose --project-name $ComposeProject --env-file $envPath -f $composeFile @($phase.args) 2>&1)
    $code=$LASTEXITCODE
    $safeOutput=$output -join "`n"
    $safeOutput=[regex]::Replace($safeOutput,'(?i)(authorization\s*:\s*bearer\s+)[^\s"'']+','$1[REDACTED]')
    $safeOutput=[regex]::Replace($safeOutput,'(?i)(postgres(?:ql)?://[^:/\s]+:)[^@\s]+(@)','$1[REDACTED]$2')
    $safeOutput=[regex]::Replace($safeOutput,'(?i)(password|secret|token|api[_-]?key|database[_-]?url|dsn)(\s*[=:]\s*["'']?)[^\s,"'']+','$1$2[REDACTED]')
    $command="docker compose --project-name $ComposeProject --env-file [E:-secret-file] -f deployments/local-first/compose.core.yml $($phase.args -join ' ')"
    $block="PHASE=$($phase.name) EXIT=$code COMMAND=$command`n$safeOutput`n"
    [System.IO.File]::AppendAllText($logPath,$block,[System.Text.UTF8Encoding]::new($false))
    $stageResults.Add([pscustomobject]@{phase=$phase.name;exit_code=$code;status=$(if($code -eq 0){'PASS'}else{'FAIL'})})
    if($code -ne 0){$failed=$true;break}
}

$cleanupStatus='NOT_REQUESTED'
if($failed -or -not $KeepRunning){
    $cleanupOutput=@(& $docker compose --project-name $ComposeProject --env-file $envPath -f $composeFile stop 2>&1)
    $cleanupExit=$LASTEXITCODE
    $cleanupStatus=if($cleanupExit -eq 0){'STOPPED_OWNED_SERVICES'}else{'STOP_FAILED'}
    $safeCleanup=$cleanupOutput -join "`n"
    $safeCleanup=[regex]::Replace($safeCleanup,'(?i)(password|secret|token|api[_-]?key|database[_-]?url|dsn)(\s*[=:]\s*["'']?)[^\s,"'']+','$1$2[REDACTED]')
    [System.IO.File]::AppendAllText($logPath,"SCOPED_STOP_EXIT=$cleanupExit`n$safeCleanup`n",[System.Text.UTF8Encoding]::new($false))
}

$commit=(& git -C $repoRoot rev-parse HEAD).Trim();$tree=(& git -C $repoRoot rev-parse 'HEAD^{tree}').Trim()
$status=if($failed){'FAIL'}else{'IMPLEMENTED_NOT_VERIFIED'}
$manifest=[ordered]@{run_id=$runId;status=$status;source_commit=$commit;source_tree=$tree;dirty_at_start=$sourceInventoryResult.dirty;source_inventory='source-inventory.json';source_inventory_sha256=$inventoryHash;project=$ComposeProject;phases=$stageResults;cleanup=$cleanupStatus;kept_data_volumes=$true;acceptance='Startup/readiness only; this runner does not assert the API-to-Worker-to-Gateway-to-S3 fenced-ACK path.';raw_log='compose-stages.log';log_closed_before_hash=$true}
Write-RunEvidence $status $manifest
$manifest|ConvertTo-Json -Depth 8
if($failed){exit 1}
