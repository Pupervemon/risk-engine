package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

// ImageAPIResponse 图片API响应结构
type ImageAPIResponse struct {
	Code   string `json:"code"`   // 状态码
	ImgURL string `json:"imgurl"` // 图片URL
	Width  string `json:"width"`  // 图片宽度
	Height string `json:"height"` // 图片高度
}

// ExternalImageAPIConfig 外部图片API配置
type ExternalImageAPIConfig struct {
	URL                string        // API基础URL
	APIKey             string        // API密钥
	Timeout            time.Duration // 请求超时时间
	RateLimitPerMinute int           // 每分钟速率限制
	RetryCount         int           // 重试次数
}

// ExternalImageFetcher 外部图片获取器
type ExternalImageFetcher struct {
	config       ExternalImageAPIConfig
	client       *http.Client
	logger       *zap.Logger
	limiter      *rate.Limiter // 速率限制器
	targetWidth  int           // 目标宽度
	targetHeight int           // 目标高度
}

// NewExternalImageFetcher 创建外部图片获取器
func NewExternalImageFetcher(config ExternalImageAPIConfig, logger *zap.Logger, targetWidth, targetHeight int) *ExternalImageFetcher {
	// 创建速率限制器
	limiter := rate.NewLimiter(rate.Limit(float64(config.RateLimitPerMinute)/60.0), config.RateLimitPerMinute)

	return &ExternalImageFetcher{
		config: config,
		client: &http.Client{
			Timeout: config.Timeout,
		},
		logger:       logger,
		limiter:      limiter,
		targetWidth:  targetWidth,
		targetHeight: targetHeight,
	}
}

// FetchImages 实现ImageProvider接口，批量获取图片
func (f *ExternalImageFetcher) FetchImages(ctx context.Context, count int) ([]ImageMeta, error) {
	f.logger.Info("开始从外部API获取图片",
		zap.Int("count", count),
		zap.String("api_url", f.config.URL))

	images := make([]ImageMeta, 0, count)
	successCount := 0
	failCount := 0

	for i := 0; i < count; i++ {
		// 速率限制
		if err := f.limiter.Wait(ctx); err != nil {
			f.logger.Warn("速率限制等待失败", zap.Error(err))
			break
		}

		// 带重试的图片获取
		img, err := f.fetchSingleImageWithRetry(ctx)
		if err != nil {
			f.logger.Warn("获取图片失败", zap.Int("index", i), zap.Error(err))
			failCount++
			continue
		}

		images = append(images, img)
		successCount++
	}

	if successCount == 0 {
		return nil, fmt.Errorf("failed to fetch any images: %d failures", failCount)
	}

	f.logger.Info("图片获取完成",
		zap.Int("success", successCount),
		zap.Int("failed", failCount))

	return images, nil
}

// fetchSingleImageWithRetry 带重试的单张图片获取
func (f *ExternalImageFetcher) fetchSingleImageWithRetry(ctx context.Context) (ImageMeta, error) {
	var lastErr error

	for attempt := 0; attempt <= f.config.RetryCount; attempt++ {
		if attempt > 0 {
			// 指数退避
			backoff := time.Duration(attempt*attempt) * time.Second
			f.logger.Debug("重试获取图片",
				zap.Int("attempt", attempt),
				zap.Duration("backoff", backoff))

			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return ImageMeta{}, ctx.Err()
			}
		}

		img, err := f.fetchSingleImage(ctx)
		if err == nil {
			return img, nil
		}
		lastErr = err
	}

	return ImageMeta{}, fmt.Errorf("failed after %d retries: %w", f.config.RetryCount, lastErr)
}

