# 快速参考 - 多环境配置

## 🚀 快速启动

### 默认开发环境
```bash
# 不设置任何环境变量，默认使用 dev 配置
go run cmd/risk-server/main.go
go run cmd/captcha-server/main.go
```

### 使用快速启动脚本

#### Windows PowerShell
```powershell
# 启动 Risk 服务（开发环境）
.\start.ps1 dev risk

# 启动 Captcha 服务（开发环境）
.\start.ps1 dev captcha

# 启动 Risk 服务（生产环境）
.\start.ps1 prod risk
```

#### Linux/Mac
```bash
# 给脚本执行权限（首次需要）
chmod +x start.sh

# 启动服务
./start.sh dev risk
./start.sh dev captcha
./start.sh prod risk
```

## 📁 配置文件列表

```
configs/
├── risk.dev.yaml      ✅ 开发环境（提交到Git）
├── risk.prod.yaml     ✅ 生产环境（提交到Git）
├── captcha.dev.yaml   ✅ 开发环境（提交到Git）
├── captcha.prod.yaml  ✅ 生产环境（提交到Git）
├── risk.template.yaml ✅ 模板文件（提交到Git）
├── captcha.template.yaml ✅ 模板文件（提交到Git）
├── risk.yaml          ❌ 本地自定义（不提交）
└── captcha.yaml       ❌ 本地自定义（不提交）
```

## 🔑 环境变量速查

### 基础环境切换
```bash
APP_ENV=dev   # 开发环境（默认）
APP_ENV=prod  # 生产环境
```

### 覆盖配置（可选）
```bash
REDIS_ADDR=127.0.0.1:6379          # Redis地址
REDIS_PASSWORD=your_password        # Redis密码
REDIS_DB=0                          # Redis数据库编号
NACOS_SERVER_ADDR=127.0.0.1:8848   # Nacos地址
NACOS_NAMESPACE=dev                 # Nacos命名空间
NACOS_ENABLE=false                  # 是否启用Nacos
TOKEN_SECRET=your_secret_key        # Token密钥
```

## ⚙️ 环境配置差异

| 配置项 | 开发环境 (dev) | 生产环境 (prod) |
|--------|---------------|----------------|
| **Redis地址** | 127.0.0.1:6379 | 118.24.164.222:6379 |
| **Redis密码** | 无 | kelab666（建议环境变量覆盖） |
| **Redis DB** | 0 | 1 (Risk), 2 (Captcha) |
| **连接池** | 10 | 100 |
| **Nacos** | 关闭 | 开启 |
| **频控限制** | 宽松（便于测试） | 严格 |
| **Token有效期** | 更长 | 标准 |

## 🐳 Docker 部署

### Dockerfile 示例
```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o risk-server cmd/risk-server/main.go

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/risk-server .
COPY --from=builder /app/configs ./configs

ENV APP_ENV=prod
EXPOSE 9080 9090
CMD ["./risk-server"]
```

### 运行容器
```bash
# 开发环境
docker run -e APP_ENV=dev -p 9080:9080 -p 9090:9090 risk-engine

# 生产环境（覆盖敏感配置）
docker run \
  -e APP_ENV=prod \
  -e REDIS_PASSWORD=secure_password \
  -p 9080:9080 -p 9090:9090 \
  risk-engine
```

## 🔒 安全最佳实践

### ✅ 推荐做法
1. **开发环境**：使用默认的 `dev` 配置，无需额外设置
2. **生产环境**：通过环境变量覆盖敏感信息
   ```bash
   APP_ENV=prod REDIS_PASSWORD=xxx ./risk-server
   ```
3. **Docker部署**：使用 Docker Secrets 或 Kubernetes ConfigMap
4. **配置文件**：
   - ✅ `risk.dev.yaml` 和 `risk.prod.yaml` 提交到Git
   - ❌ `.env` 文件不要提交到Git
   - ❌ 生产环境密码不要硬编码在配置文件中

### ❌ 避免的做法
1. ❌ 在配置文件中硬编码生产环境密码
2. ❌ 将 `.env` 文件提交到版本控制
3. ❌ 在开发环境使用生产数据库

## 📝 常用命令

### 设置环境变量（Windows）
```powershell
# 临时设置（当前会话）
$env:APP_ENV="dev"

# 永久设置（系统级）
[System.Environment]::SetEnvironmentVariable("APP_ENV", "dev", "User")
```

### 设置环境变量（Linux/Mac）
```bash
# 临时设置（当前会话）
export APP_ENV=dev

# 永久设置（添加到 ~/.bashrc 或 ~/.zshrc）
echo 'export APP_ENV=dev' >> ~/.bashrc
source ~/.bashrc
```

### 验证配置
```bash
# 查看当前环境变量
# Windows
echo $env:APP_ENV

# Linux/Mac
echo $APP_ENV

# 查看所有环境变量
# Windows
Get-ChildItem Env: | Where-Object { $_.Name -like "*REDIS*" -or $_.Name -like "*APP_ENV*" }

# Linux/Mac
env | grep -E "REDIS|APP_ENV"
```

## 🆘 故障排查

### 问题：配置文件未找到
```
ERROR: 读取配置文件 risk.dev.yaml 失败
```
**解决**：确保在项目根目录运行，检查 `configs/` 目录中是否存在对应的配置文件

### 问题：Redis 连接失败
```
ERROR: Redis 连接失败
```
**解决**：
1. 开发环境：确保 Redis 在本地运行（`redis-server`）
2. 生产环境：检查网络连接和防火墙设置
3. 验证 Redis 地址和密码是否正确

### 问题：环境变量未生效
**解决**：
1. 确认环境变量名称正确（区分大小写）
2. Windows PowerShell 使用 `$env:VAR_NAME`，Bash 使用 `export VAR_NAME`
3. 重启终端或重新设置环境变量

## 📚 相关文档

- 📖 [完整配置指南](CONFIG_GUIDE.md) - 详细的配置说明和最佳实践
- 📘 [主README](../README.md) - 项目概览和快速开始
- 🔧 [.env.example](../.env.example) - 环境变量示例文件
