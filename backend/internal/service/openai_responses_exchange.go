package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/gatewaytransport"
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	openAIResponsesCredentialUnavailableReason GatewayFailureReason = "openai_responses_credential_unavailable"
	openAIResponsesTransportUnavailableReason  GatewayFailureReason = "openai_responses_transport_unavailable"
	nativeOpenAIResponsesEndpoint                                   = "/v1/responses"
	nativeOpenAIResponsesCompactEndpoint                            = "/v1/responses/compact"
)

// ValidateTechnicalRuntime verifies the dependencies used by the native
// OpenAI Responses transport. Customer billing dependencies are intentionally
// absent from this graph.
func (s *OpenAIGatewayService) ValidateTechnicalRuntime() error {
	if s == nil {
		return errors.New("openai gateway service is nil")
	}
	missing := make([]string, 0, 8)
	if s.accountRepo == nil {
		missing = append(missing, "account repository")
	}
	if s.cache == nil {
		missing = append(missing, "gateway cache")
	}
	if s.cfg == nil {
		missing = append(missing, "gateway config")
	}
	if s.schedulerSnapshot == nil {
		missing = append(missing, "scheduler snapshot")
	}
	if s.concurrencyService == nil {
		missing = append(missing, "account concurrency service")
	}
	if s.httpUpstream == nil {
		missing = append(missing, "upstream HTTP transport")
	}
	if s.openAITokenProvider == nil {
		missing = append(missing, "OpenAI token provider")
	}
	if len(missing) > 0 {
		return fmt.Errorf("technical OpenAI dependencies are incomplete: %s", strings.Join(missing, ", "))
	}
	return nil
}

