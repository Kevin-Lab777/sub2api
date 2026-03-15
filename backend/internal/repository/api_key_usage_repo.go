package repository

// [LITE:ADD] API Key 用量仓储 - 实现 service.APIKeyUsageRepository 接口

import (
	"context"
	"time"

	dbent "github.com/Kevin-Lab777/sub2api/ent"
	"github.com/Kevin-Lab777/sub2api/ent/apikey"
	"github.com/Kevin-Lab777/sub2api/internal/service"
)

// apiKeyUsageRepository 实现 service.APIKeyUsageRepository 接口
type apiKeyUsageRepository struct {
	client *dbent.Client
}

// ProvideAPIKeyUsageRepository 创建 APIKeyUsageRepository 实例
func ProvideAPIKeyUsageRepository(client *dbent.Client) service.APIKeyUsageRepository {
	return &apiKeyUsageRepository{client: client}
}

func (r *apiKeyUsageRepository) IncrementUsage(ctx context.Context, id int64, costUSD float64) error {
	_, err := r.client.APIKey.UpdateOneID(id).
		AddDailyUsageUsd(costUSD).
		AddWeeklyUsageUsd(costUSD).
		AddMonthlyUsageUsd(costUSD).
		AddTotalUsageUsd(costUSD).
		Save(ctx)
	return err
}

func (r *apiKeyUsageRepository) ResetDailyUsage(ctx context.Context, id int64, resetTime time.Time) error {
	_, err := r.client.APIKey.UpdateOneID(id).
		SetDailyUsageUsd(0).
		SetUsageResetDaily(resetTime).
		Save(ctx)
	return err
}

func (r *apiKeyUsageRepository) ResetWeeklyUsage(ctx context.Context, id int64, resetTime time.Time) error {
	_, err := r.client.APIKey.UpdateOneID(id).
		SetWeeklyUsageUsd(0).
		SetUsageResetWeekly(resetTime).
		Save(ctx)
	return err
}

func (r *apiKeyUsageRepository) ResetMonthlyUsage(ctx context.Context, id int64, resetTime time.Time) error {
	_, err := r.client.APIKey.UpdateOneID(id).
		SetMonthlyUsageUsd(0).
		SetUsageResetMonthly(resetTime).
		Save(ctx)
	return err
}

func (r *apiKeyUsageRepository) GetByID(ctx context.Context, id int64) (*service.APIKey, error) {
	m, err := r.client.APIKey.Query().
		Where(apikey.IDEQ(id)).
		WithUser().
		WithGroup().
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, service.ErrAPIKeyNotFound
		}
		return nil, err
	}
	return apiKeyEntityToService(m), nil
}
