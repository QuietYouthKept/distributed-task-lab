[CmdletBinding()]
param([Parameter(Mandatory)][string]$ManifestFile)

$ErrorActionPreference='Stop'
$path=[System.IO.Path]::GetFullPath($ManifestFile)
if(-not $path.StartsWith('E:\',[System.StringComparison]::OrdinalIgnoreCase)){throw 'Archive verification is restricted to E:.'}
$sumPath=Join-Path (Split-Path -Parent $path) 'SHA256SUMS'
if(-not(Test-Path -LiteralPath $sumPath -PathType Leaf)){throw 'SHA256SUMS is missing.'}
$manifest=Get-Content -LiteralPath $path -Raw|ConvertFrom-Json -AsHashtable
$actual=(Get-FileHash -Algorithm SHA256 -LiteralPath $path).Hash.ToLowerInvariant()
$records=@($manifest.records)
$reasons=[System.Collections.Generic.List[string]]::new()
if($manifest.policy_version -ne 'retention-archive-v1'){$reasons.Add('policy_version')}
if($records.Count -gt [int]$manifest.limit -or $records.Count -gt 1000){$reasons.Add('batch_limit')}
try{$databaseNow=[DateTimeOffset]::Parse([string]$manifest.database_now).ToUniversalTime();$cutoff=[DateTimeOffset]::Parse([string]$manifest.cutoff).ToUniversalTime();if($cutoff -gt $databaseNow){$reasons.Add('cutoff_after_database_clock')}}catch{$reasons.Add('invalid_manifest_time')}
$sumLines=@(Get-Content -LiteralPath $sumPath|Where-Object{$_ -match "^[a-fA-F0-9]{64}\s+$([regex]::Escape((Split-Path -Leaf $path)))$"})
$listed=if($sumLines.Count -eq 1){($sumLines[0]-split '\s+',2)[0].ToLowerInvariant()}else{''}
if($listed -ne $actual){$reasons.Add('sha256_sums')}
$previousTime=$null;$previousID=''
$seenTaskIDs=[System.Collections.Generic.HashSet[string]]::new([System.StringComparer]::Ordinal)
foreach($record in $records){
    $taskID=[string]$record.task_id
    if([string]::IsNullOrWhiteSpace($taskID) -or -not $seenTaskIDs.Add($taskID)){$reasons.Add('missing_or_duplicate_task_id')}
    if([string]$record.status -notin @('success','failed','dlq')){$reasons.Add('nonterminal_record')}
    if([bool]$record.active_lease -or [bool]$record.retry_pending -or ([string]$record.status -eq 'dlq' -and [bool]$record.dlq_under_investigation)){$reasons.Add('unsafe_lease_retry_or_dlq_review')}
    foreach($effect in @($record.effects)){if([string]$effect.status -in @('intent','unknown')){$reasons.Add('unknown_effect')}elseif([string]$effect.status -notin @('succeeded','failed','cancelled')){$reasons.Add('unrecognized_effect_state')}}
    if(-not $record.artifact_inventory_complete){$reasons.Add('artifact_inventory_incomplete')}
    $artifactKeys=[System.Collections.Generic.HashSet[string]]::new([System.StringComparer]::Ordinal)
    foreach($artifact in @($record.artifacts)){if([string]::IsNullOrWhiteSpace([string]$artifact.key) -or -not $artifactKeys.Add([string]$artifact.key) -or [string]$artifact.sha256 -notmatch '^[a-f0-9]{64}$' -or [long]$artifact.size -lt 0){$reasons.Add('artifact_reference_invalid')}}
    if($null -eq $record.events -or $null -eq $record.audit_events){$reasons.Add('history_not_preserved')}
    try{$finished=[DateTimeOffset]::Parse([string]$record.finished_at).ToUniversalTime();if($finished -ge $cutoff){$reasons.Add('record_at_or_after_cutoff')}}catch{$reasons.Add('invalid_record_time');continue}
    if($null -ne $previousTime -and (($finished -lt $previousTime) -or ($finished -eq $previousTime -and [string]::CompareOrdinal($taskID,$previousID) -lt 0))){$reasons.Add('record_order')}
    $previousTime=$finished;$previousID=$taskID
}
$unique=@($reasons|Select-Object -Unique)
$report=[ordered]@{status=if($unique.Count -eq 0){'PASS'}else{'FAIL'};manifest=$path;records=$records.Count;sha256=$actual;sha256_valid=($listed -eq $actual);checks_passed=($unique.Count -eq 0);failure_reasons=$unique;action='read-only validation; no restore or deletion'}
$report|ConvertTo-Json -Depth 4
if($unique.Count -gt 0){exit 4}