// ParseOpenAIResponsesRequest validates the protocol-level fields shared by
// the dispatcher and provider forwarder.
func ParseOpenAIResponsesRequest(body []byte) (model string, stream bool, err error) {
	if len(body) == 0 || !json.Valid(body) {
		return "", false, errors.New("OpenAI Responses request must be valid JSON")
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(body, &object); err != nil || object == nil {
		return "", false, errors.New("OpenAI Responses request must be a JSON object")
	}
	modelRaw, ok := object["model"]
	if !ok {
		return "", false, errors.New("OpenAI Responses model is required")
	}
	if err := json.Unmarshal(modelRaw, &model); err != nil || strings.TrimSpace(model) == "" {
		return "", false, errors.New("OpenAI Responses model must be a non-empty string")
	}
	if model != strings.TrimSpace(model) {
		return "", false, errors.New("OpenAI Responses model must not contain surrounding whitespace")
	}
	if streamRaw, exists := object["stream"]; exists {
		var streamValue any
		if err := json.Unmarshal(streamRaw, &streamValue); err != nil {
			return "", false, errors.New("OpenAI Responses stream must be a boolean")
		}
		var ok bool
		stream, ok = streamValue.(bool)
		if !ok {
			return "", false, errors.New("OpenAI Responses stream must be a boolean")
		}
	}
	return model, stream, nil
}

func openAIAccountHasExactModelMapping(account *Account, requestedModel string) bool {
	_, err := resolveExactOpenAIAccountModel(account, requestedModel)
	return err == nil
}

func resolveExactOpenAIAccountModel(account *Account, requestedModel string) (string, error) {
	if account == nil {
		return "", errors.New("OpenAI account is required")
	}
	if requestedModel == "" || requestedModel != strings.TrimSpace(requestedModel) {
		return "", errors.New("OpenAI requested model must be an exact non-empty identifier")
	}
	if account.Credentials == nil {
		return requestedModel, nil
	}
	raw, configured := account.Credentials["model_mapping"]
	if !configured || raw == nil {
		return requestedModel, nil
	}
	mapping, ok := raw.(map[string]any)
	if !ok {
		return "", fmt.Errorf("OpenAI account %d model_mapping must be an object", account.ID)
	}
	if len(mapping) == 0 {
		return requestedModel, nil
	}
	rawMapped, exists := mapping[requestedModel]
	if !exists {
		return "", fmt.Errorf("OpenAI account %d has no exact mapping for model %q", account.ID, requestedModel)
	}
	mappedModel, ok := rawMapped.(string)
	if !ok || mappedModel == "" || mappedModel != strings.TrimSpace(mappedModel) {
		return "", fmt.Errorf("OpenAI account %d has an invalid exact mapping for model %q", account.ID, requestedModel)
	}
	return mappedModel, nil
}

func resolveExactOpenAICompactAccountModel(account *Account, requestedModel string) (string, error) {
	regularModel, err := resolveExactOpenAIAccountModel(account, requestedModel)
	if err != nil {
		return "", err
	}
	if account.Credentials == nil {
		return regularModel, nil
	}
	raw, configured := account.Credentials["compact_model_mapping"]
	if !configured || raw == nil {
		return regularModel, nil
	}
	mapping, ok := raw.(map[string]any)
	if !ok {
		return "", fmt.Errorf("OpenAI account %d compact_model_mapping must be an object", account.ID)
	}
	if len(mapping) == 0 {
		return regularModel, nil
	}
	rawMapped, exists := mapping[regularModel]
	if !exists {
		return regularModel, nil
	}
	mappedModel, ok := rawMapped.(string)
	if !ok || mappedModel == "" || mappedModel != strings.TrimSpace(mappedModel) {
		return "", fmt.Errorf("OpenAI account %d has an invalid exact compact mapping for model %q", account.ID, regularModel)
	}
	return mappedModel, nil
}

type nativeOpenAIResponsesUsage struct {
	InputTokens  *int `json:"input_tokens"`
	OutputTokens *int `json:"output_tokens"`
	InputDetails *struct {
		CachedTokens *int `json:"cached_tokens"`
		ImageTokens  *int `json:"image_tokens"`
	} `json:"input_tokens_details"`
	OutputDetails *struct {
		ImageTokens *int `json:"image_tokens"`
	} `json:"output_tokens_details"`
}

type nativeOpenAIResponsesUsageEnvelope struct {
	Usage    *nativeOpenAIResponsesUsage `json:"usage"`
	Response *struct {
		Usage *nativeOpenAIResponsesUsage `json:"usage"`
	} `json:"response"`
}

func extractNativeOpenAIResponsesUsage(body []byte) (OpenAIUsage, bool, error) {
	var envelope nativeOpenAIResponsesUsageEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return OpenAIUsage{}, false, fmt.Errorf("parse OpenAI Responses usage envelope: %w", err)
	}
	if envelope.Usage != nil && envelope.Response != nil && envelope.Response.Usage != nil {
		return OpenAIUsage{}, false, errors.New("OpenAI Responses payload contains ambiguous root and response usage")
	}
	usage := envelope.Usage
	if usage == nil && envelope.Response != nil {
		usage = envelope.Response.Usage
	}
	if usage == nil {
		return OpenAIUsage{}, false, nil
	}
	if usage.InputTokens == nil || usage.OutputTokens == nil {
		return OpenAIUsage{}, false, errors.New("OpenAI Responses usage omitted input_tokens or output_tokens")
	}
	values := []*int{usage.InputTokens, usage.OutputTokens}
	if usage.InputDetails != nil {
		values = append(values, usage.InputDetails.CachedTokens, usage.InputDetails.ImageTokens)
	}
	if usage.OutputDetails != nil {
		values = append(values, usage.OutputDetails.ImageTokens)
	}
	for _, value := range values {
		if value != nil && *value < 0 {
			return OpenAIUsage{}, false, errors.New("OpenAI Responses usage contains a negative token count")
		}
	}
	result := OpenAIUsage{
		InputTokens:  *usage.InputTokens,
		OutputTokens: *usage.OutputTokens,
	}
	if usage.InputDetails != nil {
		if usage.InputDetails.CachedTokens != nil {
			result.CacheReadInputTokens = *usage.InputDetails.CachedTokens
		}
		if usage.InputDetails.ImageTokens != nil {
			result.ImageInputTokens = *usage.InputDetails.ImageTokens
		}
	}
	if usage.OutputDetails != nil && usage.OutputDetails.ImageTokens != nil {
		result.ImageOutputTokens = *usage.OutputDetails.ImageTokens
	}
	return result, true, nil
}

func nativeOpenAIResponsesOutputTelemetry(body []byte) (imageCount int, imageSizes []string, webSearchCalls int) {
	output := gjson.GetBytes(body, "output")
	if !output.IsArray() {
		output = gjson.GetBytes(body, "response.output")
	}
	if !output.IsArray() {
		return 0, nil, 0
	}
	output.ForEach(func(_, item gjson.Result) bool {
		if !item.IsObject() {
			return true
		}
		switch item.Get("type").String() {
		case "image_generation_call":
			result := item.Get("result")
			if result.Type != gjson.String || result.String() == "" {
				return true
			}
			imageCount++
			size := item.Get("size")
			if size.Type == gjson.String && size.String() != "" {
				imageSizes = append(imageSizes, size.String())
			}
		case "web_search_call":
			webSearchCalls++
		}
		return true
	})
	return imageCount, imageSizes, webSearchCalls
}

