# Local Windows x64 build, same steps as .github/workflows/build.yml. Output: deployment\windows-amd64.
param(
    [switch]$Core,
    [switch]$Gui,
    [string]$Toolchain = (Join-Path (Split-Path (Split-Path $PSScriptRoot -Parent) -Parent) 'toolchain')
)

$ErrorActionPreference = 'Stop'
if (-not $Core -and -not $Gui) { $Core = $true; $Gui = $true }

$root = Split-Path $PSScriptRoot -Parent
$dest = Join-Path $root 'deployment\windows-amd64'
$cmakeHome = (Get-ChildItem "$Toolchain\cmake" -Directory | Select-Object -First 1).FullName

$env:GOPATH = "$Toolchain\gopath"
$env:GOCACHE = "$Toolchain\gocache"
$env:PATH = "$Toolchain\go\go\bin;$Toolchain\gopath\bin;$Toolchain\protoc\bin;$Toolchain\ninja;$cmakeHome\bin;$env:PATH"

function Invoke-Checked([scriptblock]$Command) {
    & $Command
    if ($LASTEXITCODE -ne 0) { throw "failed with exit code ${LASTEXITCODE}: $Command" }
}

# What ships must not read as a VPN or Throne from its names and properties, or name this machine's paths.
function Test-ShippedIdentity([string]$Directory) {
    $problems = [Collections.Generic.List[string]]::new()
    $latin1 = [Text.Encoding]::GetEncoding(28591)
    $roots = (Split-Path $root -Parent), ((Split-Path $root -Parent) -replace '\\', '/')
    foreach ($file in Get-ChildItem $Directory -File) {
        if ($file.Name -match '(?i)vpn|throne') { $problems.Add("file name: $($file.Name)") }
        if ($file.Extension -ne '.exe') { continue }
        $info = $file.VersionInfo
        $properties = ($info.ProductName, $info.FileDescription, $info.CompanyName, $info.LegalCopyright, $info.OriginalFilename, $info.InternalName) -join ' | '
        if ($properties -match '(?i)vpn|throne') { $problems.Add("$($file.Name) properties: $properties") }
        $text = $latin1.GetString([IO.File]::ReadAllBytes($file.FullName))
        foreach ($path in $roots) {
            if ($text.IndexOf($path, [StringComparison]::OrdinalIgnoreCase) -ge 0) { $problems.Add("$($file.Name) contains the local path $path") }
        }
    }
    if ($problems.Count) { throw "shipped identity check failed:`n$($problems -join "`n")" }
}

Push-Location $root
try {
    if ($Core) {
        Invoke-Checked { go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.12 }
        Invoke-Checked { go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.6.2 }
        $env:GOOS = 'windows'
        $env:GOARCH = 'amd64'
        $env:DEST = $dest -replace '\\', '/'
        Invoke-Checked { & "$env:ProgramFiles\Git\bin\bash.exe" ./script/build_go.sh }
    }

    if ($Gui) {
        $vswhere = "${env:ProgramFiles(x86)}\Microsoft Visual Studio\Installer\vswhere.exe"
        $vcvars = & $vswhere -latest -products * -requires Microsoft.VisualStudio.Component.VC.Tools.x86.x64 -find 'VC\Auxiliary\Build\vcvars64.bat'
        if (-not $vcvars) { throw 'MSVC not found: install the VS 2022 Build Tools C++ workload' }
        cmd /c "`"$vcvars`" >nul && set" | ForEach-Object {
            $name, $value = $_ -split '=', 2
            if ($name) { Set-Item "env:$name" $value }
        }

        Get-ChildItem "$Toolchain\dashboard-src" -Force | Where-Object Name -ne '.git' | Copy-Item -Destination res\dashboard -Recurse -Force
        New-Item -ItemType Directory -Force build | Out-Null
        if (-not (Test-Path build\srslist.h)) {
            Invoke-Checked { curl.exe -fLso build\srslist.h https://raw.githubusercontent.com/throneproj/routeprofiles/rule-set/srslist.h }
        }

        $env:CMAKE_PREFIX_PATH = "$Toolchain\Qt\lib\cmake"
        $env:OPENSSL_ROOT_DIR = "$Toolchain\openssl"
        Invoke-Checked { cmake -S . -B build -GNinja -DCMAKE_BUILD_TYPE=RelWithDebInfo }
        Invoke-Checked { ninja -C build }

        New-Item -ItemType Directory -Force $dest | Out-Null
        Copy-Item build\Poros.exe, build\Poros.pdb $dest -Force
    }
    Test-ShippedIdentity $dest
}
finally {
    Pop-Location
}
