[CmdletBinding()]
param(
    [Parameter(Mandatory)]
    [ValidateNotNullOrEmpty()]
    [string]$OutputDirectory,
    [string]$OpenSSL = 'openssl'
)

$ErrorActionPreference = 'Stop'
$resolvedOutput = [System.IO.Path]::GetFullPath($OutputDirectory)
if (-not $resolvedOutput.StartsWith('E:\', [System.StringComparison]::OrdinalIgnoreCase)) {
    throw 'Test certificates and private keys must be written under E:\.'
}
if (Test-Path -LiteralPath $resolvedOutput) {
    throw 'Output directory already exists; choose a fresh per-run directory so existing credentials are never overwritten.'
}
$opensslCommand = Get-Command $OpenSSL -ErrorAction Stop
New-Item -ItemType Directory -Path $resolvedOutput -Force | Out-Null

$caKey = Join-Path $resolvedOutput 'test-ca.key'
$caCert = Join-Path $resolvedOutput 'test-ca.crt'
$caConfig = Join-Path $resolvedOutput 'ca-extensions.cnf'
@'
[req]
prompt = no
distinguished_name = ca_dn
x509_extensions = v3_ca

[ca_dn]
CN = TaskLab Local Test CA

[v3_ca]
basicConstraints = critical,CA:TRUE
keyUsage = critical,keyCertSign,cRLSign
subjectKeyIdentifier = hash
'@ | Set-Content -LiteralPath $caConfig -Encoding ascii
& $opensslCommand.Source genrsa -out $caKey 2048 | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'OpenSSL failed to generate the local test CA key.' }
& $opensslCommand.Source req -x509 -new -key $caKey -config $caConfig -sha256 -days 30 -out $caCert | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'OpenSSL failed to generate the local test CA certificate.' }

foreach ($name in @('identity', 'api')) {
$extensionFile = Join-Path $resolvedOutput "$name-extensions.cnf"
$subjectAltNames = "DNS.1 = $name"
if ($name -eq 'identity') {
    $subjectAltNames += "`nDNS.2 = localhost`nIP.1 = 127.0.0.1"
}elseif ($name -eq 'api') {
    $subjectAltNames += "`nDNS.2 = localhost`nIP.1 = 127.0.0.1"
}
@"
[server_cert]
basicConstraints = critical,CA:FALSE
keyUsage = critical,digitalSignature,keyEncipherment
extendedKeyUsage = serverAuth
subjectAltName = @alt_names

[alt_names]
$subjectAltNames
"@ | Set-Content -LiteralPath $extensionFile -Encoding ascii

    $key = Join-Path $resolvedOutput "$name.key"
    $request = Join-Path $resolvedOutput "$name.csr"
    $cert = Join-Path $resolvedOutput "$name.crt"
    & $opensslCommand.Source genrsa -out $key 2048 | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "OpenSSL failed to generate the $name private key." }
    & $opensslCommand.Source req -new -key $key -config $caConfig -subj "/CN=$name" -out $request | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "OpenSSL failed to create the $name certificate request." }
    & $opensslCommand.Source x509 -req -in $request -CA $caCert -CAkey $caKey -CAcreateserial -sha256 -days 7 -extfile $extensionFile -extensions server_cert -out $cert | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "OpenSSL failed to sign the $name test certificate." }
    Remove-Item -LiteralPath $request -Force
}

$identity = [System.Security.Principal.WindowsIdentity]::GetCurrent().Name
& icacls.exe $resolvedOutput /inheritance:r /grant:r "${identity}:(OI)(CI)F" | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Could not restrict access to the local test certificate directory.' }
Write-Output "Created short-lived local TLS test credentials under $resolvedOutput; private keys are not for production."
