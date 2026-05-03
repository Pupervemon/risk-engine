# Captcha Service 接口总览

版本: `draft-2026-04-18`

状态: `current`

本文档用于简要展示 `captcha-server` 当前对外暴露的 HTTP 和 gRPC 接口。
如果文档与代码不一致，以当前代码、生成后的 Swagger 文档和 `risk-proto` 中的 proto 定义为准。

## 1. 接口范围

- HTTP 服务入口: `cmd/captcha-server`
- HTTP 路由实现: `internal/captcha/adapter/inbound/http`
- gRPC 服务实现: `internal/captcha/adapter/inbound/grpc`
- Swagger 导出目录: `docs/swagger/captcha`

## 2. HTTP 接口

说明:

- BasePath: `/`
- 返回格式: `application/json`
- 管理接口需要认证头:
  `X-User-Id`: 网关解析 token 后注入的当前用户 ID
  `X-User-Roles`: 网关解析 token 后注入的当前用户角色列表，支持逗号分隔或 JSON 数组；必须包含 `3`(admin)

### 2.1 验证码接口

| 方法 | 路径 | 说明 | 主要响应 |
| --- | --- | --- | --- |
| GET | `/api/v1/captcha` | 生成滑块验证码 | `200`, `500` |
| POST | `/api/v1/captcha/verify` | 校验滑块验证码并签发短期 token | `200`, `400`, `500` |

### 2.2 图片源管理接口

| 方法 | 路径 | 说明 | 主要响应 |
| --- | --- | --- | --- |
| GET | `/api/v1/admin/image-source` | 查询当前运行时图片源状态 | `200`, `401`, `403`, `500` |
| POST | `/api/v1/admin/image-source/validate` | 校验候选图片源配置但不应用 | `200`, `400`, `401`, `403`, `409` |
| PUT | `/api/v1/admin/image-source` | 更新并持久化运行时图片源配置 | `200`, `400`, `401`, `403`, `409`, `500`, `502` |
| POST | `/api/v1/admin/image-source/refresh` | 使用当前配置立即刷新图片池 | `200`, `401`, `403`, `409`, `502` |

### 2.3 健康检查接口

| 方法 | 路径 | 说明 | 主要响应 |
| --- | --- | --- | --- |
| GET | `/health` | 轻量健康检查 | `200`, `503` |
| GET | `/actuator/health` | 详细健康检查 | `200`, `503` |
| GET | `/actuator/health/liveness` | 存活探针 | `200` |
| GET | `/actuator/health/readiness` | 就绪探针 | `200`, `503` |

## 3. gRPC 接口

说明:

- gRPC 服务名: `CaptchaTokenService`
- proto 包路径: `github.com/Pupervemon/risk-proto/gen/go/captcha/v1`

当前 HTTP 文档重点覆盖 HTTP 接口。gRPC 以 proto 定义为准。

## 4. Swagger 导出

从仓库根目录执行:

```bash
swag init --parseInternal --outputTypes json,yaml --dir cmd/captcha-server,internal/captcha/adapter/inbound/http -g main.go -o docs/swagger/captcha
```

当前已生成:

- `docs/swagger/captcha/swagger.yaml`
- `docs/swagger/captcha/swagger.json`

## 5. 代码定位

- HTTP 注解入口: `cmd/captcha-server/main.go`
- HTTP 文档 stub: `internal/captcha/adapter/inbound/http/swagger.go`
- HTTP 文档模型: `internal/captcha/adapter/inbound/http/swagger_models.go`
- HTTP handler: `internal/captcha/adapter/inbound/http`
