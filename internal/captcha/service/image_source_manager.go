package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	sharedconfig "github.com/Pupervemon/risk-engine/internal/shared/config"
	"go.uber.org/zap"
)

var ErrImagePoolDisabled = errors.New("captcha image pool is disabled")

// ImageSourceRefreshError 表示候选配置已经通过校验，
// 但在真正刷新图片池、向上游拉取图片时失败了。
//
// 这个错误类型把“配置合法”与“上游可用”区分开，便于调用方判断问题到底出在配置，
// 还是出在外部图片源本身。
type ImageSourceRefreshError struct {
	Err error
}

func (e *ImageSourceRefreshError) Error() string {
	if e == nil || e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

func (e *ImageSourceRefreshError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// ImageSourcePersistenceError 表示运行时配置已经校验通过，
// 但在持久化到 Redis 或其他存储时失败了。
//
// 这类错误通常意味着“本次修改没有成功落盘”，即使内存里已经有候选配置，
// 调用方也应该把它当成一次失败的更新。
type ImageSourcePersistenceError struct {
	Err error
}

func (e *ImageSourcePersistenceError) Error() string {
	if e == nil || e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

func (e *ImageSourcePersistenceError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// ImageSourcePatch 表示对运行时图片源配置的局部更新。
//
// 这些字段使用指针是为了区分两种情况：
// 1. 字段没有传入，表示保持原值不变；
// 2. 字段传入了具体值，表示用新值覆盖旧值。
type ImageSourcePatch struct {
	URL                *string
	APIKey             *string
	TimeoutSeconds     *int
	RateLimitPerMinute *int
	RetryCount         *int
}

// ImageSourceRuntimeConfig 是运行时实际生效的图片源配置。
//
// 它是可变配置的内部表示，manager 会把这份配置交给当前活跃的 ImageProvider 使用。
type ImageSourceRuntimeConfig struct {
	URL                string
	APIKey             string
	TimeoutSeconds     int
	RateLimitPerMinute int
	RetryCount         int
}

// ImageSourceConfigView 是运行时配置对外展示的安全视图。
//
// 这里不会直接暴露 APIKey 明文，只会告诉调用方它是否已配置，避免敏感信息泄露。
type ImageSourceConfigView struct {
	URL                string `json:"url"`
	APIKeyConfigured   bool   `json:"apiKeyConfigured"`
	TimeoutSeconds     int    `json:"timeoutSeconds"`
	RateLimitPerMinute int    `json:"rateLimitPerMinute"`
	RetryCount         int    `json:"retryCount"`
}

// ImageSourceStatus 描述当前运行时图片源管理器的完整状态快照。
//
// 除了当前配置之外，还包含版本号、最近一次校验结果、最近一次刷新结果、
// 以及图片池大小等运行信息，方便运维和接口排障。
type ImageSourceStatus struct {
	Enabled             bool                  `json:"enabled"`
	Version             int64                 `json:"version"`
	Config              ImageSourceConfigView `json:"config"`
	UpdatedAt           string                `json:"updatedAt,omitempty"`
	LastValidatedAt     string                `json:"lastValidatedAt,omitempty"`
	LastValidationError string                `json:"lastValidationError,omitempty"`
	LastRefreshedAt     string                `json:"lastRefreshedAt,omitempty"`
	LastRefreshError    string                `json:"lastRefreshError,omitempty"`
	PoolSize            int                   `json:"poolSize"`
	PoolImageCount      int64                 `json:"poolImageCount"`
	ActiveGeneration    string                `json:"activeGeneration,omitempty"`
	GenerationCount     int64                 `json:"generationCount"`
}

// ImageSourceValidationResult 是配置校验接口的返回结果。
//
// 它只关心“这份候选配置是否可以工作”，因此会返回经过清理后的配置视图和校验时间。
type ImageSourceValidationResult struct {
	Config      ImageSourceConfigView `json:"config"`
	ValidatedAt string                `json:"validatedAt,omitempty"`
}

// RuntimeImageSourceManager 负责持有并管理当前正在使用的图片源配置和 provider。
//
// 它既保存当前生效的配置，也保存用于读取图片的 provider，
// 并通过内部锁保护这些状态，保证并发读取和热更新时的一致性。
type RuntimeImageSourceManager struct {
	logger          *zap.Logger
	width           int
	height          int
	providerFactory RuntimeImageProviderFactory

	mu                  sync.RWMutex
	config              ImageSourceRuntimeConfig
	provider            ImageProvider
	version             int64
	updatedAt           time.Time
	lastValidatedAt     time.Time
	lastValidationError string
	lastRefreshedAt     time.Time
	lastRefreshError    string
}

// NewRuntimeImageSourceManager 创建一个支持运行时切换的图片源管理器。
//
// 初始化时会先规范化并校验输入配置，再基于配置构造初始 provider，
// 这样 manager 一创建出来就已经可以直接服务请求。
func NewRuntimeImageSourceManager(initial ImageSourceRuntimeConfig, logger *zap.Logger, width, height int, providerFactory RuntimeImageProviderFactory) (*RuntimeImageSourceManager, error) {
	// 允许调用方不传 logger，避免后续日志调用空指针。
	if logger == nil {
		logger = zap.NewNop()
	}
	if providerFactory == nil {
		providerFactory = NewExternalImageProviderFactory(logger, width, height)
	}

	// 先做清理，去掉 URL / APIKey 两侧空白，减少配置输入错误带来的干扰。
	initial = normalizeImageSourceRuntimeConfig(initial)
	// 基于初始配置构造 provider；如果配置本身不合法，这里会直接返回错误。
	provider, err := providerFactory.BuildRuntimeProvider(initial)
	if err != nil {
		return nil, err
	}

	// 新建 manager 时版本从 1 开始，表示“已经有一份有效运行态配置”。
	// updatedAt 记录首次可用时间，便于状态接口和排障使用。
	now := time.Now()
	return &RuntimeImageSourceManager{
		logger:          logger,
		width:           width,
		height:          height,
		providerFactory: providerFactory,
		config:          initial,
		provider:        provider,
		version:         1,
		updatedAt:       now,
	}, nil
}

// FetchImages 将请求直接委托给当前活跃的 provider。
//
// manager 本身不负责生成图片，它只是持有并切换真正干活的 provider。
func (m *RuntimeImageSourceManager) FetchImages(ctx context.Context, count int) ([]ImageMeta, error) {
	// 先读取当前 provider，再在锁外执行 FetchImages，减少持锁时间。
	// 这样可以避免图片拉取过程阻塞其他状态读取或配置更新。
	provider := m.activeProvider()
	if provider == nil {
		return nil, fmt.Errorf("image provider is not configured")
	}

	return provider.FetchImages(ctx, count)
}

// BuildCandidateConfig 先把局部 patch 合并到当前配置上，再对合并后的结果做规范化和校验。
//
// 这一步的目标不是立即生效，而是先构造一个“候选配置”：
// - 只覆盖 patch 中真正传入的字段；
// - 保留原来没有修改的字段；
// - 统一做 trim 和合法性检查。
func (m *RuntimeImageSourceManager) BuildCandidateConfig(patch ImageSourcePatch) (ImageSourceRuntimeConfig, error) {
	// 先读出当前配置作为基线，再在其上应用 patch。
	m.mu.RLock()
	candidate := m.config
	m.mu.RUnlock()

	// 只有当指针字段非 nil 时，才说明调用方真的想修改这个字段。
	if patch.URL != nil {
		candidate.URL = strings.TrimSpace(*patch.URL)
	}
	if patch.APIKey != nil {
		candidate.APIKey = strings.TrimSpace(*patch.APIKey)
	}
	if patch.TimeoutSeconds != nil {
		candidate.TimeoutSeconds = *patch.TimeoutSeconds
	}
	if patch.RateLimitPerMinute != nil {
		candidate.RateLimitPerMinute = *patch.RateLimitPerMinute
	}
	if patch.RetryCount != nil {
		candidate.RetryCount = *patch.RetryCount
	}

	// 合并完后再统一做清理和校验。
	candidate = normalizeImageSourceRuntimeConfig(candidate)
	if err := validateImageSourceRuntimeConfig(candidate); err != nil {
		return ImageSourceRuntimeConfig{}, err
	}

	return candidate, nil
}

// ValidateConfig 验证候选配置是否真的能够从上游拉到图片并完成基本处理。
//
// 这里不只是做参数合法性检查，还会构造 provider 并尝试拉取 1 张图片，
// 用来确认这份配置不仅“看起来正确”，而且“实际可用”。
func (m *RuntimeImageSourceManager) ValidateConfig(ctx context.Context, candidate ImageSourceRuntimeConfig) (ImageProvider, error) {
	// 先尝试基于候选配置构造 provider。
	provider, err := m.buildRuntimeProvider(candidate)
	if err == nil {
		var images []ImageMeta
		// 再做一次真实拉取，避免配置虽然合法但上游实际上不可用。
		images, err = provider.FetchImages(ctx, 1)
		if err == nil && len(images) == 0 {
			err = fmt.Errorf("validation fetched zero images")
		}
	}

	m.recordValidationResult(err)
	if err != nil {
		return nil, err
	}

	return provider, nil
}

// ApplyConfig 将已经验证通过的配置和 provider 切换为当前生效状态。
//
// 这个方法假设传入的 candidate 和 provider 都已经过校验，因此它只负责写入状态，
// 不再重复做验证逻辑。
func (m *RuntimeImageSourceManager) ApplyConfig(candidate ImageSourceRuntimeConfig, provider ImageProvider) {
	// 写操作需要独占锁，避免和 FetchImages / Status / Validation 并发读写冲突。
	m.mu.Lock()
	defer m.mu.Unlock()

	// 更新当前配置、切换 provider，并递增版本号。
	// 版本号用于对外表达“运行时配置发生过几次生效变化”。
	m.config = candidate
	m.provider = provider
	m.version++
	m.updatedAt = time.Now()
}

// RestoreConfig 将持久化配置恢复到 manager 中，但不递增版本号。
//
// 这个方法用于服务启动或重建 manager 时，从 Redis 之类的持久化存储里恢复上一次的配置。
// 因为恢复不是一次“新的在线修改”，所以版本号保持不变更合理。
func (m *RuntimeImageSourceManager) RestoreConfig(candidate ImageSourceRuntimeConfig, provider ImageProvider) {
	// 同样使用独占锁，保证恢复过程对外是原子的。
	m.mu.Lock()
	defer m.mu.Unlock()

	// 只更新配置、provider 和更新时间，不改变版本号。
	m.config = candidate
	m.provider = provider
	m.updatedAt = time.Now()
}

// RecordRefreshResult 记录最近一次显式刷新或定时刷新的结果。
//
// 这个方法只记录结果，不负责触发刷新动作本身；它的目的是把刷新时间和错误信息
// 暴露到状态接口里，方便排查“上游是否曾经刷新失败”。
func (m *RuntimeImageSourceManager) RecordRefreshResult(err error) {
	// 刷新状态属于可并发读取的运行态信息，使用独占锁写入。
	m.mu.Lock()
	defer m.mu.Unlock()

	// 不管成功失败，都记录本次刷新发生的时间。
	m.lastRefreshedAt = time.Now()
	if err != nil {
		m.lastRefreshError = err.Error()
		return
	}

	m.lastRefreshError = ""
}

// ValidationResult 返回候选配置的对外校验结果视图。
//
// 它会把内部记录的最后一次校验时间带上，但不会暴露敏感字段。
func (m *RuntimeImageSourceManager) ValidationResult(candidate ImageSourceRuntimeConfig) ImageSourceValidationResult {
	// 只读状态，使用读锁即可。
	m.mu.RLock()
	defer m.mu.RUnlock()

	return ImageSourceValidationResult{
		Config:      candidate.publicView(),
		ValidatedAt: formatOptionalTime(m.lastValidatedAt),
	}
}

// Status 返回当前运行时状态快照。
//
// 该快照给管理接口使用，能够看到当前配置、版本、最近一次校验/刷新情况，
// 以及图片池规模等信息。
func (m *RuntimeImageSourceManager) Status(poolSize int, poolSnapshot ImagePoolSnapshot) ImageSourceStatus {
	// 状态读取走读锁，允许多个请求并发查看。
	m.mu.RLock()
	defer m.mu.RUnlock()

	return ImageSourceStatus{
		Enabled:             true,
		Version:             m.version,
		Config:              m.config.publicView(),
		UpdatedAt:           formatOptionalTime(m.updatedAt),
		LastValidatedAt:     formatOptionalTime(m.lastValidatedAt),
		LastValidationError: m.lastValidationError,
		LastRefreshedAt:     formatOptionalTime(m.lastRefreshedAt),
		LastRefreshError:    m.lastRefreshError,
		PoolSize:            poolSize,
		PoolImageCount:      poolSnapshot.ImageCount,
		ActiveGeneration:    poolSnapshot.ActiveGeneration,
		GenerationCount:     poolSnapshot.GenerationCount,
	}
}

// activeProvider 读取当前生效的 provider。
//
// 单独拆成这个方法，是为了在 FetchImages 等路径里只短暂持有读锁，然后把 provider
// 拿出来在锁外使用，减少锁竞争。
func (m *RuntimeImageSourceManager) activeProvider() ImageProvider {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.provider
}

func (m *RuntimeImageSourceManager) buildRuntimeProvider(cfg ImageSourceRuntimeConfig) (ImageProvider, error) {
	if m == nil || m.providerFactory == nil {
		return nil, fmt.Errorf("image provider factory is not configured")
	}
	return m.providerFactory.BuildRuntimeProvider(cfg)
}

// recordValidationResult 记录最近一次配置校验的时间和结果。
//
// 成功时会清空错误信息，失败时会保存错误字符串，方便 Status 接口展示。
func (m *RuntimeImageSourceManager) recordValidationResult(err error) {
	// 校验结果属于共享状态，使用独占锁写入。
	m.mu.Lock()
	defer m.mu.Unlock()

	// 无论成功还是失败，都更新最后一次校验时间。
	m.lastValidatedAt = time.Now()
	if err != nil {
		m.lastValidationError = err.Error()
		return
	}

	m.lastValidationError = ""
}

// normalizeImageSourceRuntimeConfig 对运行时配置做轻量级清理。
//
// 目前只去掉 URL 和 APIKey 两侧空白，避免用户在输入时带入多余换行或空格。
func normalizeImageSourceRuntimeConfig(cfg ImageSourceRuntimeConfig) ImageSourceRuntimeConfig {
	cfg.URL = strings.TrimSpace(cfg.URL)
	cfg.APIKey = strings.TrimSpace(cfg.APIKey)
	return cfg
}

// validateImageSourceRuntimeConfig 检查运行时配置的基础合法性。
//
// 这个函数只验证“配置本身是否合理”，不验证上游服务是否真的可用：
// - URL 必须存在且是绝对地址；
// - 超时时间必须大于 0；
// - 限流必须大于 0；
// - 重试次数不能为负数。
func validateImageSourceRuntimeConfig(cfg ImageSourceRuntimeConfig) error {
	if cfg.URL == "" {
		return fmt.Errorf("image source url is required")
	}

	parsedURL, err := url.ParseRequestURI(cfg.URL)
	if err != nil || !parsedURL.IsAbs() {
		return fmt.Errorf("image source url must be an absolute URL")
	}

	if cfg.TimeoutSeconds <= 0 {
		return fmt.Errorf("timeoutSeconds must be greater than 0")
	}
	if cfg.RateLimitPerMinute <= 0 {
		return fmt.Errorf("rateLimitPerMinute must be greater than 0")
	}
	if cfg.RetryCount < 0 {
		return fmt.Errorf("retryCount cannot be negative")
	}

	return nil
}

// fetcherConfig 将运行时配置转换成底层 ExternalImageAPIConfig。
//
// 这个转换层把“服务内部的运行时表示”与“真正调用外部图片源的配置格式”隔离开，
// 便于以后扩展或调整内部结构。
func (cfg ImageSourceRuntimeConfig) fetcherConfig() ExternalImageAPIConfig {
	return ExternalImageAPIConfig{
		URL:                cfg.URL,
		APIKey:             cfg.APIKey,
		Timeout:            time.Duration(cfg.TimeoutSeconds) * time.Second,
		RateLimitPerMinute: cfg.RateLimitPerMinute,
		RetryCount:         cfg.RetryCount,
	}
}

// publicView 返回对外可展示的安全视图。
//
// 这里故意不把 APIKey 原文暴露出去，只返回一个布尔值表示是否配置过。
func (cfg ImageSourceRuntimeConfig) publicView() ImageSourceConfigView {
	return ImageSourceConfigView{
		URL:                cfg.URL,
		APIKeyConfigured:   cfg.APIKey != "",
		TimeoutSeconds:     cfg.TimeoutSeconds,
		RateLimitPerMinute: cfg.RateLimitPerMinute,
		RetryCount:         cfg.RetryCount,
	}
}

// imageSourceRuntimeConfigFromShared 将共享配置结构转换成运行时配置结构。
//
// 这样可以把 shared/config 层的配置直接喂给 runtime manager，避免在上层做重复映射。
func imageSourceRuntimeConfigFromShared(cfg sharedconfig.ExternalImageAPIConfig) ImageSourceRuntimeConfig {
	return ImageSourceRuntimeConfig{
		URL:                cfg.URL,
		APIKey:             cfg.APIKey,
		TimeoutSeconds:     cfg.TimeoutSeconds,
		RateLimitPerMinute: cfg.RateLimitPerMinute,
		RetryCount:         cfg.RetryCount,
	}
}

// formatOptionalTime 将可选时间格式化为 RFC3339 字符串。
//
// 零值时间表示“从未发生过”，因此返回空字符串更适合 JSON 的 omitempty 语义。
func formatOptionalTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(time.RFC3339)
}
