# Captcha 服务六边形架构重构方案

## 1. 背景与目标

当前 captcha 服务的核心代码主要集中在 `internal/captcha/service` 包中，已经具备验证码生成、验证码校验、鼠标轨迹校验、token 签发与校验、图片池刷新、运行时图片源热更新等能力。但从架构边界看，应用逻辑、领域模型、Redis 访问、外部图片 API、`go-captcha` 生成器、HTTP/gRPC 传输 DTO 之间仍然耦合较紧。

本次重构目标是将 captcha 服务调整为六边形架构，也就是让业务核心只依赖内部定义的端口，HTTP/gRPC、Redis、外部图片服务、验证码生成库等技术实现都作为适配器挂在外侧。

本方案只规划重构路线，不在本阶段修改业务代码。

## 2. 当前结构梳理

### 2.1 服务入口

- `cmd/captcha-server/main.go`
  - 加载 captcha 配置。
  - 初始化 Redis。
  - 创建 `CaptchaService`、`TokenService`、`CaptchaTokenService`。
  - 启用运行时图片源管理器。
  - 启动图片池刷新任务。
  - 组装 HTTP router、gRPC server、健康检查、Nacos 注册。

### 2.2 核心业务代码

- `internal/captcha/service/captcha.go`
  - 生成滑块验证码。
  - 保存、读取、删除验证码答案。
  - 校验滑块答案和鼠标轨迹。
  - 启停图片池刷新任务。
  - 直接依赖 Redis、`go-captcha`、图片池和配置结构。
- `internal/captcha/service/token.go`
  - 签发 captcha token。
  - 校验 token 签名、有效期和 Redis 中的 token 状态。
- `internal/captcha/service/track_validator.go`
  - 鼠标轨迹校验规则。
- `internal/captcha/service/image_provider.go`
  - Redis 图片池。
  - 图片池刷新调度。
  - Redis 分布式锁。
- `internal/captcha/service/image_fetcher.go`
  - 外部图片 API 调用。
  - JSON 解析、图片下载、图片规范化。
  - Mock 图片源。
- `internal/captcha/service/image_source_*`
  - 运行时图片源配置状态机。
  - Redis 持久化。
  - 管理接口使用的状态、校验、更新、刷新能力。

### 2.3 适配层

- `internal/captcha/transport/http`
  - `GET /api/v1/captcha`
  - `POST /api/v1/captcha/verify`
  - `/api/v1/admin/image-source*`
  - `/health` 和 actuator 风格健康检查。
- `internal/captcha/service/captcha_grpc.go`
  - 当前 gRPC 实现仍放在 `service` 包中，直接依赖 `TokenService`。

### 2.4 主要耦合点

1. `CaptchaService` 同时承担应用编排、领域规则、Redis 存储、图片池、图片生成器管理。
2. HTTP handler 直接依赖具体的 `*captchaservice.CaptchaService` 和 `*TokenService`，没有面向用例端口。
3. gRPC transport 实现位于 `service` 包，传输层和应用层方向反了。
4. 运行时图片源管理既有业务状态机，也有 Redis store 和外部图片 fetcher 细节。
5. 配置结构 `internal/shared/config` 直接被核心 service 使用，业务核心会感知启动配置的形状。
6. Redis key、分布式锁、图片池 generation 规则散落在 service 包里，未来替换存储或测试隔离成本较高。

## 3. 六边形架构目标形态

目标依赖方向：

```text
cmd/captcha-server
        |
        v
inbound adapters: HTTP / gRPC / health
        |
        v
application ports + use cases
        |
        v
domain model + domain services
        ^
        |
outbound ports
        ^
        |
outbound adapters: Redis / external image API / go-captcha / registry / logger
```

核心原则：

- 领域层不 import Redis、Gin、gRPC、`go-captcha`、Viper 配置结构。
- 应用层只依赖端口接口，不依赖具体外部实现。
- 入站适配器只做协议转换、鉴权、错误映射和响应输出。
- 出站适配器封装 Redis、外部 HTTP、图片处理、验证码生成库等技术细节。
- `cmd/captcha-server/main.go` 作为 composition root，负责把具体适配器注入到用例中。

