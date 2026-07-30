package gatewayruntime

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/gatewaycore"
	"github.com/Wei-Shaw/sub2api/internal/service"
	coderws "github.com/coder/websocket"
)

type openAILiveGatewayStub struct {
	selectAccount func(*int64, map[int64]struct{}) (*service.AccountSelectionResult, error)
	create        func(*service.LiveCallRequest, service.TechnicalLiveCallIdentity, *service.Account) (*service.LiveCallCreated, error)
	getCall       func(string, int64, string) (*service.LiveCallRecord, error)
	proxy         func(*service.LiveCallRecord, *coderws.Conn) error
	pools         []int64
	leases        []int64
	releases      []int64
}

func (s *openAILiveGatewayStub) ValidateTechnicalLiveRuntime() error { return nil }

func (s *openAILiveGatewayStub) SelectTechnicalLiveAccountWithLoadAwareness(_ context.Context, poolID *int64, _ string, excluded map[int64]struct{}) (*service.AccountSelectionResult, error) {
	if poolID != nil {
		s.pools = append(s.pools, *poolID)
	}
	return s.selectAccount(poolID, excluded)
}

func (s *openAILiveGatewayStub) AcquireSelection(_ context.Context, selection *service.AccountSelectionResult) (func(), error) {
	if selection == nil || selection.Account == nil {
		return nil, service.ErrInvalidAccountSelection
	}
	if selection.ReleaseFunc != nil {
		return selection.ReleaseFunc, nil
	}
	return func() {}, nil
}

func (s *openAILiveGatewayStub) AcquireTechnicalLiveLease(_ context.Context, account *service.Account, _ string) error {
	s.leases = append(s.leases, account.ID)
	return nil
}

func (s *openAILiveGatewayStub) ReleaseTechnicalLiveLease(_ context.Context, accountID int64, _ string) error {
	s.releases = append(s.releases, accountID)
	return nil
}

func (s *openAILiveGatewayStub) CreateTechnicalLiveCall(_ context.Context, request *service.LiveCallRequest, identity service.TechnicalLiveCallIdentity, account *service.Account) (*service.LiveCallCreated, error) {
	return s.create(request, identity, account)
}

func (s *openAILiveGatewayStub) GetTechnicalLiveCall(_ context.Context, callID string, poolID int64, sessionID string) (*service.LiveCallRecord, error) {
	return s.getCall(callID, poolID, sessionID)
}

func (s *openAILiveGatewayStub) ProxyLiveSideband(_ context.Context, record *service.LiveCallRecord, downstream *coderws.Conn) error {
	return s.proxy(record, downstream)
}

func openAILiveDispatchRequest() gatewaycore.DispatchRequest {
	return gatewaycore.DispatchRequest{
		Pool: gatewaycore.Pool{ID: 81, Platform: service.PlatformOpenAI, Active: true, AllowLive: true},
		Invocation: gatewaycore.Invocation{
			PoolID:    81,
			RequestID: "request-live-1",
			SessionID: "session-live-1",
			Protocol:  gatewaycore.ProtocolOpenAI,
			Model:     "live-alias",
			OnUsage:   func(context.Context, gatewaycore.Usage) error { return nil },
		},
	}
}

func newOpenAILiveDispatcherForTest(t *testing.T, gateway *openAILiveGatewayStub) *OpenAILiveDispatcher {
	t.Helper()
	dispatcher, err := NewOpenAILiveDispatcher(gateway, OpenAILiveDispatcherConfig{MaxAccountSwitches: 3, MaxBodyBytes: 1 << 20})
	if err != nil {
		t.Fatalf("NewOpenAILiveDispatcher: %v", err)
	}
	return dispatcher
}

func technicalLiveAccount(id int64) *service.Account {
	return &service.Account{ID: id, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Concurrency: 1}
}

