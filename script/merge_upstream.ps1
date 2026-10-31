# Merges upstream into the current branch and stops before the commit, so the result can be built first.
# What Poros removed stays removed; the README and the translations stay its own (.gitattributes). Anything else that collides is listed.
param([string]$Branch = 'dev')

Set-Location (Split-Path $PSScriptRoot -Parent)
git config merge.ours.driver true
git fetch upstream --no-tags
if ($LASTEXITCODE -ne 0) { throw 'fetch failed' }
git merge --no-commit --no-ff "upstream/$Branch"

# DU is "deleted by us" here; `git diff --diff-filter=DU` would mean every unmerged file as well.
git status --porcelain | Where-Object { $_ -match '^DU ' } | ForEach-Object { git rm --quiet -- $_.Substring(3).Trim('"') }
# New upstream files in folders Poros left out: CMakeLists.txt already keeps them out of the build.
git rm -r -f --quiet --ignore-unmatch -- src/api/remote include/api/remote src/scanner include/scanner

$conflicts = @(git diff --name-only --diff-filter=U)
if ($conflicts.Count) {
    "Resolve these, then build and commit:"
    $conflicts
} else {
    'Merged without conflicts. Build it, then commit.'
}
