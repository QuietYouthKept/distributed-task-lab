[CmdletBinding()]
param([string]$EvidenceRoot=$(Join-Path $(if (-not [string]::IsNullOrWhiteSpace($env:TASKLAB_ARTIFACT_ROOT)) { $env:TASKLAB_ARTIFACT_ROOT } else { Join-Path ([System.IO.Path]::GetTempPath()) 'distributed-task-lab' }) 'production-readiness\evidence\supply-chain-static'))
if ([string]::IsNullOrWhiteSpace($env:TASKLAB_ARTIFACT_ROOT)) { $env:TASKLAB_ARTIFACT_ROOT = Join-Path ([System.IO.Path]::GetTempPath()) 'distributed-task-lab' }
$TaskLabArtifactRoot = [System.IO.Path]::GetFullPath($env:TASKLAB_ARTIFACT_ROOT)

$ErrorActionPreference='Stop'
$repoRoot=[System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
$evidenceRoot=[System.IO.Path]::GetFullPath($EvidenceRoot)
if(-not $evidenceRoot.StartsWith('E:\',[System.StringComparison]::OrdinalIgnoreCase)){throw 'Evidence must remain under E:.'}
$runId=[DateTimeOffset]::UtcNow.ToString('yyyyMMddTHHmmssfffZ');$runDir=Join-Path $evidenceRoot $runId
New-Item -ItemType Directory -Path $runDir -ErrorAction Stop|Out-Null
$checks=[System.Collections.Generic.List[object]]::new();$findings=[System.Collections.Generic.List[object]]::new()
function Add-Check([string]$Name,[bool]$Passed,[string]$Detail){$checks.Add([pscustomobject]@{name=$Name;status=$(if($Passed){'PASS'}else{'FAIL'});detail=$Detail})}
$dockerfiles=@(Get-ChildItem -LiteralPath (Join-Path $repoRoot 'deployments') -Filter 'Dockerfile*' -File)
$unpinned=[System.Collections.Generic.List[string]]::new();$nonRootMissing=[System.Collections.Generic.List[string]]::new();$revisionMissing=[System.Collections.Generic.List[string]]::new();$dynamicBase=[System.Collections.Generic.List[string]]::new()
foreach($file in $dockerfiles){
    $lines=Get-Content -LiteralPath $file.FullName
    foreach($line in $lines){if($line -match '^FROM\s+(?<image>\S+)'){if($Matches.image -match '^\$\{'){if($file.Name -eq 'Dockerfile.minio-init'){$dynamicBase.Add($file.Name)}else{$unpinned.Add("$($file.Name):$($Matches.image)")}}elseif($Matches.image -notmatch '@sha256:[a-fA-F0-9]{64}(?:\s|$)'){$unpinned.Add("$($file.Name):$($Matches.image)")}}}
    if(-not ($lines|Where-Object{$_ -match '^USER\s+(?!0\b|root\b)[^\s]+'})){$nonRootMissing.Add($file.Name)}
    if(-not ($lines|Where-Object{$_ -match 'org\.opencontainers\.image\.revision'})){$revisionMissing.Add($file.Name)}
}
Add-Check 'all-static-base-images-pinned' ($unpinned.Count -eq 0) ($unpinned -join ', ')
Add-Check 'all-images-run-as-nonroot' ($nonRootMissing.Count -eq 0) ($nonRootMissing -join ', ')
Add-Check 'all-dockerfiles-declare-revision-label' ($revisionMissing.Count -eq 0) ($revisionMissing -join ', ')
$ignore=Get-Content -LiteralPath (Join-Path $repoRoot '.dockerignore')
$requiredIgnores=@('.env','.env.*','secrets/','*.key','*.pem','evidence/','**/evidence/')
$missingIgnore=@($requiredIgnores|Where-Object{$_ -notin $ignore})
Add-Check 'docker-context-excludes-secrets-and-evidence' ($missingIgnore.Count -eq 0) ($missingIgnore -join ', ')
$compose=Get-Content -LiteralPath (Join-Path $repoRoot 'deployments/local-first/compose.core.yml') -Raw
$readonlyRuntime=$compose -match '(?m)^\s+read_only:\s*true\s*$'
if(-not $readonlyRuntime){$findings.Add([pscustomobject]@{control='read-only-root-filesystem';status='FEASIBILITY_NOT_VERIFIED';detail='No service has read_only: true; validate writable paths and tmpfs before enabling.'})}
$unknownRevision=$compose -notmatch '(?m)^\s*SOURCE_COMMIT:'
if($unknownRevision){$findings.Add([pscustomobject]@{control='oci-revision-build-arg';status='GAP';detail='Compose does not consistently pass SOURCE_COMMIT to each build, so labels may remain unknown.'})}
$workflowFiles=@(Get-ChildItem -LiteralPath (Join-Path $repoRoot '.github/workflows') -Filter '*.yml' -File)+@(Get-ChildItem -LiteralPath (Join-Path $repoRoot '.github/workflows') -Filter '*.yaml' -File)
$actionLines=@(foreach($workflow in $workflowFiles){Select-String -LiteralPath $workflow.FullName -Pattern '^\s*uses:\s*([^\s]+)'|ForEach-Object{[pscustomobject]@{file=$workflow.Name;reference=$_.Matches[0].Groups[1].Value}}})
$mutableActions=@($actionLines|Where-Object{$_.reference -notmatch '@[a-fA-F0-9]{40}$'})
if($mutableActions.Count -gt 0){$findings.Add([pscustomobject]@{control='workflow-action-immutability';status='REVIEW_REQUIRED';detail="$($mutableActions.Count) action reference(s) use a mutable tag; pin only after verifying upstream commit provenance."})}
$toolStatus=[ordered]@{}
foreach($toolName in @('govulncheck','trivy','syft','cosign')){$tool=Get-Command $toolName -ErrorAction SilentlyContinue;$toolStatus[$toolName]=if($tool){'AVAILABLE_NOT_RUN'}else{'UNAVAILABLE_NOT_INSTALLED'}}
$passed=@($checks|Where-Object{$_.status -eq 'PASS'}).Count;$failed=$checks.Count-$passed
$status=if($failed -eq 0){'OFFLINE_PASS'}else{'FAIL'}
$report=[ordered]@{status=$status;run_id=$runId;checks=@($checks);findings=@($findings);dockerfiles=@($dockerfiles.Name);dynamic_base_arguments=@($dynamicBase);action_refs=$actionLines;mutable_action_count=$mutableActions.Count;scanner_tool_status=$toolStatus;scanners_run=$false;images_built=$false;sbom_generated=$false;signatures_created=$false;read_only_runtime_verified=$false;production_approval='NOT_APPROVED'}
$reportPath=Join-Path $runDir 'supply-chain-static.json'
[System.IO.File]::WriteAllText($reportPath,($report|ConvertTo-Json -Depth 8),[System.Text.UTF8Encoding]::new($false))
$hash=(Get-FileHash -Algorithm SHA256 -LiteralPath $reportPath).Hash.ToLowerInvariant()
[System.IO.File]::WriteAllText((Join-Path $runDir 'SHA256SUMS'),"$hash  supply-chain-static.json`n",[System.Text.UTF8Encoding]::new($false))
Write-Output "STATUS=$status RUN_ID=$runId ASSERTIONS=$($checks.Count) PASS=$passed FAIL=$failed FINDINGS=$($findings.Count) EVIDENCE=$runDir"
if($failed -gt 0){exit 1}
