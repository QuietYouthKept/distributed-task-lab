[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]$InventoryFile,
    [Parameter(Mandatory)][string]$OutputDirectory,
    [ValidateRange(1,1000)][int]$Limit = 100,
    [string]$ResumeToken,
    [string]$AfterFinishedAt,
    [string]$AfterTaskId
)

$ErrorActionPreference = 'Stop'
$inputPath=[System.IO.Path]::GetFullPath($InventoryFile)
$outputPath=[System.IO.Path]::GetFullPath($OutputDirectory)
if(-not $inputPath.StartsWith('E:\',[System.StringComparison]::OrdinalIgnoreCase) -or -not $outputPath.StartsWith('E:\',[System.StringComparison]::OrdinalIgnoreCase)){throw 'Retention inventory and archive manifest must remain under E:.'}
if(-not(Test-Path -LiteralPath $inputPath -PathType Leaf)){throw 'Inventory file does not exist.'}
if(Test-Path -LiteralPath $outputPath){throw 'Refusing to overwrite an existing archive run directory.'}
if(($AfterFinishedAt -and -not $AfterTaskId) -or ($AfterTaskId -and -not $AfterFinishedAt)){throw 'Resume requires both AfterFinishedAt and AfterTaskId.'}
if($ResumeToken -and ($AfterTaskId -or $AfterFinishedAt)){throw 'Use either ResumeToken or explicit cursor fields, not both.'}
$inventory=Get-Content -LiteralPath $inputPath -Raw|ConvertFrom-Json
$databaseNow=[DateTimeOffset]::MinValue;$cutoff=[DateTimeOffset]::MinValue
if(-not [DateTimeOffset]::TryParse([string]$inventory.database_now,[ref]$databaseNow) -or -not [DateTimeOffset]::TryParse([string]$inventory.cutoff,[ref]$cutoff)){throw 'Inventory database_now/cutoff timestamps are invalid.'}
$databaseNow=$databaseNow.ToUniversalTime();$cutoff=$cutoff.ToUniversalTime()
if($cutoff -gt $databaseNow){throw 'Retention cutoff is later than the database clock.'}
if(-not $inventory.snapshot_id -or @($inventory.records).Count -gt 10000){throw 'Inventory snapshot ID is required and source inventory is bounded at 10,000 rows.'}
if($ResumeToken){
    try{
        $tokenBase64=$ResumeToken.Replace('-','+').Replace('_','/')
        switch($tokenBase64.Length%4){2{$tokenBase64+='=='};3{$tokenBase64+='='}}
        $tokenJson=[System.Text.Encoding]::UTF8.GetString([Convert]::FromBase64String($tokenBase64))
        $token=$tokenJson|ConvertFrom-Json
    }catch{throw 'Resume token is malformed.'}
    if($token.policy_version -ne 'retention-archive-v1' -or [string]$token.snapshot_id -ne [string]$inventory.snapshot_id -or [DateTimeOffset]::Parse([string]$token.cutoff).ToUniversalTime() -ne $cutoff){throw 'Resume token does not match this policy, source snapshot, or cutoff.'}
    $AfterFinishedAt=[string]$token.after_finished_at;$AfterTaskId=[string]$token.after_task_id
}
$cursorTime=[DateTimeOffset]::MinValue
if($AfterFinishedAt -and -not [DateTimeOffset]::TryParse($AfterFinishedAt,[ref]$cursorTime)){throw 'Resume cursor timestamp is invalid.'}
$cursorTime=$cursorTime.ToUniversalTime()
$eligible=[System.Collections.Generic.List[object]]::new()
$exclusionCounts=@{}
$seen=[System.Collections.Generic.HashSet[string]]::new([System.StringComparer]::Ordinal)
foreach($record in @($inventory.records)){
    $id=[string]$record.task_id
    if([string]::IsNullOrWhiteSpace($id) -or -not $seen.Add($id)){throw 'Task IDs must be nonempty and unique in an inventory snapshot.'}
    $reason=$null
    $finished=[DateTimeOffset]::MinValue
    if(-not [DateTimeOffset]::TryParse([string]$record.finished_at,[ref]$finished)){$reason='invalid_finished_at'}else{$finished=$finished.ToUniversalTime()}
    if(-not $reason -and [string]$record.status -notin @('success','failed','dlq')){$reason='nonterminal_status'}
    if(-not $reason -and $finished -ge $cutoff){$reason='not_old_enough'}
    if(-not $reason -and [bool]$record.active_lease){$reason='active_lease'}
    if(-not $reason -and [bool]$record.retry_pending){$reason='retry_pending'}
    if(-not $reason -and ([string]$record.status -eq 'dlq') -and [bool]$record.dlq_under_investigation){$reason='dlq_under_investigation'}
    if(-not $reason){
        foreach($effect in @($record.effects)){
            if([string]$effect.status -in @('intent','unknown')){$reason='effect_not_reconciled';break}
            if([string]$effect.status -notin @('succeeded','failed','cancelled')){$reason='effect_state_unrecognized';break}
        }
    }
    if($reason){if(-not $exclusionCounts.ContainsKey($reason)){$exclusionCounts[$reason]=0};$exclusionCounts[$reason]++;continue}
    if(-not $record.artifact_inventory_complete){throw "Artifact inventory is incomplete for task '$id'; refusing to build an archive manifest."}
    $artifactKeys=[System.Collections.Generic.HashSet[string]]::new([System.StringComparer]::Ordinal)
    foreach($artifact in @($record.artifacts)){
        if([string]::IsNullOrWhiteSpace([string]$artifact.key) -or -not $artifactKeys.Add([string]$artifact.key) -or [string]$artifact.sha256 -notmatch '^[a-f0-9]{64}$' -or [long]$artifact.size -lt 0){throw "Artifact inventory is invalid for task '$id'; refusing to build an archive manifest."}
    }
    if($null -eq $record.events -or $null -eq $record.audit_events){throw "Event and audit arrays must remain present in every archive record ('$id')."}
    if($AfterTaskId){
        $afterTuple=($finished -gt $cursorTime) -or ($finished -eq $cursorTime -and [string]::CompareOrdinal($id,$AfterTaskId) -gt 0)
        if(-not $afterTuple){continue}
    }
    $eligible.Add([pscustomobject]@{ finished_at=$finished; record=$record })
}
$ordered=@($eligible|Sort-Object @{Expression={$_.finished_at.UtcDateTime.Ticks}},@{Expression={$_.record.task_id};Ascending=$true})
$hasMore=$ordered.Count -gt $Limit
$selected=@($ordered|Select-Object -First $Limit)
$archiveRecords=@($selected|ForEach-Object{$_.record})
$nextToken=$null
if($hasMore -and $selected.Count -gt 0){
    $last=$selected[-1]
    $tokenPayload=[ordered]@{policy_version='retention-archive-v1';snapshot_id=[string]$inventory.snapshot_id;cutoff=$cutoff.ToString('o');after_finished_at=$last.finished_at.ToString('o');after_task_id=[string]$last.record.task_id}
    $tokenBytes=[System.Text.Encoding]::UTF8.GetBytes(($tokenPayload|ConvertTo-Json -Compress))
    $nextToken=[Convert]::ToBase64String($tokenBytes).TrimEnd('=').Replace('+','-').Replace('/','_')
}
$manifest=[ordered]@{
    policy_version='retention-archive-v1';snapshot_id=[string]$inventory.snapshot_id
    database_now=$databaseNow.ToString('o');cutoff=$cutoff.ToString('o');limit=$Limit
    resume_from=if($AfterTaskId){[ordered]@{finished_at=$AfterFinishedAt;task_id=$AfterTaskId}}else{$null}
    next_resume_token=$nextToken;eligible_count=$archiveRecords.Count
    excluded_counts=$exclusionCounts;records=$archiveRecords
}
New-Item -ItemType Directory -Path $outputPath | Out-Null
$identity=[System.Security.Principal.WindowsIdentity]::GetCurrent().Name
& icacls.exe $outputPath /inheritance:r /grant:r "${identity}:(OI)(CI)F" | Out-Null
if($LASTEXITCODE -ne 0){throw 'Could not restrict archive-manifest directory ACL.'}
$manifestPath=Join-Path $outputPath 'archive-manifest.json'
[System.IO.File]::WriteAllText($manifestPath,($manifest|ConvertTo-Json -Depth 64),[System.Text.UTF8Encoding]::new($false))
$sum=(Get-FileHash -Algorithm SHA256 -LiteralPath $manifestPath).Hash.ToLowerInvariant()
[System.IO.File]::WriteAllText((Join-Path $outputPath 'SHA256SUMS'),"$sum  archive-manifest.json`n",[System.Text.UTF8Encoding]::new($false))
Write-Output "MANIFEST=$manifestPath ELIGIBLE=$($archiveRecords.Count) EXCLUDED=$(@($exclusionCounts.Values|Measure-Object -Sum).Sum) SHA256=$sum"
