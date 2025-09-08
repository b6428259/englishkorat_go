# English Korat Go - Development Tunnel Script
# This script establishes a tunnel connection to EC2 and pings DB and Redis

Write-Host "🚀 English Korat Go - Development Environment Setup" -ForegroundColor Cyan
Write-Host "=================================================" -ForegroundColor Cyan

# Configuration
$EC2_HOST = "your-ec2-host.amazonaws.com"
$EC2_USER = "ubuntu"
$EC2_KEY_PATH = "~/.ssh/your-key.pem"
$LOCAL_DB_PORT = "3307"
$LOCAL_REDIS_PORT = "6380"
$REMOTE_DB_PORT = "3306"
$REMOTE_REDIS_PORT = "6379"
$DB_HOST = "localhost"
$REDIS_HOST = "localhost"

Write-Host "📋 Configuration:" -ForegroundColor Yellow
Write-Host "   EC2 Host: $EC2_HOST" -ForegroundColor White
Write-Host "   Local DB Port: $LOCAL_DB_PORT" -ForegroundColor White
Write-Host "   Local Redis Port: $LOCAL_REDIS_PORT" -ForegroundColor White

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
    param($Host, $Port, $ServiceName)
    
    Write-Host "🔍 Testing connection to $ServiceName ($Host`:$Port)..." -ForegroundColor Yellow
    
    try {
        $tcpClient = New-Object System.Net.Sockets.TcpClient
        $connectTask = $tcpClient.ConnectAsync($Host, $Port)
        $timeout = [System.Threading.Tasks.Task]::Delay(5000)
        
        $completedTask = [System.Threading.Tasks.Task]::WaitAny(@($connectTask, $timeout))
        
        if ($completedTask -eq 0 -and $tcpClient.Connected) {
            Write-Host "✅ $ServiceName connection successful!" -ForegroundColor Green
            $tcpClient.Close()
            return $true
        } else {
            Write-Host "❌ $ServiceName connection failed (timeout or connection refused)" -ForegroundColor Red
            $tcpClient.Close()
            return $false
        }
    } catch {
        Write-Host "❌ $ServiceName connection failed: $($_.Exception.Message)" -ForegroundColor Red
        return $false
    }
}

# Check if required tools are available
Write-Host "🔧 Checking prerequisites..." -ForegroundColor Yellow

if (-not (Get-Command ssh -ErrorAction SilentlyContinue)) {
    Write-Host "❌ SSH not found. Please install OpenSSH or Git Bash." -ForegroundColor Red
    exit 1
}

# Check if local ports are available
Write-Host "🔍 Checking port availability..." -ForegroundColor Yellow

if (-not (Test-Port $LOCAL_DB_PORT)) {
    Write-Host "❌ Port $LOCAL_DB_PORT is already in use. Please close the application using this port." -ForegroundColor Red
    exit 1
}

if (-not (Test-Port $LOCAL_REDIS_PORT)) {
    Write-Host "❌ Port $LOCAL_REDIS_PORT is already in use. Please close the application using this port." -ForegroundColor Red
    exit 1
}

Write-Host "✅ All ports are available!" -ForegroundColor Green

# Update .env file for local development
Write-Host "📝 Updating .env file for tunnel configuration..." -ForegroundColor Yellow

if (Test-Path ".env") {
    $envContent = Get-Content ".env"
    $envContent = $envContent -replace "DB_HOST=.*", "DB_HOST=localhost"
    $envContent = $envContent -replace "DB_PORT=.*", "DB_PORT=$LOCAL_DB_PORT"
    $envContent = $envContent -replace "REDIS_HOST=.*", "REDIS_HOST=localhost"
    $envContent = $envContent -replace "REDIS_PORT=.*", "REDIS_PORT=$LOCAL_REDIS_PORT"
    $envContent | Set-Content ".env"
    Write-Host "✅ .env file updated!" -ForegroundColor Green
} else {
    Write-Host "⚠️  .env file not found. Please create it from .env.example" -ForegroundColor Yellow
}

