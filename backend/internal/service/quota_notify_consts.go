package service

import "context"

// [LITE] balance_notify_service.go was removed (depends on EmailService).
// These const identifiers are retained because Account methods (QuotaNotifyConfig,
// GetQuotaNotifyDaily*, etc.) still expose the raw config values to the admin DTO layer.

const (
	thresholdTypeFixed      = "fixed"
	thresholdTypePercentage = "percentage"

	quotaDimDaily  = "daily"
	quotaDimWeekly = "weekly"
	quotaDimTotal  = "total"
)

var _ = thresholdTypePercentage

// BalanceNotifyService is a no-op stub. [LITE] The real service depended on
// EmailService which is removed; call sites keep the nil pointer and check for nil.
type BalanceNotifyService struct{}

func (*BalanceNotifyService) CheckBalanceAfterDeduction(ctx context.Context, user *User, oldBalance, cost float64) {
}

func (*BalanceNotifyService) CheckAccountQuotaAfterIncrement(ctx context.Context, account *Account, cost float64, quotaState any) {
}
