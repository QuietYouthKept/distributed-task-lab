[CmdletBinding()]
param([string]$EvidenceRoot=$(Join-Path $(if (-not [string]::IsNullOrWhiteSpace($env:TASKLAB_ARTIFACT_ROOT)) { $env:TASKLAB_ARTIFACT_ROOT } else { Join-Path ([System.IO.Path]::GetTempPath()) 'distributed-task-lab' }) 'production-readiness\evidence\compose-cleanup-tests'))
if ([string]::IsNullOrWhiteSpace($env:TASKLAB_ARTIFACT_ROOT)) { $env:TASKLAB_ARTIFACT_ROOT = Join-Path ([System.IO.Path]::GetTempPath()) 'distributed-task-lab' }
$TaskLabArtifactRoot = [System.IO.Path]::GetFullPath($env:TASKLAB_ARTIFACT_ROOT)


$ErrorActionPreference='Stop'
$module=Join-Path $PSScriptRoot 'compose-cleanup.psm1'
Import-Module -Name $module -Force -ErrorAction Stop
$root=[System.IO.Path]::GetFullPath($EvidenceRoot)
if(-not $root.StartsWith('E:\',[System.StringComparison]::OrdinalIgnoreCase)){throw 'Test evidence must remain under E:.'}
$runId=[DateTimeOffset]::UtcNow.ToString('yyyyMMddTHHmmssfffZ')
$testDir=Join-Path $root $runId
New-Item -ItemType Directory -Path $testDir -ErrorAction Stop|Out-Null
$fixtureStamp=[DateTimeOffset]::Now.ToString('yyyyMMddTHHmmsszzz').Replace(':','')
$fixtureId=$fixtureStamp.Replace('T','t').Replace('+','p').Replace('-','m').ToLowerInvariant()
$project="tasklab-local-first-$fixtureId"
$secretRoot=Join-Path $(Join-Path $TaskLabArtifactRoot 'production-readiness\secrets') "p0-runtime-$fixtureStamp"
$composeRoot=[System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
$envPath=Join-Path $secretRoot 'compose.env'
New-Item -ItemType Directory -Path (Join-Path $secretRoot 'tls/identity'),(Join-Path $secretRoot 'tls/api') -Force|Out-Null
$composeFile=Join-Path $composeRoot 'deployments/local-first/compose.core.yml'
$envContent=@(
    "COMPOSE_PROJECT_NAME=$project",
    "TASKLAB_NETWORK_NAME=$project-core",
    "TASKLAB_POSTGRES_VOLUME=$project-postgres",
    "TASKLAB_MINIO_VOLUME=$project-minio",
    "TASKLAB_SECRET_DIR=$($secretRoot -replace '\\','/')"
)
[System.IO.File]::WriteAllText($envPath,($envContent -join "`n"),[System.Text.UTF8Encoding]::new($false))
$plan=Get-ScopedComposeCleanupPlan -ComposeProject $project -ComposeEnvFile $envPath -ComposeFile $composeFile -StopServices -RemoveContainersAndNetwork
$checks=[System.Collections.Generic.List[object]]::new()
function Assert-Case([string]$Name,[scriptblock]$Check){try{$ok=[bool](& $Check);$checks.Add([pscustomobject]@{name=$Name;status=$(if($ok){'PASS'}else{'FAIL'})})}catch{$checks.Add([pscustomobject]@{name=$Name;status='FAIL';error=$_.Exception.Message})}}
function Test-Throws([scriptblock]$Action){try{& $Action|Out-Null;return $false}catch{return $true}}

Assert-Case 'default-plan-retains-data-volumes-and-evidence' { $plan.dry_run -and $plan.retain_data_volumes -and $plan.preserve_evidence -and $plan.expected_volumes.Count -eq 2 }
Assert-Case 'wrong-project-id-rejected' { Test-Throws { Get-ScopedComposeCleanupPlan -ComposeProject 'other-project' -ComposeEnvFile $envPath -ComposeFile $composeFile } }
Assert-Case 'project-env-mismatch-rejected' { $bad=Join-Path $testDir 'wrong.env';[System.IO.File]::WriteAllText($bad,($envContent -replace [regex]::Escape($project),'tasklab-local-first-20261011t111501p0800' -join "`n"),[System.Text.UTF8Encoding]::new($false));Test-Throws { Get-ScopedComposeCleanupPlan -ComposeProject $project -ComposeEnvFile $bad -ComposeFile $composeFile } }
$global:LASTEXITCODE=0
$dry=Invoke-ScopedComposeCleanup -Plan $plan -Executor { param([Parameter(ValueFromRemainingArguments=$true)][string[]]$Arguments) $global:LASTEXITCODE=0; if($Arguments[0] -eq 'ps'){ @('container1|'+$project) } elseif($Arguments[0] -eq 'network'){@("$project-core|$project")} elseif($Arguments[0] -eq 'volume'){@("$project-postgres|$project","$project-minio|$project")} }
Assert-Case 'dry-run-only-inspects-scoped-labels' { -not $dry.executed -and $dry.inspection.Count -eq 1 -and $dry.volumes_removed -eq 0 }
$calls=[System.Collections.Generic.List[string]]::new()
$applied=Invoke-ScopedComposeCleanup -Plan $plan -Executor { param([Parameter(ValueFromRemainingArguments=$true)][string[]]$Arguments) $global:LASTEXITCODE=0; $calls.Add(($Arguments -join ' ')); if($Arguments[0] -eq 'ps'){ @('container1|'+$project) } elseif($Arguments[0] -eq 'network'){@("$project-core|$project")} elseif($Arguments[0] -eq 'volume'){@("$project-postgres|$project","$project-minio|$project")} } -ConfirmRunId $fixtureId -Apply
Assert-Case 'apply-requires-matching-run-id-and-keeps-volumes' { $applied.executed -and $applied.volumes_removed -eq 0 -and $calls[-1] -notmatch '(^|\s)-v(\s|$)' -and $calls[-1] -match 'down --remove-orphans' }
Assert-Case 'mismatched-confirmation-rejected-before-mutation' { Test-Throws { Invoke-ScopedComposeCleanup -Plan $plan -Executor { param([Parameter(ValueFromRemainingArguments=$true)][string[]]$Arguments) $global:LASTEXITCODE=0 } -ConfirmRunId 'wrong' -Apply } }
Assert-Case 'apply-without-explicit-action-rejected' { $noAction=Get-ScopedComposeCleanupPlan -ComposeProject $project -ComposeEnvFile $envPath -ComposeFile $composeFile;Test-Throws { Invoke-ScopedComposeCleanup -Plan $noAction -Executor { param([Parameter(ValueFromRemainingArguments=$true)][string[]]$Arguments) $global:LASTEXITCODE=0 } -ConfirmRunId $fixtureId -Apply } }
$stopFailed=Test-Throws { Invoke-ScopedComposeCleanup -Plan $plan -Executor { param([Parameter(ValueFromRemainingArguments=$true)][string[]]$Arguments) if($Arguments[0] -eq 'ps'){@('container1|'+$project);$global:LASTEXITCODE=0}elseif($Arguments[0] -eq 'network'){@("$project-core|$project");$global:LASTEXITCODE=0}elseif($Arguments[0] -eq 'volume'){@("$project-postgres|$project","$project-minio|$project");$global:LASTEXITCODE=0}elseif($Arguments[0] -eq 'compose'){$global:LASTEXITCODE=17} } -ConfirmRunId $fixtureId -Apply }
Assert-Case 'cleanup-command-failure-stops-before-later-actions' { $stopFailed -and (Test-Path -LiteralPath $secretRoot) }
$unexpected=Test-Throws { Invoke-ScopedComposeCleanup -Plan $plan -Executor { param([Parameter(ValueFromRemainingArguments=$true)][string[]]$Arguments) $global:LASTEXITCODE=0; if($Arguments[0] -eq 'ps'){@('foreign-container|some-other-project')} } }
Assert-Case 'unexpected-existing-container-rejected' { $unexpected }
$foreignRejected=Test-Throws { Invoke-ScopedComposeCleanup -Plan $plan -Executor { param([Parameter(ValueFromRemainingArguments=$true)][string[]]$Arguments) $global:LASTEXITCODE=0; if($Arguments[0] -eq 'ps'){@('foreign-container|some-other-project')} } -ConfirmRunId $fixtureId -Apply }
Assert-Case 'foreign-container-label-blocks-destructive-actions' { $foreignRejected -and $calls.Count -eq 5 }
$unknownVolumeRejected=Test-Throws { Invoke-ScopedComposeCleanup -Plan $plan -Executor { param([Parameter(ValueFromRemainingArguments=$true)][string[]]$Arguments) $global:LASTEXITCODE=0; if($Arguments[0] -eq 'volume'){@('tasklab-local-first-other-postgres|'+$project)} } -ConfirmRunId $fixtureId -Apply }
Assert-Case 'unknown-volume-blocks-apply' { $unknownVolumeRejected }
$unknownSecretPath=Join-Path $secretRoot 'unexpected.txt'
[System.IO.File]::WriteAllText($unknownSecretPath,'fixture-only',[System.Text.UTF8Encoding]::new($false))
$secretPlanRejected=Test-Throws { Get-ScopedComposeCleanupPlan -ComposeProject $project -ComposeEnvFile $envPath -ComposeFile $composeFile -DeleteEphemeralSecrets }
Assert-Case 'unrecognized-secret-tree-blocks-delete' { $secretPlanRejected }
$null=Remove-Item -LiteralPath $unknownSecretPath -Force
$deletePlan=Get-ScopedComposeCleanupPlan -ComposeProject $project -ComposeEnvFile $envPath -ComposeFile $composeFile -DeleteEphemeralSecrets
$deleted=Invoke-ScopedComposeCleanup -Plan $deletePlan -Executor { param([Parameter(ValueFromRemainingArguments=$true)][string[]]$Arguments) $global:LASTEXITCODE=0 } -ConfirmRunId $fixtureId -Apply
Assert-Case 'ephemeral-secret-deletion-is-run-scoped-and-explicit' { $deleted.executed -and -not(Test-Path -LiteralPath $secretRoot) }
$recoveryHarness = [System.IO.File]::ReadAllText((Join-Path $PSScriptRoot 'recovery-harness.ps1'))
Assert-Case 'recovery-harness-reuses-exact-compose-run-scope' { $recoveryHarness.Contains('Get-ScopedComposeCleanupPlan -ComposeProject $ComposeProject -ComposeEnvFile $envPath -ComposeFile $composeFile') }
Assert-Case 'fault-apply-requires-explicit-matching-run-id' { $recoveryHarness.Contains('-ConfirmRunId to exactly match the Compose Run ID') }
Assert-Case 'fault-approximations-are-not-misrepresented-as-network-or-request-injection' { $recoveryHarness.Contains('Service-stop approximation only') -and $recoveryHarness.Contains('does not inject packet loss') }
$passed=@($checks|Where-Object{$_.status -eq 'PASS'}).Count
$failed=$checks.Count-$passed
$result=[ordered]@{run_id=$runId;status=$(if($failed -eq 0){'OFFLINE_PASS'}else{'FAIL'});assertions=$checks.Count;passed=$passed;failed=$failed;checks=@($checks);docker_daemon_contacted=$false;external_resources_modified=$false}
$resultPath=Join-Path $testDir 'test-result.json'
[System.IO.File]::WriteAllText($resultPath,($result|ConvertTo-Json -Depth 7),[System.Text.UTF8Encoding]::new($false))
$hash=(Get-FileHash -Algorithm SHA256 -LiteralPath $resultPath).Hash.ToLowerInvariant()
[System.IO.File]::WriteAllText((Join-Path $testDir 'SHA256SUMS'),"$hash  test-result.json`n",[System.Text.UTF8Encoding]::new($false))
Write-Output "STATUS=$($result.status) RUN_ID=$runId ASSERTIONS=$($checks.Count) PASS=$passed FAIL=$failed EVIDENCE=$testDir"
if($failed -gt 0){exit 1}
