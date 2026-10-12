[CmdletBinding()]
param(
    [Parameter(Mandatory)][ValidateSet('static','unit','race','vet','integration','e2e','failure','security','evidence-audit','retention-fixture','evidence-integrity','compose-cleanup','database-safety','backup-manifest','supply-chain-static')][string]$Suite,
    [string]$EvidenceRoot = $(Join-Path $(if (-not [string]::IsNullOrWhiteSpace($env:TASKLAB_ARTIFACT_ROOT)) { $env:TASKLAB_ARTIFACT_ROOT } else { Join-Path ([System.IO.Path]::GetTempPath()) 'distributed-task-lab' }) 'production-readiness\evidence\deep-engineering')
)
if ([string]::IsNullOrWhiteSpace($env:TASKLAB_ARTIFACT_ROOT)) { $env:TASKLAB_ARTIFACT_ROOT = Join-Path ([System.IO.Path]::GetTempPath()) 'distributed-task-lab' }
$TaskLabArtifactRoot = [System.IO.Path]::GetFullPath($env:TASKLAB_ARTIFACT_ROOT)


$ErrorActionPreference = 'Stop'
$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
$evidenceRoot = [System.IO.Path]::GetFullPath($EvidenceRoot)
if (-not $repoRoot.StartsWith('E:\', [System.StringComparison]::OrdinalIgnoreCase) -or -not $evidenceRoot.StartsWith('E:\', [System.StringComparison]::OrdinalIgnoreCase)) {
    throw 'Worktree and evidence output must both be under E:\.'
}
$integrityModule = Join-Path $PSScriptRoot 'evidence-integrity.psm1'
Import-Module -Name $integrityModule -Force -ErrorAction Stop
$git = (Get-Command git -ErrorAction Stop).Source
$go = Get-Command go -ErrorAction SilentlyContinue
$goExe = if($go){$go.Source}else{$null}
$runId = [DateTimeOffset]::UtcNow.ToString('yyyyMMddTHHmmssfffZ')
$runDir = Join-Path $evidenceRoot "$Suite-$runId"
$null = New-EvidenceRunDirectory -Path $runDir -AllowedRoot $evidenceRoot
$log = Join-Path $runDir 'raw.log'
$status = 'NOT RUN'
$exitCode = $null
$actual = 0; $passed = 0; $failed = 0; $skipped = 0
$command = ''
$limitations = [System.Collections.Generic.List[string]]::new()
$commit = (& $git -C $repoRoot rev-parse HEAD).Trim()
if ($LASTEXITCODE -ne 0 -or $commit -notmatch '^[a-fA-F0-9]{40,64}$') { throw 'Cannot resolve source commit; evidence runner fails closed.' }
$tree = (& $git -C $repoRoot rev-parse 'HEAD^{tree}').Trim()
if ($LASTEXITCODE -ne 0 -or $tree -notmatch '^[a-fA-F0-9]{40,64}$') { throw 'Cannot resolve source tree; evidence runner fails closed.' }
$inventoryResult = Get-ChangedSourceInventory -RepoRoot $repoRoot -GitExe $git
$dirty = [bool]$inventoryResult.dirty
$sourceInventory = @($inventoryResult.files)
$inventoryPath = Join-Path $runDir 'source-inventory.json'
$inventoryJson = ConvertTo-Json -InputObject $sourceInventory -Depth 5
[System.IO.File]::WriteAllText($inventoryPath, $inventoryJson, [System.Text.UTF8Encoding]::new($false))
$inventoryHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $inventoryPath).Hash.ToLowerInvariant()
$toolVersion = "PowerShell $($PSVersionTable.PSVersion)"
$goVersionRecord = $null
if ($Suite -in @('unit','race','vet')) {
    $goDirective=Select-String -LiteralPath (Join-Path $repoRoot 'go.mod') -Pattern '^go\s+(\d+\.\d+(?:\.\d+)?)\s*$'|Select-Object -First 1
    $goMinimum=if($goDirective){$goDirective.Matches[0].Groups[1].Value}else{$null}
    if($goExe){
        $goEnv=@(& $goExe env GOOS GOARCH 2>$null)
        $targetOS=if($goEnv.Count -gt 0){[string]$goEnv[0]}else{'windows'}
        $targetArch=if($goEnv.Count -gt 1){[string]$goEnv[1]}else{$env:PROCESSOR_ARCHITECTURE.ToLowerInvariant()}
        $cacheRoots=[System.Collections.Generic.List[string]]::new()
        $reportedCache=(& $goExe env GOMODCACHE 2>$null|Select-Object -First 1)
        if($reportedCache){$cacheRoots.Add([string]$reportedCache)}
        $cacheRoots.Add((Join-Path (Split-Path $repoRoot -Parent) 'gomodcache'))
        if($goMinimum){foreach($cacheRoot in $cacheRoots){$candidate=Join-Path $cacheRoot "golang.org\toolchain@v0.0.1-go$goMinimum.$targetOS-$targetArch\bin\go.exe";if(Test-Path -LiteralPath $candidate -PathType Leaf){$goExe=$candidate;break}}}
        $goVersionOutput=(& $goExe version 2>&1|Out-String).Trim()
        $goVersionRecord=Get-ToolVersionRecord -Tool go -VersionOutput $goVersionOutput -MinimumGoVersion $goMinimum
        $toolVersion=$goVersionRecord.tool_version
    }else{$goVersionRecord=[pscustomobject]@{status='DEPENDENCY_BLOCKED';tool_version=$null;reason='go_unavailable'}}
}
$resource = $null

