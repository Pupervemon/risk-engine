package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	stddraw "image/draw"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"io"
	"math"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
	"golang.org/x/time/rate"
)

const (
	// 外部图片接口默认超时时间；如果配置里没有显式指定，就使用这个兜底值。
	defaultExternalImageTimeout = 30 * time.Second
	// 上游 API 响应体允许读取的最大字节数，防止异常大响应拖垮进程内存。
	maxAPIResponseBytes = 2 << 20
	// 实际图片下载响应体允许读取的最大字节数，通常图片会比 API JSON 更大一些。
	maxImageResponseBytes = 20 << 20
)

var (
	// 这些字段名是上游 JSON 中最常见的“图片地址”候选字段。
	// 解析时会按顺序尝试，兼容不同供应商的命名习惯。
	imageURLFieldCandidates = []string{
		"imgurl",
		"imgUrl",
		"imageurl",
		"imageUrl",
		"image_url",
		"url",
		"src",
		"href",
	}
	// 这些字段名是上游 JSON 中最常见的“图片内容”候选字段。
	// 既可能是 base64，也可能是 data URI 或直接可解析的字符串。
	imageDataFieldCandidates = []string{
		"image",
		"img",
		"imageData",
		"image_data",
		"imgData",
		"img_data",
		"base64",
		"imgBase64",
		"img_base64",
		"data",
	}
	// 有些上游 API 会把真正的数据包在 data/result/payload/body/response 等字段里，
	// 因此需要沿着这些字段继续向下递归查找。
	nestedPayloadKeys = []string{"data", "result", "payload", "body", "response"}
)

// externalImagePayload 表示从上游拿到的图片载荷。
// SourceURL 用于保留图片来源，ImageData 则保存最终可直接消费的二进制图片数据。
type externalImagePayload struct {
	SourceURL string
	ImageData []byte
}

// ExternalImageAPIConfig 保存外部图片接口的访问配置。
// 这里既包含连接信息，也包含限流和重试等容错策略。
type ExternalImageAPIConfig struct {
	URL                string
	APIKey             string
	Timeout            time.Duration
	RateLimitPerMinute int
	RetryCount         int
}

// ExternalImageFetcher 负责从上游 API 拉取图片，并把图片统一规范化为内部可用格式。
// 它会处理：
// 1. 请求上游接口或图片地址
// 2. 对响应体做大小限制和内容识别
// 3. 支持 JSON 包装、直接图片、base64、data URI 等多种返回形态
// 4. 将图片统一转成 PNG，并按目标尺寸缩放/裁剪
type ExternalImageFetcher struct {
	config       ExternalImageAPIConfig
	client       *http.Client
	logger       *zap.Logger
	limiter      *rate.Limiter
	targetWidth  int
	targetHeight int
}

// NewExternalImageFetcher 创建一个外部图片抓取器。
// 如果配置中没有设置超时时间，会使用默认超时；如果 logger 为空，则退化为无日志实现。
func NewExternalImageFetcher(config ExternalImageAPIConfig, logger *zap.Logger, targetWidth, targetHeight int) *ExternalImageFetcher {
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = defaultExternalImageTimeout
	}

	if logger == nil {
		logger = zap.NewNop()
	}

	return &ExternalImageFetcher{
		config: config,
		client: &http.Client{
			Timeout: timeout,
		},
		logger:       logger,
		limiter:      newImageRateLimiter(config.RateLimitPerMinute),
		targetWidth:  targetWidth,
		targetHeight: targetHeight,
	}
}

// FetchImages 实现 ImageProvider 接口。
// 这个方法会按 count 次数连续尝试拉取图片，并受限于限流器和上下文取消信号。
func (f *ExternalImageFetcher) FetchImages(ctx context.Context, count int) ([]ImageMeta, error) {
	if count <= 0 {
		return []ImageMeta{}, nil
	}

	f.logger.Info("fetching captcha background images from external API",
		zap.Int("count", count),
		zap.String("api_url", f.config.URL))

	images := make([]ImageMeta, 0, count)
	successCount := 0
	failCount := 0

	// 逐张拉取图片，每次请求前都先经过限流器，避免把上游打爆。
	for i := 0; i < count; i++ {
		if err := f.limiter.Wait(ctx); err != nil {
			f.logger.Warn("rate limiter wait failed", zap.Error(err))
			break
		}

		img, err := f.fetchSingleImageWithRetry(ctx)
		if err != nil {
			f.logger.Warn("failed to fetch image", zap.Int("index", i), zap.Error(err))
			failCount++
			continue
		}

		images = append(images, img)
		successCount++
	}

	if successCount == 0 {
		return nil, fmt.Errorf("failed to fetch any images: %d failures", failCount)
	}

	f.logger.Info("captcha background image fetch completed",
		zap.Int("success", successCount),
		zap.Int("failed", failCount))

	return images, nil
}

