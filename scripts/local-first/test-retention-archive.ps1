[CmdletBinding()]
param([string]$EvidenceRoot=$(Join-Path $(if (-not [string]::IsNullOrWhiteSpace($env:TASKLAB_ARTIFACT_ROOT)) { $env:TASKLAB_ARTIFACT_ROOT } else { Join-Path ([System.IO.Path]::GetTempPath()) 'distributed-task-lab' }) 'production-readiness\evidence\retention-fixture'))
if ([string]::IsNullOrWhiteSpace($env:TASKLAB_ARTIFACT_ROOT)) { $env:TASKLAB_ARTIFACT_ROOT = Join-Path ([System.IO.Path]::GetTempPath()) 'distributed-task-lab' }
$TaskLabArtifactRoot = [System.IO.Path]::GetFullPath($env:TASKLAB_ARTIFACT_ROOT)


$ErrorActionPreference='Stop'
$scriptRoot=$PSScriptRoot
$evidenceRoot=[System.IO.Path]::GetFullPath($EvidenceRoot)
if(-not $evidenceRoot.StartsWith('E:\',[System.StringComparison]::OrdinalIgnoreCase)){throw 'Fixture evidence is restricted to E:.'}
$runId=[DateTimeOffset]::UtcNow.ToString('yyyyMMddTHHmmssfffZ')
$runDir=Join-Path $evidenceRoot $runId
New-Item -ItemType Directory -Path $runDir -Force|Out-Null
function Assert-True([bool]$condition,[string]$message){if(-not $condition){throw $message}}
$now=[DateTimeOffset]::UtcNow.ToString('o');$cutoff=[DateTimeOffset]::UtcNow.AddDays(-30).ToString('o');$old=[DateTimeOffset]::UtcNow.AddDays(-60).ToString('o')
$records=@(
    [ordered]@{task_id='task-a';status='success';finished_at=$old;active_lease=$false;retry_pending=$false;dlq_under_investigation=$false;effects=@();artifact_inventory_complete=$true;artifacts=@();events=@(@{event='success'});audit_events=@(@{action='read'})},
    [ordered]@{task_id='task-b';status='failed';finished_at=$old;active_lease=$false;retry_pending=$false;dlq_under_investigation=$false;effects=@();artifact_inventory_complete=$true;artifacts=@(@{key='artifacts/task-b';sha256=('a'*64);size=7});events=@();audit_events=@()},
    [ordered]@{task_id='task-lease';status='success';finished_at=$old;active_lease=$true;retry_pending=$false;dlq_under_investigation=$false;effects=@();artifact_inventory_complete=$true;artifacts=@();events=@();audit_events=@()},
    [ordered]@{task_id='task-retry';status='failed';finished_at=$old;active_lease=$false;retry_pending=$true;dlq_under_investigation=$false;effects=@();artifact_inventory_complete=$true;artifacts=@();events=@();audit_events=@()},
    [ordered]@{task_id='task-unknown';status='success';finished_at=$old;active_lease=$false;retry_pending=$false;dlq_under_investigation=$false;effects=@(@{status='unknown'});artifact_inventory_complete=$true;artifacts=@();events=@();audit_events=@()},
    [ordered]@{task_id='task-dlq';status='dlq';finished_at=$old;active_lease=$false;retry_pending=$false;dlq_under_investigation=$true;effects=@();artifact_inventory_complete=$true;artifacts=@();events=@();audit_events=@()},
    [ordered]@{task_id='task-cutoff';status='success';finished_at=$cutoff;active_lease=$false;retry_pending=$false;dlq_under_investigation=$false;effects=@();artifact_inventory_complete=$true;artifacts=@();events=@();audit_events=@()}
)
$inventory=[ordered]@{snapshot_id='fixture-snapshot-001';database_now=$now;cutoff=$cutoff;records=$records}
$inventoryPath=Join-Path $runDir 'inventory.json'
[System.IO.File]::WriteAllText($inventoryPath,($inventory|ConvertTo-Json -Depth 32),[System.Text.UTF8Encoding]::new($false))
$out1=Join-Path $runDir 'archive-one';$out2=Join-Path $runDir 'archive-two'
$generator=Join-Path $scriptRoot 'new-retention-archive-manifest.ps1';$validator=Join-Path $scriptRoot 'verify-retention-archive-manifest.ps1'
& $generator -InventoryFile $inventoryPath -OutputDirectory $out1 -Limit 1; if($LASTEXITCODE -ne 0){throw 'First manifest generation failed.'}
$manifestPath=Join-Path $out1 'archive-manifest.json';$manifest=Get-Content -LiteralPath $manifestPath -Raw|ConvertFrom-Json
Assert-True ($manifest.eligible_count -eq 1) 'First batch must contain exactly one candidate.'
Assert-True (-not [string]::IsNullOrWhiteSpace([string]$manifest.next_resume_token)) 'A continuation token must be emitted when another eligible row exists.'
Assert-True ($manifest.records[0].events.Count -eq 1 -and $manifest.records[0].audit_events.Count -eq 1) 'Task event/audit history was not preserved.'
Assert-True ($manifest.excluded_counts.active_lease -eq 1 -and $manifest.excluded_counts.retry_pending -eq 1 -and $manifest.excluded_counts.effect_not_reconciled -eq 1 -and $manifest.excluded_counts.dlq_under_investigation -eq 1 -and $manifest.excluded_counts.not_old_enough -eq 1) 'Unsafe or cutoff-boundary exclusion reasons were not all counted.'
& $validator -ManifestFile $manifestPath; if($LASTEXITCODE -ne 0){throw 'Manifest validation failed.'}
& $generator -InventoryFile $inventoryPath -OutputDirectory $out2 -Limit 1; if($LASTEXITCODE -ne 0){throw 'Deterministic replay generation failed.'}
$manifest2=Get-Content -LiteralPath (Join-Path $out2 'archive-manifest.json') -Raw|ConvertFrom-Json
$hash1=(Get-FileHash -Algorithm SHA256 -LiteralPath $manifestPath).Hash
$hash2=(Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $out2 'archive-manifest.json')).Hash
Assert-True ($hash1 -eq $hash2) 'Identical inventory and policy must produce the same archive hash.'
& $generator -InventoryFile $inventoryPath -OutputDirectory (Join-Path $runDir 'archive-resumed') -Limit 1 -ResumeToken ([string]$manifest.next_resume_token); if($LASTEXITCODE -ne 0){throw 'Resume generation failed.'}
$resumed=Get-Content -LiteralPath (Join-Path $runDir 'archive-resumed\archive-manifest.json') -Raw|ConvertFrom-Json
Assert-True ($resumed.eligible_count -eq 1 -and $resumed.records[0].task_id -ne $manifest.records[0].task_id) 'Resume cursor must continue after the prior tuple.'
$bad=$inventory|ConvertTo-Json -Depth 32|ConvertFrom-Json
$bad.records[1].artifacts[0].sha256='invalid'
$badPath=Join-Path $runDir 'invalid-artifact-inventory.json'
[System.IO.File]::WriteAllText($badPath,($bad|ConvertTo-Json -Depth 32),[System.Text.UTF8Encoding]::new($false))
$badOutput=Join-Path $runDir 'invalid-output'
$invalidFailed=$false
try { & $generator -InventoryFile $badPath -OutputDirectory $badOutput -Limit 10 2>$null } catch { $invalidFailed=$true }
Assert-True ($invalidFailed -and -not(Test-Path -LiteralPath $badOutput)) 'Invalid artifact SHA must fail closed before output creation.'
$otherSnapshot=$inventory|ConvertTo-Json -Depth 32|ConvertFrom-Json
$otherSnapshot.snapshot_id='different-snapshot'
$otherPath=Join-Path $runDir 'different-snapshot.json'
[System.IO.File]::WriteAllText($otherPath,($otherSnapshot|ConvertTo-Json -Depth 32),[System.Text.UTF8Encoding]::new($false))
$resumeMismatchFailed=$false
try { & $generator -InventoryFile $otherPath -OutputDirectory (Join-Path $runDir 'wrong-resume-output') -Limit 1 -ResumeToken ([string]$manifest.next_resume_token) 2>$null } catch { $resumeMismatchFailed=$true }
Assert-True ($resumeMismatchFailed -and -not(Test-Path -LiteralPath (Join-Path $runDir 'wrong-resume-output'))) 'Resume token must reject a different source snapshot.'
$duplicate=$inventory|ConvertTo-Json -Depth 32|ConvertFrom-Json
$duplicate.records[1].task_id='task-a'
$duplicatePath=Join-Path $runDir 'duplicate-task-id.json'
[System.IO.File]::WriteAllText($duplicatePath,($duplicate|ConvertTo-Json -Depth 32),[System.Text.UTF8Encoding]::new($false))
$duplicateRejected=$false
try { & $generator -InventoryFile $duplicatePath -OutputDirectory (Join-Path $runDir 'duplicate-output') -Limit 10 2>$null } catch { $duplicateRejected=$true }
Assert-True ($duplicateRejected -and -not(Test-Path -LiteralPath (Join-Path $runDir 'duplicate-output'))) 'Duplicate task IDs must be rejected before output creation.'
$futureCutoff=$inventory|ConvertTo-Json -Depth 32|ConvertFrom-Json
$futureCutoff.cutoff=[DateTimeOffset]::Parse($futureCutoff.database_now).AddSeconds(1).ToString('o')
$futureCutoffPath=Join-Path $runDir 'future-cutoff.json'
[System.IO.File]::WriteAllText($futureCutoffPath,($futureCutoff|ConvertTo-Json -Depth 32),[System.Text.UTF8Encoding]::new($false))
$futureCutoffRejected=$false
try { & $generator -InventoryFile $futureCutoffPath -OutputDirectory (Join-Path $runDir 'future-cutoff-output') -Limit 10 2>$null } catch { $futureCutoffRejected=$true }
Assert-True ($futureCutoffRejected -and -not(Test-Path -LiteralPath (Join-Path $runDir 'future-cutoff-output'))) 'A cutoff later than the captured database clock must be rejected before output creation.'
$missingHistory=$inventory|ConvertTo-Json -Depth 32|ConvertFrom-Json
$missingHistory.records[0].audit_events=$null
$missingHistoryPath=Join-Path $runDir 'missing-history.json'
[System.IO.File]::WriteAllText($missingHistoryPath,($missingHistory|ConvertTo-Json -Depth 32),[System.Text.UTF8Encoding]::new($false))
$missingHistoryRejected=$false
try { & $generator -InventoryFile $missingHistoryPath -OutputDirectory (Join-Path $runDir 'missing-history-output') -Limit 10 2>$null } catch { $missingHistoryRejected=$true }
Assert-True ($missingHistoryRejected -and -not(Test-Path -LiteralPath (Join-Path $runDir 'missing-history-output'))) 'Missing audit history must fail closed before output creation.'
$generatorPath=Join-Path $scriptRoot 'new-retention-archive-manifest.ps1';$validatorPath=Join-Path $scriptRoot 'verify-retention-archive-manifest.ps1';$testPath=$MyInvocation.MyCommand.Path
$sourceCommit=(& git -C (Join-Path $scriptRoot '..\..') rev-parse HEAD).Trim()
$sourceTree=(& git -C (Join-Path $scriptRoot '..\..') rev-parse 'HEAD^{tree}').Trim()
$result=[ordered]@{status='PASS';run_id=$runId;source_commit=$sourceCommit;source_tree=$sourceTree;assertions=12;eligible=2;excluded=5;database_connected=$false;object_store_connected=$false;deletes_performed=$false;limitations=@('Fixture-only policy test; inventory came from a local fixture, not PostgreSQL. Artifact SHA format is checked but object bytes are not fetched.')}
$resultPath=Join-Path $runDir 'test-result.json'
[System.IO.File]::WriteAllText($resultPath,($result|ConvertTo-Json -Depth 5),[System.Text.UTF8Encoding]::new($false))
$hashFiles=@($inventoryPath,$manifestPath,(Join-Path $out2 'archive-manifest.json'),(Join-Path $runDir 'archive-resumed\archive-manifest.json'),$resultPath,$generatorPath,$validatorPath,$testPath)
$sumLines=@(Get-FileHash -Algorithm SHA256 -LiteralPath $hashFiles|ForEach-Object{$relative=[System.IO.Path]::GetRelativePath($runDir,$_.Path).Replace('\','/');'{0}  {1}'-f $_.Hash.ToLowerInvariant(),$relative})
[System.IO.File]::WriteAllText((Join-Path $runDir 'SHA256SUMS'),($sumLines -join "`n")+"`n",[System.Text.UTF8Encoding]::new($false))
foreach($sumLine in $sumLines){$parts=$sumLine -split '\s+',2;$actual=(Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $runDir $parts[1].Replace('/','\'))).Hash.ToLowerInvariant();Assert-True ($actual -eq $parts[0]) "Evidence checksum mismatch for $($parts[1])."}
Write-Output "STATUS=PASS RUN_ID=$runId ASSERTIONS=12 EVIDENCE=$runDir"