## 4. 建议目标目录

```text
internal/captcha/
  domain/
    captcha.go
    token.go
    track.go
    image_source.go
    errors.go

  application/
    captcha_usecase.go
    token_usecase.go
    image_source_usecase.go
    lifecycle.go
    ports/
      inbound.go
      outbound.go

  adapter/
    inbound/
      http/
        router.go
        captcha_handler.go
        image_source_handler.go
        health_handler.go
        admin_auth.go
        swagger.go
        swagger_models.go
      grpc/
        token_service.go
    outbound/
      redis/
        answer_repository.go
        token_repository.go
        image_pool_repository.go
        image_source_store.go
        lock.go
      captcha/
        slide_generator.go
      image/
        external_fetcher.go
        mock_fetcher.go
        normalizer.go
      health/
        checker.go

  bootstrap/
    config_mapper.go
    wiring.go
```

说明：

- `domain` 保存业务概念和值对象，不放框架代码。
- `application` 保存用例编排和端口定义。
- `adapter/inbound` 保存 HTTP/gRPC 入口。
- `adapter/outbound` 保存 Redis、外部 API、第三方验证码库等实现。
- `bootstrap` 可选，用于把 `shared/config` 转成应用层配置并集中组装依赖。也可以先放在 `cmd/captcha-server/main.go`，等结构稳定后再抽出。

## 5. 端口设计

### 5.1 入站端口

HTTP 和 gRPC 都应该面向这些用例接口，而不是具体实现类型。

```go
type CaptchaUseCase interface {
    Generate(ctx context.Context) (domain.SliderChallenge, error)
    Verify(ctx context.Context, cmd VerifyCaptchaCommand) (VerifyCaptchaResult, error)
}

type TokenUseCase interface {
    Issue(ctx context.Context, captchaID string) (IssuedToken, error)
    Verify(ctx context.Context, token string) (TokenVerification, error)
}

type ImageSourceUseCase interface {
    Status(ctx context.Context) (ImageSourceStatus, error)
    Validate(ctx context.Context, patch ImageSourcePatch) (ImageSourceValidationResult, error)
    Update(ctx context.Context, patch ImageSourcePatch, triggerRefresh bool) (ImageSourceStatus, error)
    Refresh(ctx context.Context) (ImageSourceStatus, error)
}

type CaptchaLifecycle interface {
    StartImageRefresh(ctx context.Context) error
    StopImageRefresh()
}
```

### 5.2 出站端口

应用层通过出站端口访问外部世界。

```go
type CaptchaAnswerRepository interface {
    Save(ctx context.Context, id string, answer domain.SliderAnswer, ttl time.Duration) error
    Get(ctx context.Context, id string) (domain.SliderAnswer, error)
    Delete(ctx context.Context, id string) error
}

type TokenRepository interface {
    Save(ctx context.Context, tokenDigest string, payload []byte, ttl time.Duration) error
    Exists(ctx context.Context, tokenDigest string) (bool, error)
}

type SliderGenerator interface {
    Generate(ctx context.Context, background []byte) (domain.GeneratedSlider, error)
}

type BackgroundImagePool interface {
    Random(ctx context.Context) ([]byte, error)
    Snapshot(ctx context.Context) (domain.ImagePoolSnapshot, error)
    Refresh(ctx context.Context) error
    RefreshWithProvider(ctx context.Context, provider ImageProvider) error
    Start(ctx context.Context, interval time.Duration, refreshOnStartup bool)
    Stop()
}

type ImageProvider interface {
    FetchImages(ctx context.Context, count int) ([]domain.ImageMeta, error)
}

type RuntimeImageSourceStore interface {
    Load(ctx context.Context) (domain.ImageSourceRuntimeConfig, bool, error)
    Save(ctx context.Context, cfg domain.ImageSourceRuntimeConfig) error
}
```

端口命名可以在实际重构时根据 Go 包命名再收敛，重点是先把方向固定下来。

## 6. 领域模型建议

