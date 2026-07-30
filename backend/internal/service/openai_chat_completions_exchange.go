package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/gatewaytransport"
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/tidwall/sjson"
)

const (
	nativeOpenAIChatCompletionsEndpoint                        = "/v1/chat/completions"
	nativeOpenAICompletionsEndpoint                            = "/v1/completions"
	openAIChatCredentialUnavailableReason GatewayFailureReason = "openai_chat_credential_unavailable"
	openAIChatTransportUnavailableReason  GatewayFailureReason = "openai_chat_transport_unavailable"
)

// ParseOpenAIChatCompletionsRequest validates the routing fields used by the
// native direct Chat Completions transport.
func ParseOpenAIChatCompletionsRequest(body []byte) (model string, stream bool, err error) {
	return parseNativeOpenAICompletionsRequest(body, "Chat Completions")
}

// ParseOpenAICompletionsRequest validates the routing fields used by the
// native legacy Completions transport.
func ParseOpenAICompletionsRequest(body []byte) (model string, stream bool, err error) {
	return parseNativeOpenAICompletionsRequest(body, "Completions")
}

func parseNativeOpenAICompletionsRequest(body []byte, endpointName string) (model string, stream bool, err error) {
	if len(body) == 0 || !json.Valid(body) {
		return "", false, fmt.Errorf("OpenAI %s request must be valid JSON", endpointName)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(body, &object); err != nil || object == nil {
		return "", false, fmt.Errorf("OpenAI %s request must be a JSON object", endpointName)
	}
	modelRaw, ok := object["model"]
	if !ok {
		return "", false, fmt.Errorf("OpenAI %s model is required", endpointName)
	}
	if err := json.Unmarshal(modelRaw, &model); err != nil || model == "" || model != strings.TrimSpace(model) {
		return "", false, fmt.Errorf("OpenAI %s model must be an exact non-empty string", endpointName)
	}
	if streamRaw, exists := object["stream"]; exists {
		var streamValue any
		if err := json.Unmarshal(streamRaw, &streamValue); err != nil {
			return "", false, fmt.Errorf("OpenAI %s stream must be a boolean", endpointName)
		}
		var ok bool
		stream, ok = streamValue.(bool)
		if !ok {
			return "", false, fmt.Errorf("OpenAI %s stream must be a boolean", endpointName)
		}
	}
	return model, stream, nil
}

func prepareNativeOpenAIChatCompletionsBody(body []byte, upstreamModel string, stream bool) ([]byte, error) {
	updated, err := sjson.SetBytes(body, "model", upstreamModel)
	if err != nil {
		return nil, fmt.Errorf("map OpenAI Chat Completions model: %w", err)
	}
	if !stream {
		return updated, nil
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(updated, &object); err != nil || object == nil {
		return nil, errors.New("OpenAI Chat Completions request must be a JSON object")
	}
	if raw, exists := object["stream_options"]; exists {
		var options map[string]json.RawMessage
		if err := json.Unmarshal(raw, &options); err != nil || options == nil {
			return nil, errors.New("OpenAI Chat Completions stream_options must be an object")
		}
		if includeRaw, configured := options["include_usage"]; configured {
			var include any
			if err := json.Unmarshal(includeRaw, &include); err != nil {
				return nil, errors.New("OpenAI Chat Completions stream_options.include_usage must be a boolean")
			}
			if _, ok := include.(bool); !ok {
				return nil, errors.New("OpenAI Chat Completions stream_options.include_usage must be a boolean")
			}
		}
	}
	// The Runtime contract requires raw usage for every invocation. Asking the
	// upstream to emit its documented terminal usage chunk is transport
	// negotiation; all other request fields remain untouched.
	updated, err = sjson.SetBytes(updated, "stream_options.include_usage", true)
	if err != nil {
		return nil, fmt.Errorf("request OpenAI Chat Completions stream usage: %w", err)
	}
	return updated, nil
}

type nativeOpenAIChatUsage struct {
	PromptTokens     *int `json:"prompt_tokens"`
	CompletionTokens *int `json:"completion_tokens"`
	PromptDetails    *struct {
		CachedTokens        *int `json:"cached_tokens"`
		CacheCreationTokens *int `json:"cache_creation_tokens"`
		CacheWriteTokens    *int `json:"cache_write_tokens"`
	} `json:"prompt_tokens_details"`
}

type nativeOpenAIChatUsageEnvelope struct {
	Usage *nativeOpenAIChatUsage `json:"usage"`
}

func extractNativeOpenAIChatUsage(body []byte) (OpenAIUsage, bool, error) {
	var envelope nativeOpenAIChatUsageEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return OpenAIUsage{}, false, fmt.Errorf("parse OpenAI Chat Completions usage envelope: %w", err)
	}
	if envelope.Usage == nil {
		return OpenAIUsage{}, false, nil
	}
	usage := envelope.Usage
	if usage.PromptTokens == nil || usage.CompletionTokens == nil {
		return OpenAIUsage{}, false, errors.New("OpenAI Chat Completions usage omitted prompt_tokens or completion_tokens")
	}
	values := []*int{usage.PromptTokens, usage.CompletionTokens}
	if usage.PromptDetails != nil {
		values = append(values, usage.PromptDetails.CachedTokens, usage.PromptDetails.CacheCreationTokens, usage.PromptDetails.CacheWriteTokens)
	}
	for _, value := range values {
		if value != nil && *value < 0 {
			return OpenAIUsage{}, false, errors.New("OpenAI Chat Completions usage contains a negative token count")
		}
	}
	result := OpenAIUsage{InputTokens: *usage.PromptTokens, OutputTokens: *usage.CompletionTokens}
	if usage.PromptDetails != nil {
		if usage.PromptDetails.CachedTokens != nil {
			result.CacheReadInputTokens = *usage.PromptDetails.CachedTokens
		}
		if usage.PromptDetails.CacheCreationTokens != nil && usage.PromptDetails.CacheWriteTokens != nil {
			return OpenAIUsage{}, false, errors.New("OpenAI Chat Completions usage contains ambiguous cache creation token fields")
		}
		if usage.PromptDetails.CacheCreationTokens != nil {
			result.CacheCreationInputTokens = *usage.PromptDetails.CacheCreationTokens
		}
		if usage.PromptDetails.CacheWriteTokens != nil {
			result.CacheCreationInputTokens = *usage.PromptDetails.CacheWriteTokens
		}
	}
	return result, true, nil
}

// ForwardChatCompletionsExchange forwards one direct API-key Chat Completions
// request. Subscription-account protocol conversion is intentionally a
// separate adapter and is not selected by this method.
func (s *OpenAIGatewayService) ForwardChatCompletionsExchange(
	ctx context.Context,
	exchange gatewaytransport.Exchange,
	account *Account,
	body []byte,
) (*OpenAIForwardResult, error) {
	return s.forwardNativeOpenAICompletionsEndpoint(ctx, exchange, account, body, nativeOpenAIChatCompletionsEndpoint)
}

// ForwardCompletionsExchange forwards one direct API-key legacy Completions
// request without translating it through Chat or Responses.
func (s *OpenAIGatewayService) ForwardCompletionsExchange(
	ctx context.Context,
	exchange gatewaytransport.Exchange,
	account *Account,
	body []byte,
) (*OpenAIForwardResult, error) {
	return s.forwardNativeOpenAICompletionsEndpoint(ctx, exchange, account, body, nativeOpenAICompletionsEndpoint)
}

func (s *OpenAIGatewayService) forwardNativeOpenAICompletionsEndpoint(
	ctx context.Context,
	exchange gatewaytransport.Exchange,
	account *Account,
	body []byte,
	endpoint string,
) (*OpenAIForwardResult, error) {
	if exchange == nil || exchange.Request() == nil || exchange.Response() == nil {
		return nil, errors.New("OpenAI Chat Completions exchange is required")
	}
	if exchange.Request().URL == nil || exchange.Request().Method != http.MethodPost || exchange.Request().URL.Path != endpoint {
		return nil, errors.New("native OpenAI Chat Completions endpoint mismatch")
	}
	if account == nil || account.Platform != PlatformOpenAI || account.Type != AccountTypeAPIKey {
		return nil, errors.New("native direct OpenAI Chat Completions requires an OpenAI API-key account")
	}
	originalModel, stream, err := ParseOpenAIChatCompletionsRequest(body)
	if endpoint == nativeOpenAICompletionsEndpoint {
		originalModel, stream, err = ParseOpenAICompletionsRequest(body)
	}
	if err != nil {
		return nil, err
	}
	upstreamModel, err := resolveExactOpenAIAccountModel(account, originalModel)
	if err != nil {
		return nil, err
	}
	upstreamBody, err := prepareNativeOpenAIChatCompletionsBody(body, upstreamModel, stream)
	if err != nil {
		return nil, err
	}
	startedAt := time.Now()
	token, err := s.nativeOpenAIResponsesCredential(ctx, account)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, nativeOpenAIAccountFailoverError(
			http.StatusBadGateway,
			GatewayFailureStageAccountAuth,
			openAIChatCredentialUnavailableReason,
			fmt.Errorf("get OpenAI Chat Completions credential: %w", err),
		)
	}
	upstreamReq, err := s.buildNativeOpenAIChatCompletionsRequest(ctx, exchange, account, upstreamBody, token, stream, endpoint)
	if err != nil {
		return nil, err
	}
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		return nil, nativeOpenAIAccountFailoverError(
			http.StatusBadGateway,
			GatewayFailureStageInference,
			openAIChatTransportUnavailableReason,
			fmt.Errorf("send OpenAI Chat Completions request: %w", err),
		)
	}
	if resp == nil || resp.Body == nil {
		return nil, nativeOpenAIAccountFailoverError(
			http.StatusBadGateway,
			GatewayFailureStageInference,
			openAIChatTransportUnavailableReason,
			errors.New("OpenAI Chat Completions upstream returned an empty HTTP response"),
		)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < http.StatusOK {
		return nil, fmt.Errorf("OpenAI Chat Completions upstream returned invalid final status %d", resp.StatusCode)
	}
	if resp.StatusCode >= http.StatusMultipleChoices {
		return nil, s.handleNativeOpenAIChatCompletionsError(exchange, account, upstreamModel, resp)
	}
	kind, err := classifyNativeOpenAIChatCompletionsContentType(resp.Header.Get("Content-Type"))
	if err != nil {
		return nil, nativeOpenAIAccountFailoverError(http.StatusBadGateway, GatewayFailureStageInference, openAIChatTransportUnavailableReason, err)
	}
	if stream {
		if kind != nativeOpenAIChatContentSSE {
			return nil, nativeOpenAIAccountFailoverError(
				http.StatusBadGateway,
				GatewayFailureStageInference,
				openAIChatTransportUnavailableReason,
				errors.New("streaming OpenAI Chat Completions received a non-SSE upstream response"),
			)
		}
		return s.forwardNativeOpenAIChatCompletionsStream(exchange, resp, originalModel, upstreamModel, endpoint, startedAt)
	}
	if kind != nativeOpenAIChatContentJSON {
		return nil, nativeOpenAIAccountFailoverError(
			http.StatusBadGateway,
			GatewayFailureStageInference,
			openAIChatTransportUnavailableReason,
			errors.New("unary OpenAI Chat Completions received a non-JSON upstream response"),
		)
	}
	return s.forwardNativeOpenAIChatCompletionsJSON(exchange, resp, originalModel, upstreamModel, endpoint, startedAt)
}

