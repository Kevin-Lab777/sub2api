package repository

import (
	"context"
	"errors"
	"time"

	dbent "github.com/Kevin-Lab777/sub2api/ent"
	"github.com/Kevin-Lab777/sub2api/internal/service"
)

var ErrAuthIdentityOwnershipConflict = errors.New("auth identity ownership conflict")

func lockRepositoryScopedKeys(ctx context.Context, client *dbent.Client, executor sqlExecutor, keys ...string) (func(), error) {
	return func() {}, nil
}

func txAwareSQLExecutor(ctx context.Context, fallback sqlExecutor, client *dbent.Client) sqlExecutor {
	return fallback
}

func (r *userRepository) GetUserAvatar(ctx context.Context, userID int64) (*service.UserAvatar, error) {
	return nil, nil
}

func (r *userRepository) UpsertUserAvatar(ctx context.Context, userID int64, input service.UpsertUserAvatarInput) (*service.UserAvatar, error) {
	return &service.UserAvatar{
		StorageProvider: input.StorageProvider,
		StorageKey:      input.StorageKey,
		URL:             input.URL,
		ContentType:     input.ContentType,
		ByteSize:        input.ByteSize,
		SHA256:          input.SHA256,
	}, nil
}

func (r *userRepository) DeleteUserAvatar(ctx context.Context, userID int64) error {
	return nil
}

func (r *userRepository) ListUserAuthIdentities(ctx context.Context, userID int64) ([]service.UserAuthIdentityRecord, error) {
	return nil, nil
}

func (r *userRepository) UnbindUserAuthProvider(ctx context.Context, userID int64, provider string) error {
	return service.ErrIdentityProviderInvalid
}

func (r *userRepository) UpdateUserLastActiveAt(ctx context.Context, userID int64, activeAt time.Time) error {
	client := clientFromContext(ctx, r.client)
	_, err := client.User.UpdateOneID(userID).SetLastActiveAt(activeAt).Save(ctx)
	if err != nil {
		return translatePersistenceError(err, service.ErrUserNotFound, nil)
	}
	return nil
}