// fetchSingleImageWithRetry 负责单张图片的重试逻辑。
// 退避策略采用平方秒级别增长，既简单又能在上游不稳定时降低重试压力。
func (f *ExternalImageFetcher) fetchSingleImageWithRetry(ctx context.Context) (ImageMeta, error) {
	retryCount := f.config.RetryCount
	if retryCount < 0 {
		retryCount = 0
	}

	var lastErr error

	for attempt := 0; attempt <= retryCount; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(attempt*attempt) * time.Second
			f.logger.Debug("retrying external image fetch",
				zap.Int("attempt", attempt),
				zap.Duration("backoff", backoff))

			timer := time.NewTimer(backoff)
			select {
			case <-timer.C:
			case <-ctx.Done():
				timer.Stop()
				return ImageMeta{}, ctx.Err()
			}
		}

		img, err := f.fetchSingleImage(ctx)
		if err == nil {
			return img, nil
		}
		lastErr = err
	}

	return ImageMeta{}, fmt.Errorf("failed after %d retries: %w", retryCount, lastErr)
}

// fetchSingleImage 执行一次完整的图片获取和规范化流程。
// 它只关心“成功拿到一张可用图片”，不负责重试和批量控制。
func (f *ExternalImageFetcher) fetchSingleImage(ctx context.Context) (ImageMeta, error) {
	payload, err := f.fetchUpstreamPayload(ctx)
	if err != nil {
		return ImageMeta{}, err
	}

	processedData, err := f.processImage(payload.ImageData)
	if err != nil {
		return ImageMeta{}, fmt.Errorf("process image failed: %w", err)
	}

	hash := sha256.Sum256(processedData)
	imageID := fmt.Sprintf("%x", hash[:16])

	return ImageMeta{
		ID:   imageID,
		Data: processedData,
		URL:  payload.SourceURL,
	}, nil
}

// fetchUpstreamPayload 尝试从上游接口拿到图片载荷。
// 这里的兼容性最强：既支持 API 直接返回图片，也支持返回 JSON 再间接给出图片地址或图片内容。
func (f *ExternalImageFetcher) fetchUpstreamPayload(ctx context.Context) (externalImagePayload, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.config.URL, nil)
	if err != nil {
		return externalImagePayload{}, fmt.Errorf("create API request failed: %w", err)
	}

	req.Header.Set("Accept", "application/json, text/plain, image/*")
	f.applyAPIAuth(req, true)

	resp, err := f.client.Do(req)
	if err != nil {
		return externalImagePayload{}, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return externalImagePayload{}, fmt.Errorf("API returned status code: %d", resp.StatusCode)
	}

	body, err := readLimitedBody(resp.Body, maxAPIResponseBytes)
	if err != nil {
		return externalImagePayload{}, fmt.Errorf("read API response failed: %w", err)
	}

	contentType := resp.Header.Get("Content-Type")
	if isImageContentType(contentType) || isImageBinary(body) {
		return externalImagePayload{
			SourceURL: f.config.URL,
			ImageData: body,
		}, nil
	}

	payload, err := parseImageAPIResponse(body, f.config.URL)
	if err != nil {
		return externalImagePayload{}, err
	}

	if len(payload.ImageData) > 0 {
		return payload, nil
	}

	imageData, err := f.downloadImage(ctx, payload.SourceURL)
	if err != nil {
		return externalImagePayload{}, err
	}

	payload.ImageData = imageData
	return payload, nil
}

