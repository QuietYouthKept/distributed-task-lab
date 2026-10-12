[CmdletBinding()]
param([string]$EvidenceRoot=$(Join-Path $(if (-not [string]::IsNullOrWhiteSpace($env:TASKLAB_ARTIFACT_ROOT)) { $env:TASKLAB_ARTIFACT_ROOT } else { Join-Path ([System.IO.Path]::GetTempPath()) 'distributed-task-lab' }) 'production-readiness\evidence\backup-manifest-tests'))
if ([string]::IsNullOrWhiteSpace($env:TASKLAB_ARTIFACT_ROOT)) { $env:TASKLAB_ARTIFACT_ROOT = Join-Path ([System.IO.Path]::GetTempPath()) 'distributed-task-lab' }
$TaskLabArtifactRoot = [System.IO.Path]::GetFullPath($env:TASKLAB_ARTIFACT_ROOT)

$ErrorActionPreference='Stop'
Import-Module (Join-Path $PSScriptRoot 'backup-manifest.psm1') -Force -ErrorAction Stop
$root=[System.IO.Path]::GetFullPath($EvidenceRoot)
if(-not $root.StartsWith('E:\',[System.StringComparison]::OrdinalIgnoreCase)){throw 'Evidence must remain under E:.'}
$runId=[DateTimeOffset]::UtcNow.ToString('yyyyMMddTHHmmssfffZ')
$runDir=Join-Path $root $runId
$objectRoot=Join-Path $runDir 'objects'
New-Item -ItemType Directory -Path $objectRoot -Force|Out-Null
$bucket='tasklab-artifacts';$prefix='tasks/run-7';$backupId='backup-fixture-7';$snapshot=[DateTimeOffset]::UtcNow.AddMinutes(-1)
$objectPath=Join-Path $objectRoot 'tasklab-artifacts/tasks/run-7/result.json'
New-Item -ItemType Directory -Path (Split-Path -Parent $objectPath) -Force|Out-Null
$bytes=[System.Text.Encoding]::UTF8.GetBytes('{"result":"ok"}')
[System.IO.File]::WriteAllBytes($objectPath,$bytes)
$good=[pscustomobject]@{bucket=$bucket;key='tasks/run-7/result.json';size_bytes=$bytes.Length;sha256=[Convert]::ToHexString([System.Security.Cryptography.SHA256]::HashData($bytes)).ToLowerInvariant();referencing_task_id='task-7';artifact_role='result';snapshot_time=$snapshot.ToString('o');backup_run_id=$backupId}
function Write-Fixture([string]$Name,[object[]]$Objects){$path=Join-Path $runDir $Name;[System.IO.File]::WriteAllText($path,(@{objects=$Objects}|ConvertTo-Json -Depth 6),[System.Text.UTF8Encoding]::new($false));return $path}
function Assert-Case([string]$Name,[scriptblock]$Check){try{$ok=[bool](& $Check);$checks.Add([pscustomobject]@{name=$Name;status=$(if($ok){'PASS'}else{'FAIL'})})}catch{$checks.Add([pscustomobject]@{name=$Name;status='FAIL';error=$_.Exception.Message})}}
function Test-Throws([scriptblock]$Action){try{& $Action|Out-Null;return $false}catch{return $true}}
$checks=[System.Collections.Generic.List[object]]::new()
$validInventory=Write-Fixture 'valid-inventory.json' @($good)
$valid=Test-ArtifactInventory -InventoryPath $validInventory -ObjectRoot $objectRoot -ExpectedBucket $bucket -ExpectedPrefix $prefix -BackupRunId $backupId -SnapshotBoundary $snapshot
Assert-Case 'valid-object-inventory-and-content-hash' { $valid.status -eq 'OFFLINE_PASS' -and $valid.objects_checked -eq 1 -and $valid.objects[0].referencing_task_id -eq 'task-7' }
$missing=$good.PSObject.Copy();$missing.key='tasks/run-7/missing.json'
$missingResult=Test-ArtifactInventory -InventoryPath (Write-Fixture 'missing.json' @($missing)) -ObjectRoot $objectRoot -ExpectedBucket $bucket -ExpectedPrefix $prefix -BackupRunId $backupId -SnapshotBoundary $snapshot
Assert-Case 'missing-object-detected' { $missingResult.issues -contains 'missing_object:tasklab-artifacts/tasks/run-7/missing.json' }
$duplicateResult=Test-ArtifactInventory -InventoryPath (Write-Fixture 'duplicate.json' @($good,$good.PSObject.Copy())) -ObjectRoot $objectRoot -ExpectedBucket $bucket -ExpectedPrefix $prefix -BackupRunId $backupId -SnapshotBoundary $snapshot
Assert-Case 'duplicate-bucket-and-key-detected' { @($duplicateResult.issues|Where-Object{$_ -like 'duplicate_object_key:*'}).Count -eq 1 }
$wrongHash=$good.PSObject.Copy();$wrongHash.sha256='0'*64
$hashResult=Test-ArtifactInventory -InventoryPath (Write-Fixture 'hash-mismatch.json' @($wrongHash)) -ObjectRoot $objectRoot -ExpectedBucket $bucket -ExpectedPrefix $prefix -BackupRunId $backupId -SnapshotBoundary $snapshot
Assert-Case 'sha256-mismatch-detected' { $hashResult.issues -contains 'sha256_mismatch:tasklab-artifacts/tasks/run-7/result.json' }
$future=$good.PSObject.Copy();$future.snapshot_time=$snapshot.AddSeconds(1).ToString('o')
$futureResult=Test-ArtifactInventory -InventoryPath (Write-Fixture 'future.json' @($future)) -ObjectRoot $objectRoot -ExpectedBucket $bucket -ExpectedPrefix $prefix -BackupRunId $backupId -SnapshotBoundary $snapshot
Assert-Case 'snapshot-after-boundary-detected' { @($futureResult.issues|Where-Object{$_ -like 'snapshot_after_boundary:*'}).Count -eq 1 }
$unsafe=$good.PSObject.Copy();$unsafe.key='tasks/run-7/../escape'
$unsafeResult=Test-ArtifactInventory -InventoryPath (Write-Fixture 'unsafe-key.json' @($unsafe)) -ObjectRoot $objectRoot -ExpectedBucket $bucket -ExpectedPrefix $prefix -BackupRunId $backupId -SnapshotBoundary $snapshot
Assert-Case 'unsafe-object-path-detected' { @($unsafeResult.issues|Where-Object{$_ -like 'unsafe_object_path:*'}).Count -eq 1 }
$scope=$good.PSObject.Copy();$scope.key='tasks/run-70/result.json'
$scopeResult=Test-ArtifactInventory -InventoryPath (Write-Fixture 'prefix-confusion.json' @($scope)) -ObjectRoot $objectRoot -ExpectedBucket $bucket -ExpectedPrefix $prefix -BackupRunId $backupId -SnapshotBoundary $snapshot
Assert-Case 'source-prefix-boundary-enforced' { @($scopeResult.issues|Where-Object{$_ -like 'object_outside_source_scope:*'}).Count -eq 1 }
$manifestPath=Join-Path $runDir 'manifest.json'
$manifest=@{schema_version=1;database_schema_version=12;backup_run_id=$backupId;snapshot_at=$snapshot.ToString('o')}
[System.IO.File]::WriteAllText($manifestPath,($manifest|ConvertTo-Json),[System.Text.UTF8Encoding]::new($false))
$restore=Assert-RestoreBackupManifest -ManifestPath $manifestPath -SourceDatabase 'tasklab' -TargetDatabase 'tasklab_restore_7' -SourceBucket 'tasklab-artifacts' -SourcePrefix 'tasks/run-7' -TargetBucket 'tasklab-restore' -TargetPrefix 'restore/run-7' -SupportedSchemaVersion 12
Assert-Case 'independent-compatible-restore-target-accepted' { $restore.backup_run_id -eq $backupId }
Assert-Case 'same-database-restore-rejected' { Test-Throws { Assert-RestoreBackupManifest -ManifestPath $manifestPath -SourceDatabase 'tasklab' -TargetDatabase 'tasklab' -SourceBucket 'tasklab-artifacts' -SourcePrefix 'tasks/run-7' -TargetBucket 'tasklab-restore' -TargetPrefix 'restore/run-7' -SupportedSchemaVersion 12 } }
Assert-Case 'overlapping-bucket-prefix-rejected' { Test-Throws { Assert-RestoreBackupManifest -ManifestPath $manifestPath -SourceDatabase 'tasklab' -TargetDatabase 'tasklab_restore_7' -SourceBucket 'tasklab-artifacts' -SourcePrefix 'tasks/run-7' -TargetBucket 'tasklab-artifacts' -TargetPrefix 'tasks/run-7/restore' -SupportedSchemaVersion 12 } }
Assert-Case 'schema-incompatible-restore-rejected' { Test-Throws { Assert-RestoreBackupManifest -ManifestPath $manifestPath -SourceDatabase 'tasklab' -TargetDatabase 'tasklab_restore_7' -SourceBucket 'tasklab-artifacts' -SourcePrefix 'tasks/run-7' -TargetBucket 'tasklab-restore' -TargetPrefix 'restore/run-7' -SupportedSchemaVersion 13 } }
Assert-Case 'missing-manifest-rejected' { Test-Throws { Assert-RestoreBackupManifest -ManifestPath (Join-Path $runDir 'absent.json') -SourceDatabase 'tasklab' -TargetDatabase 'tasklab_restore_7' -SourceBucket 'tasklab-artifacts' -SourcePrefix 'tasks/run-7' -TargetBucket 'tasklab-restore' -TargetPrefix 'restore/run-7' -SupportedSchemaVersion 12 } }
$passed=@($checks|Where-Object{$_.status -eq 'PASS'}).Count;$failed=$checks.Count-$passed
$result=[ordered]@{run_id=$runId;status=$(if($failed -eq 0){'OFFLINE_PASS'}else{'FAIL'});assertions=$checks.Count;passed=$passed;failed=$failed;checks=@($checks);postgresql_contacted=$false;object_store_contacted=$false;restore_performed=$false}
Remove-Item -LiteralPath $objectRoot -Recurse -Force
$resultPath=Join-Path $runDir 'test-result.json'
[System.IO.File]::WriteAllText($resultPath,($result|ConvertTo-Json -Depth 7),[System.Text.UTF8Encoding]::new($false))
$files=Get-ChildItem -LiteralPath $runDir -File|Where-Object{$_.Name -ne 'SHA256SUMS'}
$lines=@($files|Get-FileHash -Algorithm SHA256|ForEach-Object{'{0}  {1}'-f $_.Hash.ToLowerInvariant(),[System.IO.Path]::GetFileName($_.Path)})
[System.IO.File]::WriteAllText((Join-Path $runDir 'SHA256SUMS'),($lines -join "`n")+"`n",[System.Text.UTF8Encoding]::new($false))
Write-Output "STATUS=$($result.status) RUN_ID=$runId ASSERTIONS=$($checks.Count) PASS=$passed FAIL=$failed EVIDENCE=$runDir"
if($failed -gt 0){exit 1}
