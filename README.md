# Risk Engine

`risk-engine` 是一个 Go 单仓双服务项目，包含风控服务和验证码服务。两个服务共享配置加载、日志、健康检查、Redis 连接和 Nacos 注册等基础设施。

## 服务组成

```text
cmd/
  risk-server/       风控服务入口
  captcha-server/    验证码服务入口

internal/
  risk/              风控服务领域、应用层、适配器和组装代码
  captcha/           验证码服务领域、应用层、适配器和组装代码
  shared/            共享配置、日志、健康检查、注册中心

configs/             dev/prod/template 配置
docs/                接口、架构和重构文档
web-test/            验证码 Web 调试页和 gRPC 调试工具
```

## 架构说明

风控服务和验证码服务都按六边形架构组织：

```text
domain
  领域模型和领域错误

application
  应用用例和业务编排

application/ports
  入站端口和出站端口

adapter/inbound
  HTTP / gRPC 等入站适配器

adapter/outbound
  Redis、第三方库、外部资源等出站适配器

bootstrap
  生产环境依赖组装
```

核心原则是业务层不直接依赖 Gin、gRPC、Redis、proto DTO 或配置文件结构，外部协议和基础设施通过适配器接入。

## 风控服务

入口：

```bash
go run ./cmd/risk-server --env dev
```

也可以显式指定配置文件：

```bash
go run ./cmd/risk-server --config ./configs/risk.dev.yaml
```

主要能力：

- gRPC 风控检测：黑名单、IP 频控、登录失败保护
- gRPC 事件上报：登录成功/失败计数
- gRPC 黑名单写入
- gRPC 用户行为限流：在线自测、判题提交
- HTTP 健康检查和服务信息
- HTTP 管理接口：风险 IP 列表、详情和事件历史

常用地址：

```text
GET /health
GET /info
GET /api/v1/admin/risk-ips
GET /api/v1/admin/risk-ips/{ip}
GET /api/v1/admin/risk-ips/{ip}/events
```

## 验证码服务

入口：

```bash
go run ./cmd/captcha-server --env dev
```

也可以显式指定配置文件：

```bash
go run ./cmd/captcha-server --config ./configs/captcha.dev.yaml
```

主要能力：

- HTTP 滑块验证码生成
- HTTP 滑块验证码校验
- 验证通过后签发短期 token
- gRPC token 校验
- 运行时图片源管理
- 图片池刷新和健康检查

常用地址：

```text
GET  /api/v1/captcha
POST /api/v1/captcha/verify
GET  /health
GET  /actuator/health
```

## 配置

开发环境配置文件：

```text
configs/risk.dev.yaml
configs/captcha.dev.yaml
```

常用启动参数：

```bash
--env dev
--config ./configs/risk.dev.yaml
--config ./configs/captcha.dev.yaml
```

常用环境变量：

```bash
RISK_APP_ENV=dev
RISK_CONFIG_FILE=./configs/risk.dev.yaml
RISK_REDIS_ADDR=127.0.0.1:6379

CAPTCHA_APP_ENV=dev
CAPTCHA_CONFIG_FILE=./configs/captcha.dev.yaml
CAPTCHA_REDIS_ADDR=127.0.0.1:6379
CAPTCHA_TOKEN_SECRET=replace-me
```

完整配置说明见：

```text
docs/配置说明.md
```

## 构建和测试

运行全部测试：

```bash
go test ./...
```

如果本机 Go cache 目录存在权限问题，可以指定项目内缓存：

```powershell
$env:GOCACHE="E:\Myspace\risk-engine\.gocache"; go test ./...
```

构建服务：

```bash
go build ./cmd/risk-server
go build ./cmd/captcha-server
```

## 调试工具

验证码 Web 调试页：

```text
web-test/index.html
```

验证码 gRPC token 校验工具：

```bash
go run ./web-test/test_grpc_client.go -addr localhost:9091 <TOKEN>
```

## 参考文档

```text
docs/风控服务接口说明.md
docs/风控服务六边形重构计划.md
docs/风控统计方案.md
docs/验证码服务接口说明.md
docs/验证码服务六边形架构说明.md
docs/验证码运行时图片源指南.md
```
