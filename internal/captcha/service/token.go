package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Pupervemon/risk-engine/internal/shared/config"
	"github.com/redis/go-redis/v9"
)

type tokenStorage interface {
	Set(ctx context.Context, key string, value []byte, expiration time.Duration) error
	Exists(ctx context.Context, key string) (bool, error)
}

type redisTokenStorage struct {
	rdb *redis.Client
}

func newRedisTokenStorage(rdb *redis.Client) tokenStorage {
	if rdb == nil {
		return nil
	}
	return &redisTokenStorage{rdb: rdb}
}

func (s *redisTokenStorage) Set(ctx context.Context, key string, value []byte, expiration time.Duration) error {
	return s.rdb.Set(ctx, key, value, expiration).Err()
}

func (s *redisTokenStorage) Exists(ctx context.Context, key string) (bool, error) {
	count, err := s.rdb.Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// TokenService issues signed tokens and persists their presence in Redis.
type TokenService struct {
	cfg    *config.TokenConfig
	secret []byte
	store  tokenStorage
}

type tokenPayload struct {
	CaptchaID string `json:"cid"`
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
}

func NewTokenService(rdb *redis.Client, cfg *config.TokenConfig) *TokenService {
	return newTokenServiceWithStore(cfg, newRedisTokenStorage(rdb))
}

func newTokenServiceWithStore(cfg *config.TokenConfig, store tokenStorage) *TokenService {
	return &TokenService{
		cfg:    cfg,
		secret: []byte(cfg.Secret),
		store:  store,
	}
}

func (s *TokenService) IssueToken(ctx context.Context, captchaID string) (string, int64, error) {
	if s.store == nil {
		return "", 0, fmt.Errorf("token store is not configured")
	}

	now := time.Now()
	expiresAt := now.Add(s.tokenTTL())
	payload := tokenPayload{
		CaptchaID: captchaID,
		IssuedAt:  now.Unix(),
		ExpiresAt: expiresAt.Unix(),
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", 0, err
	}

	payloadB64 := base64.RawURLEncoding.EncodeToString(payloadJSON)
	token := payloadB64 + "." + s.sign(payloadB64)

	if err := s.store.Set(ctx, s.tokenKey(token), payloadJSON, time.Until(expiresAt)); err != nil {
		return "", 0, err
	}

	return token, payload.ExpiresAt, nil
}

func (s *TokenService) VerifyToken(ctx context.Context, token string) (bool, string, int64) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return false, "TOKEN_FORMAT_INVALID", 0
	}

	payloadB64 := parts[0]
	signature := parts[1]
	expected := s.sign(payloadB64)
	if !hmac.Equal([]byte(signature), []byte(expected)) {
		return false, "TOKEN_SIGNATURE_INVALID", 0
	}

	payloadJSON, err := base64.RawURLEncoding.DecodeString(payloadB64)
	if err != nil {
		return false, "TOKEN_PAYLOAD_INVALID", 0
	}

	var payload tokenPayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return false, "TOKEN_PAYLOAD_INVALID", 0
	}

	if payload.ExpiresAt <= time.Now().Unix() {
		return false, "TOKEN_EXPIRED", payload.ExpiresAt
	}

	if s.store == nil {
		return false, "TOKEN_STORE_NOT_CONFIGURED", payload.ExpiresAt
	}

	exists, err := s.store.Exists(ctx, s.tokenKey(token))
	if err != nil {
		return false, "TOKEN_STORE_ERROR", payload.ExpiresAt
	}
	if !exists {
		return false, "TOKEN_NOT_FOUND", payload.ExpiresAt
	}

	return true, "OK", payload.ExpiresAt
}

func (s *TokenService) tokenKey(token string) string {
	sum := sha256.Sum256([]byte(token))
	return fmt.Sprintf("captcha:token:%s", hex.EncodeToString(sum[:]))
}

func (s *TokenService) tokenTTL() time.Duration {
	if s.cfg == nil || s.cfg.TTLSeconds <= 0 {
		return 10 * time.Minute
	}
	return time.Duration(s.cfg.TTLSeconds) * time.Second
}

func (s *TokenService) sign(payloadB64 string) string {
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(payloadB64))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
