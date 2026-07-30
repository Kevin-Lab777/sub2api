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

type openAIResponsesGatewayStub struct {
	selectAccount          func(*int64, map[int64]struct{}) (*service.AccountSelectionResult, error)
	selectCompactAccount   func(*int64, map[int64]struct{}) (*service.AccountSelectionResult, error)
	selectWebSocketAccount func(*int64, map[int64]struct{}) (*service.AccountSelectionResult, error)
	forward                func(gatewaytransport.Exchange, *service.Account, []byte) (*service.OpenAIForwardResult, error)
	forwardCompact         func(gatewaytransport.Exchange, *service.Account, []byte) (*service.OpenAIForwardResult, error)
	forwardWebSocket       func(http.ResponseWriter, *http.Request, *service.Account, string, string) (*service.OpenAIResponsesWebSocketResult, error)
	pools                  []int64
	boundPool              int64
	boundAccount           int64
	boundSession           string
}

func (s *openAIResponsesGatewayStub) SelectTechnicalResponsesCompactAccountWithLoadAwareness(_ context.Context, poolID *int64, _ string, _ string, excluded map[int64]struct{}) (*service.AccountSelectionResult, error) {
	if poolID != nil {
		s.pools = append(s.pools, *poolID)
	}
	return s.selectCompactAccount(poolID, excluded)
}

func (s *openAIResponsesGatewayStub) ValidateTechnicalRuntime() error { return nil }

func (s *openAIResponsesGatewayStub) SelectTechnicalResponsesAccountWithLoadAwareness(_ context.Context, poolID *int64, _ string, _ string, excluded map[int64]struct{}) (*service.AccountSelectionResult, error) {
	if poolID != nil {
		s.pools = append(s.pools, *poolID)
	}
	return s.selectAccount(poolID, excluded)
}

func (s *openAIResponsesGatewayStub) SelectTechnicalResponsesWebSocketAccountWithLoadAwareness(_ context.Context, poolID *int64, _ string, _ string, excluded map[int64]struct{}) (*service.AccountSelectionResult, error) {
	if poolID != nil {
		s.pools = append(s.pools, *poolID)
	}
	return s.selectWebSocketAccount(poolID, excluded)
}

func (s *openAIResponsesGatewayStub) AcquireSelection(_ context.Context, selection *service.AccountSelectionResult) (func(), error) {
	if selection == nil || selection.Account == nil {
		return nil, service.ErrInvalidAccountSelection
	}
	if selection.ReleaseFunc != nil {
		return selection.ReleaseFunc, nil
	}
	return func() {}, nil
}

func (s *openAIResponsesGatewayStub) ForwardResponsesExchange(_ context.Context, exchange gatewaytransport.Exchange, account *service.Account, body []byte) (*service.OpenAIForwardResult, error) {
	return s.forward(exchange, account, body)
}

func (s *openAIResponsesGatewayStub) ForwardResponsesCompactExchange(_ context.Context, exchange gatewaytransport.Exchange, account *service.Account, body []byte) (*service.OpenAIForwardResult, error) {
	return s.forwardCompact(exchange, account, body)
}

func (s *openAIResponsesGatewayStub) ForwardResponsesWebSocket(_ context.Context, w http.ResponseWriter, req *http.Request, account *service.Account, model, sessionID string) (*service.OpenAIResponsesWebSocketResult, error) {
	return s.forwardWebSocket(w, req, account, model, sessionID)
}

func (s *openAIResponsesGatewayStub) BindStickySession(_ context.Context, poolID *int64, session string, accountID int64) error {
	if poolID != nil {
		s.boundPool = *poolID
	}
	s.boundSession = session
	s.boundAccount = accountID
	return nil
}

func openAIResponsesDispatchRequest() gatewaycore.DispatchRequest {
	return gatewaycore.DispatchRequest{
		Pool: gatewaycore.Pool{ID: 51, Platform: service.PlatformOpenAI, Active: true},
		Invocation: gatewaycore.Invocation{
			PoolID:    51,
			RequestID: "req-openai",
			SessionID: "session-openai",
			Protocol:  gatewaycore.ProtocolOpenAI,
			Model:     "gpt-5.4",
			OnUsage:   func(context.Context, gatewaycore.Usage) error { return nil },
		},
	}
}

