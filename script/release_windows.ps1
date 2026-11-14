#Requires -Version 7.0
# Publishes the newest version in CHANGELOG.md as a release of tergride/poros: checks that it can be released, builds
# it, and creates the GitHub release with the archive and latest.json, the feed Poros reads. The release stays a draft
# until both files are uploaded, so no Poros ever sees a release it cannot install. A full release of a version that
# came out as a beta from the same commit makes that pre-release the latest release, without a new build.
#
#   script\release_windows.ps1          a full release from a clean main that origin/main holds
#   script\release_windows.ps1 -Beta    a pre-release from a clean dev that origin/dev holds; only Poros with beta
#                                       versions on finds it
#   script\release_windows.ps1 -DryRun  checks and builds on any branch, publishes nothing
#
# Each form takes -Toolchain <folder>, the folder of tools build_windows.ps1 builds with.
param(
    [switch]$DryRun,
    [switch]$Beta,
    [Parameter(Mandatory)][string]$Toolchain
)

$ErrorActionPreference = 'Stop'
$repository = 'tergride/poros'
$root = Split-Path $PSScriptRoot -Parent
$deployment = Join-Path $root 'deployment\windows-amd64'
$output = Join-Path $root 'deployment\release'
# The archive is flat: the program, its license and the notices; the data folder is never in it.
$programFiles = 'Poros.exe', 'PorosCore.exe', 'updater.exe', 'libcronet.dll'
$licenseFile = '^(licen[cs]e|copying|notice|unlicense)'
# The license texts in the first part of THIRD-PARTY-NOTICES.txt, for a package without a license file of its own.
$licenseTexts = @{
    'MIT'          = 'MIT License'
    'BSD-3-Clause' = 'BSD 3-Clause License'
    'ISC'          = 'ISC License'
    'Apache-2.0'   = 'Apache License 2.0'
    'MPL-2.0'      = 'Mozilla Public License 2.0'
    'OFL-1.1'      = 'SIL Open Font License 1.1'
}

function Write-Utf8([string]$Path, [string]$Text) {
    [IO.File]::WriteAllText($Path, ($Text -replace "`r`n", "`n"), [Text.UTF8Encoding]::new($false))
}

function Assert-Exit([string]$What) {
    if ($LASTEXITCODE) { throw "$What failed with exit code $LASTEXITCODE" }
}

# A package built into the release: its name with the version, its license files and its license expression.
function New-Package([string]$Name, [string]$Folder, [string]$License, [string[]]$More = @()) {
    $files = @(Get-ChildItem $Folder -File -ErrorAction Ignore | Where-Object Name -Match $licenseFile |
            ForEach-Object FullName) + @($More | Where-Object { $_ } | ForEach-Object { [IO.Path]::GetFullPath($_) })
    [pscustomobject]@{ Name = $Name; Files = @($files | Sort-Object -Unique); License = $License }
}

function Get-RustCrates {
    $manifest = Join-Path $root 'src-tauri\Cargo.toml'
    # Normal dependencies only: build scripts and macros run at build time, tests not at all.
    $built = [Collections.Generic.HashSet[string]]::new([string[]]@(
            cargo tree --manifest-path $manifest --locked -e normal --target x86_64-pc-windows-msvc --prefix none -f '{p}' |
                ForEach-Object { ($_ -split ' ')[0, 1] -join ' ' }))
    Assert-Exit 'cargo tree'
    $metadata = cargo metadata --manifest-path $manifest --locked --format-version 1 --filter-platform x86_64-pc-windows-msvc |
        ConvertFrom-Json -Depth 64
    Assert-Exit 'cargo metadata'
    foreach ($crate in $metadata.packages) {
        # A crate without a source is Poros itself.
        if (-not $crate.source -or -not $built.Contains("$($crate.name) v$($crate.version)")) { continue }
        $folder = Split-Path $crate.manifest_path
        $more = if ($crate.license_file) { Join-Path $folder $crate.license_file }
        New-Package "$($crate.name) $($crate.version)" $folder $crate.license $more
    }
}

function Get-NpmPackages {
    Push-Location $root
    try {
        $folders = npm ls --omit=dev --all --parseable
        Assert-Exit 'npm ls (node_modules has to match package-lock.json: npm ci)'
    }
    finally {
        Pop-Location
    }
    # The first folder is Poros itself.
    foreach ($folder in $folders | Select-Object -Skip 1 | Sort-Object -Unique) {
        $package = Get-Content (Join-Path $folder 'package.json') -Raw -Encoding utf8 | ConvertFrom-Json
        $license = $package.license
        if ($license -and $license -isnot [string]) { $license = $license.type }
        New-Package "$($package.name) $($package.version)" $folder $license
    }
}