# Start SSH tunnel
Write-Host "🌐 Establishing SSH tunnel to EC2..." -ForegroundColor Yellow

$sshArgs = @(
    "-N"
    "-L", "$LOCAL_DB_PORT`:localhost`:$REMOTE_DB_PORT"
    "-L", "$LOCAL_REDIS_PORT`:localhost`:$REMOTE_REDIS_PORT"
    "-i", $EC2_KEY_PATH
    "$EC2_USER@$EC2_HOST"
)

Write-Host "🔗 SSH Command: ssh $($sshArgs -join ' ')" -ForegroundColor Blue

# Start SSH tunnel in background
$sshJob = Start-Job -ScriptBlock {
    param($sshPath, $args)
    & $sshPath $args
} -ArgumentList "ssh", $sshArgs

Write-Host "🔄 Waiting for tunnel to establish..." -ForegroundColor Yellow
Start-Sleep -Seconds 3

# Test connections
Write-Host "`n🧪 Testing connections..." -ForegroundColor Cyan

$dbSuccess = Test-Connection $DB_HOST $LOCAL_DB_PORT "MySQL Database"
$redisSuccess = Test-Connection $REDIS_HOST $LOCAL_REDIS_PORT "Redis"

Write-Host "`n📊 Connection Summary:" -ForegroundColor Cyan
Write-Host "======================" -ForegroundColor Cyan

if ($dbSuccess) {
    Write-Host "✅ Database: CONNECTED" -ForegroundColor Green
} else {
    Write-Host "❌ Database: FAILED" -ForegroundColor Red
}

if ($redisSuccess) {
    Write-Host "✅ Redis: CONNECTED" -ForegroundColor Green
} else {
    Write-Host "❌ Redis: FAILED" -ForegroundColor Red
}

if ($dbSuccess -and $redisSuccess) {
    Write-Host "`n🎉 All services are connected! You can now run your Go application." -ForegroundColor Green
    Write-Host "💡 Run: go run main.go" -ForegroundColor Yellow
} else {
    Write-Host "`n⚠️  Some connections failed. Please check your EC2 configuration." -ForegroundColor Yellow
}

Write-Host "`n🔧 Tunnel Information:" -ForegroundColor Cyan
Write-Host "   MySQL: localhost:$LOCAL_DB_PORT -> EC2:$REMOTE_DB_PORT" -ForegroundColor White
Write-Host "   Redis: localhost:$LOCAL_REDIS_PORT -> EC2:$REMOTE_REDIS_PORT" -ForegroundColor White

Write-Host "`n⚠️  Press Ctrl+C to stop the tunnel and exit." -ForegroundColor Yellow

# Keep the script running and monitor the SSH tunnel
try {
    while ($sshJob.State -eq "Running") {
        Start-Sleep -Seconds 10
        
        # Periodically test connections
        $currentTime = Get-Date -Format "HH:mm:ss"
        Write-Host "[$currentTime] 🔄 Tunnel active - Testing connections..." -ForegroundColor Blue
        
        $dbTest = Test-Connection $DB_HOST $LOCAL_DB_PORT "Database"
        $redisTest = Test-Connection $REDIS_HOST $LOCAL_REDIS_PORT "Redis"
        
        if (-not $dbTest -or -not $redisTest) {
            Write-Host "⚠️  Some connections are down. Tunnel may need to be restarted." -ForegroundColor Yellow
        }
    }
} catch {
    Write-Host "`n🛑 Tunnel interrupted." -ForegroundColor Red
} finally {
    # Clean up
    Write-Host "`n🧹 Cleaning up..." -ForegroundColor Yellow
    Stop-Job $sshJob -ErrorAction SilentlyContinue
    Remove-Job $sshJob -ErrorAction SilentlyContinue
    Write-Host "✅ Cleanup completed. Tunnel closed." -ForegroundColor Green
}