package service

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"
)

// runtimeImageSourceRestoreTimeout 限制从持久化存储恢复运行时图片源配置的最长等待时间。
//
// 这里使用较短超时，避免 Redis 或其他存储故障拖慢验证码服务启动。
const runtimeImageSourceRestoreTimeout = 3 * time.Second

// runtimeImageSourceBinding 维护一个 CaptchaService 实例对应的运行时图片源管理器和配置存储。
//
// manager 负责在内存中切换图片源；
// store 负责把运行时配置持久化到 Redis，便于服务重启后恢复。
type runtimeImageSourceBinding struct {
	manager *RuntimeImageSourceManager
	store   ImageSourceStore
}

// captchaRuntimeImageSourceBindings 以 CaptchaService 指针为键保存绑定关系。
//
// 使用 sync.Map 的原因是：该绑定可能在服务启动、请求处理或配置恢复等多个并发路径中访问。
var captchaRuntimeImageSourceBindings sync.Map

// EnableRuntimeImageSourceManager 为当前验证码服务实例启用可运行时切换的图片源管理器。
//
// 启用后，图片源不再只依赖静态配置文件，而是可以在服务运行期间切换，
// 并将最新配置持久化，以便重启后恢复。
func (s *CaptchaService) EnableRuntimeImageSourceManager() error {
	// 只有在服务对象、图片池和开关状态都满足条件时，才真正启用。
	// 这里直接返回 nil，是因为“未启用”本身不是错误。
	if s == nil || s.imagePool == nil || !s.useImagePool {
		return nil
	}

	// 如果当前服务已经绑定过 runtime image source manager，则幂等返回。
	if _, ok := captchaRuntimeImageSourceBindings.Load(s); ok {
		return nil
	}

	// 从服务配置中读取图片尺寸；如果配置缺失或非法，则回退到默认值。
	// 这些尺寸会用于构建图片源 provider。
	width := s.cfg.Width
	if width <= 0 {
		width = 320
	}
	height := s.cfg.Height
	if height <= 0 {
		height = 180
	}

	// 创建运行时图片源管理器。
	// 它负责接收新的运行时配置，并驱动 imagePool 使用新的 provider。
	manager, err := NewRuntimeImageSourceManager(
		imageSourceRuntimeConfigFromShared(s.cfg.ExternalImageAPI),
		s.logger,
		width,
		height,
	)
	if err != nil {
		return err
	}

	// 为当前服务构建绑定对象：
	// 1. manager 保存内存态的运行时配置；
	// 2. store 提供持久化层，便于恢复上次配置。
	binding := &runtimeImageSourceBinding{
		manager: manager,
		store:   NewRedisImageSourceStore(s.rdb),
	}

	// 先尝试从持久化存储恢复旧配置，再把结果应用到 manager。
	// 这样可以保证服务重启后尽可能延续上一次的运行时图片源。
	s.restoreRuntimeImageSource(binding)

	// 使用 LoadOrStore 处理并发初始化：如果别的 goroutine 已经完成绑定，
	// 当前调用就复用已有绑定，避免重复创建和覆盖。
	actual, loaded := captchaRuntimeImageSourceBindings.LoadOrStore(s, binding)
	if loaded {
		// 这里可能说明另一个并发初始化先一步完成了绑定。
		// 需要把当前 imagePool 的 provider 指向已存在的 manager，确保调用方使用一致的运行时管理器。
		existing, _ := actual.(*runtimeImageSourceBinding)
		if existing != nil && existing.manager != nil {
			s.imagePool.provider = existing.manager
		}
		return nil
	}

	// 首次绑定成功后，把 imagePool 的 provider 切换到 runtime manager。
	s.imagePool.provider = manager
	// 记录启用状态，便于排查当前使用的图片源地址。
	s.logger.Info("runtime image source manager enabled",
		zap.String("url", manager.Status(s.imagePool.poolSize, 0).Config.URL))
	return nil
}

// restoreRuntimeImageSource 尝试从持久化存储恢复运行时图片源配置。
//
// 恢复流程分三步：
// 1. 从 store 读取上次保存的配置；
// 2. 校验配置是否仍然可用；
// 3. 将有效配置恢复到 manager，并让其使用对应 provider。
func (s *CaptchaService) restoreRuntimeImageSource(binding *runtimeImageSourceBinding) {
	// 任一关键对象为空都不做恢复，避免 panic。
	if s == nil || binding == nil || binding.manager == nil || binding.store == nil {
		return
	}

	// 恢复过程使用独立上下文并设置超时，避免启动阶段卡住。
	ctx, cancel := context.WithTimeout(context.Background(), runtimeImageSourceRestoreTimeout)
	defer cancel()

	// 读取持久化配置。
	cfg, found, err := binding.store.Load(ctx)
	if err != nil {
		// 读取失败时只记录警告，不阻断服务启动。
		s.logger.Warn("failed to load persisted runtime image source config", zap.Error(err))
		return
	}
	// 没有历史配置说明之前未做过运行时切换，直接保持当前文件配置。
	if !found {
		return
	}

	// 将持久化配置转换成可运行的 provider。
	// 如果配置已失效或字段非法，这里会返回错误。
	provider, err := buildRuntimeImageProvider(cfg, s.logger, binding.manager.width, binding.manager.height)
	if err != nil {
		// 配置不合法时，保留现有文件配置，避免因为脏数据影响服务可用性。
		s.logger.Warn("persisted runtime image source config is invalid; keeping file config",
			zap.Error(err),
			zap.String("url", cfg.URL))
		return
	}

	// 恢复配置并更新 provider，这样后续请求就会使用恢复出来的运行时图片源。
	binding.manager.RestoreConfig(cfg, provider)
	s.logger.Info("restored runtime image source config from redis", zap.String("url", cfg.URL))
}

// runtimeImageSourceManager 返回当前服务绑定的运行时图片源管理器。
//
// 这是一个便捷读取方法，调用方无需关心底层绑定结构。
func (s *CaptchaService) runtimeImageSourceManager() *RuntimeImageSourceManager {
	binding := s.runtimeImageSourceBinding()
	if binding == nil {
		return nil
	}
	return binding.manager
}

// runtimeImageSourceStore 返回当前服务绑定的图片源持久化存储。
//
// 主要供运行时配置保存、恢复以及调试路径使用。
func (s *CaptchaService) runtimeImageSourceStore() ImageSourceStore {
	binding := s.runtimeImageSourceBinding()
	if binding == nil {
		return nil
	}
	return binding.store
}

// runtimeImageSourceBinding 从全局绑定表中取出当前 CaptchaService 对应的绑定信息。
//
// 如果服务尚未启用 runtime image source manager，或者当前对象没有建立绑定，则返回 nil。
func (s *CaptchaService) runtimeImageSourceBinding() *runtimeImageSourceBinding {
	// 允许空接收者，避免调用方在边界场景下触发 panic。
	if s == nil {
		return nil
	}

	// 通过服务指针作为键查找绑定。
	value, ok := captchaRuntimeImageSourceBindings.Load(s)
	if !ok {
		return nil
	}

	// 这里只做类型断言；如果存储内容不符合预期，则返回 nil。
	binding, _ := value.(*runtimeImageSourceBinding)
	return binding
}
