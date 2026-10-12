[CmdletBinding()]
param(
    [ValidateSet('unit', 'compile', 'race', 'container')]
    [string]$Workload = 'container',
    [switch]$ReportOnly
)

$ErrorActionPreference = 'Stop'
$minimumRAMGiB = switch ($Workload) {
    'unit' { 0.0 }
    'compile' { 4.0 }
    'race' { 4.0 }
    'container' { 6.0 }
}
$maximumTimeWait = 5000
$minimumEFreeGiB = 20.0
$maximumCPULoadPercent = 90

$os = Get-CimInstance Win32_OperatingSystem
$availableRAMGiB = [math]::Round($os.FreePhysicalMemory / 1MB, 2)
$timeWaitCount = (netstat.exe -ano | Select-String '\sTIME_WAIT\s' | Measure-Object).Count
$eDrive = Get-Volume -DriveLetter E -ErrorAction Stop
$eFreeGiB = [math]::Round($eDrive.SizeRemaining / 1GB, 2)
$processorLoads = @(Get-CimInstance Win32_Processor | ForEach-Object { [int]$_.LoadPercentage })
$cpuLoadPercent = if ($processorLoads.Count -gt 0) { [math]::Round(($processorLoads | Measure-Object -Average).Average, 0) } else { 100 }

$dockerStatus = 'unavailable'
[long]$dockerHostMemoryBytes = 0
[int]$dockerCPUCount = 0
$dockerContainers = @()
$dockerMemoryUsage = @()
try {
    $dockerInfo = & docker info --format '{{.ServerVersion}}|{{.MemTotal}}|{{.NCPU}}' 2>$null
    if ($LASTEXITCODE -eq 0 -and $dockerInfo) {
        $parts = "$dockerInfo".Split('|')
        if ($parts.Count -ge 3) {
            $dockerStatus = "available:$($parts[0])"
            [void][long]::TryParse($parts[1], [ref]$dockerHostMemoryBytes)
            [void][int]::TryParse($parts[2], [ref]$dockerCPUCount)
        }
        $dockerContainers = @(& docker ps --format '{{.ID}} {{.Names}} {{.Status}}' 2>$null)
        if ($LASTEXITCODE -ne 0) { $dockerStatus = 'inventory-failed' }
        if ($dockerStatus -like 'available:*') {
            $dockerMemoryUsage = @(& docker stats --no-stream --format '{{.Name}} {{.MemUsage}} {{.CPUPerc}}' 2>$null)
            if ($LASTEXITCODE -ne 0) { $dockerMemoryUsage = @('stats-unavailable') }
        }
    }
} catch {
    $dockerStatus = 'unavailable'
}

$blockReasons = [System.Collections.Generic.List[string]]::new()
if ($availableRAMGiB -lt $minimumRAMGiB) { $blockReasons.Add("Available RAM ${availableRAMGiB} GiB is below the ${Workload} threshold ${minimumRAMGiB} GiB") }
if ($Workload -eq 'container') {
    if ($timeWaitCount -gt $maximumTimeWait) { $blockReasons.Add("TIME_WAIT $timeWaitCount exceeds $maximumTimeWait") }
    if ($eFreeGiB -lt $minimumEFreeGiB) { $blockReasons.Add("E: free space ${eFreeGiB} GiB is below ${minimumEFreeGiB} GiB") }
    if ($cpuLoadPercent -gt $maximumCPULoadPercent) { $blockReasons.Add("CPU load $cpuLoadPercent% exceeds $maximumCPULoadPercent%") }
    if ($dockerStatus -notlike 'available:*') { $blockReasons.Add("Docker engine is not available ($dockerStatus)") }
} elseif ($Workload -in @('compile', 'race') -and $cpuLoadPercent -gt $maximumCPULoadPercent) {
    $blockReasons.Add("CPU load $cpuLoadPercent% exceeds $maximumCPULoadPercent% for ${Workload}")
}

$snapshot = [ordered]@{
    observed_at_utc = [DateTime]::UtcNow.ToString('o')
    workload = $Workload
    minimum_ram_gib = $minimumRAMGiB
    available_ram_gib = $availableRAMGiB
    time_wait_count = $timeWaitCount
    e_free_gib = $eFreeGiB
    cpu_load_percent = $cpuLoadPercent
    docker_status = $dockerStatus
    docker_host_memory_gib = [math]::Round($dockerHostMemoryBytes / 1GB, 2)
    docker_cpu_count = $dockerCPUCount
    running_containers = $dockerContainers
    docker_container_memory_usage = $dockerMemoryUsage
    workload_allowed = ($blockReasons.Count -eq 0)
    block_reasons = @($blockReasons)
}
$snapshot | ConvertTo-Json -Depth 5

if (-not $ReportOnly -and $blockReasons.Count -gt 0) {
    exit 3
}