func (s *OpenAIGatewayService) buildNativeOpenAIChatCompletionsRequest(
	ctx context.Context,
	exchange gatewaytransport.Exchange,
	account *Account,
	body []byte,
	token string,
	stream bool,
	endpoint string,
) (*http.Request, error) {
	var targetURL string
	var err error
	if endpoint == nativeOpenAICompletionsEndpoint {
		targetURL, err = s.openAICompletionsTargetURL(account)
	} else {
		targetURL, err = s.openAIChatCompletionsTargetURL(account)
	}
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build OpenAI Chat Completions request: %w", err)
	}
	req = req.WithContext(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAI))
	headers, err := s.buildOpenAIAuthenticationHeaders(ctx, account, token)
	if err != nil {
		return nil, fmt.Errorf("build OpenAI Chat Completions authentication headers: %w", err)
	}
	req.Header = headers.Clone()
	if language := strings.TrimSpace(exchange.RequestHeader("Accept-Language")); language != "" {
		req.Header.Set("Accept-Language", language)
	}
	if customUA := strings.TrimSpace(account.GetOpenAIUserAgent()); customUA != "" {
		req.Header.Set("User-Agent", customUA)
	}
	account.ApplyHeaderOverrides(req.Header)
	req.Header.Set("Content-Type", "application/json")
	if stream {
		req.Header.Set("Accept", "text/event-stream")
	} else {
		req.Header.Set("Accept", "application/json")
	}
	return req, nil
}

