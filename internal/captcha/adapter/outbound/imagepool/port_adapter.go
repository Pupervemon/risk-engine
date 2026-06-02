package imagepool

import (
	"context"
	"fmt"
	"time"

	appports "github.com/Pupervemon/risk-engine/internal/captcha/application/ports"
	"github.com/Pupervemon/risk-engine/internal/captcha/domain"
)

// PortAdapter 将 RedisImagePool 适配为应用层图像池端口。
type PortAdapter struct {
	pool *RedisImagePool
}

var _ appports.ManagedBackgroundImagePool = (*PortAdapter)(nil)

// NewPortAdapter 将 RedisImagePool 包装为可管理后台刷新的图像池端口。
// 当 pool 为空时返回 nil，方便启动阶段跳过可选装配。
func NewPortAdapter(pool *RedisImagePool) *PortAdapter {
	if pool == nil {
		return nil
	}
	return &PortAdapter{pool: pool}
}

// Random 从底层池中返回一张随机图片。
// 当适配器未初始化时返回配置错误。
func (a *PortAdapter) Random(ctx context.Context) ([]byte, error) {
	if a == nil || a.pool == nil {
		return nil, fmt.Errorf("captcha image pool is not configured")
	}
	return a.pool.GetRandom(ctx)
}

// Snapshot 返回当前池内容的快照。
// 当适配器未初始化时返回配置错误。
func (a *PortAdapter) Snapshot(ctx context.Context) (domain.ImagePoolSnapshot, error) {
	if a == nil || a.pool == nil {
		return domain.ImagePoolSnapshot{}, fmt.Errorf("captcha image pool is not configured")
	}

	snapshot, err := a.pool.Snapshot(ctx)
	if err != nil {
		return domain.ImagePoolSnapshot{}, err
	}
	return snapshot, nil
}

// Count 返回当前池中图片的数量。
// 当适配器未初始化时返回 0。
func (a *PortAdapter) Count(ctx context.Context) (int64, error) {
	if a == nil || a.pool == nil {
		return 0, fmt.Errorf("captcha image pool is not configured")
	}
	return a.pool.Count(ctx)
}

// Refresh 强制池立即重新加载图片集合。
// 当适配器未初始化时返回配置错误。
func (a *PortAdapter) Refresh(ctx context.Context) error {
	if a == nil || a.pool == nil {
		return fmt.Errorf("captcha image pool is not configured")
	}
	return a.pool.RefreshNow(ctx)
}

// RefreshWithProvider 使用指定的图片提供者刷新池。
// 当适配器未初始化时返回配置错误。
func (a *PortAdapter) RefreshWithProvider(ctx context.Context, provider appports.ImageProvider, meta domain.ImagePoolGenerationMeta) error {
	if a == nil || a.pool == nil {
		return fmt.Errorf("captcha image pool is not configured")
	}
	return a.pool.RefreshWithProvider(ctx, provider, meta)
}

// Start 为包装的池启动后台刷新。
// 空适配器会被忽略，调用方可以安全地做防御式装配。
func (a *PortAdapter) Start(ctx context.Context, interval time.Duration, refreshOnStartup bool) {
	if a != nil && a.pool != nil {
		a.pool.StartRefresh(ctx, interval, refreshOnStartup)
	}
}

// Stop 停止包装池的后台刷新。
// 空适配器会被忽略，调用方可以安全地做防御式装配。
func (a *PortAdapter) Stop() {
	if a != nil && a.pool != nil {
		a.pool.StopRefresh()
	}
}

// SetProvider 替换包装池使用的图片提供者。
// 空适配器会被忽略，调用方可以安全地做防御式装配。
func (a *PortAdapter) SetProvider(provider appports.ImageProvider) {
	if a != nil && a.pool != nil {
		a.pool.SetProvider(provider)
	}
}

// PoolSize 返回当前池跟踪的图片数量。
// 当适配器未初始化时返回 0。
func (a *PortAdapter) PoolSize() int {
	if a == nil || a.pool == nil {
		return 0
	}
	return a.pool.PoolSize()
}
