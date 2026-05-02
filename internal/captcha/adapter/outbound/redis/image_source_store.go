package redisadapter

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	appports "github.com/Pupervemon/risk-engine/internal/captcha/application/ports"
	"github.com/Pupervemon/risk-engine/internal/captcha/domain"
	"github.com/redis/go-redis/v9"
)

const runtimeImageSourceStoreKey = "captcha:image-source:runtime-config"

// ImageSourceStore persists the effective runtime image-source config in Redis.
type ImageSourceStore struct {
	rdb *redis.Client
	key string
}

var _ appports.RuntimeImageSourceStore = (*ImageSourceStore)(nil)

type runtimeImageSourceStorePayload struct {
	URL                string `json:"url"`
	APIKey             string `json:"apiKey"`
	TimeoutSeconds     int    `json:"timeoutSeconds"`
	RateLimitPerMinute int    `json:"rateLimitPerMinute"`
	RetryCount         int    `json:"retryCount"`
	UpdatedAt          string `json:"updatedAt"`
}

func NewImageSourceStore(rdb *redis.Client) *ImageSourceStore {
	if rdb == nil {
		return nil
	}

	return &ImageSourceStore{
		rdb: rdb,
		key: runtimeImageSourceStoreKey,
	}
}

func (s *ImageSourceStore) Load(ctx context.Context) (domain.ImageSourceRuntimeConfig, bool, error) {
	if s == nil || s.rdb == nil {
		return domain.ImageSourceRuntimeConfig{}, false, nil
	}

	value, err := s.rdb.Get(ctx, s.key).Bytes()
	if err == redis.Nil {
		return domain.ImageSourceRuntimeConfig{}, false, nil
	}
	if err != nil {
		return domain.ImageSourceRuntimeConfig{}, false, fmt.Errorf("load runtime image source config: %w", err)
	}

	var payload runtimeImageSourceStorePayload
	if err := json.Unmarshal(value, &payload); err != nil {
		return domain.ImageSourceRuntimeConfig{}, false, fmt.Errorf("decode runtime image source config: %w", err)
	}

	return domain.ImageSourceRuntimeConfig{
		URL:                payload.URL,
		APIKey:             payload.APIKey,
		TimeoutSeconds:     payload.TimeoutSeconds,
		RateLimitPerMinute: payload.RateLimitPerMinute,
		RetryCount:         payload.RetryCount,
	}, true, nil
}

func (s *ImageSourceStore) Save(ctx context.Context, cfg domain.ImageSourceRuntimeConfig) error {
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
