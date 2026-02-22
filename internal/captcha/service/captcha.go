package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"time"

	"github.com/Pupervemon/risk-engine/internal/shared/config"
	"github.com/redis/go-redis/v9"
	"github.com/wenlng/go-captcha/v2/base/option"
	"github.com/wenlng/go-captcha/v2/slide"
	"go.uber.org/zap"
)

type CaptchaService struct {
	rdb            *redis.Client
	cfg            *config.CaptchaConfigSpec
	logger         *zap.Logger
	ttl            time.Duration
	slideCaptcha   slide.Captcha
	imagePool      *RedisImagePool // 图片池
	trackValidator *TrackValidator // 轨迹校验器
	useImagePool   bool            // 是否使用图片池
}

type SliderChallenge struct {
	CaptchaID         string
	MasterImage       string
	TileImage         string
	TargetY           int
	ExpiresIn         int
	RequireMouseTrack bool // 是否需要鼠标轨迹
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

	// 创建滑块验证码生成器（先用默认背景）
	builder := slide.NewBuilder(
		slide.WithImageSize(option.Size{Width: width, Height: height}),
		slide.WithRangeGraphSize(option.RangeVal{Min: graphSizeMin, Max: graphSizeMax}),
		slide.WithGenGraphNumber(1),
	)
	builder.SetResources(
		slide.WithBackgrounds(defaultBackgrounds(width, height)),
		slide.WithGraphImages(defaultGraphImages(64)),
	)

	service := &CaptchaService{
		rdb:          rdb,
		cfg:          cfg,
		logger:       logger,
		ttl:          ttl,
		slideCaptcha: builder.Make(),
		useImagePool: cfg.ImagePool.Enabled,
	}

	// 初始化图片池（如果启用）
	if cfg.ImagePool.Enabled {
		poolSize := cfg.ImagePool.PoolSize
		if poolSize <= 0 {
			poolSize = 50
		}

		// 创建图片提供者
		apiConfig := ExternalImageAPIConfig{
			URL:                cfg.ExternalImageAPI.URL,
			APIKey:             cfg.ExternalImageAPI.APIKey,
			Timeout:            cfg.ExternalImageAPI.GetTimeout(),
			RateLimitPerMinute: cfg.ExternalImageAPI.RateLimitPerMinute,
			RetryCount:         cfg.ExternalImageAPI.RetryCount,
		}

		provider := CustomImageFetcher(apiConfig, logger, width, height)
		service.imagePool = NewRedisImagePool(rdb, logger, provider, poolSize)

		logger.Info("图片池已初始化",
			zap.Int("pool_size", poolSize),
			zap.Bool("enabled", true))
	}

	// 初始化轨迹校验器
	service.trackValidator = NewTrackValidator(cfg.TrackValidation, logger)
	if cfg.TrackValidation.Enabled {
		logger.Info("轨迹校验已启用",
			zap.Int("min_points", cfg.TrackValidation.MinPoints),
			zap.Int64("min_duration_ms", cfg.TrackValidation.MinDurationMs))
	}

	return service
}

