package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const runtimeImageSourceStoreKey = "captcha:image-source:runtime-config"

// ImageSourceStore persists the active runtime image source config.
type ImageSourceStore interface {
	Load(ctx context.Context) (ImageSourceRuntimeConfig, bool, error)
	Save(ctx context.Context, cfg ImageSourceRuntimeConfig) error
}

type redisImageSourceStore struct {
	rdb *redis.Client
	key string
}

type runtimeImageSourceStorePayload struct {
	URL                string `json:"url"`
	APIKey             string `json:"apiKey"`
	TimeoutSeconds     int    `json:"timeoutSeconds"`
	RateLimitPerMinute int    `json:"rateLimitPerMinute"`
	RetryCount         int    `json:"retryCount"`
	UpdatedAt          string `json:"updatedAt"`
}

// NewRedisImageSourceStore creates a Redis-backed runtime image source store.
func NewRedisImageSourceStore(rdb *redis.Client) ImageSourceStore {
	if rdb == nil {
		return nil
	}

	return &redisImageSourceStore{
		rdb: rdb,
		key: runtimeImageSourceStoreKey,
	}
}

func (s *redisImageSourceStore) Load(ctx context.Context) (ImageSourceRuntimeConfig, bool, error) {
	if s == nil || s.rdb == nil {
		return ImageSourceRuntimeConfig{}, false, nil
	}

	value, err := s.rdb.Get(ctx, s.key).Bytes()
	if err == redis.Nil {
		return ImageSourceRuntimeConfig{}, false, nil
	}
	if err != nil {
		return ImageSourceRuntimeConfig{}, false, fmt.Errorf("load runtime image source config: %w", err)
	}

	var payload runtimeImageSourceStorePayload
	if err := json.Unmarshal(value, &payload); err != nil {
		return ImageSourceRuntimeConfig{}, false, fmt.Errorf("decode runtime image source config: %w", err)
	}

	return ImageSourceRuntimeConfig{
		URL:                payload.URL,
		APIKey:             payload.APIKey,
		TimeoutSeconds:     payload.TimeoutSeconds,
		RateLimitPerMinute: payload.RateLimitPerMinute,
		RetryCount:         payload.RetryCount,
	}, true, nil
}

func (s *redisImageSourceStore) Save(ctx context.Context, cfg ImageSourceRuntimeConfig) error {
	if s == nil || s.rdb == nil {
		return nil
	}

	payload := runtimeImageSourceStorePayload{
		URL:                cfg.URL,
		APIKey:             cfg.APIKey,
		TimeoutSeconds:     cfg.TimeoutSeconds,
		RateLimitPerMinute: cfg.RateLimitPerMinute,
		RetryCount:         cfg.RetryCount,
		UpdatedAt:          time.Now().Format(time.RFC3339),
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode runtime image source config: %w", err)
	}

	if err := s.rdb.Set(ctx, s.key, encoded, 0).Err(); err != nil {
		return fmt.Errorf("save runtime image source config: %w", err)
	}

	return nil
}
