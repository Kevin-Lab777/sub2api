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
	selectAccount func(*int64, map[int64]struct{}) (*service.AccountSelectionResult, error)
	forward       func(gatewaytransport.Exchange, *service.Account, []byte) (*service.OpenAIForwardResult, error)
	pools         []int64
	boundPool     int64
	boundAccount  int64
	boundSession  string
}

func (s *openAIResponsesGatewayStub) ValidateTechnicalRuntime() error { return nil }

func (s *openAIResponsesGatewayStub) SelectTechnicalResponsesAccountWithLoadAwareness(_ context.Context, poolID *int64, _ string, _ string, excluded map[int64]struct{}) (*service.AccountSelectionResult, error) {
	if poolID != nil {
		s.pools = append(s.pools, *poolID)
	}
	return s.selectAccount(poolID, excluded)
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
