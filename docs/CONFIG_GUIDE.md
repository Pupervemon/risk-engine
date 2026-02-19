# 多环境配置使用指南

## 概述

Risk Engine 支持多环境配置管理，通过环境变量 `APP_ENV` 切换不同的配置文件。

## 环境类型

- **dev** - 开发环境（默认）
- **prod** - 生产环境

## 配置文件说明

```
configs/
├── risk.dev.yaml      # 开发环境配置
├── risk.prod.yaml     # 生产环境配置
└── risk.template.yaml # 配置模板（参考用）
```

### 开发环境特点 (dev)
- Redis: 本地 `127.0.0.1:6379`，无密码
- Nacos: 默认关闭
- 风控规则: 更宽松的限制，便于开发测试
- 连接池: 较小配置

### 生产环境特点 (prod)
- Redis: 远程服务器，需要密码
- Nacos: 默认开启服务注册
- 风控规则: 严格的限流和防护
- 连接池: 优化的生产配置

## 使用方法

### 方法一：通过环境变量设置

#### Windows (PowerShell)
```powershell
# 开发环境
$env:APP_ENV="dev"
go run cmd/risk-server/main.go

# 生产环境
$env:APP_ENV="prod"
go run cmd/risk-server/main.go
```

#### Linux/Mac (Bash)
```bash
# 开发环境
export APP_ENV=dev
go run cmd/risk-server/main.go

# 生产环境
export APP_ENV=prod
go run cmd/risk-server/main.go
```

#### 一行命令
```bash
# Windows PowerShell
$env:APP_ENV="dev"; go run cmd/risk-server/main.go

# Linux/Mac
APP_ENV=dev go run cmd/risk-server/main.go
```

### 方法二：使用 .env 文件（推荐）

1. 复制示例文件：
```bash
cp .env.example .env
```

2. 编辑 `.env` 文件：
```env
APP_ENV=dev
```

3. 在代码中加载 .env 文件（需要集成 godotenv）：
```go
import "github.com/joho/godotenv"

// 在 main 函数开始处
godotenv.Load()
```

### 方法三：不设置环境变量（默认开发环境）

```bash
# 默认使用 dev 环境
go run cmd/risk-server/main.go
```

## 环境变量覆盖

除了切换配置文件，还可以通过环境变量覆盖特定配置项：

### 支持的环境变量

| 环境变量 | 说明 | 示例 |
|---------|------|------|
| `APP_ENV` 或 `ENV` | 环境标识 | `dev`, `prod` |
| `REDIS_ADDR` | Redis地址 | `127.0.0.1:6379` |
| `REDIS_PASSWORD` | Redis密码 | `your_password` |
| `REDIS_DB` | Redis数据库编号 | `0` |
| `NACOS_SERVER_ADDR` | Nacos服务器地址 | `127.0.0.1:8848` |
| `NACOS_NAMESPACE` | Nacos命名空间 | `dev` |
| `NACOS_ENABLE` | 是否启用Nacos | `true`, `false` |

### 示例：覆盖 Redis 密码

```bash
# Windows PowerShell
$env:APP_ENV="prod"
$env:REDIS_PASSWORD="new_secure_password"
go run cmd/risk-server/main.go

# Linux/Mac
APP_ENV=prod REDIS_PASSWORD=new_secure_password go run cmd/risk-server/main.go
```

## Docker 部署

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

# 设置环境变量
ENV APP_ENV=prod

EXPOSE 9080 9090
CMD ["./risk-server"]
```

### docker-compose.yml 示例

```yaml
version: '3.8'

services:
  risk-server-dev:
    build: .
    environment:
      - APP_ENV=dev
      - REDIS_ADDR=redis:6379
    ports:
      - "9080:9080"
      - "9090:9090"
    depends_on:
      - redis

  risk-server-prod:
    build: .
    environment:
      - APP_ENV=prod
      - REDIS_ADDR=118.24.164.222:6379
      - REDIS_PASSWORD=kelab666
    ports:
      - "9080:9080"
      - "9090:9090"

  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"
```

## 最佳实践

1. **本地开发**: 不设置环境变量，使用默认的 dev 配置
2. **测试环境**: 使用 `APP_ENV=dev` 并通过环境变量覆盖特定配置
3. **生产环境**: 使用 `APP_ENV=prod` 并通过环境变量覆盖敏感信息（如密码）
4. **安全性**: 
   - 不要在配置文件中硬编码生产环境密码
   - 使用环境变量或密钥管理系统存储敏感信息
   - 将 `.env` 文件添加到 `.gitignore`

## 验证配置

启动时查看日志输出，确认加载的环境：

```
2026-02-16T10:00:00.000+0800    INFO    Risk服务启动中...
    {"http_port": 9080, "grpc_port": 9090, "nacos_enabled": false}
```

或者在代码中添加日志：

```go
logger.Info("配置加载成功",
    zap.String("env", env),
    zap.String("redis_addr", cfg.Redis.Addr),
    zap.Bool("nacos_enabled", cfg.Nacos.Enable))
```

## 故障排查

### 问题: 配置文件未找到
```
读取配置文件 risk.dev.yaml 失败
```
**解决**: 确保在项目根目录运行，或配置文件在 `configs/` 目录中

### 问题: 环境变量未生效
**解决**: 
1. 确认环境变量名称正确（区分大小写）
2. Windows PowerShell 使用 `$env:VAR_NAME`
3. 重启终端或重新设置环境变量

### 问题: Redis 连接失败
**解决**:
1. 检查 Redis 地址和端口
2. 验证 Redis 密码
3. 确认网络连接和防火墙设置