func (s *OpenAIGatewayService) openAICompletionsTargetURL(account *Account) (string, error) {
	baseURL := account.GetOpenAIBaseURL()
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	validatedURL, err := s.validateUpstreamBaseURL(baseURL)
	if err != nil {
		return "", fmt.Errorf("invalid base_url: %w", err)
	}
	return buildOpenAIEndpointURL(validatedURL, nativeOpenAICompletionsEndpoint), nil
}

type nativeOpenAIChatContentKind uint8

const (
	nativeOpenAIChatContentJSON nativeOpenAIChatContentKind = iota + 1
	nativeOpenAIChatContentSSE
)

func classifyNativeOpenAIChatCompletionsContentType(value string) (nativeOpenAIChatContentKind, error) {
	if strings.TrimSpace(value) == "" {
		return 0, errors.New("OpenAI Chat Completions upstream response omitted Content-Type")
	}
	mediaType, _, err := mime.ParseMediaType(value)
	if err != nil {
		return 0, fmt.Errorf("parse OpenAI Chat Completions upstream Content-Type: %w", err)
	}
	switch strings.ToLower(mediaType) {
	case "application/json":
		return nativeOpenAIChatContentJSON, nil
	case "text/event-stream":
		return nativeOpenAIChatContentSSE, nil
	default:
		if strings.HasPrefix(strings.ToLower(mediaType), "application/") && strings.HasSuffix(strings.ToLower(mediaType), "+json") {
			return nativeOpenAIChatContentJSON, nil
		}
		return 0, fmt.Errorf("OpenAI Chat Completions upstream returned unsupported Content-Type %q", value)
	}
}