// downloadImage 根据给出的图片 URL 下载真实图片内容。
// 这一步通常发生在上游 JSON 里只返回图片地址的场景。
func (f *ExternalImageFetcher) downloadImage(ctx context.Context, imageURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create image request failed: %w", err)
	}

	req.Header.Set("Accept", "image/*, application/octet-stream")
	f.applyAPIAuth(req, false)

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download image failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("image download status code: %d", resp.StatusCode)
	}

	body, err := readLimitedBody(resp.Body, maxImageResponseBytes)
	if err != nil {
		return nil, fmt.Errorf("read image data failed: %w", err)
	}

	contentType := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Type")))
	switch {
	case isJSONContentType(contentType):
		return nil, fmt.Errorf("image endpoint returned JSON instead of image: %s", bodySnippet(body))
	case strings.HasPrefix(contentType, "text/"):
		return nil, fmt.Errorf("image endpoint returned text instead of image: %s", bodySnippet(body))
	}

	return body, nil
}

// applyAPIAuth 会按需给请求附加认证信息。
// 对于同源请求和上游接口请求会使用 APIKey；如果目标 URL 是外链图片，则默认不强行附带认证头。
func (f *ExternalImageFetcher) applyAPIAuth(req *http.Request, force bool) {
	if f.config.APIKey == "" {
		return
	}

	if !force && !sameOrigin(f.config.URL, req.URL) {
		return
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", f.config.APIKey))
}

// processImage 将任意可解码图片统一处理成 PNG，并在必要时缩放到目标尺寸。
// 这一步是为了让后续的验证码逻辑只面对一种稳定的数据格式。
func (f *ExternalImageFetcher) processImage(data []byte) ([]byte, error) {
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode image failed: %w", err)
	}

	bounds := img.Bounds()
	f.logger.Debug("decoded upstream image",
		zap.String("format", format),
		zap.Int("width", bounds.Dx()),
		zap.Int("height", bounds.Dy()))

	if f.targetWidth > 0 && f.targetHeight > 0 && (bounds.Dx() != f.targetWidth || bounds.Dy() != f.targetHeight) {
		img = f.resizeImage(img, f.targetWidth, f.targetHeight)
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("encode PNG failed: %w", err)
	}

	return buf.Bytes(), nil
}

// resizeImage 先按“覆盖目标尺寸”的原则等比缩放，再从中心裁剪到指定宽高。
// 这样可以尽量保留图片主体内容，避免简单拉伸造成变形。
func (f *ExternalImageFetcher) resizeImage(src image.Image, targetWidth, targetHeight int) image.Image {
	bounds := src.Bounds()
	srcWidth := bounds.Dx()
	srcHeight := bounds.Dy()

	if targetWidth <= 0 || targetHeight <= 0 || srcWidth <= 0 || srcHeight <= 0 {
		return src
	}

	scale := math.Max(float64(targetWidth)/float64(srcWidth), float64(targetHeight)/float64(srcHeight))
	scaledWidth := int(math.Ceil(float64(srcWidth) * scale))
	scaledHeight := int(math.Ceil(float64(srcHeight) * scale))
	if scaledWidth < targetWidth {
		scaledWidth = targetWidth
	}
	if scaledHeight < targetHeight {
		scaledHeight = targetHeight
	}

	scaled := image.NewRGBA(image.Rect(0, 0, scaledWidth, scaledHeight))
	xdraw.CatmullRom.Scale(scaled, scaled.Bounds(), src, bounds, stddraw.Over, nil)

	dst := image.NewRGBA(image.Rect(0, 0, targetWidth, targetHeight))
	cropPoint := image.Point{
		X: maxInt((scaledWidth-targetWidth)/2, 0),
		Y: maxInt((scaledHeight-targetHeight)/2, 0),
	}
	stddraw.Draw(dst, dst.Bounds(), scaled, cropPoint, stddraw.Src)
	return dst
}

