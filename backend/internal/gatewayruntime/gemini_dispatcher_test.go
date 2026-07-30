package gatewayruntime

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/gatewaycore"
	"github.com/Wei-Shaw/sub2api/internal/gatewaytransport"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type geminiSchedulerStub struct {
	selectAccount func(context.Context, *int64, string, string, map[int64]struct{}) (*service.AccountSelectionResult, error)
	selectedPools []int64
	selectedIDs   []int64
	boundPool     int64
	boundSession  string
	boundAccount  int64
	rpmAccount    int64
}

func (s *geminiSchedulerStub) ValidateTechnicalSchedulerRuntime() error { return nil }

func (s *geminiSchedulerStub) SelectAccountWithLoadAwareness(ctx context.Context, poolID *int64, sessionID, model string, excluded map[int64]struct{}) (*service.AccountSelectionResult, error) {
	if poolID != nil {
		s.selectedPools = append(s.selectedPools, *poolID)
	}
	if platform, _ := ctx.Value(ctxkey.ForcePlatform).(string); platform != service.PlatformGemini {
		return nil, errors.New("native Gemini platform constraint missing")
	}
	return s.selectAccount(ctx, poolID, sessionID, model, excluded)
}

func (s *geminiSchedulerStub) AcquireSelection(_ context.Context, selection *service.AccountSelectionResult) (func(), error) {
	if selection == nil || selection.Account == nil {
		return nil, service.ErrInvalidAccountSelection
	}
	s.selectedIDs = append(s.selectedIDs, selection.Account.ID)
	if selection.ReleaseFunc != nil {
		return selection.ReleaseFunc, nil
	}
	return func() {}, nil
}

func (s *geminiSchedulerStub) TempUnscheduleRetryableError(context.Context, int64, *service.UpstreamFailoverError) {
}

func (s *geminiSchedulerStub) IncrementAccountRPM(_ context.Context, accountID int64) error {
	s.rpmAccount = accountID
	return nil
}

func (s *geminiSchedulerStub) BindStickySession(_ context.Context, poolID *int64, sessionID string, accountID int64) error {
	if poolID != nil {
		s.boundPool = *poolID
	}
	s.boundSession = sessionID
	s.boundAccount = accountID
	return nil
}

type geminiForwarderStub struct {
	forward func(gatewaytransport.Exchange, *service.Account, string, string, bool, []byte) (*service.ForwardResult, error)
}

func (s *geminiForwarderStub) ValidateTechnicalRuntime() error { return nil }

func (s *geminiForwarderStub) ForwardNativeExchange(_ context.Context, exchange gatewaytransport.Exchange, account *service.Account, model, action string, stream bool, body []byte) (*service.ForwardResult, error) {
	return s.forward(exchange, account, model, action, stream, body)
}

func newGeminiDispatcherForTest(t *testing.T, scheduler *geminiSchedulerStub, forwarder *geminiForwarderStub) *GeminiDispatcher {
	t.Helper()
	dispatcher, err := NewGeminiDispatcher(scheduler, forwarder, GeminiDispatcherConfig{
		MaxAccountSwitches: 3,
		MaxBodyBytes:       1 << 20,
	})
	if err != nil {
		t.Fatalf("NewGeminiDispatcher: %v", err)
	}
	return dispatcher
}

func geminiDispatchRequest() gatewaycore.DispatchRequest {
	return gatewaycore.DispatchRequest{
		Pool: gatewaycore.Pool{ID: 41, Platform: service.PlatformGemini, Active: true},
		Invocation: gatewaycore.Invocation{
			PoolID:    41,
			RequestID: "req-gemini",
			SessionID: "session-gemini",
			Protocol:  gatewaycore.ProtocolGemini,
			Model:     "gemini-2.5-pro",
			OnUsage:   func(context.Context, gatewaycore.Usage) error { return nil },
		},
	}
}

