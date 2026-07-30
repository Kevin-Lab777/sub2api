package gatewayruntime

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/gatewaycore"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestPoolResolverBindsAuthoritativeGroupForSchedulerReuse(t *testing.T) {
	group := &service.Group{
		ID:        17,
		Name:      "Codex Pool",
		Platform:  service.PlatformOpenAI,
		Status:    service.StatusActive,
		Hydrated:  true,
		AllowLive: true,
	}
	resolver := &PoolResolver{loadGroup: func(_ context.Context, poolID int64) (*service.Group, error) {
		if poolID != group.ID {
			t.Fatalf("poolID = %d, want %d", poolID, group.ID)
		}
		return group, nil
	}}

	resolvedCtx, pool, err := resolver.ResolvePool(context.Background(), group.ID)
	if err != nil {
		t.Fatalf("ResolvePool() error = %v", err)
	}
	if pool.ID != group.ID || pool.Platform != group.Platform || !pool.Active || !pool.AllowLive {
		t.Fatalf("pool = %#v", pool)
	}
	if got := resolvedCtx.Value(ctxkey.Group); got != group {
		t.Fatalf("resolved group = %#v, want original authoritative group", got)
	}
}

func TestPoolResolverRejectsUntrustedGroup(t *testing.T) {
	resolver := &PoolResolver{loadGroup: func(context.Context, int64) (*service.Group, error) {
		return &service.Group{ID: 17, Platform: service.PlatformOpenAI, Status: service.StatusActive}, nil
	}}

	_, _, err := resolver.ResolvePool(context.Background(), 17)
	if !errors.Is(err, gatewaycore.ErrInvalidPool) {
		t.Fatalf("ResolvePool() error = %v, want ErrInvalidPool", err)
	}
}

func TestPoolResolverPreservesRepositoryError(t *testing.T) {
	repositoryErr := errors.New("database unavailable")
	resolver := &PoolResolver{loadGroup: func(context.Context, int64) (*service.Group, error) {
		return nil, repositoryErr
	}}

	_, _, err := resolver.ResolvePool(context.Background(), 17)
	if !errors.Is(err, repositoryErr) {
		t.Fatalf("ResolvePool() error = %v, want repository error", err)
	}
}
