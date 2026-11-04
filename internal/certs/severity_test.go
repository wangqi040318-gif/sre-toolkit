package certs

import (
	"testing"
	"time"
)

func TestClassifyBuckets(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	critical, warn := 7, 30
	cases := []struct {
		name string
		at   time.Time
		tier string
		days int
	}{
		{"equal now is expired", now, TierExpired, 0},
		{"one hour ago", now.Add(-time.Hour), TierExpired, 0},
		{"twelve days ago", now.Add(-12 * 24 * time.Hour), TierExpired, 12},
		{"one hour left", now.Add(time.Hour), TierCritical, 0},
		{"exactly 7d", now.Add(7 * 24 * time.Hour), TierCritical, 7},
		{"7d plus an hour still 7", now.Add(7*24*time.Hour + time.Hour), TierCritical, 7},
		{"exactly 8d", now.Add(8 * 24 * time.Hour), TierWarn, 8},
		{"exactly 30d", now.Add(30 * 24 * time.Hour), TierWarn, 30},
		{"exactly 31d", now.Add(31 * 24 * time.Hour), TierOK, 31},
		{"120d", now.Add(120 * 24 * time.Hour), TierOK, 120},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tier, days := Classify(tc.at, now, critical, warn)
			if tier != tc.tier || days != tc.days {
				t.Fatalf("Classify(%s) = %s, %d; want %s, %d", tc.name, tier, days, tc.tier, tc.days)
			}
		})
	}
}

func TestClassifyCustomWindow(t *testing.T) {
	now := time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC)
	tier, days := Classify(now.Add(2*24*time.Hour), now, 1, 10)
	if tier != TierWarn || days != 2 {
		t.Fatalf("got %s %d", tier, days)
	}
	tier, days = Classify(now.Add(24*time.Hour), now, 0, 10)
	if tier != TierWarn || days != 1 {
		t.Fatalf("critical 0: got %s %d", tier, days)
	}
	tier, days = Classify(now.Add(time.Hour), now, 0, 10)
	if tier != TierCritical || days != 0 {
		t.Fatalf("under 24h with critical 0: got %s %d", tier, days)
	}
}
