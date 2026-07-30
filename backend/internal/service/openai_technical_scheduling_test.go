package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

type technicalOpenAIStickyCache struct {
	bindings map[string]int64
	readErr  error
	sets     int
	refresh  int
	deletes  int
}

type technicalOpenAIConcurrencyCache struct {
	ConcurrencyCache
	loadMap    map[int64]*AccountLoadInfo
	acquireErr error
}

type technicalOpenAIAccountRepo struct {
	stubOpenAIAccountRepo
	readErrID int64
	readErr   error
}

func (r technicalOpenAIAccountRepo) GetByID(ctx context.Context, accountID int64) (*Account, error) {
	if accountID == r.readErrID {
		return nil, r.readErr
	}
	return r.stubOpenAIAccountRepo.GetByID(ctx, accountID)
}

func (c *technicalOpenAIConcurrencyCache) AcquireAccountSlot(_ context.Context, _ int64, _ int, _ string) (bool, error) {
	if c.acquireErr != nil {
		return false, c.acquireErr
	}
	return true, nil
}

func (c *technicalOpenAIConcurrencyCache) ReleaseAccountSlot(_ context.Context, _ int64, _ string) error {
	return nil
}

func (c *technicalOpenAIConcurrencyCache) GetAccountsLoadBatch(_ context.Context, _ []AccountWithConcurrency) (map[int64]*AccountLoadInfo, error) {
	return c.loadMap, nil
}

func (c *technicalOpenAIStickyCache) GetSessionAccountID(_ context.Context, _ int64, sessionHash string) (int64, error) {
	if c.readErr != nil {
		return 0, c.readErr
	}
	if accountID, ok := c.bindings[sessionHash]; ok {
		return accountID, nil
	}
	return 0, redis.Nil
}

func (c *technicalOpenAIStickyCache) SetSessionAccountID(_ context.Context, _ int64, sessionHash string, accountID int64, _ time.Duration) error {
	c.sets++
	if c.bindings == nil {
		c.bindings = make(map[string]int64)
	}
	c.bindings[sessionHash] = accountID
	return nil
}

func (c *technicalOpenAIStickyCache) RefreshSessionTTL(_ context.Context, _ int64, _ string, _ time.Duration) error {
	c.refresh++
	return nil
}

func (c *technicalOpenAIStickyCache) DeleteSessionAccountID(_ context.Context, _ int64, sessionHash string) error {
	c.deletes++
	delete(c.bindings, sessionHash)
	return nil
}

func TestOpenAISelectTechnicalResponsesAccountPropagatesStickyReadError(t *testing.T) {
	t.Parallel()

	stateErr := errors.New("sticky store unavailable")
	cache := &technicalOpenAIStickyCache{readErr: stateErr}
	svc := &OpenAIGatewayService{
		accountRepo: stubOpenAIAccountRepo{accounts: []Account{{
			ID:          1,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
		}}},
		cache: cache,
	}

	_, err := svc.SelectTechnicalResponsesAccountWithLoadAwareness(context.Background(), nil, "session", "gpt-5.4", nil)
	require.ErrorIs(t, err, stateErr)
	require.Zero(t, cache.sets)
	require.Zero(t, cache.refresh)
	require.Zero(t, cache.deletes)
}

func TestOpenAISelectTechnicalResponsesAccountDoesNotMutateStickyState(t *testing.T) {
	t.Parallel()

	cacheKey := "openai:session"
	cache := &technicalOpenAIStickyCache{bindings: map[string]int64{cacheKey: 2}}
	svc := &OpenAIGatewayService{
		accountRepo: stubOpenAIAccountRepo{accounts: []Account{
			{
				ID:          1,
				Platform:    PlatformOpenAI,
				Type:        AccountTypeAPIKey,
				Status:      StatusActive,
				Schedulable: true,
				Concurrency: 1,
			},
			{
				ID:          2,
				Platform:    PlatformOpenAI,
				Type:        AccountTypeAPIKey,
				Status:      StatusDisabled,
				Schedulable: false,
				Concurrency: 1,
			},
		}},
		cache: cache,
	}

	selection, err := svc.SelectTechnicalResponsesAccountWithLoadAwareness(context.Background(), nil, "session", "gpt-5.4", nil)
	require.NoError(t, err)
	require.Equal(t, int64(1), selection.Account.ID)
	require.Equal(t, int64(2), cache.bindings[cacheKey])
	require.Zero(t, cache.sets)
	require.Zero(t, cache.refresh)
	require.Zero(t, cache.deletes)
}

func TestOpenAISelectTechnicalResponsesAccountPropagatesLoadReadError(t *testing.T) {
	t.Parallel()

	stateErr := errors.New("account load store unavailable")
	svc := &OpenAIGatewayService{
		accountRepo: stubOpenAIAccountRepo{accounts: []Account{{
			ID:          1,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
		}}},
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{loadBatchErr: stateErr}),
	}

	_, err := svc.SelectTechnicalResponsesAccountWithLoadAwareness(context.Background(), nil, "", "gpt-5.4", nil)
	require.ErrorIs(t, err, stateErr)
}