// parseImageAPIResponse 解析上游 API 返回体，尽可能提取出图片地址或图片二进制内容。
// 支持的格式包括：
// 1. 纯文本，直接是图片地址或 base64
// 2. JSON 对象，图片可能藏在若干候选字段中
// 3. JSON 包裹的更深层对象，需要递归向下继续查找
func parseImageAPIResponse(body []byte, baseURL string) (externalImagePayload, error) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return externalImagePayload{}, fmt.Errorf("empty API response body")
	}

	if payload, ok, err := parseStringImageSource(string(trimmed), baseURL); ok || err != nil {
		return payload, err
	}

	var decoded any
	if err := json.Unmarshal(trimmed, &decoded); err != nil {
		return externalImagePayload{}, fmt.Errorf("failed to parse API response JSON: %w", err)
	}

	if payload, ok, err := extractImagePayload(decoded, baseURL); ok || err != nil {
		return payload, err
	}

	if reason := extractAPIErrorReason(decoded); reason != "" {
		return externalImagePayload{}, fmt.Errorf("upstream API did not return a usable image: %s", reason)
	}

	return externalImagePayload{}, fmt.Errorf("no image URL or inline image data found in API response")
}

// extractImagePayload 从任意 JSON 值中提取图片载荷。
// 先尝试结构化字段，再退化为启发式遍历，兼容不规则的上游返回。
func extractImagePayload(value any, baseURL string) (externalImagePayload, bool, error) {
	for _, candidate := range collectCandidateValues(value) {
		switch typed := candidate.(type) {
		case string:
			if payload, ok, err := parseStringImageSource(typed, baseURL); ok || err != nil {
				return payload, ok, err
			}
		case map[string]any:
			if payload, ok, err := parseImagePayloadFromMap(typed, baseURL); ok || err != nil {
				return payload, ok, err
			}
		}
	}

	return findImagePayloadByHeuristic(value, baseURL)
}

// collectCandidateValues 会把当前值以及其常见嵌套数据包都收集起来，方便后续统一扫描。
func collectCandidateValues(value any) []any {
	values := make([]any, 0, 4)
	appendCandidateValues(value, &values)
	return values
}

// appendCandidateValues 递归展开常见的包装字段，避免图片字段被包在多层 data/result 中时漏掉。
func appendCandidateValues(value any, out *[]any) {
	*out = append(*out, value)

	container, ok := value.(map[string]any)
	if !ok {
		return
	}

	for _, key := range nestedPayloadKeys {
		child, exists := container[key]
		if !exists {
			continue
		}
		appendCandidateValues(child, out)
	}
}

// parseImagePayloadFromMap 尝试从 map 结构里按候选字段名读取图片地址或图片内容。
// 这里会同时兼容大小写差异和不同字段命名风格。
func parseImagePayloadFromMap(value map[string]any, baseURL string) (externalImagePayload, bool, error) {
	for _, key := range imageURLFieldCandidates {
		raw, ok := lookupMapValueFold(value, key)
		if !ok {
			continue
		}
		str, ok := raw.(string)
		if !ok {
			continue
		}
		if payload, ok, err := parseStringImageSource(str, baseURL); ok || err != nil {
			return payload, ok, err
		}
	}

	for _, key := range imageDataFieldCandidates {
		raw, ok := lookupMapValueFold(value, key)
		if !ok {
			continue
		}
		str, ok := raw.(string)
		if !ok {
			continue
		}
		if payload, ok, err := parseStringImageSource(str, baseURL); ok || err != nil {
			return payload, ok, err
		}
	}

	return externalImagePayload{}, false, nil
}

// findImagePayloadByHeuristic 在结构化解析失败后，递归扫描字符串、数组和对象中的所有值。
// 这是兜底逻辑，用于处理字段名不规范但数据仍然可识别的上游响应。
func findImagePayloadByHeuristic(value any, baseURL string) (externalImagePayload, bool, error) {
	switch typed := value.(type) {
	case string:
		return parseStringImageSource(typed, baseURL)
	case []any:
		for _, item := range typed {
			if payload, ok, err := findImagePayloadByHeuristic(item, baseURL); ok || err != nil {
				return payload, ok, err
			}
		}
	case map[string]any:
		for _, item := range typed {
			if payload, ok, err := findImagePayloadByHeuristic(item, baseURL); ok || err != nil {
				return payload, ok, err
			}
		}
	}

	return externalImagePayload{}, false, nil
}

