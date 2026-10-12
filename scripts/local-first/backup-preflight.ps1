[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]$DatabaseURLFile,
    [Parameter(Mandatory)][string]$ComposeProject,
    [Parameter(Mandatory)][string]$RecoveryDatabase,
    [Parameter(Mandatory)][string]$SourceBucket,
    [Parameter(Mandatory)][string]$RecoveryBucket,
    [Parameter(Mandatory)][string]$RecoveryPrefix,
    [string]$OutputRoot = $(Join-Path $(if (-not [string]::IsNullOrWhiteSpace($env:TASKLAB_ARTIFACT_ROOT)) { $env:TASKLAB_ARTIFACT_ROOT } else { Join-Path ([System.IO.Path]::GetTempPath()) 'distributed-task-lab' }) 'production-readiness\backups')
)
if ([string]::IsNullOrWhiteSpace($env:TASKLAB_ARTIFACT_ROOT)) { $env:TASKLAB_ARTIFACT_ROOT = Join-Path ([System.IO.Path]::GetTempPath()) 'distributed-task-lab' }
$TaskLabArtifactRoot = [System.IO.Path]::GetFullPath($env:TASKLAB_ARTIFACT_ROOT)


$ErrorActionPreference = 'Continue'
$checks = [System.Collections.Generic.List[object]]::new()
$failures = [System.Collections.Generic.List[string]]::new()
function Add-Check([string]$Name,[bool]$Passed,[string]$Detail) {
    $checks.Add([pscustomobject]@{ name=$Name; status=$(if($Passed){'PASS'}else{'FAIL'}); detail=$Detail })
    if(-not $Passed){$failures.Add($Name)}
}

$dbFile = [System.IO.Path]::GetFullPath($DatabaseURLFile)
$outRoot = [System.IO.Path]::GetFullPath($OutputRoot)
$dbValue = ''
if(Test-Path -LiteralPath $dbFile -PathType Leaf){$dbValue=(Get-Content -LiteralPath $dbFile -Raw).Trim()}
$uri=$null
$urlValid=[uri]::TryCreate($dbValue,[System.UriKind]::Absolute,[ref]$uri)
$safeSource=($urlValid -and $uri.Scheme -eq 'postgres' -and $uri.Host -eq 'postgres' -and $uri.Port -eq 5432 -and $uri.AbsolutePath -eq '/tasklab')
Add-Check 'source_database_identity' $safeSource 'only tasklab on the local-first postgres service is accepted; credential value is never emitted'
Add-Check 'compose_project_scope' ($ComposeProject -match '^tasklab-local-first-[0-9]{8}t[0-9]{6}[pm][0-9]{4}$') 'run-scoped local-first Compose project required'
Add-Check 'recovery_database_is_distinct' ($RecoveryDatabase -match '^tasklab_recovery_[a-z0-9_]{8,48}$' -and $RecoveryDatabase -ne 'tasklab') "target=$RecoveryDatabase"
Add-Check 'recovery_bucket_is_distinct' ($RecoveryBucket -match '^[a-z0-9][a-z0-9.-]{2,62}$' -and $RecoveryBucket -ne $SourceBucket) "source=$SourceBucket target=$RecoveryBucket"
Add-Check 'recovery_prefix_is_bounded' ($RecoveryPrefix -match '^recovery/[a-z0-9][a-z0-9-]{7,63}/$') 'requires a fresh bounded recovery/ prefix'
Add-Check 'e_drive_output' ($outRoot.StartsWith('E:\',[System.StringComparison]::OrdinalIgnoreCase)) 'output must remain on E:'
$destination = Join-Path $outRoot $RecoveryPrefix.TrimEnd('/').Replace('/','_')
Add-Check 'destination_does_not_exist' (-not (Test-Path -LiteralPath $destination)) 'refuses to overwrite an existing backup/recovery directory'
$pgDump=Get-Command pg_dump -ErrorAction SilentlyContinue
$pgRestore=Get-Command pg_restore -ErrorAction SilentlyContinue
$mc=Get-Command mc -ErrorAction SilentlyContinue
$aws=Get-Command aws -ErrorAction SilentlyContinue
Add-Check 'postgres_tools' ([bool]$pgDump -and [bool]$pgRestore) 'pg_dump and pg_restore must be installed; versions are not changed by this preflight'
Add-Check 'object_inventory_tool' ([bool]$mc -or [bool]$aws) 'requires an installed MinIO client or AWS CLI; no tool is downloaded'

$report=[ordered]@{
    status=if($failures.Count -eq 0){'PASS'}else{'FAIL'}
    action='preflight-only; no database connection, dump, object read, restore, or deletion performed'
    recovery_database=$RecoveryDatabase; recovery_bucket=$RecoveryBucket; recovery_prefix=$RecoveryPrefix
    checks=@($checks); failure_reasons=@($failures)
}
$report|ConvertTo-Json -Depth 5
if($failures.Count -gt 0){exit 4}
