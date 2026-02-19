#!/bin/bash
# Risk Engine 快速启动脚本 (Linux/Mac)
# 用法: ./start.sh [dev|prod] [risk|captcha|all]

ENV=${1:-dev}
SERVICE=${2:-risk}

echo "========================================"
echo " Risk Engine 启动脚本"
echo "========================================"
echo "环境: $ENV"
echo "服务: $SERVICE"
echo "========================================\n"

# 设置环境变量
export APP_ENV=$ENV

# 启动服务
case $SERVICE in
    risk)
        echo "启动 Risk 服务..."
        go run cmd/risk-server/main.go
        ;;
    captcha)
        echo "启动 Captcha 服务..."
        go run cmd/captcha-server/main.go
        ;;
    all)
        echo "同时启动两个服务需要在不同的终端窗口运行:"
        echo "  终端1: ./start.sh $ENV risk"
        echo "  终端2: ./start.sh $ENV captcha"
        ;;
    *)
        echo "错误: 无效的服务名称 '$SERVICE'"
        echo "用法: ./start.sh [dev|prod] [risk|captcha|all]"
        exit 1
        ;;
esac
