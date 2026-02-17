package service

import "time"

// API Key status constants
const (
	StatusAPIKeyActive         = "active"
	StatusAPIKeyDisabled       = "disabled"
	StatusAPIKeyQuotaExhausted = "quota_exhausted"
	StatusAPIKeyExpired        = "expired"
)

type APIKey struct {
	ID          int64
	UserID      int64
	Key         string
	Name        string
	GroupID     *int64
	Status      string
	IPWhitelist []string
	IPBlacklist []string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	User        *User
	Group       *Group

	// Quota fields
	Quota     float64    // Quota limit in USD (0 = unlimited)
	QuotaUsed float64    // Used quota amount
	ExpiresAt *time.Time // Expiration time (nil = never expires)

	// Per-period limit fields (Lite)
	DailyLimitUSD   *float64   // Daily spending limit in USD (nil = unlimited)
	WeeklyLimitUSD  *float64   // Weekly spending limit in USD (nil = unlimited)
	MonthlyLimitUSD *float64   // Monthly spending limit in USD (nil = unlimited)
	TotalLimitUSD   *float64   // Total spending limit in USD (nil = unlimited)
	DailyUsageUSD   float64    // Current daily usage in USD
	WeeklyUsageUSD  float64    // Current weekly usage in USD
	MonthlyUsageUSD float64    // Current monthly usage in USD
	TotalUsageUSD   float64    // Total cumulative usage in USD

	// Usage reset timestamps (Lite)
	UsageResetDaily   *time.Time // Next daily usage reset time
	UsageResetWeekly  *time.Time // Next weekly usage reset time
	UsageResetMonthly *time.Time // Next monthly usage reset time
}

func (k *APIKey) IsActive() bool {
	return k.Status == StatusActive
}

// IsExpired checks if the API key has expired
func (k *APIKey) IsExpired() bool {
	if k.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*k.ExpiresAt)
}

// IsQuotaExhausted checks if the API key quota is exhausted
func (k *APIKey) IsQuotaExhausted() bool {
	if k.Quota <= 0 {
		return false // unlimited
	}
	return k.QuotaUsed >= k.Quota
}

// GetQuotaRemaining returns remaining quota (-1 for unlimited)
func (k *APIKey) GetQuotaRemaining() float64 {
	if k.Quota <= 0 {
		return -1 // unlimited
	}
	remaining := k.Quota - k.QuotaUsed
	if remaining < 0 {
		return 0
	}
	return remaining
}

// GetDaysUntilExpiry returns days until expiry (-1 for never expires)
func (k *APIKey) GetDaysUntilExpiry() int {
	if k.ExpiresAt == nil {
		return -1 // never expires
	}
	duration := time.Until(*k.ExpiresAt)
	if duration < 0 {
		return 0
	}
	return int(duration.Hours() / 24)
}

// HasAnyLimit returns true if the API key has any per-period spending limit configured.
func (k *APIKey) HasAnyLimit() bool {
	return (k.DailyLimitUSD != nil && *k.DailyLimitUSD > 0) ||
		(k.WeeklyLimitUSD != nil && *k.WeeklyLimitUSD > 0) ||
		(k.MonthlyLimitUSD != nil && *k.MonthlyLimitUSD > 0) ||
		(k.TotalLimitUSD != nil && *k.TotalLimitUSD > 0)
}

// IsDailyLimitExceeded returns true if daily usage has reached or exceeded the daily limit.
func (k *APIKey) IsDailyLimitExceeded() bool {
	if k.DailyLimitUSD == nil || *k.DailyLimitUSD <= 0 {
		return false
	}
	return k.DailyUsageUSD >= *k.DailyLimitUSD
}

// IsWeeklyLimitExceeded returns true if weekly usage has reached or exceeded the weekly limit.
func (k *APIKey) IsWeeklyLimitExceeded() bool {
	if k.WeeklyLimitUSD == nil || *k.WeeklyLimitUSD <= 0 {
		return false
	}
	return k.WeeklyUsageUSD >= *k.WeeklyLimitUSD
}

// IsMonthlyLimitExceeded returns true if monthly usage has reached or exceeded the monthly limit.
func (k *APIKey) IsMonthlyLimitExceeded() bool {
	if k.MonthlyLimitUSD == nil || *k.MonthlyLimitUSD <= 0 {
		return false
	}
	return k.MonthlyUsageUSD >= *k.MonthlyLimitUSD
}

// IsTotalLimitExceeded returns true if total cumulative usage has reached or exceeded the total limit.
func (k *APIKey) IsTotalLimitExceeded() bool {
	if k.TotalLimitUSD == nil || *k.TotalLimitUSD <= 0 {
		return false
	}
	return k.TotalUsageUSD >= *k.TotalLimitUSD
}