// parseStringImageSource 会把单个字符串解释成“图片地址”或“内联图片内容”。
// 返回 ok=true 表示这个字符串被识别为图片来源，即使最终仍然可能需要进一步下载。
func parseStringImageSource(raw string, baseURL string) (externalImagePayload, bool, error) {
	candidate := strings.TrimSpace(raw)
	if candidate == "" {
		return externalImagePayload{}, false, nil
	}

	if imageData, ok, err := decodeInlineImage(candidate); ok || err != nil {
		if err != nil {
			return externalImagePayload{}, true, err
		}
		return externalImagePayload{
			SourceURL: inlineSourceURL(baseURL),
			ImageData: imageData,
		}, true, nil
	}

	if !looksLikeImageURL(candidate) {
		return externalImagePayload{}, false, nil
	}

	resolvedURL, err := resolveImageURL(baseURL, candidate)
	if err != nil {
		return externalImagePayload{}, true, err
	}

	return externalImagePayload{SourceURL: resolvedURL}, true, nil
}

// decodeInlineImage 尝试把字符串识别为内联图片数据。
// 支持两种常见形式：data:image/...;base64,... 和直接的 base64 内容。
func decodeInlineImage(raw string) ([]byte, bool, error) {
	candidate := strings.TrimSpace(raw)
	if candidate == "" {
		return nil, false, nil
	}

	lower := strings.ToLower(candidate)
	if strings.HasPrefix(lower, "data:image/") {
		parts := strings.SplitN(candidate, ",", 2)
		if len(parts) != 2 {
			return nil, true, fmt.Errorf("invalid image data URI")
		}
		if !strings.Contains(strings.ToLower(parts[0]), ";base64") {
			return nil, true, fmt.Errorf("unsupported image data URI encoding")
		}

		decoded, err := decodeBase64String(parts[1])
		if err != nil {
			return nil, true, fmt.Errorf("decode data URI failed: %w", err)
		}
		if !isImageBinary(decoded) {
			return nil, true, fmt.Errorf("decoded data URI is not a valid image")
		}
		return decoded, true, nil
	}

	if !looksLikeBase64(candidate) {
		return nil, false, nil
	}

	decoded, err := decodeBase64String(candidate)
	if err != nil {
		return nil, false, nil
	}
	if !isImageBinary(decoded) {
		return nil, false, nil
	}

	return decoded, true, nil
}

// decodeBase64String 会尝试多种 base64 编码变体，提升对不同上游格式的兼容性。
func decodeBase64String(raw string) ([]byte, error) {
	compact := compactBase64(raw)
	decoders := []func(string) ([]byte, error){
		base64.StdEncoding.DecodeString,
		base64.RawStdEncoding.DecodeString,
		base64.URLEncoding.DecodeString,
		base64.RawURLEncoding.DecodeString,
	}

	var lastErr error
	for _, decode := range decoders {
		decoded, err := decode(compact)
		if err == nil {
			return decoded, nil
		}
		lastErr = err
	}

	return nil, lastErr
}

// compactBase64 去掉 base64 字符串中的空白字符，避免换行或分隔空格导致解码失败。
func compactBase64(raw string) string {
	replacer := strings.NewReplacer(" ", "", "\n", "", "\r", "", "\t", "")
	return replacer.Replace(raw)
}

// looksLikeBase64 通过字符集和长度做一个轻量判断，避免把普通短文本误判成 base64。
func looksLikeBase64(raw string) bool {
	compact := compactBase64(raw)
	if len(compact) < 64 {
		return false
	}

	for _, r := range compact {
		switch {
		case r >= 'A' && r <= 'Z':
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '+', r == '/', r == '=', r == '-', r == '_':
		default:
			return false
		}
	}

	return true
}

// looksLikeImageURL 判断字符串是否像一个可访问的图片地址。
// 既支持完整 URL，也支持相对路径和常见图片后缀的路径。
func looksLikeImageURL(raw string) bool {
	candidate := strings.TrimSpace(raw)
	if candidate == "" || strings.ContainsAny(candidate, " \t\r\n") {
		return false
	}

	lower := strings.ToLower(candidate)
	switch {
	case strings.HasPrefix(lower, "http://"),
		strings.HasPrefix(lower, "https://"),
		strings.HasPrefix(lower, "/"),
		strings.HasPrefix(lower, "./"),
		strings.HasPrefix(lower, "../"):
		return true
	}

	parsed, err := url.Parse(candidate)
	if err != nil || parsed.Path == "" {
		return false
	}

	switch strings.ToLower(path.Ext(parsed.Path)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp":
		return true
	default:
		return false
	}
}