function Get-GoModules {
    $cache = go env GOMODCACHE
    # The module cache spells an upper-case letter as '!' and its lower case.
    $cached = { param($path, $version) Join-Path $cache ([regex]::Replace("$path@$version", '[A-Z]', { '!' + $args[0].Value.ToLowerInvariant() })) }
    $modules = [ordered]@{}
    foreach ($binary in 'PorosCore.exe', 'updater.exe') {
        $required = $null
        foreach ($entry in go version -m (Join-Path $deployment $binary)) {
            $fields = $entry.Trim() -split '\s+'
            if ($fields[0] -eq 'dep') {
                $required = "$($fields[1]) $($fields[2])"
                $modules[$required] = @{ Name = $required; Folder = & $cached $fields[1] $fields[2] }
            }
            # A replacement follows the module it replaces: a folder of this repository or another module.
            elseif ($fields[0] -eq '=>' -and $required) {
                if ($fields[1].StartsWith('.')) {
                    $folder = [IO.Path]::GetFullPath((Join-Path $root "core\$($fields[1])"))
                    $copy = [IO.Path]::GetRelativePath($root, $folder) -replace '\\', '/'
                    $modules[$required] = @{ Name = "$required, built from the copy in $copy"; Folder = $folder }
                }
                else {
                    $modules[$required] = @{ Name = "$required => $($fields[1]) $($fields[2])"; Folder = & $cached $fields[1] $fields[2] }
                }
            }
        }
        Assert-Exit "go version -m $binary"
    }
    foreach ($module in $modules.Values) { New-Package $module.Name $module.Folder '' }
}