// fetchSingleImage 获取单张图片
func (f *ExternalImageFetcher) fetchSingleImage(ctx context.Context) (ImageMeta, error) {
	// 步骤1: 调用API获取图片URL
	req, err := http.NewRequestWithContext(ctx, "GET", f.config.URL, nil)
	if err != nil {
		return ImageMeta{}, fmt.Errorf("create request failed: %w", err)
	}

	// 添加认证头（如果需要）
	if f.config.APIKey != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", f.config.APIKey))
	}

	// 发送请求到API
	resp, err := f.client.Do(req)
	if err != nil {
		return ImageMeta{}, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ImageMeta{}, fmt.Errorf("API returned status code: %d", resp.StatusCode)
	}

	// 步骤2: 解析JSON响应
	var apiResp ImageAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return ImageMeta{}, fmt.Errorf("failed to parse API response: %w", err)
	}

	// 步骤3: 验证响应
	if apiResp.Code != "200" {
		return ImageMeta{}, fmt.Errorf("API error code: %s", apiResp.Code)
	}

	if apiResp.ImgURL == "" {
		return ImageMeta{}, fmt.Errorf("empty image URL in API response")
	}

	f.logger.Debug("获取到图片URL",
		zap.String("url", apiResp.ImgURL),
		zap.String("width", apiResp.Width),
		zap.String("height", apiResp.Height))

	// 步骤4: 下载实际图片
	imgReq, err := http.NewRequestWithContext(ctx, "GET", apiResp.ImgURL, nil)
	if err != nil {
		return ImageMeta{}, fmt.Errorf("create image request failed: %w", err)
	}

	imgResp, err := f.client.Do(imgReq)
	if err != nil {
		return ImageMeta{}, fmt.Errorf("download image failed: %w", err)
	}
	defer imgResp.Body.Close()

	if imgResp.StatusCode != http.StatusOK {
		return ImageMeta{}, fmt.Errorf("image download status code: %d", imgResp.StatusCode)
	}

	// 读取图片数据
	imageData, err := io.ReadAll(imgResp.Body)
	if err != nil {
		return ImageMeta{}, fmt.Errorf("read image data failed: %w", err)
	}

	// 步骤5: 处理图片（解码、调整尺寸、重新编码）
	processedData, err := f.processImage(imageData)
	if err != nil {
		return ImageMeta{}, fmt.Errorf("process image failed: %w", err)
	}

	// 步骤6: 生成唯一ID（使用内容hash）
	hash := sha256.Sum256(processedData)
	imageID := fmt.Sprintf("%x", hash[:16])

	return ImageMeta{
		ID:   imageID,
		Data: processedData,
		URL:  apiResp.ImgURL, // 保存原始图片URL用于日志
	}, nil
}

// processImage 处理图片：调整尺寸并转换为PNG
func (f *ExternalImageFetcher) processImage(data []byte) ([]byte, error) {
	// 解码图片
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode image failed: %w", err)
	}

	f.logger.Debug("图片解码成功", zap.String("format", format))

	// 如果尺寸已经匹配，直接返回
	bounds := img.Bounds()
	if bounds.Dx() == f.targetWidth && bounds.Dy() == f.targetHeight {
		// 转换为PNG格式（统一格式）
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			return nil, fmt.Errorf("encode to PNG failed: %w", err)
		}
		return buf.Bytes(), nil
	}

	// 调整尺寸（使用简单的缩放算法）
	resizedImg := f.resizeImage(img, f.targetWidth, f.targetHeight)

	// 重新编码为PNG
	var buf bytes.Buffer
	if err := png.Encode(&buf, resizedImg); err != nil {
		return nil, fmt.Errorf("encode resized image failed: %w", err)
	}

	return buf.Bytes(), nil
}