func (s *CaptchaService) Generate(ctx context.Context) (*SliderChallenge, error) {
	// 如果启用图片池，尝试从图片池获取背景图
	if s.useImagePool && s.imagePool != nil {
		imageData, err := s.imagePool.GetRandom(ctx)
		if err != nil {
			s.logger.Warn("从图片池获取图片失败，使用默认背景", zap.Error(err))
		} else {
			// 解码图片
			img, err := png.Decode(bytes.NewReader(imageData))
			if err == nil {
				// 使用图片池的背景重建builder
				builder := slide.NewBuilder(
					slide.WithImageSize(option.Size{Width: s.cfg.Width, Height: s.cfg.Height}),
					slide.WithRangeGraphSize(option.RangeVal{Min: s.cfg.GraphSizeMin, Max: s.cfg.GraphSizeMax}),
					slide.WithGenGraphNumber(1),
				)
				builder.SetResources(
					slide.WithBackgrounds([]image.Image{img}),
					slide.WithGraphImages(defaultGraphImages(64)),
				)
				s.slideCaptcha = builder.Make()
			} else {
				s.logger.Warn("解码图片失败，使用默认背景", zap.Error(err))
			}
		}
	}

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
		CaptchaID:         captchaID,
		MasterImage:       masterBase64,
		TileImage:         tileBase64,
		TargetY:           block.DY,
		ExpiresIn:         ttlSeconds,
		RequireMouseTrack: s.cfg.TrackValidation.Enabled,
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

// VerifyWithTrack 带轨迹校验的验证方法
func (s *CaptchaService) VerifyWithTrack(ctx context.Context, captchaID string, pointX, pointY int, mouseTrack *[]TrackPoint) (bool, string, error) {
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

	// 1. 先校验坐标
	padding := s.cfg.SliderTolerance
	if padding <= 0 {
		padding = 8
	}

	matched := slide.Validate(pointX, answer.DY, answer.DX, answer.DY, padding)
	if !matched {
		if delErr := s.deleteAnswer(ctx, captchaID); delErr != nil {
			s.logger.Warn("删除滑块验证码答案失败", zap.String("captcha_id", captchaID), zap.Error(delErr))
		}
		return false, "CAPTCHA_MISMATCH", nil
	}

	// 2. 如果启用轨迹校验且提供了轨迹数据，则进行轨迹校验
	if s.trackValidator.config.Enabled && mouseTrack != nil && len(*mouseTrack) > 0 {
		result := s.trackValidator.Validate(*mouseTrack, answer.DX)
		if !result.Valid {
			if delErr := s.deleteAnswer(ctx, captchaID); delErr != nil {
				s.logger.Warn("删除滑块验证码答案失败", zap.String("captcha_id", captchaID), zap.Error(delErr))
			}
			s.logger.Info("轨迹校验失败",
				zap.String("captcha_id", captchaID),
				zap.String("reason", result.Code),
				zap.String("message", result.Message))
			return false, result.Code, nil
		}
		s.logger.Debug("轨迹校验通过", zap.String("captcha_id", captchaID))
	}

	// 3. 清理答案
	if delErr := s.deleteAnswer(ctx, captchaID); delErr != nil {
		s.logger.Warn("删除滑块验证码答案失败", zap.String("captcha_id", captchaID), zap.Error(delErr))
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

	// 创建拼图形状的遮罩（定义滑块形状）
	// mask 决定了哪些部分会从背景图中切割出来
	mask := image.NewRGBA(rect)
	drawPuzzleShape(mask, size)

	// 创建覆盖图（在滑块上叠加的装饰效果）
	// 使用轻微的白边效果，让滑块更明显但不遮挡背景图
	overlay := image.NewRGBA(rect)
	drawPuzzleBorder(overlay, size, color.RGBA{R: 255, G: 255, B: 255, A: 100})

	// 创建阴影图（背景缺口的阴影效果）
	shadow := image.NewRGBA(rect)
	drawPuzzleShapeWithStyle(shadow, size, color.RGBA{R: 0, G: 0, B: 0, A: 80})

	return []*slide.GraphImage{{
		OverlayImage: overlay,
		ShadowImage:  shadow,
		MaskImage:    mask,
	}}
}

// drawPuzzleBorder 只绘制拼图形状的边框，不填充内部
func drawPuzzleBorder(img *image.RGBA, size int, borderColor color.RGBA) {
	center := size / 2
	mainSize := int(float64(size) * 0.7)
	offset := (size - mainSize) / 2
	bumpSize := mainSize / 4

	// 先填充透明
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			img.Set(x, y, color.RGBA{R: 0, G: 0, B: 0, A: 0})
		}
	}

	// 绘制边框（2像素宽）
	borderWidth := 2

	// 绘制主体边框
	for y := offset; y < offset+mainSize; y++ {
		for x := offset; x < offset+mainSize; x++ {
			// 只绘制边缘
			if x < offset+borderWidth || x >= offset+mainSize-borderWidth ||
				y < offset+borderWidth || y >= offset+mainSize-borderWidth {
				img.Set(x, y, borderColor)
			}
		}
	}

	// 右侧凸起边框
	rightX := offset + mainSize
	rightY := center
	for y := -bumpSize; y <= bumpSize; y++ {
		for x := 0; x <= bumpSize; x++ {
			if x*x+y*y <= bumpSize*bumpSize && x*x+y*y >= (bumpSize-borderWidth)*(bumpSize-borderWidth) {
				px := rightX + x
				py := rightY + y
				if px < size && py >= 0 && py < size {
					img.Set(px, py, borderColor)
				}
			}
		}
	}

	// 底部凹陷边框
	bottomX := center
	bottomY := offset + mainSize
	for y := -bumpSize; y <= 0; y++ {
		for x := -bumpSize; x <= bumpSize; x++ {
			if x*x+y*y <= bumpSize*bumpSize && x*x+y*y >= (bumpSize-borderWidth)*(bumpSize-borderWidth) {
				px := bottomX + x
				py := bottomY + y
				if px >= offset && px < offset+mainSize && py >= offset && py < offset+mainSize {
					img.Set(px, py, borderColor)
				}
			}
		}
	}
}