// resolveImageURL 将相对图片地址解析为绝对地址，基准是上游 API 的 URL。
func resolveImageURL(baseURL, raw string) (string, error) {
	parsedURL, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", fmt.Errorf("invalid image URL %q: %w", raw, err)
	}
	if parsedURL.IsAbs() {
		return parsedURL.String(), nil
	}

	rootURL, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("invalid API URL %q: %w", baseURL, err)
	}

	return rootURL.ResolveReference(parsedURL).String(), nil
}

// extractAPIErrorReason 从上游 JSON 中提取失败原因，便于日志和排障。
// 它会优先读取 code/status/errno，并结合 message/msg/error 等字段拼接更完整的错误信息。
func extractAPIErrorReason(value any) string {
	root, ok := value.(map[string]any)
	if !ok {
		return ""
	}

	message := extractMessage(root)
	for _, key := range []string{"code", "status", "errno"} {
		raw, exists := lookupMapValueFold(root, key)
		if !exists {
			continue
		}
		if looksLikeSuccessStatus(raw) {
			continue
		}
		if message != "" {
			return fmt.Sprintf("%s=%v, message=%s", key, raw, message)
		}
		return fmt.Sprintf("%s=%v", key, raw)
	}

	if raw, exists := lookupMapValueFold(root, "success"); exists {
		if success, ok := raw.(bool); ok && !success {
			if message != "" {
				return fmt.Sprintf("success=false, message=%s", message)
			}
			return "success=false"
		}
	}

	return ""
}

// extractMessage 从常见消息字段中提取人类可读的错误说明。
func extractMessage(value map[string]any) string {
	for _, key := range []string{"message", "msg", "error", "reason", "detail"} {
		raw, ok := lookupMapValueFold(value, key)
		if !ok {
			continue
		}

		message := strings.TrimSpace(fmt.Sprint(raw))
		if message != "" && message != "<nil>" {
			return message
		}
	}

	return ""
}

// lookupMapValueFold 按大小写不敏感的方式在 map 中查找 key。
// 用于兼容不同上游返回的字段命名风格。
func lookupMapValueFold(value map[string]any, key string) (any, bool) {
	for currentKey, currentValue := range value {
		if strings.EqualFold(currentKey, key) {
			return currentValue, true
		}
	}
	return nil, false
}

// looksLikeSuccessStatus 判断状态字段是否表示成功。
// 这里兼容布尔、数字和字符串三种常见形态。
func looksLikeSuccessStatus(raw any) bool {
	switch typed := raw.(type) {
	case nil:
		return false
	case bool:
		return typed
	case float64:
		return typed == 0 || (typed >= 200 && typed < 300)
	case string:
		value := strings.ToLower(strings.TrimSpace(typed))
		switch value {
		case "", "0", "ok", "success", "true":
			return true
		}
		if value == "200" || value == "201" || value == "204" {
			return true
		}
	}

	return false
}

// readLimitedBody 读取响应体并施加字节上限，避免上游返回超大内容时占用过多内存。
func readLimitedBody(reader io.Reader, limit int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}

	if int64(len(body)) > limit {
		return nil, fmt.Errorf("response body exceeds %d bytes", limit)
	}

	return body, nil
}

// isImageBinary 通过 image.DecodeConfig 轻量判断字节数据是否像一张有效图片。
// 这里只需要判断“能否被识别”，不需要真正完整解码像素。
func isImageBinary(data []byte) bool {
	if len(data) == 0 {
		return false
	}

	_, _, err := image.DecodeConfig(bytes.NewReader(data))
	return err == nil
}

// isImageContentType 判断 Content-Type 是否明确表示图片或通用二进制流。
func isImageContentType(contentType string) bool {
	mediaType := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	return strings.HasPrefix(mediaType, "image/") || mediaType == "application/octet-stream"
}

// isJSONContentType 判断 Content-Type 是否表示 JSON 或 JSON 派生类型。
func isJSONContentType(contentType string) bool {
	mediaType := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	return mediaType == "application/json" || strings.HasSuffix(mediaType, "+json")
}

