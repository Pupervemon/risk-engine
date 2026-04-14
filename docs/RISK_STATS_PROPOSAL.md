# 风控服务统计方案（Redis 版）

版本: `draft-2026-04-14`

状态: `proposal`

## 1. 背景

当前仓库包含两个服务：

- `risk-server`
- `captcha-server`

目前已经有一部分 Redis 数据沉淀能力：

- `risk-server` 已经将风险 IP 洞察、事件历史、黑名单命中、登录失败计数写入 Redis
- `captcha-server` 已经将验证码答案、验证码 token 写入 Redis

但当前缺少面向运营和排障的数据统计能力，尤其是：

- 请求量、成功率、失败率
- 平均耗时
- 风控判定分布
- 验证码完成成功率
- 验证码失败原因分布
- 高频异常 IP / 用户分布

考虑到现阶段希望继续使用 Redis，不引入额外存储，本方案目标是基于现有架构补一层轻量级统计能力，并提供后台查询接口。

## 2. 当前代码现状

### 2.1 现有业务入口

`risk-server`

- HTTP
  - `GET /health`
  - `GET /info`
  - `GET /api/v1/admin/risk-ips`
  - `GET /api/v1/admin/risk-ips/{ip}`
  - `GET /api/v1/admin/risk-ips/{ip}/events`
- gRPC
  - `Check`
  - `ReportEvent`
  - `AddBlacklist`
  - `OnlineSelfTest`
  - `JudgeSubmission`

`captcha-server`

- HTTP
  - `GET /api/v1/captcha`
  - `POST /api/v1/captcha/verify`
  - `GET /health`
  - `GET /actuator/health`
  - `GET /actuator/health/liveness`
  - `GET /actuator/health/readiness`
- gRPC
  - `VerifyToken`

### 2.2 现有埋点条件

当前代码里已经有两个天然的埋点位置：

1. HTTP Router 中统一日志中间件
2. gRPC Unary Interceptor

这两个位置已经能拿到：

- 请求路径 / RPC 方法
- 返回状态
- 耗时
- 查询参数或请求上下文中的部分业务字段

此外，业务层也已经暴露了明确的结果状态：

- `RiskService.Check` 的 `action` / `reason`
- `RiskService.ReportEvent` 的成功失败
- `CaptchaService.VerifyWithTrack` 的验证结果与失败原因
- `TokenService.IssueToken` / `VerifyToken` 的结果

### 2.3 当前限制

当前默认配置里两个服务使用不同 Redis DB：

- `risk.dev.yaml` -> `redis.db = 1`
- `captcha.dev.yaml` -> `redis.db = 2`

这意味着如果后台统计接口由 `risk-server` 提供，那么必须先解决“跨服务统计数据放在哪个 Redis DB”这个问题。

## 3. 方案目标

### 3.1 目标

- 支持分钟 / 小时 / 天级别聚合
- 支持接口级统计和业务事件级统计
- 支持平均耗时、成功率、失败原因分布
- 支持验证码转化漏斗
- 支持风控决策分布
- 支持 Top N 异常主体查询
- 不落完整请求明细，仅落聚合数据

### 3.2 非目标

- 第一阶段不追求精确的全量明细审计
- 第一阶段不做复杂 OLAP 查询
- 第一阶段不做 Redis 之外的新基础设施
- 第一阶段不直接做可视化大盘，只提供查询接口

## 4. 推荐落地原则

### 4.1 统计数据单独存放

推荐新增一套独立统计 Redis 配置，而不是继续混用各自业务 Redis DB。

建议配置形态：

```yaml
stats_redis:
  addr: "127.0.0.1:6379"
  password: ""
  db: 3
  pool_size: 20
  dial_timeout_seconds: 5
  read_timeout_seconds: 3
  write_timeout_seconds: 3
```

原因：

- `risk` 和 `captcha` 目前 Redis DB 分离，统一查询不方便
- 统计数据和业务数据混放，后续 TTL、清理策略、容量评估会互相影响
- 独立 DB 更适合后续扩展成统一统计中心

如果短期不想改配置，也可以采用过渡方案：

- `risk-server` 继续读取 `db=1`
- `captcha-server` 继续写入 `db=2`
- 后台查询接口由单独的统计服务或 `risk-server` 双 Redis Client 聚合

但这只是过渡，不建议作为长期方案。