// ForwardResponsesExchange forwards exactly one /v1/responses request. It
// performs model routing and transport adaptation but does not inject prompts,
// remove request fields, recover invalid continuation state, or fabricate usage.
func (s *OpenAIGatewayService) ForwardResponsesExchange(
	ctx context.Context,
	exchange gatewaytransport.Exchange,
	account *Account,
	body []byte,
) (*OpenAIForwardResult, error) {
	return s.forwardNativeOpenAIResponsesEndpoint(ctx, exchange, account, body, false)
}

// ForwardResponsesCompactExchange forwards exactly one unary
// /v1/responses/compact request without dropping or rewriting request fields.
func (s *OpenAIGatewayService) ForwardResponsesCompactExchange(
	ctx context.Context,
	exchange gatewaytransport.Exchange,
	account *Account,
	body []byte,
) (*OpenAIForwardResult, error) {
	return s.forwardNativeOpenAIResponsesEndpoint(ctx, exchange, account, body, true)
}

func (s *OpenAIGatewayService) forwardNativeOpenAIResponsesEndpoint(
	ctx context.Context,
	exchange gatewaytransport.Exchange,
	account *Account,
	body []byte,
	compact bool,
) (*OpenAIForwardResult, error) {
	if exchange == nil || exchange.Request() == nil || exchange.Response() == nil {
		return nil, errors.New("openai exchange is required")
	}
	expectedEndpoint := nativeOpenAIResponsesEndpoint
	if compact {
		expectedEndpoint = nativeOpenAIResponsesCompactEndpoint
	}
	if exchange.Request().URL == nil || exchange.Request().URL.Path != expectedEndpoint {
		return nil, fmt.Errorf("native OpenAI Responses endpoint mismatch: expected %s", expectedEndpoint)
	}
	if account == nil {
		return nil, errors.New("openai account is required")
	}
	if account.Platform != PlatformOpenAI {
		return nil, fmt.Errorf("native OpenAI Responses cannot use platform %q", account.Platform)
	}
	if account.Type != AccountTypeOAuth && account.Type != AccountTypeAPIKey {
		return nil, fmt.Errorf("unsupported OpenAI account type %q", account.Type)
	}
	originalModel, stream, err := ParseOpenAIResponsesRequest(body)
	if err != nil {
		return nil, err
	}
	startedAt := time.Now()
	upstreamModel, err := resolveExactOpenAIAccountModel(account, originalModel)
	if compact {
		upstreamModel, err = resolveExactOpenAICompactAccountModel(account, originalModel)
	}
	if err != nil {
		return nil, err
	}
	if upstreamModel != originalModel {
		body, err = sjson.SetBytes(body, "model", upstreamModel)
		if err != nil {
			return nil, fmt.Errorf("map OpenAI Responses model: %w", err)
		}
	}

	token, err := s.nativeOpenAIResponsesCredential(ctx, account)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, errors.Join(&UpstreamFailoverError{
			StatusCode:        http.StatusBadGateway,
			Stage:             GatewayFailureStageAccountAuth,
			Scope:             GatewayFailureScopeAccount,
			Reason:            openAIResponsesCredentialUnavailableReason,
			NextAccountAction: NextAccountRetry,
		}, fmt.Errorf("get OpenAI account credential: %w", err))
	}
	upstreamReq, err := s.buildNativeOpenAIResponsesRequest(ctx, exchange, account, body, token, stream, compact)
	if err != nil {
		return nil, err
	}
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		return nil, errors.Join(&UpstreamFailoverError{
			StatusCode:        http.StatusBadGateway,
			Stage:             GatewayFailureStageInference,
			Scope:             GatewayFailureScopeAccount,
			Reason:            openAIResponsesTransportUnavailableReason,
			NextAccountAction: NextAccountRetry,
		}, fmt.Errorf("send OpenAI upstream request: %w", err))
	}
	if resp == nil || resp.Body == nil {
		return nil, errors.Join(&UpstreamFailoverError{
			StatusCode:        http.StatusBadGateway,
			Stage:             GatewayFailureStageInference,
			Scope:             GatewayFailureScopeAccount,
			Reason:            openAIResponsesTransportUnavailableReason,
			NextAccountAction: NextAccountRetry,
		}, errors.New("OpenAI upstream returned an empty HTTP response"))
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < http.StatusOK {
		return nil, fmt.Errorf("OpenAI upstream returned invalid final status %d", resp.StatusCode)
	}
	if resp.StatusCode >= http.StatusMultipleChoices {
		return nil, s.handleNativeOpenAIResponsesError(exchange, account, resp)
	}
	if account.Type == AccountTypeOAuth && !account.IsShadow() {
		if snapshot := ParseCodexRateLimitHeaders(resp.Header); snapshot != nil {
			s.updateCodexUsageSnapshot(ctx, account.ID, snapshot)
		}
	}

	responseKind, err := classifyNativeOpenAIResponsesContentType(resp.Header.Get("Content-Type"))
	if err != nil {
		return nil, err
	}
	if compact {
		if responseKind != nativeOpenAIResponsesContentJSON {
			return nil, fmt.Errorf("OpenAI compact request received non-JSON upstream content type %q", resp.Header.Get("Content-Type"))
		}
		return s.forwardNativeOpenAIResponsesJSON(exchange, resp, originalModel, upstreamModel, expectedEndpoint, startedAt)
	}
	if stream {
		if responseKind != nativeOpenAIResponsesContentSSE {
			return nil, fmt.Errorf("OpenAI streaming request received non-SSE upstream content type %q", resp.Header.Get("Content-Type"))
		}
		return s.forwardNativeOpenAIResponsesStream(exchange, resp, originalModel, upstreamModel, startedAt)
	}
	if responseKind == nativeOpenAIResponsesContentSSE {
		return s.collectNativeOpenAIResponsesStream(exchange, resp, originalModel, upstreamModel, startedAt)
	}
	return s.forwardNativeOpenAIResponsesJSON(exchange, resp, originalModel, upstreamModel, expectedEndpoint, startedAt)
}

