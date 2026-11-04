package certs

import "time"

const day = 24 * time.Hour

// Classify maps notAfter onto a tier and a day count.
// Future certificates use whole days remaining, truncated toward zero.
// A certificate that is not strictly after now is expired. The day count
// for an expired certificate is how many whole days ago notAfter was
// (zero when it lapsed less than 24h ago).
func Classify(notAfter, now time.Time, criticalDays, warnDays int) (tier string, days int) {
	if !notAfter.After(now) {
		return TierExpired, overdueDays(notAfter, now)
	}
	days = int(notAfter.Sub(now) / day)
	switch {
	case days <= criticalDays:
		return TierCritical, days
	case days <= warnDays:
		return TierWarn, days
	default:
		return TierOK, days
	}
}

func overdueDays(notAfter, now time.Time) int {
	d := now.Sub(notAfter)
	if d < 0 {
		return 0
	}
	return int(d / day)
}
