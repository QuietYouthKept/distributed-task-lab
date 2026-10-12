[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]$ComposeEnvFile,
    [Parameter(Mandatory)][string]$ComposeProject,
    [switch]$StopServices,
    [switch]$RemoveContainersAndNetwork,
    [switch]$DeleteEphemeralSecrets,
    [switch]$Apply,
    [string]$ConfirmRunId
)

$ErrorActionPreference = 'Stop'
$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
if (-not $repoRoot.StartsWith('E:\',[System.StringComparison]::OrdinalIgnoreCase)) { throw 'Cleanup is restricted to the dedicated E: worktree.' }
$modulePath = Join-Path $PSScriptRoot 'compose-cleanup.psm1'
Import-Module -Name $modulePath -Force -ErrorAction Stop
$plan = Get-ScopedComposeCleanupPlan -ComposeProject $ComposeProject -ComposeEnvFile $ComposeEnvFile -ComposeFile (Join-Path $repoRoot 'deployments/local-first/compose.core.yml') -StopServices:$StopServices -RemoveContainersAndNetwork:$RemoveContainersAndNetwork -DeleteEphemeralSecrets:$DeleteEphemeralSecrets
$docker = (Get-Command docker -ErrorAction Stop).Source
$executor = {
    param([Parameter(ValueFromRemainingArguments=$true)][string[]]$Arguments)
    & $docker @Arguments
}
$result = Invoke-ScopedComposeCleanup -Plan $plan -Executor $executor -ConfirmRunId $ConfirmRunId -Apply:$Apply
$result | ConvertTo-Json -Depth 8