func (s *OpenAIGatewayService) handleNativeOpenAIChatCompletionsError(exchange gatewaytransport.Exchange, account *Account, upstreamModel string, resp *http.Response) error {
	body, err := readUpstreamResponseBodyExchange(resp.Body, s.cfg, exchange, nil)
	if err != nil {
		return nativeOpenAIAccountFailoverError(http.StatusBadGateway, GatewayFailureStageInference, openAIChatTransportUnavailableReason, err)
	}
	if nativeOpenAIResponsesFailoverStatus(resp.StatusCode) {
		return &UpstreamFailoverError{
			StatusCode:             resp.StatusCode,
			ResponseBody:           body,
			ResponseHeaders:        resp.Header.Clone(),
			RetryableOnSameAccount: account.IsPoolMode() && account.IsPoolModeRetryableStatus(resp.StatusCode),
			Stage:                  GatewayFailureStageInference,
			Scope:                  GatewayFailureScopeAccount,
			NextAccountAction:      NextAccountRetry,
		}
	}
	responseheaders.WriteFilteredHeaders(exchange.Response().Header(), resp.Header, s.responseHeaderFilter)
	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	MarkResponseCommitted(exchange.Values())
	return errors.Join(
		fmt.Errorf("OpenAI Chat Completions upstream error for model %q: %d", upstreamModel, resp.StatusCode),
		exchange.WriteData(resp.StatusCode, contentType, body),
	)
}

