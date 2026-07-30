package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	coderws "github.com/coder/websocket"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type liveHTTPUpstreamStub struct {
	request *http.Request
	body    []byte
}

type liveAttestationStub struct {
	header string
	err    error
}

type technicalLiveCreateStore struct {
	*liveTestStore
}

func (s *technicalLiveCreateStore) ClaimLiveController(_ context.Context, _ string, controller, _ string) (bool, error) {
	if controller == LiveControllerObserver {
		return false, nil
	}
	return false, nil
}

func (s liveAttestationStub) Check(context.Context) error {
	return s.err
}

func (s liveAttestationStub) Generate(context.Context) (string, error) {
	return s.header, s.err
}

func (s *liveHTTPUpstreamStub) Do(
	request *http.Request,
	_ string,
	_ int64,
	_ int,
) (*http.Response, error) {
	s.request = request
	body, err := io.ReadAll(request.Body)
	if err != nil {
		return nil, err
	}
	s.body = body
	return &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Location": {"/backend-api/codex/call_test"},
		},
		Body: io.NopCloser(strings.NewReader("v=0\r\n")),
	}, nil
}

func (s *liveHTTPUpstreamStub) DoWithTLS(
	request *http.Request,
	proxyURL string,
	accountID int64,
	accountConcurrency int,
	_ *tlsfingerprint.Profile,
) (*http.Response, error) {
	return s.Do(request, proxyURL, accountID, accountConcurrency)
}

func TestLiveCapabilityOnlyAllowsOpenAIOAuth(t *testing.T) {
	require.True(t, (&Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}).SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityLive))
	require.False(t, (&Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}).SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityLive))
	require.False(t, (&Account{Platform: PlatformGrok, Type: AccountTypeOAuth}).SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityLive))
	require.False(t, (&Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			openAIAuthModeCredentialKey: OpenAIAuthModePersonalAccessToken,
		},
	}).SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityLive))
	require.False(t, (&Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			openAIAuthModeCredentialKey: OpenAIAuthModeAgentIdentity,
		},
	}).SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityLive))
}

func TestValidateLiveCallRequestDoesNotRequireDelegation(t *testing.T) {
	request := &LiveCallRequest{
		SDP:     "v=0\r\n",
		Session: json.RawMessage(`{"model":"gpt-live-test","instructions":"hello"}`),
	}
	require.NoError(t, ValidateLiveCallRequest(request))
	require.NotContains(t, string(request.Session), "delegation")
}

func TestCreateUpstreamLiveCallPreservesSession(t *testing.T) {
	upstream := &liveHTTPUpstreamStub{}
	service := &OpenAIGatewayService{
		cfg:          &config.Config{},
		httpUpstream: upstream,
	}
	account := &Account{
		ID:          7,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 2,
		Credentials: map[string]any{
			"access_token":       "test-access-token",
			"chatgpt_account_id": "acct_test",
		},
	}
	session := json.RawMessage(`{
		"model":"gpt-live-test",
		"delegation":{"type":"client"},
		"custom":{"keep":true}
	}`)

	created, err := service.createUpstreamLiveCall(context.Background(), account, &LiveCallRequest{
		SDP:     "v=offer\r\n",
		Session: session,
	}, `{"v":1,"s":0,"t":"v1.test"}`)
	require.NoError(t, err)
	require.Equal(t, "call_test", created.CallID)
	require.Equal(t, []byte("v=0\r\n"), created.SDP)

	var forwarded struct {
		SDP     string          `json:"sdp"`
		Session json.RawMessage `json:"session"`
	}
	require.NoError(t, json.Unmarshal(upstream.body, &forwarded))
	require.Equal(t, "v=offer\r\n", forwarded.SDP)
	require.JSONEq(t, string(session), string(forwarded.Session))
	require.Equal(t, "Bearer test-access-token", upstream.request.Header.Get("Authorization"))
	require.Equal(t, "acct_test", upstream.request.Header.Get("Chatgpt-Account-Id"))
	require.Equal(t, "quicksilver=v2", upstream.request.Header.Get("OpenAI-Alpha"))
	require.Equal(t, `{"v":1,"s":0,"t":"v1.test"}`, upstream.request.Header.Get(liveAttestationHeader))
	require.NotEmpty(t, upstream.request.Header.Get("Session-Id"))
	require.NotEmpty(t, upstream.request.Header.Get("Thread-Id"))
	require.Empty(t, upstream.request.Header.Get("OpenAI-Beta"))
	require.Equal(t, HTTPUpstreamProfileOpenAI, HTTPUpstreamProfileFromContext(upstream.request.Context()))
	require.True(t, HTTPUpstreamRedirectsDisabled(upstream.request.Context()))
}

