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
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type openAIChatCompletionsGatewayStub struct {
	selectAccount  func(*int64, map[int64]struct{}) (*service.AccountSelectionResult, error)
	forward        func(gatewaytransport.Exchange, *service.Account, []byte) (*service.OpenAIForwardResult, error)
	pools          []int64
	boundPool      int64
	boundAccount   int64
	boundSession   string
	directSelects  int
	unifiedSelects int
}

func (s *openAIChatCompletionsGatewayStub) ValidateTechnicalRuntime() error { return nil }

func (s *openAIChatCompletionsGatewayStub) SelectTechnicalChatCompletionsDirectAccountWithLoadAwareness(_ context.Context, poolID *int64, _ string, _ string, excluded map[int64]struct{}) (*service.AccountSelectionResult, error) {
	s.directSelects++
	if poolID != nil {
		s.pools = append(s.pools, *poolID)
	}
	return s.selectAccount(poolID, excluded)
}

func (s *openAIChatCompletionsGatewayStub) SelectTechnicalChatCompletionsAccountWithLoadAwareness(ctx context.Context, poolID *int64, session, model string, excluded map[int64]struct{}) (*service.AccountSelectionResult, error) {
	_ = ctx
	_ = session
	_ = model
	s.unifiedSelects++
	if poolID != nil {
		s.pools = append(s.pools, *poolID)
	}
	return s.selectAccount(poolID, excluded)
}

func (s *openAIChatCompletionsGatewayStub) SelectTechnicalCompletionsDirectAccountWithLoadAwareness(ctx context.Context, poolID *int64, session, model string, excluded map[int64]struct{}) (*service.AccountSelectionResult, error) {
	return s.SelectTechnicalChatCompletionsDirectAccountWithLoadAwareness(ctx, poolID, session, model, excluded)
}

func (s *openAIChatCompletionsGatewayStub) AcquireSelection(_ context.Context, selection *service.AccountSelectionResult) (func(), error) {
	if selection == nil || selection.Account == nil {
		return nil, service.ErrInvalidAccountSelection
	}
	if selection.ReleaseFunc != nil {
		return selection.ReleaseFunc, nil
	}
	return func() {}, nil
}

func (s *openAIChatCompletionsGatewayStub) ForwardChatCompletionsExchange(_ context.Context, exchange gatewaytransport.Exchange, account *service.Account, body []byte) (*service.OpenAIForwardResult, error) {
	return s.forward(exchange, account, body)
}

func (s *openAIChatCompletionsGatewayStub) ForwardCompletionsExchange(ctx context.Context, exchange gatewaytransport.Exchange, account *service.Account, body []byte) (*service.OpenAIForwardResult, error) {
	return s.ForwardChatCompletionsExchange(ctx, exchange, account, body)
}

func (s *openAIChatCompletionsGatewayStub) BindStickySession(_ context.Context, poolID *int64, session string, accountID int64) error {
	if poolID != nil {
		s.boundPool = *poolID
	}
	s.boundSession = session
	s.boundAccount = accountID
	return nil
}

func newOpenAIChatCompletionsDispatcherForTest(t *testing.T, gateway *openAIChatCompletionsGatewayStub) *OpenAIChatCompletionsDispatcher {
	t.Helper()
	dispatcher, err := NewOpenAIChatCompletionsDispatcher(gateway, OpenAIChatCompletionsDispatcherConfig{
		MaxAccountSwitches: 3,
		MaxBodyBytes:       1 << 20,
	})
	if err != nil {
		t.Fatalf("NewOpenAIChatCompletionsDispatcher: %v", err)
	}
	return dispatcher
}

func openAIChatCompletionsDispatchRequest() gatewaycore.DispatchRequest {
	return gatewaycore.DispatchRequest{
		Pool: gatewaycore.Pool{ID: 61, Platform: service.PlatformOpenAI, Active: true},
		Invocation: gatewaycore.Invocation{
			PoolID:    61,
			RequestID: "request-chat",
			SessionID: "session-chat",
			Protocol:  gatewaycore.ProtocolOpenAI,
			Model:     "gpt-5.4",
			OnUsage:   func(context.Context, gatewaycore.Usage) error { return nil },
		},
	}
}

