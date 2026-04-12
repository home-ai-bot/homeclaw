# Build-Android.ps1 - PowerShell 5.1 Compatible Version
# Compiles Homeclaw for Android platforms

param(
    [ValidateSet("arm64", "arm", "amd64")]
    [string]$Architecture = "arm64",
    
    [string]$OutputDir = "",
    
    [switch]$AllArchitectures,
    
    [switch]$SkipGateway,
    
    [switch]$SkipWeb
)

$ErrorActionPreference = "Stop"

# Set console output encoding to UTF-8
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$OutputEncoding = [System.Text.Encoding]::UTF8

# Project root directory
$ProjectRoot = Split-Path -Parent $PSScriptRoot
$BuildDir = Join-Path $ProjectRoot "build"

function Write-Step {
    param([string]$Message, [string]$Color = "Cyan")
    Write-Host ""
    Write-Host "[$Message]" -ForegroundColor $Color
}

function Write-Success {
    param([string]$Message)
    Write-Host "[OK] $Message" -ForegroundColor Green
}

function Write-Fail {
    param([string]$Message)
    Write-Host "[FAIL] $Message" -ForegroundColor Red
}

function Test-GoCommand {
    try {
        $null = Get-Command go -ErrorAction Stop
        $version = go version
        Write-Step "Go detected: $version" "Green"
        return $true
    }
    catch {
        Write-Fail "Go not found. Please install Go 1.25.8+"
        return $false
    }
}

function Build-Architecture {
    param(
        [string]$Arch,
        [string]$ArchLabel
    )
    
    Write-Step "Building Android $ArchLabel ($Arch)"
    
    # Set output directory
    if ($OutputDir -eq "") {
        $ArchOutputDir = Join-Path $BuildDir "android\$Arch"
    } else {
        $ArchOutputDir = $OutputDir
    }
    
    # Create output directory
    if (-not (Test-Path $ArchOutputDir)) {
        New-Item -ItemType Directory -Path $ArchOutputDir -Force | Out-Null
    }
    
    # Set environment variables
    $env:CGO_ENABLED = "0"
    $env:GOOS = "linux"
    $env:GOARCH = $Arch
    if ($Arch -eq "arm") {
        $env:GOARM = "7"
    } else {
        Remove-Item Env:\GOARM -ErrorAction SilentlyContinue
    }
    
    # Build Gateway
    if (-not $SkipGateway) {
        Write-Step "  Building Gateway (libhomeclaw.so)..." "Yellow"
        $GatewayOutput = Join-Path $ArchOutputDir "libhomeclaw.so"
        
        $StartTime = Get-Date
        & go build -v -tags goolm,stdjson -ldflags "-s -w" -o $GatewayOutput (Join-Path $ProjectRoot "cmd\picoclaw")
        $Duration = (Get-Date) - $StartTime
        
        if ($LASTEXITCODE -eq 0) {
            $FileSize = [math]::Round((Get-Item $GatewayOutput).Length / 1MB, 2)
            Write-Success "  Gateway built: ${FileSize}MB (took: $($Duration.TotalSeconds.ToString('0.0'))s)"
        } else {
            Write-Fail "  Gateway build failed"
            return $false
        }
    }
    
    # Build Web Console
    if (-not $SkipWeb) {
        Write-Step "  Building Web Console (libhomeclaw-web.so)..." "Yellow"
        $WebOutput = Join-Path $ArchOutputDir "libhomeclaw-web.so"
        $WebBackendDir = Join-Path $ProjectRoot "web\backend"
        
        $StartTime = Get-Date
        Push-Location $WebBackendDir
        & go build -ldflags "-s -w" -o $WebOutput .
        Pop-Location
        $Duration = (Get-Date) - $StartTime
        
        if ($LASTEXITCODE -eq 0) {
            $FileSize = [math]::Round((Get-Item $WebOutput).Length / 1MB, 2)
            Write-Success "  Web Console built: ${FileSize}MB (took: $($Duration.TotalSeconds.ToString('0.0'))s)"
        } else {
            Write-Fail "  Web Console build failed"
            return $false
        }
    }
    
    return $true
}

