param(
    [string]$EvidenceRoot = "E:\distributed-task-lab-temp\wave4-artifacts\alert-e2e-runs"
)

$ErrorActionPreference = "Stop"
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
$composeFile = Join-Path $repoRoot "deployments\observability\alert-e2e-compose.yml"
$runId = "alert-" + [Guid]::NewGuid().ToString("N").Substring(0, 12)
$evidenceDir = Join-Path $EvidenceRoot $runId
$project = "tasklab-$runId"
$rng = [System.Security.Cryptography.RandomNumberGenerator]::Create()
$bytes = New-Object byte[] 24
$rng.GetBytes($bytes)
$dbPassword = [Convert]::ToBase64String($bytes).Replace("+", "A").Replace("/", "B").TrimEnd("=")
$listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, 0)
$listener.Start()
$promPort = $listener.LocalEndpoint.Port
$listener.Stop()
New-Item -ItemType Directory -Force -Path $evidenceDir | Out-Null
$env:E2E_POSTGRES_PASSWORD = $dbPassword
$env:E2E_PROMETHEUS_PORT = "$promPort"
$env:GOPROXY = if ($env:GOPROXY) { $env:GOPROXY } else { "https://proxy.golang.org,direct" }

function Invoke-Compose {
	param([string[]]$ComposeArgs)
	& docker compose --project-name $project --file $composeFile @ComposeArgs
	if ($LASTEXITCODE -ne 0) { throw "docker compose failed ($LASTEXITCODE): $($ComposeArgs -join ' ')" }
}

function Get-Alerts {
    Invoke-RestMethod -Uri "http://127.0.0.1:$promPort/api/v1/alerts" -TimeoutSec 3
}

function Wait-Condition([scriptblock]$Condition, [int]$TimeoutSeconds, [string]$Description) {
    $deadline = [DateTime]::UtcNow.AddSeconds($TimeoutSeconds)
    while ([DateTime]::UtcNow -lt $deadline) {
        try { if (& $Condition) { return } } catch { }
        Start-Sleep -Milliseconds 1000
    }
    throw "Timed out waiting for $Description after ${TimeoutSeconds}s"
}

try {
    Invoke-Compose -ComposeArgs @("up", "--build", "--wait", "--wait-timeout", "240")
    Wait-Condition {
        $targets = Invoke-RestMethod -Uri "http://127.0.0.1:$promPort/api/v1/targets" -TimeoutSec 3
        @($targets.data.activeTargets | Where-Object { $_.labels.job -eq "tasklab-api" -and $_.health -eq "up" }).Count -eq 1
    } 60 "API target to become healthy"
    Wait-Condition {
        $alerts = Get-Alerts
        @($alerts.data.alerts | Where-Object { $_.labels.alertname -eq "TasklabAPIScrapeDown" -and $_.state -eq "firing" }).Count -eq 0
    } 20 "initial normal state"
    Add-Content -Path (Join-Path $evidenceDir "state-transitions.txt") -Value "NORMAL: API scrape target healthy; TasklabAPIScrapeDown not firing."

    Invoke-Compose -ComposeArgs @("stop", "api")
    Wait-Condition {
        $alerts = Get-Alerts
        @($alerts.data.alerts | Where-Object { $_.labels.alertname -eq "TasklabAPIScrapeDown" -and $_.state -eq "firing" }).Count -gt 0
    } 60 "TasklabAPIScrapeDown to fire after API outage"
    Add-Content -Path (Join-Path $evidenceDir "state-transitions.txt") -Value "FIRING: API stopped; TasklabAPIScrapeDown observed firing."

    Invoke-Compose -ComposeArgs @("up", "-d", "--no-deps", "api")
    Wait-Condition {
        $targets = Invoke-RestMethod -Uri "http://127.0.0.1:$promPort/api/v1/targets" -TimeoutSec 3
        @($targets.data.activeTargets | Where-Object { $_.labels.job -eq "tasklab-api" -and $_.health -eq "up" }).Count -eq 1
    } 60 "API target to recover"
    Wait-Condition {
        $alerts = Get-Alerts
        @($alerts.data.alerts | Where-Object { $_.labels.alertname -eq "TasklabAPIScrapeDown" -and $_.state -eq "firing" }).Count -eq 0
    } 30 "alert to resolve after API recovery"
    Add-Content -Path (Join-Path $evidenceDir "state-transitions.txt") -Value "RESOLVED: API scrape healthy again; TasklabAPIScrapeDown no longer firing."

    $targets = Invoke-RestMethod -Uri "http://127.0.0.1:$promPort/api/v1/targets"
    $alerts = Get-Alerts
    $rules = Invoke-RestMethod -Uri "http://127.0.0.1:$promPort/api/v1/rules"
    $targets | ConvertTo-Json -Depth 12 | Set-Content (Join-Path $evidenceDir "targets-final.json")
    $alerts | ConvertTo-Json -Depth 12 | Set-Content (Join-Path $evidenceDir "alerts-final.json")
    $rules | ConvertTo-Json -Depth 16 | Set-Content (Join-Path $evidenceDir "rules-loaded.json")
    (git -C $repoRoot rev-parse HEAD) | Set-Content (Join-Path $evidenceDir "source-commit.txt")
    "Prometheus alert state E2E PASS. Evidence: $evidenceDir"
}
catch {
    $_ | Out-String | Set-Content (Join-Path $evidenceDir "failure.txt")
    throw
}
finally {
    try {
        & docker compose --project-name $project --file $composeFile logs --no-color api prometheus 2>&1 |
            Set-Content (Join-Path $evidenceDir "container-logs.txt")
        & docker compose --project-name $project --file $composeFile down --volumes --remove-orphans 2>&1 |
            Set-Content (Join-Path $evidenceDir "cleanup.txt")
    }
    finally {
        Remove-Item Env:E2E_POSTGRES_PASSWORD -ErrorAction SilentlyContinue
        Remove-Item Env:E2E_PROMETHEUS_PORT -ErrorAction SilentlyContinue
        $rng.Dispose()
    }
}