func TestOpenAILiveDispatcherCreatesInsideExactPoolWithoutCustomerIdentity(t *testing.T) {
	account := technicalLiveAccount(1101)
	gateway := &openAILiveGatewayStub{}
	gateway.selectAccount = func(poolID *int64, excluded map[int64]struct{}) (*service.AccountSelectionResult, error) {
		if poolID == nil || *poolID != 81 || len(excluded) != 0 {
			t.Fatalf("unexpected selection pool=%v excluded=%v", poolID, excluded)
		}
		return &service.AccountSelectionResult{Account: account, Acquired: true, ReleaseFunc: func() {}}, nil
	}
	gateway.create = func(request *service.LiveCallRequest, identity service.TechnicalLiveCallIdentity, got *service.Account) (*service.LiveCallCreated, error) {
		if got.ID != account.ID || request.SDP != "v=offer\r\n" {
			t.Fatalf("unexpected create account=%d request=%+v", got.ID, request)
		}
		if identity.PoolID != 81 || identity.RequestID != "request-live-1" || identity.SessionID != "session-live-1" || identity.InboundEndpoint != "/v1/live" {
			t.Fatalf("unexpected technical identity: %+v", identity)
		}
		return &service.LiveCallCreated{SDP: []byte("v=answer\r\n"), CallID: "call_technical", Account: got, UpstreamModel: "gpt-live-upstream"}, nil
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/live", strings.NewReader(`{"sdp":"v=offer\r\n","session":{"model":"live-alias","voice":"alloy"}}`))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	measurement, err := newOpenAILiveDispatcherForTest(t, gateway).Forward(context.Background(), recorder, req, openAILiveDispatchRequest())
	if err != nil {
		t.Fatalf("Forward: %v", err)
	}
	if recorder.Code != http.StatusOK || recorder.Body.String() != "v=answer\r\n" || recorder.Header().Get("Location") != "/v1/live/call_technical" {
		t.Fatalf("unexpected response code=%d location=%q body=%q", recorder.Code, recorder.Header().Get("Location"), recorder.Body.String())
	}
	if measurement.AccountID != account.ID || measurement.UpstreamModel != "gpt-live-upstream" || measurement.UpstreamStatusCode != http.StatusOK {
		t.Fatalf("unexpected measurement: %+v", measurement)
	}
	if len(gateway.pools) != 1 || gateway.pools[0] != 81 || len(gateway.leases) != 1 || len(gateway.releases) != 0 {
		t.Fatalf("unexpected scheduling state: %+v", gateway)
	}
}

func TestOpenAILiveDispatcherFailoverReleasesOnlyFailedAccountLease(t *testing.T) {
	accounts := []*service.Account{technicalLiveAccount(1111), technicalLiveAccount(1112)}
	gateway := &openAILiveGatewayStub{}
	gateway.selectAccount = func(_ *int64, excluded map[int64]struct{}) (*service.AccountSelectionResult, error) {
		account := accounts[0]
		if _, failed := excluded[account.ID]; failed {
			account = accounts[1]
		}
		return &service.AccountSelectionResult{Account: account, Acquired: true, ReleaseFunc: func() {}}, nil
	}
	gateway.create = func(_ *service.LiveCallRequest, _ service.TechnicalLiveCallIdentity, account *service.Account) (*service.LiveCallCreated, error) {
		if account.ID == accounts[0].ID {
			return nil, &service.UpstreamFailoverError{StatusCode: http.StatusTooManyRequests, Scope: service.GatewayFailureScopeAccount, NextAccountAction: service.NextAccountRetry}
		}
		return &service.LiveCallCreated{SDP: []byte("v=answer"), CallID: "call_second", Account: account, UpstreamModel: "gpt-live-upstream"}, nil
	}
	req := httptest.NewRequest(http.MethodPost, "/backend-api/codex/realtime/calls", strings.NewReader(`{"sdp":"v=offer","session":{"model":"live-alias"}}`))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	measurement, err := newOpenAILiveDispatcherForTest(t, gateway).Forward(context.Background(), recorder, req, openAILiveDispatchRequest())
	if err != nil || measurement.AccountID != accounts[1].ID {
		t.Fatalf("unexpected failover measurement=%+v err=%v", measurement, err)
	}
	if len(gateway.pools) != 2 || gateway.pools[0] != 81 || gateway.pools[1] != 81 || len(gateway.releases) != 1 || gateway.releases[0] != accounts[0].ID {
		t.Fatalf("failover left technical pool or lease state: pools=%v releases=%v", gateway.pools, gateway.releases)
	}
	if recorder.Header().Get("Location") != "/backend-api/codex/call_second" {
		t.Fatalf("unexpected location %q", recorder.Header().Get("Location"))
	}
}

func TestOpenAILiveDispatcherRejectsDisabledPoolAndModelMismatchBeforeScheduling(t *testing.T) {
	gateway := &openAILiveGatewayStub{
		selectAccount: func(*int64, map[int64]struct{}) (*service.AccountSelectionResult, error) {
			t.Fatal("scheduler must not run")
			return nil, nil
		},
	}
	dispatcher := newOpenAILiveDispatcherForTest(t, gateway)
	body := `{"sdp":"v=offer","session":{"model":"other-model"}}`
	req := httptest.NewRequest(http.MethodPost, "/v1/live", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	_, err := dispatcher.Forward(context.Background(), httptest.NewRecorder(), req, openAILiveDispatchRequest())
	if !errors.Is(err, ErrInvocationModelMismatch) {
		t.Fatalf("expected model mismatch, got %v", err)
	}
	dispatch := openAILiveDispatchRequest()
	dispatch.Pool.AllowLive = false
	body = `{"sdp":"v=offer","session":{"model":"live-alias"}}`
	req = httptest.NewRequest(http.MethodPost, "/v1/live", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if _, err := dispatcher.Forward(context.Background(), httptest.NewRecorder(), req, dispatch); err == nil {
		t.Fatal("expected disabled pool error")
	}
}

func TestOpenAILiveDispatcherProxiesTechnicalSideband(t *testing.T) {
	record := &service.LiveCallRecord{
		CallID:           "call_sideband",
		AccountID:        1201,
		Model:            "live-alias",
		UpstreamModel:    "gpt-live-upstream",
		Technical:        true,
		TechnicalPoolID:  81,
		TechnicalSession: "session-live-1",
	}
	gateway := &openAILiveGatewayStub{}
	gateway.getCall = func(callID string, poolID int64, sessionID string) (*service.LiveCallRecord, error) {
		if callID != record.CallID || poolID != 81 || sessionID != record.TechnicalSession {
			t.Fatalf("unexpected sideband identity call=%q pool=%d session=%q", callID, poolID, sessionID)
		}
		return record, nil
	}
	gateway.proxy = func(got *service.LiveCallRecord, downstream *coderws.Conn) error {
		if got != record {
			t.Fatal("unexpected Live record")
		}
		messageType, payload, err := downstream.Read(context.Background())
		if err != nil {
			return err
		}
		if err := downstream.Write(context.Background(), messageType, payload); err != nil {
			return err
		}
		return service.ErrLiveCallNotFound
	}
	dispatcher := newOpenAILiveDispatcherForTest(t, gateway)
	type result struct {
		measurement *gatewaycore.Measurement
		err         error
	}
	resultCh := make(chan result, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		measurement, err := dispatcher.Forward(req.Context(), w, req, openAILiveDispatchRequest())
		resultCh <- result{measurement: measurement, err: err}
	}))
	defer server.Close()

	client, _, err := coderws.Dial(context.Background(), "ws"+strings.TrimPrefix(server.URL, "http")+"/v1/live/call_sideband", nil)
	if err != nil {
		t.Fatalf("dial sideband: %v", err)
	}
	defer client.CloseNow()
	if err := client.Write(context.Background(), coderws.MessageBinary, []byte{1, 2, 3}); err != nil {
		t.Fatalf("write sideband: %v", err)
	}
	messageType, payload, err := client.Read(context.Background())
	if err != nil || messageType != coderws.MessageBinary || string(payload) != string([]byte{1, 2, 3}) {
		t.Fatalf("unexpected sideband echo type=%v payload=%v err=%v", messageType, payload, err)
	}
	observed := <-resultCh
	if observed.err != nil || observed.measurement == nil || observed.measurement.AccountID != record.AccountID || observed.measurement.UpstreamStatusCode != http.StatusSwitchingProtocols || observed.measurement.Endpoint != "/v1/live/:call_id" {
		t.Fatalf("unexpected sideband result measurement=%+v err=%v", observed.measurement, observed.err)
	}
}

func TestTechnicalLiveSidebandPathIsExact(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/backend-api/codex/call_1", nil)
	callID, endpoint, ok := technicalLiveSidebandPath(req)
	if !ok || callID != "call_1" || endpoint != "/backend-api/codex/:call_id" {
		t.Fatalf("unexpected path result call=%q endpoint=%q ok=%v", callID, endpoint, ok)
	}
	for _, path := range []string{"/v1/live/", "/v1/live/a/b", "/backend-api/codex/realtime/calls"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if _, _, ok := technicalLiveSidebandPath(req); ok {
			t.Fatalf("accepted invalid sideband path %q", path)
		}
	}
}
