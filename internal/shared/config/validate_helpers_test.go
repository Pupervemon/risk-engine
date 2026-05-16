package config

import "testing"

func TestValidateRedisConfigRequiresHostPort(t *testing.T) {
	if err := validateRedisConfig(RedisConfig{Addr: "127.0.0.1:6379", PoolSize: 1, DialTimeoutSeconds: 1, ReadTimeoutSeconds: 1, WriteTimeoutSeconds: 1}); err != nil {
		t.Fatalf("expected valid redis addr, got error: %v", err)
	}

	if err := validateRedisConfig(RedisConfig{Addr: "localhost", PoolSize: 1, DialTimeoutSeconds: 1, ReadTimeoutSeconds: 1, WriteTimeoutSeconds: 1}); err == nil {
		t.Fatal("expected missing-port redis addr to be rejected")
	}
}