type nativeOpenAIResponsesContentKind uint8

const (
	nativeOpenAIResponsesContentJSON nativeOpenAIResponsesContentKind = iota + 1
	nativeOpenAIResponsesContentSSE
)

func classifyNativeOpenAIResponsesContentType(value string) (nativeOpenAIResponsesContentKind, error) {
	if strings.TrimSpace(value) == "" {
		return 0, errors.New("OpenAI upstream response omitted Content-Type")
	}
	mediaType, _, err := mime.ParseMediaType(value)
	if err != nil {
		return 0, fmt.Errorf("parse OpenAI upstream Content-Type: %w", err)
	}
	mediaType = strings.ToLower(mediaType)
	switch {
	case mediaType == "text/event-stream":
		return nativeOpenAIResponsesContentSSE, nil
	case mediaType == "application/json":
		return nativeOpenAIResponsesContentJSON, nil
	case strings.HasPrefix(mediaType, "application/") && strings.HasSuffix(mediaType, "+json"):
		return nativeOpenAIResponsesContentJSON, nil
	default:
		return 0, fmt.Errorf("OpenAI upstream returned unsupported Content-Type %q", value)
	}
}

func (s *OpenAIGatewayService) nativeOpenAIResponsesCredential(ctx context.Context, account *Account) (string, error) {
	credentialAccount, err := resolveCredentialAccount(ctx, s.accountRepo, account)
	if err != nil {
		return "", err
	}
	if credentialAccount == nil {
		return "", errors.New("OpenAI credential account is missing")
	}
	switch credentialAccount.Type {
	case AccountTypeAPIKey:
		token := strings.TrimSpace(credentialAccount.GetOpenAIApiKey())
		if token == "" {
			return "", errors.New("OpenAI API key is missing")
		}
		return token, nil
	case AccountTypeOAuth:
		if credentialAccount.IsOpenAIAgentIdentity() {
			return "", nil
		}
		if s.openAITokenProvider == nil {
			return "", errors.New("OpenAI token provider is unavailable")
		}
		return s.openAITokenProvider.GetAccessToken(ctx, credentialAccount)
	default:
		return "", fmt.Errorf("unsupported OpenAI account type %q", credentialAccount.Type)
	}
}