# Appends one part of the list: each license text once, under the packages it belongs to.
function Add-Packages([Text.StringBuilder]$Text, [string]$Title, [object[]]$Packages) {
    if (-not $Packages) { throw "no packages found for: $Title" }
    $texts = [Collections.Specialized.OrderedDictionary]::new([StringComparer]::Ordinal)
    $borrowed = [Collections.Generic.List[string]]::new()
    foreach ($package in $Packages | Sort-Object Name) {
        foreach ($file in $package.Files) {
            $body = ((Get-Content $file -Raw -Encoding utf8) -replace "`r`n", "`n").Trim()
            if (-not $texts.Contains($body)) { $texts[$body] = [Collections.Generic.List[string]]::new() }
            $texts[$body].Add("$($package.Name), $(Split-Path $file -Leaf)")
        }
        if ($package.Files) { continue }
        $ids = @([regex]::Matches([string]$package.License, '[\w.+-]+').Value | Where-Object { $_ -notin 'AND', 'OR', 'WITH' })
        $known = @($ids | Where-Object { $licenseTexts.Contains($_) })
        # Under "A OR B" one text is enough, under "A AND B" every one is needed.
        $enough = if ($package.License -match '\bAND\b') { $known -and $known.Count -eq $ids.Count } else { $known }
        if (-not $enough) {
            throw "$($package.Name) has no license file, and THIRD-PARTY-NOTICES.txt has no text of its license '$($package.License)'"
        }
        $used = if ($package.License -match '\bAND\b') { $known } else { $known[0] }
        $borrowed.Add("    $($package.Name): $($package.License), $(($used | ForEach-Object { $licenseTexts[$_] }) -join ' and ')")
    }
    $rule = '=' * 78
    $line = '-' * 78
    [void]$Text.Append("`n`n`n$rule`n$Title`n$rule`n")
    if ($borrowed.Count) {
        [void]$Text.Append("`nWithout a license file of their own, under the texts in the first part of this file:`n`n")
        [void]$Text.Append(($borrowed -join "`n") + "`n")
    }
    foreach ($body in $texts.Keys) {
        [void]$Text.Append("`n$line`n$($texts[$body] -join "`n")`n$line`n`n$body`n")
    }
}

# THIRD-PARTY-NOTICES.txt with the Rust crates, npm packages and Go modules of this build appended. Uses the tools that
# build_windows.ps1 put on PATH.
function Write-Notices([string]$Path) {
    $text = [Text.StringBuilder]::new()
    [void]$text.Append(((Get-Content (Join-Path $root 'THIRD-PARTY-NOTICES.txt') -Raw -Encoding utf8) -replace "`r`n", "`n").TrimEnd())
    Add-Packages $text 'Rust crates in Poros.exe' @(Get-RustCrates)
    Add-Packages $text 'npm packages of the interface in Poros.exe, with the dependencies of their build tools' @(Get-NpmPackages)
    Add-Packages $text 'Go modules in PorosCore.exe and updater.exe' @(Get-GoModules)
    Write-Utf8 $Path $text.ToString()
}

Set-Location $root

$changelog = (Get-Content CHANGELOG.md -Raw -Encoding utf8) -replace "`r`n", "`n"
$section = [regex]::Match($changelog, '(?ms)^## (\d+\.\d+\.\d+)[^\n]*\n(.*?)(?=^## |\z)')
if (-not $section.Success) { throw 'CHANGELOG.md has no version section: add "## <version>" first.' }
$version = $section.Groups[1].Value
$notes = $section.Groups[2].Value.Trim()
foreach ($language in 'ru', 'en') {
    if (-not [regex]::Match($notes, "(?s)<!-- $language -->(.*?)(?=<!-- \w+ -->|\z)").Groups[1].Value.Trim()) {
        throw "CHANGELOG.md: $version needs a <!-- $language --> part."
    }
}
$tag = "v$version"
# The file properties of Poros.exe come from the crate's version.
$crateVersion = [regex]::Match((Get-Content src-tauri\Cargo.toml -Raw), '(?m)^version = "([^"]+)"').Groups[1].Value
if ($crateVersion -ne $version) { throw "src-tauri/Cargo.toml has version $crateVersion, CHANGELOG.md has ${version}: make them the same." }

$branch = if ($Beta) { 'dev' } else { 'main' }
$problems = @()
if ((git rev-parse --abbrev-ref HEAD) -ne $branch) { $problems += "$(if ($Beta) { 'betas' } else { 'full releases' }) are built from $branch" }
$status = git status --porcelain
if ($status) { $problems += 'the working tree has changes' }
git fetch origin $branch --quiet
Assert-Exit 'git fetch'
$head = git rev-parse HEAD
if ($head -ne (git rev-parse "origin/$branch")) { $problems += "HEAD is not what origin/$branch holds" }
if ($problems -and -not $DryRun) { throw ($problems -join '; ') }
$problems | ForEach-Object { Write-Warning "$_; going on because of -DryRun." }

gh auth status *> $null
if ($LASTEXITCODE) { throw 'gh is not signed in to GitHub: run gh auth login.' }
$released = gh release view $tag --repo $repository --json isDraft,isPrerelease 2> $null
if (-not $LASTEXITCODE) {
    $released = $released | ConvertFrom-Json
    if ($released.isDraft) { throw "$repository has a draft of ${tag}: delete it on GitHub and run again." }
    if ($Beta -or -not $released.isPrerelease) { throw "$repository already has the release ${tag}: add a newer section to CHANGELOG.md." }
    # The tag of a GitHub release is lightweight; a peeled line comes only with an annotated one.
    $refs = git ls-remote origin "refs/tags/$tag" "refs/tags/$tag^{}"
    Assert-Exit 'git ls-remote'
    $built = (@($refs) | Sort-Object { $_ -notmatch '\^\{\}$' } | Select-Object -First 1) -split '\s+' | Select-Object -First 1
    if ($built -ne $head) {
        throw "the beta $tag was built from $built, and main is at ${head}: add a newer section to CHANGELOG.md for a full release."
    }
    if ($DryRun) {
        "the beta $tag would become the latest full release (-DryRun)"
        return
    }
    gh release edit $tag --repo $repository --prerelease=false --latest
    if ($LASTEXITCODE) { throw "the beta $tag did not become a full release; change it on GitHub." }
    "the beta $tag is now the latest release: https://github.com/$repository/releases/tag/$tag"
    return
}
git ls-remote --exit-code --tags origin "refs/tags/$tag" *> $null
if (-not $LASTEXITCODE) { throw "origin already has the tag $tag without a release: delete it or take a newer version." }
if ($LASTEXITCODE -ne 2) { throw "the tags of origin could not be read: git ls-remote exited with $LASTEXITCODE" }

$env:POROS_VERSION = $version
try { & (Join-Path $PSScriptRoot 'build_windows.ps1') -Toolchain $Toolchain }
finally { Remove-Item Env:POROS_VERSION -ErrorAction Ignore }
Set-Location $root
if (((git status --porcelain) -join "`n") -ne ($status -join "`n")) {
    $message = 'the build changed files of the repository: see git status'
    if (-not $DryRun) { throw $message }
    Write-Warning "$message; going on because of -DryRun."
}

if (Test-Path $output) { Remove-Item -Recurse -Force $output }
New-Item -ItemType Directory -Force $output | Out-Null
$notices = Join-Path $output 'THIRD-PARTY-NOTICES.txt'
Write-Notices $notices
$file = "Poros-$version-windows-amd64.zip"
$archive = Join-Path $output $file
$files = @($programFiles | ForEach-Object { Join-Path $deployment $_ }) + (Join-Path $root 'LICENSE') + $notices
Compress-Archive -Path $files -DestinationPath $archive

$feed = [ordered]@{
    version = $version
    file    = $file
    size    = (Get-Item $archive).Length
    sha256  = (Get-FileHash $archive -Algorithm SHA256).Hash.ToLowerInvariant()
}
$feedPath = Join-Path $output 'latest.json'
Write-Utf8 $feedPath (($feed | ConvertTo-Json) + "`n")

if ($DryRun) {
    "$tag is built and not published (-DryRun):`n  $archive`n  $feedPath"
    return
}

$notesPath = Join-Path $output 'notes.md'
Write-Utf8 $notesPath "$notes`n"
$create = 'release', 'create', $tag, '--repo', $repository, '--target', (git rev-parse HEAD), '--title', "Poros $version",
'--notes-file', $notesPath, '--draft'
if ($Beta) { $create += '--prerelease' }
gh @create $archive $feedPath
if ($LASTEXITCODE) { throw "the release was not created; if a draft $tag is left on GitHub, delete it and run again." }
$publish = 'release', 'edit', $tag, '--repo', $repository, '--draft=false'
if (-not $Beta) { $publish += '--latest' }
gh @publish
if ($LASTEXITCODE) { throw "the draft $tag is uploaded but not published; publish it on GitHub." }
"published https://github.com/$repository/releases/tag/$tag"
