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

type openAIImagesGatewayStub struct {
	selectAccount func(*int64, map[int64]struct{}) (*service.AccountSelectionResult, error)
	forward       func(gatewaytransport.Exchange, *service.Account, []byte) (*service.OpenAIForwardResult, error)
	pools         []int64
	selectSession string
}

func (s *openAIImagesGatewayStub) ValidateTechnicalRuntime() error { return nil }

func (s *openAIImagesGatewayStub) SelectTechnicalImagesDirectAccountWithLoadAwareness(_ context.Context, poolID *int64, session string, _ string, excluded map[int64]struct{}) (*service.AccountSelectionResult, error) {
	if poolID != nil {
		s.pools = append(s.pools, *poolID)
	}
	s.selectSession = session
	return s.selectAccount(poolID, excluded)
}

func (s *openAIImagesGatewayStub) AcquireSelection(_ context.Context, selection *service.AccountSelectionResult) (func(), error) {
	if selection == nil || selection.Account == nil {
		return nil, service.ErrInvalidAccountSelection
	}
	if selection.ReleaseFunc != nil {
		return selection.ReleaseFunc, nil
	}
	return func() {}, nil
}

func (s *openAIImagesGatewayStub) ForwardImagesDirectExchange(_ context.Context, exchange gatewaytransport.Exchange, account *service.Account, body []byte) (*service.OpenAIForwardResult, error) {
	return s.forward(exchange, account, body)
}

func openAIImagesDispatchRequest() gatewaycore.DispatchRequest {
	return gatewaycore.DispatchRequest{
		Pool: gatewaycore.Pool{ID: 71, Platform: service.PlatformOpenAI, Active: true},
		Invocation: gatewaycore.Invocation{
			PoolID:    71,
			RequestID: "req-images",
			SessionID: "session-images",
			Protocol:  gatewaycore.ProtocolOpenAI,
			Model:     "image-alias",
			OnUsage:   func(context.Context, gatewaycore.Usage) error { return nil },
		},
	}
}

func newOpenAIImagesDispatcherForTest(t *testing.T, gateway *openAIImagesGatewayStub) *OpenAIImagesDispatcher {
	t.Helper()
	dispatcher, err := NewOpenAIImagesDispatcher(gateway, OpenAIImagesDispatcherConfig{MaxAccountSwitches: 3, MaxBodyBytes: 1 << 20})
	if err != nil {
		t.Fatalf("NewOpenAIImagesDispatcher: %v", err)
	}
	return dispatcher
}

func TestOpenAIImagesDispatcherUsesExactPoolWithoutStickyCache(t *testing.T) {
	account := &service.Account{ID: 1001, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey}
	gateway := &openAIImagesGatewayStub{}
	gateway.selectAccount = func(poolID *int64, excluded map[int64]struct{}) (*service.AccountSelectionResult, error) {
		if poolID == nil || *poolID != 71 || len(excluded) != 0 {
			t.Fatalf("unexpected selection pool=%v excluded=%v", poolID, excluded)
		}
		return &service.AccountSelectionResult{Account: account, Acquired: true, ReleaseFunc: func() {}}, nil
	}
	gateway.forward = func(exchange gatewaytransport.Exchange, got *service.Account, body []byte) (*service.OpenAIForwardResult, error) {
		if got.ID != 1001 || string(body) != `{"model":"image-alias","prompt":"cat"}` {
			t.Fatalf("unexpected forward account=%d body=%s", got.ID, body)
		}
		if err := exchange.WriteData(http.StatusOK, "application/json", []byte(`{"data":[{"b64_json":"aW1hZ2U="}]}`)); err != nil {
			return nil, err
		}
		first := 3
		return &service.OpenAIForwardResult{
			UpstreamModel: "gpt-image-2",
			Usage: service.OpenAIUsage{
				InputTokens:       40,
				ImageInputTokens:  10,
				OutputTokens:      50,
				ImageOutputTokens: 50,
			},
			ImageCount:       1,
			ImageOutputSizes: []string{"1024x1024"},
			Duration:         6 * time.Millisecond,
			FirstTokenMs:     &first,
		}, nil
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"model":"image-alias","prompt":"cat"}`))
	req.Header.Set("Content-Type", "application/json")

	measurement, err := newOpenAIImagesDispatcherForTest(t, gateway).Forward(context.Background(), httptest.NewRecorder(), req, openAIImagesDispatchRequest())
	if err != nil {
		t.Fatalf("Forward: %v", err)
	}
	if len(gateway.pools) != 1 || gateway.pools[0] != 71 || gateway.selectSession != "" {
		t.Fatalf("dispatcher left exact pool or used sticky state: %+v", gateway)
	}
	if measurement.AccountID != 1001 || measurement.ImageCount != 1 || len(measurement.ImageOutputSizes) != 1 || measurement.ImageOutputSizes[0] != "1024x1024" || measurement.ImageInputTokens != 10 || measurement.ImageOutputTokens != 50 || measurement.UpstreamModel != "gpt-image-2" {
		t.Fatalf("unexpected measurement: %+v", measurement)
	}
}

func TestOpenAIImagesDispatcherFailoverStaysInsidePool(t *testing.T) {
	accounts := []*service.Account{
		{ID: 1011, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey},
		{ID: 1012, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey},
	}
	gateway := &openAIImagesGatewayStub{}
	gateway.selectAccount = func(_ *int64, excluded map[int64]struct{}) (*service.AccountSelectionResult, error) {
		account := accounts[0]
		if _, failed := excluded[account.ID]; failed {
			account = accounts[1]
		}
		return &service.AccountSelectionResult{Account: account, Acquired: true, ReleaseFunc: func() {}}, nil
	}
	gateway.forward = func(exchange gatewaytransport.Exchange, account *service.Account, _ []byte) (*service.OpenAIForwardResult, error) {
		if account.ID == 1011 {
			return nil, &service.UpstreamFailoverError{StatusCode: http.StatusTooManyRequests, NextAccountAction: service.NextAccountRetry}
		}
		if err := exchange.WriteData(http.StatusOK, "application/json", []byte(`{"data":[{"b64_json":"aW1hZ2U="}]}`)); err != nil {
			return nil, err
		}
		return &service.OpenAIForwardResult{UpstreamModel: "gpt-image-2", ImageCount: 1, Duration: time.Millisecond}, nil
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"model":"image-alias","prompt":"cat"}`))
	req.Header.Set("Content-Type", "application/json")

	measurement, err := newOpenAIImagesDispatcherForTest(t, gateway).Forward(context.Background(), httptest.NewRecorder(), req, openAIImagesDispatchRequest())
	if err != nil || measurement.AccountID != 1012 || len(gateway.pools) != 2 || gateway.pools[0] != 71 || gateway.pools[1] != 71 {
		t.Fatalf("unexpected pool failover measurement=%+v pools=%v err=%v", measurement, gateway.pools, err)
	}
}