func (s *OpenAIGatewayService) buildNativeOpenAIResponsesRequest(
	ctx context.Context,
	exchange gatewaytransport.Exchange,
	account *Account,
	body []byte,
	token string,
	stream bool,
	compact bool,
) (*http.Request, error) {
	targetURL := openaiPlatformAPIURL
	if account.Type == AccountTypeOAuth {
		targetURL = chatgptCodexURL
	} else if baseURL := strings.TrimSpace(account.GetOpenAIBaseURL()); baseURL != "" {
		validatedURL, err := s.validateUpstreamBaseURL(baseURL)
		if err != nil {
			return nil, err
		}
		targetURL = buildOpenAIResponsesURL(validatedURL)
	}
	if compact {
		targetURL = strings.TrimRight(targetURL, "/") + "/compact"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build OpenAI Responses request: %w", err)
	}
	req = req.WithContext(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAI))
	authHeaders, err := s.buildOpenAIAuthenticationHeaders(ctx, account, token)
	if err != nil {
		return nil, fmt.Errorf("build OpenAI authentication headers: %w", err)
	}
	req.Header = authHeaders.Clone()
	for key, values := range exchange.Request().Header {
		if !openaiAllowedHeaders[strings.ToLower(key)] {
			continue
		}
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	req.Header.Set("Content-Type", "application/json")
	if stream && !compact {
		req.Header.Set("Accept", "text/event-stream")
	} else {
		req.Header.Set("Accept", "application/json")
	}
	if account.Type == AccountTypeOAuth {
		req.Host = "chatgpt.com"
		if err := resolveAndSetOpenAIChatGPTAccountHeaders(ctx, s.accountRepo, req.Header, account); err != nil {
			return nil, fmt.Errorf("resolve ChatGPT account headers: %w", err)
		}
		req.Header.Set("OpenAI-Beta", "responses=experimental")
		req.Header.Set("originator", "next-api")
	}
	if customUA := strings.TrimSpace(account.GetOpenAIUserAgent()); customUA != "" {
		req.Header.Set("User-Agent", customUA)
	}
	return req, nil
}

func (s *OpenAIGatewayService) handleNativeOpenAIResponsesError(exchange gatewaytransport.Exchange, account *Account, resp *http.Response) error {
	body, err := readUpstreamResponseBodyExchange(resp.Body, s.cfg, exchange, nil)
	if err != nil {
		return err
	}
	headers := resp.Header.Clone()
	if nativeOpenAIResponsesFailoverStatus(resp.StatusCode) {
		return &UpstreamFailoverError{
			StatusCode:             resp.StatusCode,
			ResponseBody:           body,
			ResponseHeaders:        headers,
			RetryableOnSameAccount: account.IsPoolMode() && account.IsPoolModeRetryableStatus(resp.StatusCode),
			Stage:                  GatewayFailureStageInference,
			Scope:                  GatewayFailureScopeAccount,
			NextAccountAction:      NextAccountRetry,
		}
	}
	responseheaders.WriteFilteredHeaders(exchange.Response().Header(), headers, s.responseHeaderFilter)
	contentType := strings.TrimSpace(headers.Get("Content-Type"))
	MarkResponseCommitted(exchange.Values())
	writeErr := exchange.WriteData(resp.StatusCode, contentType, body)
	return errors.Join(fmt.Errorf("OpenAI upstream error: %d", resp.StatusCode), writeErr)
}

func nativeOpenAIResponsesFailoverStatus(status int) bool {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusRequestTimeout,
		http.StatusConflict, http.StatusTooManyRequests:
		return true
	default:
		return status >= http.StatusInternalServerError
	}
}

func (s *OpenAIGatewayService) forwardNativeOpenAIResponsesJSON(
	exchange gatewaytransport.Exchange,
	resp *http.Response,
	originalModel, upstreamModel string,
	upstreamEndpoint string,
	startedAt time.Time,
) (*OpenAIForwardResult, error) {
	body, err := readUpstreamResponseBodyExchange(resp.Body, s.cfg, exchange, nil)
	if err != nil {
		return nil, err
	}
	if !json.Valid(body) {
		return nil, errors.New("OpenAI upstream returned invalid JSON")
	}
	usage, ok, err := extractNativeOpenAIResponsesUsage(body)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.New("OpenAI upstream response omitted usage")
	}
	responseheaders.WriteFilteredHeaders(exchange.Response().Header(), resp.Header, s.responseHeaderFilter)
	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if err := exchange.WriteData(resp.StatusCode, contentType, body); err != nil {
		return nil, err
	}
	return nativeOpenAIResponsesResult(body, usage, originalModel, upstreamModel, upstreamEndpoint, false, resp, startedAt, nil), nil
}

