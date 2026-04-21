# 验证码图片源热更新链路说明

版本: `2026-04-21`

状态: `current`

本文档专门解释 `captcha-server` 里“图片源热更新”这条链路。它的核心目标不是改验证码算法，而是让服务在不重启的情况下切换验证码背景图的上游来源，并把这份运行时配置持久化到 Redis，保证下次启动还能恢复。

如果你现在觉得配置类很多、名字很像、职责重叠，这个感觉是正常的。当前实现里，至少同时存在这几层“图片源配置”概念：

1. 启动配置
2. 运行时生效配置
3. Redis 持久化快照
4. 图片池当前使用的 provider

真正理解这套设计的关键，不是记住每个文件名，而是先把这四层区分开。

## 1. 一句话先讲清楚

现实现状可以概括成一句话：

`配置文件/环境变量` 只负责提供“服务启动时的默认图片源”，而 `RuntimeImageSourceManager` 才是“服务运行中真正生效的图片源状态”；当管理员调用热更新接口时，系统会先校验新配置，再写入 Redis，再切换内存态，最后按需刷新图片池。

也就是说：

- 启动时，先吃静态配置
- 运行后，以运行时 manager 为准
- 重启后，以 Redis 里保存的最后一次运行时配置为准

## 2. 先建立正确心智模型

### 2.1 分层视角

把当前实现拆开看，会更清楚：

| 层级 | 代表类型/对象 | 作用 | 生命周期 |
| --- | --- | --- | --- |
| 启动配置层 | `config.CaptchaConfig` / `config.CaptchaConfigSpec` / `config.ExternalImageAPIConfig` | 从 YAML 和环境变量加载配置，作为服务启动默认值 | 进程启动时确定 |
| 运行时配置层 | `service.ImageSourceRuntimeConfig` | 表示当前真正生效的图片源配置 | 运行期间可变 |
| 运行时管理层 | `service.RuntimeImageSourceManager` | 持有当前配置、provider、版本号、最近校验/刷新结果 | 整个进程生命周期 |
| 持久化层 | `service.ImageSourceStore` / Redis key `captcha:image-source:runtime-config` | 保存最后一次成功应用的运行时配置 | 跨进程、跨重启 |
| 图片池层 | `service.RedisImagePool` | 缓存实际图片数据，不直接保存“配置”，而是通过 provider 拉图 | 持续运行 |
| 上游拉图层 | `service.ExternalImageFetcher` | 真正去调用外部 API，解析 JSON/图片/base64，下载并规范化图片 | 每次刷新时工作 |
| 管理接口层 | `internal/captcha/transport/http/image_source_handler.go` | 暴露管理 API，接收管理员请求 | HTTP 请求期间 |

### 2.2 最容易混淆的地方

最常见的误区是把下面几个对象当成“同一个配置”：

- `cfg.Captcha.ExternalImageAPI`
- `ImageSourceRuntimeConfig`
- Redis 里的 `runtimeImageSourceStorePayload`
- `imagePool.provider`

它们不是同一个东西。

可以这样理解：

- `cfg.Captcha.ExternalImageAPI`：启动默认值
- `ImageSourceRuntimeConfig`：当前内存里真正生效的值
- Redis payload：为了重启恢复保存的快照
- `imagePool.provider`：图片池真正拉图时使用的 provider 引用

## 3. 关键对象职责速查

### 3.1 配置对象

#### `config.CaptchaConfig`

这是 captcha 服务的完整根配置，包含：

- `HTTP`
- `Grpc`
- `Redis`
- `Captcha`
- `Token`
- `Nacos`

这里只是配置树入口，不直接参与热更新逻辑。

#### `config.CaptchaConfigSpec`

这是验证码业务配置主体，包含：

- 验证码尺寸、容差、TTL
- `ImagePool`
- `TrackValidation`
- `ExternalImageAPI`

其中和图片源最相关的是：

