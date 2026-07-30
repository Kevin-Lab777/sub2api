package gatewayruntime

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/gatewaycore"
	"github.com/Wei-Shaw/sub2api/internal/gatewaytransport"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type anthropicGatewayStub struct {
	accounts       []*service.Account
	selectCalls    int
	selectedPools  []int64
	excludedByCall []map[int64]struct{}
	forward        func(gatewaytransport.Exchange, *service.Account, *service.ParsedRequest) (*service.ForwardResult, error)
	releases       int
	boundPool      int64
	boundSession   string
	boundAccount   int64
}

func (s *anthropicGatewayStub) ValidateTechnicalRuntime() error { return nil }

func (s *anthropicGatewayStub) SelectAccountWithLoadAwareness(
	_ context.Context,
	poolID *int64,
	_ string,
	_ string,
	excluded map[int64]struct{},
) (*service.AccountSelectionResult, error) {
	s.selectCalls++
	s.selectedPools = append(s.selectedPools, *poolID)
	copyExcluded := make(map[int64]struct{}, len(excluded))
	for id := range excluded {
		copyExcluded[id] = struct{}{}
	}
	s.excludedByCall = append(s.excludedByCall, copyExcluded)
	for _, account := range s.accounts {
		if _, skip := excluded[account.ID]; skip {
			continue
		}
		return &service.AccountSelectionResult{
			Account:     account,
			Acquired:    true,
			ReleaseFunc: func() { s.releases++ },
		}, nil
	}
	return nil, service.ErrNoAvailableAccounts
}

func (s *anthropicGatewayStub) AcquireSelection(_ context.Context, selection *service.AccountSelectionResult) (func(), error) {
	return selection.ReleaseFunc, nil
}

func (s *anthropicGatewayStub) ApplyBedrockCCCompatExchange(_ gatewaytransport.Exchange, body []byte, _ string, _ *service.Account, _ *int64) []byte {
	return body
}

func (s *anthropicGatewayStub) ForwardExchange(_ context.Context, exchange gatewaytransport.Exchange, account *service.Account, parsed *service.ParsedRequest) (*service.ForwardResult, error) {
	return s.forward(exchange, account, parsed)
}

func (s *anthropicGatewayStub) TempUnscheduleRetryableError(context.Context, int64, *service.UpstreamFailoverError) {
}

func (s *anthropicGatewayStub) IncrementAccountRPM(context.Context, int64) error { return nil }

func (s *anthropicGatewayStub) BindStickySession(_ context.Context, poolID *int64, session string, accountID int64) error {
	s.boundPool = *poolID
	s.boundSession = session
	s.boundAccount = accountID
	return nil
}

func newAnthropicTestDispatcher(t *testing.T, gateway anthropicGateway) *AnthropicDispatcher {
	t.Helper()
	dispatcher, err := NewAnthropicDispatcher(gateway, AnthropicDispatcherConfig{
		MaxAccountSwitches: 3,
		MaxBodyBytes:       1 << 20,
	})
	if err != nil {
		t.Fatalf("NewAnthropicDispatcher() error = %v", err)
	}
	return dispatcher
}

func anthropicDispatchRequest() gatewaycore.DispatchRequest {
	return gatewaycore.DispatchRequest{
		Pool: gatewaycore.Pool{ID: 42, Name: "anthropic-pool", Platform: service.PlatformAnthropic, Active: true},
		Invocation: gatewaycore.Invocation{
			PoolID:    42,
			RequestID: "req-1",
			SessionID: "session-1",
			Protocol:  gatewaycore.ProtocolAnthropic,
			Model:     "claude-sonnet-4-6",
		},
	}
}

func anthropicRequest(model string) *http.Request {
	body := []byte(`{"model":"` + model + `","max_tokens":128,"messages":[{"role":"user","content":"hello"}]}`)
	return httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
}

func TestAnthropicDispatcherForwardsWithinExactPool(t *testing.T) {
	gateway := &anthropicGatewayStub{
		accounts: []*service.Account{{ID: 7, Platform: service.PlatformAnthropic, Type: service.AccountTypeAPIKey}},
	}
	gateway.forward = func(exchange gatewaytransport.Exchange, account *service.Account, parsed *service.ParsedRequest) (*service.ForwardResult, error) {
		if account.ID != 7 || parsed.GroupID == nil || *parsed.GroupID != 42 {
			t.Fatalf("forward account/group = %d/%v", account.ID, parsed.GroupID)
		}
		if err := exchange.WriteData(http.StatusOK, "application/json", []byte(`{"ok":true}`)); err != nil {
			t.Fatalf("WriteData() error = %v", err)
		}
		return &service.ForwardResult{
			Model:         parsed.Model,
			UpstreamModel: "claude-sonnet-4-6-20260101",
			Duration:      5 * time.Millisecond,
			Usage: service.ClaudeUsage{
				InputTokens:              11,
				OutputTokens:             3,
				CacheCreationInputTokens: 5,
				CacheReadInputTokens:     2,
			},
		}, nil
	}

	recorder := httptest.NewRecorder()
	measurement, err := newAnthropicTestDispatcher(t, gateway).Forward(
		context.Background(), recorder, anthropicRequest("claude-sonnet-4-6"), anthropicDispatchRequest(),
	)
	if err != nil {
		t.Fatalf("Forward() error = %v", err)
	}
	if measurement.AccountID != 7 || measurement.InputTokens != 11 || measurement.CacheWriteInputTokens != 5 {
		t.Fatalf("measurement = %+v", measurement)
	}
	if gateway.boundPool != 42 || gateway.boundSession != "session-1" || gateway.boundAccount != 7 {
		t.Fatalf("sticky binding = pool:%d session:%q account:%d", gateway.boundPool, gateway.boundSession, gateway.boundAccount)
	}
	if gateway.releases != 1 {
		t.Fatalf("release count = %d, want 1", gateway.releases)
	}
}

