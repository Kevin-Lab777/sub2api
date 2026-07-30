//go:build unit

package handler

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type userHandlerRepoStub struct {
	user       *service.User
	identities []service.UserAuthIdentityRecord
}

func (s *userHandlerRepoStub) Create(context.Context, *service.User) error { return nil }
func (s *userHandlerRepoStub) CreateWithEmailAliasGuard(context.Context, *service.User) error {
	return nil
}
func (s *userHandlerRepoStub) GetByID(context.Context, int64) (*service.User, error) {
	cloned := *s.user
	return &cloned, nil
}
func (s *userHandlerRepoStub) GetByIDIncludeDeleted(ctx context.Context, id int64) (*service.User, error) {
	return s.GetByID(ctx, id)
}
func (s *userHandlerRepoStub) GetByEmail(context.Context, string) (*service.User, error) {
	cloned := *s.user
	return &cloned, nil
}
func (s *userHandlerRepoStub) GetFirstAdmin(context.Context) (*service.User, error) {
	cloned := *s.user
	return &cloned, nil
}
func (s *userHandlerRepoStub) Update(_ context.Context, user *service.User, _ service.UserUpdateFields) error {
	cloned := *user
	s.user = &cloned
	return nil
}
func (s *userHandlerRepoStub) Delete(context.Context, int64) error { return nil }
func (s *userHandlerRepoStub) GetUserAvatar(context.Context, int64) (*service.UserAvatar, error) {
	if s.user == nil || s.user.AvatarURL == "" {
		return nil, nil
	}
	return &service.UserAvatar{
		StorageProvider: s.user.AvatarSource,
		URL:             s.user.AvatarURL,
		ContentType:     s.user.AvatarMIME,
		ByteSize:        s.user.AvatarByteSize,
		SHA256:          s.user.AvatarSHA256,
	}, nil
}
func (s *userHandlerRepoStub) UpsertUserAvatar(context.Context, int64, service.UpsertUserAvatarInput) (*service.UserAvatar, error) {
	return nil, nil
}
func (s *userHandlerRepoStub) DeleteUserAvatar(context.Context, int64) error { return nil }
func (s *userHandlerRepoStub) List(context.Context, pagination.PaginationParams) ([]service.User, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (s *userHandlerRepoStub) ListWithFilters(context.Context, pagination.PaginationParams, service.UserListFilters) ([]service.User, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (s *userHandlerRepoStub) GetLatestUsedAtByUserIDs(context.Context, []int64) (map[int64]*time.Time, error) {
	return map[int64]*time.Time{}, nil
}
func (s *userHandlerRepoStub) GetLatestUsedAtByUserID(context.Context, int64) (*time.Time, error) {
	return nil, nil
}
func (s *userHandlerRepoStub) UpdateUserLastActiveAt(context.Context, int64, time.Time) error {
	return nil
}
func (s *userHandlerRepoStub) UpdateBalance(context.Context, int64, float64) error { return nil }
func (s *userHandlerRepoStub) DeductBalance(context.Context, int64, float64) error { return nil }
func (s *userHandlerRepoStub) AdjustBalance(context.Context, int64, float64) (service.BalanceChange, error) {
	return service.BalanceChange{}, nil
}
func (s *userHandlerRepoStub) SetBalance(context.Context, int64, float64) (service.BalanceChange, error) {
	return service.BalanceChange{}, nil
}
func (s *userHandlerRepoStub) UpdateConcurrency(context.Context, int64, int) error { return nil }
func (s *userHandlerRepoStub) BatchSetConcurrency(context.Context, []int64, int) (int, error) {
	return 0, nil
}
func (s *userHandlerRepoStub) BatchAddConcurrency(context.Context, []int64, int) (int, error) {
	return 0, nil
}
func (s *userHandlerRepoStub) BatchUpdateLimits(context.Context, []int64, *int, *int) (int, error) {
	return 0, nil
}
func (s *userHandlerRepoStub) ExistsByEmail(context.Context, string) (bool, error) {
	return false, nil
}
func (s *userHandlerRepoStub) ExistsByEmailAlias(context.Context, string) (bool, error) {
	return false, nil
}
func (s *userHandlerRepoStub) RemoveGroupFromAllowedGroups(context.Context, int64) (int64, error) {
	return 0, nil
}
func (s *userHandlerRepoStub) AddGroupToAllowedGroups(context.Context, int64, int64) error {
	return nil
}
func (s *userHandlerRepoStub) RemoveGroupFromUserAllowedGroups(context.Context, int64, int64) error {
	return nil
}
func (s *userHandlerRepoStub) ListUserAuthIdentities(context.Context, int64) ([]service.UserAuthIdentityRecord, error) {
	return append([]service.UserAuthIdentityRecord(nil), s.identities...), nil
}
func (s *userHandlerRepoStub) UnbindUserAuthProvider(context.Context, int64, string) error {
	return nil
}
func (s *userHandlerRepoStub) UpdateTotpSecret(context.Context, int64, *string) error { return nil }
func (s *userHandlerRepoStub) EnableTotp(context.Context, int64) error                { return nil }
func (s *userHandlerRepoStub) DisableTotp(context.Context, int64) error               { return nil }

type userHandlerRefreshTokenCacheStub struct {
	revokedUserIDs []int64
}

func (s *userHandlerRefreshTokenCacheStub) StoreRefreshToken(context.Context, string, *service.RefreshTokenData, time.Duration) error {
	return nil
}
func (s *userHandlerRefreshTokenCacheStub) GetRefreshToken(context.Context, string) (*service.RefreshTokenData, error) {
	return nil, service.ErrRefreshTokenNotFound
}
func (s *userHandlerRefreshTokenCacheStub) DeleteRefreshToken(context.Context, string) error {
	return nil
}
func (s *userHandlerRefreshTokenCacheStub) DeleteUserRefreshTokens(_ context.Context, userID int64) error {
	s.revokedUserIDs = append(s.revokedUserIDs, userID)
	return nil
}
func (s *userHandlerRefreshTokenCacheStub) DeleteTokenFamily(context.Context, string) error {
	return nil
}
func (s *userHandlerRefreshTokenCacheStub) AddToUserTokenSet(context.Context, int64, string, time.Duration) error {
	return nil
}
func (s *userHandlerRefreshTokenCacheStub) AddToFamilyTokenSet(context.Context, string, string, time.Duration) error {
	return nil
}
func (s *userHandlerRefreshTokenCacheStub) GetUserTokenHashes(context.Context, int64) ([]string, error) {
	return nil, nil
}
func (s *userHandlerRefreshTokenCacheStub) GetFamilyTokenHashes(context.Context, string) ([]string, error) {
	return nil, nil
}
func (s *userHandlerRefreshTokenCacheStub) IsTokenInFamily(context.Context, string, string) (bool, error) {
	return false, nil
}
