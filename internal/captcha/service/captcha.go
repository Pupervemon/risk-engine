package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"time"

	"github.com/Pupervemon/risk-engine/internal/config"
	"github.com/redis/go-redis/v9"
	"github.com/wenlng/go-captcha/v2/base/option"
	"github.com/wenlng/go-captcha/v2/slide"
	"go.uber.org/zap"
)

type CaptchaService struct {
	rdb          *redis.Client
	cfg          *config.CaptchaConfigSpec
	logger       *zap.Logger
	ttl          time.Duration
	slideCaptcha slide.Captcha
}

type SliderChallenge struct {
	CaptchaID   string
	MasterImage string
	TileImage   string
	TargetY     int
	ExpiresIn   int
}

type sliderAnswer struct {
	DX int `json:"dx"`
	DY int `json:"dy"`
}

func NewCaptchaService(rdb *redis.Client, cfg *config.CaptchaConfigSpec, logger *zap.Logger) *CaptchaService {
	ttl := time.Duration(cfg.TTLSeconds) * time.Second
	if cfg.TTLSeconds <= 0 {
		ttl = 120 * time.Second
	}

	width := cfg.Width
	if width <= 0 {
		width = 320
	}
	height := cfg.Height
	if height <= 0 {
		height = 180
	}

	graphSizeMin := cfg.GraphSizeMin
	if graphSizeMin <= 0 {
		graphSizeMin = 52
	}
	graphSizeMax := cfg.GraphSizeMax
	if graphSizeMax < graphSizeMin {
		graphSizeMax = graphSizeMin + 8
	}

	builder := slide.NewBuilder(
		slide.WithImageSize(option.Size{Width: width, Height: height}),
		slide.WithRangeGraphSize(option.RangeVal{Min: graphSizeMin, Max: graphSizeMax}),
		slide.WithGenGraphNumber(1),
	)
	builder.SetResources(
		slide.WithBackgrounds(defaultBackgrounds(width, height)),
		slide.WithGraphImages(defaultGraphImages(64)),
	)

	return &CaptchaService{
		rdb:          rdb,
		cfg:          cfg,
		logger:       logger,
		ttl:          ttl,
		slideCaptcha: builder.Make(),
	}
}

func (s *CaptchaService) Generate(ctx context.Context) (*SliderChallenge, error) {
	capData, err := s.slideCaptcha.Generate()
	if err != nil {
		return nil, err
	}

	block := capData.GetData()
	if block == nil {
		return nil, fmt.Errorf("CAPTCHA_DATA_INVALID")
	}

	masterBase64, err := capData.GetMasterImage().ToBase64()
	if err != nil {
		return nil, err
	}
	tileBase64, err := capData.GetTileImage().ToBase64()
	if err != nil {
		return nil, err
	}

	captchaID, err := randomHex(16)
	if err != nil {
		return nil, err
	}

	// 使用 block.X 作为正确答案
	answer := sliderAnswer{DX: block.X, DY: block.Y}
	if err := s.saveAnswer(ctx, captchaID, answer); err != nil {
		return nil, err
	}

	ttlSeconds := s.cfg.TTLSeconds
	if ttlSeconds <= 0 {
		ttlSeconds = 120
	}

	return &SliderChallenge{
		CaptchaID:   captchaID,
		MasterImage: masterBase64,
		TileImage:   tileBase64,
		TargetY:     block.DY,
		ExpiresIn:   ttlSeconds,
	}, nil
}

func (s *CaptchaService) Verify(ctx context.Context, captchaID string, pointX, pointY int) (bool, string, error) {
	if captchaID == "" {
		return false, "CAPTCHA_ID_EMPTY", nil
	}

	answer, err := s.loadAnswer(ctx, captchaID)
	if err == redis.Nil {
		return false, "CAPTCHA_NOT_FOUND", nil
	}
	if err != nil {
		return false, "REDIS_ERROR", err
	}

	padding := s.cfg.SliderTolerance
	if padding <= 0 {
		padding = 8
	}

	// 对于滑块验证码，通常只需要校验 X 轴的偏移量。
	matched := slide.Validate(pointX, answer.DY, answer.DX, answer.DY, padding)
	if delErr := s.deleteAnswer(ctx, captchaID); delErr != nil {
		s.logger.Warn("删除滑块验证码答案失败", zap.String("captcha_id", captchaID), zap.Error(delErr))
	}

	if !matched {
		return false, "CAPTCHA_MISMATCH", nil
	}
	return true, "OK", nil
}

func (s *CaptchaService) answerKey(captchaID string) string {
	return fmt.Sprintf("captcha:slide:%s", captchaID)
}

func (s *CaptchaService) saveAnswer(ctx context.Context, captchaID string, answer sliderAnswer) error {
	payload, err := json.Marshal(answer)
	if err != nil {
		return err
	}
	return s.rdb.Set(ctx, s.answerKey(captchaID), payload, s.ttl).Err()
}

func (s *CaptchaService) loadAnswer(ctx context.Context, captchaID string) (*sliderAnswer, error) {
	value, err := s.rdb.Get(ctx, s.answerKey(captchaID)).Bytes()
	if err != nil {
		return nil, err
	}
	var answer sliderAnswer
	if err := json.Unmarshal(value, &answer); err != nil {
		return nil, err
	}
	return &answer, nil
}

func (s *CaptchaService) deleteAnswer(ctx context.Context, captchaID string) error {
	return s.rdb.Del(ctx, s.answerKey(captchaID)).Err()
}

func randomHex(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func defaultBackgrounds(width, height int) []image.Image {
	bg1 := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(bg1, bg1.Bounds(), &image.Uniform{C: color.RGBA{R: 245, G: 248, B: 255, A: 255}}, image.Point{}, draw.Src)

	bg2 := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(bg2, bg2.Bounds(), &image.Uniform{C: color.RGBA{R: 242, G: 250, B: 244, A: 255}}, image.Point{}, draw.Src)

	bg3 := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(bg3, bg3.Bounds(), &image.Uniform{C: color.RGBA{R: 252, G: 245, B: 242, A: 255}}, image.Point{}, draw.Src)

	return []image.Image{bg1, bg2, bg3}
}

func defaultGraphImages(size int) []*slide.GraphImage {
	rect := image.Rect(0, 0, size, size)

	overlay := image.NewRGBA(rect)
	draw.Draw(overlay, overlay.Bounds(), &image.Uniform{C: color.RGBA{R: 255, G: 255, B: 255, A: 230}}, image.Point{}, draw.Src)

	shadow := image.NewRGBA(rect)
	draw.Draw(shadow, shadow.Bounds(), &image.Uniform{C: color.RGBA{R: 40, G: 40, B: 40, A: 140}}, image.Point{}, draw.Src)

	mask := image.NewRGBA(rect)
	draw.Draw(mask, mask.Bounds(), &image.Uniform{C: color.RGBA{R: 255, G: 255, B: 255, A: 255}}, image.Point{}, draw.Src)

	return []*slide.GraphImage{{
		OverlayImage: overlay,
		ShadowImage:  shadow,
		MaskImage:    mask,
	}}
}