- `Captcha.ImagePool`
- `Captcha.ExternalImageAPI`

#### `config.ExternalImageAPIConfig`

这个类型表示“静态启动配置里的上游图片 API 参数”，字段包括：

- `URL`
- `APIKey`
- `TimeoutSeconds`
- `RateLimitPerMinute`
- `RetryCount`

注意：它是启动配置类型，不是运行时热更新状态类型。

### 3.2 运行时对象

#### `service.ImageSourceRuntimeConfig`

这是热更新真正使用的配置对象。字段和静态配置很像，但职责不同：

- 它表示“当前生效值”
- 它由 `RuntimeImageSourceManager` 持有
- 它会被持久化到 Redis

#### `service.ImageSourcePatch`

这是热更新接口接收的局部补丁对象。字段是指针，目的是区分两种语义：

- `nil`：这个字段不改，沿用旧值
- 非 `nil`：这个字段用新值覆盖

所以更新接口支持“部分字段更新”，并不是每次都要把整份配置全传一遍。

#### `service.RuntimeImageSourceManager`

这是整条热更新链路最核心的对象。它负责：

- 保存当前运行时图片源配置
- 保存当前激活的 `ImageProvider`
- 维护版本号、更新时间
- 记录最近一次校验结果
- 记录最近一次刷新结果
- 向图片池暴露统一的 provider 能力

可以把它理解成“图片源运行时控制面”。

### 3.3 持久化对象

#### `service.ImageSourceStore`

这是抽象接口，只定义两件事：

- `Load`
- `Save`

它把“如何持久化”从运行时逻辑里抽掉。

#### `service.redisImageSourceStore`

当前唯一实现，底层用 Redis，固定 key 是：

`captcha:image-source:runtime-config`

这个 key 里存的是配置快照，不是图片内容。

### 3.4 图片池和拉图对象

#### `service.RedisImagePool`

它负责缓存图片数据本身，而不是配置。关键点：

- 刷新时会通过 provider 拉取一批图片
- 拉到后会整体替换 Redis 图片池内容
- `GetRandom` 只负责从池子里随机取一张图

所以验证码生成接口平时并不会直接请求外部图片 API，而是吃图片池。

#### `service.ExternalImageFetcher`

这个对象负责真正访问外部图片源。当前已经支持三种上游返回模式：

1. 直接返回图片二进制
2. JSON 里返回图片地址
3. JSON 里返回图片内容或 base64/data URI

也就是说，热更新切换的“图片源配置”，最终就是驱动这个 fetcher 去拉图。

## 4. 启动链路到底怎么走

启动链路可以按下面顺序理解。

### 4.1 加载静态配置

入口在 `cmd/captcha-server/main.go`。

服务启动时会调用：

- `config.LoadCaptchaConfigWithOptions`

配置来源优先级是：

1. CLI 参数
2. 环境变量
3. YAML 文件

和图片源相关的静态默认配置主要来自：

- `configs/captcha.dev.yaml`
- `configs/captcha.prod.yaml`
- 环境变量 `CAPTCHA_CAPTCHA_EXTERNAL_IMAGE_API_URL`
- 环境变量 `CAPTCHA_CAPTCHA_EXTERNAL_IMAGE_API_API_KEY`

此外还兼容旧别名：

- `CAPTCHA_EXTERNAL_IMAGE_API_URL`
- `CAPTCHA_EXTERNAL_IMAGE_API_API_KEY`

### 4.2 创建 `CaptchaService`

`NewCaptchaService` 会读取 `cfg.Captcha.ExternalImageAPI`，然后：

1. 构造 `ExternalImageAPIConfig`
2. 调用 `CustomImageFetcher`
3. 创建 `RedisImagePool`

这一步只是把“启动默认 provider”准备好。

注意此时还没有真正启用运行时 manager。

### 4.3 绑定运行时图片源 manager

紧接着，`main.go` 会调用：