// bodySnippet 生成一小段可打印的响应体内容，便于日志里快速定位问题。
func bodySnippet(body []byte) string {
	snippet := strings.TrimSpace(string(body))
	if len(snippet) > 200 {
		return snippet[:200] + "..."
	}
	return snippet
}

// sameOrigin 判断两个 URL 是否同源，用于控制认证头是否可以安全复用。
func sameOrigin(rawBaseURL string, targetURL *url.URL) bool {
	if targetURL == nil {
		return false
	}

	baseURL, err := url.Parse(rawBaseURL)
	if err != nil {
		return false
	}

	return strings.EqualFold(baseURL.Scheme, targetURL.Scheme) && strings.EqualFold(baseURL.Host, targetURL.Host)
}

// inlineSourceURL 为内联图片数据构造一个可追踪的伪来源地址。
// 这样后续日志或元数据字段仍然能够表达“这张图来自上游响应体本身”。
func inlineSourceURL(baseURL string) string {
	if baseURL == "" {
		return "inline://upstream"
	}
	return baseURL + "#inline"
}

// newImageRateLimiter 根据每分钟限额创建限流器。
// 当 rateLimitPerMinute <= 0 时，表示不启用限流。
func newImageRateLimiter(rateLimitPerMinute int) *rate.Limiter {
	if rateLimitPerMinute <= 0 {
		return rate.NewLimiter(rate.Inf, 1)
	}

	interval := time.Minute / time.Duration(rateLimitPerMinute)
	return rate.NewLimiter(rate.Every(interval), rateLimitPerMinute)
}

// maxInt 返回两个整数中的较大值。
func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

// MockImageFetcher 是一个本地/测试兜底实现，不依赖外部图片接口。
// 它会生成一些纯色 PNG，便于在接口未配置或上游不可用时继续跑通流程。
type MockImageFetcher struct {
	logger       *zap.Logger
	targetWidth  int
	targetHeight int
}

// NewMockImageFetcher 创建 mock 图片抓取器。
// 只有在外部接口未配置时才通常会使用它。
func NewMockImageFetcher(logger *zap.Logger, targetWidth, targetHeight int) *MockImageFetcher {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &MockImageFetcher{
		logger:       logger,
		targetWidth:  targetWidth,
		targetHeight: targetHeight,
	}
}

// FetchImages 实现 mock 版本的图片提供逻辑。
// 这里直接生成指定尺寸的纯色图片，用于测试和降级场景。
func (m *MockImageFetcher) FetchImages(ctx context.Context, count int) ([]ImageMeta, error) {
	m.logger.Warn("using mock captcha image fetcher", zap.Int("count", count))

	images := make([]ImageMeta, 0, count)

	// 预设几种浅色背景，避免生成的图片过于单一，也方便人工识别。
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

	// 逐张生成，过程中持续检查上下文是否已取消。
	for i := 0; i < count; i++ {
		select {
		case <-ctx.Done():
			return images, ctx.Err()
		default:
		}

		colorIdx := i % len(colors)
		bgColor := colors[colorIdx]

		img := image.NewRGBA(image.Rect(0, 0, m.targetWidth, m.targetHeight))
		fillColor := color.RGBA{R: bgColor.r, G: bgColor.g, B: bgColor.b, A: 255}
		for y := 0; y < m.targetHeight; y++ {
			for x := 0; x < m.targetWidth; x++ {
				img.SetRGBA(x, y, fillColor)
			}
		}

		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			m.logger.Error("failed to encode mock image", zap.Error(err))
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

// CustomImageFetcher 根据配置返回实际的外部抓取器，或者在未配置时返回 mock 实现。
// 这是业务层最常用的入口，调用方无需关心具体实现细节。
func CustomImageFetcher(apiConfig ExternalImageAPIConfig, logger *zap.Logger, width, height int) ImageProvider {
	if apiConfig.URL == "" {
		if logger == nil {
			logger = zap.NewNop()
		}
		logger.Warn("external image API is not configured, falling back to mock images")
		return NewMockImageFetcher(logger, width, height)
	}

	return NewExternalImageFetcher(apiConfig, logger, width, height)
}
