# PicoClaw Windows Build Script
# Builds all 3 executables: picoclaw, picoclaw-launcher, picoclaw-launcher-tui
# Embeds workspace and config files into the exe

$ErrorActionPreference = "Stop"

# Colors for output
function Write-Color($message, $color) {
    Write-Host $message -ForegroundColor $color
}

# Get script directory and project root
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ProjectRoot = Split-Path -Parent $ScriptDir
$BuildDir = Join-Path $ProjectRoot "build"

# Build configuration
$GoFlags = "-v -tags stdjson"
$LdFlags = "-s -w"

# Workspace paths
$WorkspaceSource = Join-Path $ProjectRoot "homeclaw-workspace"
$OnboardDir = Join-Path $ProjectRoot "cmd\picoclaw\internal\onboard"
$WorkspaceTarget = Join-Path $OnboardDir "workspace"

# Ensure build directory exists
if (!(Test-Path $BuildDir)) {
    New-Item -ItemType Directory -Path $BuildDir | Out-Null
}

Write-Color "`n========================================" "Cyan"
Write-Color "  PicoClaw Windows Build Script" "Cyan"
Write-Color "========================================`n" "Cyan"

# Change to project root
Push-Location $ProjectRoot

try {
    # Step 0: Copy workspace to embed directory (equivalent to go:generate)
    Write-Color "[0/3] Preparing workspace for embedding..." "Magenta"
    
    # Remove existing workspace in onboard directory
    if (Test-Path $WorkspaceTarget) {
        Write-Color "      Removing existing workspace copy..." "Gray"
        Remove-Item -Recurse -Force $WorkspaceTarget
    }
    
    # Copy workspace directory to onboard package for embedding
    if (Test-Path $WorkspaceSource) {
        Write-Color "      Copying workspace to $WorkspaceTarget..." "Gray"
        Copy-Item -Recurse -Force $WorkspaceSource $WorkspaceTarget
        Write-Color "      Workspace prepared for embedding!" "Green"
    } else {
        throw "Workspace source directory not found: $WorkspaceSource"
    }

    # Build 1: picoclaw.exe
    Write-Color "[1/3] Building picoclaw.exe..." "Yellow"
    $env:CGO_ENABLED = "0"
    go build -v -tags stdjson -ldflags "$LdFlags" -o "$BuildDir\picoclaw.exe" .\cmd\picoclaw
    if ($LASTEXITCODE -ne 0) { throw "Failed to build picoclaw.exe" }
    Write-Color "      picoclaw.exe built successfully!" "Green"

    # Build 2: picoclaw-launcher.exe (web backend)
    Write-Color "[2/3] Building picoclaw-launcher.exe..." "Yellow"
    
    # Check if frontend needs to be built
    $FrontendDist = Join-Path $ProjectRoot "web\backend\dist\index.html"
    if (!(Test-Path $FrontendDist)) {
        Write-Color "      Frontend not found, building..." "Magenta"
        Push-Location (Join-Path $ProjectRoot "web\frontend")
        try {
            npm install
            if ($LASTEXITCODE -ne 0) { throw "npm install failed" }
            npm run build:backend
            if ($LASTEXITCODE -ne 0) { throw "npm run build:backend failed" }
        } finally {
            Pop-Location
        }
        Write-Color "      Frontend built successfully!" "Magenta"
    }
    
    go build -v -tags stdjson -ldflags "$LdFlags" -o "$BuildDir\picoclaw-launcher.exe" .\web\backend
    if ($LASTEXITCODE -ne 0) { throw "Failed to build picoclaw-launcher.exe" }
    Write-Color "      picoclaw-launcher.exe built successfully!" "Green"

    # Build 3: picoclaw-launcher-tui.exe
    Write-Color "[3/3] Building picoclaw-launcher-tui.exe..." "Yellow"
    go build -v -tags stdjson -ldflags "$LdFlags" -o "$BuildDir\picoclaw-launcher-tui.exe" .\cmd\picoclaw-launcher-tui
    if ($LASTEXITCODE -ne 0) { throw "Failed to build picoclaw-launcher-tui.exe" }
    Write-Color "      picoclaw-launcher-tui.exe built successfully!" "Green"

    # Summary
    Write-Color "`n========================================" "Cyan"
    Write-Color "  Build Complete!" "Green"
    Write-Color "========================================" "Cyan"
    Write-Color "`nOutput directory: $BuildDir" "White"
    Write-Color "`nBuilt executables:" "White"
    
    Get-ChildItem "$BuildDir\*.exe" | ForEach-Object {
        $size = [math]::Round($_.Length / 1MB, 2)
        Write-Color "  - $($_.Name) ($size MB)" "Gray"
    }
    
    Write-Color "`n" "White"

} catch {
    Write-Color "`nBuild failed: $_" "Red"
    exit 1
} finally {
    Pop-Location
}