- `captchaService.EnableRuntimeImageSourceManager()`

这是热更新体系真正被接入的关键点。

它做了几件事：

1. 从 `cfg.ExternalImageAPI` 构造初始的 `ImageSourceRuntimeConfig`
2. 创建 `RuntimeImageSourceManager`
3. 创建 Redis 持久化 store
4. 尝试从 Redis 恢复历史运行时配置
5. 把 `imagePool.provider` 切换成这个 manager

这里最重要的是最后一步：

原来图片池可能直接持有某个固定 `ExternalImageFetcher`，启用 runtime manager 后，图片池持有的是 `RuntimeImageSourceManager`。之后无论是定时刷新还是手动刷新，都会通过 manager 找到当前激活的 provider。

### 4.4 启动时恢复 Redis 中的历史配置

`EnableRuntimeImageSourceManager()` 里会调用 `restoreRuntimeImageSource()`。

恢复逻辑是：

1. 从 Redis key `captcha:image-source:runtime-config` 读取快照
2. 如果没读到，继续使用配置文件里的默认值
3. 如果读到了，就尝试用它构造 provider
4. 如果 provider 构造失败，说明 Redis 里的历史值已经脏了或不合法，放弃恢复，继续用文件配置
5. 如果构造成功，就调用 `manager.RestoreConfig(...)`

这里还有两个关键细节：

- 恢复超时是 `3s`
- 恢复不会递增版本号

也就是说，恢复被视为“把上次状态找回来”，而不是“新发生了一次线上变更”。

### 4.5 启动定时刷新任务

如果 `cfg.Captcha.ImagePool.Enabled = true`，`main.go` 会继续调用：

- `captchaService.StartImageRefresh(context.Background())`

这一步会：

1. 先执行一次 `RefreshNow`
2. 再按 `refresh_interval_minutes` 启动定时刷新

而 `RefreshNow` 在 runtime manager 已启用的情况下，最终用的是 manager 当前持有的 provider。

所以整个启动链路可以总结成：

`静态配置 -> 创建默认图片源 -> 启用 runtime manager -> 从 Redis 恢复覆盖 -> 启动图片池刷新`

## 5. 验证码生成时到底会不会直接请求上游图片 API

不会，正常生成验证码时走的是图片池，不是实时拉上游。

`CaptchaService.Generate()` 的逻辑是：

1. 如果启用了图片池，就先从 Redis 图片池随机取一张背景图
2. 用这张背景图重建 `slideCaptcha`
3. 生成滑块验证码
4. 把答案写入 Redis

也就是说：

- 热更新改的是“以后刷新图片池时从哪里拉图”
- 不是“每次 `/api/v1/captcha` 请求时都临时去上游拉一张图”

这点非常关键，因为它解释了为什么“更新配置”和“刷新图片池”被设计成两个步骤。

## 6. 热更新链路详细拆解

### 6.1 管理接口入口

当前图片源管理接口有 4 个：

- `GET /api/v1/admin/image-source`
- `POST /api/v1/admin/image-source/validate`
- `PUT /api/v1/admin/image-source`
- `POST /api/v1/admin/image-source/refresh`

它们都挂在：

- `/api/v1/admin`

### 6.2 管理接口鉴权

鉴权不再解析 token，本地只信任网关注入的请求头。

当前读取的头是：

- `X-User-Id`
- `X-User-Roles`

管理员角色要求是：

- `3`

`X-User-Roles` 支持多种格式：

- `3`
- `1,3`
- `[3]`
- `["3","5"]`

如果请求头缺失、格式非法或不含角色 `3`，接口会直接拒绝。

这里要特别强调一遍：

`TokenConfig.Secret` 是验证码验证成功后签发 captcha token 用的，和管理员接口鉴权不是一回事。管理员接口现在走的是网关头透传，不靠本服务自己验 token。

### 6.3 查询当前状态

