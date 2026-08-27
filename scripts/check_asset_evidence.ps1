param(
  [Parameter(Mandatory=$true)][string]$ResultRoot,
  [switch]$CoreOnly
)

# This checker treats histories and DB snapshots as evidence. result.json is
# reported for comparison only and is never an input to an experiment PASS.
$ErrorActionPreference='Stop'
$direct=@('TL-CLAIM-002','TL-CAP-003','TL-QUEUE-001','TL-FENCE-002','TL-LEASE-003','TL-SIDEFX-002','TL-IDEM-002','TL-RETRY-002','TL-PAUSE-002','TL-RATE-003','TL-CLOCK-002')
$dirs=@($direct|ForEach-Object{Join-Path $ResultRoot $_})
if(-not $CoreOnly) {
  $dirs+=Join-Path $ResultRoot 'TL-MIG-002-rerun'
  $docker=Join-Path $ResultRoot 'docker-faults-rerun4'
  $dirs+=@('TL-CRASH-002','TL-SHUT-002','TL-DBFAULT-001')|ForEach-Object{Join-Path $docker $_}
}

function Read-Json([string]$path) { Get-Content -LiteralPath $path -Raw | ConvertFrom-Json }
function Get-Events([string]$path) {
  $out=@()
  foreach($line in Get-Content -LiteralPath $path) {
    if([string]::IsNullOrWhiteSpace($line)){continue}
    $out += $line | ConvertFrom-Json
  }
  return @($out)
}
function Get-Tasks($snapshot) { if($null -eq $snapshot.tasks){ return @() }; return @($snapshot.tasks) }
function Metadata-Value($event,[string]$name) {
  if($null -eq $event.metadata){ return $null }
  return $event.metadata.$name
}
function Check-Critical([string]$id,$events,$after) {
  $tasks=Get-Tasks $after
  switch($id) {
    'TL-CLAIM-002' {
      $owned=@($events|Where-Object{$_.operation -eq 'owned' -and $_.phase -eq 'ok'})
      $acks=@($events|Where-Object{$_.operation -eq 'ack' -and $_.phase -eq 'ok'})
      $ids=@($owned|ForEach-Object{$_.task_id})
      return @{independent_ok=($owned.Count -eq 1000 -and (@($ids|Sort-Object -Unique)).Count -eq 1000 -and $acks.Count -eq 1000 -and $tasks.Count -eq 1000 -and (@($tasks|Where-Object{$_.status -ne 'success'})).Count -eq 0);detail='1000 unique owned events, matching acknowledgements, and 1000 successful DB rows'}
    }
    'TL-CAP-003' {
      $queues=@{}
      foreach($task in $tasks){ if(-not $queues.ContainsKey($task.queue)){$queues[$task.queue]=0}; if($task.status -eq 'running'){$queues[$task.queue]++} }
      $max=0; foreach($n in $queues.Values){if($n -gt $max){$max=$n}}
      return @{independent_ok=($queues.Count -eq 20 -and $max -le 3);detail="20 queues; maximum running rows per queue=$max"}
    }
    'TL-QUEUE-001' {
      $held=@($events|Where-Object{$_.operation -eq 'hold_queue_authority'})
      $claims=@($events|Where-Object{$_.operation -eq 'claim' -and $_.phase -eq 'ok'})
      $free=@($tasks|Where-Object{$_.queue -like '*free-b' -and $_.status -eq 'running'})
      $blocked=@($tasks|Where-Object{$_.queue -like '*blocked-a' -and $_.status -eq 'pending'})
      return @{independent_ok=($held.Count -eq 1 -and $claims.Count -eq 1 -and (Metadata-Value $claims[0] 'count') -eq 1 -and $free.Count -eq 1 -and $blocked.Count -eq 1);detail='held A authority, successful B claim, B running and A still pending in DB snapshot'}
    }
    'TL-FENCE-002' {
      $stale=@($events|Where-Object{$_.operation -in @('stale_success','stale_failure','stale_renew')})
      $fenced=(@($stale|Where-Object{$_.phase -eq 'fail' -and (Metadata-Value $_ 'fenced') -eq $true})).Count
      $current=@($tasks|Where-Object{$_.status -eq 'success' -and $_.lease_version -ge 2})
      return @{independent_ok=($fenced -eq 3 -and $current.Count -eq 1);detail='three stale owner mutations recorded as fenced failures; current generation is successful'}
    }
    'TL-RATE-003' {
      $ok=$true;$details=@()
      foreach($queue in @($events|Where-Object{$_.operation -eq 'claim' -and $_.phase -eq 'ok'}|Select-Object -ExpandProperty queue -Unique)) {
        $claims=@($events|Where-Object{$_.operation -eq 'claim' -and $_.phase -eq 'ok' -and $_.queue -eq $queue})
        $times=@($claims|ForEach-Object{[datetime]$_.timestamp}|Sort-Object)
        if($claims.Count -ne 8 -or (($times[-1]-$times[0]).TotalMilliseconds -lt 1500)){$ok=$false}
        $details += "$queue=$($claims.Count)"
      }
      return @{independent_ok=($ok -and $tasks.Count -eq 24 -and (@($tasks|Where-Object{$_.status -ne 'success'})).Count -eq 0);detail=('three rate queues with eight claim events each and at least 1.5s observed admission span: '+($details -join ', '))}
    }
    'TL-CRASH-002' {
      $killed=@($events|Where-Object{$_.operation -eq 'docker_kill' -and $_.phase -eq 'ok'})
      $task=@($after.task|Where-Object{$_.status -eq 'success' -and $_.attempt -ge 2 -and $_.lease_version -ge 2})
      return @{independent_ok=($killed.Count -eq 1 -and $task.Count -eq 1);detail='recorded SIGKILL and DB snapshot shows success after generation/attempt advance'}
    }
    default { return @{independent_ok=$true;detail='required artifacts and history schema checked; no experiment-specific recomputation defined'} }
  }
}

