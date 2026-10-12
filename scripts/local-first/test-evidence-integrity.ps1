[CmdletBinding()]
param([string]$EvidenceRoot=$(Join-Path $(if (-not [string]::IsNullOrWhiteSpace($env:TASKLAB_ARTIFACT_ROOT)) { $env:TASKLAB_ARTIFACT_ROOT } else { Join-Path ([System.IO.Path]::GetTempPath()) 'distributed-task-lab' }) 'production-readiness\evidence\evidence-integrity-tests'))
if ([string]::IsNullOrWhiteSpace($env:TASKLAB_ARTIFACT_ROOT)) { $env:TASKLAB_ARTIFACT_ROOT = Join-Path ([System.IO.Path]::GetTempPath()) 'distributed-task-lab' }
$TaskLabArtifactRoot = [System.IO.Path]::GetFullPath($env:TASKLAB_ARTIFACT_ROOT)


$ErrorActionPreference='Stop'
$module=Join-Path $PSScriptRoot 'evidence-integrity.psm1'
Import-Module -Name $module -Force -ErrorAction Stop
$root=[System.IO.Path]::GetFullPath($EvidenceRoot)
if(-not $root.StartsWith('E:\',[System.StringComparison]::OrdinalIgnoreCase)){throw 'Test evidence must remain on E:.'}
$runId=[DateTimeOffset]::UtcNow.ToString('yyyyMMddTHHmmssfffZ')
$runDir=New-EvidenceRunDirectory -Path (Join-Path $root $runId) -AllowedRoot $root
$checks=[System.Collections.Generic.List[object]]::new()
function Assert-Case([string]$Name,[scriptblock]$Check){
    try{$ok=[bool](& $Check);$checks.Add([pscustomobject]@{name=$Name;status=$(if($ok){'PASS'}else{'FAIL'})})}
    catch{$checks.Add([pscustomobject]@{name=$Name;status='FAIL';error=$_.Exception.Message})}
}

Assert-Case 'nonzero-command-exit-cannot-pass' { (Get-VerificationOutcome -ExitCode 7 -Executed 1 -Passed 1 -Failed 0 -Skipped 0) -eq 'FAIL' }
Assert-Case 'zero-tests-is-not-pass' { (Get-VerificationOutcome -ExitCode 0 -Executed 0 -Passed 0 -Failed 0 -Skipped 0 -MinimumExecuted 1) -eq 'NOT RUN' }
Assert-Case 'all-skipped-is-not-pass' { (Get-VerificationOutcome -ExitCode 0 -Executed 4 -Passed 0 -Failed 0 -Skipped 4) -eq 'NOT RUN' }
Assert-Case 'counter-mismatch-is-fail' { (Get-VerificationOutcome -ExitCode 0 -Executed 2 -Passed 1 -Failed 0 -Skipped 0) -eq 'FAIL' }
$badVersion=Get-ToolVersionRecord -Tool go -VersionOutput 'go version go1.26.6 windows/amd64' -MinimumGoVersion '1.26.9'
Assert-Case 'below-minimum-tool-version-not-recorded' { $badVersion.status -eq 'DEPENDENCY_BLOCKED' -and $null -eq $badVersion.tool_version }
$goodVersion=Get-ToolVersionRecord -Tool powershell -VersionOutput "PowerShell $($PSVersionTable.PSVersion)"
Assert-Case 'valid-tool-version-recorded' { $goodVersion.status -eq 'PASS' -and $goodVersion.tool_version -match '^PowerShell ' }

$bundle=Join-Path $runDir 'sealed-bundle'
New-Item -ItemType Directory -Path $bundle | Out-Null
$stream=[System.IO.File]::Open((Join-Path $bundle 'raw.log'),[System.IO.FileMode]::CreateNew,[System.IO.FileAccess]::Write,[System.IO.FileShare]::None)
try{$bytes=[System.Text.Encoding]::UTF8.GetBytes('closed-log-before-hash');$stream.Write($bytes,0,$bytes.Length);$stream.Flush($true)}finally{$stream.Dispose()}
[System.IO.File]::WriteAllText((Join-Path $bundle 'manifest.json'),'{}',[System.Text.UTF8Encoding]::new($false))
$null=Write-EvidenceSums -Directory $bundle -FileNames @('raw.log','manifest.json')
Assert-Case 'closed-log-can-be-sealed-and-verified' { Test-EvidenceSums -Directory $bundle }
[System.IO.File]::AppendAllText((Join-Path $bundle 'raw.log'),'tamper',[System.Text.UTF8Encoding]::new($false))
Assert-Case 'append-after-seal-fails-integrity-check' { -not (Test-EvidenceSums -Directory $bundle) }

$gitRoot=Join-Path $runDir 'git-fixture'
New-Item -ItemType Directory -Path $gitRoot | Out-Null
& git -C $gitRoot init -b main *> $null
if($LASTEXITCODE -ne 0){throw 'Could not initialize the isolated source-inventory test repository.'}
& git -C $gitRoot config user.name 'Evidence Fixture' *> $null
& git -C $gitRoot config user.email 'evidence-fixture@example.invalid' *> $null
[System.IO.File]::WriteAllText((Join-Path $gitRoot 'staged-only.txt'),'base',[System.Text.UTF8Encoding]::new($false))
[System.IO.File]::WriteAllText((Join-Path $gitRoot 'unstaged.txt'),'base',[System.Text.UTF8Encoding]::new($false))
& git -C $gitRoot add --all *> $null
& git -C $gitRoot commit -m 'fixture baseline' *> $null
if($LASTEXITCODE -ne 0){throw 'Could not create baseline commit in isolated source-inventory test repository.'}
[System.IO.File]::WriteAllText((Join-Path $gitRoot 'staged-only.txt'),'staged',[System.Text.UTF8Encoding]::new($false))
& git -C $gitRoot add staged-only.txt *> $null
[System.IO.File]::WriteAllText((Join-Path $gitRoot 'unstaged.txt'),'changed',[System.Text.UTF8Encoding]::new($false))
[System.IO.File]::WriteAllText((Join-Path $gitRoot 'untracked.txt'),'new',[System.Text.UTF8Encoding]::new($false))
$inventory=Get-ChangedSourceInventory -RepoRoot $gitRoot
$inventoryNames=@($inventory.files|ForEach-Object{$_.path}|Sort-Object)
Assert-Case 'staged-unstaged-and-untracked-sources-are-inventoried' { $inventory.dirty -and $inventoryNames.Count -eq 3 -and $inventoryNames -contains 'staged-only.txt' -and $inventoryNames -contains 'unstaged.txt' -and $inventoryNames -contains 'untracked.txt' }
$inventoryChecksums=@($inventory.files|Where-Object{$_.sha256 -match '^[a-f0-9]{64}$'}).Count
Assert-Case 'dirty-source-files-have-content-hashes' { $inventoryChecksums -eq 3 }
$readFailed=$false
try{$null=Get-ChangedSourceInventory -RepoRoot (Join-Path $runDir 'missing-git-repo')}catch{$readFailed=$true}
Assert-Case 'unreadable-source-inventory-fails-closed' { $readFailed }

$historical=Join-Path $runDir 'historical-failure-evidence'
New-Item -ItemType Directory -Path $historical | Out-Null
[System.IO.File]::WriteAllText((Join-Path $historical 'raw.log'),'preserve-original-failure',[System.Text.UTF8Encoding]::new($false))
$historicalHash=(Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $historical 'raw.log')).Hash
$overwriteRefused=$false
try{$null=New-EvidenceRunDirectory -Path $historical -AllowedRoot $runDir}catch{$overwriteRefused=$true}
$historicalHashAfter=(Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $historical 'raw.log')).Hash
Assert-Case 'historical-evidence-is-read-only-and-not-overwritten' { $overwriteRefused -and $historicalHash -eq $historicalHashAfter }

