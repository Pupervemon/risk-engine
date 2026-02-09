@echo off
echo 开始构建项目...

:: 创建 dist 文件夹（如果不存在）
if not exist "dist" (
    mkdir dist
)

:: 设置交叉编译环境变量
SET CGO_ENABLED=0
SET GOOS=linux
SET GOARCH=amd64

:: 编译并将输出重定向到 dist 文件夹
:: 假设你的 main.go 在项目根目录
echo 正在编译 Linux (amd64) 版本至 dist/server...
go build -o ./dist/server main.go

if %errorlevel% equ 0 (
    echo [成功] 构建完成！文件位于 dist/server
) else (
    echo [错误] 构建失败，请检查代码。
)

pause