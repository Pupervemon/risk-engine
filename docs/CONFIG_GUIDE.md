# Risk Engine 配置契约

版本: `draft-v1`

状态: `当前实现基线`

本文档用于描述 `risk-engine` 当前已经落地的配置入口、命名方式、校验规则和后续收敛方向。出现文档与代码冲突时，以代码中的实际运行行为为准，但新增改动应尽量向本文档靠拢。

## 1. 目标

- 为 `risk-server` 和 `captcha-server` 提供统一的配置入口模型
- 明确配置来源和覆盖顺序，避免“猜路径”或隐式行为
- 让生产环境在启动阶段完成严格校验，而不是运行中才暴露问题
- 保持本地开发简单，支持 `.env`、YAML、CLI 参数协同工作
- 让 README、配置模板、Web Demo 和服务实现遵循同一套契约

## 2. 当前实现概览

当前配置加载入口位于 `internal/shared/config`，整体形态是：

- 通用加载器: `loader.go`
- 服务专属加载入口: `captcha_loader.go`、`risk_config.go`
- 共享结构体: `shared_config.go`
- 严格校验: `strict_validation.go`、`validate_helpers.go`
- 脱敏摘要输出: `captcha_config_methods.go`、`risk_config_methods.go`

当前仓库仍处于兼容迁移阶段，部分旧环境变量别名仍被支持，但新约定应优先使用服务前缀。

## 3. 配置来源与优先级

当前目标优先级从高到低如下：

1. CLI 参数
2. 环境变量
3. YAML 配置文件

具体规则：

- `--config` 优先于环境变量中的配置文件路径
- `--env` 优先于 `RISK_APP_ENV` / `CAPTCHA_APP_ENV`，再优先于全局 `APP_ENV`
- `RISK_CONFIG_FILE` / `CAPTCHA_CONFIG_FILE` 优先于全局 `CONFIG_FILE`
- 如果没有显式配置文件，则按服务名和环境名在 `configs/` 下寻找默认文件

当前默认搜索路径仍包含 `./configs`、`../configs` 和当前目录，这是过渡实现，不是最终理想态。

## 4. 启动契约

推荐启动方式：

```bash
go run ./cmd/risk-server --config ./configs/risk.dev.yaml
go run ./cmd/captcha-server --config ./configs/captcha.dev.yaml
```

也支持按环境名选择配置：

```bash
go run ./cmd/risk-server --env dev
go run ./cmd/captcha-server --env dev
```

启动时服务会输出脱敏后的配置摘要，包括端口、Redis 地址、Nacos 开关和部分业务配置。

## 5. 环境变量命名规则

### 5.1 服务前缀

- 风控服务使用前缀 `RISK_`
- 验证码服务使用前缀 `CAPTCHA_`

### 5.2 命名方式

环境变量名称由配置路径转换为大写蛇形，例如：

- `RISK_HTTP_PORT`
- `RISK_GRPC_PORT`
- `RISK_REDIS_ADDR`
- `CAPTCHA_TOKEN_SECRET`
- `CAPTCHA_CAPTCHA_EXTERNAL_IMAGE_API_URL`

### 5.3 当前兼容别名

为了平滑迁移，当前实现仍兼容以下无前缀变量：

- `REDIS_ADDR`
- `REDIS_PASSWORD`
- `REDIS_DB`
- `REDIS_POOL_SIZE`
- `REDIS_DIAL_TIMEOUT_SECONDS`
- `REDIS_READ_TIMEOUT_SECONDS`
- `REDIS_WRITE_TIMEOUT_SECONDS`
- `NACOS_ENABLE`
- `NACOS_SERVER_ADDR`
- `NACOS_NAMESPACE`
- `NACOS_SERVICE_NAME`
- `NACOS_GROUP_NAME`
- `NACOS_CLUSTER_NAME`
- `NACOS_REGISTER_IP`
- `NACOS_WEIGHT`
- `TOKEN_SECRET` 仅用于 `captcha`

这些别名是兼容能力，不应再作为新的配置约定继续扩散。

## 6. 当前 YAML 模型

### 6.1 通用顶层结构

两个服务当前共享以下顶层结构：

```yaml
http:
  port: 0

grpc:
  port: 0

redis:
  addr: ""
  password: ""
  db: 0
  pool_size: 0
  dial_timeout_seconds: 0
  read_timeout_seconds: 0
  write_timeout_seconds: 0

nacos:
  enable: false
  server_addr: ""
  namespace: ""
  service_name: ""
  group_name: ""
  cluster_name: ""
  register_ip: ""
  weight: 1.0
  metadata: {}
```

### 6.2 风控服务专属结构