领域层建议从现有 service 类型中抽出以下模型：

- `SliderChallenge`
  - `CaptchaID`
  - `MasterImage`
  - `TileImage`
  - `TargetY`
  - `ExpiresIn`
  - `RequireMouseTrack`
- `SliderAnswer`
  - `DX`
  - `DY`
- `TrackPoint`
  - `X`
  - `Y`
  - `Time`
- `TrackValidationResult`
  - `Valid`
  - `Code`
  - `Message`
- `TokenPayload`
  - `CaptchaID`
  - `IssuedAt`
  - `ExpiresAt`
- `ImageMeta`
  - `ID`
  - `Data`
  - `URL`
- `ImageSourceRuntimeConfig`
- `ImageSourcePatch`
- `ImageSourceStatus`
- `ImageSourceValidationResult`
- `ImagePoolSnapshot`

领域错误建议集中定义，避免 HTTP/gRPC 适配器依赖字符串判断：

- `ErrCaptchaIDEmpty`
- `ErrCaptchaNotFound`
- `ErrCaptchaMismatch`
- `ErrTokenEmpty`
- `ErrTokenInvalid`
- `ErrImagePoolDisabled`
- `ErrImagePoolRefreshInProgress`
- `ErrImageSourceInvalid`
- `ErrImageSourceRefreshFailed`
- `ErrImageSourcePersistFailed`

对外仍可保持现有错误 reason 字符串，例如 `CAPTCHA_MISMATCH`、`TOKEN_EXPIRED`、`IMAGE_POOL_DISABLED`，但这些字符串应由应用层结果或错误 mapper 统一产出。

## 7. 用例拆分

### 7.1 Captcha 用例

职责：

1. 生成验证码。
2. 保存验证码答案。
3. 校验滑块答案。
4. 在启用轨迹校验时校验鼠标轨迹。
5. 不直接知道 Redis 和 `go-captcha`。

依赖端口：

- `SliderGenerator`
- `CaptchaAnswerRepository`
- `BackgroundImagePool`
- `TrackValidator`
- `IDGenerator`
- `Clock`

当前 `CaptchaService.Generate` 中直接重建 `slideCaptcha` 的逻辑应迁移到 `adapter/outbound/captcha/slide_generator.go`。应用层只知道“给定可选背景图，生成滑块挑战和答案”。

### 7.2 Token 用例

职责：

1. 签发 token。
2. 校验签名、payload、过期时间、存储状态。

依赖端口：

- `TokenRepository`
- `Signer` 或应用内 HMAC 实现。
- `Clock`

当前 `tokenStorage` 已经是一个很好的过渡点，可先扩大为正式端口，再把 Redis 实现移到 outbound adapter。

### 7.3 Image Source 用例

职责：

1. 查询当前运行时图片源状态。
2. 构建候选配置。
3. 校验候选配置。
4. 持久化新配置。
5. 应用新配置。
6. 按需刷新图片池。

依赖端口：

- `RuntimeImageSourceStore`
- `ImageProviderFactory`
- `BackgroundImagePool`
- `Clock`

当前 `RuntimeImageSourceManager` 的状态机可以保留，但应移入 application 或 domain，且它不能直接构建 `ExternalImageFetcher`。构建 provider 的动作应通过 `ImageProviderFactory` 端口完成。

### 7.4 Lifecycle 用例

职责：

1. 启动图片池刷新。
2. 停止图片池刷新。
3. 启动时恢复运行时图片源配置。

生命周期逻辑可以放在 `application/lifecycle.go`，由 `cmd/captcha-server` 在启动和退出时调用。

## 8. 适配器拆分

### 8.1 HTTP 入站适配器

迁移目标：

- `internal/captcha/transport/http` 移到 `internal/captcha/adapter/inbound/http`。
- handler 字段改为接口：
  - `CaptchaUseCase`
  - `TokenUseCase`
  - `ImageSourceUseCase`
- DTO 仍保留在 HTTP 包中。
- `TrackPoint` DTO 由 HTTP 层转换为 domain 类型。
- HTTP 错误映射集中到 `error_mapper.go`。

