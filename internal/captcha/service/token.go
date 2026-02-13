package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"

	"github.com/Pupervemon/risk-engine/internal/config"
)

type TokenService struct {
	cfg    *config.TokenConfig
	secret []byte
}

type tokenPayload struct {
	CaptchaID string `json:"cid"`
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
}

func NewTokenService(cfg *config.TokenConfig) *TokenService {
	return &TokenService{cfg: cfg, secret: []byte(cfg.Secret)}
}

func (s *TokenService) IssueToken(captchaID string) (string, int64, error) {
	now := time.Now().Unix()
	exp := now + int64(s.cfg.TTLSeconds)

	payload := tokenPayload{
		CaptchaID: captchaID,
		IssuedAt:  now,
		ExpiresAt: exp,
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", 0, err
	}

	payloadB64 := base64.RawURLEncoding.EncodeToString(payloadJSON)
	sig := s.sign(payloadB64)

	token := payloadB64 + "." + sig
	return token, exp, nil
}

func (s *TokenService) VerifyToken(token string) (bool, string, int64) {
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

	now := time.Now().Unix()
	if payload.ExpiresAt <= now {
		return false, "TOKEN_EXPIRED", payload.ExpiresAt
	}

	return true, "OK", payload.ExpiresAt
}

func (s *TokenService) sign(payloadB64 string) string {
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(payloadB64))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