`GET /api/v1/admin/image-source` 会调用：

- `CaptchaService.GetImageSourceStatus`

返回内容包括：

- 是否启用
- 当前版本号
- 当前配置视图
- 最近一次校验时间/错误
- 最近一次刷新时间/错误
- 图片池容量
- 图片池实际图片数

这相当于运行时控制面的状态面板。

### 6.4 预校验链路

`POST /api/v1/admin/image-source/validate` 会调用：

- `CaptchaService.ValidateImageSource`

内部步骤如下：

1. 从 `ImageSourcePatch` 构造候选配置
2. 合并当前旧配置和补丁
3. 做规范化
4. 做基础合法性校验
5. 构造 provider
6. 真实拉取 1 张图片做验证

第 6 步非常重要。当前不是只检查 URL 格式，而是会真正走一次上游请求，所以它验证的是“可用性”，不是单纯“参数长得像是对的”。

### 6.5 更新链路

`PUT /api/v1/admin/image-source` 是真正的热更新入口。

请求里支持一个字段：

- `triggerRefresh`

默认是 `true`。

完整链路如下：

1. handler 解析 JSON
2. 转成 `ImageSourcePatch`
3. `BuildCandidateConfig`
4. `ValidateConfig`
5. `persistRuntimeImageSourceConfig`
6. `manager.ApplyConfig`
7. 如果 `triggerRefresh=true`，执行 `imagePool.RefreshWithProvider`
8. 记录刷新结果并返回最新状态

请注意这里的执行顺序：

`先校验 -> 先持久化 -> 再切换内存 -> 最后刷新图片池`

这个顺序的含义是：

- 只有校验通过的配置，才会进入 Redis
- 只有持久化成功的配置，才会被应用成当前内存态
- 图片池刷新只是“让新配置尽快影响图片池内容”，不是配置切换本身

### 6.6 如果刷新失败，会发生什么

这是非常重要的实现细节。

如果更新接口里：

- 校验通过了
- Redis 持久化也成功了
- `manager.ApplyConfig` 也执行了
- 但是后面的 `RefreshWithProvider` 失败了

那么结果是：

- 新配置已经生效
- 新配置已经落到 Redis
- 只是图片池这次刷新失败了

接口层会把这个错误包装成 `ImageSourceRefreshError`，HTTP 返回 `502`，同时把当前状态一并返回。

换句话说，这不是“整次更新回滚失败”，而是“配置已经切了，但池子还没刷新成功”。

这点运维上一定要知道。

### 6.7 手动刷新链路

`POST /api/v1/admin/image-source/refresh` 不改配置，只做刷新。

内部调用：

- `CaptchaService.RefreshImagePool`
- `imagePool.RefreshNow`

它会使用当前激活的 provider 去重刷图片池。

所以它的语义是：

- “重新按当前配置拉一批图”
- 不是“重新设置配置”

## 7. 图片池刷新链路详细说明

图片池刷新分两种触发方式：

1. 启动后和定时任务自动刷新
2. 管理接口手动刷新

底层都汇总到：

- `RedisImagePool.RefreshWithProvider`

### 7.1 刷新时做了什么

刷新步骤如下：

1. 加锁 `refreshMu`
2. 用 provider 拉取 `poolSize` 张图
3. 调用 `LoadImages`
4. 删除旧池内容
5. 写入新图片数据
6. 重建图片 ID 索引集合

这意味着当前实现是“整池替换”，不是增量更新。

### 7.2 为什么要有 `refreshMu`

因为可能同时存在：

- 定时刷新
- 管理员手动刷新
- 管理员更新配置后顺手刷新

`refreshMu` 的作用是防止这些刷新并发执行，避免互相覆盖和状态错乱。

## 8. Redis 恢复链路详细说明

热更新之所以叫“热更新”，不只是运行时能切，更关键是重启后不能丢。

当前恢复链路是：

