[CmdletBinding()]
param([Parameter(Mandatory)][string]$BackupDirectory)

$ErrorActionPreference = 'Stop'
$directory = [System.IO.Path]::GetFullPath($BackupDirectory)
if (-not $directory.StartsWith('E:\',[System.StringComparison]::OrdinalIgnoreCase)) { throw 'Backup verification is restricted to E:.' }
if ((Split-Path -Leaf $directory) -notmatch '^pgdump-[a-z0-9-]{8,80}$') { throw 'Directory is not a recognized TaskLab backup run.' }
$manifestPath=Join-Path $directory 'manifest.json'
$dumpPath=Join-Path $directory 'tasklab.dump'
$sumsPath=Join-Path $directory 'SHA256SUMS'
foreach($path in @($manifestPath,$dumpPath,$sumsPath)){if(-not(Test-Path -LiteralPath $path -PathType Leaf)){throw "Required backup evidence file is missing: $(Split-Path -Leaf $path)"}}
$manifest=Get-Content -LiteralPath $manifestPath -Raw|ConvertFrom-Json
$actual=(Get-FileHash -Algorithm SHA256 -LiteralPath $dumpPath).Hash.ToLowerInvariant()
$stream=[System.IO.File]::OpenRead($dumpPath)
try{$signature=[byte[]]::new(5);$read=$stream.Read($signature,0,5)}finally{$stream.Dispose()}
$signatureValid=$read -eq 5 -and [System.Text.Encoding]::ASCII.GetString($signature) -eq 'PGDMP'
$sums=(Get-Content -LiteralPath $sumsPath)|Where-Object{$_ -match '^[a-fA-F0-9]{64}\s+tasklab\.dump$'}
$listedHash=if($sums.Count -eq 1){($sums[0] -split '\s+',2)[0].ToLowerInvariant()}else{''}
$checks=@(
    [pscustomobject]@{name='manifest_status';passed=($manifest.status -eq 'PASS')},
    [pscustomobject]@{name='manifest_dump_hash';passed=($manifest.dump_sha256 -eq $actual)},
    [pscustomobject]@{name='manifest_dump_size';passed=([long]$manifest.dump_bytes -eq (Get-Item -LiteralPath $dumpPath).Length)},
    [pscustomobject]@{name='sha256_sums_entry';passed=($listedHash -eq $actual)},
    [pscustomobject]@{name='postgres_custom_signature';passed=$signatureValid}
)
$failed=@($checks|Where-Object{-not $_.passed})
$report=[ordered]@{status=if($failed.Count -eq 0){'PASS'}else{'FAIL'};backup_directory=$directory;checks=$checks;failure_reasons=@($failed.name);action='read-only verification; no restore or mutation performed'}
$report|ConvertTo-Json -Depth 4
if($failed.Count -gt 0){exit 4}
