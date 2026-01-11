package service

import (
	"context"
	"log"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

// APIKeyUsageRepository 定义 API Key 用量更新接口
type APIKeyUsageRepository interface {
	// IncrementUsage 增加 API Key 用量
	IncrementUsage(ctx context.Context, id int64, costUSD float64) error
	// ResetDailyUsage 重置日用量
	ResetDailyUsage(ctx context.Context, id int64, resetTime time.Time) error
	// ResetWeeklyUsage 重置周用量
	ResetWeeklyUsage(ctx context.Context, id int64, resetTime time.Time) error
	// ResetMonthlyUsage 重置月用量
	ResetMonthlyUsage(ctx context.Context, id int64, resetTime time.Time) error
	// GetByID 获取 API Key（用于检查用量）
	GetByID(ctx context.Context, id int64) (*APIKey, error)
}

// APIKeyUsageService API Key 用量管理服务
type APIKeyUsageService struct {
	apiKeyRepo APIKeyUsageRepository
	cfg        *config.Config
}

// NewAPIKeyUsageService 创建 API Key 用量管理服务实例
func NewAPIKeyUsageService(
	apiKeyRepo APIKeyUsageRepository,
	cfg *config.Config,
) *APIKeyUsageService {
	return &APIKeyUsageService{
		apiKeyRepo: apiKeyRepo,
		cfg:        cfg,
	}
}

// CheckLimits 检查 API Key 是否超限（在请求前调用）
// 优先级：API Key 限额 > Group 限额
// 如果 API Key 没有设置限额，则使用 Group 限额
func (s *APIKeyUsageService) CheckLimits(ctx context.Context, apiKey *APIKey, group *Group) error {
	// 先检查并重置过期的周期
	if err := s.CheckAndResetExpiredPeriods(ctx, apiKey); err != nil {
		log.Printf("Warning: failed to reset expired periods for API key %d: %v", apiKey.ID, err)
		// 不阻断请求，继续检查
	}

	// 检查 API Key 自身限额（优先级最高）
	if apiKey.HasAnyLimit() {
		return s.checkAPIKeyLimits(apiKey)
	}

	// 如果 API Key 没有限额，检查 Group 限额
	if group != nil {
		return s.checkGroupLimits(apiKey, group)
	}

	// 没有任何限额，允许通过
	return nil
}

// checkAPIKeyLimits 检查 API Key 自身限额
func (s *APIKeyUsageService) checkAPIKeyLimits(apiKey *APIKey) error {
	// 日限额
	if apiKey.IsDailyLimitExceeded() {
		return ErrAPIKeyDailyLimitExceeded
	}

	// 周限额
	if apiKey.IsWeeklyLimitExceeded() {
		return ErrAPIKeyWeeklyLimitExceeded
	}

	// 月限额
	if apiKey.IsMonthlyLimitExceeded() {
		return ErrAPIKeyMonthlyLimitExceeded
	}

	// 总限额
	if apiKey.IsTotalLimitExceeded() {
		return ErrAPIKeyTotalLimitExceeded
	}

	return nil
}

// checkGroupLimits 使用 Group 限额检查 API Key 用量
func (s *APIKeyUsageService) checkGroupLimits(apiKey *APIKey, group *Group) error {
	// 日限额
	if group.HasDailyLimit() && apiKey.DailyUsageUSD >= *group.DailyLimitUSD {
		return ErrAPIKeyDailyLimitExceeded
	}

	// 周限额
	if group.HasWeeklyLimit() && apiKey.WeeklyUsageUSD >= *group.WeeklyLimitUSD {
		return ErrAPIKeyWeeklyLimitExceeded
	}

	// 月限额
	if group.HasMonthlyLimit() && apiKey.MonthlyUsageUSD >= *group.MonthlyLimitUSD {
		return ErrAPIKeyMonthlyLimitExceeded
	}

	return nil
}

// IncrementUsage 增加 API Key 用量
func (s *APIKeyUsageService) IncrementUsage(ctx context.Context, apiKeyID int64, costUSD float64) error {
	if costUSD <= 0 {
		return nil
	}
	return s.apiKeyRepo.IncrementUsage(ctx, apiKeyID, costUSD)
}

// CheckAndResetExpiredPeriods 检查并重置过期的用量周期
func (s *APIKeyUsageService) CheckAndResetExpiredPeriods(ctx context.Context, apiKey *APIKey) error {
	now := timezone.Now()
	tz := s.getTimezone()

	// 检查日重置
	if s.shouldResetDaily(apiKey, now, tz) {
		nextReset := s.getNextDailyReset(now, tz)
		if err := s.apiKeyRepo.ResetDailyUsage(ctx, apiKey.ID, nextReset); err != nil {
			return err
		}
		apiKey.DailyUsageUSD = 0
		apiKey.UsageResetDaily = &nextReset
	}

	// 检查周重置
	if s.shouldResetWeekly(apiKey, now, tz) {
		nextReset := s.getNextWeeklyReset(now, tz)
		if err := s.apiKeyRepo.ResetWeeklyUsage(ctx, apiKey.ID, nextReset); err != nil {
			return err
		}
		apiKey.WeeklyUsageUSD = 0
		apiKey.UsageResetWeekly = &nextReset
	}

	// 检查月重置
	if s.shouldResetMonthly(apiKey, now, tz) {
		nextReset := s.getNextMonthlyReset(now, tz)
		if err := s.apiKeyRepo.ResetMonthlyUsage(ctx, apiKey.ID, nextReset); err != nil {
			return err
		}
		apiKey.MonthlyUsageUSD = 0
		apiKey.UsageResetMonthly = &nextReset
	}

	return nil
}

// ResetUsage 手动重置用量（管理员使用）
func (s *APIKeyUsageService) ResetUsage(ctx context.Context, apiKeyID int64, period string) error {
	now := timezone.Now()
	tz := s.getTimezone()

	switch period {
	case "daily":
		nextReset := s.getNextDailyReset(now, tz)
		return s.apiKeyRepo.ResetDailyUsage(ctx, apiKeyID, nextReset)
	case "weekly":
		nextReset := s.getNextWeeklyReset(now, tz)
		return s.apiKeyRepo.ResetWeeklyUsage(ctx, apiKeyID, nextReset)
	case "monthly":
		nextReset := s.getNextMonthlyReset(now, tz)
		return s.apiKeyRepo.ResetMonthlyUsage(ctx, apiKeyID, nextReset)
	case "all":
		// 重置所有周期
		if err := s.apiKeyRepo.ResetDailyUsage(ctx, apiKeyID, s.getNextDailyReset(now, tz)); err != nil {
			return err
		}
		if err := s.apiKeyRepo.ResetWeeklyUsage(ctx, apiKeyID, s.getNextWeeklyReset(now, tz)); err != nil {
			return err
		}
		return s.apiKeyRepo.ResetMonthlyUsage(ctx, apiKeyID, s.getNextMonthlyReset(now, tz))
	default:
		return infraerrors.BadRequest("INVALID_PERIOD", "period must be one of: daily, weekly, monthly, all")
	}
}

// getTimezone 获取配置的时区
func (s *APIKeyUsageService) getTimezone() *time.Location {
	if s.cfg != nil && s.cfg.Timezone != "" {
		loc, err := time.LoadLocation(s.cfg.Timezone)
		if err == nil {
			return loc
		}
	}
	// 默认使用 Asia/Shanghai
	loc, _ := time.LoadLocation("Asia/Shanghai")
	return loc
}

// shouldResetDaily 检查是否应该重置日用量
func (s *APIKeyUsageService) shouldResetDaily(apiKey *APIKey, now time.Time, _ *time.Location) bool {
	if apiKey.UsageResetDaily == nil {
		return true // 从未设置过，需要初始化
	}
	return now.After(*apiKey.UsageResetDaily)
}

// shouldResetWeekly 检查是否应该重置周用量
func (s *APIKeyUsageService) shouldResetWeekly(apiKey *APIKey, now time.Time, _ *time.Location) bool {
	if apiKey.UsageResetWeekly == nil {
		return true
	}
	return now.After(*apiKey.UsageResetWeekly)
}

// shouldResetMonthly 检查是否应该重置月用量
func (s *APIKeyUsageService) shouldResetMonthly(apiKey *APIKey, now time.Time, _ *time.Location) bool {
	if apiKey.UsageResetMonthly == nil {
		return true
	}
	return now.After(*apiKey.UsageResetMonthly)
}

// getNextDailyReset 获取下一个日重置时间（当天或次日 00:00）
func (s *APIKeyUsageService) getNextDailyReset(now time.Time, tz *time.Location) time.Time {
	localNow := now.In(tz)
	// 明天 00:00
	tomorrow := time.Date(localNow.Year(), localNow.Month(), localNow.Day()+1, 0, 0, 0, 0, tz)
	return tomorrow
}

// getNextWeeklyReset 获取下一个周重置时间（周一 00:00）
func (s *APIKeyUsageService) getNextWeeklyReset(now time.Time, tz *time.Location) time.Time {
	localNow := now.In(tz)
	// 计算到下周一的天数
	weekday := localNow.Weekday()
	daysUntilMonday := (8 - int(weekday)) % 7
	if daysUntilMonday == 0 {
		daysUntilMonday = 7 // 如果今天是周一，则下周一
	}
	nextMonday := time.Date(localNow.Year(), localNow.Month(), localNow.Day()+daysUntilMonday, 0, 0, 0, 0, tz)
	return nextMonday
}

// getNextMonthlyReset 获取下一个月重置时间（下月 1 日 00:00）
func (s *APIKeyUsageService) getNextMonthlyReset(now time.Time, tz *time.Location) time.Time {
	localNow := now.In(tz)
	// 下个月 1 日 00:00
	nextMonth := time.Date(localNow.Year(), localNow.Month()+1, 1, 0, 0, 0, 0, tz)
	return nextMonth
}

// GetUsageStats 获取 API Key 的用量统计
func (s *APIKeyUsageService) GetUsageStats(ctx context.Context, apiKeyID int64) (*APIKeyUsageStats, error) {
	apiKey, err := s.apiKeyRepo.GetByID(ctx, apiKeyID)
	if err != nil {
		return nil, err
	}

	// 确保周期已重置
	if err := s.CheckAndResetExpiredPeriods(ctx, apiKey); err != nil {
		log.Printf("Warning: failed to reset expired periods: %v", err)
	}

	return &APIKeyUsageStats{
		DailyUsageUSD:     apiKey.DailyUsageUSD,
		WeeklyUsageUSD:    apiKey.WeeklyUsageUSD,
		MonthlyUsageUSD:   apiKey.MonthlyUsageUSD,
		TotalUsageUSD:     apiKey.TotalUsageUSD,
		DailyLimitUSD:     apiKey.DailyLimitUSD,
		WeeklyLimitUSD:    apiKey.WeeklyLimitUSD,
		MonthlyLimitUSD:   apiKey.MonthlyLimitUSD,
		TotalLimitUSD:     apiKey.TotalLimitUSD,
		UsageResetDaily:   apiKey.UsageResetDaily,
		UsageResetWeekly:  apiKey.UsageResetWeekly,
		UsageResetMonthly: apiKey.UsageResetMonthly,
	}, nil
}

// APIKeyUsageStats API Key 用量统计
type APIKeyUsageStats struct {
	DailyUsageUSD     float64    `json:"daily_usage_usd"`
	WeeklyUsageUSD    float64    `json:"weekly_usage_usd"`
	MonthlyUsageUSD   float64    `json:"monthly_usage_usd"`
	TotalUsageUSD     float64    `json:"total_usage_usd"`
	DailyLimitUSD     *float64   `json:"daily_limit_usd,omitempty"`
	WeeklyLimitUSD    *float64   `json:"weekly_limit_usd,omitempty"`
	MonthlyLimitUSD   *float64   `json:"monthly_limit_usd,omitempty"`
	TotalLimitUSD     *float64   `json:"total_limit_usd,omitempty"`
	UsageResetDaily   *time.Time `json:"usage_reset_daily,omitempty"`
	UsageResetWeekly  *time.Time `json:"usage_reset_weekly,omitempty"`
	UsageResetMonthly *time.Time `json:"usage_reset_monthly,omitempty"`
}