兼容要求：

- 路由路径不变。
- 请求 JSON 字段不变。
- 响应 JSON 字段不变。
- HTTP 状态码不变，除非另有明确产品决策。

### 8.2 gRPC 入站适配器

迁移目标：

- `internal/captcha/service/captcha_grpc.go` 移到 `internal/captcha/adapter/inbound/grpc/token_service.go`。
- gRPC server 依赖 `TokenUseCase`。
- proto 包仍由 `github.com/Pupervemon/risk-proto/gen/go/captcha/v1` 提供。

兼容要求：

- `VerifyToken` 行为不变。
- 空 token 仍返回 `TOKEN_EMPTY`。
- 成功和失败 reason 保持现有值。

### 8.3 Redis 出站适配器

迁移目标：

- 验证码答案存储移到 `adapter/outbound/redis/answer_repository.go`。
- token 存储移到 `adapter/outbound/redis/token_repository.go`。
- 图片池 Redis generation、index、data、active key 移到 `adapter/outbound/redis/image_pool_repository.go`。
- 运行时图片源 store 移到 `adapter/outbound/redis/image_source_store.go`。
- 分布式锁移到 `adapter/outbound/redis/lock.go` 或封装在图片池 repository 内。

兼容要求：

- Redis key 必须保持不变：
  - `captcha:slide:{captchaID}`
  - `captcha:token:{sha256}`
  - `captcha:images:active_generation`
  - `captcha:images:generations`
  - `captcha:images:g:{generation}:data:{imageID}`
  - `captcha:images:g:{generation}:index`
  - `captcha:images:refresh:lock`
  - `captcha:image-source:runtime-config`
- JSON 存储格式保持向后兼容。

### 8.4 Captcha 生成器出站适配器

迁移目标：

- 把 `go-captcha` 依赖封装在 `adapter/outbound/captcha`。
- `defaultBackgrounds`、`defaultGraphImages`、拼图 mask 绘制逻辑移入该适配器。
- 应用层不感知 `slide.Captcha`、`option.Size`、`slide.Validate`。

需要特别注意：

- 当前 `CaptchaService.Generate` 在并发请求中会修改 `s.slideCaptcha`。重构后建议 generator 变为无共享可变状态，或者在适配器内显式加锁。
- 校验坐标的逻辑可以由应用层用纯 Go 实现，或由 captcha adapter 暴露 `ValidatePosition` 端口。为了减少对第三方库的依赖扩散，建议应用层实现简单的 tolerance 判断。

### 8.5 外部图片 API 出站适配器

迁移目标：

- `ExternalImageFetcher`、`MockImageFetcher`、图片规范化、JSON payload 解析移到 `adapter/outbound/image`。
- 应用层只依赖 `ImageProvider` 和 `ImageProviderFactory`。

兼容要求：

- 支持当前所有响应格式：
  - 直接图片响应。
  - JSON 中的图片 URL。
  - JSON 中的 base64/data URI。
  - 嵌套 `data/result/payload/body/response`。
- 保持当前响应大小限制、限流、重试、认证头策略。

## 9. 配置与启动组装

`internal/shared/config` 可以继续作为配置加载层，但不要让领域和应用层直接依赖它。建议增加映射层：

```text
shared/config.CaptchaConfigSpec
        |
        v
bootstrap.CaptchaOptions
        |
        v
application.NewCaptchaUseCase(options, ports...)
```

建议应用层配置结构：

- `CaptchaOptions`
  - `TTL`
  - `Width`
  - `Height`
  - `GraphSizeMin`
  - `GraphSizeMax`
  - `SliderTolerance`
  - `RequireTrack`
- `TrackValidationOptions`
- `TokenOptions`
- `ImagePoolOptions`
- `ExternalImageSourceOptions`

`cmd/captcha-server/main.go` 最终只负责：

1. 加载配置。
2. 创建 logger、Redis client、registry。
3. 创建 outbound adapters。
4. 创建 application use cases。
5. 创建 inbound adapters。
6. 启动 HTTP/gRPC/lifecycle。

