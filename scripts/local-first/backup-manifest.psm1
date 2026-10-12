Set-StrictMode -Version Latest

function Get-JsonField {
    param([Parameter(Mandatory)]$Object,[Parameter(Mandatory)][string]$Name)
    $property=$Object.PSObject.Properties[$Name]
    if ($property) { return $property.Value }
    return $null
}

function Get-SafeArtifactPath {
    param([Parameter(Mandatory)][string]$ObjectRoot,[Parameter(Mandatory)][string]$Bucket,[Parameter(Mandatory)][string]$Key)
    if ($Bucket -notmatch '^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$' -or $Bucket.Contains('..')) { throw 'Artifact bucket name is invalid.' }
    if ([string]::IsNullOrWhiteSpace($Key) -or $Key.StartsWith('/') -or $Key.Contains('\') -or $Key.Contains(':') -or $Key -match '[\x00-\x1f]' ) { throw 'Artifact object key is unsafe.' }
    $segments = $Key.Split('/')
    if (@($segments | Where-Object { $_ -in @('','.','..') }).Count -gt 0) { throw 'Artifact key contains an unsafe path segment.' }
    $root = [System.IO.Path]::GetFullPath($ObjectRoot).TrimEnd('\')
    $path = [System.IO.Path]::GetFullPath((Join-Path $root (Join-Path $Bucket ($Key.Replace('/','\')))))
    if (-not $path.StartsWith($root + '\',[System.StringComparison]::OrdinalIgnoreCase)) { throw 'Artifact path escaped the object root.' }
    return $path
}

function Test-ArtifactInventory {
    [CmdletBinding()]
    param([Parameter(Mandatory)][string]$InventoryPath,[Parameter(Mandatory)][string]$ObjectRoot,[Parameter(Mandatory)][string]$ExpectedBucket,[Parameter(Mandatory)][string]$ExpectedPrefix,[Parameter(Mandatory)][string]$BackupRunId,[Parameter(Mandatory)][DateTimeOffset]$SnapshotBoundary)
    if (-not (Test-Path -LiteralPath $InventoryPath -PathType Leaf)) { throw 'Object inventory file is missing.' }
    $document = Get-Content -LiteralPath $InventoryPath -Raw | ConvertFrom-Json
    if (-not (Get-JsonField -Object $document -Name 'objects')) { throw 'Object inventory has no objects array.' }
    $issues = [System.Collections.Generic.List[string]]::new()
    $seen = [System.Collections.Generic.HashSet[string]]::new([System.StringComparer]::Ordinal)
    $checked = [System.Collections.Generic.List[object]]::new()
    $boundary = $SnapshotBoundary.ToUniversalTime()
    foreach ($item in @($document.objects)) {
        $bucket=[string](Get-JsonField $item 'bucket'); $key=[string](Get-JsonField $item 'key'); $identity="$bucket/$key"
        if (-not $seen.Add($identity)) { $issues.Add("duplicate_object_key:$identity"); continue }
        $expectedPrefix=if([string]::IsNullOrEmpty($ExpectedPrefix)){''}else{$ExpectedPrefix.TrimEnd('/')+'/' }
        if ($bucket -cne $ExpectedBucket -or ($expectedPrefix -and -not $key.StartsWith($expectedPrefix,[System.StringComparison]::Ordinal))) { $issues.Add("object_outside_source_scope:$identity") }
        if ([string](Get-JsonField $item 'backup_run_id') -cne $BackupRunId) { $issues.Add("backup_run_mismatch:$identity") }
        $expectedHash=[string](Get-JsonField $item 'sha256')
        if ($expectedHash -notmatch '^[a-fA-F0-9]{64}$') { $issues.Add("invalid_sha256:$identity") }
        $itemSize=Get-JsonField $item 'size_bytes'
        if ($null -eq $itemSize -or [long]$itemSize -lt 0) { $issues.Add("invalid_size:$identity") }
        $taskReference=[string](Get-JsonField $item 'referencing_task_id');$artifactRole=[string](Get-JsonField $item 'artifact_role')
        if ([string]::IsNullOrWhiteSpace($taskReference) -or [string]::IsNullOrWhiteSpace($artifactRole)) { $issues.Add("missing_reference:$identity") }
        try { $captured=[DateTimeOffset]::Parse([string](Get-JsonField $item 'snapshot_time')).ToUniversalTime(); if ($captured -gt $boundary) { $issues.Add("snapshot_after_boundary:$identity") } }
        catch { $issues.Add("invalid_snapshot_time:$identity"); $captured=[DateTimeOffset]::MinValue }
        try { $objectPath=Get-SafeArtifactPath -ObjectRoot $ObjectRoot -Bucket $bucket -Key $key }
        catch { $issues.Add("unsafe_object_path:$identity"); continue }
        if (-not (Test-Path -LiteralPath $objectPath -PathType Leaf)) { $issues.Add("missing_object:$identity"); continue }
        $file=Get-Item -LiteralPath $objectPath -Force
        if ($file.Attributes -band [System.IO.FileAttributes]::ReparsePoint) { $issues.Add("reparse_point_object:$identity"); continue }
        if ([long]$file.Length -ne [long]$itemSize) { $issues.Add("size_mismatch:$identity") }
        $hash=(Get-FileHash -Algorithm SHA256 -LiteralPath $objectPath).Hash.ToLowerInvariant()
        if ($hash -ne $expectedHash.ToLowerInvariant()) { $issues.Add("sha256_mismatch:$identity") }
        $checked.Add([pscustomobject]@{bucket=$bucket;key=$key;size_bytes=[long]$file.Length;sha256=$hash;referencing_task_id=$taskReference;artifact_role=$artifactRole;snapshot_time=$captured.ToString('o');backup_run_id=$BackupRunId})
    }
    return [pscustomobject]@{status=$(if($issues.Count -eq 0 -and $checked.Count -gt 0){'OFFLINE_PASS'}else{'FAIL'});snapshot_boundary=$boundary.ToString('o');backup_run_id=$BackupRunId;objects_checked=$checked.Count;objects=@($checked);issues=@($issues)}
}

function Assert-RestoreBackupManifest {
    [CmdletBinding()]
    param([Parameter(Mandatory)][string]$ManifestPath,[Parameter(Mandatory)][string]$SourceDatabase,[Parameter(Mandatory)][string]$TargetDatabase,[Parameter(Mandatory)][string]$SourceBucket,[Parameter(Mandatory)][string]$SourcePrefix,[Parameter(Mandatory)][string]$TargetBucket,[Parameter(Mandatory)][string]$TargetPrefix,[Parameter(Mandatory)][int]$SupportedSchemaVersion)
    if (-not (Test-Path -LiteralPath $ManifestPath -PathType Leaf)) { throw 'Backup manifest is missing.' }
    $manifest=Get-Content -LiteralPath $ManifestPath -Raw|ConvertFrom-Json
    if ([int]$manifest.schema_version -ne 1) { throw 'Backup manifest schema version is unsupported.' }
    if ([int]$manifest.database_schema_version -ne $SupportedSchemaVersion) { throw 'Backup database schema version is incompatible with this restore harness.' }
    if ([string]::IsNullOrWhiteSpace([string]$manifest.backup_run_id) -or -not $manifest.snapshot_at) { throw 'Backup manifest identity or snapshot boundary is missing.' }
    if ($SourceDatabase -ceq $TargetDatabase) { throw 'Restore target database must differ from the source database.' }
    if ($TargetBucket -notmatch '^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$' -or $TargetPrefix -match '(^|/)\.\.?(/|$)|\\|:') { throw 'Restore destination bucket or prefix is unsafe.' }
    $source=$SourcePrefix.TrimEnd('/');$target=$TargetPrefix.TrimEnd('/')
    if ($TargetBucket -ceq $SourceBucket -and ($target -ceq $source -or $target.StartsWith($source+'/',[System.StringComparison]::Ordinal) -or $source.StartsWith($target+'/',[System.StringComparison]::Ordinal))) { throw 'Restore destination bucket/prefix overlaps the source object scope.' }
    return $manifest
}

Export-ModuleMember -Function Test-ArtifactInventory,Assert-RestoreBackupManifest
