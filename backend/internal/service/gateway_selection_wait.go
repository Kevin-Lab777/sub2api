package service

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"
)

var (
	ErrInvalidAccountSelection     = errors.New("invalid account selection")
	ErrAccountConcurrencyBusy      = errors.New("account concurrency is full")
	ErrAccountWaitQueueFull        = errors.New("account wait queue is full")
	ErrAccountConcurrencyWaitTimed = errors.New("account concurrency wait timed out")
)

const (
	accountWaitInitialBackoff = 100 * time.Millisecond
	accountWaitMaxBackoff     = 2 * time.Second
	accountWaitMultiplier     = 1.5
)

// AcquireSelection resolves a scheduler wait plan into an account concurrency lease.
func (s *GatewayService) AcquireSelection(ctx context.Context, selection *AccountSelectionResult) (func(), error) {
	if s == nil || selection == nil || selection.Account == nil {
		return nil, ErrInvalidAccountSelection
	}
	if selection.Acquired {
		if selection.ReleaseFunc == nil {
			return nil, fmt.Errorf("%w: acquired selection has no release function", ErrInvalidAccountSelection)
		}
		return selection.ReleaseFunc, nil
	}
	if selection.WaitPlan == nil {
		return nil, ErrAccountConcurrencyBusy
	}
	if s.concurrencyService == nil {
		return nil, fmt.Errorf("%w: concurrency service is unavailable", ErrInvalidAccountSelection)
	}

	plan := selection.WaitPlan
	if plan.AccountID != selection.Account.ID || plan.MaxConcurrency <= 0 || plan.Timeout <= 0 || plan.MaxWaiting <= 0 {
		return nil, fmt.Errorf("%w: malformed account wait plan", ErrInvalidAccountSelection)
	}

	canWait, err := s.concurrencyService.IncrementAccountWaitCount(ctx, plan.AccountID, plan.MaxWaiting)
	if err != nil {
		return nil, fmt.Errorf("increment account wait count: %w", err)
	}
	if !canWait {
		return nil, ErrAccountWaitQueueFull
	}
	defer s.concurrencyService.DecrementAccountWaitCount(ctx, plan.AccountID)

	waitCtx, cancel := context.WithTimeout(ctx, plan.Timeout)
	defer cancel()

	backoff := accountWaitInitialBackoff
	for {
		result, err := s.concurrencyService.AcquireAccountSlot(waitCtx, plan.AccountID, plan.MaxConcurrency)
		if err != nil {
			return nil, fmt.Errorf("acquire account slot: %w", err)
		}
		if result.Acquired {
			if result.ReleaseFunc == nil {
				return nil, fmt.Errorf("%w: acquired wait result has no release function", ErrInvalidAccountSelection)
			}
			return result.ReleaseFunc, nil
		}

		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			stopAndDrainTimer(timer)
			return nil, ctx.Err()
		case <-waitCtx.Done():
			stopAndDrainTimer(timer)
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, ErrAccountConcurrencyWaitTimed
		case <-timer.C:
		}
		backoff = nextAccountWaitBackoff(backoff)
	}
}

func stopAndDrainTimer(timer *time.Timer) {
	if timer == nil || timer.Stop() {
		return
	}
	select {
	case <-timer.C:
	default:
	}
}

func nextAccountWaitBackoff(current time.Duration) time.Duration {
	next := time.Duration(float64(current) * accountWaitMultiplier)
	if next > accountWaitMaxBackoff {
		next = accountWaitMaxBackoff
	}
	jittered := time.Duration(float64(next) * (0.8 + rand.Float64()*0.4))
	if jittered < accountWaitInitialBackoff {
		return accountWaitInitialBackoff
	}
	if jittered > accountWaitMaxBackoff {
		return accountWaitMaxBackoff
	}
	return jittered
}