switch ($Suite) {
    'unit' {
        $resourceRaw = & (Join-Path $PSScriptRoot 'check-resources.ps1') -Workload compile -ReportOnly 2>$null
        try { $resource = ($resourceRaw -join "`n") | ConvertFrom-Json } catch { }
        if (-not $resource -or -not $resource.workload_allowed -or $resource.available_ram_gib -lt 4) {
            $status='RESOURCE_LIMITED'; $limitations.Add('Unit suite not launched; compile resource guard rejected or available RAM is below 4 GiB.')
            $command='scripts/local-first/check-resources.ps1 -Workload compile -ReportOnly; no Go test command launched'
            $exitCode=3; [System.IO.File]::WriteAllText($log, $command, [System.Text.UTF8Encoding]::new($false)); break
        }
        if (-not $goExe -or -not $goVersionRecord -or $goVersionRecord.status -ne 'PASS') { $status='DEPENDENCY_BLOCKED'; $exitCode=3; $limitations.Add($(if($goVersionRecord.reason){$goVersionRecord.reason}else{'go_unavailable'})); [System.IO.File]::WriteAllText($log,'Go toolchain gate failed; no test command executed.',[System.Text.UTF8Encoding]::new($false)); break }
        $command = 'go test -p 1 -json ./cmd/migrate ./cmd/oidc-fixture ./cmd/gateway-simulator ./internal/config ./internal/auth ./internal/domain/taskpayload ./internal/api/... ./internal/observability ./internal/worker/handlers ./pkg/client'
        $stdoutPath = Join-Path $runDir 'go.stdout.jsonl'; $stderrPath = Join-Path $runDir 'go.stderr.log'
        $start = [System.Diagnostics.ProcessStartInfo]::new()
        $start.FileName = $goExe; $start.WorkingDirectory = $repoRoot; $start.Arguments = 'test -p 1 -json ./cmd/migrate ./cmd/oidc-fixture ./cmd/gateway-simulator ./internal/config ./internal/auth ./internal/domain/taskpayload ./internal/api/... ./internal/observability ./internal/worker/handlers ./pkg/client'
        $start.UseShellExecute = $false; $start.CreateNoWindow = $true
        $start.Environment['GOTOOLCHAIN']='local'
        $start.Environment['GOMAXPROCS']='1'
        $start.RedirectStandardOutput = $true; $start.RedirectStandardError = $true
        $process = [System.Diagnostics.Process]::new(); $process.StartInfo = $start
        $null = $process.Start()
        $stdoutTask = $process.StandardOutput.ReadToEndAsync(); $stderrTask = $process.StandardError.ReadToEndAsync()
        $process.WaitForExit(); $stdout = $stdoutTask.GetAwaiter().GetResult(); $stderr = $stderrTask.GetAwaiter().GetResult()
        [System.IO.File]::WriteAllText($stdoutPath, $stdout, [System.Text.UTF8Encoding]::new($false))
        [System.IO.File]::WriteAllText($stderrPath, $stderr, [System.Text.UTF8Encoding]::new($false))
        [System.IO.File]::WriteAllText($log, "COMMAND=$command`n$stdout`nSTDERR:`n$stderr", [System.Text.UTF8Encoding]::new($false))
        $exitCode = $process.ExitCode
        foreach ($line in ($stdout -split "`r?`n")) {
            if (-not $line) { continue }
            try { $event = $line | ConvertFrom-Json } catch { continue }
            if ($event.Test) {
                switch ($event.Action) {
                    'pass' { $actual++; $passed++ }
                    'fail' { $actual++; $failed++ }
                    'skip' { $actual++; $skipped++ }
                }
            }
        }
        if ($stderr -match 'go\.mod requires go >= .* \(running go ' -or $stderr -match 'toolchain not available') {
            $status='DEPENDENCY_BLOCKED'; $limitations.Add('Go test command reported an unavailable or incompatible toolchain.')
        } else { $status=Get-VerificationOutcome -ExitCode $exitCode -Executed $actual -Passed $passed -Failed $failed -Skipped $skipped -MinimumExecuted 1; if($status -eq 'NOT RUN'){$limitations.Add('No non-skipped test completed; this is not PASS.')} }
    }
    'retention-fixture' {
        $fixtureScript=Join-Path $PSScriptRoot 'test-retention-archive.ps1'
        $powershell=Join-Path $PSHOME 'pwsh.exe'
        if(-not(Test-Path -LiteralPath $powershell -PathType Leaf) -or -not(Test-Path -LiteralPath $fixtureScript -PathType Leaf)){
            $status='DEPENDENCY_BLOCKED';$limitations.Add('PowerShell 7 or the retention fixture script is unavailable.');$exitCode=3
            [System.IO.File]::WriteAllText($log,'Retention fixture not started.',[System.Text.UTF8Encoding]::new($false))
        }else{
            $command='pwsh -NoProfile -File scripts/local-first/test-retention-archive.ps1'
            $start=[System.Diagnostics.ProcessStartInfo]::new();$start.FileName=$powershell;$start.WorkingDirectory=$repoRoot;$start.UseShellExecute=$false;$start.CreateNoWindow=$true;$start.RedirectStandardOutput=$true;$start.RedirectStandardError=$true
            foreach($argument in @('-NoProfile','-File',$fixtureScript)){$start.ArgumentList.Add($argument)}
            $process=[System.Diagnostics.Process]::new();$process.StartInfo=$start;$null=$process.Start()
            $stdoutTask=$process.StandardOutput.ReadToEndAsync();$stderrTask=$process.StandardError.ReadToEndAsync();$process.WaitForExit()
            $stdout=$stdoutTask.GetAwaiter().GetResult();$stderr=$stderrTask.GetAwaiter().GetResult();$exitCode=$process.ExitCode
            [System.IO.File]::WriteAllText($log,"COMMAND=$command`n$stdout`nSTDERR:`n$stderr",[System.Text.UTF8Encoding]::new($false))
            if($stdout -match 'EVIDENCE=(?<path>E:[^\r\n]+)'){
                $fixtureResult=Join-Path $Matches.path 'test-result.json'
                if(Test-Path -LiteralPath $fixtureResult -PathType Leaf){$fixtureManifest=Get-Content -LiteralPath $fixtureResult -Raw|ConvertFrom-Json;$actual=[int]$fixtureManifest.assertions;$passed=if($fixtureManifest.status -eq 'PASS'){$actual}else{0}}
            }
            if($exitCode -ne 0 -or $passed -eq 0){$status='FAIL';$failed=1}else{$status='PASS'}
        }
    }
    'evidence-integrity' {
        $testScript=Join-Path $PSScriptRoot 'test-evidence-integrity.ps1'
        $powershell=Join-Path $PSHOME 'pwsh.exe'
        if(-not(Test-Path -LiteralPath $powershell -PathType Leaf) -or -not(Test-Path -LiteralPath $testScript -PathType Leaf)){
            $status='DEPENDENCY_BLOCKED';$limitations.Add('PowerShell 7 or evidence-integrity self-test is unavailable.');$exitCode=3
            [System.IO.File]::WriteAllText($log,'Evidence integrity self-test not started.',[System.Text.UTF8Encoding]::new($false))
        }else{
            $command='pwsh -NoProfile -File scripts/local-first/test-evidence-integrity.ps1'
            $start=[System.Diagnostics.ProcessStartInfo]::new();$start.FileName=$powershell;$start.WorkingDirectory=$repoRoot;$start.UseShellExecute=$false;$start.CreateNoWindow=$true;$start.RedirectStandardOutput=$true;$start.RedirectStandardError=$true
            foreach($argument in @('-NoProfile','-File',$testScript)){$start.ArgumentList.Add($argument)}
            $process=[System.Diagnostics.Process]::new();$process.StartInfo=$start;$null=$process.Start()
            $stdoutTask=$process.StandardOutput.ReadToEndAsync();$stderrTask=$process.StandardError.ReadToEndAsync();$process.WaitForExit()
            $stdout=$stdoutTask.GetAwaiter().GetResult();$stderr=$stderrTask.GetAwaiter().GetResult();$exitCode=$process.ExitCode
            [System.IO.File]::WriteAllText($log,"COMMAND=$command`n$stdout`nSTDERR:`n$stderr",[System.Text.UTF8Encoding]::new($false))
            if($stdout -match 'ASSERTIONS=(?<count>\d+) PASS=(?<passed>\d+) FAIL=(?<failed>\d+)'){$actual=[int]$Matches.count;$passed=[int]$Matches.passed;$failed=[int]$Matches.failed}
            $status=Get-VerificationOutcome -ExitCode $exitCode -Executed $actual -Passed $passed -Failed $failed -Skipped 0 -MinimumExecuted 1
        }
    }
    'compose-cleanup' {
        $testScript=Join-Path $PSScriptRoot 'test-compose-cleanup.ps1'
        $powershell=Join-Path $PSHOME 'pwsh.exe'
        if(-not(Test-Path -LiteralPath $powershell -PathType Leaf) -or -not(Test-Path -LiteralPath $testScript -PathType Leaf)){
            $status='DEPENDENCY_BLOCKED';$limitations.Add('PowerShell 7 or Compose Cleanup fake-executor tests are unavailable.');$exitCode=3
            [System.IO.File]::WriteAllText($log,'Compose Cleanup fake-executor tests not started.',[System.Text.UTF8Encoding]::new($false))
        }else{
            $command='pwsh -NoProfile -File scripts/local-first/test-compose-cleanup.ps1'
            $start=[System.Diagnostics.ProcessStartInfo]::new();$start.FileName=$powershell;$start.WorkingDirectory=$repoRoot;$start.UseShellExecute=$false;$start.CreateNoWindow=$true;$start.RedirectStandardOutput=$true;$start.RedirectStandardError=$true
            foreach($argument in @('-NoProfile','-File',$testScript)){$start.ArgumentList.Add($argument)}
            $process=[System.Diagnostics.Process]::new();$process.StartInfo=$start;$null=$process.Start()
            $stdoutTask=$process.StandardOutput.ReadToEndAsync();$stderrTask=$process.StandardError.ReadToEndAsync();$process.WaitForExit()
            $stdout=$stdoutTask.GetAwaiter().GetResult();$stderr=$stderrTask.GetAwaiter().GetResult();$exitCode=$process.ExitCode
            [System.IO.File]::WriteAllText($log,"COMMAND=$command`n$stdout`nSTDERR:`n$stderr",[System.Text.UTF8Encoding]::new($false))
            if($stdout -match 'ASSERTIONS=(?<count>\d+) PASS=(?<passed>\d+) FAIL=(?<failed>\d+)'){$actual=[int]$Matches.count;$passed=[int]$Matches.passed;$failed=[int]$Matches.failed}
            $status=Get-VerificationOutcome -ExitCode $exitCode -Executed $actual -Passed $passed -Failed $failed -Skipped 0 -MinimumExecuted 1
        }
    }
    'database-safety' {
        $testScript=Join-Path $PSScriptRoot 'test-database-safety.ps1'
        $powershell=Join-Path $PSHOME 'pwsh.exe'
        if(-not(Test-Path -LiteralPath $powershell -PathType Leaf) -or -not(Test-Path -LiteralPath $testScript -PathType Leaf)){
            $status='DEPENDENCY_BLOCKED';$limitations.Add('PowerShell 7 or database URL safety tests are unavailable.');$exitCode=3
            [System.IO.File]::WriteAllText($log,'Database URL safety tests not started.',[System.Text.UTF8Encoding]::new($false))
        }else{
            $command='pwsh -NoProfile -File scripts/local-first/test-database-safety.ps1'
            $start=[System.Diagnostics.ProcessStartInfo]::new();$start.FileName=$powershell;$start.WorkingDirectory=$repoRoot;$start.UseShellExecute=$false;$start.CreateNoWindow=$true;$start.RedirectStandardOutput=$true;$start.RedirectStandardError=$true
            foreach($argument in @('-NoProfile','-File',$testScript)){$start.ArgumentList.Add($argument)}
            $process=[System.Diagnostics.Process]::new();$process.StartInfo=$start;$null=$process.Start()
            $stdoutTask=$process.StandardOutput.ReadToEndAsync();$stderrTask=$process.StandardError.ReadToEndAsync();$process.WaitForExit()
            $stdout=$stdoutTask.GetAwaiter().GetResult();$stderr=$stderrTask.GetAwaiter().GetResult();$exitCode=$process.ExitCode
            [System.IO.File]::WriteAllText($log,"COMMAND=$command`n$stdout`nSTDERR:`n$stderr",[System.Text.UTF8Encoding]::new($false))
            if($stdout -match 'ASSERTIONS=(?<count>\d+) PASS=(?<passed>\d+) FAIL=(?<failed>\d+)'){$actual=[int]$Matches.count;$passed=[int]$Matches.passed;$failed=[int]$Matches.failed}
            $status=Get-VerificationOutcome -ExitCode $exitCode -Executed $actual -Passed $passed -Failed $failed -Skipped 0 -MinimumExecuted 1
        }
    }
    'backup-manifest' {
        $testScript=Join-Path $PSScriptRoot 'test-backup-manifest.ps1'
        $powershell=Join-Path $PSHOME 'pwsh.exe'
        if(-not(Test-Path -LiteralPath $powershell -PathType Leaf) -or -not(Test-Path -LiteralPath $testScript -PathType Leaf)){
            $status='DEPENDENCY_BLOCKED';$limitations.Add('PowerShell 7 or backup manifest fixture tests are unavailable.');$exitCode=3
            [System.IO.File]::WriteAllText($log,'Backup manifest offline tests not started.',[System.Text.UTF8Encoding]::new($false))
        }else{
            $command='pwsh -NoProfile -File scripts/local-first/test-backup-manifest.ps1'
            $start=[System.Diagnostics.ProcessStartInfo]::new();$start.FileName=$powershell;$start.WorkingDirectory=$repoRoot;$start.UseShellExecute=$false;$start.CreateNoWindow=$true;$start.RedirectStandardOutput=$true;$start.RedirectStandardError=$true
            foreach($argument in @('-NoProfile','-File',$testScript)){$start.ArgumentList.Add($argument)}
            $process=[System.Diagnostics.Process]::new();$process.StartInfo=$start;$null=$process.Start()
            $stdoutTask=$process.StandardOutput.ReadToEndAsync();$stderrTask=$process.StandardError.ReadToEndAsync();$process.WaitForExit()
            $stdout=$stdoutTask.GetAwaiter().GetResult();$stderr=$stderrTask.GetAwaiter().GetResult();$exitCode=$process.ExitCode
            [System.IO.File]::WriteAllText($log,"COMMAND=$command`n$stdout`nSTDERR:`n$stderr",[System.Text.UTF8Encoding]::new($false))
            if($stdout -match 'ASSERTIONS=(?<count>\d+) PASS=(?<passed>\d+) FAIL=(?<failed>\d+)'){$actual=[int]$Matches.count;$passed=[int]$Matches.passed;$failed=[int]$Matches.failed}
            $status=Get-VerificationOutcome -ExitCode $exitCode -Executed $actual -Passed $passed -Failed $failed -Skipped 0 -MinimumExecuted 1
        }
    }
    'supply-chain-static' {
        $testScript=Join-Path $PSScriptRoot 'test-supply-chain-static.ps1'
        $powershell=Join-Path $PSHOME 'pwsh.exe'
        if(-not(Test-Path -LiteralPath $powershell -PathType Leaf) -or -not(Test-Path -LiteralPath $testScript -PathType Leaf)){
            $status='DEPENDENCY_BLOCKED';$limitations.Add('PowerShell 7 or supply-chain static audit is unavailable.');$exitCode=3
            [System.IO.File]::WriteAllText($log,'Supply-chain static audit not started.',[System.Text.UTF8Encoding]::new($false))
        }else{
            $command='pwsh -NoProfile -File scripts/local-first/test-supply-chain-static.ps1'
            $start=[System.Diagnostics.ProcessStartInfo]::new();$start.FileName=$powershell;$start.WorkingDirectory=$repoRoot;$start.UseShellExecute=$false;$start.CreateNoWindow=$true;$start.RedirectStandardOutput=$true;$start.RedirectStandardError=$true
            foreach($argument in @('-NoProfile','-File',$testScript)){$start.ArgumentList.Add($argument)}
            $process=[System.Diagnostics.Process]::new();$process.StartInfo=$start;$null=$process.Start()
            $stdoutTask=$process.StandardOutput.ReadToEndAsync();$stderrTask=$process.StandardError.ReadToEndAsync();$process.WaitForExit()
            $stdout=$stdoutTask.GetAwaiter().GetResult();$stderr=$stderrTask.GetAwaiter().GetResult();$exitCode=$process.ExitCode
            [System.IO.File]::WriteAllText($log,"COMMAND=$command`n$stdout`nSTDERR:`n$stderr",[System.Text.UTF8Encoding]::new($false))
            if($stdout -match 'ASSERTIONS=(?<count>\d+) PASS=(?<passed>\d+) FAIL=(?<failed>\d+)'){$actual=[int]$Matches.count;$passed=[int]$Matches.passed;$failed=[int]$Matches.failed}
            $status=Get-VerificationOutcome -ExitCode $exitCode -Executed $actual -Passed $passed -Failed $failed -Skipped 0 -MinimumExecuted 1
        }
    }
    'static' {
        $command = 'PowerShell AST parse of scripts/local-first/*.ps1 and *.psm1; git diff --check'
        $parseErrors=[System.Collections.Generic.List[string]]::new()
        $files = @(Get-ChildItem -LiteralPath (Join-Path $repoRoot 'scripts/local-first') -File | Where-Object { $_.Extension -in @('.ps1','.psm1') })
        foreach ($file in $files) {
            $fileTokens=$null; $fileErrors=$null
            [void][System.Management.Automation.Language.Parser]::ParseFile($file.FullName,[ref]$fileTokens,[ref]$fileErrors)
            foreach($fileError in @($fileErrors)){$parseErrors.Add($fileError.Message)}
        }
        $diff = & $git -C $repoRoot diff --check 2>&1
        $exitCode = if ($parseErrors.Count -eq 0 -and $LASTEXITCODE -eq 0) { 0 } else { 1 }
        $status = if ($exitCode -eq 0) { 'PASS' } else { 'FAIL' }
        [System.IO.File]::WriteAllText($log, "COMMAND=$command`nFILES=$($files.Count)`nPARSE_ERRORS=$($parseErrors.Count)`n$($parseErrors -join "`n")`n$($diff -join "`n")", [System.Text.UTF8Encoding]::new($false))
    }
    'race' {
        $resourceRaw = & (Join-Path $PSScriptRoot 'check-resources.ps1') -Workload race -ReportOnly 2>$null
        try { $resource = ($resourceRaw -join "`n") | ConvertFrom-Json } catch { }
        $gcc = Get-Command gcc -ErrorAction SilentlyContinue
        if (-not $resource -or -not $resource.workload_allowed -or $resource.available_ram_gib -lt 4) {
            $status='RESOURCE_LIMITED'; $limitations.Add('Race suite not launched; resource guard rejected or available RAM is below 4 GiB.')
            $command='scripts/local-first/check-resources.ps1 -Workload race -ReportOnly; no Race command launched'
            $exitCode=3; [System.IO.File]::WriteAllText($log, $command, [System.Text.UTF8Encoding]::new($false))
        }
        elseif (-not $goExe -or -not $goVersionRecord -or $goVersionRecord.status -ne 'PASS') {
            $status='DEPENDENCY_BLOCKED'; $limitations.Add($(if($goVersionRecord.reason){$goVersionRecord.reason}else{'go_unavailable'}))
            $command='Go toolchain gate; no Race command launched'
            $exitCode=3; [System.IO.File]::WriteAllText($log,$command,[System.Text.UTF8Encoding]::new($false))
        }
        elseif (($cgoEnabled = (& $goExe env CGO_ENABLED 2>$null | Select-Object -First 1)) -ne '1' -or -not $gcc) {
            $status='DEPENDENCY_BLOCKED'; $limitations.Add('Windows Race requires CGO and an installed GCC-compatible C compiler; none was available.')
            $command='Race prerequisite guard; no Race command launched'
            $exitCode=3; [System.IO.File]::WriteAllText($log, "COMMAND=$command`nCGO_ENABLED=$cgoEnabled`nGCC_AVAILABLE=$([bool]$gcc)", [System.Text.UTF8Encoding]::new($false))
        }
        else {
            $command='go test -p 1 -race -json ./cmd/migrate ./cmd/oidc-fixture ./cmd/gateway-simulator ./internal/config ./internal/auth ./internal/domain/taskpayload ./internal/api/... ./internal/observability ./internal/worker/handlers ./pkg/client'
            $stdoutPath=Join-Path $runDir 'go.stdout.jsonl'; $stderrPath=Join-Path $runDir 'go.stderr.log'
            $start=[System.Diagnostics.ProcessStartInfo]::new(); $start.FileName=$goExe; $start.WorkingDirectory=$repoRoot
            $start.Arguments='test -p 1 -race -json ./cmd/migrate ./cmd/oidc-fixture ./cmd/gateway-simulator ./internal/config ./internal/auth ./internal/domain/taskpayload ./internal/api/... ./internal/observability ./internal/worker/handlers ./pkg/client'
            $start.UseShellExecute=$false; $start.CreateNoWindow=$true; $start.RedirectStandardOutput=$true; $start.RedirectStandardError=$true
            $start.Environment['GOTOOLCHAIN']='local'
            $start.Environment['GOMAXPROCS']='1'
            $process=[System.Diagnostics.Process]::new(); $process.StartInfo=$start; $null=$process.Start()
            $stdoutTask=$process.StandardOutput.ReadToEndAsync(); $stderrTask=$process.StandardError.ReadToEndAsync(); $process.WaitForExit()
            $stdout=$stdoutTask.GetAwaiter().GetResult(); $stderr=$stderrTask.GetAwaiter().GetResult()
            [System.IO.File]::WriteAllText($stdoutPath,$stdout,[System.Text.UTF8Encoding]::new($false)); [System.IO.File]::WriteAllText($stderrPath,$stderr,[System.Text.UTF8Encoding]::new($false))
            [System.IO.File]::WriteAllText($log,"COMMAND=$command`n$stdout`nSTDERR:`n$stderr",[System.Text.UTF8Encoding]::new($false)); $exitCode=$process.ExitCode
            foreach($line in ($stdout -split "`r?`n")){if(-not $line){continue};try{$event=$line|ConvertFrom-Json}catch{continue};if($event.Test){switch($event.Action){'pass'{$actual++;$passed++};'fail'{$actual++;$failed++};'skip'{$actual++;$skipped++}}}}
            if($stderr -match 'go\.mod requires go >= .* \(running go '){$status='DEPENDENCY_BLOCKED';$limitations.Add('Go race command reported an incompatible toolchain.')}
            else{$status=Get-VerificationOutcome -ExitCode $exitCode -Executed $actual -Passed $passed -Failed $failed -Skipped $skipped -MinimumExecuted 1;if($status -eq 'NOT RUN'){$limitations.Add('No non-skipped race test completed.')}}
        }
    }
    'vet' {
        $resourceRaw = & (Join-Path $PSScriptRoot 'check-resources.ps1') -Workload compile -ReportOnly 2>$null
        try { $resource = ($resourceRaw -join "`n") | ConvertFrom-Json } catch { }
        if (-not $resource -or -not $resource.workload_allowed -or $resource.available_ram_gib -lt 4) {
            $status='RESOURCE_LIMITED'; $limitations.Add('Vet suite not launched; compile resource guard rejected or available RAM is below 4 GiB.')
            $command='scripts/local-first/check-resources.ps1 -Workload compile -ReportOnly; no Go vet command launched'
            $exitCode=3; [System.IO.File]::WriteAllText($log, $command, [System.Text.UTF8Encoding]::new($false))
        }
        elseif (-not $goExe -or -not $goVersionRecord -or $goVersionRecord.status -ne 'PASS') {
            $status='DEPENDENCY_BLOCKED'; $limitations.Add($(if($goVersionRecord.reason){$goVersionRecord.reason}else{'go_unavailable'}))
            $command='Go toolchain gate; no vet command launched'; $exitCode=3
            [System.IO.File]::WriteAllText($log, $command, [System.Text.UTF8Encoding]::new($false))
        }
        else {
            $command='go vet -p 1 ./...'
            $start=[System.Diagnostics.ProcessStartInfo]::new(); $start.FileName=$goExe; $start.WorkingDirectory=$repoRoot
            $start.Arguments='vet -p 1 ./...'
            $start.UseShellExecute=$false; $start.CreateNoWindow=$true; $start.RedirectStandardOutput=$true; $start.RedirectStandardError=$true
            $start.Environment['GOTOOLCHAIN']='local'; $start.Environment['GOMAXPROCS']='1'
            $process=[System.Diagnostics.Process]::new(); $process.StartInfo=$start; $null=$process.Start()
            $stdoutTask=$process.StandardOutput.ReadToEndAsync(); $stderrTask=$process.StandardError.ReadToEndAsync(); $process.WaitForExit()
            $stdout=$stdoutTask.GetAwaiter().GetResult(); $stderr=$stderrTask.GetAwaiter().GetResult(); $exitCode=$process.ExitCode
            $status=if($exitCode -eq 0){'PASS'}else{'FAIL'}
            [System.IO.File]::WriteAllText($log,"COMMAND=$command`n$stdout`nSTDERR:`n$stderr",[System.Text.UTF8Encoding]::new($false))
        }
    }
    'integration' {
        if (-not $env:TEST_DATABASE_URL) { $status='DEPENDENCY_BLOCKED'; $limitations.Add('TEST_DATABASE_URL is absent; no database tests executed.') }
        else { $status='IMPLEMENTED_NOT_VERIFIED'; $limitations.Add('Use test-db-bootstrap.ps1 or test-db-privileges.ps1 with an isolated loopback tasklab_local_test_admin database.') }
        $command='PowerShell guard: check TEST_DATABASE_URL; no PostgreSQL test command launched'
        $exitCode=3; [System.IO.File]::WriteAllText($log, 'No integration command executed by this suite.', [System.Text.UTF8Encoding]::new($false))
    }
    { $_ -in 'e2e','failure' } {
        $resourceRaw = & (Join-Path $PSScriptRoot 'check-resources.ps1') -Workload container -ReportOnly 2>$null
        $command='scripts/local-first/check-resources.ps1 -Workload container -ReportOnly; no E2E/failure command launched'
        try { $resource = ($resourceRaw -join "`n") | ConvertFrom-Json } catch { }
        if (-not $resource -or -not $resource.workload_allowed) { $status='RESOURCE_LIMITED'; $limitations.Add('Container resource preflight failed; no container command executed.') }
        else { $status='DEPENDENCY_BLOCKED'; $limitations.Add('Run the focused Compose preflight first; this runner never starts a stack implicitly.') }
        $exitCode=3; [System.IO.File]::WriteAllText($log, 'No E2E/failure command executed.', [System.Text.UTF8Encoding]::new($false))
    }
    'security' {
        $tool = Get-Command govulncheck -ErrorAction SilentlyContinue
        $command='PowerShell guard: check govulncheck availability; no scanner command launched'
        if (-not $tool) { $status='DEPENDENCY_BLOCKED'; $limitations.Add('govulncheck is not installed; no download was attempted.') }
        else { $status='IMPLEMENTED_NOT_VERIFIED'; $limitations.Add('Security scans require an explicitly reviewed bounded command; not run automatically.') }
        $exitCode=3; [System.IO.File]::WriteAllText($log, 'No security scanner command executed.', [System.Text.UTF8Encoding]::new($false))
    }
    'evidence-audit' {
        $auditScript=Join-Path $PSScriptRoot 'audit-verification-evidence.ps1'
        $powershell=Join-Path $PSHOME 'pwsh.exe'
        if(-not(Test-Path -LiteralPath $powershell -PathType Leaf) -or -not(Test-Path -LiteralPath $auditScript -PathType Leaf)){
            $status='DEPENDENCY_BLOCKED';$limitations.Add('PowerShell 7 or the evidence audit script is unavailable.');$exitCode=3
            [System.IO.File]::WriteAllText($log,'Evidence audit not started.',[System.Text.UTF8Encoding]::new($false))
        }else{
            $command='pwsh -NoProfile -File scripts/local-first/audit-verification-evidence.ps1'
            $start=[System.Diagnostics.ProcessStartInfo]::new();$start.FileName=$powershell;$start.WorkingDirectory=$repoRoot;$start.UseShellExecute=$false;$start.CreateNoWindow=$true;$start.RedirectStandardOutput=$true;$start.RedirectStandardError=$true
            foreach($argument in @('-NoProfile','-File',$auditScript)){$start.ArgumentList.Add($argument)}
            $process=[System.Diagnostics.Process]::new();$process.StartInfo=$start;$null=$process.Start()
            $stdoutTask=$process.StandardOutput.ReadToEndAsync();$stderrTask=$process.StandardError.ReadToEndAsync();$process.WaitForExit()
            $stdout=$stdoutTask.GetAwaiter().GetResult();$stderr=$stderrTask.GetAwaiter().GetResult();$exitCode=$process.ExitCode
            [System.IO.File]::WriteAllText($log,"COMMAND=$command`n$stdout`nSTDERR:`n$stderr",[System.Text.UTF8Encoding]::new($false))
            if($stdout -match 'RUNS=(?<count>\d+) ISSUES=(?<failed>\d+)'){$actual=[int]$Matches.count;$failed=[int]$Matches.failed;$passed=[Math]::Max(0,$actual-$failed)}
            if($exitCode -ne 0 -or $failed -gt 0){$status='FAIL';if($actual -eq 0){$failed=1};$limitations.Add('Evidence audit found one or more integrity/source-inventory issues; see the report.')}
            elseif($actual -gt 0){$status='PASS';$exitCode=0}
            else{$status='NOT RUN';$limitations.Add('Evidence audit did not identify any manifest rows.')}
        }
    }
}

$resourceSnapshot = if ($resource) { $resource } else { $null }
$manifest = [ordered]@{
    run_id=$runId; suite=$Suite; status=$status; source_commit=$commit; source_tree=$tree
    dirty_at_start=$dirty; exact_command=$command; exit_code=$exitCode
    source_inventory='source-inventory.json'; source_inventory_sha256=$inventoryHash
    tests_executed=$actual; passed=$passed; failed=$failed; skipped=$skipped
    tool_version=$toolVersion; resource_snapshot=$resourceSnapshot; limitations=@($limitations)
    raw_log='raw.log'; log_closed_before_hash=$true
}
$manifestPath=Join-Path $runDir 'manifest.json'
[System.IO.File]::WriteAllText($manifestPath, ($manifest | ConvertTo-Json -Depth 8), [System.Text.UTF8Encoding]::new($false))
$sealedFiles = @(Get-ChildItem -LiteralPath $runDir -File | Where-Object { $_.Name -ne 'SHA256SUMS' } | ForEach-Object { $_.Name })
$null = Write-EvidenceSums -Directory $runDir -FileNames $sealedFiles
if (-not (Test-EvidenceSums -Directory $runDir)) { throw 'Evidence integrity verification failed immediately after sealing.' }
Write-Output "STATUS=$status RUN_ID=$runId EVIDENCE=$runDir TESTS=$actual PASSED=$passed FAILED=$failed SKIPPED=$skipped"
$finalExitCode=Get-VerificationExitCode -Status $status
exit $finalExitCode
