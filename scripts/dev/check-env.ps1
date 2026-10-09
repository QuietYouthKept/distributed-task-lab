param(
    [string]$PostgresHost = "127.0.0.1",
    [int]$PostgresPort = 5433,
    [string]$MinioEndpoint = $env:MINIO_ENDPOINT
)

$ErrorActionPreference = "Continue"
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
Set-Location $repoRoot
$failures = 0

function Report([string]$Name, [string]$State, [string]$Detail) {
    Write-Output ("{0,-8} {1,-24} {2}" -f $State, $Name, $Detail)
    if ($State -eq "FAIL") { $script:failures++ }
}

$go = & go version 2>$null
if ($LASTEXITCODE -eq 0) { Report "Go" "PASS" $go } else { Report "Go" "FAIL" "go executable unavailable" }

$branch = & git branch --show-current 2>$null
if ($LASTEXITCODE -eq 0 -and $branch -eq "codex/phase1-production-hardening") {
    Report "Git branch" "PASS" $branch
} else {
    Report "Git branch" "FAIL" ("expected codex/phase1-production-hardening; got " + $branch)
}

$docker = & docker info --format '{{.ServerVersion}}' 2>$null
if ($LASTEXITCODE -eq 0 -and $docker) { Report "Docker Engine" "PASS" $docker } else { Report "Docker Engine" "FAIL" "Docker daemon unavailable" }

$pgContainer = & docker ps --filter "name=^tasklab-test-postgres$" --format "{{.Names}}" 2>$null
if ($pgContainer -eq "tasklab-test-postgres") { Report "PostgreSQL container" "PASS" "tasklab-test-postgres is running" } else { Report "PostgreSQL container" "NOT VERIFIED" "expected container is not running" }
$tcp = Test-NetConnection -ComputerName $PostgresHost -Port $PostgresPort -InformationLevel Quiet -WarningAction SilentlyContinue
if ($tcp) { Report "PostgreSQL TCP" "PASS" ("{0}:{1}" -f $PostgresHost, $PostgresPort) } else { Report "PostgreSQL TCP" "FAIL" ("{0}:{1} is unreachable" -f $PostgresHost, $PostgresPort) }
if ($env:TEST_DATABASE_URL -or $env:DATABASE_URL) { Report "PostgreSQL DSN" "PASS" "a DSN is configured (value suppressed)" } else { Report "PostgreSQL DSN" "NOT VERIFIED" "set TEST_DATABASE_URL or use the test helper default" }

if (-not $MinioEndpoint) { $MinioEndpoint = "http://127.0.0.1:19000" }
try {
    $health = Invoke-WebRequest -Uri (($MinioEndpoint.TrimEnd('/')) + "/minio/health/live") -TimeoutSec 3 -UseBasicParsing
    if ($health.StatusCode -eq 200) { Report "MinIO health" "PASS" $MinioEndpoint } else { Report "MinIO health" "FAIL" ("HTTP " + $health.StatusCode) }
} catch { Report "MinIO health" "FAIL" ("unreachable: " + $MinioEndpoint) }

$missing = @("MINIO_BUCKET", "MINIO_ACCESS_KEY", "MINIO_SECRET_KEY") | Where-Object { -not [Environment]::GetEnvironmentVariable($_) }
if ($MinioEndpoint -and $missing.Count -eq 0) {
    Report "MinIO credentials" "PASS" "required variables are present (values suppressed)"
} else {
    Report "MinIO credentials" "NOT VERIFIED" ("required variables absent: " + ($missing -join ", "))
}

$network = & docker network ls --filter "name=^tasklab-wave2-net$" --format "{{.Name}}" 2>$null
if ($network -eq "tasklab-wave2-net") { Report "E2E network" "PASS" $network } else { Report "E2E network" "NOT VERIFIED" "tasklab-wave2-net absent" }
if ($failures -gt 0) { exit 1 }
exit 0
