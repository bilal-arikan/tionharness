# TionHarness - git worktree helper.
#
# Manages sibling worktrees so you can develop several branches at once without
# stash/checkout churn. Each worktree shares the single .git store but has its
# own working dir + branch.
#
# Keep this file ASCII-only (Windows PowerShell 5.1 mis-decodes UTF-8 no-BOM).
#
# Usage (from the main repo, or anywhere):
#   .\scripts\worktree.ps1 add   feat-login          # new branch feature/feat-login
#   .\scripts\worktree.ps1 add   hotfix -Branch fix/crash -Existing   # attach existing branch
#   .\scripts\worktree.ps1 list
#   .\scripts\worktree.ps1 remove feat-login
#   .\scripts\worktree.ps1 prune
#
# Layout: worktrees are created as siblings of the repo, e.g.
#   C:\Users\user\Desktop\Projects\TionHarness-feat-login
#
# On "add" it also links frontend/node_modules from the main repo (junction) so
# you skip a fresh npm install; pass -NoLink to run npm install instead.

param(
    [Parameter(Position = 0)][string]$Action = "list",
    [Parameter(Position = 1)][string]$Name,
    [string]$Branch,        # explicit branch name (default: feature/<Name>)
    [switch]$Existing,      # attach an existing branch instead of creating one
    [switch]$NoLink         # run "npm install" instead of junctioning node_modules
)

$ErrorActionPreference = "Stop"

# Resolve the MAIN repo root (the worktree whose .git is a real dir, not a file).
$repoRoot = (git rev-parse --show-toplevel 2>$null)
if (-not $repoRoot) { Write-Error "Not inside a git repo."; exit 1 }
$mainRoot = (git worktree list --porcelain |
    Select-String '^worktree ' | Select-Object -First 1).Line -replace '^worktree ', ''
$mainRoot = $mainRoot -replace '/', '\'
$repoName = Split-Path $mainRoot -Leaf
$parent   = Split-Path $mainRoot -Parent

function Get-WorktreePath($n) { Join-Path $parent "$repoName-$n" }

switch ($Action.ToLower()) {

    "add" {
        if (-not $Name) { Write-Error "Name required: worktree.ps1 add <name>"; exit 1 }
        $wtPath = Get-WorktreePath $Name
        if (Test-Path $wtPath) { Write-Error "Already exists: $wtPath"; exit 1 }
        $br = if ($Branch) { $Branch } else { "feature/$Name" }

        Write-Host "==> Creating worktree: $wtPath  (branch: $br)" -ForegroundColor Cyan
        if ($Existing) {
            git worktree add $wtPath $br
        } else {
            git worktree add $wtPath -b $br
        }

        # Speed up frontend: share node_modules from the main repo via a junction.
        $srcNM = Join-Path $mainRoot "frontend\node_modules"
        $dstNM = Join-Path $wtPath   "frontend\node_modules"
        if (-not $NoLink -and (Test-Path $srcNM)) {
            Write-Host "==> Linking frontend/node_modules (junction)" -ForegroundColor Cyan
            cmd /c mklink /J "`"$dstNM`"" "`"$srcNM`"" | Out-Null
        } elseif (Test-Path (Join-Path $wtPath "frontend\package.json")) {
            Write-Host "==> npm install in worktree frontend..." -ForegroundColor Cyan
            Push-Location (Join-Path $wtPath "frontend"); npm install; Pop-Location
        }

        Write-Host "OK. cd `"$wtPath`"" -ForegroundColor Green
    }

    "list" {
        git worktree list
    }

    "remove" {
        if (-not $Name) { Write-Error "Name required: worktree.ps1 remove <name>"; exit 1 }
        $wtPath = Get-WorktreePath $Name
        # Drop the junction first so "git worktree remove" doesn't chase it.
        $dstNM = Join-Path $wtPath "frontend\node_modules"
        if (Test-Path $dstNM) {
            $item = Get-Item $dstNM -Force
            if ($item.LinkType -eq "Junction") { cmd /c rmdir "`"$dstNM`"" | Out-Null }
        }
        Write-Host "==> Removing worktree: $wtPath" -ForegroundColor Cyan
        git worktree remove $wtPath
        Write-Host "OK" -ForegroundColor Green
    }

    "prune" {
        git worktree prune -v
    }

    default {
        Write-Host "Actions: add <name> [-Branch b] [-Existing] [-NoLink] | list | remove <name> | prune"
    }
}
