package service

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

// AdminAuthService owns only administrator credentials and session tokens.
// Customer registration, grants, promotions, and third-party identity flows
// remain outside this dependency graph.
type AdminAuthService struct {
	userRepo          UserRepository
	refreshTokenCache RefreshTokenCache
	cfg               *config.Config
	settingService    *SettingService
}

func NewAdminAuthService(
	userRepo UserRepository,
	refreshTokenCache RefreshTokenCache,
	cfg *config.Config,
	settingService *SettingService,
) *AdminAuthService {
	return &AdminAuthService{
		userRepo:          userRepo,
		refreshTokenCache: refreshTokenCache,
		cfg:               cfg,
		settingService:    settingService,
	}
}

// RecordSuccessfulLogin records administrator activity without running any
// customer onboarding or identity-binding workflow.
func (s *AdminAuthService) RecordSuccessfulLogin(ctx context.Context, userID int64) {
	if s == nil || s.userRepo == nil || userID <= 0 {
		return
	}
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil || user == nil {
		return
	}
	now := time.Now().UTC()
	user.LastLoginAt = &now
	user.LastActiveAt = &now
	if err := s.userRepo.Update(ctx, user, UserUpdateFields{LastLoginAt: true, LastActiveAt: true}); err != nil {
		logger.LegacyPrintf("service.admin_auth", "failed to record administrator login: user_id=%d err=%v", userID, err)
	}
}
