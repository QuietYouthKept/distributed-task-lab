[CmdletBinding()]
param([string]$EvidenceRoot=$(Join-Path $(if (-not [string]::IsNullOrWhiteSpace($env:TASKLAB_ARTIFACT_ROOT)) { $env:TASKLAB_ARTIFACT_ROOT } else { Join-Path ([System.IO.Path]::GetTempPath()) 'distributed-task-lab' }) 'production-readiness\evidence\database-safety-tests'))
if ([string]::IsNullOrWhiteSpace($env:TASKLAB_ARTIFACT_ROOT)) { $env:TASKLAB_ARTIFACT_ROOT = Join-Path ([System.IO.Path]::GetTempPath()) 'distributed-task-lab' }
$TaskLabArtifactRoot = [System.IO.Path]::GetFullPath($env:TASKLAB_ARTIFACT_ROOT)

$ErrorActionPreference='Stop'
Import-Module (Join-Path $PSScriptRoot 'database-safety.psm1') -Force -ErrorAction Stop
$root=[System.IO.Path]::GetFullPath($EvidenceRoot)
if(-not $root.StartsWith('E:\',[System.StringComparison]::OrdinalIgnoreCase)){throw 'Evidence must stay under E:.'}
$runId=[DateTimeOffset]::UtcNow.ToString('yyyyMMddTHHmmssfffZ')
$runDir=Join-Path $root $runId
New-Item -ItemType Directory -Path $runDir -ErrorAction Stop|Out-Null
$checks=[System.Collections.Generic.List[object]]::new()
function Assert-Case([string]$Name,[scriptblock]$Check){try{$ok=[bool](& $Check);$checks.Add([pscustomobject]@{name=$Name;status=$(if($ok){'PASS'}else{'FAIL'})})}catch{$checks.Add([pscustomobject]@{name=$Name;status='FAIL';error=$_.Exception.Message})}}
function Test-Throws([scriptblock]$Action){try{& $Action|Out-Null;return $false}catch{return $true}}
$good='postgres://tasklab_local_test_admin:placeholder@127.0.0.1:55432/tasklab_local_test_admin'
$parsed=Assert-DisposablePostgresURL $good
Assert-Case 'explicit-loopback-port-and-maintenance-identity-accepted' { $parsed.port -eq 55432 -and $parsed.host -eq '127.0.0.1' -and $parsed.maintenance_role -eq 'tasklab_local_test_admin' }
$ipv6=Assert-DisposablePostgresURL 'postgres://tasklab_local_test_admin:placeholder@[::1]:55433/tasklab_local_test_admin'
Assert-Case 'ipv6-loopback-with-explicit-port-accepted' { $ipv6.host -eq '::1' -and $ipv6.port -eq 55433 }
Assert-Case 'default-port-url-rejected' { Test-Throws { Assert-DisposablePostgresURL 'postgres://tasklab_local_test_admin:placeholder@localhost/tasklab_local_test_admin' } }
Assert-Case 'remote-host-rejected' { Test-Throws { Assert-DisposablePostgresURL 'postgres://tasklab_local_test_admin:placeholder@db.example:55432/tasklab_local_test_admin' } }
Assert-Case 'unsafe-database-name-rejected' { Test-Throws { Assert-DisposablePostgresURL 'postgres://tasklab_local_test_admin:placeholder@127.0.0.1:55432/postgres' } }
Assert-Case 'missing-maintenance-role-rejected' { Test-Throws { Assert-DisposablePostgresURL 'postgres://127.0.0.1:55432/tasklab_local_test_admin' } }
Assert-Case 'invalid-port-rejected' { Test-Throws { Assert-DisposablePostgresURL 'postgres://tasklab_local_test_admin:placeholder@127.0.0.1:99999/tasklab_local_test_admin' } }
$repoRoot=[System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
$toolchain=Resolve-ModuleGoToolchain -RepoRoot $repoRoot
Assert-Case 'go-toolchain-meets-module-floor-without-download' { (Test-Path -LiteralPath $toolchain.path -PathType Leaf) -and ([version]($toolchain.version -replace '^go','') -ge [version]$toolchain.minimum) }
$sample="db error $good"
$safe=Protect-DatabaseTestLog -Text $sample -ConnectionString $good
Assert-Case 'connection-string-secret-redacted' { $safe -notmatch 'placeholder' -and $safe -match '\[REDACTED_DATABASE_URL\]' }
$passed=@($checks|Where-Object{$_.status -eq 'PASS'}).Count
$failed=$checks.Count-$passed
$result=[ordered]@{run_id=$runId;status=$(if($failed -eq 0){'OFFLINE_PASS'}else{'FAIL'});assertions=$checks.Count;passed=$passed;failed=$failed;checks=@($checks);postgresql_contacted=$false}
$resultPath=Join-Path $runDir 'test-result.json'
[System.IO.File]::WriteAllText($resultPath,($result|ConvertTo-Json -Depth 6),[System.Text.UTF8Encoding]::new($false))
$hash=(Get-FileHash -Algorithm SHA256 -LiteralPath $resultPath).Hash.ToLowerInvariant()
[System.IO.File]::WriteAllText((Join-Path $runDir 'SHA256SUMS'),"$hash  test-result.json`n",[System.Text.UTF8Encoding]::new($false))
Write-Output "STATUS=$($result.status) RUN_ID=$runId ASSERTIONS=$($checks.Count) PASS=$passed FAIL=$failed EVIDENCE=$runDir"
if($failed -gt 0){exit 1}
