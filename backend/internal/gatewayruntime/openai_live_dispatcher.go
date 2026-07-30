package gatewayruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/gatewaycore"
	"github.com/Wei-Shaw/sub2api/internal/config"
	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/service"
	coderws "github.com/coder/websocket"
)

type OpenAILiveDispatcherConfig struct {
	MaxAccountSwitches int
	MaxBodyBytes       int64
}

func OpenAILiveDispatcherConfigFromApplication(cfg *config.Config) (OpenAILiveDispatcherConfig, error) {
	if cfg == nil {
		return OpenAILiveDispatcherConfig{}, errors.New("gateway application config is required")
	}
	return OpenAILiveDispatcherConfig{
		MaxAccountSwitches: cfg.Gateway.MaxAccountSwitches,
		MaxBodyBytes:       cfg.Gateway.MaxBodySize,
	}, nil
}

type openAILiveGateway interface {
	ValidateTechnicalLiveRuntime() error
	SelectTechnicalLiveAccountWithLoadAwareness(context.Context, *int64, string, map[int64]struct{}) (*service.AccountSelectionResult, error)
	AcquireSelection(context.Context, *service.AccountSelectionResult) (func(), error)
	AcquireTechnicalLiveLease(context.Context, *service.Account, string) error
	ReleaseTechnicalLiveLease(context.Context, int64, string) error
	CreateTechnicalLiveCall(context.Context, *service.LiveCallRequest, service.TechnicalLiveCallIdentity, *service.Account) (*service.LiveCallCreated, error)
	GetTechnicalLiveCall(context.Context, string, int64, string) (*service.LiveCallRecord, error)
	ProxyLiveSideband(context.Context, *service.LiveCallRecord, *coderws.Conn) error
}

type OpenAILiveDispatcher struct {
	gateway            openAILiveGateway
	maxAccountSwitches int
	maxBodyBytes       int64
}

func NewOpenAILiveDispatcher(gateway openAILiveGateway, cfg OpenAILiveDispatcherConfig) (*OpenAILiveDispatcher, error) {
	if gateway == nil {
		return nil, errors.New("OpenAI Live gateway is required")
	}
	if err := gateway.ValidateTechnicalLiveRuntime(); err != nil {
		return nil, fmt.Errorf("validate OpenAI Live gateway: %w", err)
	}
	if cfg.MaxAccountSwitches <= 0 {
		return nil, errors.New("OpenAI max account switches must be positive")
	}
	if cfg.MaxBodyBytes <= 0 {
		return nil, errors.New("OpenAI max body bytes must be positive")
	}
	return &OpenAILiveDispatcher{
		gateway:            gateway,
		maxAccountSwitches: cfg.MaxAccountSwitches,
		maxBodyBytes:       cfg.MaxBodyBytes,
	}, nil
}

func (d *OpenAILiveDispatcher) Forward(
	ctx context.Context,
	w http.ResponseWriter,
	req *http.Request,
	dispatch gatewaycore.DispatchRequest,
) (*gatewaycore.Measurement, error) {
	if d == nil || d.gateway == nil {
		return nil, errors.New("OpenAI Live dispatcher is not initialized")
	}
	if dispatch.Pool.Platform != service.PlatformOpenAI {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedPoolPlatform, dispatch.Pool.Platform)
	}
	if !dispatch.Pool.AllowLive {
		return nil, errors.New("OpenAI Live is disabled for the technical pool")
	}
	if req == nil || req.URL == nil {
		return nil, errors.New("OpenAI Live request is incomplete")
	}
	if req.Method == http.MethodPost && (req.URL.Path == "/v1/live" || req.URL.Path == "/backend-api/codex/realtime/calls") {
		return d.forwardCreate(ctx, w, req, dispatch)
	}
	callID, endpoint, ok := technicalLiveSidebandPath(req)
	if !ok {
		return nil, fmt.Errorf("invalid OpenAI Live endpoint: %s %s", req.Method, req.URL.Path)
	}
	return d.forwardSideband(ctx, w, req, dispatch, callID, endpoint)
}

