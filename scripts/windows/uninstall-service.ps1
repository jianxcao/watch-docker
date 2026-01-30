# Watch Docker Windows Service 卸载脚本
# 需要管理员权限运行

param(
    [string]$ServiceName = "WatchDocker"
)

# 检查是否以管理员权限运行
$currentPrincipal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
$isAdmin = $currentPrincipal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)

if (-not $isAdmin) {
    Write-Error "此脚本需要管理员权限运行。请右键点击 PowerShell 并选择'以管理员身份运行'。"
    exit 1
}

# 检查服务是否存在
$service = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue

if (-not $service) {
    Write-Warning "服务 '$ServiceName' 不存在。"
    exit 0
}

Write-Host "正在卸载服务 '$ServiceName'..." -ForegroundColor Cyan

# 停止服务
if ($service.Status -eq 'Running') {
    Write-Host "正在停止服务..." -ForegroundColor Yellow
    Stop-Service -Name $ServiceName -Force
    Start-Sleep -Seconds 2
}

# 检查是否使用 NSSM
$nssmPath = Get-Command nssm -ErrorAction SilentlyContinue

if ($nssmPath) {
    # 使用 NSSM 删除服务
    Write-Host "使用 NSSM 删除服务..." -ForegroundColor Cyan
    nssm remove $ServiceName confirm
} else {
    # 使用 sc.exe 删除服务
    Write-Host "使用 sc.exe 删除服务..." -ForegroundColor Cyan
    sc.exe delete $ServiceName
}

Start-Sleep -Seconds 2

# 验证删除
$checkService = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
if (-not $checkService) {
    Write-Host "`n服务卸载成功！" -ForegroundColor Green
    Write-Host "注意: 配置文件仍保留在 %USERPROFILE%\.watch-docker" -ForegroundColor Yellow
    Write-Host "如需完全删除，请手动删除该目录。" -ForegroundColor Yellow
} else {
    Write-Error "服务卸载失败，请检查权限或手动删除。"
    exit 1
}