1. 服务启动
2. 创建 runtime manager
3. 读取 Redis 中的 `captcha:image-source:runtime-config`
4. 尝试用该配置构造 provider
5. 构造成功则恢复，失败则忽略

### 8.1 Redis 里到底存了什么

当前 Redis 里存的是一个 JSON，大致字段包括：

- `url`
- `apiKey`
- `timeoutSeconds`
- `rateLimitPerMinute`
- `retryCount`
- `updatedAt`

它不是运行时完整状态快照，不包含：

- 当前版本号
- 最近一次校验错误
- 最近一次刷新错误
- 图片池内容

它只保存“下次重启恢复图片源所需的最小配置”。

### 8.2 为什么恢复失败时不报 fatal

因为当前设计把“Redis 历史值失效”视为可降级问题。

如果 Redis 恢复失败：

- 服务仍然能启动
- 回退到配置文件里的静态默认图片源

这是一个偏可用性优先的策略。

## 9. 外部图片获取链路详细说明

你之前提到的核心问题是：

“现在不是所有上游都直接在 JSON 里给图片内容，很多时候给的是图片地址。”

当前实现已经按这个方向做了兼容，完整链路如下。

### 9.1 第一次请求上游 API

`ExternalImageFetcher.fetchUpstreamPayload()` 会先请求配置里的：

- `ExternalImageAPIConfig.URL`

接受的响应有两类：

1. 直接就是图片
2. 是 JSON 或文本包装

### 9.2 如果响应直接是图片

会直接把响应体当图片内容处理。

判断依据包括：

- `Content-Type` 是 `image/*`
- 或响应体本身可被识别为图片二进制

### 9.3 如果响应是 JSON

会进入 `parseImageAPIResponse()`。

当前支持从 JSON 里提取：

- 图片 URL
- 内联图片内容
- base64
- data URI

并且支持常见字段名候选，比如：

- `imgurl`
- `imgUrl`
- `imageUrl`
- `image_url`
- `url`
- `src`
- `image`
- `img`
- `base64`
- `data`

还支持从常见嵌套字段里递归往下找：

- `data`
- `result`
- `payload`
- `body`
- `response`

### 9.4 如果 JSON 里拿到的是图片 URL

会继续调用：

- `downloadImage()`

也就是发第二次请求，真正去下载图片。

默认只有当下载地址和 API 同源时，才会自动复用 `Authorization: Bearer <APIKey>` 头；如果是外链图片，默认不会把 API key 强行带过去。

这个设计是对的，因为它避免把上游 API key 泄露给第三方图片域名。

### 9.5 图片下载后会怎么处理

下载到的图片会进入：

- `processImage()`

处理流程是：

1. 解码成图片对象
2. 按目标尺寸缩放和居中裁剪
3. 统一转成 PNG

所以最终图片池里保存的内容是规范化后的 PNG，不依赖上游原格式。

## 10. 配置类为什么会让人混乱

你的困惑本质上来自两个原因。

### 10.1 同一概念在不同阶段用了不同类型

例如“图片源配置”至少出现为：

- `config.ExternalImageAPIConfig`
- `service.ExternalImageAPIConfig`
- `service.ImageSourceRuntimeConfig`
- Redis payload

这里面字段相似，但职责不同，所以阅读时很容易误以为只是“重复定义”。

实际上它们分别服务于：

- 配置加载
- fetcher 调用
- 运行时管理
- 持久化存储

### 10.2 运行时状态和启动配置混在同一业务里

`CaptchaService` 既持有：

- 启动配置 `cfg`
- 图片池 `imagePool`
- 运行时 manager 绑定

所以如果只看 `CaptchaService`，会觉得所有东西都揉在一起。

正确的理解方式应该是：

- `cfg` 决定启动默认值
- `runtime manager` 决定当前生效值
- `imagePool` 决定当前可用图片缓存

## 11. 建议你以后按这个顺序看配置