// resizeImage 调整图片尺寸（简单裁剪或缩放）
func (f *ExternalImageFetcher) resizeImage(src image.Image, targetWidth, targetHeight int) image.Image {
	bounds := src.Bounds()
	srcWidth := bounds.Dx()
	srcHeight := bounds.Dy()

	// 计算缩放比例，保持宽高比并裁剪
	scaleX := float64(targetWidth) / float64(srcWidth)
	scaleY := float64(targetHeight) / float64(srcHeight)
	scale := scaleX
	if scaleY > scaleX {
		scale = scaleY
	}

	// 创建新图片
	dst := image.NewRGBA(image.Rect(0, 0, targetWidth, targetHeight))

	// 计算居中裁剪位置
	scaledWidth := int(float64(srcWidth) * scale)
	scaledHeight := int(float64(srcHeight) * scale)
	offsetX := (scaledWidth - targetWidth) / 2
	offsetY := (scaledHeight - targetHeight) / 2

	// 简单的最近邻采样（可替换为更复杂的算法）
	for y := 0; y < targetHeight; y++ {
		for x := 0; x < targetWidth; x++ {
			srcX := int(float64(x+offsetX)/scale) + bounds.Min.X
			srcY := int(float64(y+offsetY)/scale) + bounds.Min.Y

			// 边界检查
			if srcX >= bounds.Min.X && srcX < bounds.Max.X &&
				srcY >= bounds.Min.Y && srcY < bounds.Max.Y {
				dst.Set(x, y, src.At(srcX, srcY))
			}
		}
	}

	return dst
}

// MockImageFetcher 模拟图片获取器（用于测试或fallback）
type MockImageFetcher struct {
	logger       *zap.Logger
	targetWidth  int
	targetHeight int
}

// NewMockImageFetcher 创建模拟图片获取器
func NewMockImageFetcher(logger *zap.Logger, targetWidth, targetHeight int) *MockImageFetcher {
	return &MockImageFetcher{
		logger:       logger,
		targetWidth:  targetWidth,
		targetHeight: targetHeight,
	}
}

// FetchImages 实现ImageProvider接口，生成测试图片
func (m *MockImageFetcher) FetchImages(ctx context.Context, count int) ([]ImageMeta, error) {
	m.logger.Warn("使用模拟图片获取器（Mock模式）", zap.Int("count", count))

	images := make([]ImageMeta, 0, count)

	// 生成纯色背景图片（与原有逻辑相同，仅用于fallback）
	colors := []struct {
		r, g, b uint8
		name    string
	}{
		{230, 240, 250, "light-blue"},
		{240, 250, 230, "light-green"},
		{250, 240, 230, "light-orange"},
		{245, 235, 255, "light-purple"},
		{255, 245, 235, "light-pink"},
	}

	for i := 0; i < count; i++ {
		colorIdx := i % len(colors)
		bgColor := colors[colorIdx]

		// 创建纯色图片
		img := image.NewRGBA(image.Rect(0, 0, m.targetWidth, m.targetHeight))
		c := color.RGBA{R: bgColor.r, G: bgColor.g, B: bgColor.b, A: 255}
		for y := 0; y < m.targetHeight; y++ {
			for x := 0; x < m.targetWidth; x++ {
				img.SetRGBA(x, y, c)
			}
		}

		// 编码为PNG
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			m.logger.Error("编码模拟图片失败", zap.Error(err))
			continue
		}

		imageID := fmt.Sprintf("mock-%s-%s", bgColor.name, uuid.New().String()[:8])

		images = append(images, ImageMeta{
			ID:   imageID,
			Data: buf.Bytes(),
			URL:  fmt.Sprintf("mock://%s", bgColor.name),
		})
	}

	return images, nil
}

// CustomImageFetcher 自定义图片获取器工厂函数
// 用户可以根据自己的API实现此函数
func CustomImageFetcher(apiConfig ExternalImageAPIConfig, logger *zap.Logger, width, height int) ImageProvider {
	// 如果没有配置API URL，使用模拟获取器
	if apiConfig.URL == "" {
		logger.Warn("未配置外部图片API，使用模拟图片")
		return NewMockImageFetcher(logger, width, height)
	}

	// 否则使用真实的外部API获取器
	return NewExternalImageFetcher(apiConfig, logger, width, height)
}
