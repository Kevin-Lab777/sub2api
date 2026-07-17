package repository

import (
	"context"
	"errors"
	"hash/fnv"
	"sort"
	"strings"
	"sync"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Kevin-Lab777/sub2api/ent"
	"github.com/Kevin-Lab777/sub2api/internal/service"
)

var ErrAuthIdentityOwnershipConflict = errors.New("auth identity ownership conflict")

var repositoryScopedKeyLocks = newScopedKeyLockRegistry()

type scopedKeyLockRegistry struct {
	mu    sync.Mutex
	locks map[string]*scopedKeyLockEntry
}

type scopedKeyLockEntry struct {
	mu   sync.Mutex
	refs int
}

func newScopedKeyLockRegistry() *scopedKeyLockRegistry {
	return &scopedKeyLockRegistry{locks: make(map[string]*scopedKeyLockEntry)}
}

func (r *scopedKeyLockRegistry) lock(keys ...string) func() {
	normalized := normalizeLockKeys(keys...)
	if len(normalized) == 0 {
		return func() {}
	}

	entries := make([]*scopedKeyLockEntry, 0, len(normalized))
	r.mu.Lock()
	for _, key := range normalized {
		entry := r.locks[key]
		if entry == nil {
			entry = &scopedKeyLockEntry{}
			r.locks[key] = entry
		}
		entry.refs++
		entries = append(entries, entry)
	}
	r.mu.Unlock()

	for _, entry := range entries {
		entry.mu.Lock()
	}

	return func() {
		for i := len(entries) - 1; i >= 0; i-- {
			entries[i].mu.Unlock()
		}

		r.mu.Lock()
		defer r.mu.Unlock()
		for i, key := range normalized {
			entry := entries[i]
			entry.refs--
			if entry.refs == 0 {
				delete(r.locks, key)
			}
		}
	}
}

func normalizeLockKeys(keys ...string) []string {
	deduped := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		if key = strings.TrimSpace(key); key != "" {
			deduped[key] = struct{}{}
		}
	}

	normalized := make([]string, 0, len(deduped))
	for key := range deduped {
		normalized = append(normalized, key)
	}
	sort.Strings(normalized)
	return normalized
}

func advisoryLockHash(key string) int64 {
	hasher := fnv.New64a()
	_, _ = hasher.Write([]byte(key))
	return int64(hasher.Sum64())
}

func lockRepositoryScopedKeys(ctx context.Context, client *dbent.Client, _ sqlExecutor, keys ...string) (func(), error) {
	release := repositoryScopedKeyLocks.lock(keys...)
	normalized := normalizeLockKeys(keys...)
	if len(normalized) == 0 || client == nil || client.Driver().Dialect() != dialect.Postgres {
		return release, nil
	}

	for _, key := range normalized {
		var rows entsql.Rows
		if err := client.Driver().Query(ctx, "SELECT pg_advisory_xact_lock($1)", []any{advisoryLockHash(key)}, &rows); err != nil {
			release()
			return nil, err
		}
		_ = rows.Close()
	}
	return release, nil
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
