#!/usr/bin/env pwsh

# EnglishKorat Go - Full Development Server (with seeding)
# This script establishes SSH tunnels, runs seeding if needed, and starts the development server

Write-Host "=================================================
EnglishKorat Go - Full Development Server
=================================================" -ForegroundColor Cyan

# Check if .env file exists
if (-not (Test-Path ".env")) {
    Write-Host "Error: .env file not found!" -ForegroundColor Red
    Write-Host "Please create .env file from .env.example first." -ForegroundColor Yellow
    exit 1
}

# Configuration
$EC2_HOST = "ec2-54-254-53-52.ap-southeast-1.compute.amazonaws.com"
$EC2_USER = "ubuntu"
$EC2_KEY_PATH = "./EKLS.pem"
$LOCAL_DB_PORT = "3307"
$LOCAL_REDIS_PORT = "6380"
$REMOTE_DB_PORT = "3306"
$REMOTE_REDIS_PORT = "6379"
$RDS_HOST = "ekorat-db.c96wcau48ea0.ap-southeast-1.rds.amazonaws.com"

# Load environment variables from .env
$DB_HOST = "localhost"
$REDIS_HOST = "localhost"

if (Test-Path ".env") {
    Get-Content ".env" | ForEach-Object {
        if ($_ -match "^([^#][^=]*?)=(.*)$") {
            $name = $matches[1].Trim()
            $value = $matches[2].Trim()
            if ($name -eq "DB_HOST") { $DB_HOST = $value }
            if ($name -eq "REDIS_HOST") { $REDIS_HOST = $value }
        }
    }
}

Write-Host "Configuration:
   EC2 Host: $EC2_HOST
   Local DB Port: $LOCAL_DB_PORT
   Local Redis Port: $LOCAL_REDIS_PORT
   DB Host: $DB_HOST
   Redis Host: $REDIS_HOST" -ForegroundColor Green

Write-Host ""
Write-Host "  This will start the full development environment with seeding if database is empty." -ForegroundColor Yellow
Write-Host "  If you want to run seeding manually, use start-seed.ps1 instead." -ForegroundColor Yellow
Write-Host ""
$confirmation = Read-Host "Do you want to continue? Type 'YES' to proceed"

if ($confirmation -ne "YES") {
    Write-Host " Operation cancelled." -ForegroundColor Yellow
    exit 0
}

# Function to test if a port is available
function Test-Port {
    param($Port)
    try {
        $connection = New-Object System.Net.Sockets.TcpClient
        $connection.Connect("localhost", $Port)
        $connection.Close()
        return $true
    }
    catch {
        return $false
    }
}

# Check prerequisites
Write-Host "Checking prerequisites..." -ForegroundColor Cyan

# Check if SSH key exists
if (-not (Test-Path $EC2_KEY_PATH)) {
    Write-Host " SSH key not found: $EC2_KEY_PATH" -ForegroundColor Red
    exit 1
}

# Check if Go is installed
try {
    $goVersion = go version 2>$null
    if (-not $goVersion) {
        Write-Host " Go is not installed or not in PATH" -ForegroundColor Red
        exit 1
    }
    Write-Host " Go found: $goVersion" -ForegroundColor Green
} catch {
    Write-Host " Go is not installed or not in PATH" -ForegroundColor Red
    exit 1
}

# Check port availability
Write-Host "Checking port availability..." -ForegroundColor Cyan
$portsInUse = @()

if (Test-Port $LOCAL_DB_PORT) {
    $portsInUse += $LOCAL_DB_PORT
}
if (Test-Port $LOCAL_REDIS_PORT) {
    $portsInUse += $LOCAL_REDIS_PORT
}

if ($portsInUse.Count -gt 0) {
    Write-Host " The following ports are already in use: $($portsInUse -join ', ')" -ForegroundColor Red
    Write-Host "Please stop any services using these ports and try again." -ForegroundColor Yellow
    exit 1
}

Write-Host " All ports are available!" -ForegroundColor Green

# Update .env file for tunnel configuration
Write-Host "Updating .env file for tunnel configuration..." -ForegroundColor Cyan
if (Test-Path ".env") {
    $envContent = Get-Content ".env"
    # Update port lines only
    $envContent = $envContent -replace "DB_PORT=.*", "DB_PORT=$LOCAL_DB_PORT"
    $envContent = $envContent -replace "REDIS_PORT=.*", "REDIS_PORT=$LOCAL_REDIS_PORT"
    $envContent | Set-Content ".env"
    Write-Host " .env file ports updated!" -ForegroundColor Green
} else {
    Write-Host " .env file not found. Please create it from .env.example" -ForegroundColor Yellow
    exit 1
}

# Establish SSH tunnel to EC2
Write-Host "Establishing SSH tunnel to EC2..." -ForegroundColor Cyan

$sshArgs = @(
    "-N"
    # Forward local DB port to RDS host:3306 from the EC2 side
    "-L", "$LOCAL_DB_PORT`:$RDS_HOST`:$REMOTE_DB_PORT"
    # Forward Redis (still assumed to be on EC2 localhost)
    "-L", "$LOCAL_REDIS_PORT`:localhost`:$REMOTE_REDIS_PORT"
    "-i", $EC2_KEY_PATH
    "$EC2_USER@$EC2_HOST"
)

