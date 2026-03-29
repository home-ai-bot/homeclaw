# PicoClaw Windows Build Script
# Builds all 3 executables: picoclaw, picoclaw-launcher, picoclaw-launcher-tui

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