func (s *OpenAIGatewayService) collectNativeOpenAIResponsesStream(
	exchange gatewaytransport.Exchange,
	resp *http.Response,
	originalModel, upstreamModel string,
	startedAt time.Time,
) (*OpenAIForwardResult, error) {
	body, err := readUpstreamResponseBodyExchange(resp.Body, s.cfg, exchange, nil)
	if err != nil {
		return nil, err
	}
	terminal, usage, ok, err := parseNativeOpenAIResponsesTerminal(body)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.New("OpenAI upstream stream ended without a completed response and usage")
	}
	responseheaders.WriteFilteredHeaders(exchange.Response().Header(), resp.Header, s.responseHeaderFilter)
	if err := exchange.WriteData(resp.StatusCode, "application/json", terminal); err != nil {
		return nil, err
	}
	return nativeOpenAIResponsesResult(terminal, usage, originalModel, upstreamModel, nativeOpenAIResponsesEndpoint, false, resp, startedAt, nil), nil
}

func parseNativeOpenAIResponsesTerminal(stream []byte) ([]byte, OpenAIUsage, bool, error) {
	var terminal []byte
	var usage OpenAIUsage
	usageFound := false
	err := scanNativeOpenAIResponsesSSE(bytes.NewReader(stream), defaultMaxLineSize, nil, func(data []byte) error {
		if !json.Valid(data) {
			return errors.New("OpenAI upstream emitted non-JSON SSE data")
		}
		eventType := gjson.GetBytes(data, "type").String()
		if eventType != "response.completed" && eventType != "response.done" {
			return nil
		}
		if terminal != nil {
			return errors.New("OpenAI upstream emitted multiple terminal responses")
		}
		response := gjson.GetBytes(data, "response")
		if !response.Exists() || !response.IsObject() || !json.Valid([]byte(response.Raw)) {
			return errors.New("OpenAI terminal SSE event omitted a valid response object")
		}
		terminal = append([]byte(nil), response.Raw...)
		if parsed, parsedOK, parseErr := extractNativeOpenAIResponsesUsage(data); parseErr != nil {
			return parseErr
		} else if parsedOK {
			usage = parsed
			usageFound = true
		}
		return nil
	})
	if err != nil {
		return nil, OpenAIUsage{}, false, err
	}
	if terminal == nil {
		return nil, OpenAIUsage{}, false, nil
	}
	if !usageFound {
		if parsed, ok, parseErr := extractNativeOpenAIResponsesUsage(terminal); parseErr != nil {
			return nil, OpenAIUsage{}, false, parseErr
		} else if ok {
			usage = parsed
			usageFound = true
		}
	}
	return terminal, usage, usageFound, nil
}

