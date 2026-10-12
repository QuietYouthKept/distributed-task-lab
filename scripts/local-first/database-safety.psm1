Set-StrictMode -Version Latest

function Assert-DisposablePostgresURL {
    [CmdletBinding()]
    param([Parameter(Mandatory)][string]$ConnectionString)
    $uri = $null
    if (-not [System.Uri]::TryCreate($ConnectionString,[System.UriKind]::Absolute,[ref]$uri)) { throw 'PostgreSQL URL is invalid.' }
    $hostName=$uri.DnsSafeHost.Trim('[',']')
    if ($uri.Scheme -notin @('postgres','postgresql') -or $hostName -notin @('localhost','127.0.0.1','::1')) { throw 'Database tests require an explicit loopback PostgreSQL URL.' }
    $authority = ($ConnectionString -replace '^postgres(?:ql)?://','').Split('/')[0]
    if ($authority -notmatch '^(?:[^@/]+@)?(?:\[::1\]|localhost|127(?:\.\d{1,3}){3}):(?<port>\d+)$') { throw 'Database URL must include an explicit TCP port; defaults are not accepted.' }
    $port = 0
    if (-not [int]::TryParse($Matches.port,[ref]$port) -or $port -lt 1 -or $port -gt 65535) { throw 'Database URL port is outside the valid TCP range.' }
    if ([string]::IsNullOrWhiteSpace($uri.UserInfo)) { throw 'Database URL must explicitly identify the maintenance role.' }
    $databaseName = [System.Uri]::UnescapeDataString($uri.AbsolutePath.TrimStart('/'))
    if ($databaseName -notmatch '^tasklab_local_test_admin(?:_[a-zA-Z0-9]+)?$') { throw 'Maintenance database must be tasklab_local_test_admin (or an explicit suffix).' }
    $username = [System.Uri]::UnescapeDataString(($uri.UserInfo -split ':',2)[0])
    return [pscustomobject]@{host=$hostName;port=$port;maintenance_role=$username;database_name=$databaseName;password_present=($uri.UserInfo -match ':')}
}

function Protect-DatabaseTestLog {
    [CmdletBinding()]
    param([Parameter(Mandatory)][string]$Text,[Parameter(Mandatory)][string]$ConnectionString)
    $safe = $Text
    $safe = $safe.Replace($ConnectionString,'[REDACTED_DATABASE_URL]')
    $safe = [regex]::Replace($safe,'(?i)(postgres(?:ql)?://[^:/\s]+:)[^@\s]+(@)','$1[REDACTED]$2')
    return $safe
}

function Resolve-ModuleGoToolchain {
    [CmdletBinding()]
    param([Parameter(Mandatory)][string]$RepoRoot)
    $moduleRoot=[System.IO.Path]::GetFullPath($RepoRoot)
    $goCommand=Get-Command go -ErrorAction Stop
    $goExe=$goCommand.Source
    $directive=Select-String -LiteralPath (Join-Path $moduleRoot 'go.mod') -Pattern '^go\s+(\d+\.\d+(?:\.\d+)?)\s*$'|Select-Object -First 1
    if(-not $directive){throw 'Cannot determine the minimum Go version from go.mod.'}
    $minimum=$directive.Matches[0].Groups[1].Value
    $cacheRoots=[System.Collections.Generic.List[string]]::new()
    if($env:GOMODCACHE){$cacheRoots.Add([string]$env:GOMODCACHE)}
    $cacheRoots.Add((Join-Path (Split-Path $moduleRoot -Parent) 'gomodcache'))
    if($env:USERPROFILE){$cacheRoots.Add((Join-Path $env:USERPROFILE 'go\pkg\mod'))}
    $targetOS='windows';$targetArch=$env:PROCESSOR_ARCHITECTURE.ToLowerInvariant()
    foreach($cacheRoot in $cacheRoots){$candidate=Join-Path $cacheRoot "golang.org\toolchain@v0.0.1-go$minimum.$targetOS-$targetArch\bin\go.exe";if(Test-Path -LiteralPath $candidate -PathType Leaf){$goExe=$candidate;break}}
    $versionOutput=(& $goExe version 2>&1|Out-String).Trim()
    if($versionOutput -notmatch '\bgo(\d+\.\d+(?:\.\d+)?)\b'){throw 'Unable to parse selected Go toolchain version.'}
    if([version]$Matches[1] -lt [version]$minimum){throw "Go toolchain $($Matches[1]) is below go.mod minimum $minimum; no automatic download will be attempted."}
    return [pscustomobject]@{path=$goExe;version="go$($Matches[1])";minimum=$minimum}
}

Export-ModuleMember -Function Assert-DisposablePostgresURL,Protect-DatabaseTestLog,Resolve-ModuleGoToolchain