以后再看 captcha 配置，不要从文件名开始看，建议按下面顺序：

1. 先看 `configs/captcha.dev.yaml` 或 `configs/captcha.prod.yaml`
2. 再看 `internal/shared/config/captcha_config.go`
3. 再看 `internal/shared/config/strict_validation.go`
4. 再看 `cmd/captcha-server/main.go`
5. 最后看 `internal/captcha/service` 里的 runtime manager / image pool

这样会形成下面这条脑回路：

- YAML/ENV 定义了什么
- 结构体怎么接
- 启动时校验什么
- 服务启动时怎么把配置接进业务
- 运行中又怎么被热更新改掉

## 12. 当前配置速查表

下面这张表专门用来解决“这个配置到底什么时候生效”的问题。

| 名称 | 所在位置 | 什么时候决定 | 什么时候可能变化 | 影响什么 |
| --- | --- | --- | --- | --- |
| `captcha.external_image_api.*` | YAML / ENV | 启动时 | 运行中可被热更新覆盖 | 默认图片源 |
| `captcha.image_pool.enabled` | YAML / ENV | 启动时 | 运行中不变 | 是否启用图片池和热更新体系 |
| `captcha.image_pool.pool_size` | YAML / ENV | 启动时 | 运行中不变 | 每次刷新拉多少张图 |
| `captcha.image_pool.refresh_interval_minutes` | YAML / ENV | 启动时 | 运行中不变 | 定时刷新周期 |
| `ImageSourceRuntimeConfig` | 内存 | 启动后初始化 | 管理接口可变 | 当前真正生效的图片源 |
| Redis key `captcha:image-source:runtime-config` | Redis | 每次更新时写入 | 管理接口可变 | 重启恢复 |
| 图片池数据 `captcha:images:*` / `captcha:images:index` | Redis | 刷新时写入 | 定时或手动刷新可变 | `/api/v1/captcha` 取图 |

## 13. 代码导航索引

如果你要回到代码里追链路，建议直接按下面这张表跳转：

| 文件 | 你应该重点看什么 |
| --- | --- |
| `cmd/captcha-server/main.go` | 服务启动顺序，runtime manager 绑定点，路由挂载点 |
| `internal/shared/config/captcha_config.go` | captcha 静态配置结构体定义 |
| `internal/shared/config/captcha_loader.go` | captcha 配置加载入口 |
| `internal/shared/config/loader.go` | 环境变量绑定规则、旧别名兼容 |
| `internal/shared/config/strict_validation.go` | 启动严格校验规则 |
| `internal/captcha/service/captcha.go` | `CaptchaService` 初始化、生成验证码、启动刷新任务 |
| `internal/captcha/service/captcha_runtime_image_source.go` | runtime manager 启用与 Redis 恢复入口 |
| `internal/captcha/service/image_source_manager.go` | 运行时图片源状态机、候选配置构造、校验、应用 |
| `internal/captcha/service/image_source_runtime.go` | 查询、校验、更新、手动刷新这几个 service API |
| `internal/captcha/service/image_source_store.go` | 运行时配置写入/读取 Redis 的实现 |
| `internal/captcha/service/image_provider.go` | 图片池刷新、整池替换、并发控制 |
| `internal/captcha/service/image_fetcher.go` | 上游图片拉取、JSON 解析、图片地址下载、图片规范化 |
| `internal/captcha/transport/http/image_source_handler.go` | 管理接口的 HTTP 层 |
| `internal/captcha/transport/http/admin_auth.go` | 网关头鉴权逻辑 |
| `internal/captcha/transport/http/router.go` | `/api/v1` 和 `/api/v1/admin` 路由挂载 |

## 14. 环境变量到底该看哪个

这也是当前最容易让人混乱的一点。

### 14.1 推荐使用的规范名字

captcha 服务现在推荐使用带服务前缀、并按配置路径展开的环境变量，例如：