func (d *OpenAILiveDispatcher) forwardCreate(
	ctx context.Context,
	w http.ResponseWriter,
	req *http.Request,
	dispatch gatewaycore.DispatchRequest,
) (*gatewaycore.Measurement, error) {
	body, err := pkghttputil.ReadRequestBodyWithPreallocLimit(req, d.maxBodyBytes)
	if err != nil {
		return nil, fmt.Errorf("read OpenAI Live request body: %w", err)
	}
	parsed, err := service.ParseTechnicalLiveCallRequest(req, body)
	if err != nil {
		return nil, err
	}
	model := strings.TrimSpace(dispatch.Invocation.Model)
	if model == "" {
		return nil, errors.New("OpenAI Live invocation model is required")
	}
	var session struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(parsed.Session, &session); err != nil || session.Model != model {
		return nil, fmt.Errorf("%w: invocation=%q session=%q", ErrInvocationModelMismatch, model, session.Model)
	}

	poolID := dispatch.Pool.ID
	excluded := make(map[int64]struct{})
	sameAccountRetries := make(map[int64]int)
	switchCount := 0
	var lastFailover *service.UpstreamFailoverError
	for {
		selection, selectErr := d.gateway.SelectTechnicalLiveAccountWithLoadAwareness(ctx, &poolID, model, excluded)
		if selectErr != nil {
			if lastFailover != nil {
				return nil, errors.Join(lastFailover, selectErr)
			}
			return nil, selectErr
		}
		if selection == nil || selection.Account == nil {
			return nil, service.ErrInvalidAccountSelection
		}
		account := selection.Account
		release, err := d.gateway.AcquireSelection(ctx, selection)
		if err != nil {
			return nil, err
		}
		release = releaseOnContextDone(ctx, release)
		if account.Platform != service.PlatformOpenAI || account.Type != service.AccountTypeOAuth ||
			!account.SupportsOpenAIEndpointCapability(service.OpenAIEndpointCapabilityLive) {
			release()
			return nil, fmt.Errorf("scheduler selected unsupported account %d for OpenAI Live", account.ID)
		}
		if err := d.gateway.AcquireTechnicalLiveLease(ctx, account, dispatch.Invocation.RequestID); err != nil {
			release()
			if !errors.Is(err, service.ErrLiveConcurrencyFull) {
				return nil, err
			}
			excluded[account.ID] = struct{}{}
			if switchCount >= d.maxAccountSwitches {
				return nil, err
			}
			switchCount++
			continue
		}
		startedAt := time.Now()
		created, createErr := d.gateway.CreateTechnicalLiveCall(ctx, parsed, service.TechnicalLiveCallIdentity{
			PoolID:          poolID,
			RequestID:       dispatch.Invocation.RequestID,
			SessionID:       dispatch.Invocation.SessionID,
			InboundEndpoint: req.URL.Path,
		}, account)
		release()
		if createErr == nil && created != nil {
			location := technicalLiveDownstreamLocation(req.URL.Path, created.CallID)
			w.Header().Set("Location", location)
			w.Header().Set("Content-Type", "application/sdp")
			w.WriteHeader(http.StatusOK)
			_, writeErr := w.Write(created.SDP)
			return &gatewaycore.Measurement{
				AccountID:          account.ID,
				Endpoint:           req.URL.Path,
				UpstreamModel:      created.UpstreamModel,
				UpstreamStatusCode: http.StatusOK,
				StartedAt:          startedAt,
				Duration:           time.Since(startedAt),
			}, writeErr
		}
		d.releaseTechnicalLease(account.ID, dispatch.Invocation.RequestID)
		if createErr == nil {
			return nil, errors.New("OpenAI Live create returned neither result nor error")
		}
		var failover *service.UpstreamFailoverError
		if !errors.As(createErr, &failover) || !failover.ShouldRetryNextAccount() {
			return nil, createErr
		}
		lastFailover = failover
		retryLimit := account.GetPoolModeRetryCount()
		if failover.RetryableOnSameAccount && sameAccountRetries[account.ID] < retryLimit {
			sameAccountRetries[account.ID]++
			if err := waitForContext(ctx, sameAccountRetryDelay); err != nil {
				return nil, err
			}
			continue
		}
		excluded[account.ID] = struct{}{}
		if switchCount >= d.maxAccountSwitches {
			return nil, lastFailover
		}
		switchCount++
	}
}

