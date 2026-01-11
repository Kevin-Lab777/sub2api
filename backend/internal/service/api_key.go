package service

import "time"

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

	// [LITE] 限额字段
	DailyLimitUSD   *float64
	WeeklyLimitUSD  *float64
	MonthlyLimitUSD *float64
	TotalLimitUSD   *float64

	// [LITE] 用量追踪字段
	DailyUsageUSD   float64
	WeeklyUsageUSD  float64
	MonthlyUsageUSD float64
	TotalUsageUSD   float64

	// [LITE] 用量重置时间字段
	UsageResetDaily   *time.Time
	UsageResetWeekly  *time.Time
	UsageResetMonthly *time.Time
}

func (k *APIKey) IsActive() bool {
	return k.Status == StatusActive
}

// [LITE] 限额检查辅助方法

// HasDailyLimit 检查是否设置了日限额
func (k *APIKey) HasDailyLimit() bool {
	return k.DailyLimitUSD != nil && *k.DailyLimitUSD > 0
}

// HasWeeklyLimit 检查是否设置了周限额
func (k *APIKey) HasWeeklyLimit() bool {
	return k.WeeklyLimitUSD != nil && *k.WeeklyLimitUSD > 0
}

// HasMonthlyLimit 检查是否设置了月限额
func (k *APIKey) HasMonthlyLimit() bool {
	return k.MonthlyLimitUSD != nil && *k.MonthlyLimitUSD > 0
}

// HasTotalLimit 检查是否设置了总限额
func (k *APIKey) HasTotalLimit() bool {
	return k.TotalLimitUSD != nil && *k.TotalLimitUSD > 0
}

// HasAnyLimit 检查是否设置了任何限额
func (k *APIKey) HasAnyLimit() bool {
	return k.HasDailyLimit() || k.HasWeeklyLimit() || k.HasMonthlyLimit() || k.HasTotalLimit()
}

// IsDailyLimitExceeded 检查日限额是否超限
func (k *APIKey) IsDailyLimitExceeded() bool {
	if !k.HasDailyLimit() {
		return false
	}
	return k.DailyUsageUSD >= *k.DailyLimitUSD
}

// IsWeeklyLimitExceeded 检查周限额是否超限
func (k *APIKey) IsWeeklyLimitExceeded() bool {
	if !k.HasWeeklyLimit() {
		return false
	}
	return k.WeeklyUsageUSD >= *k.WeeklyLimitUSD
}

// IsMonthlyLimitExceeded 检查月限额是否超限
func (k *APIKey) IsMonthlyLimitExceeded() bool {
	if !k.HasMonthlyLimit() {
		return false
	}
	return k.MonthlyUsageUSD >= *k.MonthlyLimitUSD
}

// IsTotalLimitExceeded 检查总限额是否超限
func (k *APIKey) IsTotalLimitExceeded() bool {
	if !k.HasTotalLimit() {
		return false
	}
	return k.TotalUsageUSD >= *k.TotalLimitUSD
}
