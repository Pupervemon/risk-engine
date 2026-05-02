package application

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

	appports "github.com/Pupervemon/risk-engine/internal/captcha/application/ports"
)

// TokenOptions configures token issuance and verification.
type TokenOptions struct {
	TTLSeconds int
	Secret     string
}

// TokenUseCase implements token issuance and verification.
type TokenUseCase struct {
	opts TokenOptions
	repo appports.TokenRepository
}

type tokenPayload struct {
	CaptchaID string `json:"cid"`
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
}

// NewTokenUseCase creates a token use case.
func NewTokenUseCase(repo appports.TokenRepository, opts TokenOptions) *TokenUseCase {
	return &TokenUseCase{
		opts: normalizeTokenOptions(opts),
		repo: repo,
	}
}

// Issue generates a new signed token and persists it through the repository.
func (u *TokenUseCase) Issue(ctx context.Context, captchaID string) (appports.IssuedToken, error) {
	if u.repo == nil {
		return appports.IssuedToken{}, fmt.Errorf("token repository is not configured")
	}

	now := time.Now()
	expiresAt := now.Add(u.tokenTTL())
	payload := tokenPayload{
		CaptchaID: captchaID,
		IssuedAt:  now.Unix(),
		ExpiresAt: expiresAt.Unix(),
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return appports.IssuedToken{}, err
	}

	payloadB64 := base64.RawURLEncoding.EncodeToString(payloadJSON)
	token := payloadB64 + "." + u.sign(payloadB64)

	if err := u.repo.Save(ctx, u.tokenDigest(token), payloadJSON, time.Until(expiresAt)); err != nil {
		return appports.IssuedToken{}, err
	}

	return appports.IssuedToken{
		Token:     token,
		ExpiresAt: payload.ExpiresAt,
	}, nil
}

// Verify checks token signature, payload, expiry and repository presence.
func (u *TokenUseCase) Verify(ctx context.Context, token string) (appports.TokenVerification, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return appports.TokenVerification{Valid: false, Reason: "TOKEN_FORMAT_INVALID"}, nil
	}

	payloadB64 := parts[0]
	signature := parts[1]
	expected := u.sign(payloadB64)
	if !hmac.Equal([]byte(signature), []byte(expected)) {
		return appports.TokenVerification{Valid: false, Reason: "TOKEN_SIGNATURE_INVALID"}, nil
	}

	payloadJSON, err := base64.RawURLEncoding.DecodeString(payloadB64)
	if err != nil {
		return appports.TokenVerification{Valid: false, Reason: "TOKEN_PAYLOAD_INVALID"}, nil
	}

	var payload tokenPayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return appports.TokenVerification{Valid: false, Reason: "TOKEN_PAYLOAD_INVALID"}, nil
	}

	if payload.ExpiresAt <= time.Now().Unix() {
		return appports.TokenVerification{Valid: false, Reason: "TOKEN_EXPIRED", ExpiresAt: payload.ExpiresAt}, nil
	}

	if u.repo == nil {
		return appports.TokenVerification{Valid: false, Reason: "TOKEN_STORE_NOT_CONFIGURED", ExpiresAt: payload.ExpiresAt}, nil
	}

	exists, err := u.repo.Exists(ctx, u.tokenDigest(token))
	if err != nil {
		return appports.TokenVerification{Valid: false, Reason: "TOKEN_STORE_ERROR", ExpiresAt: payload.ExpiresAt}, nil
	}
	if !exists {
		return appports.TokenVerification{Valid: false, Reason: "TOKEN_NOT_FOUND", ExpiresAt: payload.ExpiresAt}, nil
	}

	return appports.TokenVerification{
		Valid:     true,
		Reason:    "OK",
		ExpiresAt: payload.ExpiresAt,
	}, nil
}

func normalizeTokenOptions(opts TokenOptions) TokenOptions {
	if opts.TTLSeconds <= 0 {
		opts.TTLSeconds = 600
	}
	return opts
}

func (u *TokenUseCase) tokenTTL() time.Duration {
	return time.Duration(u.opts.TTLSeconds) * time.Second
}

func (u *TokenUseCase) tokenDigest(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (u *TokenUseCase) sign(payloadB64 string) string {
	mac := hmac.New(sha256.New, []byte(u.opts.Secret))
	mac.Write([]byte(payloadB64))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
