# Risk Engine

`risk-engine` 是一个双服务 Go 单仓，包含验证码服务和风控服务，两者共享配置、健康检查和注册中心等基础设施。

## 项目结构

```text
.
├── cmd/
│   ├── captcha-server/      # 验证码服务入口
│   └── risk-server/         # 风控服务入口
├── configs/                 # 各服务 dev/prod/template 配置
├── docs/                    # 契约与设计文档
├── internal/
│   ├── captcha/             # 验证码领域
│   ├── risk/                # 风控领域
│   └── shared/              # 共享基础设施
└── web-test/                # Web 调试页与 gRPC 调试工具
```

## 运行入口

当前推荐通过 CLI 参数和带服务前缀的环境变量启动，不再依赖仓库内不存在的 `start.ps1` / `start.sh`。

- CLI 参数: `--config <path>`、`--env <name>`
- 全局回退: `APP_ENV`、`CONFIG_FILE`
- 服务前缀环境变量: `RISK_*`、`CAPTCHA_*`
- 兼容别名: 旧的无前缀变量仍可读取，但应视为过渡能力

完整配置说明见 [docs/配置说明.md](docs/配置说明.md)。

## 快速启动

1. 复制环境变量模板

```bash
cp .env.example .env
```

2. 启动验证码服务

```bash
go run ./cmd/captcha-server --env dev
```

3. 启动风控服务

```bash
go run ./cmd/risk-server --env dev
```

也可以显式指定配置文件：

```bash
go run ./cmd/captcha-server --config ./configs/captcha.dev.yaml
go run ./cmd/risk-server --config ./configs/risk.dev.yaml
```

## 常用环境变量

```bash
RISK_APP_ENV=dev
RISK_CONFIG_FILE=./configs/risk.dev.yaml
RISK_REDIS_ADDR=127.0.0.1:6379

CAPTCHA_APP_ENV=dev
CAPTCHA_CONFIG_FILE=./configs/captcha.dev.yaml
CAPTCHA_REDIS_ADDR=127.0.0.1:6379
CAPTCHA_TOKEN_SECRET=replace-me
```

## 常用地址

- Captcha health: `http://localhost:8091/health`
- Captcha actuator: `http://localhost:8091/actuator/health`
- Risk health: `http://localhost:9080/health`
- Risk info: `http://localhost:9080/info`

## Web 调试工具

- 直接打开 `web-test/index.html`
- 页面顶部输入框会将 API Base 持久化到 `localStorage`
- 也可以通过 `?api=http://localhost:8091/api/v1` 覆盖默认地址
- gRPC Token 校验工具：

```bash
go run ./web-test/test_grpc_client.go -addr localhost:9091 <TOKEN>
```

## 构建

```bash
go build ./cmd/risk-server ./cmd/captcha-server ./web-test
```

## 参考文档

- [配置契约](docs/配置说明.md)
- [验证码前端调试说明](web-test/README.md)
