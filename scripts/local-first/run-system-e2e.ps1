[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]$ComposeEnvFile,
    [Parameter(Mandatory)][string]$ComposeProject,
    [switch]$Execute,
    [switch]$StopServicesAfter,
    [ValidateRange(15,600)][int]$TimeoutSeconds=120,
    [string]$EvidenceRoot=$(Join-Path $(if (-not [string]::IsNullOrWhiteSpace($env:TASKLAB_ARTIFACT_ROOT)) { $env:TASKLAB_ARTIFACT_ROOT } else { Join-Path ([System.IO.Path]::GetTempPath()) 'distributed-task-lab' }) 'production-readiness\evidence\system-e2e')
)
if ([string]::IsNullOrWhiteSpace($env:TASKLAB_ARTIFACT_ROOT)) { $env:TASKLAB_ARTIFACT_ROOT = Join-Path ([System.IO.Path]::GetTempPath()) 'distributed-task-lab' }
$TaskLabArtifactRoot = [System.IO.Path]::GetFullPath($env:TASKLAB_ARTIFACT_ROOT)


$ErrorActionPreference='Stop'
$repoRoot=[System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
if(-not $repoRoot.StartsWith('E:\',[System.StringComparison]::OrdinalIgnoreCase)){throw 'System E2E is restricted to the E: worktree.'}
if($ComposeProject -notmatch '^tasklab-local-first-(?<run>[0-9]{8}t[0-9]{6}[pm][0-9]{4})$'){throw 'Compose project must include one exact TaskLab Run ID.'}
$runId=$Matches.run
$envPath=[System.IO.Path]::GetFullPath($ComposeEnvFile)
$evidenceRoot=[System.IO.Path]::GetFullPath($EvidenceRoot)
if(-not $envPath.StartsWith('E:\',[System.StringComparison]::OrdinalIgnoreCase) -or -not $evidenceRoot.StartsWith('E:\',[System.StringComparison]::OrdinalIgnoreCase)){throw 'Compose env and E2E evidence must remain under E:.'}
if(-not(Test-Path -LiteralPath $envPath -PathType Leaf)){throw 'Compose env file is missing.'}
Import-Module (Join-Path $PSScriptRoot 'evidence-integrity.psm1') -Force -ErrorAction Stop
Import-Module (Join-Path $PSScriptRoot 'compose-cleanup.psm1') -Force -ErrorAction Stop
$composeFile=Join-Path $repoRoot 'deployments/local-first/compose.core.yml'
$cleanupPlan=Get-ScopedComposeCleanupPlan -ComposeProject $ComposeProject -ComposeEnvFile $envPath -ComposeFile $composeFile
$envValues=@{}
foreach($line in Get-Content -LiteralPath $envPath){if($line -match '^([A-Z0-9_]+)=(.*)$'){$envValues[$Matches[1]]=$Matches[2]}}
$secretDir=[System.IO.Path]::GetFullPath(([string]$envValues.TASKLAB_SECRET_DIR).Replace('/','\'))
$apiTLS=[System.IO.Path]::GetFullPath(([string]$envValues.TASKLAB_API_TLS_DIR).Replace('/','\'))
foreach($path in @($secretDir,$apiTLS)){if(-not $path.StartsWith($(Join-Path $TaskLabArtifactRoot 'production-readiness\secrets\'),[System.StringComparison]::OrdinalIgnoreCase)){throw 'Test credential or CA path is outside the dedicated E: secret root.'}}
foreach($name in @('api-cursor-signing-key','api-database-url','gateway-api-key','artifact-s3-access-key','artifact-s3-secret-key')){if(-not(Test-Path -LiteralPath (Join-Path $secretDir $name) -PathType Leaf)){throw "Required run-scoped secret file is missing: $name"}}
$inventory=Get-ChangedSourceInventory -RepoRoot $repoRoot -GitExe (Get-Command git -ErrorAction Stop).Source
$runDir=New-EvidenceRunDirectory -Path (Join-Path $evidenceRoot "e2e-$runId-$([DateTimeOffset]::UtcNow.ToString('yyyyMMddTHHmmssfffZ'))") -AllowedRoot $evidenceRoot
$inventoryPath=Join-Path $runDir 'source-inventory.json'
[System.IO.File]::WriteAllText($inventoryPath,(ConvertTo-Json -InputObject @($inventory.files) -Depth 5),[System.Text.UTF8Encoding]::new($false))
$inventoryHash=(Get-FileHash -Algorithm SHA256 -LiteralPath $inventoryPath).Hash.ToLowerInvariant()
$logPath=Join-Path $runDir 'raw.log'
$safeLog=[System.Text.StringBuilder]::new()
$status='PLAN_ONLY';$exitCode=0;$stopStatus='NOT_REQUESTED';$failure=$null;$logCollectionStatus='NOT_REQUESTED'
$e2eStarted=$false
$apiResponse=$null;$events=@();$artifactResults=[System.Collections.Generic.List[object]]::new()
$httpClient=$null;$handler=$null;$caCert=$null
function Add-SafeLog([string]$Message){$null=$safeLog.AppendLine($Message)}
function New-TestHttpClient([string]$CAPath){
    $script:caCert=[System.Security.Cryptography.X509Certificates.X509Certificate2]::new($CAPath)
    $script:handler=[System.Net.Http.HttpClientHandler]::new()
    $script:handler.ServerCertificateCustomValidationCallback={param($request,$certificate,$presentedChain,$sslErrors)
        if(($sslErrors -band [System.Net.Security.SslPolicyErrors]::RemoteCertificateNameMismatch) -ne 0 -or ($sslErrors -band [System.Net.Security.SslPolicyErrors]::RemoteCertificateNotAvailable) -ne 0){return $false}
        $chain=[System.Security.Cryptography.X509Certificates.X509Chain]::new()
        try{$chain.ChainPolicy.TrustMode=[System.Security.Cryptography.X509Certificates.X509ChainTrustMode]::CustomRootTrust;$chain.ChainPolicy.CustomTrustStore.Add($script:caCert)|Out-Null;$chain.ChainPolicy.RevocationMode=[System.Security.Cryptography.X509Certificates.X509RevocationMode]::NoCheck;return $chain.Build([System.Security.Cryptography.X509Certificates.X509Certificate2]$certificate)}finally{$chain.Dispose()}
    }.GetNewClosure()
    $script:httpClient=[System.Net.Http.HttpClient]::new($script:handler)
    $script:httpClient.Timeout=[TimeSpan]::FromSeconds(10)
    return $script:httpClient
}
function Invoke-TestJson([string]$Method,[string]$Url,[string]$Token,[object]$Body){
    $request=[System.Net.Http.HttpRequestMessage]::new([System.Net.Http.HttpMethod]::new($Method),$Url)
    if($Token){$request.Headers.Authorization=[System.Net.Http.Headers.AuthenticationHeaderValue]::new('Bearer',$Token)}
    if($null -ne $Body -and $Body.idempotency_key){$request.Headers.Add('Idempotency-Key',[string]$Body.idempotency_key)}
    if($null -ne $Body){$json=ConvertTo-Json -InputObject $Body -Depth 16 -Compress;$request.Content=[System.Net.Http.StringContent]::new($json,[System.Text.Encoding]::UTF8,'application/json')}
    $response=$httpClient.SendAsync($request).GetAwaiter().GetResult()
    $content=$response.Content.ReadAsStringAsync().GetAwaiter().GetResult()
    if(-not $response.IsSuccessStatusCode){$code=[int]$response.StatusCode;$response.Dispose();$request.Dispose();throw "HTTPS request failed with status $code for $Method $Url."}
    $response.Dispose();$request.Dispose()
    if([string]::IsNullOrWhiteSpace($content)){return $null}
    return $content|ConvertFrom-Json
}
function Get-ArtifactContent([string]$Key){
    if($Key -notmatch '^eval/gateway/[a-f0-9]{64}/(?:manifest\.json|case-results\.jsonl)$'){throw 'Artifact key does not match the bounded eval/gateway result namespace.'}
    $scriptBody='set -eu; set -o pipefail; export MC_CONFIG_DIR=/tmp/tasklab-e2e-mc; trap ''rm -rf "$MC_CONFIG_DIR"'' EXIT; mc alias set tasklab http://minio:9000 "$(cat /run/secrets/artifact_s3_access_key)" "$(cat /run/secrets/artifact_s3_secret_key)" >/dev/null; mc cat "tasklab/tasklab-artifacts/' + $Key + '" | base64 | tr -d "\r\n"'
    $docker=(Get-Command docker -ErrorAction Stop).Source
    $output=& $docker compose --project-name $ComposeProject --env-file $envPath -f $composeFile run --rm --pull never --no-deps -T --entrypoint /bin/sh minio-init -c $scriptBody 2>$null
    if($LASTEXITCODE -ne 0){throw "Artifact fetch failed for an exact run-scoped key (exit $LASTEXITCODE)."}
    $encoded=($output -join '').Trim()
    try{$contentBytes=[Convert]::FromBase64String($encoded)}catch{throw 'Fetched artifact is not a valid base64 transport payload.'}
    if($contentBytes.Length -gt 1048576){throw 'Fetched E2E artifact exceeds the 1 MiB acceptance bound.'}
    return ,$contentBytes
}

try{
    $preflightOutput=& (Join-Path $PSScriptRoot 'preflight.ps1') -ComposeEnvFile $envPath -ComposeProject $ComposeProject -RequireImages 2>$null
    $preflight=($preflightOutput -join "`n")|ConvertFrom-Json
    if($preflight.status -ne 'PASS'){
        $status=if(@($preflight.checks|Where-Object{$_.name -eq 'resource_budget' -and $_.status -eq 'FAIL'}).Count -gt 0){'RESOURCE_LIMITED'}else{'DEPENDENCY_BLOCKED'}
        $exitCode=3
        Add-SafeLog "PREFLIGHT=$status FAILURE_REASONS=$(@($preflight.failure_reasons) -join ',')"
    }elseif(-not $Execute){
        $status='PLAN_ONLY';Add-SafeLog 'Preflight passed. No token was requested and no task was submitted; pass -Execute for bounded one-task acceptance.'
    }else{
        $status='RUNNING';$e2eStarted=$true;$caPath=Join-Path $apiTLS 'test-ca.crt'
        if(-not(Test-Path -LiteralPath $caPath -PathType Leaf)){throw 'Run-scoped test CA is missing.'}
        $httpClient=New-TestHttpClient -CAPath $caPath
        $apiPort=[int]$envValues.API_HOST_PORT;$identityPort=[int]$envValues.IDENTITY_HOST_PORT
        if($apiPort -lt 1 -or $apiPort -gt 65535 -or $identityPort -lt 1 -or $identityPort -gt 65535){throw 'Local API/identity port configuration is invalid.'}
        $apiBase="https://127.0.0.1:$apiPort";$identityBase="https://127.0.0.1:$identityPort"
        $ready=Invoke-TestJson -Method Get -Url "$apiBase/ready" -Token '' -Body $null
        if($ready.status -ne 'ready'){throw 'API readiness response is not ready.'}
        $tokenResponse=Invoke-TestJson -Method Post -Url "$identityBase/test/tokens/operator" -Token '' -Body @{}
        $token=[string]$tokenResponse.access_token
        if([string]::IsNullOrWhiteSpace($token)){throw 'Identity fixture returned no operator token.'}
        $idempotencyKey="tasklab-e2e-$runId"
        $payload=@{type='eval';queue='default';priority=0;max_attempts=1;timeout_seconds=45;idempotency_key=$idempotencyKey;payload=@{version='1';kind='eval';data=@{execution_mode='gateway';version='2';dataset_version='local-e2e-v1';dataset_uri='artifact://datasets/local-e2e.jsonl';dataset_sha256=('a'*64);model_profile='staging-sim';prompt_version='e2e-v1';retriever_version='e2e-v1';evaluator_schema_version='2';run_id=$runId;case_start=0;case_end=1}}}
        $created=Invoke-TestJson -Method Post -Url "$apiBase/v1/tasks" -Token $token -Body $payload
        $taskId=[string]$created.id
        if([string]::IsNullOrWhiteSpace($taskId)){throw 'Task submission returned no task ID.'}
        Add-SafeLog "TASK_SUBMITTED id=$taskId idempotency_key=$idempotencyKey"
        $deadline=[DateTimeOffset]::UtcNow.AddSeconds($TimeoutSeconds);$delay=250;$task=$null
        do{
            Start-Sleep -Milliseconds $delay
            $task=Invoke-TestJson -Method Get -Url "$apiBase/v1/tasks/$taskId" -Token $token -Body $null
            if([string]$task.status -in @('success','failed','dlq')){break}
            $delay=[Math]::Min(2000,[int]($delay*1.6))
        }while([DateTimeOffset]::UtcNow -lt $deadline)
        if(-not $task -or $task.status -ne 'success'){throw "Task did not reach success before the bounded $TimeoutSeconds second deadline (last status: $($task.status))."}
        $events=Invoke-TestJson -Method Get -Url "$apiBase/v1/tasks/$taskId/events" -Token $token -Body $null
        if(-not (@($events|Where-Object{$_.event_type -eq 'claimed'}).Count -gt 0) -or -not (@($events|Where-Object{$_.event_type -eq 'succeeded'}).Count -gt 0)){throw 'Task event history lacks claim or successful completion evidence.'}
        if([int]$task.attempt -lt 1 -or [int]$task.lease_version -lt 1 -or -not $task.result){throw 'Successful task response lacks attempt, lease fencing, or result data.'}
        $manifestUri=[string]$task.result.artifacts.eval_manifest_uri;$casesUri=[string]$task.result.artifacts.case_results_uri;$fingerprint=[string]$task.result.artifacts.result_fingerprint
        foreach($artifactUri in @($manifestUri,$casesUri)){if($artifactUri -notmatch '^artifact://(?<key>eval/gateway/[a-f0-9]{64}/(?:manifest\.json|case-results\.jsonl))$'){throw 'Task result has an unexpected artifact URI.'};$artifactKey=$Matches.key;$bytes=Get-ArtifactContent -Key $artifactKey;$hash=[Convert]::ToHexString([System.Security.Cryptography.SHA256]::HashData($bytes)).ToLowerInvariant();$content=[System.Text.Encoding]::UTF8.GetString($bytes);$artifactResults.Add([pscustomobject]@{key=$artifactKey;size_bytes=$bytes.Length;sha256=$hash;verified=$true});if($artifactKey.EndsWith('manifest.json')){$manifestContent=$content|ConvertFrom-Json;if($manifestContent.run_id -ne $runId -or $manifestContent.fingerprint -ne $fingerprint){throw 'Fetched manifest does not match task run ID/fingerprint.'}}else{if($content -notmatch [regex]::Escape($runId)){throw 'Fetched case results do not identify the submitted E2E run.'}}}
        $status='PASS';$exitCode=0
        Add-SafeLog "TASK_FINAL id=$taskId status=$($task.status) attempt=$($task.attempt) lease_version=$($task.lease_version) events=$(@($events).Count) artifacts=$($artifactResults.Count)"
    }
}catch{
    $failure=$_.Exception.Message
    $status='FAIL';$exitCode=1
    Add-SafeLog "FAILURE=$failure"
}finally{
    if($httpClient){$httpClient.Dispose()};if($handler){$handler.Dispose()};if($caCert){$caCert.Dispose()}
    if($StopServicesAfter -and $Execute -and $e2eStarted){
        try{
            $docker=(Get-Command docker -ErrorAction Stop).Source
            $stopPlan=Get-ScopedComposeCleanupPlan -ComposeProject $ComposeProject -ComposeEnvFile $envPath -ComposeFile $composeFile -StopServices
            $dockerExecutor={param([Parameter(ValueFromRemainingArguments=$true)][string[]]$Arguments)& $docker @Arguments}
            $dockerExecutor=$dockerExecutor.GetNewClosure()
            $null=Invoke-ScopedComposeCleanup -Plan $stopPlan -Executor $dockerExecutor -ConfirmRunId $runId -Apply
            $stopStatus='STOPPED_OWNED_SERVICES';Add-SafeLog "SCOPED_STOP=$stopStatus"
        }catch{$stopStatus='STOP_FAILED';$status='FAIL';$exitCode=1;if(-not $failure){$failure=$_.Exception.Message};Add-SafeLog "SCOPED_STOP=STOP_FAILED REASON=$($_.Exception.Message)"}
    }
    if($e2eStarted){
        try{
            $docker=(Get-Command docker -ErrorAction Stop).Source
            $serviceLogs=@(& $docker compose --project-name $ComposeProject --env-file $envPath -f $composeFile logs --since 10m --no-color postgres api worker gateway-simulator minio 2>&1)
            $logsExit=$LASTEXITCODE;$safeServiceLogs=$serviceLogs -join "`n"
            foreach($secretFile in Get-ChildItem -LiteralPath $secretDir -File -Recurse -Force){$secretValue=[System.IO.File]::ReadAllText($secretFile.FullName).TrimEnd("`r","`n");if($secretValue.Length -gt 0){$safeServiceLogs=$safeServiceLogs.Replace($secretValue,'[REDACTED]')}}
            $safeServiceLogs=[regex]::Replace($safeServiceLogs,'(?i)(authorization\s*:\s*bearer\s+)\S+','$1[REDACTED]')
            $safeServiceLogs=[regex]::Replace($safeServiceLogs,'(?i)(postgres(?:ql)?://[^:/\s]+:)[^@\s]+(@)','$1[REDACTED]$2')
            [System.IO.File]::WriteAllText((Join-Path $runDir 'service-logs.redacted.log'),$safeServiceLogs,[System.Text.UTF8Encoding]::new($false))
            if($logsExit -ne 0){throw "Scoped service log collection exited $logsExit."}
            $logCollectionStatus='REDACTED_SCOPED_LOGS_COLLECTED';Add-SafeLog "SERVICE_LOG_COLLECTION=$logCollectionStatus"
        }catch{$logCollectionStatus='FAILED';$status='FAIL';$exitCode=1;if(-not $failure){$failure=$_.Exception.Message};Add-SafeLog "SERVICE_LOG_COLLECTION=FAILED REASON=$($_.Exception.Message)"}
    }
    $commit=(& git -C $repoRoot rev-parse HEAD).Trim();$tree=(& git -C $repoRoot rev-parse 'HEAD^{tree}').Trim()
    $manifest=[ordered]@{run_id=$runId;project=$ComposeProject;status=$status;source_commit=$commit;source_tree=$tree;dirty_at_start=$inventory.dirty;source_inventory='source-inventory.json';source_inventory_sha256=$inventoryHash;command='bounded eval/gateway API -> PostgreSQL -> Worker -> Gateway Simulator -> MinIO artifact read';exit_code=$exitCode;timeout_seconds=$TimeoutSeconds;stop_services_after=$stopStatus;service_log_collection=$logCollectionStatus;service_logs='service-logs.redacted.log';task_id=$(if($taskId){$taskId}else{$null});task_status=$(if($task){[string]$task.status}else{$null});attempt=$(if($task){[int]$task.attempt}else{$null});lease_version=$(if($task){[int]$task.lease_version}else{$null});event_types=@($events|ForEach-Object{[string]$_.event_type});artifacts=@($artifactResults);failure=$failure;token_recorded=$false;secret_values_recorded=$false;cross_process_trace='NOT ASSERTED';production_approval='NOT_APPROVED'}
    [System.IO.File]::WriteAllText($logPath,$safeLog.ToString(),[System.Text.UTF8Encoding]::new($false))
    [System.IO.File]::WriteAllText((Join-Path $runDir 'manifest.json'),($manifest|ConvertTo-Json -Depth 10),[System.Text.UTF8Encoding]::new($false))
    $sumFiles=@(Get-ChildItem -LiteralPath $runDir -File|Where-Object{$_.Name -ne 'SHA256SUMS'}|ForEach-Object{$_.Name})
    $null=Write-EvidenceSums -Directory $runDir -FileNames $sumFiles
}
Write-Output "STATUS=$status RUN_ID=$runId EVIDENCE=$runDir TASK_ID=$(if($taskId){$taskId}else{'none'}) STOP=$stopStatus"
exit $exitCode
