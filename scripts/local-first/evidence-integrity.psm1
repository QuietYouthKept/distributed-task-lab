Set-StrictMode -Version Latest

function Get-ChangedSourceInventory {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)][string]$RepoRoot,
        [string]$GitExe = 'git'
    )

    $root = [System.IO.Path]::GetFullPath($RepoRoot).TrimEnd('\')
    $topLines = @(& $GitExe -C $root rev-parse --show-toplevel 2>$null)
    if ($LASTEXITCODE -ne 0 -or $topLines.Count -eq 0) { throw 'Cannot resolve Git worktree root.' }
    $top = [System.IO.Path]::GetFullPath([string]$topLines[0]).TrimEnd('\')
    if (-not $top.Equals($root, [System.StringComparison]::OrdinalIgnoreCase)) { throw 'Git worktree root does not match the requested source root.' }

    $statusLines = @(& $GitExe -C $root status --porcelain 2>$null)
    if ($LASTEXITCODE -ne 0) { throw 'Cannot read Git dirty state.' }
    $dirty = $statusLines.Count -gt 0

    # Compare the working tree to HEAD, not only to the index. This includes staged-only edits.
    $trackedChanges = @(& $GitExe -C $root diff --name-only --no-renames HEAD 2>$null)
    if ($LASTEXITCODE -ne 0) { throw 'Cannot enumerate tracked source changes against HEAD.' }
    $untrackedChanges = @(& $GitExe -C $root ls-files --others --exclude-standard 2>$null)
    if ($LASTEXITCODE -ne 0) { throw 'Cannot enumerate untracked source files.' }
    $paths = @($trackedChanges + $untrackedChanges | Where-Object { -not [string]::IsNullOrWhiteSpace([string]$_) } | Select-Object -Unique)
    if ($dirty -and $paths.Count -eq 0) { throw 'Git reports a dirty worktree but no source paths could be inventoried.' }

    $files = [System.Collections.Generic.List[object]]::new()
    foreach ($relative in $paths) {
        $relativePath = [string]$relative
        if ([System.IO.Path]::IsPathRooted($relativePath)) { throw 'Git returned a rooted path in source inventory.' }
        $fullPath = [System.IO.Path]::GetFullPath((Join-Path $root $relativePath))
        if (-not $fullPath.StartsWith($root + '\', [System.StringComparison]::OrdinalIgnoreCase)) { throw 'Git returned a path outside the source root.' }
        if (Test-Path -LiteralPath $fullPath -PathType Leaf) {
            $hash = (Get-FileHash -Algorithm SHA256 -LiteralPath $fullPath -ErrorAction Stop).Hash.ToLowerInvariant()
            $files.Add([pscustomobject]@{ path=$relativePath.Replace('\','/'); sha256=$hash })
        } else {
            $files.Add([pscustomobject]@{ path=$relativePath.Replace('\','/'); sha256=$null; state='deleted-or-unavailable' })
        }
    }
    return [pscustomobject]@{ dirty=$dirty; files=@($files) }
}

function Get-VerificationOutcome {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)][int]$ExitCode,
        [Parameter(Mandatory)][int]$Executed,
        [Parameter(Mandatory)][int]$Passed,
        [Parameter(Mandatory)][int]$Failed,
        [Parameter(Mandatory)][int]$Skipped,
        [int]$MinimumExecuted = 1
    )
    if ($Executed -lt 0 -or $Passed -lt 0 -or $Failed -lt 0 -or $Skipped -lt 0 -or $MinimumExecuted -lt 0) { return 'FAIL' }
    if (($Passed + $Failed + $Skipped) -ne $Executed) { return 'FAIL' }
    if ($ExitCode -ne 0 -or $Failed -gt 0) { return 'FAIL' }
    if ($Executed -lt $MinimumExecuted -or $Passed -eq 0) { return 'NOT RUN' }
    return 'PASS'
}

function Get-VerificationExitCode {
    [CmdletBinding()]
    param([Parameter(Mandatory)][ValidateSet('PASS','FAIL','NOT RUN','RESOURCE_LIMITED','DEPENDENCY_BLOCKED','IMPLEMENTED_NOT_VERIFIED')][string]$Status)
    if ($Status -eq 'PASS') { return 0 }
    if ($Status -eq 'FAIL') { return 1 }
    return 3
}

function Get-ToolVersionRecord {
    [CmdletBinding()]
    param([Parameter(Mandatory)][ValidateSet('go','powershell')][string]$Tool,[Parameter(Mandatory)][string]$VersionOutput,[string]$MinimumGoVersion)
    if ($Tool -eq 'powershell') {
        if ($VersionOutput -notmatch '^PowerShell\s+\d+\.\d+\.\d+') { return [pscustomobject]@{ status='DEPENDENCY_BLOCKED'; tool_version=$null; reason='tool_version_unparseable' } }
        return [pscustomobject]@{ status='PASS'; tool_version=$VersionOutput; reason=$null }
    }
    if ($VersionOutput -notmatch '\bgo(\d+\.\d+(?:\.\d+)?)\b') { return [pscustomobject]@{ status='DEPENDENCY_BLOCKED'; tool_version=$null; reason='tool_version_unparseable' } }
    $actual = [version]$Matches[1]
    if ($MinimumGoVersion -and $actual -lt [version]$MinimumGoVersion) { return [pscustomobject]@{ status='DEPENDENCY_BLOCKED'; tool_version=$null; reason="go_version_below_module_minimum:$($Matches[1])<$MinimumGoVersion" } }
    return [pscustomobject]@{ status='PASS'; tool_version="go$actual"; reason=$null }
}

function New-EvidenceRunDirectory {
    [CmdletBinding()]
    param([Parameter(Mandatory)][string]$Path,[Parameter(Mandatory)][string]$AllowedRoot)
    $root = [System.IO.Path]::GetFullPath($AllowedRoot).TrimEnd('\')
    $target = [System.IO.Path]::GetFullPath($Path)
    if (-not $target.StartsWith($root + '\', [System.StringComparison]::OrdinalIgnoreCase)) { throw 'Evidence run directory is outside the allowed root.' }
    if (Test-Path -LiteralPath $target) { throw 'Evidence run directory already exists; refusing to overwrite historical evidence.' }
    New-Item -ItemType Directory -Path $target -ErrorAction Stop | Out-Null
    return $target
}

function Write-EvidenceSums {
    [CmdletBinding()]
    param([Parameter(Mandatory)][string]$Directory,[Parameter(Mandatory)][string[]]$FileNames)
    $root = [System.IO.Path]::GetFullPath($Directory).TrimEnd('\')
    $lines = [System.Collections.Generic.List[string]]::new()
    foreach ($name in $FileNames) {
        if ([System.IO.Path]::IsPathRooted($name) -or $name -match '(^|[\\/])\.\.([\\/]|$)' -or $name -match '[\\/]') { throw 'Evidence hash entries must be plain file names inside the run directory.' }
        $path = Join-Path $root $name
        if (-not (Test-Path -LiteralPath $path -PathType Leaf)) { throw "Evidence file is missing: $name" }
        $hash = (Get-FileHash -Algorithm SHA256 -LiteralPath $path -ErrorAction Stop).Hash.ToLowerInvariant()
        $lines.Add("$hash  $name")
    }
    $sumPath = Join-Path $root 'SHA256SUMS'
    [System.IO.File]::WriteAllText($sumPath, ($lines -join "`n") + "`n", [System.Text.UTF8Encoding]::new($false))
    return $sumPath
}

function Test-EvidenceSums {
    [CmdletBinding()]
    param([Parameter(Mandatory)][string]$Directory)
    $root = [System.IO.Path]::GetFullPath($Directory).TrimEnd('\')
    $sumPath = Join-Path $root 'SHA256SUMS'
    if (-not (Test-Path -LiteralPath $sumPath -PathType Leaf)) { return $false }
    $lines = @(Get-Content -LiteralPath $sumPath -ErrorAction Stop)
    if ($lines.Count -eq 0) { return $false }
    $seen = [System.Collections.Generic.HashSet[string]]::new([System.StringComparer]::OrdinalIgnoreCase)
    foreach ($line in $lines) {
        if ($line -notmatch '^([a-fA-F0-9]{64})  ([^\\/:]+)$') { return $false }
        $expected = $Matches[1].ToLowerInvariant(); $name = $Matches[2]
        if (-not $seen.Add($name)) { return $false }
        $path = Join-Path $root $name
        if (-not (Test-Path -LiteralPath $path -PathType Leaf)) { return $false }
        $actual = (Get-FileHash -Algorithm SHA256 -LiteralPath $path -ErrorAction Stop).Hash.ToLowerInvariant()
        if ($actual -ne $expected) { return $false }
    }
    return $true
}

Export-ModuleMember -Function Get-ChangedSourceInventory,Get-VerificationOutcome,Get-VerificationExitCode,Get-ToolVersionRecord,New-EvidenceRunDirectory,Write-EvidenceSums,Test-EvidenceSums
