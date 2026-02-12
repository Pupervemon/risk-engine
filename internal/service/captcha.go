package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/Pupervemon/risk-engine/internal/config"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type CaptchaService struct {
	rdb    *redis.Client
	cfg    *config.CaptchaConfigSpec
	logger *zap.Logger
}

func NewCaptchaService(rdb *redis.Client, cfg *config.CaptchaConfigSpec, logger *zap.Logger) *CaptchaService {
	return &CaptchaService{rdb: rdb, cfg: cfg, logger: logger}
}

func (s *CaptchaService) Generate(ctx context.Context) (string, string, string, int, error) {
	captchaID, err := randomHex(16)
	if err != nil {
		return "", "", "", 0, err
	}

	code, err := s.randomCode(s.cfg.Length)
	if err != nil {
		return "", "", "", 0, err
	}

	key := s.keyFor(captchaID)
	normalized := s.normalize(code)
	if err := s.rdb.Set(ctx, key, normalized, time.Duration(s.cfg.TTLSeconds)*time.Second).Err(); err != nil {
		return "", "", "", 0, err
	}

	svg := renderSVG(code)
	imageBase64 := base64.StdEncoding.EncodeToString([]byte(svg))

	return captchaID, imageBase64, code, s.cfg.TTLSeconds, nil
}

func (s *CaptchaService) Verify(ctx context.Context, captchaID, text string) (bool, string, error) {
	if captchaID == "" {
		return false, "CAPTCHA_ID_EMPTY", nil
	}
	if text == "" {
		return false, "CAPTCHA_TEXT_EMPTY", nil
	}

	key := s.keyFor(captchaID)
	stored, err := s.rdb.Get(ctx, key).Result()
	if err == redis.Nil {
		return false, "CAPTCHA_NOT_FOUND", nil
	}
	if err != nil {
		return false, "REDIS_ERROR", err
	}

	input := s.normalize(text)
	if stored != input {
		return false, "CAPTCHA_MISMATCH", nil
	}

	if err := s.rdb.Del(ctx, key).Err(); err != nil {
		s.logger.Warn("删除验证码失败", zap.String("captcha_id", captchaID), zap.Error(err))
	}

	return true, "OK", nil
}

func (s *CaptchaService) keyFor(captchaID string) string {
	return fmt.Sprintf("captcha:code:%s", captchaID)
}

func (s *CaptchaService) normalize(text string) string {
	if s.cfg.CaseInsensitive {
		return strings.ToUpper(text)
	}
	return text
}

func (s *CaptchaService) randomCode(length int) (string, error) {
	allowed := s.cfg.AllowedChars
	if allowed == "" {
		allowed = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	}
	if length <= 0 {
		length = 4
	}

	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}

	chars := []byte(allowed)
	for i := 0; i < length; i++ {
		buf[i] = chars[int(buf[i])%len(chars)]
	}

	return string(buf), nil
}

func randomHex(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func renderSVG(code string) string {
	width := 160
	height := 60
	return fmt.Sprintf(
		"<svg xmlns=\"http://www.w3.org/2000/svg\" width=\"%d\" height=\"%d\" viewBox=\"0 0 %d %d\">"+
			"<rect width=\"100%%\" height=\"100%%\" fill=\"#f2f2f2\"/>"+
			"<text x=\"50%%\" y=\"50%%\" dominant-baseline=\"middle\" text-anchor=\"middle\" "+
			"font-family=\"Arial, sans-serif\" font-size=\"32\" fill=\"#333\">%s</text>"+
			"</svg>",
		width, height, width, height, code,
	)
}
