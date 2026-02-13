@echo off
:: 切换终端编码为 UTF-8
chcp 65001 >nul

echo 开始构建风险引擎项目...

if not exist "dist" (
    mkdir dist
)

:: 设置交叉编译环境变量
SET CGO_ENABLED=0
SET GOOS=linux
SET GOARCH=amd64

echo ---------------------------------------
echo [1/2] 正在编译 验证码服务 (captcha-server)...
go build -o ./dist/captcha-server ./cmd/captcha-server/main.go
if %errorlevel% neq 0 goto :ERROR

echo [2/2] 正在编译 风险主服务 (risk-server)...
go build -o ./dist/risk-server ./cmd/risk-server/main.go
if %errorlevel% neq 0 goto :ERROR
echo ---------------------------------------

echo [成功] 构建完成！
echo 编译结果：
echo   - dist/captcha-server ^(验证码服务^)
echo   - dist/risk-server    ^(风险主服务^)
goto :END

:ERROR
echo ---------------------------------------
echo [错误] 构建过程中出现错误，请检查代码。
exit /b 1

:END
pause
