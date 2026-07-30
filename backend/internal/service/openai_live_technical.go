package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/tidwall/sjson"
)

const openAITechnicalLiveCreateUnavailableReason GatewayFailureReason = "openai_live_create_unavailable"

func ParseTechnicalLiveCallRequest(req *http.Request, body []byte) (*LiveCallRequest, error) {
	if req == nil || req.URL == nil || req.Method != http.MethodPost ||
		(req.URL.Path != "/v1/live" && req.URL.Path != "/backend-api/codex/realtime/calls") {
		return nil, errors.New("technical Live create endpoint mismatch")
	}
	mediaType, params, err := mime.ParseMediaType(req.Header.Get("Content-Type"))
	if err != nil {
		return nil, fmt.Errorf("parse technical Live Content-Type: %w", err)
	}
	var result LiveCallRequest
	switch strings.ToLower(mediaType) {
	case "application/json":
		var object map[string]json.RawMessage
		if err := json.Unmarshal(body, &object); err != nil || object == nil {
			return nil, errors.New("technical Live request must be a JSON object")
		}
		if err := rejectUnknownJSONObjectFields(object, map[string]struct{}{"sdp": {}, "session": {}}, "technical Live request"); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(object["sdp"], &result.SDP); err != nil {
			return nil, errors.New("technical Live sdp must be a string")
		}
		result.Session = append(json.RawMessage(nil), object["session"]...)
	case "multipart/form-data":
		boundary := params["boundary"]
		if boundary == "" || boundary != strings.TrimSpace(boundary) {
			return nil, errors.New("technical Live multipart boundary must be exact and non-empty")
		}
		reader := multipart.NewReader(bytes.NewReader(body), boundary)
		fields := make(map[string][]byte)
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("read technical Live multipart body: %w", err)
			}
			name := part.FormName()
			if part.FileName() != "" || (name != "sdp" && name != "session") {
				_ = part.Close()
				return nil, fmt.Errorf("technical Live multipart contains unsupported field %q", name)
			}
			if _, duplicate := fields[name]; duplicate {
				_ = part.Close()
				return nil, fmt.Errorf("technical Live multipart field %q must not be duplicated", name)
			}
			value, readErr := io.ReadAll(part)
			_ = part.Close()
			if readErr != nil {
				return nil, fmt.Errorf("read technical Live multipart field %q: %w", name, readErr)
			}
			fields[name] = value
		}
		result.SDP = string(fields["sdp"])
		result.Session = append(json.RawMessage(nil), fields["session"]...)
	default:
		return nil, fmt.Errorf("technical Live request has unsupported Content-Type %q", req.Header.Get("Content-Type"))
	}
	if err := ValidateLiveCallRequest(&result); err != nil {
		return nil, err
	}
	if _, err := technicalLiveRequestModel(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ValidateTechnicalLiveRuntime verifies only provider-facing Live
// dependencies. Customer authentication, billing, and concurrency services
// are intentionally not part of this graph.
func (s *OpenAIGatewayService) ValidateTechnicalLiveRuntime() error {
	if err := s.ValidateTechnicalRuntime(); err != nil {
		return err
	}
	missing := make([]string, 0, 4)
	if _, err := s.liveStore(); err != nil {
		missing = append(missing, "Live call store")
	}
	if _, err := s.technicalLiveConcurrencyCache(); err != nil {
		missing = append(missing, "technical Live concurrency cache")
	}
	if s.liveAttestation == nil {
		missing = append(missing, "Live attestation provider")
	}
	if s.liveAttestationCipher == nil {
		missing = append(missing, "Live attestation cipher")
	}
	if len(missing) > 0 {
		return fmt.Errorf("technical OpenAI Live dependencies are incomplete: %s", strings.Join(missing, ", "))
	}
	return nil
}

func validateTechnicalLiveIdentity(identity TechnicalLiveCallIdentity) error {
	if identity.PoolID <= 0 {
		return errors.New("technical Live pool ID must be positive")
	}
	if identity.RequestID == "" || identity.RequestID != strings.TrimSpace(identity.RequestID) {
		return errors.New("technical Live request ID must be exact and non-empty")
	}
	if identity.SessionID == "" || identity.SessionID != strings.TrimSpace(identity.SessionID) {
		return errors.New("technical Live session ID must be exact and non-empty")
	}
	if identity.InboundEndpoint == "" || identity.InboundEndpoint != strings.TrimSpace(identity.InboundEndpoint) {
		return errors.New("technical Live inbound endpoint must be exact and non-empty")
	}
	return nil
}

func technicalLiveRequestModel(request *LiveCallRequest) (string, error) {
	if err := ValidateLiveCallRequest(request); err != nil {
		return "", err
	}
	var session map[string]json.RawMessage
	if err := json.Unmarshal(request.Session, &session); err != nil || session == nil {
		return "", errors.New("technical Live session must be a JSON object")
	}
	modelRaw, ok := session["model"]
	if !ok {
		return "", errors.New("technical Live session model is required")
	}
	var model string
	if err := json.Unmarshal(modelRaw, &model); err != nil || model == "" || model != strings.TrimSpace(model) {
		return "", errors.New("technical Live session model must be an exact non-empty string")
	}
	return model, nil
}

// CreateTechnicalLiveCall creates a provider Live session for one account
// already selected from the exact technical pool. The caller owns the
// account-only Live lease until this method successfully persists the record.
func (s *OpenAIGatewayService) CreateTechnicalLiveCall(
	ctx context.Context,
	request *LiveCallRequest,
	identity TechnicalLiveCallIdentity,
	account *Account,
) (*LiveCallCreated, error) {
	if err := validateTechnicalLiveIdentity(identity); err != nil {
		return nil, err
	}
	if account == nil || account.ID <= 0 || !account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityLive) {
		return nil, errors.New("technical Live requires an eligible OpenAI subscription account")
	}
	originalModel, err := technicalLiveRequestModel(request)
	if err != nil {
		return nil, err
	}
	upstreamModel, err := resolveExactOpenAIAccountModel(account, originalModel)
	if err != nil {
		return nil, err
	}
	upstreamSession := append(json.RawMessage(nil), request.Session...)
	if upstreamModel != originalModel {
		upstreamSession, err = sjson.SetBytes(upstreamSession, "model", upstreamModel)
		if err != nil {
			return nil, fmt.Errorf("map technical Live model: %w", err)
		}
	}
	attestation, attestationCiphertext, err := s.prepareLiveAttestation(ctx)
	if err != nil {
		return nil, err
	}
	created, err := s.createUpstreamLiveCall(ctx, account, &LiveCallRequest{
		SDP:     request.SDP,
		Session: upstreamSession,
	}, attestation)
	if err != nil {
		return nil, technicalLiveCreateError(account, err)
	}
	now := time.Now()
	record := &LiveCallRecord{
		CallID:                created.CallID,
		CallHash:              hashLiveCallID(created.CallID),
		AccountID:             account.ID,
		LeaseID:               identity.RequestID,
		Model:                 originalModel,
		UpstreamModel:         upstreamModel,
		CreatedAt:             now,
		ExpiresAt:             now.Add(s.liveMaxSessionDuration()),
		Controller:            LiveControllerPending,
		InboundEndpoint:       identity.InboundEndpoint,
		Technical:             true,
		TechnicalPoolID:       identity.PoolID,
		TechnicalRequest:      identity.RequestID,
		TechnicalSession:      identity.SessionID,
		AttestationCiphertext: attestationCiphertext,
	}
	store, err := s.liveStore()
	if err != nil {
		return nil, err
	}
	if err := store.SaveLiveCall(ctx, record, s.liveMaxSessionDuration()+5*time.Minute); err != nil {
		return nil, fmt.Errorf("save technical Live call mapping: %w", err)
	}
	created.Account = account
	created.UpstreamModel = upstreamModel
	go s.observeLiveCall(record)
	return created, nil
}