- `CAPTCHA_HTTP_PORT`
- `CAPTCHA_GRPC_PORT`
- `CAPTCHA_REDIS_ADDR`
- `CAPTCHA_TOKEN_SECRET`
- `CAPTCHA_CAPTCHA_EXTERNAL_IMAGE_API_URL`
- `CAPTCHA_CAPTCHA_EXTERNAL_IMAGE_API_API_KEY`

图片源最关键的是后两个。

### 14.2 为什么看起来名字很怪

因为环境变量名是按完整配置路径拼出来的：

- 服务前缀：`CAPTCHA`
- 配置路径：`captcha.external_image_api.url`

所以最终会变成：

`CAPTCHA_CAPTCHA_EXTERNAL_IMAGE_API_URL`

这不是写错了，而是当前 loader 的命名规则就是这样。

### 14.3 为什么你还会在代码里看到另一套旧名字

因为 `internal/shared/config/loader.go` 里还保留了兼容别名：

- `CAPTCHA_EXTERNAL_IMAGE_API_URL`
- `CAPTCHA_EXTERNAL_IMAGE_API_API_KEY`
- `TOKEN_SECRET`

这些旧名字还能用，但应该只当迁移兼容，不建议继续扩散。

## 15. 典型请求示例

### 13.1 查询当前状态

```bash
curl -X GET "http://localhost:8091/api/v1/admin/image-source" ^
  -H "X-User-Id: 1" ^
  -H "X-User-Roles: 3"
```

### 13.2 先做预校验

```bash
curl -X POST "http://localhost:8091/api/v1/admin/image-source/validate" ^
  -H "Content-Type: application/json" ^
  -H "X-User-Id: 1" ^
  -H "X-User-Roles: 3" ^
  -d "{\"url\":\"https://example.com/api/random-image\",\"timeoutSeconds\":30,\"rateLimitPerMinute\":60,\"retryCount\":2}"
```

### 13.3 更新并立即刷新

```bash
curl -X PUT "http://localhost:8091/api/v1/admin/image-source" ^
  -H "Content-Type: application/json" ^
  -H "X-User-Id: 1" ^
  -H "X-User-Roles: 3" ^
  -d "{\"url\":\"https://example.com/api/random-image\",\"apiKey\":\"xxx\",\"timeoutSeconds\":30,\"rateLimitPerMinute\":60,\"retryCount\":2,\"triggerRefresh\":true}"
```

### 13.4 只改配置，不立刻刷新

```bash
curl -X PUT "http://localhost:8091/api/v1/admin/image-source" ^
  -H "Content-Type: application/json" ^
  -H "X-User-Id: 1" ^
  -H "X-User-Roles: 3" ^
  -d "{\"url\":\"https://example.com/api/random-image\",\"triggerRefresh\":false}"
```

### 13.5 用当前配置手动刷新

```bash
curl -X POST "http://localhost:8091/api/v1/admin/image-source/refresh" ^
  -H "X-User-Id: 1" ^
  -H "X-User-Roles: 3"
```

## 16. 常见误区和排障点

### 14.1 为什么改了 YAML，运行中的服务没变

因为 YAML 只在启动时加载。服务跑起来后，真正生效的是 runtime manager 里的配置。

### 14.2 为什么重启后不是我 YAML 里的值

因为 Redis 里保存了历史运行时配置，启动时恢复逻辑会覆盖静态默认值。

### 14.3 为什么更新接口报错，但状态看起来又像改成功了

很可能是：

- 配置已经保存
- manager 已切换
- 但图片池刷新失败

这时会返回 `ImageSourceRefreshError` 对应的 `502`，但状态里显示的已经是新配置。

### 14.4 为什么管理员接口和 `token.secret` 没关系

因为管理员接口不再本地解析 token，而是只看网关透传的：

- `X-User-Id`
- `X-User-Roles`