## 10. 分阶段迁移计划

### 阶段 0：锁定行为基线

目标是在重构前明确不能破坏的行为。

工作项：

1. 记录现有 HTTP/gRPC API 契约。
2. 补充或确认以下测试：
   - `GET /api/v1/captcha` 返回字段完整。
   - `POST /api/v1/captcha/verify` 成功后签发 token。
   - captcha 校验失败后删除答案。
   - token 过期、签名错误、Redis 不存在时 reason 不变。
   - 图片源 validate/update/refresh 的错误映射不变。
3. 固定 Redis key 和 JSON 格式清单。

验收：

- `go test ./internal/captcha/... ./internal/shared/config/...` 通过。
- 可手工跑通 `web-test` 的生成、拖动、校验流程。

### 阶段 1：引入 domain 模型与入站接口

目标是先建立六边形中心，但不移动复杂实现。

工作项：

1. 新增 `internal/captcha/domain`。
2. 新增 `internal/captcha/application/ports`。
3. 把当前 `SliderChallenge`、`TrackPoint`、`ImageSource*` 等类型复制或迁移为 domain/application 类型。
4. 让 HTTP handler 依赖接口，而不是具体 `*CaptchaService`。
5. 暂时用现有 `CaptchaService` 适配这些接口。

验收：

- HTTP handler 不再直接依赖具体 service 类型。
- 外部行为不变。

### 阶段 2：拆分 Token 用例

目标是先处理最小、最清晰的业务。

工作项：

1. 将 `TokenService` 移为 `application.TokenUseCase`。
2. 将 `tokenStorage` 扩展为正式 `TokenRepository` 端口。
3. 将 Redis token 存储移动到 `adapter/outbound/redis`。
4. 将 gRPC 实现移动到 `adapter/inbound/grpc`。

验收：

- token 相关单测保持通过。
- gRPC `VerifyToken` reason 和 expiresAt 行为不变。

### 阶段 3：拆分 Captcha 生成与校验用例

目标是把验证码核心流程从 Redis 和 `go-captcha` 中解耦。

工作项：

1. 新增 `CaptchaAnswerRepository` 端口。
2. 将答案 Redis 读写移动到 outbound Redis adapter。
3. 新增 `SliderGenerator` 端口。
4. 将 `go-captcha` 生成逻辑移动到 outbound captcha adapter。
5. 将轨迹校验调整为 domain service 或 application 内部服务。
6. 将 `CaptchaService.Generate`、`VerifyWithTrack` 拆成 `CaptchaUseCase`。

验收：

- 验证码生成、校验、失败删除答案行为不变。
- 应用层不再 import Redis 和 `go-captcha`。

### 阶段 4：拆分图片池与图片源热更新

目标是处理当前最复杂、状态最多的链路。

工作项：

1. 把图片池 Redis 存储、generation、分布式锁封装进 outbound Redis adapter。
2. 把外部图片 fetcher 移动到 outbound image adapter。
3. 为 `RuntimeImageSourceManager` 注入 `ImageProviderFactory`，移除直接构造 `ExternalImageFetcher`。
4. 移除 `captchaRuntimeImageSourceBindings sync.Map`，改为组合根显式持有 manager/store/pool。
5. 将 `ImageSourceUseCase` 作为应用层入口，HTTP admin handler 只依赖该接口。
6. 将图片池启停放入 lifecycle。

验收：

- 运行时图片源配置恢复逻辑不阻塞启动超过当前 3 秒策略。
- admin image-source 接口行为不变。
- Redis 中图片池 generation 切换和清理行为不变。

### 阶段 5：整理启动组装与包路径

目标是让入口只做组装，不承载业务决策。

工作项：

1. 新增 `bootstrap/wiring.go` 或在 `cmd/captcha-server/main.go` 中集中 wiring。
2. 将配置映射逻辑收敛到 `bootstrap/config_mapper.go`。
3. 确认 `cmd/captcha-server/main.go` 不直接引用 application 内部实现细节之外的对象。
4. 清理旧 service 包或保留薄兼容层。