```yaml
risk_rules:
  login:
    max_fail_count: 0
    fail_count_expire_minutes: 0
  ip_rate_limit:
    limit: 0
    window_seconds: 0
  user_rate_limit:
    online_self_test:
      limit: 0
      window_seconds: 0
    judge_submission:
      limit: 0
      window_seconds: 0
```

### 6.3 验证码服务专属结构

```yaml
captcha:
  ttl_seconds: 0
  width: 0
  height: 0
  graph_size_min: 0
  graph_size_max: 0
  slider_tolerance: 0
  image_pool:
    enabled: false
    pool_size: 0
    refresh_interval_minutes: 0
  track_validation:
    enabled: false
    min_points: 0
    min_duration_ms: 0
    max_duration_ms: 0
    point_tolerance: 0
  external_image_api:
    url: ""
    api_key: ""
    timeout_seconds: 0
    rate_limit_per_minute: 0
    retry_count: 0

token:
  ttl_seconds: 0
  secret: ""
```

## 7. 严格校验规则

### 7.1 通用校验

所有服务都会校验：

- `http.port > 0`
- `grpc.port > 0`
- `redis.addr != ""`
- `redis.db >= 0`
- `redis.pool_size > 0`
- Redis 各类 timeout 必须大于 0

### 7.2 Nacos 校验

当 `nacos.enable = true` 时：

- `server_addr` 必填
- `service_name` 必填
- `group_name` 必填
- `cluster_name` 必填
- `namespace` 可以为空，空值表示使用 Nacos 默认命名空间
- `register_ip` 如果设置，必须是合法 IPv4

### 7.3 风控服务校验

- `risk_rules.login.max_fail_count > 0`
- `risk_rules.login.fail_count_expire_minutes > 0`
- `risk_rules.ip_rate_limit.limit > 0`
- `risk_rules.ip_rate_limit.window_seconds > 0`
- `risk_rules.user_rate_limit.*.limit > 0`
- `risk_rules.user_rate_limit.*.window_seconds > 0`

### 7.4 验证码服务校验

- `captcha.ttl_seconds > 0`
- `captcha.width > 0`
- `captcha.height > 0`
- `captcha.graph_size_min > 0`
- `captcha.graph_size_max >= captcha.graph_size_min`
- `captcha.slider_tolerance > 0`
- `token.ttl_seconds > 0`

当 `captcha.image_pool.enabled = true` 时，还会校验：

- `image_pool.pool_size > 0`
- `image_pool.refresh_interval_minutes > 0`
- `external_image_api.url != ""`
- `external_image_api.timeout_seconds > 0`
- `external_image_api.rate_limit_per_minute > 0`
- `external_image_api.retry_count >= 0`

当 `captcha.track_validation.enabled = true` 时，还会校验：

- `min_points > 0`
- `min_duration_ms > 0`
- `max_duration_ms >= min_duration_ms`
- `point_tolerance > 0`

### 7.5 生产环境额外约束

在 `prod` 环境下：

- `redis.password` 不能为空
- `captcha` 服务的 `token.secret` 不能为空且不能是占位值
- 如果启用 Nacos，`namespace` 可以留空以使用默认命名空间；只有显式填写时才会使用自定义命名空间

## 8. 敏感信息策略

以下字段视为敏感信息：

- `redis.password`
- `token.secret`
- `captcha.external_image_api.api_key`

策略要求：

- 样例文件可以留空，但不应提供可直接使用的默认密钥
- 生产环境应通过环境变量或 Secret 管理系统注入
- 启动摘要必须脱敏输出，不能打印明文

## 9. Nacos 元数据约定

当前实现中，注册中心会统一写入端口相关元数据：

- `http-port`
- `grpc-port`

当前配置加载阶段还保留了兼容性字段 `gRPC_port`，这是迁移中的历史包袱，后续应统一收敛为短横线风格。

## 10. Demo 与文档约定

`README.md`、`web-test/README.md` 和 `web-test/index.html` 都属于配置契约的消费端，必须满足：

- 不再引用仓库内不存在的启动脚本
- 示例命令与真实入口保持一致
- 默认端口、接口路径、配置文件名与当前实现一致
- Web Demo 可以通过显式输入或查询参数覆盖默认 API 地址

## 11. 后续收敛方向

当前已经完成统一加载器和严格校验，但还存在几个未完全收口的点：

- 默认值仍有部分散落在业务代码中，而不是完全收敛到配置层
- 默认配置文件搜索路径仍偏宽松
- 旧环境变量别名仍然存在
- Nacos metadata 命名尚未完全统一

建议后续按以下顺序继续推进：

1. 统一默认值来源
2. 收紧配置文件发现策略
3. 清理历史环境变量别名
4. 统一 Nacos metadata 命名
5. 为配置层补自动化测试
