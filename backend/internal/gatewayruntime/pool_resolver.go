package gatewayruntime

import (
	"context"
	"fmt"

	"github.com/Wei-Shaw/sub2api/gatewaycore"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type groupLoader func(context.Context, int64) (*service.Group, error)

// PoolResolver loads one authoritative technical group and binds it to the
// request context so the scheduler can reuse it without a second repository
// lookup.
type PoolResolver struct {
	loadGroup groupLoader
}

func NewPoolResolver(repo service.GroupRepository) *PoolResolver {
	return &PoolResolver{loadGroup: repo.GetByIDLite}
}

func (r *PoolResolver) ResolvePool(ctx context.Context, poolID int64) (context.Context, gatewaycore.Pool, error) {
	if r == nil || r.loadGroup == nil {
		return nil, gatewaycore.Pool{}, fmt.Errorf("%w: group loader is required", gatewaycore.ErrInvalidPool)
	}
	group, err := r.loadGroup(ctx, poolID)
	if err != nil {
		return nil, gatewaycore.Pool{}, err
	}
	if !service.IsGroupContextValid(group) || group.ID != poolID {
		return nil, gatewaycore.Pool{}, fmt.Errorf("%w: repository returned an untrusted group for pool %d", gatewaycore.ErrInvalidPool, poolID)
	}

	resolvedCtx := context.WithValue(ctx, ctxkey.Group, group)
	return resolvedCtx, gatewaycore.Pool{
		ID:        group.ID,
		Name:      group.Name,
		Platform:  group.Platform,
		Active:    group.IsActive(),
		AllowLive: group.AllowLive,
	}, nil
}

var _ gatewaycore.PoolResolver = (*PoolResolver)(nil)