// drawPuzzleShape 绘制拼图形状的遮罩
func drawPuzzleShape(img *image.RGBA, size int) {
	// 创建一个经典的拼图块形状
	// 主体是一个正方形，带有一个凸起和一个凹陷

	center := size / 2
	mainSize := int(float64(size) * 0.7) // 主体大小
	offset := (size - mainSize) / 2
	bumpSize := mainSize / 4 // 凸起/凹陷的大小

	// 填充整个区域为透明
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			img.Set(x, y, color.RGBA{R: 0, G: 0, B: 0, A: 0})
		}
	}

	// 绘制主体矩形
	for y := offset; y < offset+mainSize; y++ {
		for x := offset; x < offset+mainSize; x++ {
			img.Set(x, y, color.RGBA{R: 255, G: 255, B: 255, A: 255})
		}
	}

	// 在右侧添加半圆形凸起
	rightX := offset + mainSize
	rightY := center
	for y := -bumpSize; y <= bumpSize; y++ {
		for x := 0; x <= bumpSize; x++ {
			// 半圆判断
			if x*x+y*y <= bumpSize*bumpSize {
				px := rightX + x
				py := rightY + y
				if px < size && py >= 0 && py < size {
					img.Set(px, py, color.RGBA{R: 255, G: 255, B: 255, A: 255})
				}
			}
		}
	}

	// 在底部添加半圆形凹陷（通过不绘制来实现）
	bottomX := center
	bottomY := offset + mainSize
	for y := -bumpSize; y <= 0; y++ {
		for x := -bumpSize; x <= bumpSize; x++ {
			// 半圆判断
			if x*x+y*y <= bumpSize*bumpSize {
				px := bottomX + x
				py := bottomY + y
				if px >= offset && px < offset+mainSize && py >= offset && py < offset+mainSize {
					img.Set(px, py, color.RGBA{R: 0, G: 0, B: 0, A: 0})
				}
			}
		}
	}
}

// drawPuzzleShapeWithStyle 绘制带样式的拼图形状
func drawPuzzleShapeWithStyle(img *image.RGBA, size int, fillColor color.RGBA) {
	center := size / 2
	mainSize := int(float64(size) * 0.7)
	offset := (size - mainSize) / 2
	bumpSize := mainSize / 4

	// 先填充透明
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			img.Set(x, y, color.RGBA{R: 0, G: 0, B: 0, A: 0})
		}
	}

	// 绘制主体
	for y := offset; y < offset+mainSize; y++ {
		for x := offset; x < offset+mainSize; x++ {
			img.Set(x, y, fillColor)
		}
	}

	// 右侧凸起
	rightX := offset + mainSize
	rightY := center
	for y := -bumpSize; y <= bumpSize; y++ {
		for x := 0; x <= bumpSize; x++ {
			if x*x+y*y <= bumpSize*bumpSize {
				px := rightX + x
				py := rightY + y
				if px < size && py >= 0 && py < size {
					img.Set(px, py, fillColor)
				}
			}
		}
	}

	// 底部凹陷保持透明
	bottomX := center
	bottomY := offset + mainSize
	for y := -bumpSize; y <= 0; y++ {
		for x := -bumpSize; x <= bumpSize; x++ {
			if x*x+y*y <= bumpSize*bumpSize {
				px := bottomX + x
				py := bottomY + y
				if px >= offset && px < offset+mainSize && py >= offset && py < offset+mainSize {
					img.Set(px, py, color.RGBA{R: 0, G: 0, B: 0, A: 0})
				}
			}
		}
	}
}

// StartImageRefresh 启动图片池定时刷新
func (s *CaptchaService) StartImageRefresh(ctx context.Context) error {
	if !s.useImagePool || s.imagePool == nil {
		s.logger.Info("图片池未启用，跳过刷新任务")
		return nil
	}

	refreshInterval := s.cfg.ImagePool.GetRefreshInterval()
	s.imagePool.StartRefresh(ctx, refreshInterval)

	s.logger.Info("图片池刷新任务已启动",
		zap.Duration("interval", refreshInterval))

	return nil
}

// StopImageRefresh 停止图片池定时刷新
func (s *CaptchaService) StopImageRefresh() {
	if s.imagePool != nil {
		s.imagePool.StopRefresh()
		s.logger.Info("图片池刷新任务已停止")
	}
}

// GetImagePoolStatus 获取图片池状态（用于健康检查）
func (s *CaptchaService) GetImagePoolStatus(ctx context.Context) map[string]interface{} {
	status := map[string]interface{}{
		"enabled": s.useImagePool,
	}

	if s.useImagePool && s.imagePool != nil {
		count, err := s.imagePool.Count(ctx)
		if err != nil {
			status["error"] = err.Error()
			status["count"] = 0
		} else {
			status["count"] = count
		}
	}

	return status
}
