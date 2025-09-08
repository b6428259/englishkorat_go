#!/usr/bin/env pwsh

# EnglishKorat Go - Database Seeder Script
# This script will drop all data and create fresh seed data

Write-Host "=================================================
EnglishKorat Database Seeder
=================================================" -ForegroundColor Cyan

# Check if .env file exists
if (-not (Test-Path ".env")) {
    Write-Host " Error: .env file not found!" -ForegroundColor Red
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
Write-Host "  WARNING: This will DELETE ALL existing data and create fresh seed data!" -ForegroundColor Red
Write-Host "  This action cannot be undone!" -ForegroundColor Red
Write-Host ""
$confirmation = Read-Host "Are you sure you want to proceed? Type 'YES' to continue"

if ($confirmation -ne "YES") {
    Write-Host " Operation cancelled." -ForegroundColor Yellow
    exit 0
}

Write-Host ""
Write-Host " Starting seeding process..." -ForegroundColor Yellow

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

# Create and run seeder
Write-Host "Creating seeder application..." -ForegroundColor Cyan

$seederContent = @"
package main

import (
	"englishkorat_go/config"
	"englishkorat_go/database"
	"englishkorat_go/database/seeders"
	"englishkorat_go/models"
	"log"
)

func main() {
	log.Println(" Starting database seeding process...")
	
	// Load configuration
	config.LoadConfig()
	
	// Connect to database
	database.Connect()
	
	log.Println("  Dropping existing tables...")
	
	// Drop all tables in reverse order to handle foreign keys
	database.DB.Migrator().DropTable(&models.Notification{})
	database.DB.Migrator().DropTable(&models.ActivityLog{})
	database.DB.Migrator().DropTable(&models.Course{})
	database.DB.Migrator().DropTable(&models.Room{})
	database.DB.Migrator().DropTable(&models.Teacher{})
	database.DB.Migrator().DropTable(&models.Student{})
	database.DB.Migrator().DropTable(&models.User{})
	database.DB.Migrator().DropTable(&models.Branch{})
	
	log.Println(" All tables dropped successfully")
	
	log.Println(" Running auto migration...")
	
	// Run auto migration to recreate tables
	database.AutoMigrate()
	
	log.Println(" Running seeders...")
	
	// Run all seeders
	seeders.SeedAll()
	
	log.Println(" Database seeding completed successfully!")
	log.Println(" Fresh seed data has been created.")
}
"@

# Write seeder to temporary file
# Create directory if it doesn't exist
if (-not (Test-Path "cmd\seeder")) {
    New-Item -ItemType Directory -Path "cmd\seeder" -Force | Out-Null
}

$seederContent | Out-File -FilePath "cmd\seeder\main.go" -Encoding UTF8 -Force

try {
    Write-Host " Running seeder..." -ForegroundColor Yellow
    $result = & go run cmd/seeder/main.go 2>&1
    
    if ($LASTEXITCODE -eq 0) {
        Write-Host " Seeding completed successfully!" -ForegroundColor Green
        Write-Host $result
    } else {
        Write-Host " Seeding failed!" -ForegroundColor Red
        Write-Host $result
    }
} catch {
    Write-Host " Error running seeder: $_" -ForegroundColor Red
} finally {
    # Cleanup
    if (Test-Path "cmd\seeder\main.go") {
        Remove-Item "cmd\seeder\main.go" -Force
    }
    if ((Test-Path "cmd\seeder") -and ((Get-ChildItem "cmd\seeder").Count -eq 0)) {
        Remove-Item "cmd\seeder" -Force
    }
    
    # Stop SSH tunnel
    if ($sshProcess -and -not $sshProcess.HasExited) {
        Write-Host "Stopping SSH tunnel..." -ForegroundColor Yellow
        $sshProcess.Kill()
        Write-Host "SSH tunnel stopped." -ForegroundColor Green
    }
}

Write-Host ""
Write-Host "Seeding process completed!" -ForegroundColor Cyan
