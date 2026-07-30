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

type openAIEmbeddingsGatewayStub struct {
	selectAccount func(*int64, map[int64]struct{}) (*service.AccountSelectionResult, error)
	forward       func(gatewaytransport.Exchange, *service.Account, []byte) (*service.OpenAIForwardResult, error)
	pools         []int64
	selectSession string
}

func (s *openAIEmbeddingsGatewayStub) ValidateTechnicalRuntime() error { return nil }

func (s *openAIEmbeddingsGatewayStub) SelectTechnicalEmbeddingsAccountWithLoadAwareness(_ context.Context, poolID *int64, session string, _ string, excluded map[int64]struct{}) (*service.AccountSelectionResult, error) {
	s.selectSession = session
	if poolID != nil {
		s.pools = append(s.pools, *poolID)
	}
	return s.selectAccount(poolID, excluded)
}

func (s *openAIEmbeddingsGatewayStub) AcquireSelection(_ context.Context, selection *service.AccountSelectionResult) (func(), error) {
	if selection == nil || selection.Account == nil {
		return nil, service.ErrInvalidAccountSelection
	}
	if selection.ReleaseFunc != nil {
		return selection.ReleaseFunc, nil
	}
	return func() {}, nil
}

func (s *openAIEmbeddingsGatewayStub) ForwardEmbeddingsExchange(_ context.Context, exchange gatewaytransport.Exchange, account *service.Account, body []byte) (*service.OpenAIForwardResult, error) {
	return s.forward(exchange, account, body)
}

func openAIEmbeddingsDispatchRequest() gatewaycore.DispatchRequest {
	return gatewaycore.DispatchRequest{
		Pool: gatewaycore.Pool{ID: 61, Platform: service.PlatformOpenAI, Active: true},
		Invocation: gatewaycore.Invocation{
			PoolID:    61,
			RequestID: "req-embeddings",
			SessionID: "session-embeddings",
			Protocol:  gatewaycore.ProtocolOpenAI,
			Model:     "embedding-alias",
			OnUsage:   func(context.Context, gatewaycore.Usage) error { return nil },
		},
	}
}

func newOpenAIEmbeddingsDispatcherForTest(t *testing.T, gateway *openAIEmbeddingsGatewayStub) *OpenAIEmbeddingsDispatcher {
	t.Helper()
	dispatcher, err := NewOpenAIEmbeddingsDispatcher(gateway, OpenAIEmbeddingsDispatcherConfig{MaxAccountSwitches: 3, MaxBodyBytes: 1 << 20})
	if err != nil {
		t.Fatalf("NewOpenAIEmbeddingsDispatcher: %v", err)
	}
	return dispatcher
}