func TestCreateTechnicalLiveCallPersistsOnlyTechnicalIdentityAndExactMapping(t *testing.T) {
	upstream := &liveHTTPUpstreamStub{}
	store := &technicalLiveCreateStore{liveTestStore: &liveTestStore{}}
	concurrency := &liveTestConcurrencyCache{}
	cfg := &config.Config{
		JWT: config.JWTConfig{Secret: "technical-live-secret"},
		Gateway: config.GatewayConfig{
			Live: config.GatewayLiveConfig{MaxSessionDurationSeconds: 60},
		},
	}
	service := &OpenAIGatewayService{
		cfg:                   cfg,
		httpUpstream:          upstream,
		cache:                 store,
		concurrencyService:    NewConcurrencyService(concurrency),
		liveAttestation:       liveAttestationStub{header: `{"v":1,"s":0,"t":"v1.technical"}`},
		liveAttestationCipher: newLiveAttestationCipher(cfg),
	}
	account := &Account{
		ID:          17,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 2,
		Credentials: map[string]any{
			"access_token":       "technical-access-token",
			"chatgpt_account_id": "technical-account",
			"model_mapping": map[string]any{
				"live-alias": "gpt-live-upstream",
			},
		},
	}
	identity := TechnicalLiveCallIdentity{
		PoolID:          44,
		RequestID:       "request-live-1",
		SessionID:       "session-live-1",
		InboundEndpoint: "/v1/live",
	}
	require.NoError(t, service.AcquireTechnicalLiveLease(context.Background(), account, identity.RequestID))
	created, err := service.CreateTechnicalLiveCall(context.Background(), &LiveCallRequest{
		SDP:     "v=offer\r\n",
		Session: json.RawMessage(`{"model":"live-alias","instructions":"preserve"}`),
	}, identity, account)
	require.NoError(t, err)
	require.Equal(t, "call_test", created.CallID)
	require.Equal(t, account, created.Account)
	require.Equal(t, "gpt-live-upstream", gjson.GetBytes(upstream.body, "session.model").String())
	require.Equal(t, "preserve", gjson.GetBytes(upstream.body, "session.instructions").String())

	record, err := service.GetTechnicalLiveCall(context.Background(), created.CallID, identity.PoolID, identity.SessionID)
	require.NoError(t, err)
	require.True(t, record.Technical)
	require.Equal(t, identity.RequestID, record.TechnicalRequest)
	require.Equal(t, "live-alias", record.Model)
	require.Zero(t, record.UserID)
	require.Zero(t, record.APIKeyID)
	require.Zero(t, record.SubscriptionID)

	_, err = service.GetTechnicalLiveCall(context.Background(), created.CallID, identity.PoolID, "other-session")
	require.ErrorIs(t, err, ErrLiveIdentityMismatch)
	require.NoError(t, service.ReleaseTechnicalLiveLease(context.Background(), account.ID, identity.RequestID))
}

func TestTechnicalLiveRequestRequiresExactModel(t *testing.T) {
	for _, session := range []string{
		`{}`,
		`{"model":null}`,
		`{"model":" live-alias"}`,
	} {
		_, err := technicalLiveRequestModel(&LiveCallRequest{SDP: "v=0", Session: json.RawMessage(session)})
		require.Error(t, err)
	}
}

func TestParseTechnicalLiveCallRequestIsStrictForJSONAndMultipart(t *testing.T) {
	jsonBody := []byte(`{"sdp":"v=offer","session":{"model":"live-alias"},"user_id":1}`)
	jsonReq := httptest.NewRequest(http.MethodPost, "/v1/live", bytes.NewReader(jsonBody))
	jsonReq.Header.Set("Content-Type", "application/json")
	_, err := ParseTechnicalLiveCallRequest(jsonReq, jsonBody)
	require.ErrorContains(t, err, "unsupported fields")

	var multipartBody bytes.Buffer
	writer := multipart.NewWriter(&multipartBody)
	require.NoError(t, writer.WriteField("sdp", "v=offer\r\n"))
	require.NoError(t, writer.WriteField("session", `{"model":"live-alias","voice":"alloy"}`))
	require.NoError(t, writer.Close())
	multipartReq := httptest.NewRequest(http.MethodPost, "/backend-api/codex/realtime/calls", bytes.NewReader(multipartBody.Bytes()))
	multipartReq.Header.Set("Content-Type", writer.FormDataContentType())
	parsed, err := ParseTechnicalLiveCallRequest(multipartReq, multipartBody.Bytes())
	require.NoError(t, err)
	require.Equal(t, "v=offer\r\n", parsed.SDP)
	require.Equal(t, "live-alias", gjson.GetBytes(parsed.Session, "model").String())
	require.Equal(t, "alloy", gjson.GetBytes(parsed.Session, "voice").String())
}

