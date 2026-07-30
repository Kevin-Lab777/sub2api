//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGatewayServiceAcquireSelectionUsesExistingLease(t *testing.T) {
	released := false
	selection := &AccountSelectionResult{
		Account:     &Account{ID: 7},
		Acquired:    true,
		ReleaseFunc: func() { released = true },
	}

	release, err := (&GatewayService{}).AcquireSelection(context.Background(), selection)
	require.NoError(t, err)
	require.NotNil(t, release)
	release()
	require.True(t, released)
}

func TestGatewayServiceAcquireSelectionRejectsFullWaitQueue(t *testing.T) {
	cache := &stubConcurrencyCacheForTest{waitAllowed: false}
	gateway := &GatewayService{concurrencyService: NewConcurrencyService(cache)}
	selection := &AccountSelectionResult{
		Account: &Account{ID: 9},
		WaitPlan: &AccountWaitPlan{
			AccountID:      9,
			MaxConcurrency: 1,
			Timeout:        time.Second,
			MaxWaiting:     2,
		},
	}

	release, err := gateway.AcquireSelection(context.Background(), selection)
	require.Nil(t, release)
	require.ErrorIs(t, err, ErrAccountWaitQueueFull)
}

func TestGatewayServiceAcquireSelectionPropagatesWaitCounterFailure(t *testing.T) {
	wantErr := errors.New("redis unavailable")
	cache := &stubConcurrencyCacheForTest{waitErr: wantErr}
	gateway := &GatewayService{concurrencyService: NewConcurrencyService(cache)}
	selection := &AccountSelectionResult{
		Account: &Account{ID: 9},
		WaitPlan: &AccountWaitPlan{
			AccountID:      9,
			MaxConcurrency: 1,
			Timeout:        time.Second,
			MaxWaiting:     2,
		},
	}

	release, err := gateway.AcquireSelection(context.Background(), selection)
	require.Nil(t, release)
	require.ErrorIs(t, err, wantErr)
}
