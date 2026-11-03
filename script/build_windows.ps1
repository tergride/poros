#Requires -Version 7.0
# Builds the portable Windows folder: the core, then the app. The tools come from one folder, see the README.
param(
    [string]$Toolchain = (Join-Path (Split-Path (Split-Path $PSScriptRoot -Parent) -Parent) 'toolchain')
)

$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
$out = Join-Path $root 'deployment\windows-amd64'

# A full-load build freezes the machine: leave a tenth of the processors free at below normal priority.
$jobs = [int][Math]::Max(1, [Math]::Floor([Environment]::ProcessorCount * 0.9))
(Get-Process -Id $PID).PriorityClass = 'BelowNormal'
(Get-Process -Id $PID).ProcessorAffinity = [IntPtr]((1L -shl $jobs) - 1)

$env:GOPATH = "$Toolchain\gopath"
$env:GOCACHE = "$Toolchain\gocache"
$env:GOFLAGS = "-p=$jobs"
$env:RUSTUP_HOME = "$Toolchain\rust\rustup"
$env:CARGO_HOME = "$Toolchain\rust\cargo"
$env:CARGO_BUILD_JOBS = "$jobs"
# With HTTP/2 multiplexing cargo hung on the crates.io index behind a VPN tunnel.
$env:CARGO_HTTP_MULTIPLEXING = 'false'
$env:npm_config_cache = "$Toolchain\npm-cache"
$env:PATH = "$Toolchain\go\go\bin;$Toolchain\rust\cargo\bin;$Toolchain\node;$env:PATH"

function Invoke-Step([string]$name, [scriptblock]$step) {
    Write-Host "== $name"
    & $step
    if ($LASTEXITCODE -ne 0) { throw "$name failed with exit code $LASTEXITCODE" }
}

if (Test-Path $out) { Remove-Item -Recurse -Force $out }
New-Item -ItemType Directory -Force $out | Out-Null

$tags = 'with_quic,with_wireguard,with_utls,with_dhcp,with_openvpn,with_openconnect,badlinkname,tfogo_checklinkname0,with_purego,with_naive_outbound'
$env:GOOS = 'windows'; $env:GOARCH = 'amd64'; $env:CGO_ENABLED = '0'

Push-Location (Join-Path $root 'core')
try {
    # The interface is built from these types: a change to core/engine/api.go reaches it here.
    Invoke-Step 'types' { go tool tygo generate }
    $cronet = 'github.com/sagernet/cronet-go/lib/windows_amd64'
    Invoke-Step 'cronet' { go mod download $cronet }
    Copy-Item (Join-Path (go list -m -f '{{.Dir}}' $cronet) 'libcronet.dll') $out
    $singbox = go list -m -f '{{.Version}}' github.com/sagernet/sing-box
    $ldflags = "-w -s -X 'github.com/sagernet/sing-box/constant.Version=$singbox' -X 'internal/godebug.defaultGODEBUG=multipathtcp=0' -checklinkname=0"
    Invoke-Step 'core' { go build -o "$out\PorosCore.exe" -trimpath -ldflags $ldflags -tags $tags }
    Invoke-Step 'updater' { go build -o "$out\updater.exe" -trimpath -ldflags '-w -s -H=windowsgui' ./cmd/updater }
}
finally {
    Pop-Location
}

Push-Location $root
try {
    if (-not (Test-Path 'node_modules')) { Invoke-Step 'npm ci' { npm ci } }
    Invoke-Step 'app' { npx tauri build --no-bundle }
    Copy-Item 'src-tauri\target\release\Poros.exe' $out
}
finally {
    Pop-Location
}

Write-Host "== done: $out"
Get-ChildItem $out | Select-Object Name, Length | Format-Table -AutoSize