func technicalLiveCreateError(account *Account, err error) error {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	var upstream *UpstreamFailoverError
	if errors.As(err, &upstream) {
		if !nativeOpenAIResponsesFailoverStatus(upstream.StatusCode) {
			return err
		}
		copy := *upstream
		copy.Stage = GatewayFailureStageInference
		copy.Scope = GatewayFailureScopeAccount
		copy.Reason = openAITechnicalLiveCreateUnavailableReason
		copy.NextAccountAction = NextAccountRetry
		copy.RetryableOnSameAccount = account != nil && account.IsPoolMode() && account.IsPoolModeRetryableStatus(copy.StatusCode)
		return &copy
	}
	return nativeOpenAIAccountFailoverError(
		http.StatusBadGateway,
		GatewayFailureStageInference,
		openAITechnicalLiveCreateUnavailableReason,
		err,
	)
}

func (s *OpenAIGatewayService) GetTechnicalLiveCall(
	ctx context.Context,
	callID string,
	poolID int64,
	sessionID string,
) (*LiveCallRecord, error) {
	if callID == "" || callID != strings.TrimSpace(callID) || poolID <= 0 ||
		sessionID == "" || sessionID != strings.TrimSpace(sessionID) {
		return nil, ErrLiveIdentityMismatch
	}
	store, err := s.liveStore()
	if err != nil {
		return nil, err
	}
	record, err := store.GetLiveCall(ctx, hashLiveCallID(callID))
	if err != nil {
		return nil, err
	}
	if !record.Technical || record.CallID != callID || record.TechnicalPoolID != poolID || record.TechnicalSession != sessionID {
		return nil, ErrLiveIdentityMismatch
	}
	if record.Controller == LiveControllerClosed {
		return nil, ErrLiveCallNotFound
	}
	return record, nil
}