验收：

- 依赖方向清晰。
- `internal/captcha/service` 不再作为事实上的上帝包。

## 11. 测试策略

### 11.1 单元测试

重点覆盖：

- `CaptchaUseCase`
  - 生成成功。
  - 图片池为空时 fallback。
  - Redis 保存答案失败。
  - 校验成功、坐标错误、答案不存在、Redis 错误。
  - 轨迹校验开关与失败 reason。
- `TokenUseCase`
  - 签发 token。
  - 格式错误、签名错误、payload 错误、过期、store missing。
- `ImageSourceUseCase`
  - patch 合并。
  - 参数校验。
  - validate 只校验不持久化。
  - update 持久化失败和刷新失败的错误分类。
- Redis adapters
  - key 格式。
  - JSON 兼容。
  - Redis nil 映射。
- External image adapter
  - 直接图片响应。
  - JSON URL。
  - base64/data URI。
  - nested payload。
  - 超大 body 限制。

### 11.2 集成测试

可使用真实 Redis 或 testcontainer 风格环境，验证：

- 生成验证码后 Redis 出现 `captcha:slide:{id}`。
- 校验后答案 key 被删除。
- token key 写入和查询正常。
- 图片池 refresh 会写入 active generation。
- 运行时图片源配置写入 `captcha:image-source:runtime-config` 并可恢复。

### 11.3 回归测试

每个阶段至少运行：

```bash
go test ./internal/captcha/... ./internal/shared/config/...
go build ./cmd/captcha-server
```

涉及 Swagger 或 API DTO 变更时，还需要重新生成并比对 `docs/swagger/captcha`。

## 12. 兼容性要求

重构期间默认保持以下内容不变：

- HTTP 路由。
- gRPC service 和方法。
- JSON 请求字段。
- JSON 响应字段。
- 错误 reason 字符串。
- Redis key。
- Redis JSON payload。
- 配置文件字段。
- 环境变量名，包括历史兼容别名。

任何不兼容调整都应单独开设计说明，不混在本次架构重构里。

## 13. 风险与处理方式

### 13.1 图片池链路风险最高

图片池涉及外部 API、Redis generation、分布式锁、定时刷新、运行时配置恢复。建议最后迁移，并用接口包一层后再移动实现。

### 13.2 并发安全风险

当前生成验证码时可能修改共享 `slideCaptcha`。重构时应让 generator 无共享状态，或在适配器中显式串行化共享对象访问。

### 13.3 错误映射漂移

HTTP/gRPC 对外依赖 reason 字符串。重构时应先建立错误映射表，避免在移动代码时顺手改掉错误语义。

### 13.4 配置映射遗漏

现有配置字段较多，尤其图片源和轨迹校验配置。建议配置映射配套单测，覆盖默认值和边界值。

### 13.5 一次性移动过多文件

不建议第一步就大规模改包路径。更稳妥的方式是先抽接口和 wrapper，让测试通过后再逐步移动实现。

## 14. 推荐落地顺序

优先级建议：

1. 固定测试和行为基线。
2. 抽入站接口，让 HTTP/gRPC 先面向用例。
3. 拆 token，因为最独立、最容易验证。
4. 拆 captcha 生成校验。
5. 拆图片池和运行时图片源。
6. 最后整理目录和启动 wiring。

这套顺序能最大限度减少一次性改动范围，也便于每一步都保持服务可运行。

## 15. 完成后的判断标准

完成重构后，应满足：

1. `domain` 和 `application` 不 import Gin、gRPC、Redis、`go-captcha`。
2. HTTP 和 gRPC 只依赖入站端口。
3. Redis、外部图片 API、验证码生成库只出现在 outbound adapter。
4. `cmd/captcha-server` 是唯一主要依赖组装点。
5. captcha、token、image-source 三组用例可用 mock port 做纯单元测试。
6. 原有 API、Redis key、配置字段保持兼容。
7. `go test ./internal/captcha/... ./internal/shared/config/...` 和 `go build ./cmd/captcha-server` 通过。