# Main function
function Main {
    Write-Host "============================================================" -ForegroundColor Cyan
    Write-Host "  Homeclaw Android .so Build Script" -ForegroundColor Cyan
    Write-Host "============================================================" -ForegroundColor Cyan
    
    # Check Go
    if (-not (Test-GoCommand)) {
        exit 1
    }
    
    $SuccessCount = 0
    $TotalCount = 0
    
    if ($AllArchitectures) {
        # Build all architectures
        $Architectures = @(
            @{Arch="arm64"; Label="arm64-v8a (64-bit ARM)"},
            @{Arch="arm"; Label="armeabi-v7a (32-bit ARM)"},
            @{Arch="amd64"; Label="x86_64 (Emulator)"}
        )
        
        foreach ($ArchInfo in $Architectures) {
            $TotalCount++
            if (Build-Architecture -Arch $ArchInfo.Arch -ArchLabel $ArchInfo.Label) {
                $SuccessCount++
            }
        }
        
        Write-Host ""
        Write-Host "============================================================" -ForegroundColor Cyan
        if ($SuccessCount -eq $TotalCount) {
            Write-Success "All architectures built successfully! ($SuccessCount/$TotalCount)"
            
            # Show build results
            Write-Host ""
            Write-Host "Build output:" -ForegroundColor Cyan
            Get-ChildItem (Join-Path $BuildDir "android\*\*.so") | 
                Format-Table Name, 
                    @{Label="Size(MB)";Expression={[math]::Round($_.Length/1MB,2)}},
                    @{Label="Architecture";Expression={$_.Directory.Name}} -AutoSize
        } else {
            Write-Fail "Some builds failed ($SuccessCount/$TotalCount succeeded)"
            exit 1
        }
    } else {
        # Build single architecture
        $TotalCount = 1
        $ArchLabel = switch ($Architecture) {
            "arm64" { "arm64-v8a (64-bit ARM)" }
            "arm" { "armeabi-v7a (32-bit ARM)" }
            "amd64" { "x86_64 (Emulator)" }
        }
        
        if (Build-Architecture -Arch $Architecture -ArchLabel $ArchLabel) {
            Write-Host ""
            Write-Host "============================================================" -ForegroundColor Cyan
            Write-Success "Build completed!"
            
            # Show build results
            $ArchOutputDir = Join-Path $BuildDir "android\$Architecture"
            Write-Host ""
            Write-Host "Build output:" -ForegroundColor Cyan
            Get-ChildItem (Join-Path $ArchOutputDir "*.so") | 
                Format-Table Name, @{Label="Size(MB)";Expression={[math]::Round($_.Length/1MB,2)}} -AutoSize
        } else {
            Write-Fail "Build failed"
            exit 1
        }
    }
    
    # Show usage instructions
    Write-Host ""
    Write-Host "Usage Instructions:" -ForegroundColor Cyan
    Write-Host "  1. Copy .so files to Flutter project:" -ForegroundColor White
    Write-Host "     copy build\android\arm64-v8a\*.so <flutter-project>\android\app\src\main\jniLibs\arm64-v8a\" -ForegroundColor Gray
    Write-Host ""
    Write-Host "  2. Configure in build.gradle.kts:" -ForegroundColor White
    Write-Host "     packaging {" -ForegroundColor Gray
    Write-Host "         jniLibs {" -ForegroundColor Gray
    Write-Host "             keepDebugSymbols += `"**/libhomeclaw.so`"" -ForegroundColor Gray
    Write-Host "             keepDebugSymbols += `"**/libhomeclaw-web.so`"" -ForegroundColor Gray
    Write-Host "             useLegacyPackaging = true" -ForegroundColor Gray
    Write-Host "         }" -ForegroundColor Gray
    Write-Host "     }" -ForegroundColor Gray
}

# Execute main function
Main
