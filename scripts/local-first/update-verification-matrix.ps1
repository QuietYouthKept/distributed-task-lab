[CmdletBinding()]
param(
    [string]$EvidenceRoot = $(Join-Path $(if (-not [string]::IsNullOrWhiteSpace($env:TASKLAB_ARTIFACT_ROOT)) { $env:TASKLAB_ARTIFACT_ROOT } else { Join-Path ([System.IO.Path]::GetTempPath()) 'distributed-task-lab' }) 'production-readiness\evidence\deep-engineering'),
    [string]$OutputDirectory = 'docs\production-readiness'
)
if ([string]::IsNullOrWhiteSpace($env:TASKLAB_ARTIFACT_ROOT)) { $env:TASKLAB_ARTIFACT_ROOT = Join-Path ([System.IO.Path]::GetTempPath()) 'distributed-task-lab' }
$TaskLabArtifactRoot = [System.IO.Path]::GetFullPath($env:TASKLAB_ARTIFACT_ROOT)


$ErrorActionPreference = 'Stop'
$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
$evidenceRoot = [System.IO.Path]::GetFullPath($EvidenceRoot)
if (-not $evidenceRoot.StartsWith('E:\', [System.StringComparison]::OrdinalIgnoreCase)) { throw 'Evidence root must be under E:\.' }
$outDir = [System.IO.Path]::GetFullPath((Join-Path $repoRoot $OutputDirectory))
if (-not $outDir.StartsWith($repoRoot, [System.StringComparison]::OrdinalIgnoreCase)) { throw 'Output directory escaped the E: worktree.' }
if (-not (Test-Path -LiteralPath $evidenceRoot -PathType Container)) { throw 'No evidence root exists; matrix cannot be generated.' }
$runs = [System.Collections.Generic.List[object]]::new()
foreach ($manifestFile in Get-ChildItem -LiteralPath $evidenceRoot -Filter 'manifest.json' -File -Recurse) {
    try { $manifest = Get-Content -LiteralPath $manifestFile.FullName -Raw | ConvertFrom-Json } catch { continue }
    if (-not $manifest.run_id -or -not $manifest.status) { continue }
    $interpretedStatus = [string]$manifest.status
    $interpretation = ''
    $rawLog = Join-Path $manifestFile.DirectoryName ([string]$manifest.raw_log)
    if ($manifest.status -eq 'FAIL' -and [int]$manifest.tests_executed -eq 0 -and (Test-Path -LiteralPath $rawLog -PathType Leaf)) {
        $logText = [System.IO.File]::ReadAllText($rawLog)
        if ($logText -match 'go\.mod requires go >= .* \(running go ') {
            $interpretedStatus = 'DEPENDENCY_BLOCKED'
            $interpretation = 'Original runner classified nonzero exit as FAIL; raw log proves the Go version guard prevented any test from running.'
        }
    }
    $runs.Add([pscustomobject]@{
        run_id = [string]$manifest.run_id
        suite = [string]$manifest.suite
        status = $interpretedStatus
        status_in_original_manifest = [string]$manifest.status
        status_interpretation = $interpretation
        source_commit = [string]$manifest.source_commit
        source_tree = [string]$manifest.source_tree
        exit_code = $manifest.exit_code
        tests_executed = [int]$manifest.tests_executed
        passed = [int]$manifest.passed
        failed = [int]$manifest.failed
        skipped = [int]$manifest.skipped
        evidence_path = [System.IO.Path]::GetRelativePath($evidenceRoot, $manifestFile.DirectoryName)
        limitations = @($manifest.limitations)
    })
}
$ordered = @($runs | Sort-Object run_id)
$latestBySuite = @{}
foreach ($run in $ordered) { $latestBySuite[$run.suite] = $run }
$matrix = [ordered]@{
    generated_at_utc = [DateTime]::UtcNow.ToString('o')
    generator = 'scripts/local-first/update-verification-matrix.ps1'
    status_semantics = @('PASS','FAIL','NOT RUN','RESOURCE_LIMITED','DEPENDENCY_BLOCKED','IMPLEMENTED_NOT_VERIFIED')
    runs = $ordered
    latest_by_suite = $latestBySuite
    warning = 'A suite PASS applies only to the exact tests and source SHA in that run. It is not production approval or whole-system acceptance.'
}
New-Item -ItemType Directory -Path $outDir -Force | Out-Null
$jsonPath = Join-Path $outDir 'VERIFICATION_MATRIX.json'
[System.IO.File]::WriteAllText($jsonPath, ($matrix | ConvertTo-Json -Depth 10), [System.Text.UTF8Encoding]::new($false))
$lines = [System.Collections.Generic.List[string]]::new()
$lines.Add('<!-- CURRENT RUNS BEGIN: generated from evidence manifests -->')
$lines.Add("### Current deep-engineering runs (generated at UTC $($matrix.generated_at_utc))")
$lines.Add('')
$lines.Add('| Run ID | Suite | Status | Source commit | Tests | Pass | Fail | Skip | Evidence path |')
$lines.Add('|---|---|---|---|---:|---:|---:|---:|---|')
foreach ($run in $ordered) {
    $lines.Add("| $($run.run_id) | $($run.suite) | $($run.status) | $($run.source_commit) | $($run.tests_executed) | $($run.passed) | $($run.failed) | $($run.skipped) | ``$($run.evidence_path)`` |")
}
$lines.Add('')
$lines.Add('A PASS applies only to the exact tests and source SHA recorded by that evidence manifest; it does not imply whole-system or production acceptance.')
$lines.Add('<!-- CURRENT RUNS END -->')
$generated = ($lines -join "`n")
$markdownPath = Join-Path $outDir 'VERIFICATION_MATRIX.md'
if (Test-Path -LiteralPath $markdownPath -PathType Leaf) {
    $existing = [System.IO.File]::ReadAllText($markdownPath)
    if ($existing.Contains('<!-- CURRENT RUNS BEGIN:')) {
        $pattern = '(?s)<!-- CURRENT RUNS BEGIN:.*?<!-- CURRENT RUNS END -->'
        $existing = [regex]::Replace($existing, $pattern, [System.Text.RegularExpressions.MatchEvaluator]{ param($match) $generated })
    } else { $existing = $existing.TrimEnd() + "`n`n" + $generated + "`n" }
    [System.IO.File]::WriteAllText($markdownPath, $existing, [System.Text.UTF8Encoding]::new($false))
} else {
    [System.IO.File]::WriteAllText($markdownPath, $generated + "`n", [System.Text.UTF8Encoding]::new($false))
}
Write-Output "Generated manifest-backed matrix for $($ordered.Count) runs."