Assert-Case 'failure-exit-code-propagates' { (Get-VerificationExitCode -Status FAIL) -eq 1 }
Assert-Case 'blocked-exit-code-propagates' { (Get-VerificationExitCode -Status DEPENDENCY_BLOCKED) -eq 3 }
$forcedSelfTestFailure=Get-VerificationOutcome -ExitCode 1 -Executed 1 -Passed 0 -Failed 1 -Skipped 0
Assert-Case 'self-test-failure-cannot-create-pass-result' { $forcedSelfTestFailure -eq 'FAIL' }
$runnerSource=[System.IO.File]::ReadAllText((Join-Path $PSScriptRoot 'run-verification.ps1'))
Assert-Case 'blocked-guard-suites-record-what-ran-and-what-did-not' { $runnerSource.Contains('PowerShell guard: check TEST_DATABASE_URL; no PostgreSQL test command launched') -and $runnerSource.Contains('scripts/local-first/check-resources.ps1 -Workload container -ReportOnly; no E2E/failure command launched') -and $runnerSource.Contains('scripts/local-first/check-resources.ps1 -Workload race -ReportOnly; no Race command launched') -and $runnerSource.Contains('PowerShell guard: check govulncheck availability; no scanner command launched') }
Assert-Case 'go-build-suites-are-resource-gated-and-single-parallel' { $runnerSource.Contains("'unit','race','vet'") -and $runnerSource.Contains("-Workload compile -ReportOnly") -and $runnerSource.Contains("test -p 1") -and $runnerSource.Contains("vet -p 1") -and $runnerSource.Contains("GOMAXPROCS']='1'") }
$inventorySource=[System.IO.File]::ReadAllText($module)
Assert-Case 'git-diagnostics-cannot-be-inventoried-as-source-paths' { -not $inventorySource.Contains('2>&1') -and ([regex]::Matches($inventorySource,'2>\$null')).Count -ge 4 }

