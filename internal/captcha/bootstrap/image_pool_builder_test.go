package bootstrap

import (
	"testing"

	"github.com/Pupervemon/risk-engine/internal/shared/config"
)

func TestNewConfiguredImagePoolDisabled(t *testing.T) {
	t.Parallel()

	pool := NewConfiguredImagePool(nil, &config.CaptchaConfigSpec{}, nil)
	if pool != nil {
		t.Fatal("NewConfiguredImagePool() returned pool when image pool is disabled")
	}
}

func TestNewConfiguredImagePoolUsesDefaultPoolSize(t *testing.T) {
	t.Parallel()

	pool := NewConfiguredImagePool(nil, &config.CaptchaConfigSpec{
		ImagePool: config.ImagePoolConfig{
			Enabled:  true,
			PoolSize: 0,
		},
		ExternalImageAPI: config.ExternalImageAPIConfig{
			URL:                "mock://images",
			TimeoutSeconds:     1,
			RateLimitPerMinute: 1,
		},
	}, nil)

	if pool == nil {
		t.Fatal("NewConfiguredImagePool() returned nil")
	}
	if pool.PoolSize() != 50 {
		t.Fatalf("pool size = %d, want 50", pool.PoolSize())
	}
	if !pool.HasProvider() {
		t.Fatal("provider is nil")
	}
}