func TestGeminiDispatcherUsesExactPoolAndReportsRawMeasurement(t *testing.T) {
	account := &service.Account{ID: 81, Platform: service.PlatformGemini, Type: service.AccountTypeAPIKey, Extra: map[string]any{"base_rpm": 60}}
	scheduler := &geminiSchedulerStub{}
	scheduler.selectAccount = func(_ context.Context, poolID *int64, sessionID, model string, excluded map[int64]struct{}) (*service.AccountSelectionResult, error) {
		if poolID == nil || *poolID != 41 || sessionID != "session-gemini" || model != "gemini-2.5-pro" || len(excluded) != 0 {
			t.Fatalf("unexpected selection request: pool=%v session=%q model=%q excluded=%v", poolID, sessionID, model, excluded)
		}
		return &service.AccountSelectionResult{Account: account, Acquired: true, ReleaseFunc: func() {}}, nil
	}
	forwarder := &geminiForwarderStub{}
	forwarder.forward = func(exchange gatewaytransport.Exchange, got *service.Account, model, action string, stream bool, body []byte) (*service.ForwardResult, error) {
		if got.ID != account.ID || model != "gemini-2.5-pro" || action != "generateContent" || stream || string(body) != `{"contents":[]}` {
			t.Fatalf("unexpected forward: account=%d model=%q action=%q stream=%v body=%s", got.ID, model, action, stream, body)
		}
		if err := exchange.WriteJSON(http.StatusOK, map[string]any{"candidates": []any{}}); err != nil {
			return nil, err
		}
		firstToken := 4
		return &service.ForwardResult{
			Usage: service.ClaudeUsage{
				InputTokens:              17,
				OutputTokens:             9,
				CacheReadInputTokens:     5,
				CacheCreationInputTokens: 3,
			},
			Model:         model,
			UpstreamModel: "gemini-2.5-pro-001",
			Duration:      8 * time.Millisecond,
			FirstTokenMs:  &firstToken,
			ImageCount:    1,
		}, nil
	}

	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.5-pro:generateContent", strings.NewReader(`{"contents":[]}`))
	recorder := httptest.NewRecorder()
	measurement, err := newGeminiDispatcherForTest(t, scheduler, forwarder).Forward(context.Background(), recorder, req, geminiDispatchRequest())
	if err != nil {
		t.Fatalf("Forward: %v", err)
	}
	if len(scheduler.selectedPools) != 1 || scheduler.selectedPools[0] != 41 {
		t.Fatalf("selection escaped pool: %v", scheduler.selectedPools)
	}
	if measurement.AccountID != 81 || measurement.UpstreamModel != "gemini-2.5-pro-001" || measurement.InputTokens != 17 || measurement.OutputTokens != 9 || measurement.CacheReadInputTokens != 5 || measurement.CacheWriteInputTokens != 3 || measurement.ImageCount != 1 || measurement.UpstreamStatusCode != http.StatusOK {
		t.Fatalf("unexpected measurement: %+v", measurement)
	}
	if scheduler.boundPool != 41 || scheduler.boundSession != "session-gemini" || scheduler.boundAccount != 81 || scheduler.rpmAccount != 81 {
		t.Fatalf("unexpected scheduler state: %+v", scheduler)
	}
}

