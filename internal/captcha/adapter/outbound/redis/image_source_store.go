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

const (
	runtimeImageSourceConfigKey = "captcha:image_source:config"
	runtimeImageSourceStatusKey = "captcha:image_source:status"
)

// ImageSourceStore persists the effective runtime image-source config in Redis.
type ImageSourceStore struct {
	rdb *redis.Client
}

var _ appports.RuntimeImageSourceStore = (*ImageSourceStore)(nil)

type runtimeImageSourceStorePayload struct {
	Version            int64  `json:"version"`
	URL                string `json:"url"`
	APIKey             string `json:"apiKey"`
	TimeoutSeconds     int    `json:"timeoutSeconds"`
	RateLimitPerMinute int    `json:"rateLimitPerMinute"`
	RetryCount         int    `json:"retryCount"`
	UpdatedAt          string `json:"updatedAt"`
}

type runtimeImageSourceStatusPayload struct {
	LastValidatedAt     string `json:"lastValidatedAt"`
	LastValidationError string `json:"lastValidationError"`
	LastRefreshedAt     string `json:"lastRefreshedAt"`
	LastRefreshError    string `json:"lastRefreshError"`
}

func NewImageSourceStore(rdb *redis.Client) *ImageSourceStore {
	if rdb == nil {
		return nil
	}

	return &ImageSourceStore{rdb: rdb}
}

func (s *ImageSourceStore) Load(ctx context.Context) (domain.ImageSourceRuntimeConfig, bool, error) {
	if s == nil || s.rdb == nil {
		return domain.ImageSourceRuntimeConfig{}, false, nil
	}

	value, err := s.rdb.Get(ctx, runtimeImageSourceConfigKey).Bytes()
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
		Version:            payload.Version,
		URL:                payload.URL,
		APIKey:             payload.APIKey,
		TimeoutSeconds:     payload.TimeoutSeconds,
		RateLimitPerMinute: payload.RateLimitPerMinute,
		RetryCount:         payload.RetryCount,
		UpdatedAt:          payload.UpdatedAt,
	}, true, nil
}

func (s *ImageSourceStore) Save(ctx context.Context, cfg domain.ImageSourceRuntimeConfig) error {
	if s == nil || s.rdb == nil {
		return nil
	}

	payload := runtimeImageSourceStorePayload{
		Version:            cfg.Version,
		URL:                cfg.URL,
		APIKey:             cfg.APIKey,
		TimeoutSeconds:     cfg.TimeoutSeconds,
		RateLimitPerMinute: cfg.RateLimitPerMinute,
		RetryCount:         cfg.RetryCount,
		UpdatedAt:          cfg.UpdatedAt,
	}
	if payload.UpdatedAt == "" {
		payload.UpdatedAt = time.Now().Format(time.RFC3339)
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode runtime image source config: %w", err)
	}

	if err := s.rdb.Set(ctx, runtimeImageSourceConfigKey, encoded, 0).Err(); err != nil {
		return fmt.Errorf("save runtime image source config: %w", err)
	}

	return nil
}

func (s *ImageSourceStore) LoadStatus(ctx context.Context) (domain.ImageSourceRuntimeStatus, error) {
	if s == nil || s.rdb == nil {
		return domain.ImageSourceRuntimeStatus{}, nil
	}

	value, err := s.rdb.Get(ctx, runtimeImageSourceStatusKey).Bytes()
	if err == redis.Nil {
		return domain.ImageSourceRuntimeStatus{}, nil
	}
	if err != nil {
		return domain.ImageSourceRuntimeStatus{}, fmt.Errorf("load runtime image source status: %w", err)
	}

	var payload runtimeImageSourceStatusPayload
	if err := json.Unmarshal(value, &payload); err != nil {
		return domain.ImageSourceRuntimeStatus{}, fmt.Errorf("decode runtime image source status: %w", err)
	}

	return domain.ImageSourceRuntimeStatus{
		LastValidatedAt:     payload.LastValidatedAt,
		LastValidationError: payload.LastValidationError,
		LastRefreshedAt:     payload.LastRefreshedAt,
		LastRefreshError:    payload.LastRefreshError,
	}, nil
}

func (s *ImageSourceStore) SaveStatus(ctx context.Context, status domain.ImageSourceRuntimeStatus) error {
	if s == nil || s.rdb == nil {
		return nil
	}

	payload := runtimeImageSourceStatusPayload{
		LastValidatedAt:     status.LastValidatedAt,
		LastValidationError: status.LastValidationError,
		LastRefreshedAt:     status.LastRefreshedAt,
		LastRefreshError:    status.LastRefreshError,
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode runtime image source status: %w", err)
	}

	if err := s.rdb.Set(ctx, runtimeImageSourceStatusKey, encoded, 0).Err(); err != nil {
		return fmt.Errorf("save runtime image source status: %w", err)
	}

	return nil
}
