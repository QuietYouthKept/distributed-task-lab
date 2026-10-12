[CmdletBinding()]
param(
    [string]$EvidenceRoot = $(Join-Path $(if (-not [string]::IsNullOrWhiteSpace($env:TASKLAB_ARTIFACT_ROOT)) { $env:TASKLAB_ARTIFACT_ROOT } else { Join-Path ([System.IO.Path]::GetTempPath()) 'distributed-task-lab' }) 'production-readiness\evidence\deep-engineering'),
    [string]$MatrixPath = 'docs\production-readiness\VERIFICATION_MATRIX.json',
    [string]$ReportPath = 'docs\production-readiness\EVIDENCE_INTEGRITY_ACCEPTANCE.md'
)
if ([string]::IsNullOrWhiteSpace($env:TASKLAB_ARTIFACT_ROOT)) { $env:TASKLAB_ARTIFACT_ROOT = Join-Path ([System.IO.Path]::GetTempPath()) 'distributed-task-lab' }
$TaskLabArtifactRoot = [System.IO.Path]::GetFullPath($env:TASKLAB_ARTIFACT_ROOT)


$ErrorActionPreference = 'Stop'
$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
$evidenceRoot = [System.IO.Path]::GetFullPath($EvidenceRoot)
$matrixFullPath = [System.IO.Path]::GetFullPath((Join-Path $repoRoot $MatrixPath))
$reportFullPath = [System.IO.Path]::GetFullPath((Join-Path $repoRoot $ReportPath))
foreach ($path in @($evidenceRoot, $matrixFullPath, $reportFullPath)) {
    if (-not $path.StartsWith('E:\', [System.StringComparison]::OrdinalIgnoreCase)) { throw 'Evidence audit inputs and output must remain under E:.' }
}
if (-not (Test-Path -LiteralPath $evidenceRoot -PathType Container)) { throw 'Evidence root is missing.' }
if (-not (Test-Path -LiteralPath $matrixFullPath -PathType Leaf)) { throw 'Verification matrix is missing.' }

$matrix = Get-Content -LiteralPath $matrixFullPath -Raw | ConvertFrom-Json
$matrixById = @{}
foreach ($entry in @($matrix.runs)) { $matrixById[[string]$entry.run_id] = $entry }
$results = [System.Collections.Generic.List[object]]::new()
foreach ($manifestFile in Get-ChildItem -LiteralPath $evidenceRoot -Filter 'manifest.json' -File -Recurse) {
    $dir = $manifestFile.DirectoryName
    $issues = [System.Collections.Generic.List[string]]::new()
    try { $manifest = Get-Content -LiteralPath $manifestFile.FullName -Raw | ConvertFrom-Json }
    catch { $results.Add([pscustomobject]@{run_id=$manifestFile.Directory.Name;suite='unknown';status='UNREADABLE';source_commit=$null;source_tree=$null;dirty=$null;changed_sources=$null;command=$null;exit_code=$null;tests=$null;passed=$null;failed=$null;skipped=$null;hashes_ok=$false;issues=@('manifest_json_invalid')}); continue }

    if (-not $manifest.run_id -or -not $manifest.suite -or -not $manifest.status) { $issues.Add('manifest_required_identity_missing') }
    if (-not $manifest.source_commit -or [string]$manifest.source_commit -notmatch '^[a-f0-9]{40,64}$') { $issues.Add('source_commit_missing_or_invalid') }
    if (-not $manifest.source_tree -or [string]$manifest.source_tree -notmatch '^[a-f0-9]{40,64}$') { $issues.Add('source_tree_missing_or_invalid') }
    if ($manifest.source_commit -and $manifest.source_tree) {
        $resolvedTree = @(& git -C $repoRoot rev-parse "$($manifest.source_commit)^{tree}" 2>&1)
        if ($LASTEXITCODE -ne 0 -or $resolvedTree.Count -eq 0) { $issues.Add('source_commit_unresolvable') }
        elseif ([string]$resolvedTree[0] -cne [string]$manifest.source_tree) { $issues.Add('source_tree_commit_mismatch') }
    }
    if ($null -eq $manifest.dirty_at_start) { $issues.Add('dirty_state_missing') }
    if ($manifest.dirty_at_start -and -not $manifest.source_inventory) { $issues.Add('dirty_source_inventory_missing') }
    if ([string]::IsNullOrWhiteSpace([string]$manifest.exact_command)) { $issues.Add('test_command_missing') }
    if ($null -eq $manifest.exit_code) { $issues.Add('exit_code_missing') }
    if ($null -eq $manifest.tests_executed -or $null -eq $manifest.passed -or $null -eq $manifest.failed -or $null -eq $manifest.skipped) { $issues.Add('test_counters_missing') }
    elseif ([int]$manifest.passed + [int]$manifest.failed + [int]$manifest.skipped -ne [int]$manifest.tests_executed) { $issues.Add('test_counters_inconsistent') }
    if ($manifest.status -eq 'PASS' -and [int]$manifest.tests_executed -gt 0 -and [int]$manifest.passed -eq 0) { $issues.Add('pass_without_successful_test') }
    if ($manifest.status -eq 'PASS' -and [int]$manifest.failed -gt 0) { $issues.Add('pass_with_failed_test') }

    $inventoryCount = $null
    if ($manifest.source_inventory) {
        $inventoryPath = Join-Path $dir ([string]$manifest.source_inventory)
        if (-not (Test-Path -LiteralPath $inventoryPath -PathType Leaf)) { $issues.Add('source_inventory_missing') }
        else {
            $inventoryHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $inventoryPath).Hash.ToLowerInvariant()
            if ($manifest.source_inventory_sha256 -and $inventoryHash -ne ([string]$manifest.source_inventory_sha256).ToLowerInvariant()) { $issues.Add('source_inventory_hash_mismatch') }
            try { $inventory = @(Get-Content -LiteralPath $inventoryPath -Raw | ConvertFrom-Json); $inventoryCount = $inventory.Count }
            catch { $issues.Add('source_inventory_json_invalid') }
            if ($manifest.dirty_at_start -and $inventoryCount -eq 0) { $issues.Add('dirty_source_inventory_empty') }
        }
    }

    $sumPath = Join-Path $dir 'SHA256SUMS'
    $hashesOk = $false
    if (-not (Test-Path -LiteralPath $sumPath -PathType Leaf)) { $issues.Add('sha256sums_missing') }
    else {
        $sumLines = @(Get-Content -LiteralPath $sumPath)
        $seen = [System.Collections.Generic.HashSet[string]]::new([System.StringComparer]::OrdinalIgnoreCase)
        $valid = $sumLines.Count -gt 0
        foreach ($line in $sumLines) {
            if ($line -notmatch '^([a-fA-F0-9]{64})  ([^\\/:]+)$') { $valid = $false; break }
            $expected = $Matches[1].ToLowerInvariant(); $name = $Matches[2]
            if (-not $seen.Add($name)) { $valid = $false; break }
            $target = Join-Path $dir $name
            if (-not (Test-Path -LiteralPath $target -PathType Leaf)) { $valid = $false; break }
            $actual = (Get-FileHash -Algorithm SHA256 -LiteralPath $target).Hash.ToLowerInvariant()
            if ($actual -ne $expected) { $valid = $false; break }
        }
        foreach ($requiredFile in @('manifest.json',[string]$manifest.raw_log,[string]$manifest.source_inventory)) {
            if (-not [string]::IsNullOrWhiteSpace($requiredFile) -and -not $seen.Contains($requiredFile)) { $valid = $false }
        }
        $hashesOk = $valid
        if (-not $valid) { $issues.Add('sha256sums_invalid_or_mismatch') }
    }

    $matrixRow = $matrixById[[string]$manifest.run_id]
    if (-not $matrixRow) { $issues.Add('manifest_not_in_matrix') }
    else {
        foreach ($field in @('suite','source_commit','source_tree','exit_code','tests_executed','passed','failed','skipped')) {
            if ([string]$matrixRow.$field -ne [string]$manifest.$field) { $issues.Add("matrix_mismatch:$field") }
        }
        if ([string]$matrixRow.status -ne [string]$manifest.status) {
            $documentedInterpretation = [string]$matrixRow.status_interpretation
            $originalStatus = [string]$matrixRow.status_in_original_manifest
            if (-not $documentedInterpretation -or $originalStatus -ne [string]$manifest.status) { $issues.Add('matrix_mismatch:status') }
        }
    }
    $results.Add([pscustomobject]@{
        run_id=[string]$manifest.run_id; suite=[string]$manifest.suite; status=[string]$manifest.status
        source_commit=[string]$manifest.source_commit; source_tree=[string]$manifest.source_tree
        dirty=[bool]$manifest.dirty_at_start; changed_sources=$inventoryCount; command=[string]$manifest.exact_command
        exit_code=$manifest.exit_code; tests=[int]$manifest.tests_executed; passed=[int]$manifest.passed
        failed=[int]$manifest.failed; skipped=[int]$manifest.skipped; hashes_ok=$hashesOk; issues=@($issues)
    })
}

$missingManifests = @($matrix.runs | Where-Object { -not ($results.run_id -contains [string]$_.run_id) })
foreach ($missing in $missingManifests) { $results.Add([pscustomobject]@{run_id=[string]$missing.run_id;suite=[string]$missing.suite;status='MISSING_MANIFEST';source_commit=[string]$missing.source_commit;source_tree=[string]$missing.source_tree;dirty=$null;changed_sources=$null;command=$null;exit_code=$missing.exit_code;tests=$missing.tests_executed;passed=$missing.passed;failed=$missing.failed;skipped=$missing.skipped;hashes_ok=$false;issues=@('matrix_row_has_no_manifest')}) }
$issuesTotal = @($results | Where-Object { @($_.issues).Count -gt 0 }).Count
$statusCounts = @($results | Group-Object status | Sort-Object Name | ForEach-Object { "| $($_.Name) | $($_.Count) |" })
$manifestRows = @($results | Sort-Object run_id | ForEach-Object { "| $($_.run_id) | $($_.suite) | $($_.status) | $($_.source_commit) | $($_.source_tree) | $($_.dirty) | $($_.changed_sources) | $($_.exit_code) | $($_.tests) | $($_.passed) | $($_.failed) | $($_.skipped) | $($_.hashes_ok) | $(@($_.issues) -join ', ') |" })
$content = @(
    '# Evidence Integrity Acceptance'
    ''
    "Audit generated UTC: $([DateTime]::UtcNow.ToString('o'))"
    "Evidence root: ``$evidenceRoot``"
    "Matrix: ``$MatrixPath``"
    "Manifest rows audited: $($results.Count); rows with issues: $issuesTotal."
    ''
    'This is an integrity audit, not a new test execution. Historical source commits and trees remain unchanged. A historical PASS is scoped to the exact recorded command, test counters, source inventory, commit/tree, and files sealed by that run.'
    ''
    '## Per-run results'
    ''
    '| Run ID | Suite | Status | Source commit | Tree | Dirty | Inventory files | Exit | Tests | Pass | Fail | Skip | Hashes valid | Issues |'
    '|---|---|---|---|---|---:|---:|---:|---:|---:|---:|---:|---|---|'
    $manifestRows
    ''
    '## Status counts'
    ''
    '| Status | Count |'
    '|---|---:|'
    $statusCounts
    ''
    '## Interpretation'
    ''
    '- A PASS with zero executed tests is static/tooling evidence only; it does not establish functional acceptance.'
    '- Historical Unit/Race results remain attached to their original commit and tree; they are not evidence for a later HEAD.'
    '- The earliest five runs predate source-inventory capture. Their `dirty_at_start=true` state cannot be tied to a changed-file list; those historical rows are intentionally flagged and have not been rewritten.'
    '- The new runner self-tests separately prove status classification, dirty-source inventory, closed-log sealing, tamper detection, and refusal to overwrite existing evidence. They do not prove external service behavior.'
    '- If any row has an issue, the matrix and source artifacts must be reconciled before claiming a clean audit.'
    ''
    '## Audit summary'
    ''
    "AUDIT_STATUS=$(if ($issuesTotal -eq 0) { 'PASS' } else { 'FAIL' })"
    "Issues: $issuesTotal"
)
New-Item -ItemType Directory -Path (Split-Path -Parent $reportFullPath) -Force | Out-Null
[System.IO.File]::WriteAllText($reportFullPath, ($content -join "`n") + "`n", [System.Text.UTF8Encoding]::new($false))
Write-Output "AUDIT_STATUS=$(if ($issuesTotal -eq 0) { 'PASS' } else { 'FAIL' }) RUNS=$($results.Count) ISSUES=$issuesTotal REPORT=$reportFullPath"
if ($issuesTotal -gt 0) { exit 1 }
