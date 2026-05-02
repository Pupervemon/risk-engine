package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	redisadapter "github.com/Pupervemon/risk-engine/internal/captcha/adapter/outbound/redis"
	captchaapp "github.com/Pupervemon/risk-engine/internal/captcha/application"
	appports "github.com/Pupervemon/risk-engine/internal/captcha/application/ports"
	"github.com/Pupervemon/risk-engine/internal/shared/config"
	"github.com/redis/go-redis/v9"
)

const tokenKeyPrefix = "captcha:token:"

// tokenStorage is kept as a compatibility seam for existing service tests.
type tokenStorage interface {
	Set(ctx context.Context, key string, value []byte, expiration time.Duration) error
	Exists(ctx context.Context, key string) (bool, error)
}

// TokenService keeps the old service-facing API while delegating token behavior
// to the application layer.
type TokenService struct {
	usecase appports.TokenUseCase
}

// NewTokenService initializes the legacy token service wrapper.
func NewTokenService(rdb *redis.Client, cfg *config.TokenConfig) *TokenService {
	return newTokenServiceWithUseCase(captchaapp.NewTokenUseCase(
		redisadapter.NewTokenRepository(rdb),
		tokenOptionsFromSharedConfig(cfg),
	))
}

func newTokenServiceWithStore(cfg *config.TokenConfig, store tokenStorage) *TokenService {
	var repo appports.TokenRepository
	if store != nil {
		repo = tokenStorageRepository{store: store}
	}

	return newTokenServiceWithUseCase(captchaapp.NewTokenUseCase(repo, tokenOptionsFromSharedConfig(cfg)))
}

func newTokenServiceWithUseCase(usecase appports.TokenUseCase) *TokenService {
	return &TokenService{usecase: usecase}
}

// IssueToken generates a new signed token and persists it through the
// application use case.
func (s *TokenService) IssueToken(ctx context.Context, captchaID string) (string, int64, error) {
	if s == nil || s.usecase == nil {
		return "", 0, fmt.Errorf("token use case is not configured")
	}

	token, err := s.usecase.Issue(ctx, captchaID)
	if err != nil {
		return "", 0, err
	}

	return token.Token, token.ExpiresAt, nil
}

// VerifyToken verifies a token through the application use case.
func (s *TokenService) VerifyToken(ctx context.Context, token string) (bool, string, int64) {
	if s == nil || s.usecase == nil {
		return false, "TOKEN_STORE_NOT_CONFIGURED", 0
	}

	result, err := s.usecase.Verify(ctx, token)
	if err != nil {
		return false, "TOKEN_STORE_ERROR", 0
	}

	return result.Valid, result.Reason, result.ExpiresAt
}

// tokenKey is kept for service-package tests that inspect the legacy Redis key.
func (s *TokenService) tokenKey(token string) string {
	return tokenKeyFromDigest(tokenDigest(token))
}

type tokenStorageRepository struct {
	store tokenStorage
}

var _ appports.TokenRepository = tokenStorageRepository{}

func (r tokenStorageRepository) Save(ctx context.Context, tokenDigest string, value []byte, expiration time.Duration) error {
	if r.store == nil {
		return fmt.Errorf("token store is not configured")
	}
	return r.store.Set(ctx, tokenKeyFromDigest(tokenDigest), value, expiration)
}

func (r tokenStorageRepository) Exists(ctx context.Context, tokenDigest string) (bool, error) {
	if r.store == nil {
		return false, fmt.Errorf("token store is not configured")
	}
	return r.store.Exists(ctx, tokenKeyFromDigest(tokenDigest))
}

func tokenOptionsFromSharedConfig(cfg *config.TokenConfig) captchaapp.TokenOptions {
	if cfg == nil {
		return captchaapp.TokenOptions{}
	}

	return captchaapp.TokenOptions{
		TTLSeconds: cfg.TTLSeconds,
		Secret:     cfg.Secret,
	}
}

func tokenDigest(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func tokenKeyFromDigest(digest string) string {
	return tokenKeyPrefix + digest
}