### 14.5 为什么图片池关闭后管理接口不可用

因为当前实现把 runtime image source manager 绑定在图片池启用路径上。

也就是说：

- `image_pool.enabled = false`
- 就不会启用 runtime manager
- 相关管理接口会返回 `IMAGE_POOL_DISABLED`

## 17. 对当前实现的评价

### 15.1 已有优点

当前实现有几个明显优点：

- 热更新链路清晰，分成校验、持久化、应用、刷新四步
- Redis 恢复逻辑让运行时配置具备重启保留能力
- 图片池和运行时配置分层明确，生成验证码时不直接依赖上游接口
- 上游图片解析兼容性比较强，已经支持 JSON 返回图片地址这类主流模式
- 管理接口把配置错误、持久化错误、刷新错误区分得比较清楚

### 15.2 当前还不够完善的点

下面这些点，从当前代码看，后续很值得继续补强。

#### 1. 多实例实时一致性还不够

从当前实现看，Redis 只承担“启动恢复”的作用，不承担“运行中实例之间实时同步”的作用。

这意味着如果你部署多个 captcha 实例：

- A 实例更新了图片源
- A 会把配置写进 Redis
- 但 B、C 实例不会立刻自动切换

它们通常要等到：

- 自己重启后恢复
- 或者后续你主动实现一个同步机制

这是当前最重要的架构级缺口。

#### 2. 缺少“回滚/恢复默认配置”接口

现在有更新、校验、刷新，但没有显式的：

- 清空运行时覆盖
- 回退到 YAML/ENV 默认值

一旦线上配了一个不理想但还能工作的地址，目前只能再发一次更新把值改回去，不够直接。

#### 3. 持久化内容偏少

Redis 里只保存了最小配置，没有保存：

- 版本号
- 操作人
- 变更来源
- 变更时间的更完整审计信息

如果后面要做运维审计，这部分信息不够。

#### 4. 更新成功但刷新失败时没有自动回滚

当前策略偏向“配置先落地，刷新失败另算”。这个选择可用性不错，但如果你希望“图片池没刷成功就不算切换成功”，那还需要补一套回滚策略。

#### 5. 缺少运行时配置变更广播

当前没有：

- Redis Pub/Sub
- Nacos 配置推送
- 本地 watch/reconcile

所以变更只影响当前实例，不能天然广播到全部副本。

## 18. 我建议的落地演进方案

如果你要继续把这套能力做扎实，我建议按下面顺序推进。

### 第一阶段：先把控制面补完整

建议新增两个接口：

1. “恢复默认图片源”
2. “删除 Redis 中的运行时覆盖配置”

这样可以把“热更新”从单向写入，变成可回退、可清理。

### 第二阶段：把运行时变更做成多实例一致

建议二选一：

1. Redis Pub/Sub 广播配置变更
2. 每个实例定时从 Redis 拉取 runtime config 并比对版本

如果实例数量不大，第二种更容易落地。

### 第三阶段：把审计信息补齐

建议 Redis payload 增加：

- `version`
- `operatorUserId`
- `source`
- `updatedAt`

这样状态页和排障都会轻松很多。

### 第四阶段：补一个更明确的状态机

把当前图片源状态区分成：

- `validated`
- `persisted`
- `applied`
- `pool_refreshed`

现在虽然逻辑上已经有这几个阶段，但对外状态里没有完整表达。

## 19. 最后的结论

当前验证码热更新的主链路已经比较完整，真正要记住的只有三句话：

1. `captcha.external_image_api` 是启动默认值，不是运行时最终值
2. `RuntimeImageSourceManager` 才是运行中真正生效的图片源控制中心
3. Redis 里的 `captcha:image-source:runtime-config` 只负责重启恢复，不负责多实例实时同步

只要你把“静态配置、运行时状态、持久化快照、图片池数据”这四层分开，整个 captcha 图片源热更新链路就不会再乱。
