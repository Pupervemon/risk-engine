# Risk Engine 快速启动脚本 (Windows PowerShell)
# 用法: .\start.ps1 [dev|prod] [risk|captcha|all]

param(
    [Parameter(Position=0)]
    [ValidateSet('dev', 'prod')]
    [string]$Env = 'dev',
    
    [Parameter(Position=1)]
    [ValidateSet('risk', 'captcha', 'all')]
    [string]$Service = 'risk'
)

Write-Host "========================================" -ForegroundColor Cyan
Write-Host " Risk Engine 启动脚本" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host "环境: $Env" -ForegroundColor Green
Write-Host "服务: $Service" -ForegroundColor Green
Write-Host "========================================`n" -ForegroundColor Cyan

# 设置环境变量
$env:APP_ENV = $Env

# 启动服务
switch ($Service) {
    'risk' {
        Write-Host "启动 Risk 服务..." -ForegroundColor Yellow
        go run cmd/risk-server/main.go
    }
    'captcha' {
        Write-Host "启动 Captcha 服务..." -ForegroundColor Yellow
        go run cmd/captcha-server/main.go
    }
    'all' {
        Write-Host "同时启动两个服务需要在不同的终端窗口运行:" -ForegroundColor Yellow
        Write-Host "  终端1: .\start.ps1 $Env risk" -ForegroundColor Cyan
        Write-Host "  终端2: .\start.ps1 $Env captcha" -ForegroundColor Cyan
    }
}