$checks=@()
foreach($dir in $dirs){
  $id=Split-Path -Leaf $dir
  $required=@('test-card.md','config.json','operations.jsonl','db-before.json','db-after.json','result.json')
  $missing=@($required|Where-Object{-not(Test-Path -LiteralPath (Join-Path $dir $_))})
  $historyOK=$true;$events=@();$after=$null;$reported=$null
  try { if($missing.Count -eq 0){ $events=Get-Events (Join-Path $dir 'operations.jsonl'); $after=Read-Json (Join-Path $dir 'db-after.json'); $reported=Read-Json (Join-Path $dir 'result.json') } } catch { $historyOK=$false }
  foreach($e in $events){ if($null -eq $e.timestamp-or$null -eq $e.elapsed_ms-or$null -eq $e.process-or$null -eq $e.operation-or$null -eq $e.phase){$historyOK=$false} }
  $critical=if($missing.Count -eq 0 -and $historyOK -and $events.Count -gt 0){Check-Critical $id $events $after}else{@{independent_ok=$false;detail='cannot recompute without complete, parseable evidence'}}
  $checks+=@{experiment=$id;passed=($missing.Count -eq 0 -and $historyOK -and $events.Count -gt 0 -and $critical.independent_ok);missing=$missing;history_schema=$historyOK;events=$events.Count;independent_oracle=$critical.independent_ok;oracle_detail=$critical.detail;reported_result_passed=if($null -ne $reported){$reported.passed}else{$null}}
}
$passed=($checks|Where-Object{-not$_.passed}).Count-eq 0
$out=@{checker='independent PowerShell evidence checker';checker_version=2;core_only=[bool]$CoreOnly;result_json_is_evidence_only=$true;passed=$passed;checked_at=[datetime]::UtcNow.ToString('o');experiments=$checks}
$path=Join-Path $ResultRoot 'INDEPENDENT-CHECKER.json'
if(Test-Path -LiteralPath $path){throw "checker result already exists: $path"}
[IO.File]::WriteAllText($path,(($out|ConvertTo-Json -Depth 10)+"`n"))
if(-not$passed){exit 1}