func TestOpenAIChatCompletionsDispatcherUsesExactPoolAndMeasuresUsage(t *testing.T) {
	account := &service.Account{ID: 501, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey}
	gateway := &openAIChatCompletionsGatewayStub{}
	gateway.selectAccount = func(poolID *int64, excluded map[int64]struct{}) (*service.AccountSelectionResult, error) {
		if poolID == nil || *poolID != 61 || len(excluded) != 0 {
			t.Fatalf("unexpected selection: pool=%v excluded=%v", poolID, excluded)
		}
		return &service.AccountSelectionResult{Account: account, Acquired: true, ReleaseFunc: func() {}}, nil
	}
	gateway.forward = func(exchange gatewaytransport.Exchange, got *service.Account, body []byte) (*service.OpenAIForwardResult, error) {
		if got.ID != account.ID || string(body) != `{"model":"gpt-5.4","messages":[]}` {
			t.Fatalf("unexpected forward: account=%d body=%s", got.ID, body)
		}
		if err := exchange.WriteData(http.StatusOK, "application/json", []byte(`{"id":"chatcmpl_1"}`)); err != nil {
			return nil, err
		}
		first := 2
		return &service.OpenAIForwardResult{
			Usage: service.OpenAIUsage{
				InputTokens:              21,
				OutputTokens:             8,
				CacheReadInputTokens:     5,
				CacheCreationInputTokens: 3,
			},
			UpstreamModel: "gpt-5.4-upstream",
			Duration:      6 * time.Millisecond,
			FirstTokenMs:  &first,
		}, nil
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-5.4","messages":[]}`))
	measurement, err := newOpenAIChatCompletionsDispatcherForTest(t, gateway).Forward(context.Background(), httptest.NewRecorder(), req, openAIChatCompletionsDispatchRequest())
	if err != nil {
		t.Fatalf("Forward: %v", err)
	}
	if measurement.AccountID != 501 || measurement.InputTokens != 21 || measurement.OutputTokens != 8 || measurement.CacheReadInputTokens != 5 || measurement.CacheWriteInputTokens != 3 || measurement.UpstreamModel != "gpt-5.4-upstream" {
		t.Fatalf("unexpected measurement: %+v", measurement)
	}
	if len(gateway.pools) != 1 || gateway.pools[0] != 61 || gateway.boundPool != 61 || gateway.boundAccount != 501 || gateway.boundSession != "session-chat" {
		t.Fatalf("unexpected pool/binding state: %+v", gateway)
	}
}

func TestOpenAIChatCompletionsDispatcherFailoverStaysInsidePool(t *testing.T) {
	accounts := []*service.Account{
		{ID: 511, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey},
		{ID: 512, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey},
	}
	gateway := &openAIChatCompletionsGatewayStub{}
	gateway.selectAccount = func(_ *int64, excluded map[int64]struct{}) (*service.AccountSelectionResult, error) {
		account := accounts[0]
		if _, failed := excluded[account.ID]; failed {
			account = accounts[1]
		}
		return &service.AccountSelectionResult{Account: account, Acquired: true, ReleaseFunc: func() {}}, nil
	}
	gateway.forward = func(exchange gatewaytransport.Exchange, account *service.Account, _ []byte) (*service.OpenAIForwardResult, error) {
		if account.ID == 511 {
			return nil, &service.UpstreamFailoverError{StatusCode: http.StatusTooManyRequests, NextAccountAction: service.NextAccountRetry}
		}
		if err := exchange.WriteData(http.StatusOK, "application/json", []byte(`{"id":"chatcmpl_2"}`)); err != nil {
			return nil, err
		}
		return &service.OpenAIForwardResult{UpstreamModel: "gpt-5.4", Duration: time.Millisecond}, nil
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-5.4","messages":[]}`))
	measurement, err := newOpenAIChatCompletionsDispatcherForTest(t, gateway).Forward(context.Background(), httptest.NewRecorder(), req, openAIChatCompletionsDispatchRequest())
	if err != nil || measurement.AccountID != 512 || len(gateway.pools) != 2 || gateway.pools[0] != 61 || gateway.pools[1] != 61 {
		t.Fatalf("unexpected failover: measurement=%+v pools=%v err=%v", measurement, gateway.pools, err)
	}
}

func TestOpenAIChatCompletionsDispatcherNeverSwitchesAfterStreamCommit(t *testing.T) {
	selections := 0
	gateway := &openAIChatCompletionsGatewayStub{}
	gateway.selectAccount = func(_ *int64, _ map[int64]struct{}) (*service.AccountSelectionResult, error) {
		selections++
		return &service.AccountSelectionResult{
			Account:     &service.Account{ID: 521, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey},
			Acquired:    true,
			ReleaseFunc: func() {},
		}, nil
	}
	gateway.forward = func(exchange gatewaytransport.Exchange, _ *service.Account, _ []byte) (*service.OpenAIForwardResult, error) {
		exchange.Response().WriteHeader(http.StatusOK)
		_, _ = exchange.Response().Write([]byte("data: partial\n\n"))
		return &service.OpenAIForwardResult{
			Usage:         service.OpenAIUsage{InputTokens: 3, OutputTokens: 1},
			UpstreamModel: "gpt-5.4",
			Duration:      time.Millisecond,
		}, errors.New("committed stream failed")
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-5.4","messages":[],"stream":true}`))

	measurement, err := newOpenAIChatCompletionsDispatcherForTest(t, gateway).Forward(context.Background(), httptest.NewRecorder(), req, openAIChatCompletionsDispatchRequest())
	if err == nil || measurement == nil || selections != 1 || gateway.boundAccount != 0 {
		t.Fatalf("committed stream switched or bound: measurement=%+v selections=%d bound=%d err=%v", measurement, selections, gateway.boundAccount, err)
	}
}

func TestOpenAIChatCompletionsDispatcherRejectsModelMismatchBeforeScheduling(t *testing.T) {
	gateway := &openAIChatCompletionsGatewayStub{
		selectAccount: func(*int64, map[int64]struct{}) (*service.AccountSelectionResult, error) {
			t.Fatal("scheduler must not run")
			return nil, nil
		},
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-5.3","messages":[]}`))
	_, err := newOpenAIChatCompletionsDispatcherForTest(t, gateway).Forward(context.Background(), httptest.NewRecorder(), req, openAIChatCompletionsDispatchRequest())
	if !errors.Is(err, ErrInvocationModelMismatch) {
		t.Fatalf("expected model mismatch, got %v", err)
	}
}

func TestOpenAIChatCompletionsDispatcherRoutesLegacyCompletionsDirectly(t *testing.T) {
	account := &service.Account{ID: 531, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey}
	gateway := &openAIChatCompletionsGatewayStub{}
	gateway.selectAccount = func(_ *int64, _ map[int64]struct{}) (*service.AccountSelectionResult, error) {
		return &service.AccountSelectionResult{Account: account, Acquired: true, ReleaseFunc: func() {}}, nil
	}
	gateway.forward = func(exchange gatewaytransport.Exchange, _ *service.Account, body []byte) (*service.OpenAIForwardResult, error) {
		if exchange.Request().URL.Path != "/v1/completions" || string(body) != `{"model":"gpt-5.4","prompt":"hello"}` {
			t.Fatalf("legacy request was not preserved: path=%s body=%s", exchange.Request().URL.Path, body)
		}
		if err := exchange.WriteData(http.StatusOK, "application/json", []byte(`{"id":"cmpl_1"}`)); err != nil {
			return nil, err
		}
		return &service.OpenAIForwardResult{UpstreamModel: "gpt-5.4", UpstreamEndpoint: "/v1/completions", Duration: time.Millisecond}, nil
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/completions", strings.NewReader(`{"model":"gpt-5.4","prompt":"hello"}`))
	measurement, err := newOpenAIChatCompletionsDispatcherForTest(t, gateway).Forward(context.Background(), httptest.NewRecorder(), req, openAIChatCompletionsDispatchRequest())
	if err != nil || measurement.Endpoint != "/v1/completions" || measurement.AccountID != 531 {
		t.Fatalf("unexpected legacy measurement=%+v err=%v", measurement, err)
	}
}

func TestOpenAIChatCompletionsDispatcherAllowsSubscriptionAccount(t *testing.T) {
	account := &service.Account{ID: 541, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth}
	gateway := &openAIChatCompletionsGatewayStub{}
	gateway.selectAccount = func(_ *int64, _ map[int64]struct{}) (*service.AccountSelectionResult, error) {
		return &service.AccountSelectionResult{Account: account, Acquired: true, ReleaseFunc: func() {}}, nil
	}
	gateway.forward = func(exchange gatewaytransport.Exchange, got *service.Account, _ []byte) (*service.OpenAIForwardResult, error) {
		if got.Type != service.AccountTypeOAuth {
			t.Fatalf("subscription account was not preserved: %+v", got)
		}
		if err := exchange.WriteData(http.StatusOK, "application/json", []byte(`{"id":"chatcmpl_subscription"}`)); err != nil {
			return nil, err
		}
		return &service.OpenAIForwardResult{UpstreamModel: "gpt-5.4", UpstreamEndpoint: "/v1/responses", Duration: time.Millisecond}, nil
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hi"}]}`))
	measurement, err := newOpenAIChatCompletionsDispatcherForTest(t, gateway).Forward(context.Background(), httptest.NewRecorder(), req, openAIChatCompletionsDispatchRequest())
	if err != nil || measurement == nil || measurement.AccountID != 541 || gateway.unifiedSelects != 1 || gateway.directSelects != 0 {
		t.Fatalf("subscription dispatcher failed: measurement=%+v err=%v", measurement, err)
	}
}

func TestOpenAIChatCompletionsDispatcherRoutesLossySubscriptionShapeOnlyToDirectAccounts(t *testing.T) {
	account := &service.Account{ID: 551, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey}
	gateway := &openAIChatCompletionsGatewayStub{}
	gateway.selectAccount = func(_ *int64, _ map[int64]struct{}) (*service.AccountSelectionResult, error) {
		return &service.AccountSelectionResult{Account: account, Acquired: true, ReleaseFunc: func() {}}, nil
	}
	gateway.forward = func(exchange gatewaytransport.Exchange, _ *service.Account, body []byte) (*service.OpenAIForwardResult, error) {
		if !strings.Contains(string(body), `"stop":"END"`) {
			t.Fatalf("direct request lost unsupported subscription field: %s", body)
		}
		if err := exchange.WriteData(http.StatusOK, "application/json", []byte(`{"id":"chatcmpl_direct"}`)); err != nil {
			return nil, err
		}
		return &service.OpenAIForwardResult{UpstreamModel: "gpt-5.4", Duration: time.Millisecond}, nil
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hi"}],"stop":"END"}`))
	measurement, err := newOpenAIChatCompletionsDispatcherForTest(t, gateway).Forward(context.Background(), httptest.NewRecorder(), req, openAIChatCompletionsDispatchRequest())
	if err != nil || measurement == nil || gateway.directSelects != 1 || gateway.unifiedSelects != 0 {
		t.Fatalf("lossy shape routing failed: measurement=%+v direct=%d unified=%d err=%v", measurement, gateway.directSelects, gateway.unifiedSelects, err)
	}
}