func TestOpenAIImagesDispatcherNeverSwitchesAfterResponseCommit(t *testing.T) {
	selections := 0
	gateway := &openAIImagesGatewayStub{}
	gateway.selectAccount = func(_ *int64, _ map[int64]struct{}) (*service.AccountSelectionResult, error) {
		selections++
		return &service.AccountSelectionResult{Account: &service.Account{ID: 1021, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey}, Acquired: true, ReleaseFunc: func() {}}, nil
	}
	gateway.forward = func(exchange gatewaytransport.Exchange, _ *service.Account, _ []byte) (*service.OpenAIForwardResult, error) {
		_ = exchange.WriteData(http.StatusOK, "application/json", []byte(`{"data":[]}`))
		return nil, &service.UpstreamFailoverError{StatusCode: http.StatusBadGateway, NextAccountAction: service.NextAccountRetry}
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"model":"image-alias","prompt":"cat"}`))
	req.Header.Set("Content-Type", "application/json")

	_, err := newOpenAIImagesDispatcherForTest(t, gateway).Forward(context.Background(), httptest.NewRecorder(), req, openAIImagesDispatchRequest())
	if err == nil || selections != 1 {
		t.Fatalf("committed response switched accounts selections=%d err=%v", selections, err)
	}
}

func TestOpenAIImagesDispatcherReturnsMeasurementWithCommittedProtocolError(t *testing.T) {
	selections := 0
	gateway := &openAIImagesGatewayStub{}
	gateway.selectAccount = func(_ *int64, _ map[int64]struct{}) (*service.AccountSelectionResult, error) {
		selections++
		return &service.AccountSelectionResult{Account: &service.Account{ID: 1031, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey}, Acquired: true, ReleaseFunc: func() {}}, nil
	}
	gateway.forward = func(exchange gatewaytransport.Exchange, _ *service.Account, _ []byte) (*service.OpenAIForwardResult, error) {
		_ = exchange.WriteData(http.StatusOK, "application/json", []byte(`{"data":[{"b64_json":"aW1hZ2U="}]}`))
		return &service.OpenAIForwardResult{UpstreamModel: "gpt-image-2", Usage: service.OpenAIUsage{InputTokens: 2, OutputTokens: 3, ImageOutputTokens: 3}, ImageCount: 1, Duration: time.Millisecond}, errors.New("upstream returned 1 image for n=2")
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"model":"image-alias","prompt":"cat","n":2}`))
	req.Header.Set("Content-Type", "application/json")

	measurement, err := newOpenAIImagesDispatcherForTest(t, gateway).Forward(context.Background(), httptest.NewRecorder(), req, openAIImagesDispatchRequest())
	if err == nil || measurement == nil || measurement.ImageCount != 1 || measurement.OutputTokens != 3 || selections != 1 {
		t.Fatalf("committed protocol error lost measurement or switched: measurement=%+v selections=%d err=%v", measurement, selections, err)
	}
}

func TestOpenAIImagesDispatcherRejectsInvalidRequestBeforeScheduling(t *testing.T) {
	gateway := &openAIImagesGatewayStub{
		selectAccount: func(*int64, map[int64]struct{}) (*service.AccountSelectionResult, error) {
			t.Fatal("scheduler must not run")
			return nil, nil
		},
		forward: func(gatewaytransport.Exchange, *service.Account, []byte) (*service.OpenAIForwardResult, error) {
			t.Fatal("forwarder must not run")
			return nil, nil
		},
	}
	dispatcher := newOpenAIImagesDispatcherForTest(t, gateway)

	wrongEndpoint := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"image-alias"}`))
	if _, err := dispatcher.Forward(context.Background(), httptest.NewRecorder(), wrongEndpoint, openAIImagesDispatchRequest()); err == nil {
		t.Fatal("expected endpoint error")
	}
	mismatch := httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"model":"other","prompt":"cat"}`))
	mismatch.Header.Set("Content-Type", "application/json")
	if _, err := dispatcher.Forward(context.Background(), httptest.NewRecorder(), mismatch, openAIImagesDispatchRequest()); !errors.Is(err, ErrInvocationModelMismatch) {
		t.Fatalf("expected model mismatch, got %v", err)
	}
	invalid := httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"model":"image-alias","stream":null}`))
	invalid.Header.Set("Content-Type", "application/json")
	if _, err := dispatcher.Forward(context.Background(), httptest.NewRecorder(), invalid, openAIImagesDispatchRequest()); err == nil {
		t.Fatal("expected strict stream error")
	}
}
