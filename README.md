# Risk Engine (风控引擎)

[![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat&logo=go)](https://golang.org)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

本项目是一个高性能的分布式风控系统，基于 Go 语言开发。采用微服务架构，在同一仓库内维护两个高度解耦的服务：**风险引擎 (Risk Service)** 与 **验证码服务 (Captcha Service)**，通过 Nacos 进行服务注册与发现。

## ✨ 核心特性

### 🛡️ 风险引擎 (Risk Service)
- **黑名单管理**
  - 支持基于 IP 和 UserID 的动态黑名单拦截
  - 可设置临时或永久封禁时长
  - 支持批量导入和实时更新

- **智能频控检测**
  - **多维度限流**：IP 级别和用户级别的双重限流机制
  - **原子操作**：基于 Redis Lua 脚本实现高性能原子计数，确保并发安全
  - **灵活配置**：支持不同场景的自定义限流规则

- **防暴力破解**
  - 识别高频失败登录等异常行为
  - 自动触发验证码挑战机制
  - 可配置的失败次数阈值和时间窗口

- **服务注册**
  - 集成 Nacos 服务注册中心
  - 支持服务发现与负载均衡
  - 提供健康检查端点

### 🔐 验证码服务 (Captcha Service)
- **滑动拼图验证**
  - 基于 `go-captcha` 库实现交互友好的滑动验证
  - 随机生成拼图缺口位置，防止机器学习破解
  - 支持自定义拼图尺寸和容差范围

- **安全 Token 机制**
  - 验证通过后签发基于 HMAC-SHA256 的无状态 Token
  - Token 自动过期机制，防止重放攻击
  - 独立的 Token 验证接口，支持跨服务调用

- **双协议支持**
  - **HTTP REST API**：前端直接调用，获取验证码和提交验证结果
  - **gRPC 接口**：内部服务调用，用于 Token 合法性验证

- **服务注册与健康检查**
  - 自动注册到 Nacos 服务中心
  - 多级健康检查端点（Kubernetes 就绪/存活探针）
  - 支持优雅关闭，确保流量平滑切换

## 🏗️ 系统架构

### 整体架构图

```
┌─────────────────────────────────────────────────────────────┐
│                    Spring Cloud Gateway                      │
│                   (Nacos 服务发现 + 负载均衡)                  │
└────────────┬─────────────────────────┬──────────────────────┘
             │                         │
             ▼                         ▼
    ┌────────────────┐        ┌────────────────┐
    │  Captcha Service│        │  Risk Service  │
    │   HTTP: 8091    │        │   HTTP: 9080   │
    │   gRPC: 9091    │        │   gRPC: 9090   │
    └────────┬───────┘        └────────┬────────┘
             │                         │
             └──────────┬──────────────┘
                        ▼
                 ┌─────────────┐
                 │    Redis    │
                 │   (存储层)   │
                 └─────────────┘
                        ▲
                        │
                 ┌─────────────┐
                 │    Nacos    │
                 │  (注册中心)  │
                 └─────────────┘
```

### 服务交互流程

```mermaid
sequenceDiagram
| 类型 | 技术 | 说明 |
|------|------|------|
| **开发语言** | Go 1.25.1+ | 高性能并发处理 |
| **RPC 框架** | gRPC | 服务间高效通信 |
| **HTTP 框架** | Gin | RESTful API 支持 |
| **缓存/存储** | Redis | 验证码答案、频控计数、黑名单存储 |
| **配置管理** | Viper | 支持 YAML 配置文件 |
| **日志系统** | Uber Zap | 结构化 JSON 日志 |
| **服务注册** | Nacos v2 | 服务发现与健康检查 |
| **验证码库** | go-captcha | 滑动拼图验证码生成 |
| **网关** | Spring Cloud Gateway | 统一流量入口与负载均衡 |
    Client->>Gateway: 1. 发起登录请求
    Gateway->>Risk: 2. gRPC Check(ip, userId, scene)
    Risk->>Redis: 3. 查询黑名单 & 频控
    Redis-->>Risk: 4. 返回风险数据
    
    alt 高风险行为
        Risk-->>Gateway: 5. ACTION_VERIFY (需要验证)
        Gateway-->>Client: 6. 返回需要验证码
        
        Client->>Gateway: 7. 请求验证码
        Gateway->>Captcha: 8. GET /api/v1/captcha
        Captcha->>Redis: 9. 生成并存储答案
        Captcha-->>Gateway: 10. 返回验证码图片
        Gateway-->>Client: 11. 展示滑动验证码
        
        Client->>Gateway: 12. 提交滑动坐标
        Gateway->>Captcha: 13. POST /api/v1/captcha/verify
        Captcha->>Redis: 14. 校验答案
        Captcha-->>Gateway: 15. 返回 Token
        Gateway-->>Client: 16. 返回验证通过
        
        Client->>Gateway: 17. 携带 Token 重试登录
        Gateway->>Captcha: 18. gRPC VerifyToken(token)
        Captcha->>Redis: 19. 验证 Token 有效性
        Captcha-->>Gateway: 20. Token 有效
        Gateway->>Risk: 21. 上报登录成功事件
        Gateway-->>Client: 22. 登录成功
    else 正常行为
        Risk-->>Gateway: 5. ACTION_PASS (放行)
        Gateway->>Risk: 6. 上报登录成功
        Gateway-->>Client: 7. 直接登录成功
    end
```

### 设计原则

- **职责分离**：风控服务负责风险判断，验证码服务负责人机验证
- **服务自治**：每个服务独立部署、独立扩展、独立故障隔离
- **无状态设计**：通过 Redis 和 Token 机制实现无状态服务
- **高可用性**：支持多实例部署，通过 Nacos 实现负载均衡

## 🛠️ 技术栈

- **语言**：Go (v1.25.1+)
- **通信**：gRPC, Gin (HTTP)
- **存储**：Redis (用于存储验证码答案及频控计数)
- **配置**：Viper
- **日志**：Uber-go Zap (结构化 JSON 日志)
- **服务注册**：Nacos (支持服务发现与负载均衡)
                            # 服务入口
│   ├── captcha-server/            
│   │   └── main.go                 # 验证码服务启动入口
│   └── risk-server/               
│       └── main.go                 # 风控服务启动入口
│
├── internal/                       # 内部代码（不对外暴露）
│   ├── captcha/                    # 验证码服务模块
│   │   ├── service/                # 业务逻辑层
│   │   │   ├── captcha.go          # 滑动验证码生成与校验
│   │   │   ├── captcha_grpc.go     # gRPC 服务实现
│   │   │   └── token.go            # Token 签发与验证
│   │   └── transport/              # 传输层
│   │       └── http/
│   │           ├── captcha_handler.go  # HTTP 路由处理
│   │           ├── health_handler.go   # 健康检查端点
│   │           └── router.go           # 路由配置
│   │
│   ├── risk/                       # 风控服务模块
│   │   ├── service/
│   │   │   └── risk.go             # 风控检测、黑名单、频控逻辑
│   │   └── transport/
│   │       └── health_handler.go   # 健康检查路由
│   │
│   └── shared/                     # 共享模块（跨服务复用）
│       ├── conf与发现

两个服务均已集成 Nacos 服务注册中心：

### 功能特性

| 功能 | Captcha Service | Risk Service |
|------|----------------|--------------|
| **自动注册** | ✅ HTTP 8091 | ✅ HTTP 9080 |
| **健康检查** | ✅ 多端点支持 | ✅ 多端点支持 |
| **负载均衡** | ✅ 权重配置 | ✅ 权重配置 |
| **优雅关闭** | ✅ 自动注销 | ✅ 自动注销 |
| **元数据** | ✅ 协议/版本/端口 | ✅ 协议/版本/端口 |

### 健康检查端点

两个服务都提供标准的健康检查接口：

```bash
# 详细健康检查（检查所有依赖）
GET /health

# Kubernetes 就绪探针（检查关键依赖）
GET /health/ready

# Kubernetes 存活探针（快速检查）
GET /health/live
```

### 配置示例

**验证码服务** (`configs/captcha.yaml`)：
```yaml
http:
  port: 8091                          # HTTP 服务端口

naco� 快速开始

### 前置要求

- **Go**: 1.25.1 或更高版本
- **Redis**: 6.0+ （用于存储验证码答案和频控数据）
- **Nacos**: 2.0+ （可选，用于服务注册）
- **Protobuf**: 本项目遵循 API First 原则

### 1. Protobuf 依赖管理

本项目不直接维护 `.proto` 文件，所有接口定义统一维护在独立仓库：

👉 **[risk-proto](https://github.com/Pupervemon/risk-proto)**

**本地开发环境目录结构**：
```text
workspace/
├── risk-engine/     # 当前仓库
└── risk-proto/      # 接口仓库（需手动克隆）
```

**克隆接口仓库**：
```bash
cd ..
git clone https://github.com/Pupervemon/risk-proto.git
cd risk-engine
```

`go.mod` 中已配置 `replace` 指令指向本地 `../risk-proto` 目录。

### 2. 配置环境

#### 2.1 创建配置文件

```bash
# 复制配置模板
cp configs/captcha.template.yaml configs/captcha.yaml
cp configs/risk.template.yaml configs/risk.yaml
```

#### 2.2 修改关键配置

**修改 `configs/captcha.yaml`**：
```yaml
redis:
  addr: "127.0.0.1:6379"              # Redis 地址
  pass测试与验证

### 1. 滑动验证码演示

在浏览器中打开 `web-test/index.html`，可以：
- 获取滑动验证码
- 进行滑动操作
- 查看验证结果和 Token

### 2. gRPC 接口测试

**测试 Token 验证接口**：
```bash
# 先通过 HTTP 获取 Token，然后测试 gRPC 接口
go run web-test/test_grpc_client.go "YOUR_TOKEN_HERE"
```

### 3. API 测试示例

**获取验证码**：
```bash
curl -X GET http://localhost:8091/api/v1/captcha
```

**验证滑动结果**：
```bash
curl -X POST http://localhost:8091/api/v1/captcha/verify \
  -H "Content-Type: application/json" \
  -d '{
    "captchaId": "xxx",
    "pointX": 123
  }'
```

**风控检测**：
```bash
# 需要使用 gRPC 客户端（参考 web-test/test_grpc_client.go）
```

## 📊 监控与运维

### 健康检查端点

| 端点 | 说明 | 用途 |
|------|------|------|
| `/health` | 详细健康检查 | 检查所有依赖（Redis 等） |
| `/health/ready` | 就绪探针 | Kubernetes Readiness Probe |
| `/health/live` | 存活探针 | Kubernetes Liveness Probe |

### Kubernetes 部署示例

```yaml
apiVersion: v1
kind: Deployment
metadata:
  name: captcha-service
spec:
  replicas: 3
  template:
    spec:
      containers:
      - name: captcha
        image: captcha-service:latest
        ports:
        - containerPort: 8091
        - containerPort: 9091
        livenessProbe:
          httpGet:
            path: /health/live
            port: 8091
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /health/ready
            port: 8091
          initialDelaySeconds: 5
          periodSeconds: 5
```

## 🔧 开发指南

### 项目规范

- **代码风格**：遵循 Go 官方代码规范
- **提交规范**：使用语义化提交信息
- **分支策略**：main (生产) / develop (开发)

### 本地开发建议

1. **使用 Air 进行热重载**：
   ```bash
   go install github.com/cosmtrek/air@latest
   air
   ```

2. **代码格式化**：
   ```bash
   go fmt ./...
   goimports -w .
   ```

3. **代码检查**：
   ```bash
   golangci-lint run
   ```

## 📝 许可证

本项目采用 MIT 许可证。详见 [LICENSE](LICENSE) 文件。

## 👥 贡献

欢迎提交 Issue 和 Pull Request！

## 📮 联系方式

- **作者**: [Pupervemon](https://github.com/Pupervemon)
- **项目地址**: [risk-engine](https://github.com/Pupervemon/risk-engine)
- **接口定义**: [risk-proto](https://github.com/Pupervemon/risk-proto)

---

⭐ 如果这个项目对你有帮助，欢迎 Star！
```yaml
redis:
  addr: "127.0.0.1:6379"
  password: "your_redis_password"

risk_rules:
  login:
    max_fail_count: 5                 # 失败次数阈值
    fail_count_expire_minutes: 30     # 失败计数过期时间
  ip_rate_limit:
    limit: 100                        # IP 频控限制
    window_seconds: 1                 # 时间窗口

nacos:
  enable: true
  server_addr: "127.0.0.1:8848"
```

### 3. 安装依赖

```bash
go mod download
go mod tidy
```

### 4. 启动服务

#### 方式一：开发模式

**启动验证码服务**：
```bash
go run cmd/captcha-server/main.go
```
- HTTP 端口: `8091`
- gRPC 端口: `9091`

**启动风控服务**：
```bash
go run cmd/risk-server/main.go
```
- HTTP 端口: `9080` (健康检查)
- gRPC 端口: `9090` (业务接口)

#### 方式二：编译后运行

```bash
# 编译
go build -o bin/captcha-server cmd/captcha-server/main.go
go build -o bin/risk-server cmd/risk-server/main.go

# 运行
./bin/captcha-server &
./bin/risk-server &
```

#### 方式三：Windows 构建 Linux 二进制

```bash
# 运行构建脚本
build.bat

# 输出文件位于 dist/ 目录
# dist/captcha-server (Linux amd64)
# dist/risk-server (Linux amd64)
```

### 5. 验证服务状态

**检查服务健康状态**：
```bash
# 验证码服务
curl http://localhost:8091/health

# 风控服务
curl http://localhost:9080/health
```

**查看 Nacos 注册状态**：
访问 Nacos 控制台 `http://localhost:8848/nacos`，在服务列表中应能看到：
- `captcha-service`
- `risk-service
      discovery:
        locator:
          enabled: true                # 启用服务发现
      routes:
        # 验证码服务路由
        - id: captcha-service
          uri: lb://captcha-service    # 通过 Nacos 负载均衡
          predicates:
            - Path=/api/v1/captcha/**
          filters:
            - StripPrefix=0
        
        # 风控服务路由（如果需要 HTTP 访问）
        - id: risk-service
          uri: lb://risk-service
          predicates:
            - Path=/api/v1/risk/**
```

📖 **详细文档**：
- [Nacos 集成完整文档](docs/NACOS_INTEGRATION.md)
- [风控服务 Nacos 集成](docs/RISK_ervice) 与传输层(transport) 分离
- **独立部署**：每个服务可以独立编译和部署
- **配置驱动**：所有业务规则通过配置文件管理，无需修改代码 └── QUICKSTART.md           # 快速启动指南
├── web-test/               # 测试工具 (前端演示页面 & gRPC 客户端脚本)
├── build.bat               # Windows 一键构建脚本 (输出 Linux 二进制)
├── go.mod                  # 依赖管理
└── README.md
```

## 🌐 Nacos 服务注册集成

验证码服务已集成 Nacos 服务注册中心，支持：
- ✅ **自动服务注册与发现**：服务启动时自动注册到 Nacos
- ✅ **健康检查**：提供多个健康检查端点（/health, /actuator/health）
- ✅ **负载均衡**：支持多实例部署，通过 Spring Cloud Gateway 自动负载均衡
- ✅ **优雅关闭**：服务停止时自动注销，确保流量不会路由到已关闭的实例

### 配置示例
```yaml
# configs/captcha.yaml
nacos:
  enable: true                        # 启用/禁用 Nacos 注册
  server_addr: "127.0.0.1:8848"       # Nacos 服务器地址
  service_name: "captcha-service"     # 服务名称
  group_name: "DEFAULT_GROUP"         # 服务分组
  weight: 1.0                         # 负载均衡权重
```

### Gateway集成
在 Spring Cloud Gateway 中配置路由：
```yaml
spring:
  cloud:
    gateway:
      routes:
        - id: captcha-service
          uri: lb://captcha-service    # 通过服务名负载均衡
          predicates:
            - Path=/captcha/**
```

📖 **详细文档**：
- [Nacos集成完整文档](docs/NACOS_INTEGRATION.md)
- [快速启动指南](docs/QUICKSTART.md)

## 🚥 快速开始

### 1. Protobuf 依赖管理 (API First)

本项目遵循 **API First** 原则，不直接维护任何 `.proto` 文件。所有的接口定义及生成的 Go 代码均统一维护在独立仓库：
👉 **[risk-proto](https://github.com/Pupervemon/risk-proto)**

- **引用方式**：本项目通过 `go.mod` 直接引用 `github.com/Pupervemon/risk-proto`。
- **本地开发**：为了方便联调，`go.mod` 中默认使用了 `replace` 指令指向本地的 `../risk-proto` 目录。
- **环境要求**：在本地开发环境下，请确保 `risk-proto` 仓库已克隆至与本项目平行的目录中：
  ```text
  workspace/
  ├── risk-engine/  (当前仓库)
  └── risk-proto/   (接口仓库)
  ```

### 2. 配置环境

1. 准备配置文件：
   ```bash
   cp configs/risk.template.yaml configs/risk.yaml
   ```
2. 配置验证码服务（涉及 Redis 及签名 Secret）：
   ```bash
   cp configs/captcha.template.yaml configs/captcha.yaml
   ```

### 2. 运行服务

**启动风险引擎：**
```bash
go run cmd/risk-server/main.go
```
*默认端口：gRPC :9090*

**启动验证码服务：**
```bash
go run cmd/captcha-server/main.go
```
*默认端口：HTTP :8080, gRPC :9091*

### 3. 构建部署

运行 `build.bat` 将在 `dist/` 目录下生成适用于 Linux 环境的二进制文件：
- `dist/risk-server`
- `dist/captcha-server`

## 🧪 验证与调试

本项目提供了便捷的本地验证工具：
1. **滑动验证演示**：直接在浏览器打开 `web-test/index.html`，可进行完整的滑动验证流程。
2. **gRPC 接口测试**：
   ```bash
   go run web-test/test_grpc_client.go "YOUR_TOKEN_HERE"
   ```

---
Developed by [Pupervemon](https://github.com/Pupervemon)
