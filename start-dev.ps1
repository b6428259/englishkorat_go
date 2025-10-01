# English Korat Go - Development Tunnel Script
# This script establishes a tunnel connection to EC2 and pings DB and Redis

Write-Host "English Korat Go - Development Environment Setup" -ForegroundColor Cyan
Write-Host "=================================================" -ForegroundColor Cyan

# Configuration

$EC2_HOST = "ec2-54-254-53-52.ap-southeast-1.compute.amazonaws.com"
$EC2_USER = "ubuntu"
$EC2_KEY_PATH = "./EKLS.pem"
$LOCAL_DB_PORT = "3307"
$LOCAL_REDIS_PORT = "6380"
$REMOTE_DB_PORT = "3306"
$REMOTE_REDIS_PORT = "6379"
$RDS_HOST = "ekorat-db.c96wcau48ea0.ap-southeast-1.rds.amazonaws.com"

# Read DB_HOST and REDIS_HOST from .env if exists
if (Test-Path ".env") {
    $envLines = Get-Content ".env"
    $DB_HOST = ($envLines | Where-Object { $_ -match "^DB_HOST=" }) -replace "^DB_HOST=", ""
    $REDIS_HOST = ($envLines | Where-Object { $_ -match "^REDIS_HOST=" }) -replace "^REDIS_HOST=", ""
    if (-not $DB_HOST) { $DB_HOST = "localhost" }
    if (-not $REDIS_HOST) { $REDIS_HOST = "localhost" }
} else {
    $DB_HOST = "localhost"
    $REDIS_HOST = "localhost"
}

Write-Host "Configuration:" -ForegroundColor Yellow
Write-Host "   EC2 Host: $EC2_HOST" -ForegroundColor White
Write-Host "   Local DB Port: $LOCAL_DB_PORT" -ForegroundColor White
Write-Host "   Local Redis Port: $LOCAL_REDIS_PORT" -ForegroundColor White
Write-Host "   DB Host: $DB_HOST" -ForegroundColor White
Write-Host "   Redis Host: $REDIS_HOST" -ForegroundColor White

# Function to test if a port is available
function Test-Port {
    param($Port)
    try {
        $listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Any, $Port)
        $listener.Start()
        $listener.Stop()
        return $true
    } catch {
        return $false
    }
}

# Function to test connection to a service
function Test-Connection {
    param($TargetHost, $Port, $ServiceName)
    
    Write-Host "Testing connection to $ServiceName ($TargetHost`:$Port)..." -ForegroundColor Yellow
    
    try {
        $tcpClient = New-Object System.Net.Sockets.TcpClient
        $connectTask = $tcpClient.ConnectAsync($TargetHost, $Port)
        $timeout = [System.Threading.Tasks.Task]::Delay(5000)
        
        $completedTask = [System.Threading.Tasks.Task]::WaitAny(@($connectTask, $timeout))
        
        if ($completedTask -eq 0 -and $tcpClient.Connected) {
            Write-Host "$ServiceName connection successful!" -ForegroundColor Green
            $tcpClient.Close()
            return $true
        } else {
            Write-Host "$ServiceName connection failed (timeout or connection refused)" -ForegroundColor Red
            $tcpClient.Close()
            return $false
        }
    } catch {
        Write-Host "$ServiceName connection failed: $($_.Exception.Message)" -ForegroundColor Red
        return $false
    }
}

# Check if required tools are available
Write-Host "Checking prerequisites..." -ForegroundColor Yellow

if (-not (Get-Command ssh -ErrorAction SilentlyContinue)) {
    Write-Host "SSH not found. Please install OpenSSH or Git Bash." -ForegroundColor Red
    exit 1
}

# Check if local ports are available
Write-Host "Checking port availability..." -ForegroundColor Yellow

if (-not (Test-Port $LOCAL_DB_PORT)) {
    Write-Host "Port $LOCAL_DB_PORT is already in use. Please close the application using this port." -ForegroundColor Red
    exit 1
}

if (-not (Test-Port $LOCAL_REDIS_PORT)) {
    Write-Host "Port $LOCAL_REDIS_PORT is already in use. Please close the application using this port." -ForegroundColor Red
    exit 1
}

Write-Host "All ports are available!" -ForegroundColor Green

# Update .env file for local development
Write-Host "Updating .env file for tunnel configuration..." -ForegroundColor Yellow

# Only adjust ports; preserve original host if it's already an external hostname
if (Test-Path ".env") {
    $envContent = Get-Content ".env"
    # Update port lines only
    $envContent = $envContent -replace "DB_PORT=.*", "DB_PORT=$LOCAL_DB_PORT"
    $envContent = $envContent -replace "REDIS_PORT=.*", "REDIS_PORT=$LOCAL_REDIS_PORT"
    $envContent | Set-Content ".env"
    Write-Host ".env file ports updated!" -ForegroundColor Green
} else {
    Write-Host ".env file not found. Please create it from .env.example" -ForegroundColor Yellow
}

# Start SSH tunnel using Start-Process (like the old method)
Write-Host "Establishing SSH tunnel to EC2..." -ForegroundColor Yellow