func TestAnthropicDispatcherFailoverCannotLeavePool(t *testing.T) {
	gateway := &anthropicGatewayStub{
		accounts: []*service.Account{
			{ID: 1, Platform: service.PlatformAnthropic, Type: service.AccountTypeAPIKey},
			{ID: 2, Platform: service.PlatformAnthropic, Type: service.AccountTypeAPIKey},
		},
	}
	gateway.forward = func(exchange gatewaytransport.Exchange, account *service.Account, parsed *service.ParsedRequest) (*service.ForwardResult, error) {
		if account.ID == 1 {
			return nil, &service.UpstreamFailoverError{StatusCode: http.StatusServiceUnavailable}
		}
		if err := exchange.WriteData(http.StatusOK, "application/json", []byte(`{"ok":true}`)); err != nil {
			return nil, err
		}
		return &service.ForwardResult{Model: parsed.Model, Duration: time.Millisecond, Usage: service.ClaudeUsage{}}, nil
	}

	measurement, err := newAnthropicTestDispatcher(t, gateway).Forward(
		context.Background(), httptest.NewRecorder(), anthropicRequest("claude-sonnet-4-6"), anthropicDispatchRequest(),
	)
	if err != nil {
		t.Fatalf("Forward() error = %v", err)
	}
	if measurement.AccountID != 2 || gateway.selectCalls != 2 {
		t.Fatalf("measurement/select calls = %+v/%d", measurement, gateway.selectCalls)
	}
	for _, poolID := range gateway.selectedPools {
		if poolID != 42 {
			t.Fatalf("selected pool %d, want 42", poolID)
		}
	}
	if _, ok := gateway.excludedByCall[1][1]; !ok {
		t.Fatalf("second selection exclusions = %v, want account 1", gateway.excludedByCall[1])
	}
}

func TestAnthropicDispatcherDoesNotFailoverAfterResponseWrite(t *testing.T) {
	gateway := &anthropicGatewayStub{
		accounts: []*service.Account{
			{ID: 1, Platform: service.PlatformAnthropic, Type: service.AccountTypeAPIKey},
			{ID: 2, Platform: service.PlatformAnthropic, Type: service.AccountTypeAPIKey},
		},
	}
	gateway.forward = func(exchange gatewaytransport.Exchange, _ *service.Account, _ *service.ParsedRequest) (*service.ForwardResult, error) {
		_, _ = exchange.Response().Write([]byte("partial"))
		return nil, &service.UpstreamFailoverError{StatusCode: http.StatusBadGateway}
	}

	measurement, err := newAnthropicTestDispatcher(t, gateway).Forward(
		context.Background(), httptest.NewRecorder(), anthropicRequest("claude-sonnet-4-6"), anthropicDispatchRequest(),
	)
	if measurement != nil || err == nil {
		t.Fatalf("Forward() = %+v, %v; want nil measurement and error", measurement, err)
	}
	if gateway.selectCalls != 1 {
		t.Fatalf("select calls = %d, want 1", gateway.selectCalls)
	}
}

func TestAnthropicDispatcherRejectsInvocationBodyModelMismatch(t *testing.T) {
	gateway := &anthropicGatewayStub{forward: func(gatewaytransport.Exchange, *service.Account, *service.ParsedRequest) (*service.ForwardResult, error) {
		return nil, errors.New("must not forward")
	}}
	measurement, err := newAnthropicTestDispatcher(t, gateway).Forward(
		context.Background(), httptest.NewRecorder(), anthropicRequest("claude-opus-4-6"), anthropicDispatchRequest(),
	)
	if measurement != nil || !errors.Is(err, ErrInvocationModelMismatch) {
		t.Fatalf("Forward() = %+v, %v", measurement, err)
	}
	if gateway.selectCalls != 0 {
		t.Fatalf("select calls = %d, want 0", gateway.selectCalls)
	}
}