### 4.2 只做聚合，不做高基数原始明细

Redis 更适合做计数、桶聚合、TopN，不适合保存大量原始请求日志。

建议约束：

- 不按 `req_id`、`captcha_id` 做长期聚合 key
- 不按完整用户 ID 做全量维度扩散
- 用户维度仅在 TopN 或指定排查场景中使用，且建议脱敏 / 哈希
- reason、scene、api_name 都使用白名单枚举

## 5. 建议统计指标

### 5.1 通用接口指标

适用于 `risk-server` 和 `captcha-server`：

- 请求总量 `total`
- 成功量 `success`
- 失败量 `failure`
- 成功率 `success_rate`
- 平均耗时 `avg_latency_ms`
- 最大耗时 `max_latency_ms`
- 状态码分布
- 每分钟 / 每小时趋势

说明：

- `avg_latency_ms = latency_sum_ms / total`
- 第一阶段优先做平均值和最大值
- 如果后续需要 `p95/p99`，建议追加直方图桶，而不是直接做明细排序

### 5.2 风控业务指标

- `Check` 总调用量
- `Check` 决策分布：`pass / verify / reject`
- `Check` 原因分布：如 `BLACKLIST_IP`、`IP_RATE_LIMIT_EXCEEDED`、`TOO_MANY_FAILED_ATTEMPTS`
- `ReportEvent` 成功 / 失败次数
- 登录失败累计次数
- 黑名单新增次数
- 用户限流触发次数
- IP 限流触发次数

核心比率：

- 风控拦截率 = `reject / check_total`
- 二次验证率 = `verify / check_total`
- 黑名单命中率 = `blacklist_hit / check_total`

### 5.3 验证码业务指标

- 验证码生成次数
- 验证码校验请求次数
- 验证码校验成功次数
- 验证码校验失败次数
- 验证码完成成功率
- 验证码失败原因分布
- 鼠标轨迹失败原因分布
- token 签发次数
- token 校验成功 / 失败次数
- token 校验失败原因分布

核心比率：

- 验证码完成成功率 = `verify_success / verify_total`
- token 签发率 = `token_issue_success / verify_success`
- token 有效率 = `token_verify_success / token_verify_total`

## 6. Redis 数据模型

## 6.1 时间桶

建议同时维护三种粒度：

- 分钟桶：排障和实时看板，保留 7 天
- 小时桶：常规分析，保留 30 天
- 天桶：长期趋势，保留 180 天

桶时间示例：

- 分钟：`202604141630`
- 小时：`2026041416`
- 天：`20260414`

### 6.2 Key 设计

推荐统一前缀：

```text
stats:{service}:{granularity}:{bucket}:...
```

其中：

- `service` in `risk | captcha`
- `granularity` in `min | hour | day`

#### 1. 总览统计

```text
stats:{service}:{granularity}:{bucket}:overview
```

类型：`HASH`

字段建议：

- `total`
- `success`
- `failure`
- `client_error`
- `server_error`
- `latency_sum_ms`
- `latency_max_ms`

#### 2. 接口级统计

```text
stats:{service}:{granularity}:{bucket}:api:{api_name}
```

类型：`HASH`

字段建议：

- `total`
- `success`
- `failure`
- `latency_sum_ms`
- `latency_max_ms`

接口命名建议固定化，例如：

- `risk.grpc.Check`
- `risk.grpc.ReportEvent`
- `risk.grpc.AddBlacklist`
- `captcha.http.GET_/api/v1/captcha`
- `captcha.http.POST_/api/v1/captcha/verify`
- `captcha.grpc.VerifyToken`

#### 3. 业务结果分布

```text
stats:{service}:{granularity}:{bucket}:group:{group_name}
```

类型：`HASH`

示例：

- `stats:risk:min:202604141630:group:check_action`
- `stats:risk:min:202604141630:group:check_reason`
- `stats:captcha:min:202604141630:group:verify_reason`
- `stats:captcha:min:202604141630:group:token_verify_reason`

字段示例：

- `pass`
- `verify`
- `reject`
- `CAPTCHA_MISMATCH`
- `TRACK_TOO_FAST`
- `TOKEN_EXPIRED`

#### 4. 场景维度统计

```text
stats:risk:{granularity}:{bucket}:scene:{scene_name}
```

类型：`HASH`

字段建议：

