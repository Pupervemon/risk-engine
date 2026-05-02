package service

import (
	"testing"
	"time"
)

func TestNextMidnightRefreshTime(t *testing.T) {
	t.Parallel()

	location := time.FixedZone("UTC+8", 8*60*60)
	now := time.Date(2026, time.April, 22, 15, 4, 5, 0, location)

	got := nextMidnightRefreshTime(now)
	want := time.Date(2026, time.April, 23, 0, 0, 0, 0, location)
	if !got.Equal(want) {
		t.Fatalf("nextMidnightRefreshTime(%v) = %v, want %v", now, got, want)
	}
}

func TestNextMidnightRefreshTimeAcrossYearBoundary(t *testing.T) {
	t.Parallel()

	location := time.FixedZone("UTC", 0)
	now := time.Date(2026, time.December, 31, 23, 59, 59, 0, location)

	got := nextMidnightRefreshTime(now)
	want := time.Date(2027, time.January, 1, 0, 0, 0, 0, location)
	if !got.Equal(want) {
		t.Fatalf("nextMidnightRefreshTime(%v) = %v, want %v", now, got, want)
	}
}

func TestNextMidnightRefreshDelay(t *testing.T) {
	t.Parallel()

	location := time.FixedZone("UTC+8", 8*60*60)
	now := time.Date(2026, time.April, 22, 23, 59, 30, 0, location)

	got := nextMidnightRefreshDelay(now)
	want := 30 * time.Second
	if got != want {
		t.Fatalf("nextMidnightRefreshDelay(%v) = %v, want %v", now, got, want)
	}
}