func TestOpenAISelectTechnicalResponsesAccountRejectsMissingLoadState(t *testing.T) {
	t.Parallel()

	svc := &OpenAIGatewayService{
		accountRepo: stubOpenAIAccountRepo{accounts: []Account{{
			ID:          1,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
		}}},
		concurrencyService: NewConcurrencyService(&technicalOpenAIConcurrencyCache{loadMap: map[int64]*AccountLoadInfo{}}),
	}

	_, err := svc.SelectTechnicalResponsesAccountWithLoadAwareness(context.Background(), nil, "", "gpt-5.4", nil)
	require.ErrorContains(t, err, "load missing account 1")
}

func TestOpenAISelectTechnicalResponsesAccountPropagatesConcurrencyAcquireError(t *testing.T) {
	t.Parallel()

	stateErr := errors.New("concurrency store unavailable")
	svc := &OpenAIGatewayService{
		accountRepo: stubOpenAIAccountRepo{accounts: []Account{{
			ID:          1,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
		}}},
		concurrencyService: NewConcurrencyService(&technicalOpenAIConcurrencyCache{
			loadMap:    map[int64]*AccountLoadInfo{1: {AccountID: 1, LoadRate: 0}},
			acquireErr: stateErr,
		}),
	}

	_, err := svc.SelectTechnicalResponsesAccountWithLoadAwareness(context.Background(), nil, "", "gpt-5.4", nil)
	require.ErrorIs(t, err, stateErr)
}

func TestOpenAISelectTechnicalResponsesAccountRequiresExactModelMapping(t *testing.T) {
	t.Parallel()

	svc := &OpenAIGatewayService{
		accountRepo: stubOpenAIAccountRepo{accounts: []Account{
			{
				ID:          1,
				Platform:    PlatformOpenAI,
				Type:        AccountTypeAPIKey,
				Status:      StatusActive,
				Schedulable: true,
				Concurrency: 1,
				Priority:    0,
				Credentials: map[string]any{
					"model_mapping": map[string]any{"gpt-*": "wildcard-upstream"},
				},
			},
			{
				ID:          2,
				Platform:    PlatformOpenAI,
				Type:        AccountTypeAPIKey,
				Status:      StatusActive,
				Schedulable: true,
				Concurrency: 1,
				Priority:    1,
				Credentials: map[string]any{
					"model_mapping": map[string]any{"gpt-5.4": "gpt-5.4-upstream"},
				},
			},
		}},
		concurrencyService: NewConcurrencyService(&technicalOpenAIConcurrencyCache{
			loadMap: map[int64]*AccountLoadInfo{2: {AccountID: 2, LoadRate: 0}},
		}),
	}

	selection, err := svc.SelectTechnicalResponsesAccountWithLoadAwareness(context.Background(), nil, "", "gpt-5.4", nil)
	require.NoError(t, err)
	require.Equal(t, int64(2), selection.Account.ID)
}

func TestOpenAISelectTechnicalResponsesAccountDoesNotBypassAccountReadError(t *testing.T) {
	t.Parallel()

	stateErr := errors.New("account repository unavailable")
	accounts := []Account{
		{
			ID:          1,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
			Priority:    0,
		},
		{
			ID:          2,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
			Priority:    1,
		},
	}
	svc := &OpenAIGatewayService{
		accountRepo: technicalOpenAIAccountRepo{
			stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: accounts},
			readErrID:             1,
			readErr:               stateErr,
		},
	}

	_, err := svc.SelectTechnicalResponsesAccountWithLoadAwareness(context.Background(), nil, "", "gpt-5.4", nil)
	require.ErrorIs(t, err, stateErr)
}

func TestOpenAISelectTechnicalResponsesCompactAccountRequiresKnownCapability(t *testing.T) {
	t.Parallel()

	svc := &OpenAIGatewayService{
		accountRepo: stubOpenAIAccountRepo{accounts: []Account{
			{
				ID:          1,
				Platform:    PlatformOpenAI,
				Type:        AccountTypeAPIKey,
				Status:      StatusActive,
				Schedulable: true,
				Concurrency: 1,
				Priority:    0,
			},
			{
				ID:          2,
				Platform:    PlatformOpenAI,
				Type:        AccountTypeAPIKey,
				Status:      StatusActive,
				Schedulable: true,
				Concurrency: 1,
				Priority:    1,
				Extra:       map[string]any{"openai_compact_supported": true},
			},
		}},
		concurrencyService: NewConcurrencyService(&technicalOpenAIConcurrencyCache{
			loadMap: map[int64]*AccountLoadInfo{2: {AccountID: 2, LoadRate: 0}},
		}),
	}

	selection, err := svc.SelectTechnicalResponsesCompactAccountWithLoadAwareness(context.Background(), nil, "", "gpt-5.4", nil)
	require.NoError(t, err)
	require.Equal(t, int64(2), selection.Account.ID)
}

func TestOpenAISelectTechnicalResponsesCompactAccountRejectsUnknownCapability(t *testing.T) {
	t.Parallel()

	svc := &OpenAIGatewayService{
		accountRepo: stubOpenAIAccountRepo{accounts: []Account{{
			ID:          1,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
		}}},
	}

	_, err := svc.SelectTechnicalResponsesCompactAccountWithLoadAwareness(context.Background(), nil, "", "gpt-5.4", nil)
	require.ErrorIs(t, err, ErrNoAvailableCompactAccounts)
}
