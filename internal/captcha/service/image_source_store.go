package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// runtimeImageSourceStoreKey 是运行时图片源配置在 Redis 中的固定存储键。
//
// 这个 key 只保存“当前生效的运行时配置快照”，不是图片内容本身。
const runtimeImageSourceStoreKey = "captcha:image-source:runtime-config"

// ImageSourceStore 定义运行时图片源配置的持久化接口。
//
// 这个接口把“如何保存/读取配置”从业务逻辑里抽出来，
// 这样上层 manager 只需要关心配置本身，不需要知道后端是 Redis 还是别的存储。
type ImageSourceStore interface {
	// Load 读取当前保存的运行时配置。
	//
	// 返回值含义：
	// - cfg: 读到的配置内容；
	// - bool: 是否真的存在有效记录；
	// - error: 读取或解码过程中的异常。
	Load(ctx context.Context) (ImageSourceRuntimeConfig, bool, error)
	// Save 保存一份新的运行时配置。
	//
	// 一般在运行时切换图片源配置成功后调用，用于让后续启动的实例恢复同样的状态。
	Save(ctx context.Context, cfg ImageSourceRuntimeConfig) error
}

// redisImageSourceStore 是 ImageSourceStore 的 Redis 实现。
//
// 它只负责把运行时配置序列化后写入 Redis，或者从 Redis 读出并反序列化回来。
type redisImageSourceStore struct {
	rdb *redis.Client
	key string
}

// runtimeImageSourceStorePayload 是 Redis 中实际存储的 JSON 结构。
//
// 这里和 ImageSourceRuntimeConfig 分开，是为了控制持久化格式，
// 也方便未来在不影响内部结构的情况下扩展存储内容，例如增加版本、来源或审计字段。
type runtimeImageSourceStorePayload struct {
	URL                string `json:"url"`
	APIKey             string `json:"apiKey"`
	TimeoutSeconds     int    `json:"timeoutSeconds"`
	RateLimitPerMinute int    `json:"rateLimitPerMinute"`
	RetryCount         int    `json:"retryCount"`
	UpdatedAt          string `json:"updatedAt"`
}

// NewRedisImageSourceStore 创建一个基于 Redis 的运行时图片源配置存储。
//
// 如果没有传入 Redis 客户端，则返回 nil，表示当前环境没有可用的持久化层。
func NewRedisImageSourceStore(rdb *redis.Client) ImageSourceStore {
	// 允许调用方在没有 Redis 时继续工作；此时运行时配置只能存在于内存中。
	if rdb == nil {
		return nil
	}

	return &redisImageSourceStore{
		rdb: rdb,
		key: runtimeImageSourceStoreKey,
	}
}

// Load 从 Redis 中读取并恢复运行时图片源配置。
//
// 返回约定：
// - key 不存在时，返回 false, nil，表示“当前还没有持久化配置”；
// - Redis 访问失败或 JSON 解码失败时，返回 error；
// - 读取成功时，返回 true, nil。
func (s *redisImageSourceStore) Load(ctx context.Context) (ImageSourceRuntimeConfig, bool, error) {
	// 允许空接收者或空客户端，方便上层在没有 Redis 时直接跳过恢复逻辑。
	if s == nil || s.rdb == nil {
		return ImageSourceRuntimeConfig{}, false, nil
	}

	// 读取 Redis 中固定 key 对应的 JSON 数据。
	value, err := s.rdb.Get(ctx, s.key).Bytes()
	if err == redis.Nil {
		// key 不存在不是错误，只表示从未保存过运行时配置。
		return ImageSourceRuntimeConfig{}, false, nil
	}
	if err != nil {
		// 这里包装错误，方便上层日志里直接看到是 load 阶段失败。
		return ImageSourceRuntimeConfig{}, false, fmt.Errorf("load runtime image source config: %w", err)
	}

	// 把 Redis 中的 JSON 解码成持久化 payload。
	var payload runtimeImageSourceStorePayload
	if err := json.Unmarshal(value, &payload); err != nil {
		// 如果数据被手工修改或版本不兼容，这里会失败。
		return ImageSourceRuntimeConfig{}, false, fmt.Errorf("decode runtime image source config: %w", err)
	}

	// 将持久化结构转换回运行时配置结构。
	return ImageSourceRuntimeConfig{
		URL:                payload.URL,
		APIKey:             payload.APIKey,
		TimeoutSeconds:     payload.TimeoutSeconds,
		RateLimitPerMinute: payload.RateLimitPerMinute,
		RetryCount:         payload.RetryCount,
	}, true, nil
}

// Save 将运行时图片源配置写入 Redis。
//
// 保存的是当前已经生效的配置快照，便于后续实例重启时恢复。
func (s *redisImageSourceStore) Save(ctx context.Context, cfg ImageSourceRuntimeConfig) error {
	// 允许空接收者或空客户端；这种情况下保存动作直接视为成功跳过。
	if s == nil || s.rdb == nil {
		return nil
	}

	// 组装 Redis 中的持久化 payload。
	// UpdatedAt 记录写入时间，便于排查最近一次配置落盘的时间点。
	payload := runtimeImageSourceStorePayload{
		URL:                cfg.URL,
		APIKey:             cfg.APIKey,
		TimeoutSeconds:     cfg.TimeoutSeconds,
		RateLimitPerMinute: cfg.RateLimitPerMinute,
		RetryCount:         cfg.RetryCount,
		UpdatedAt:          time.Now().Format(time.RFC3339),
	}

	// 统一采用 JSON 编码，保证后续读取方可以稳定解析。
	encoded, err := json.Marshal(payload)
	if err != nil {
		// 序列化失败通常意味着结构体里出现了不可编码的内容，
		// 这里把错误包装后返回给上层。
		return fmt.Errorf("encode runtime image source config: %w", err)
	}

	// TTL 设为 0，表示这个配置不自动过期。
	// 运行时图片源配置需要长期保留，除非被新的配置覆盖。
	if err := s.rdb.Set(ctx, s.key, encoded, 0).Err(); err != nil {
		// Redis 写入失败时直接返回错误，让上层知道这次持久化没有成功。
		return fmt.Errorf("save runtime image source config: %w", err)
	}

	return nil
}
