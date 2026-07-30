package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestGatewayCacheLiveCallIdentityAndController(t *testing.T) {
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	cache, ok := NewGatewayCache(client).(service.LiveCallStore)
	require.True(t, ok)
	otherInstance, ok := NewGatewayCache(client).(service.LiveCallStore)
	require.True(t, ok)
	record := &service.LiveCallRecord{
		CallID:                "call_secret",
		CallHash:              HashLiveCallID("call_secret"),
		AccountID:             11,
		APIKeyID:              22,
		UserID:                33,
		GroupID:               44,
		LeaseID:               "lease",
		Model:                 "gpt-live-test",
		AttestationCiphertext: "encrypted-attestation",
		CreatedAt:             time.Now(),
		ExpiresAt:             time.Now().Add(time.Hour),
		Controller:            service.LiveControllerPending,
	}
	require.NoError(t, cache.SaveLiveCall(context.Background(), record, time.Hour))

	loaded, err := otherInstance.GetLiveCall(context.Background(), record.CallHash)
	require.NoError(t, err)
	require.Equal(t, record.CallID, loaded.CallID)
	require.Equal(t, record.AccountID, loaded.AccountID)
	require.Equal(t, record.AttestationCiphertext, loaded.AttestationCiphertext)

	claimed, err := cache.ClaimLiveController(context.Background(), record.CallHash, service.LiveControllerObserver, "observer-1")
	require.NoError(t, err)
	require.True(t, claimed)
	claimed, err = cache.ClaimLiveController(context.Background(), record.CallHash, service.LiveControllerProxy, "proxy-1")
	require.NoError(t, err)
	require.True(t, claimed)
	controller, err := cache.GetLiveController(context.Background(), record.CallHash)
	require.NoError(t, err)
	require.Equal(t, service.LiveControllerProxy, controller)

	released, err := cache.ReleaseLiveController(context.Background(), record.CallHash, "proxy-1")
	require.NoError(t, err)
	require.True(t, released)
	closed, err := cache.MarkLiveCallClosed(context.Background(), record.CallHash, time.Hour)
	require.NoError(t, err)
	require.True(t, closed)
	closed, err = cache.MarkLiveCallClosed(context.Background(), record.CallHash, time.Hour)
	require.NoError(t, err)
	require.False(t, closed)
}

func TestGatewayCacheLiveCallPersistsTechnicalIdentityWithoutCustomerIDs(t *testing.T) {
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	cache, ok := NewGatewayCache(client).(service.LiveCallStore)
	require.True(t, ok)
	record := &service.LiveCallRecord{
		CallID:           "call_technical",
		CallHash:         HashLiveCallID("call_technical"),
		AccountID:        11,
		LeaseID:          "technical-lease",
		Model:            "gpt-live-test",
		CreatedAt:        time.Now(),
		ExpiresAt:        time.Now().Add(time.Hour),
		Controller:       service.LiveControllerPending,
		Technical:        true,
		TechnicalPoolID:  44,
		TechnicalRequest: "request-1",
		TechnicalSession: "session-1",
	}
	require.NoError(t, cache.SaveLiveCall(context.Background(), record, time.Hour))

	loaded, err := cache.GetLiveCall(context.Background(), record.CallHash)
	require.NoError(t, err)
	require.True(t, loaded.Technical)
	require.Equal(t, int64(44), loaded.TechnicalPoolID)
	require.Equal(t, "request-1", loaded.TechnicalRequest)
	require.Equal(t, "session-1", loaded.TechnicalSession)
	require.Zero(t, loaded.UserID)
	require.Zero(t, loaded.APIKeyID)
	require.Zero(t, loaded.SubscriptionID)
}
