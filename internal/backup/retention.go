package backup

import (
	"fmt"
	"strconv"
	"time"

	"github.com/goforj/env/v2"
)

// RetentionPolicy defines completed backup counts by calendar age.
type RetentionPolicy struct {
	Daily   int
	Weekly  int
	Monthly int
}

// DefaultRetentionPolicy reads the documented retention environment keys.
func DefaultRetentionPolicy() RetentionPolicy {
	return RetentionPolicy{
		Daily:   retentionValue("APP_BACKUP_KEEP_DAILY", 14),
		Weekly:  retentionValue("APP_BACKUP_KEEP_WEEKLY", 4),
		Monthly: retentionValue("APP_BACKUP_KEEP_MONTHLY", 6),
	}
}

// KeepFor returns whether a backup should remain under the calendar-bucket policy.
func (p RetentionPolicy) KeepFor(created, now time.Time, seen map[string]int) bool {
	if created.After(now) {
		return true
	}
	age := now.Sub(created)
	key := created.UTC().Format("2006-01-02")
	limit := p.Daily
	if age >= 30*24*time.Hour {
		key = created.UTC().Format("2006-01")
		limit = p.Monthly
	} else if age >= 7*24*time.Hour {
		year, week := created.UTC().ISOWeek()
		key = fmt.Sprintf("%04d-W%02d", year, week)
		limit = p.Weekly
	}
	if seen[key] >= limit {
		return false
	}
	seen[key]++
	return true
}

// retentionValue keeps negative counts invalid while allowing zero to disable a retention bucket.
func retentionValue(key string, fallback int) int {
	value := env.GetInt(key, strconv.Itoa(fallback))
	if value < 0 {
		return fallback
	}
	return value
}