- `total`
- `pass`
- `verify`
- `reject`
- `latency_sum_ms`

#### 5. TopN 统计

```text
stats:{service}:{granularity}:{bucket}:top:{dimension}
```

类型：`ZSET`

示例：

- `stats:risk:min:202604141630:top:ip_reject`
- `stats:risk:min:202604141630:top:user_verify`
- `stats:captcha:min:202604141630:top:ip_verify_fail`

成员值建议：

- IP 可以直接使用
- 用户 ID 建议脱敏或哈希后存储

### 6.3 TTL 建议

建议不同粒度使用不同 TTL：

- `min`：7 天
- `hour`：30 天
- `day`：180 天

### 6.4 写入方式

建议统一封装一个 `StatsRecorder`，内部使用 Redis Pipeline 批量写入：

- `HINCRBY`
- `HINCRBYFLOAT`
- `ZINCRBY`
- `EXPIRE`

好处：

- 业务代码只负责上报事件
- 避免每个接口手写 Redis key
- 后续更容易替换存储实现

## 7. 埋点位置建议

### 7.1 通用请求耗时埋点

建议位置：

- `internal/risk/transport/router.go`
- `internal/captcha/transport/http/router.go`
- `cmd/risk-server/main.go` 中的 gRPC Unary Interceptor
- `cmd/captcha-server/main.go` 中的 gRPC Unary Interceptor

这些位置负责记录：

- 请求总量
- 状态结果
- 平均耗时
- 接口级统计

### 7.2 风控业务埋点

建议在以下方法补业务维度统计：

- `internal/risk/service/risk.go` -> `Check`
  - 记录 `scene`
  - 记录 `action`
  - 记录 `reason`
- `internal/risk/service/risk.go` -> `ReportEvent`
  - 记录登录成功 / 失败
  - 记录失败累计趋势
- `internal/risk/service/risk.go` -> `AddBlacklist`
  - 记录黑名单新增次数
- `internal/risk/service/risk.go` -> `OnlineSelfTest`
  - 记录是否命中限流
- `internal/risk/service/risk.go` -> `JudgeSubmission`
  - 记录是否命中限流

### 7.3 验证码业务埋点

建议在以下方法补业务维度统计：

- `internal/captcha/service/captcha.go` -> `Generate`
  - 记录验证码生成次数
- `internal/captcha/service/captcha.go` -> `VerifyWithTrack`
  - 记录成功 / 失败
  - 记录失败原因
- `internal/captcha/service/token.go` -> `IssueToken`
  - 记录 token 签发次数
- `internal/captcha/service/token.go` -> `VerifyToken`
  - 记录 token 校验成功 / 失败和原因

## 8. 建议新增后台统计接口

建议统一挂在 `risk-server` 的管理接口下，风格和当前 `/api/v1/admin/risk-ips` 保持一致。

### 8.1 第一阶段建议做的 5 个接口

#### 1. 总览接口

```text
GET /api/v1/admin/stats/overview
```

用途：

- 看某段时间内总请求量、成功率、平均耗时

建议参数：

- `service`: `risk | captcha | all`
- `from`
- `to`
- `granularity`: `min | hour | day`

建议返回：

- `total`
- `success`
- `failure`
- `success_rate`
- `avg_latency_ms`
- `max_latency_ms`

#### 2. 趋势接口

```text
GET /api/v1/admin/stats/trend
```

用途：

- 看某个指标随时间变化

建议参数：

- `service`
- `metric`: `total | success_rate | avg_latency_ms | reject_rate | captcha_success_rate`
- `from`
- `to`
- `granularity`

建议返回：

- `points: [{bucket, value}]`

#### 3. 接口明细接口

```text
GET /api/v1/admin/stats/apis
```

用途：

- 按接口维度查看调用量、成功率、平均耗时

建议参数：

- `service`
- `from`
- `to`
- `granularity`
- `sort_by`: `total | avg_latency_ms | failure`

建议返回：

- `items: [{api_name, total, success_rate, avg_latency_ms, max_latency_ms}]`

#### 4. 原因分布接口

```text
GET /api/v1/admin/stats/reasons
```

用途：

- 看失败原因、风控原因、验证码失败原因分布

建议参数：

- `service`
- `group`: `check_reason | verify_reason | token_verify_reason`
- `from`
- `to`
- `granularity`

建议返回：

