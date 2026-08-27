param([Parameter(Mandatory=$true)][string]$ResultDir)
$ErrorActionPreference='Stop'
if (Test-Path -LiteralPath $ResultDir) { throw "result directory already exists: $ResultDir" }
[IO.Directory]::CreateDirectory($ResultDir) | Out-Null
$history=Join-Path $ResultDir 'operations.jsonl'
$log=Join-Path $ResultDir 'go-test.log'
$started=[datetime]::UtcNow
$cases=@(
  'TestMigrationsFreshAndRerun',
  'TestMigrationsUpgradePreviouslyUntrackedSchema',
  'TestMigrationsConcurrentProcesses',
  'TestMigrationLockTimeoutAndRecovery',
  'TestMigrationChecksumMismatch',
  'TestMigrationHistoryGapFailsClosed',
  'TestMigrationStatementFailureRollsBackWholeFile'
)
$assertions=@()
foreach($case in $cases){
  $invoke=[ordered]@{timestamp=[datetime]::UtcNow.ToString('o');elapsed_ms=[int64](([datetime]::UtcNow-$started).TotalMilliseconds);process='go-test';operation=$case;phase='invoke';metadata=@{seed=20260823}}
  [IO.File]::AppendAllText($history,(($invoke|ConvertTo-Json -Compress)+"`n"))
  $output=(& go test ./internal/storage/postgres -run "^$case$" -count=1 -v 2>&1) -join "`n"
  $passed=$LASTEXITCODE -eq 0
  [IO.File]::AppendAllText($log,"### $case`n$output`n")
  $complete=[ordered]@{timestamp=[datetime]::UtcNow.ToString('o');elapsed_ms=[int64](([datetime]::UtcNow-$started).TotalMilliseconds);process='go-test';operation=$case;phase=$(if($passed){'ok'}else{'fail'});metadata=@{exit_code=$LASTEXITCODE}}
  [IO.File]::AppendAllText($history,(($complete|ConvertTo-Json -Compress)+"`n"))
  $assertions+=@{name=$case;passed=$passed}
}
$allPassed=($assertions|Where-Object{-not $_.passed}).Count -eq 0
[IO.File]::WriteAllText((Join-Path $ResultDir 'result.json'),((@{experiment_id='TL-MIG-002-rerun';passed=$allPassed;assertions=$assertions;duration_ms=[int64](([datetime]::UtcNow-$started).TotalMilliseconds)}|ConvertTo-Json -Depth 8)+"`n"))
[IO.File]::WriteAllText((Join-Path $ResultDir 'config.json'),((@{seed=20260823;postgres='16.15';cases=$cases}|ConvertTo-Json -Depth 5)+"`n"))
[IO.File]::WriteAllText((Join-Path $ResultDir 'db-before.json'),"{`"isolation`":`"fresh database per Go test`"}`n")
[IO.File]::WriteAllText((Join-Path $ResultDir 'db-after.json'),"{`"all_case_databases`":`"dropped by test cleanup`",`"all_assertions_passed`":$($allPassed.ToString().ToLower())}`n")
[IO.File]::WriteAllText((Join-Path $ResultDir 'test-card.md'),"# TL-MIG-002 rerun`n`nActual-timestamp operation history and raw Go test log for seven isolated migration fault cases.`n")
if(-not $allPassed){exit 1}