$passed=@($checks|Where-Object{$_.status -eq 'PASS'}).Count
$failed=$checks.Count-$passed
$status=if($failed -eq 0 -and $checks.Count -gt 0){'PASS'}else{'FAIL'}
$resultPath=Join-Path $runDir 'test-result.json'
$result=[ordered]@{status=$status;run_id=$runId;assertions=$checks.Count;passed=$passed;failed=$failed;checks=@($checks);created_by='scripts/local-first/test-evidence-integrity.ps1';historical_evidence_modified=$false;go_or_docker_tests_run=$false;seal_validation='PENDING'}
[System.IO.File]::WriteAllText($resultPath,($result|ConvertTo-Json -Depth 8),[System.Text.UTF8Encoding]::new($false))
$resultHashFiles=@('test-result.json')
$null=Write-EvidenceSums -Directory $runDir -FileNames $resultHashFiles
if(Test-EvidenceSums -Directory $runDir){$result.seal_validation='PASS'}else{$status='FAIL';$result.status='FAIL';$result.seal_validation='FAIL'}
[System.IO.File]::WriteAllText($resultPath,($result|ConvertTo-Json -Depth 8),[System.Text.UTF8Encoding]::new($false))
$null=Write-EvidenceSums -Directory $runDir -FileNames $resultHashFiles
if(-not(Test-EvidenceSums -Directory $runDir)){$status='FAIL';$result.status='FAIL';$result.seal_validation='FAIL';[System.IO.File]::WriteAllText($resultPath,($result|ConvertTo-Json -Depth 8),[System.Text.UTF8Encoding]::new($false));$null=Write-EvidenceSums -Directory $runDir -FileNames $resultHashFiles}
Write-Output "STATUS=$status RUN_ID=$runId ASSERTIONS=$($checks.Count) PASS=$passed FAIL=$failed EVIDENCE=$runDir"
if($status -ne 'PASS'){exit 1}
