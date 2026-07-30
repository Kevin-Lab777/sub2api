package service

import (
	"context"
	"testing"
)

type providerRPMCacheStub struct {
	counts map[int64]int
}

func (s *providerRPMCacheStub) IncrementRPM(_ context.Context, accountID int64) (int, error) {
	s.counts[accountID]++
	return s.counts[accountID], nil
}

func (s *providerRPMCacheStub) GetRPM(_ context.Context, accountID int64) (int, error) {
	return s.counts[accountID], nil
}

func (s *providerRPMCacheStub) GetRPMBatch(_ context.Context, accountIDs []int64) (map[int64]int, error) {
	result := make(map[int64]int, len(accountIDs))
	for _, accountID := range accountIDs {
		result[accountID] = s.counts[accountID]
	}
	return result, nil
}

func TestGatewayAccountRPMAppliesToGeminiAccounts(t *testing.T) {
	svc := &GatewayService{rpmCache: &providerRPMCacheStub{counts: map[int64]int{71: 10}}}
	account := &Account{
		ID:       71,
		Platform: PlatformGemini,
		Extra:    map[string]any{"base_rpm": 10},
	}

	if svc.isAccountSchedulableForRPM(context.Background(), account, false) {
		t.Fatal("non-sticky Gemini request must be rejected at its account RPM limit")
	}
	if !svc.isAccountSchedulableForRPM(context.Background(), account, true) {
		t.Fatal("sticky Gemini request must remain eligible in the configured sticky tier")
	}
}
