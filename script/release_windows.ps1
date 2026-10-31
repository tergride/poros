# Publishes the newest version in CHANGELOG.md as a release of this repository: checks that it can be released, builds it,
# and creates the GitHub release with latest.json, the feed Poros reads. The release stays a draft until both files are
# uploaded, so no Poros ever sees a release it cannot install.
#
#   script\release_windows.ps1          from a clean, pushed main
#   script\release_windows.ps1 -DryRun  checks and builds on any branch, publishes nothing
param([switch]$DryRun)

$ErrorActionPreference = 'Stop'
$repository = 'tergride/poros'
$root = Split-Path $PSScriptRoot -Parent
$deployment = Join-Path $root 'deployment\windows-amd64'
$output = Join-Path $root 'deployment\release'
# The archive is flat and holds the program only: updater.exe refuses anything else, and the data folder is never in it.
$programFiles = 'Poros.exe', 'PorosCore.exe', 'libcronet.dll', 'updater.exe'

function Write-Utf8([string]$Path, [string]$Text) {
    [IO.File]::WriteAllText($Path, $Text, [Text.UTF8Encoding]::new($false))
}

Set-Location $root

$changelog = (Get-Content CHANGELOG.md -Raw -Encoding utf8) -replace "`r`n", "`n"
$section = [regex]::Match($changelog, '(?ms)^## (\d+\.\d+\.\d+)[^\n]*\n(.*?)(?=^## |\z)')
if (-not $section.Success) { throw 'CHANGELOG.md has no version section: add "## <version>" first.' }
$version = $section.Groups[1].Value
$notes = $section.Groups[2].Value.Trim()
$notesRu = [regex]::Match($notes, '(?s)<!-- ru -->(.*?)(?=<!-- en -->|\z)').Groups[1].Value.Trim()
$notesEn = [regex]::Match($notes, '(?s)<!-- en -->(.*?)(?=<!-- ru -->|\z)').Groups[1].Value.Trim()
if (-not $notesRu -or -not $notesEn) { throw "CHANGELOG.md: $version needs both <!-- ru --> and <!-- en --> parts." }
$tag = "v$version"

$problems = @()
if ((git rev-parse --abbrev-ref HEAD) -ne 'main') { $problems += 'releases are built from main' }
if (git status --porcelain) { $problems += 'the working tree has changes' }
git fetch origin main --quiet
if ((git rev-parse HEAD) -ne (git rev-parse origin/main)) { $problems += 'HEAD is not what origin/main holds' }
if ($problems -and -not $DryRun) { throw ($problems -join '; ') }
$problems | ForEach-Object { Write-Warning "$_; going on because of -DryRun." }

gh auth status *> $null
if ($LASTEXITCODE -ne 0) { throw 'gh is not signed in to GitHub: run gh auth login.' }
gh release view $tag --repo $repository --json tagName *> $null
if ($LASTEXITCODE -eq 0) { throw "$repository already has ${tag}: add a newer section to CHANGELOG.md." }

$env:INPUT_VERSION = $version
try { & "$PSScriptRoot\build_windows.ps1" }
finally { $env:INPUT_VERSION = $null }

New-Item -ItemType Directory -Force $output | Out-Null
$archive = Join-Path $output "Poros-$version-windows-amd64.zip"
Compress-Archive -Path ($programFiles | ForEach-Object { Join-Path $deployment $_ }) -DestinationPath $archive -Force

$feed = [ordered]@{
    version = $version
    file    = Split-Path $archive -Leaf
    size    = (Get-Item $archive).Length
    sha256  = (Get-FileHash $archive -Algorithm SHA256).Hash.ToLowerInvariant()
    notes   = [ordered]@{ ru = $notesRu; en = $notesEn }
}
$feedPath = Join-Path $output 'latest.json'
Write-Utf8 $feedPath ($feed | ConvertTo-Json -Depth 3)

if ($DryRun) {
    "$tag is built and not published (-DryRun):`n  $archive`n  $feedPath"
    return
}

$notesPath = Join-Path $output 'notes.md'
Write-Utf8 $notesPath "$notes`n"
gh release create $tag --repo $repository --target (git rev-parse HEAD) --title "Poros $version" --notes-file $notesPath --draft $archive $feedPath
if ($LASTEXITCODE -ne 0) { throw "the release was not created; if a draft $tag is left on GitHub, delete it and run again." }
gh release edit $tag --repo $repository --draft=false --latest
if ($LASTEXITCODE -ne 0) { throw "the draft $tag is uploaded but not published; publish it on GitHub." }
"published https://github.com/$repository/releases/tag/$tag"