func TestGeminiDispatcherFailoverRemainsInsidePool(t *testing.T) {
	accounts := []*service.Account{
		{ID: 91, Platform: service.PlatformGemini, Type: service.AccountTypeAPIKey},
		{ID: 92, Platform: service.PlatformGemini, Type: service.AccountTypeAPIKey},
	}
	scheduler := &geminiSchedulerStub{}
	scheduler.selectAccount = func(_ context.Context, poolID *int64, _ string, _ string, excluded map[int64]struct{}) (*service.AccountSelectionResult, error) {
		if poolID == nil || *poolID != 41 {
			t.Fatalf("unexpected pool: %v", poolID)
		}
		account := accounts[0]
		if _, failed := excluded[account.ID]; failed {
			account = accounts[1]
		}
		return &service.AccountSelectionResult{Account: account, Acquired: true, ReleaseFunc: func() {}}, nil
	}
	forwarder := &geminiForwarderStub{}
	forwarder.forward = func(exchange gatewaytransport.Exchange, account *service.Account, model, _ string, _ bool, _ []byte) (*service.ForwardResult, error) {
		if account.ID == 91 {
			return nil, &service.UpstreamFailoverError{StatusCode: http.StatusTooManyRequests}
		}
		if err := exchange.WriteData(http.StatusOK, "application/json", []byte(`{"ok":true}`)); err != nil {
			return nil, err
		}
		return &service.ForwardResult{Model: model, Duration: time.Millisecond}, nil
	}

	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.5-pro:generateContent", strings.NewReader(`{"contents":[]}`))
	measurement, err := newGeminiDispatcherForTest(t, scheduler, forwarder).Forward(context.Background(), httptest.NewRecorder(), req, geminiDispatchRequest())
	if err != nil {
		t.Fatalf("Forward: %v", err)
	}
	if measurement.AccountID != 92 || len(scheduler.selectedPools) != 2 || scheduler.selectedPools[0] != 41 || scheduler.selectedPools[1] != 41 {
		t.Fatalf("failover escaped pool: measurement=%+v pools=%v", measurement, scheduler.selectedPools)
	}
}

func TestGeminiDispatcherDoesNotFailoverAfterResponseCommit(t *testing.T) {
	scheduler := &geminiSchedulerStub{}
	selections := 0
	scheduler.selectAccount = func(_ context.Context, _ *int64, _ string, _ string, _ map[int64]struct{}) (*service.AccountSelectionResult, error) {
		selections++
		return &service.AccountSelectionResult{
			Account:     &service.Account{ID: int64(100 + selections), Platform: service.PlatformGemini, Type: service.AccountTypeAPIKey},
			Acquired:    true,
			ReleaseFunc: func() {},
		}, nil
	}
	forwarder := &geminiForwarderStub{}
	forwarder.forward = func(exchange gatewaytransport.Exchange, _ *service.Account, _ string, _ string, _ bool, _ []byte) (*service.ForwardResult, error) {
		if err := exchange.WriteData(http.StatusBadGateway, "application/json", []byte(`{"error":"committed"}`)); err != nil {
			return nil, err
		}
		return nil, &service.UpstreamFailoverError{StatusCode: http.StatusBadGateway}
	}

	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.5-pro:generateContent", strings.NewReader(`{"contents":[]}`))
	_, err := newGeminiDispatcherForTest(t, scheduler, forwarder).Forward(context.Background(), httptest.NewRecorder(), req, geminiDispatchRequest())
	if err == nil || selections != 1 {
		t.Fatalf("expected committed response to stop failover: err=%v selections=%d", err, selections)
	}
}

func TestGeminiDispatcherRejectsInvocationPathModelMismatch(t *testing.T) {
	scheduler := &geminiSchedulerStub{selectAccount: func(context.Context, *int64, string, string, map[int64]struct{}) (*service.AccountSelectionResult, error) {
		t.Fatal("scheduler must not run for mismatched model")
		return nil, nil
	}}
	forwarder := &geminiForwarderStub{forward: func(gatewaytransport.Exchange, *service.Account, string, string, bool, []byte) (*service.ForwardResult, error) {
		t.Fatal("forwarder must not run for mismatched model")
		return nil, nil
	}}

	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.0-flash:generateContent", strings.NewReader(`{"contents":[]}`))
	_, err := newGeminiDispatcherForTest(t, scheduler, forwarder).Forward(context.Background(), httptest.NewRecorder(), req, geminiDispatchRequest())
	if !errors.Is(err, ErrInvocationModelMismatch) {
		t.Fatalf("expected model mismatch, got %v", err)
	}
}

func TestParseNativeGeminiEndpointPreservesQualifiedModel(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/publisher/gemini-2.5-pro:streamGenerateContent?alt=sse", nil)
	model, action, stream, err := parseNativeGeminiEndpoint(req)
	if err != nil || model != "publisher/gemini-2.5-pro" || action != "streamGenerateContent" || !stream {
		t.Fatalf("unexpected parse result: model=%q action=%q stream=%v err=%v", model, action, stream, err)
	}
}