func (d *OpenAILiveDispatcher) forwardSideband(
	ctx context.Context,
	w http.ResponseWriter,
	req *http.Request,
	dispatch gatewaycore.DispatchRequest,
	callID string,
	endpoint string,
) (*gatewaycore.Measurement, error) {
	if !isTechnicalLiveWebSocketUpgrade(req) {
		return nil, errors.New("OpenAI Live sideband requires a WebSocket upgrade")
	}
	record, err := d.gateway.GetTechnicalLiveCall(ctx, callID, dispatch.Pool.ID, dispatch.Invocation.SessionID)
	if err != nil {
		return nil, err
	}
	if record.Model != dispatch.Invocation.Model {
		return nil, fmt.Errorf("%w: invocation=%q call=%q", ErrInvocationModelMismatch, dispatch.Invocation.Model, record.Model)
	}
	startedAt := time.Now()
	downstream, err := coderws.Accept(w, req, &coderws.AcceptOptions{CompressionMode: coderws.CompressionContextTakeover})
	if err != nil {
		return nil, fmt.Errorf("accept OpenAI Live sideband: %w", err)
	}
	defer func() { _ = downstream.CloseNow() }()
	proxyErr := d.gateway.ProxyLiveSideband(ctx, record, downstream)
	if errors.Is(proxyErr, service.ErrLiveCallNotFound) || technicalLiveGracefulClose(proxyErr) {
		proxyErr = nil
		_ = downstream.Close(coderws.StatusNormalClosure, "")
	} else if proxyErr != nil {
		_ = downstream.Close(coderws.StatusInternalError, "live sideband closed")
	}
	return &gatewaycore.Measurement{
		AccountID:          record.AccountID,
		Endpoint:           endpoint,
		UpstreamModel:      record.UpstreamModel,
		UpstreamStatusCode: http.StatusSwitchingProtocols,
		StartedAt:          startedAt,
		Duration:           time.Since(startedAt),
	}, proxyErr
}

func (d *OpenAILiveDispatcher) releaseTechnicalLease(accountID int64, leaseID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = d.gateway.ReleaseTechnicalLiveLease(ctx, accountID, leaseID)
}

func technicalLiveSidebandPath(req *http.Request) (callID string, endpoint string, ok bool) {
	if req == nil || req.URL == nil || req.Method != http.MethodGet {
		return "", "", false
	}
	for _, candidate := range []struct {
		prefix   string
		endpoint string
	}{
		{prefix: "/v1/live/", endpoint: "/v1/live/:call_id"},
		{prefix: "/backend-api/codex/", endpoint: "/backend-api/codex/:call_id"},
	} {
		if !strings.HasPrefix(req.URL.Path, candidate.prefix) {
			continue
		}
		raw := strings.TrimPrefix(req.URL.Path, candidate.prefix)
		if raw == "" || strings.Contains(raw, "/") {
			return "", "", false
		}
		decoded, err := url.PathUnescape(raw)
		if err != nil || decoded == "" || decoded != strings.TrimSpace(decoded) || strings.Contains(decoded, "/") {
			return "", "", false
		}
		return decoded, candidate.endpoint, true
	}
	return "", "", false
}

func technicalLiveDownstreamLocation(createPath, callID string) string {
	if createPath == "/backend-api/codex/realtime/calls" {
		return "/backend-api/codex/" + url.PathEscape(callID)
	}
	return "/v1/live/" + url.PathEscape(callID)
}

func isTechnicalLiveWebSocketUpgrade(req *http.Request) bool {
	if req == nil {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(req.Header.Get("Upgrade")), "websocket") {
		return false
	}
	for _, token := range strings.Split(req.Header.Get("Connection"), ",") {
		if strings.EqualFold(strings.TrimSpace(token), "upgrade") {
			return true
		}
	}
	return false
}

func technicalLiveGracefulClose(err error) bool {
	status := coderws.CloseStatus(err)
	return status == coderws.StatusNormalClosure || status == coderws.StatusGoingAway
}

func (d *OpenAILiveDispatcher) Close(context.Context) error { return nil }

var _ gatewaycore.Dispatcher = (*OpenAILiveDispatcher)(nil)