- `items: [{name, count, ratio}]`

#### 5. TopN 接口

```text
GET /api/v1/admin/stats/top
```

用途：

- 查看高频异常 IP / 用户

建议参数：

- `service`
- `dimension`: `ip_reject | user_verify | ip_verify_fail`
- `from`
- `to`
- `granularity`
- `limit`

建议返回：

- `items: [{key, count}]`

### 8.2 第二阶段建议追加的业务接口

#### 6. 风控决策接口

```text
GET /api/v1/admin/stats/risk/check-decisions
```

用途：

- 看 `Check` 的 `pass / verify / reject` 分布
- 可按 `scene` 过滤

#### 7. 验证码漏斗接口

```text
GET /api/v1/admin/stats/captcha/funnel
```

用途：

- 看验证码生成 -> 校验 -> token 签发的转化链路

建议返回：

- `captcha_generated`
- `captcha_verify_total`
- `captcha_verify_success`
- `token_issue_success`
- `verify_success_rate`
- `token_issue_rate`

#### 8. 验证码轨迹质量接口

```text
GET /api/v1/admin/stats/captcha/track-quality
```

用途：

- 看轨迹校验失败原因，如：
  - `TRACK_TOO_SHORT`
  - `TRACK_TOO_FAST`
  - `TRACK_INVALID_END`
  - `TRACK_DISCONTINUOUS`

## 9. 返回结构建议

建议与当前管理接口风格保持一致，统一使用：

```json
{
  "items": [],
  "offset": 0,
  "limit": 20,
  "total": 0,
  "has_more": false
}
```

对于非分页接口，建议使用：

```json
{
  "service": "captcha",
  "from": "2026-04-14T10:00:00Z",
  "to": "2026-04-14T11:00:00Z",
  "granularity": "min",
  "data": {}
}
```

## 10. 实施顺序

### 阶段 1：先打点，再查询

1. 增加统一 `StatsRecorder`
2. 在 HTTP middleware / gRPC interceptor 中记录通用请求统计
3. 在业务方法中记录风控决策、验证码结果、token 结果
4. 先提供 `overview / trend / apis / reasons / top` 五个接口

### 阶段 2：补业务聚合

1. 增加验证码漏斗查询
2. 增加风控决策查询
3. 增加轨迹质量查询

### 阶段 3：如果后续需要更精细分析

可选增强：

- 增加耗时直方图桶，支持近似 `p95/p99`
- 增加定时任务，将分钟桶汇总为小时桶 / 天桶
- 接 Prometheus / ClickHouse / OLAP 系统做长期分析

## 11. 风险与注意事项

### 11.1 Redis 内存增长

如果维度放开过多，Redis 内存会快速增长。必须限制：

- API 名称数量
- reason 枚举数量
- TopN 维度数量

### 11.2 高基数问题

以下维度不建议直接做长期聚合：

- `req_id`
- `captcha_id`
- 原始 `user_id`
- 原始 `phone_number`

### 11.3 统一查询问题

如果 `risk` 和 `captcha` 的统计仍然写在不同 DB，后台接口实现会变复杂。因此推荐尽早引入独立 `stats_redis`。

### 11.4 成功率口径要统一

例如验证码成功率需要明确定义：

- 是 `verify_success / verify_total`
- 还是 `verify_success / captcha_generated`

建议两个都保留，但默认展示前者。

## 12. 结论

基于当前代码结构，统计功能完全可以在现有仓库内增量实现，最合适的做法是：

1. 新增一层统一的 `StatsRecorder`
2. 将统计数据写入独立的 `stats_redis`
3. 在 HTTP / gRPC 中间层记录通用请求指标
4. 在 `risk` 和 `captcha` 业务方法中补业务结果埋点
5. 优先开放 5 个后台统计接口：
   - `GET /api/v1/admin/stats/overview`
   - `GET /api/v1/admin/stats/trend`
   - `GET /api/v1/admin/stats/apis`
   - `GET /api/v1/admin/stats/reasons`
   - `GET /api/v1/admin/stats/top`

如果按这个方案推进，第一阶段就已经可以覆盖你提到的两个核心需求：

- 请求平均时间
- 验证码完成成功率

同时还能顺手拿到：

- 风控拦截率
- 二次验证率
- 验证码失败原因分布
- token 校验失败原因分布
- 高频异常 IP / 用户分布