func (s *OpenAIGatewayService) forwardNativeOpenAIChatCompletionsJSON(
	exchange gatewaytransport.Exchange,
	resp *http.Response,
	originalModel string,
	upstreamModel string,
	endpoint string,
	startedAt time.Time,
) (*OpenAIForwardResult, error) {
	body, err := readUpstreamResponseBodyExchange(resp.Body, s.cfg, exchange, nil)
	if err != nil {
		return nil, nativeOpenAIAccountFailoverError(http.StatusBadGateway, GatewayFailureStageInference, openAIChatTransportUnavailableReason, err)
	}
	if !json.Valid(body) {
		return nil, nativeOpenAIAccountFailoverError(
			http.StatusBadGateway,
			GatewayFailureStageInference,
			openAIChatTransportUnavailableReason,
			errors.New("OpenAI Chat Completions upstream returned invalid JSON"),
		)
	}
	usage, found, err := extractNativeOpenAIChatUsage(body)
	if err != nil || !found {
		if err == nil {
			err = errors.New("OpenAI Chat Completions upstream response omitted usage")
		}
		return nil, nativeOpenAIAccountFailoverError(http.StatusBadGateway, GatewayFailureStageInference, openAIChatTransportUnavailableReason, err)
	}
	responseheaders.WriteFilteredHeaders(exchange.Response().Header(), resp.Header, s.responseHeaderFilter)
	if err := exchange.WriteData(resp.StatusCode, resp.Header.Get("Content-Type"), body); err != nil {
		return nil, err
	}
	return &OpenAIForwardResult{
		RequestID:        resp.Header.Get("X-Request-Id"),
		Usage:            usage,
		Model:            originalModel,
		UpstreamModel:    upstreamModel,
		UpstreamEndpoint: endpoint,
		Duration:         time.Since(startedAt),
	}, nil
}

func (s *OpenAIGatewayService) forwardNativeOpenAIChatCompletionsStream(
	exchange gatewaytransport.Exchange,
	resp *http.Response,
	originalModel string,
	upstreamModel string,
	endpoint string,
	startedAt time.Time,
) (*OpenAIForwardResult, error) {
	writer := exchange.Response()
	if !writer.SupportsFlush() {
		return nil, errors.New("OpenAI Chat Completions streaming requires flush support")
	}
	responseheaders.WriteFilteredHeaders(writer.Header(), resp.Header, s.responseHeaderFilter)
	writer.Header().Set("Content-Type", "text/event-stream")
	writer.Header().Set("Cache-Control", "no-cache")
	writer.Header().Set("X-Accel-Buffering", "no")
	writer.WriteHeader(resp.StatusCode)

	var usage OpenAIUsage
	usageFound := false
	doneFound := false
	var firstTokenMs *int
	maxLineSize := defaultMaxLineSize
	if s.cfg != nil && s.cfg.Gateway.MaxLineSize > 0 {
		maxLineSize = s.cfg.Gateway.MaxLineSize
	}
	processData := func(data []byte) error {
		if doneFound {
			return errors.New("OpenAI Chat Completions upstream emitted data after [DONE]")
		}
		if !json.Valid(data) {
			return errors.New("OpenAI Chat Completions upstream emitted non-JSON SSE data")
		}
		if firstTokenMs == nil {
			elapsed := int(time.Since(startedAt).Milliseconds())
			firstTokenMs = &elapsed
		}
		parsed, found, err := extractNativeOpenAIChatUsage(data)
		if err != nil {
			return err
		}
		if found {
			if usageFound {
				return errors.New("OpenAI Chat Completions upstream emitted multiple usage chunks")
			}
			usage = parsed
			usageFound = true
		}
		return nil
	}
	streamErr := scanNativeOpenAIResponsesSSE(resp.Body, maxLineSize, func(line string) error {
		if _, err := writer.Write([]byte(line + "\n")); err != nil {
			return err
		}
		if strings.HasPrefix(line, "data:") && strings.TrimSpace(strings.TrimPrefix(line, "data:")) == "[DONE]" {
			if doneFound {
				return errors.New("OpenAI Chat Completions upstream emitted multiple [DONE] sentinels")
			}
			doneFound = true
		}
		if line == "" {
			return writer.Flush()
		}
		return nil
	}, processData)
	flushErr := writer.Flush()
	result := &OpenAIForwardResult{
		RequestID:        resp.Header.Get("X-Request-Id"),
		Usage:            usage,
		Model:            originalModel,
		UpstreamModel:    upstreamModel,
		UpstreamEndpoint: endpoint,
		Stream:           true,
		Duration:         time.Since(startedAt),
		FirstTokenMs:     firstTokenMs,
	}
	if streamErr != nil || flushErr != nil {
		return result, errors.Join(streamErr, flushErr)
	}
	if !doneFound || !usageFound {
		return result, errors.New("OpenAI Chat Completions upstream stream ended without [DONE] and exact usage")
	}
	return result, nil
}