func newOpenAIResponsesDispatcherForTest(t *testing.T, gateway *openAIResponsesGatewayStub) *OpenAIResponsesDispatcher {
	t.Helper()
	dispatcher, err := NewOpenAIResponsesDispatcher(gateway, OpenAIResponsesDispatcherConfig{
		MaxAccountSwitches: 3,
		MaxBodyBytes:       1 << 20,
	})
	if err != nil {
		t.Fatalf("NewOpenAIResponsesDispatcher: %v", err)
	}
	return dispatcher
}

func TestOpenAIResponsesDispatcherUsesExactPoolAndMeasuresUsage(t *testing.T) {
	account := &service.Account{ID: 101, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey}
	gateway := &openAIResponsesGatewayStub{}
	gateway.selectAccount = func(poolID *int64, excluded map[int64]struct{}) (*service.AccountSelectionResult, error) {
		if poolID == nil || *poolID != 51 || len(excluded) != 0 {
			t.Fatalf("unexpected selection: pool=%v excluded=%v", poolID, excluded)
		}
		return &service.AccountSelectionResult{Account: account, Acquired: true, ReleaseFunc: func() {}}, nil
	}
	gateway.forward = func(exchange gatewaytransport.Exchange, got *service.Account, body []byte) (*service.OpenAIForwardResult, error) {
		if got.ID != 101 || string(body) != `{"model":"gpt-5.4","input":"hello"}` {
			t.Fatalf("unexpected forward account=%d body=%s", got.ID, body)
		}
		if exchange.RequestHeader("session_id") != "session-openai" {
			t.Fatalf("invocation session was not bound to request")
		}
		if err := exchange.WriteData(http.StatusOK, "application/json", []byte(`{"id":"resp_1"}`)); err != nil {
			return nil, err
		}
		first := 3
		return &service.OpenAIForwardResult{
			Model:         "gpt-5.4",
			UpstreamModel: "gpt-5.4-2026-07-01",
			Usage: service.OpenAIUsage{
				InputTokens:              20,
				OutputTokens:             7,
				CacheReadInputTokens:     5,
				CacheCreationInputTokens: 2,
			},
			Duration:     6 * time.Millisecond,
			FirstTokenMs: &first,
		}, nil
	}

	body := `{"model":"gpt-5.4","input":"hello"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	measurement, err := newOpenAIResponsesDispatcherForTest(t, gateway).Forward(context.Background(), httptest.NewRecorder(), req, openAIResponsesDispatchRequest())
	if err != nil {
		t.Fatalf("Forward: %v", err)
	}
	if len(gateway.pools) != 1 || gateway.pools[0] != 51 || gateway.boundPool != 51 || gateway.boundAccount != 101 || gateway.boundSession != "session-openai" {
		t.Fatalf("dispatcher left exact pool/session: %+v", gateway)
	}
	if measurement.AccountID != 101 || measurement.InputTokens != 20 || measurement.OutputTokens != 7 || measurement.CacheReadInputTokens != 5 || measurement.CacheWriteInputTokens != 2 || measurement.UpstreamModel != "gpt-5.4-2026-07-01" {
		t.Fatalf("unexpected measurement: %+v", measurement)
	}
}

func TestOpenAIResponsesDispatcherRoutesCompactWithoutRewritingBody(t *testing.T) {
	account := &service.Account{ID: 102, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth}
	gateway := &openAIResponsesGatewayStub{
		selectAccount: func(*int64, map[int64]struct{}) (*service.AccountSelectionResult, error) {
			t.Fatal("regular Responses scheduler must not run")
			return nil, nil
		},
		forward: func(gatewaytransport.Exchange, *service.Account, []byte) (*service.OpenAIForwardResult, error) {
			t.Fatal("regular Responses forwarder must not run")
			return nil, nil
		},
	}
	gateway.selectCompactAccount = func(poolID *int64, excluded map[int64]struct{}) (*service.AccountSelectionResult, error) {
		if poolID == nil || *poolID != 51 || len(excluded) != 0 {
			t.Fatalf("unexpected compact selection: pool=%v excluded=%v", poolID, excluded)
		}
		return &service.AccountSelectionResult{Account: account, Acquired: true, ReleaseFunc: func() {}}, nil
	}
	gateway.forwardCompact = func(exchange gatewaytransport.Exchange, got *service.Account, body []byte) (*service.OpenAIForwardResult, error) {
		want := `{"model":"gpt-5.4","stream":true,"store":true,"input":"compact me"}`
		if got.ID != 102 || string(body) != want || exchange.Request().URL.Path != "/v1/responses/compact" {
			t.Fatalf("unexpected compact forward account=%d path=%s body=%s", got.ID, exchange.Request().URL.Path, body)
		}
		if err := exchange.WriteData(http.StatusOK, "application/json", []byte(`{"id":"cmp_1"}`)); err != nil {
			return nil, err
		}
		return &service.OpenAIForwardResult{
			Model:            "gpt-5.4",
			UpstreamModel:    "gpt-5.4-compact",
			UpstreamEndpoint: "/v1/responses/compact",
			Usage:            service.OpenAIUsage{InputTokens: 11, OutputTokens: 2},
			Duration:         time.Millisecond,
		}, nil
	}

	body := `{"model":"gpt-5.4","stream":true,"store":true,"input":"compact me"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/responses/compact", strings.NewReader(body))
	measurement, err := newOpenAIResponsesDispatcherForTest(t, gateway).Forward(context.Background(), httptest.NewRecorder(), req, openAIResponsesDispatchRequest())
	if err != nil {
		t.Fatalf("Forward compact: %v", err)
	}
	if measurement.Endpoint != "/v1/responses/compact" || measurement.AccountID != 102 || measurement.InputTokens != 11 || measurement.OutputTokens != 2 {
		t.Fatalf("unexpected compact measurement: %+v", measurement)
	}
}

func TestOpenAIResponsesDispatcherFailoverStaysInsidePool(t *testing.T) {
	accounts := []*service.Account{
		{ID: 111, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey},
		{ID: 112, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey},
	}
	gateway := &openAIResponsesGatewayStub{}
	gateway.selectAccount = func(_ *int64, excluded map[int64]struct{}) (*service.AccountSelectionResult, error) {
		account := accounts[0]
		if _, failed := excluded[account.ID]; failed {
			account = accounts[1]
		}
		return &service.AccountSelectionResult{Account: account, Acquired: true, ReleaseFunc: func() {}}, nil
	}
	gateway.forward = func(exchange gatewaytransport.Exchange, account *service.Account, _ []byte) (*service.OpenAIForwardResult, error) {
		if account.ID == 111 {
			return nil, &service.UpstreamFailoverError{StatusCode: http.StatusTooManyRequests, NextAccountAction: service.NextAccountRetry}
		}
		if err := exchange.WriteData(http.StatusOK, "application/json", []byte(`{"id":"resp_2"}`)); err != nil {
			return nil, err
		}
		return &service.OpenAIForwardResult{Model: "gpt-5.4", Usage: service.OpenAIUsage{}, Duration: time.Millisecond}, nil
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-5.4","input":"hello"}`))
	measurement, err := newOpenAIResponsesDispatcherForTest(t, gateway).Forward(context.Background(), httptest.NewRecorder(), req, openAIResponsesDispatchRequest())
	if err != nil || measurement.AccountID != 112 || len(gateway.pools) != 2 || gateway.pools[0] != 51 || gateway.pools[1] != 51 {
		t.Fatalf("unexpected pool failover: measurement=%+v pools=%v err=%v", measurement, gateway.pools, err)
	}
}

func TestOpenAIResponsesDispatcherStopsAfterCommit(t *testing.T) {
	gateway := &openAIResponsesGatewayStub{}
	selections := 0
	gateway.selectAccount = func(_ *int64, _ map[int64]struct{}) (*service.AccountSelectionResult, error) {
		selections++
		return &service.AccountSelectionResult{Account: &service.Account{ID: 120, Platform: service.PlatformOpenAI}, Acquired: true, ReleaseFunc: func() {}}, nil
	}
	gateway.forward = func(exchange gatewaytransport.Exchange, _ *service.Account, _ []byte) (*service.OpenAIForwardResult, error) {
		_ = exchange.WriteData(http.StatusBadGateway, "application/json", []byte(`{"error":"written"}`))
		return nil, &service.UpstreamFailoverError{StatusCode: http.StatusBadGateway, NextAccountAction: service.NextAccountRetry}
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-5.4"}`))
	_, err := newOpenAIResponsesDispatcherForTest(t, gateway).Forward(context.Background(), httptest.NewRecorder(), req, openAIResponsesDispatchRequest())
	if err == nil || selections != 1 {
		t.Fatalf("committed response did not stop failover: err=%v selections=%d", err, selections)
	}
}

func TestOpenAIResponsesDispatcherRejectsUnsupportedEndpointAndModelMismatch(t *testing.T) {
	gateway := &openAIResponsesGatewayStub{
		selectAccount: func(*int64, map[int64]struct{}) (*service.AccountSelectionResult, error) {
			t.Fatal("scheduler must not run")
			return nil, nil
		},
		forward: func(gatewaytransport.Exchange, *service.Account, []byte) (*service.OpenAIForwardResult, error) {
			t.Fatal("forwarder must not run")
			return nil, nil
		},
	}
	dispatcher := newOpenAIResponsesDispatcherForTest(t, gateway)
	wrongEndpoint := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-5.4"}`))
	if _, err := dispatcher.Forward(context.Background(), httptest.NewRecorder(), wrongEndpoint, openAIResponsesDispatchRequest()); err == nil {
		t.Fatal("expected unsupported endpoint error")
	}
	mismatch := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-5.3"}`))
	if _, err := dispatcher.Forward(context.Background(), httptest.NewRecorder(), mismatch, openAIResponsesDispatchRequest()); !errors.Is(err, ErrInvocationModelMismatch) {
		t.Fatalf("expected model mismatch, got %v", err)
	}
}

func TestOpenAIResponsesDispatcherDoesNotRepairInvalidJSON(t *testing.T) {
	gateway := &openAIResponsesGatewayStub{
		selectAccount: func(*int64, map[int64]struct{}) (*service.AccountSelectionResult, error) {
			t.Fatal("scheduler must not run")
			return nil, nil
		},
		forward: func(gatewaytransport.Exchange, *service.Account, []byte) (*service.OpenAIForwardResult, error) {
			t.Fatal("forwarder must not run")
			return nil, nil
		},
	}
	body := "{\"model\":\"gpt-5.4\",\"input\":\"hello\nworld\"}"
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))

	_, err := newOpenAIResponsesDispatcherForTest(t, gateway).Forward(context.Background(), httptest.NewRecorder(), req, openAIResponsesDispatchRequest())
	if err == nil || !strings.Contains(err.Error(), "valid JSON") {
		t.Fatalf("expected strict JSON rejection, got %v", err)
	}
}

func TestOpenAIResponsesDispatcherWebSocketFailoverBeforeCommitAndMeasuresConnection(t *testing.T) {
	accounts := []*service.Account{
		{ID: 201, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey},
		{ID: 202, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey},
	}
	gateway := &openAIResponsesGatewayStub{}
	gateway.selectWebSocketAccount = func(poolID *int64, excluded map[int64]struct{}) (*service.AccountSelectionResult, error) {
		if poolID == nil || *poolID != 51 {
			t.Fatalf("unexpected WebSocket pool %v", poolID)
		}
		account := accounts[0]
		if _, failed := excluded[account.ID]; failed {
			account = accounts[1]
		}
		return &service.AccountSelectionResult{Account: account, Acquired: true, ReleaseFunc: func() {}}, nil
	}
	startedAt := time.Now()
	gateway.forwardWebSocket = func(_ http.ResponseWriter, req *http.Request, account *service.Account, model, sessionID string) (*service.OpenAIResponsesWebSocketResult, error) {
		if req.Method != http.MethodGet || req.URL.Path != "/v1/responses" || model != "gpt-5.4" || sessionID != "session-openai" {
			t.Fatalf("unexpected WebSocket invocation: method=%s path=%s model=%s session=%s", req.Method, req.URL.Path, model, sessionID)
		}
		if account.ID == 201 {
			return nil, &service.UpstreamFailoverError{
				StatusCode:        http.StatusBadGateway,
				NextAccountAction: service.NextAccountRetry,
			}
		}
		return &service.OpenAIResponsesWebSocketResult{
			Usage: service.OpenAIUsage{
				InputTokens:              17,
				OutputTokens:             9,
				CacheReadInputTokens:     4,
				CacheCreationInputTokens: 2,
			},
			UpstreamModel:       "gpt-5.4-upstream",
			ImageCount:          1,
			WebSearchCalls:      2,
			UpstreamStatusCode:  http.StatusSwitchingProtocols,
			StartedAt:           startedAt,
			Duration:            5 * time.Millisecond,
			TimeToFirstResponse: time.Millisecond,
		}, nil
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	req.Header.Set("Connection", "keep-alive, Upgrade")
	req.Header.Set("Upgrade", "websocket")
	measurement, err := newOpenAIResponsesDispatcherForTest(t, gateway).Forward(context.Background(), httptest.NewRecorder(), req, openAIResponsesDispatchRequest())
	if err != nil {
		t.Fatalf("Forward WebSocket: %v", err)
	}
	if measurement.AccountID != 202 || measurement.UpstreamStatusCode != http.StatusSwitchingProtocols || measurement.InputTokens != 17 || measurement.OutputTokens != 9 || measurement.ImageCount != 1 || measurement.WebSearchCalls != 2 {
		t.Fatalf("unexpected WebSocket measurement: %+v", measurement)
	}
	if len(gateway.pools) != 2 || gateway.boundPool != 51 || gateway.boundAccount != 202 || gateway.boundSession != "session-openai" {
		t.Fatalf("unexpected WebSocket pool or binding state: %+v", gateway)
	}
}

func TestOpenAIResponsesDispatcherWebSocketNeverSwitchesAfterCommit(t *testing.T) {
	selections := 0
	gateway := &openAIResponsesGatewayStub{}
	gateway.selectWebSocketAccount = func(_ *int64, _ map[int64]struct{}) (*service.AccountSelectionResult, error) {
		selections++
		return &service.AccountSelectionResult{
			Account:     &service.Account{ID: 203, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey},
			Acquired:    true,
			ReleaseFunc: func() {},
		}, nil
	}
	gateway.forwardWebSocket = func(http.ResponseWriter, *http.Request, *service.Account, string, string) (*service.OpenAIResponsesWebSocketResult, error) {
		return &service.OpenAIResponsesWebSocketResult{
			Usage:              service.OpenAIUsage{InputTokens: 3, OutputTokens: 1},
			UpstreamModel:      "gpt-5.4",
			UpstreamStatusCode: http.StatusSwitchingProtocols,
			StartedAt:          time.Now(),
			Duration:           time.Millisecond,
		}, errors.New("committed WebSocket relay failed")
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")

	measurement, err := newOpenAIResponsesDispatcherForTest(t, gateway).Forward(context.Background(), httptest.NewRecorder(), req, openAIResponsesDispatchRequest())
	if err == nil || measurement == nil || selections != 1 || gateway.boundAccount != 0 {
		t.Fatalf("committed WebSocket switched or bound unexpectedly: measurement=%+v selections=%d bound=%d err=%v", measurement, selections, gateway.boundAccount, err)
	}
}
