# Watch Docker Windows Service 安装脚本
# 需要管理员权限运行

param(
    [string]$ServiceName = "WatchDocker",
    [string]$DisplayName = "Watch Docker Service",
    [string]$Description = "Watch Docker - Docker Container Management and Monitoring Tool",
    [string]$ConfigPath = "",
    [string]$UserName = "admin",
    [string]$UserPassword = "admin",
    [string]$Port = "8080"
)

# 检查是否以管理员权限运行
$currentPrincipal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
$isAdmin = $currentPrincipal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)

if (-not $isAdmin) {
    Write-Error "此脚本需要管理员权限运行。请右键点击 PowerShell 并选择'以管理员身份运行'。"
    exit 1
}

# 获取脚本所在目录
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ExePath = Join-Path (Split-Path -Parent $ScriptDir) "watch-docker.exe"

# 检查可执行文件是否存在
if (-not (Test-Path $ExePath)) {
    Write-Error "找不到 watch-docker.exe，请确保已正确安装。"
    exit 1
}

# 设置配置路径
if ([string]::IsNullOrEmpty($ConfigPath)) {
    $ConfigPath = Join-Path $env:USERPROFILE ".watch-docker"
}

# 创建配置目录
if (-not (Test-Path $ConfigPath)) {
    New-Item -ItemType Directory -Path $ConfigPath -Force | Out-Null
    Write-Host "已创建配置目录: $ConfigPath" -ForegroundColor Green
}

# 检查服务是否已存在
$existingService = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue

if ($existingService) {
    Write-Host "服务 '$ServiceName' 已存在，正在停止并删除..." -ForegroundColor Yellow
    
    if ($existingService.Status -eq 'Running') {
        Stop-Service -Name $ServiceName -Force
        Start-Sleep -Seconds 2
    }
    
    sc.exe delete $ServiceName
    Start-Sleep -Seconds 2
}

# 创建服务
Write-Host "正在创建服务 '$ServiceName'..." -ForegroundColor Cyan

# 构建环境变量
$envVars = @(
    "CONFIG_PATH=$ConfigPath",
    "USER_NAME=$UserName",
    "USER_PASSWORD=$UserPassword",
    "STATIC_DIR=",
    "IS_OPEN_DOCKER_SHELL=false",
    "IS_SECONDARY_VERIFICATION=false"
)

# 使用 NSSM（如果可用）或 sc.exe
$nssmPath = Get-Command nssm -ErrorAction SilentlyContinue

if ($nssmPath) {
    # 使用 NSSM 创建服务
    Write-Host "使用 NSSM 创建服务..." -ForegroundColor Cyan
    
    nssm install $ServiceName $ExePath
    nssm set $ServiceName DisplayName $DisplayName
    nssm set $ServiceName Description $Description
    nssm set $ServiceName Start SERVICE_AUTO_START
    nssm set $ServiceName AppDirectory (Split-Path -Parent $ExePath)
    
    # 设置环境变量
    foreach ($env in $envVars) {
        nssm set $ServiceName AppEnvironmentExtra $env
    }
    
    # 设置日志
    $logPath = Join-Path $ConfigPath "service.log"
    nssm set $ServiceName AppStdout $logPath
    nssm set $ServiceName AppStderr $logPath
    
    # 设置重启策略
    nssm set $ServiceName AppExit Default Restart
    nssm set $ServiceName AppRestartDelay 5000
    
    Write-Host "服务已通过 NSSM 创建成功！" -ForegroundColor Green
} else {
    # 使用 sc.exe 创建基本服务
    Write-Host "使用 sc.exe 创建服务（推荐安装 NSSM 以获得更好的服务管理）..." -ForegroundColor Yellow
    
    sc.exe create $ServiceName binPath= $ExePath start= auto DisplayName= $DisplayName
    sc.exe description $ServiceName $Description
    
    Write-Host "服务已创建成功！" -ForegroundColor Green
    Write-Host "注意: 使用 sc.exe 创建的服务不包含环境变量配置。" -ForegroundColor Yellow
    Write-Host "推荐安装 NSSM (https://nssm.cc/) 以获得完整的服务管理功能。" -ForegroundColor Yellow
}

# 启动服务
Write-Host "正在启动服务..." -ForegroundColor Cyan
Start-Service -Name $ServiceName

# 检查服务状态
$service = Get-Service -Name $ServiceName
if ($service.Status -eq 'Running') {
    Write-Host "`n服务安装并启动成功！" -ForegroundColor Green
    Write-Host "服务名称: $ServiceName" -ForegroundColor Cyan
    Write-Host "配置目录: $ConfigPath" -ForegroundColor Cyan
    Write-Host "访问地址: http://localhost:$Port" -ForegroundColor Cyan
    Write-Host "`n管理命令:" -ForegroundColor Yellow
    Write-Host "  启动服务: Start-Service $ServiceName" -ForegroundColor White
    Write-Host "  停止服务: Stop-Service $ServiceName" -ForegroundColor White
    Write-Host "  重启服务: Restart-Service $ServiceName" -ForegroundColor White
    Write-Host "  查看状态: Get-Service $ServiceName" -ForegroundColor White
    Write-Host "  卸载服务: .\uninstall-service.ps1" -ForegroundColor White
} else {
    Write-Error "服务启动失败，请检查日志。"
    Write-Host "日志位置: $ConfigPath\service.log" -ForegroundColor Yellow
    exit 1
}