func (s *OpenAIGatewayService) forwardNativeOpenAIResponsesStream(
	exchange gatewaytransport.Exchange,
	resp *http.Response,
	originalModel, upstreamModel string,
	startedAt time.Time,
) (*OpenAIForwardResult, error) {
	writer := exchange.Response()
	if !writer.SupportsFlush() {
		return nil, errors.New("OpenAI streaming requires flush support")
	}
	responseheaders.WriteFilteredHeaders(writer.Header(), resp.Header, s.responseHeaderFilter)
	writer.Header().Set("Content-Type", "text/event-stream")
	writer.Header().Set("Cache-Control", "no-cache")
	writer.Header().Set("X-Accel-Buffering", "no")
	writer.WriteHeader(resp.StatusCode)

	maxLineSize := defaultMaxLineSize
	if s.cfg != nil && s.cfg.Gateway.MaxLineSize > 0 {
		maxLineSize = s.cfg.Gateway.MaxLineSize
	}
	usage := OpenAIUsage{}
	usageFound := false
	terminalFound := false
	responseID := ""
	imageCount := 0
	imageOutputSizes := []string(nil)
	webSearchCalls := 0
	var firstTokenMs *int

	processEvent := func(data []byte) error {
		if !json.Valid(data) {
			return errors.New("OpenAI upstream emitted non-JSON SSE data")
		}
		if firstTokenMs == nil {
			ms := int(time.Since(startedAt).Milliseconds())
			firstTokenMs = &ms
		}
		if parsed, ok, parseErr := extractNativeOpenAIResponsesUsage(data); parseErr != nil {
			return parseErr
		} else if ok {
			usage = parsed
			usageFound = true
		}
		eventType := gjson.GetBytes(data, "type").String()
		switch eventType {
		case "response.completed", "response.done":
			terminalFound = true
			responseID = strings.TrimSpace(gjson.GetBytes(data, "response.id").String())
			response := gjson.GetBytes(data, "response")
			if response.Exists() && response.IsObject() {
				raw := []byte(response.Raw)
				imageCount, imageOutputSizes, webSearchCalls = nativeOpenAIResponsesOutputTelemetry(raw)
			}
		case "response.failed", "response.incomplete", "response.cancelled", "response.canceled":
			return fmt.Errorf("OpenAI upstream terminated stream with %s", eventType)
		}
		return nil
	}

	err := scanNativeOpenAIResponsesSSE(resp.Body, maxLineSize, func(line string) error {
		if _, err := io.WriteString(writer, line+"\n"); err != nil {
			return err
		}
		if line == "" {
			if err := writer.Flush(); err != nil {
				return err
			}
		}
		return nil
	}, processEvent)
	if err != nil {
		return nil, err
	}
	if err := writer.Flush(); err != nil {
		return nil, err
	}
	if !terminalFound || !usageFound {
		return nil, errors.New("OpenAI upstream stream ended without a completed response and usage")
	}
	return &OpenAIForwardResult{
		ResponseID:       responseID,
		Usage:            usage,
		Model:            originalModel,
		UpstreamModel:    upstreamModel,
		UpstreamEndpoint: "/v1/responses",
		Stream:           true,
		ResponseHeaders:  resp.Header.Clone(),
		Duration:         time.Since(startedAt),
		FirstTokenMs:     firstTokenMs,
		ImageCount:       imageCount,
		ImageOutputSizes: imageOutputSizes,
		WebSearchCalls:   webSearchCalls,
	}, nil
}

func scanNativeOpenAIResponsesSSE(
	reader io.Reader,
	maxLineSize int,
	onLine func(string) error,
	onData func([]byte) error,
) error {
	if reader == nil {
		return errors.New("OpenAI SSE reader is nil")
	}
	if maxLineSize <= 0 {
		return errors.New("OpenAI SSE maximum line size must be positive")
	}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), maxLineSize)
	dataLines := make([]string, 0, 1)
	flushData := func() error {
		if len(dataLines) == 0 {
			return nil
		}
		data := []byte(strings.Join(dataLines, "\n"))
		dataLines = dataLines[:0]
		if bytes.Equal(bytes.TrimSpace(data), []byte("[DONE]")) {
			return nil
		}
		if onData != nil {
			return onData(data)
		}
		return nil
	}
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if onLine != nil {
			if err := onLine(line); err != nil {
				return err
			}
		}
		if line == "" {
			if err := flushData(); err != nil {
				return err
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			value := strings.TrimPrefix(line, "data:")
			value = strings.TrimPrefix(value, " ")
			dataLines = append(dataLines, value)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read OpenAI upstream stream: %w", err)
	}
	return flushData()
}

func nativeOpenAIResponsesResult(
	body []byte,
	usage OpenAIUsage,
	originalModel, upstreamModel string,
	upstreamEndpoint string,
	stream bool,
	resp *http.Response,
	startedAt time.Time,
	firstTokenMs *int,
) *OpenAIForwardResult {
	imageCount, imageOutputSizes, webSearchCalls := nativeOpenAIResponsesOutputTelemetry(body)
	return &OpenAIForwardResult{
		RequestID:          resp.Header.Get("X-Request-Id"),
		ResponseID:         extractOpenAIResponseIDFromJSONBytes(body),
		Usage:              usage,
		Model:              originalModel,
		UpstreamModel:      upstreamModel,
		UpstreamEndpoint:   upstreamEndpoint,
		Stream:             stream,
		ResponseHeaders:    resp.Header.Clone(),
		Duration:           time.Since(startedAt),
		FirstTokenMs:       firstTokenMs,
		ImageCount:         imageCount,
		ImageOutputSizes:   imageOutputSizes,
		ImageOutputSize:    "",
		ImageSizeSource:    "",
		ImageSizeBreakdown: nil,
		WebSearchCalls:     webSearchCalls,
	}
}