$sshArgs = @(
    "-N"
    # Forward local DB port to RDS host:3306 from the EC2 side
    "-L", "$LOCAL_DB_PORT`:$RDS_HOST`:$REMOTE_DB_PORT"
    # Forward Redis (still assumed to be on EC2 localhost)
    "-L", "$LOCAL_REDIS_PORT`:$REDIS_HOST`:$REMOTE_REDIS_PORT"
    # Fail fast if a local forward cannot be established
    "-o", "ExitOnForwardFailure=yes"
    "-i", $EC2_KEY_PATH
    "$EC2_USER@$EC2_HOST"
)

Write-Host "SSH Command: ssh $($sshArgs -join ' ')" -ForegroundColor Blue

# Start SSH tunnel using Start-Process (similar to old method)
$sshProc = Start-Process -FilePath "ssh" -ArgumentList $sshArgs -NoNewWindow -PassThru

Write-Host "Waiting for tunnel to establish..." -ForegroundColor Yellow
Start-Sleep -Seconds 3

# Test connections
Write-Host "`nTesting connections..." -ForegroundColor Cyan

$dbSuccess = Test-Connection $DB_HOST $LOCAL_DB_PORT "MySQL Database"

# Read REDIS_PASSWORD from .env (if present) for AUTH during the ping
if (Test-Path ".env") {
    $envLines = Get-Content ".env"
} else {
    $envLines = @()
}
$REDIS_PASSWORD = ($envLines | Where-Object { $_ -match "^REDIS_PASSWORD=" }) -replace "^REDIS_PASSWORD=", ""

# Function to perform a Redis PING over the forwarded TCP port (validates end-to-end)
function Test-RedisPing {
    param($TargetHost, $Port, $Password)

    Write-Host "Testing Redis PING to $TargetHost`:$Port..." -ForegroundColor Yellow
    try {
        $client = New-Object System.Net.Sockets.TcpClient
        $async = $client.ConnectAsync($TargetHost, [int]$Port)
        $timeout = [System.Threading.Tasks.Task]::Delay(5000)
        $completed = [System.Threading.Tasks.Task]::WaitAny(@($async, $timeout))
        if ($completed -ne 0 -or -not $client.Connected) {
            Write-Host "Redis TCP connect failed or timed out" -ForegroundColor Red
            return $false
        }

        $stream = $client.GetStream()
        $writer = New-Object System.IO.StreamWriter($stream)
        $writer.NewLine = "`r`n"
        $writer.AutoFlush = $true

        # If password provided, send AUTH first
        if ($Password -and $Password -ne "") {
            $authCmd = "*2`r`n$4`r`nAUTH`r`n$($Password.Length)`r`n$Password`r`n"
            $bytes = [System.Text.Encoding]::ASCII.GetBytes($authCmd)
            $stream.Write($bytes, 0, $bytes.Length)
            Start-Sleep -Milliseconds 200
        }

        # Send PING
        $pingCmd = "*1`r`n$4`r`nPING`r`n"
        $bytes = [System.Text.Encoding]::ASCII.GetBytes($pingCmd)
        $stream.Write($bytes, 0, $bytes.Length)

        # Read response
        $reader = New-Object System.IO.StreamReader($stream)
        Start-Sleep -Milliseconds 200
        $resp = ""
        if ($stream.DataAvailable) { $resp = $reader.ReadLine() }

        $reader.Close()
        $writer.Close()
        $client.Close()

        if ($resp -and $resp -match "^\+PONG") {
            Write-Host "Redis PING successful (PONG received)" -ForegroundColor Green
            return $true
        } else {
            Write-Host "Redis PING failed (no PONG). Response: $resp" -ForegroundColor Red
            return $false
        }
    } catch {
        Write-Host "Redis PING failed: $($_.Exception.Message)" -ForegroundColor Red
        return $false
    }
}

$redisSuccess = Test-RedisPing "localhost" $LOCAL_REDIS_PORT $REDIS_PASSWORD

Write-Host "`nConnection Summary:" -ForegroundColor Cyan
Write-Host "======================" -ForegroundColor Cyan

if ($dbSuccess) {
    Write-Host "Database: CONNECTED" -ForegroundColor Green
} else {
    Write-Host "Database: FAILED" -ForegroundColor Red
}

if ($redisSuccess) {
    Write-Host "Redis: CONNECTED" -ForegroundColor Green
} else {
    Write-Host "Redis: FAILED" -ForegroundColor Red
}

if ($dbSuccess -and $redisSuccess) {
    Write-Host "`nAll services are connected! You can now run your Go application." -ForegroundColor Green
    Write-Host "Starting: go run main.go (press Ctrl+C to stop)" -ForegroundColor Yellow
    
        try {
            # To speed up local development, skip automatic migrations/schema checks.
            # The application honors the SKIP_MIGRATE environment variable (set to "true").
            go run main.go
        }
    finally {
        Write-Host "`nStopping SSH tunnel..." -ForegroundColor Yellow
        if ($sshProc -and -not $sshProc.HasExited) {
            Stop-Process -Id $sshProc.Id -Force
        }
        Write-Host "Done." -ForegroundColor Green
    }
} else {
    Write-Host "`nSome connections failed. Please check your EC2 configuration." -ForegroundColor Yellow
    # Clean up tunnel if connections failed
    if ($sshProc -and -not $sshProc.HasExited) {
        Stop-Process -Id $sshProc.Id -Force
    }
}