package service

import (
	"context"
	"testing"
	"time"

	"github.com/Pupervemon/risk-engine/internal/shared/config"
)

type memoryTokenStore struct {
	values map[string]memoryTokenEntry
}

type memoryTokenEntry struct {
	value     []byte
	expiresAt time.Time
}

func newMemoryTokenStore() *memoryTokenStore {
	return &memoryTokenStore{
		values: make(map[string]memoryTokenEntry),
	}
}

func (s *memoryTokenStore) Set(ctx context.Context, key string, value []byte, expiration time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	expiresAt := time.Time{}
	if expiration > 0 {
		expiresAt = time.Now().Add(expiration)
	}

	s.values[key] = memoryTokenEntry{
		value:     append([]byte(nil), value...),
		expiresAt: expiresAt,
	}
	return nil
}

func (s *memoryTokenStore) Exists(ctx context.Context, key string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	entry, ok := s.values[key]
	if !ok {
		return false, nil
	}
	if !entry.expiresAt.IsZero() && time.Now().After(entry.expiresAt) {
		delete(s.values, key)
		return false, nil
	}
	return true, nil
}

func (s *memoryTokenStore) Delete(key string) {
	delete(s.values, key)
}

func TestTokenServiceIssueAndVerify(t *testing.T) {
	store := newMemoryTokenStore()
	svc := newTokenServiceWithStore(&config.TokenConfig{
		TTLSeconds: 60,
		Secret:     "test-secret",
	}, store)

	token, exp, err := svc.IssueToken(context.Background(), "captcha-1")
	if err != nil {
		t.Fatalf("IssueToken returned error: %v", err)
	}
	if token == "" {
		t.Fatal("IssueToken returned an empty token")
	}
	if exp <= time.Now().Unix() {
		t.Fatalf("IssueToken returned an expired exp: %d", exp)
	}

	if exists, err := store.Exists(context.Background(), svc.tokenKey(token)); err != nil || !exists {
		t.Fatalf("token was not stored in memory store, exists=%v err=%v", exists, err)
	}

	valid, reason, gotExp := svc.VerifyToken(context.Background(), token)
	if !valid {
		t.Fatalf("VerifyToken returned invalid token: %s", reason)
	}
	if reason != "OK" {
		t.Fatalf("VerifyToken returned unexpected reason: %s", reason)
	}
	if gotExp != exp {
		t.Fatalf("VerifyToken returned unexpected exp: got %d want %d", gotExp, exp)
	}
}

func TestTokenServiceVerifyFailsWhenTokenMissingInStore(t *testing.T) {
	store := newMemoryTokenStore()
	svc := newTokenServiceWithStore(&config.TokenConfig{
		TTLSeconds: 60,
		Secret:     "test-secret",
	}, store)

	token, _, err := svc.IssueToken(context.Background(), "captcha-2")
	if err != nil {
		t.Fatalf("IssueToken returned error: %v", err)
	}

	store.Delete(svc.tokenKey(token))

	valid, reason, _ := svc.VerifyToken(context.Background(), token)
	if valid {
		t.Fatal("VerifyToken unexpectedly returned valid token")
	}
	if reason != "TOKEN_NOT_FOUND" {
		t.Fatalf("VerifyToken returned unexpected reason: %s", reason)
	}
}