Write-Host "SSH Command: ssh $($sshArgs -join ' ')" -ForegroundColor Gray

try {
    $sshProcess = Start-Process -FilePath "ssh" -ArgumentList $sshArgs -PassThru -WindowStyle Hidden
    Write-Host " SSH tunnel process started (PID: $($sshProcess.Id))" -ForegroundColor Green
} catch {
    Write-Host " Failed to start SSH tunnel: $_" -ForegroundColor Red
    exit 1
}

Write-Host "Waiting for tunnel to establish..." -ForegroundColor Yellow
Start-Sleep -Seconds 5

# Test connections
Write-Host "Testing connections..." -ForegroundColor Cyan

Write-Host "Testing connection to MySQL Database (localhost:$LOCAL_DB_PORT)..." -ForegroundColor Yellow
if (Test-Port $LOCAL_DB_PORT) {
    Write-Host " MySQL Database connection successful!" -ForegroundColor Green
} else {
    Write-Host " MySQL Database connection failed!" -ForegroundColor Red
    if ($sshProcess -and -not $sshProcess.HasExited) {
        Write-Host "Stopping SSH tunnel..." -ForegroundColor Yellow
        $sshProcess.Kill()
    }
    exit 1
}

Write-Host "Testing connection to Redis (localhost:$LOCAL_REDIS_PORT)..." -ForegroundColor Yellow
if (Test-Port $LOCAL_REDIS_PORT) {
    Write-Host " Redis connection successful!" -ForegroundColor Green
} else {
    Write-Host " Redis connection failed!" -ForegroundColor Red
    if ($sshProcess -and -not $sshProcess.HasExited) {
        Write-Host "Stopping SSH tunnel..." -ForegroundColor Yellow
        $sshProcess.Kill()
    }
    exit 1
}

Write-Host ""
Write-Host "Connection Summary:
======================" -ForegroundColor Cyan
Write-Host "Database: CONNECTED" -ForegroundColor Green
Write-Host "Redis: CONNECTED" -ForegroundColor Green
Write-Host ""

Write-Host "All services are connected! Starting application..." -ForegroundColor Green

# Check if database needs seeding and run if needed
Write-Host "Checking if database needs seeding..." -ForegroundColor Cyan

$checkSeederContent = @"
package main

import (
	"englishkorat_go/config"
	"englishkorat_go/database"
	"englishkorat_go/database/seeders"
	"englishkorat_go/models"
	"log"
	"os"
)

func main() {
	// Load configuration
	config.LoadConfig()
	
	// Connect to database
	database.Connect()
	
	// Check if we need seeding
	var count int64
	database.DB.Model(&models.User{}).Count(&count)
	
	if count == 0 {
		log.Println(" Database is empty, running seeders...")
		database.AutoMigrate()
		seeders.SeedAll()
		log.Println(" Seeding completed!")
	} else {
		log.Println(" Database already has data, skipping seeding.")
	}
	
	// Just to indicate success
	os.Exit(0)
}
"@

# Create directory if it doesn't exist
if (-not (Test-Path "cmd\check-seed")) {
    New-Item -ItemType Directory -Path "cmd\check-seed" -Force | Out-Null
}

$checkSeederContent | Out-File -FilePath "cmd\check-seed\main.go" -Encoding UTF8 -Force

try {
    Write-Host " Checking and seeding if needed..." -ForegroundColor Yellow
    $result = & go run cmd/check-seed/main.go 2>&1
    
    if ($LASTEXITCODE -eq 0) {
        Write-Host $result
    } else {
        Write-Host "  Seeding check had issues, but continuing..." -ForegroundColor Yellow
        Write-Host $result
    }
} catch {
    Write-Host "  Error checking seeding status, but continuing..." -ForegroundColor Yellow
}

# Cleanup check-seed
if (Test-Path "cmd\check-seed\main.go") {
    Remove-Item "cmd\check-seed\main.go" -Force
}
if (Test-Path "cmd\check-seed" -and (Get-ChildItem "cmd\check-seed").Count -eq 0) {
    Remove-Item "cmd\check-seed" -Force
}

# Handle cleanup on Ctrl+C
$cleanup = {
    Write-Host ""
    Write-Host "Shutting down..." -ForegroundColor Yellow
    if ($sshProcess -and -not $sshProcess.HasExited) {
        Write-Host "Stopping SSH tunnel..." -ForegroundColor Yellow
        $sshProcess.Kill()
        Write-Host "SSH tunnel stopped." -ForegroundColor Green
    }
    Write-Host "Cleanup completed." -ForegroundColor Green
    exit 0
}

# Register cleanup for Ctrl+C
$null = Register-EngineEvent -SourceIdentifier "PowerShell.Exiting" -Action $cleanup

try {
    Write-Host "Starting: go run main.go (press Ctrl+C to stop)" -ForegroundColor Yellow
    Write-Host ""
    
    # Start the Go application
    & go run main.go
    
} catch {
    Write-Host " Error starting application: $_" -ForegroundColor Red
} finally {
    # Cleanup
    if ($sshProcess -and -not $sshProcess.HasExited) {
        Write-Host "Stopping SSH tunnel..." -ForegroundColor Yellow
        $sshProcess.Kill()
        Write-Host "SSH tunnel stopped." -ForegroundColor Green
    }
    Write-Host "Development server stopped." -ForegroundColor Green
}