func TestOpenAIEmbeddingsDispatcherUsesExactPoolAndMeasuresUsage(t *testing.T) {
	account := &service.Account{ID: 801, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey}
	gateway := &openAIEmbeddingsGatewayStub{}
	gateway.selectAccount = func(poolID *int64, excluded map[int64]struct{}) (*service.AccountSelectionResult, error) {
		if poolID == nil || *poolID != 61 || len(excluded) != 0 {
			t.Fatalf("unexpected selection pool=%v excluded=%v", poolID, excluded)
		}
		return &service.AccountSelectionResult{Account: account, Acquired: true, ReleaseFunc: func() {}}, nil
	}
	gateway.forward = func(exchange gatewaytransport.Exchange, got *service.Account, body []byte) (*service.OpenAIForwardResult, error) {
		if got.ID != 801 || string(body) != `{"model":"embedding-alias","input":"hello"}` {
			t.Fatalf("unexpected forward account=%d body=%s", got.ID, body)
		}
		if err := exchange.WriteData(http.StatusOK, "application/json", []byte(`{"object":"list","data":[]}`)); err != nil {
			return nil, err
		}
		return &service.OpenAIForwardResult{
			UpstreamModel: "text-embedding-3-large",
			Usage: service.OpenAIUsage{
				InputTokens:              20,
				ImageInputTokens:         5,
				CacheReadInputTokens:     3,
				CacheCreationInputTokens: 2,
			},
			Duration: 4 * time.Millisecond,
		}, nil
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/embeddings", strings.NewReader(`{"model":"embedding-alias","input":"hello"}`))
	measurement, err := newOpenAIEmbeddingsDispatcherForTest(t, gateway).Forward(context.Background(), httptest.NewRecorder(), req, openAIEmbeddingsDispatchRequest())
	if err != nil {
		t.Fatalf("Forward: %v", err)
	}
	if len(gateway.pools) != 1 || gateway.pools[0] != 61 || gateway.selectSession != "" {
		t.Fatalf("dispatcher left exact pool or used sticky state: %+v", gateway)
	}
	if measurement.AccountID != 801 || measurement.InputTokens != 20 || measurement.ImageInputTokens != 5 || measurement.CacheReadInputTokens != 3 || measurement.CacheWriteInputTokens != 2 || measurement.UpstreamModel != "text-embedding-3-large" {
		t.Fatalf("unexpected measurement: %+v", measurement)
	}
}

func TestOpenAIEmbeddingsDispatcherFailoverStaysInsidePool(t *testing.T) {
	accounts := []*service.Account{
		{ID: 811, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey},
		{ID: 812, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey},
	}
	gateway := &openAIEmbeddingsGatewayStub{}
	gateway.selectAccount = func(_ *int64, excluded map[int64]struct{}) (*service.AccountSelectionResult, error) {
		account := accounts[0]
		if _, failed := excluded[account.ID]; failed {
			account = accounts[1]
		}
		return &service.AccountSelectionResult{Account: account, Acquired: true, ReleaseFunc: func() {}}, nil
	}
	gateway.forward = func(exchange gatewaytransport.Exchange, account *service.Account, _ []byte) (*service.OpenAIForwardResult, error) {
		if account.ID == 811 {
			return nil, &service.UpstreamFailoverError{StatusCode: http.StatusTooManyRequests, NextAccountAction: service.NextAccountRetry}
		}
		if err := exchange.WriteData(http.StatusOK, "application/json", []byte(`{"object":"list","data":[]}`)); err != nil {
			return nil, err
		}
		return &service.OpenAIForwardResult{UpstreamModel: "text-embedding-3-large", Duration: time.Millisecond}, nil
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/embeddings", strings.NewReader(`{"model":"embedding-alias","input":"hello"}`))
	measurement, err := newOpenAIEmbeddingsDispatcherForTest(t, gateway).Forward(context.Background(), httptest.NewRecorder(), req, openAIEmbeddingsDispatchRequest())
	if err != nil || measurement.AccountID != 812 || len(gateway.pools) != 2 || gateway.pools[0] != 61 || gateway.pools[1] != 61 {
		t.Fatalf("unexpected pool failover measurement=%+v pools=%v err=%v", measurement, gateway.pools, err)
	}
}

func TestOpenAIEmbeddingsDispatcherNeverSwitchesAfterCommit(t *testing.T) {
	selections := 0
	gateway := &openAIEmbeddingsGatewayStub{}
	gateway.selectAccount = func(_ *int64, _ map[int64]struct{}) (*service.AccountSelectionResult, error) {
		selections++
		return &service.AccountSelectionResult{Account: &service.Account{ID: 821, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey}, Acquired: true, ReleaseFunc: func() {}}, nil
	}
	gateway.forward = func(exchange gatewaytransport.Exchange, _ *service.Account, _ []byte) (*service.OpenAIForwardResult, error) {
		_ = exchange.WriteData(http.StatusBadGateway, "application/json", []byte(`{"error":"written"}`))
		return nil, &service.UpstreamFailoverError{StatusCode: http.StatusBadGateway, NextAccountAction: service.NextAccountRetry}
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/embeddings", strings.NewReader(`{"model":"embedding-alias","input":"hello"}`))
	_, err := newOpenAIEmbeddingsDispatcherForTest(t, gateway).Forward(context.Background(), httptest.NewRecorder(), req, openAIEmbeddingsDispatchRequest())
	if err == nil || selections != 1 {
		t.Fatalf("committed response switched account: selections=%d err=%v", selections, err)
	}
}

func TestOpenAIEmbeddingsDispatcherRejectsInvalidRequestBeforeScheduling(t *testing.T) {
	gateway := &openAIEmbeddingsGatewayStub{
		selectAccount: func(*int64, map[int64]struct{}) (*service.AccountSelectionResult, error) {
			t.Fatal("scheduler must not run")
			return nil, nil
		},
		forward: func(gatewaytransport.Exchange, *service.Account, []byte) (*service.OpenAIForwardResult, error) {
			t.Fatal("forwarder must not run")
			return nil, nil
		},
	}
	dispatcher := newOpenAIEmbeddingsDispatcherForTest(t, gateway)

	wrongEndpoint := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"embedding-alias"}`))
	if _, err := dispatcher.Forward(context.Background(), httptest.NewRecorder(), wrongEndpoint, openAIEmbeddingsDispatchRequest()); err == nil {
		t.Fatal("expected endpoint error")
	}
	mismatch := httptest.NewRequest(http.MethodPost, "/v1/embeddings", strings.NewReader(`{"model":"other","input":"hello"}`))
	if _, err := dispatcher.Forward(context.Background(), httptest.NewRecorder(), mismatch, openAIEmbeddingsDispatchRequest()); !errors.Is(err, ErrInvocationModelMismatch) {
		t.Fatalf("expected model mismatch, got %v", err)
	}
	invalidJSON := httptest.NewRequest(http.MethodPost, "/v1/embeddings", strings.NewReader(`{"model":"embedding-alias","input":"hello
world"}`))
	if _, err := dispatcher.Forward(context.Background(), httptest.NewRecorder(), invalidJSON, openAIEmbeddingsDispatchRequest()); err == nil {
		t.Fatal("expected strict JSON error")
	}
}