func TestLiveAttestationCipherRoundTripAndRejectsOtherInstanceKey(t *testing.T) {
	first := newLiveAttestationCipher(&config.Config{
		JWT: config.JWTConfig{Secret: "first-live-secret"},
	})
	second := newLiveAttestationCipher(&config.Config{
		JWT: config.JWTConfig{Secret: "second-live-secret"},
	})
	require.NotNil(t, first)
	require.NotNil(t, second)

	ciphertext, err := first.Encrypt(`{"v":1,"s":0,"t":"v1.opaque"}`)
	require.NoError(t, err)
	require.NotContains(t, ciphertext, "opaque")

	plaintext, err := first.Decrypt(ciphertext)
	require.NoError(t, err)
	require.Equal(t, `{"v":1,"s":0,"t":"v1.opaque"}`, plaintext)

	_, err = second.Decrypt(ciphertext)
	require.Error(t, err)
}

func TestPrepareLiveAttestationEncryptsHeaderAndReturnsExplicitProviderError(t *testing.T) {
	cipher := newLiveAttestationCipher(&config.Config{
		JWT: config.JWTConfig{Secret: "live-attestation-test-secret"},
	})
	service := &OpenAIGatewayService{
		liveAttestation:       liveAttestationStub{header: `{"v":1,"s":0,"t":"v1.test"}`},
		liveAttestationCipher: cipher,
	}
	header, ciphertext, err := service.prepareLiveAttestation(context.Background())
	require.NoError(t, err)
	require.Equal(t, `{"v":1,"s":0,"t":"v1.test"}`, header)
	require.NotContains(t, ciphertext, "v1.test")
	decrypted, err := cipher.Decrypt(ciphertext)
	require.NoError(t, err)
	require.Equal(t, header, decrypted)

	service.liveAttestation = liveAttestationStub{err: errors.New("macOS app missing")}
	_, _, err = service.prepareLiveAttestation(context.Background())
	var unavailable *LiveAttestationUnavailableError
	require.ErrorAs(t, err, &unavailable)
	require.Contains(t, unavailable.Error(), "macOS app missing")
}

func TestLiveMaxSessionDurationDefaultsAndOverrides(t *testing.T) {
	require.Equal(t, defaultLiveMaxSessionDuration, (&OpenAIGatewayService{}).liveMaxSessionDuration())
	require.Equal(
		t,
		90*time.Second,
		(&OpenAIGatewayService{cfg: &config.Config{
			Gateway: config.GatewayConfig{
				Live: config.GatewayLiveConfig{MaxSessionDurationSeconds: 90},
			},
		}}).liveMaxSessionDuration(),
	)
}

func TestLiveSidebandNormalCloseEndsCall(t *testing.T) {
	normalClose := coderws.CloseError{Code: coderws.StatusNormalClosure}
	require.ErrorIs(t, liveSidebandReadError(normalClose), ErrLiveCallNotFound)

	abnormalClose := coderws.CloseError{Code: coderws.StatusInternalError}
	require.Equal(t, abnormalClose, liveSidebandReadError(abnormalClose))
}

func TestLiveCreateFailoverUsesExistingOpenAIPolicy(t *testing.T) {
	service := &OpenAIGatewayService{}
	require.False(t, service.shouldFailoverLiveCreateError(&UpstreamFailoverError{
		StatusCode:   http.StatusBadRequest,
		ResponseBody: []byte(`{"error":{"message":"invalid session"}}`),
	}))
	require.True(t, service.shouldFailoverLiveCreateError(&UpstreamFailoverError{
		StatusCode: http.StatusForbidden,
	}))
	require.True(t, service.shouldFailoverLiveCreateError(&UpstreamFailoverError{
		StatusCode: http.StatusBadGateway,
	}))
	require.True(t, service.shouldFailoverLiveCreateError(errors.New("transport failed")))
}

func TestLiveCallIDFromLocation(t *testing.T) {
	callID, err := liveCallIDFromLocation("https://chatgpt.com/backend-api/codex/call_123?intent=quicksilver")
	require.NoError(t, err)
	require.Equal(t, "call_123", callID)

	callID, err = liveCallIDFromLocation("/backend-api/codex/call_456")
	require.NoError(t, err)
	require.Equal(t, "call_456", callID)
}

func TestRequestTypeLive(t *testing.T) {
	require.True(t, RequestTypeLive.IsValid())
	require.Equal(t, "live", RequestTypeLive.String())
	parsed, err := ParseUsageRequestType("live")
	require.NoError(t, err)
	require.Equal(t, RequestTypeLive, parsed)
}
