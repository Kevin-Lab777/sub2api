package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/gatewaytransport"
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/tidwall/sjson"
)

const (
	nativeOpenAIEmbeddingsEndpoint = "/v1/embeddings"

	openAIEmbeddingsCredentialUnavailableReason GatewayFailureReason = "openai_embeddings_credential_unavailable"
	openAIEmbeddingsTransportUnavailableReason  GatewayFailureReason = "openai_embeddings_transport_unavailable"
)

// ParseOpenAIEmbeddingsRequest validates the routing field shared by the
// dispatcher and provider forwarder without rewriting any request content.
func ParseOpenAIEmbeddingsRequest(body []byte) (string, error) {
	if len(body) == 0 || !json.Valid(body) {
		return "", errors.New("OpenAI Embeddings request must be valid JSON")
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(body, &object); err != nil || object == nil {
		return "", errors.New("OpenAI Embeddings request must be a JSON object")
	}
	var model string
	if err := json.Unmarshal(object["model"], &model); err != nil || model == "" || model != strings.TrimSpace(model) {
		return "", errors.New("OpenAI Embeddings model must be an exact non-empty string")
	}
	return model, nil
}

func prepareNativeOpenAIEmbeddingsBody(body []byte, upstreamModel string) ([]byte, error) {
	updated, err := sjson.SetBytes(body, "model", upstreamModel)
	if err != nil {
		return nil, fmt.Errorf("map OpenAI Embeddings model: %w", err)
	}
	return updated, nil
}

// ForwardEmbeddingsExchange forwards the raw Embeddings protocol through one
// selected API-key account and returns exact upstream usage for New API.
func (s *OpenAIGatewayService) ForwardEmbeddingsExchange(
	ctx context.Context,
	exchange gatewaytransport.Exchange,
	account *Account,
	body []byte,
) (*OpenAIForwardResult, error) {
	if exchange == nil || exchange.Request() == nil || exchange.Response() == nil || exchange.Request().URL == nil ||
		exchange.Request().Method != http.MethodPost || exchange.Request().URL.Path != nativeOpenAIEmbeddingsEndpoint {
		return nil, errors.New("native OpenAI Embeddings endpoint mismatch")
	}
	if account == nil || account.Platform != PlatformOpenAI || account.Type != AccountTypeAPIKey {
		return nil, errors.New("native OpenAI Embeddings requires an OpenAI API-key account")
	}
	originalModel, err := ParseOpenAIEmbeddingsRequest(body)
	if err != nil {
		return nil, err
	}
	upstreamModel, err := resolveExactOpenAIAccountModel(account, originalModel)
	if err != nil {
		return nil, err
	}
	upstreamBody, err := prepareNativeOpenAIEmbeddingsBody(body, upstreamModel)
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
			openAIEmbeddingsCredentialUnavailableReason,
			fmt.Errorf("get OpenAI Embeddings credential: %w", err),
		)
	}
	upstreamReq, err := s.buildNativeOpenAIEmbeddingsRequest(ctx, exchange, account, upstreamBody, token)
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
			openAIEmbeddingsTransportUnavailableReason,
			fmt.Errorf("send OpenAI Embeddings request: %w", err),
		)
	}
	if resp == nil || resp.Body == nil {
		return nil, nativeOpenAIAccountFailoverError(
			http.StatusBadGateway,
			GatewayFailureStageInference,
			openAIEmbeddingsTransportUnavailableReason,
			errors.New("OpenAI Embeddings upstream returned an empty HTTP response"),
		)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < http.StatusOK {
		return nil, fmt.Errorf("OpenAI Embeddings upstream returned invalid final status %d", resp.StatusCode)
	}
	if resp.StatusCode >= http.StatusMultipleChoices {
		return nil, s.handleNativeOpenAIEmbeddingsError(exchange, account, resp)
	}
	kind, err := classifyNativeOpenAIResponsesContentType(resp.Header.Get("Content-Type"))
	if err != nil || kind != nativeOpenAIResponsesContentJSON {
		if err == nil {
			err = errors.New("OpenAI Embeddings upstream response must be JSON")
		}
		return nil, nativeOpenAIAccountFailoverError(http.StatusBadGateway, GatewayFailureStageInference, openAIEmbeddingsTransportUnavailableReason, err)
	}
	respBody, err := readUpstreamResponseBodyExchange(resp.Body, s.cfg, exchange, nil)
	if err != nil {
		return nil, nativeOpenAIAccountFailoverError(http.StatusBadGateway, GatewayFailureStageInference, openAIEmbeddingsTransportUnavailableReason, err)
	}
	usage, err := extractNativeOpenAIEmbeddingsUsage(respBody)
	if err != nil {
		return nil, nativeOpenAIAccountFailoverError(http.StatusBadGateway, GatewayFailureStageInference, openAIEmbeddingsTransportUnavailableReason, err)
	}
	responseheaders.WriteFilteredHeaders(exchange.Response().Header(), resp.Header, s.responseHeaderFilter)
	if err := exchange.WriteData(resp.StatusCode, resp.Header.Get("Content-Type"), respBody); err != nil {
		return nil, err
	}
	return &OpenAIForwardResult{
		RequestID:        firstNonEmptyString(resp.Header.Get("X-Request-Id"), resp.Header.Get("Request-Id")),
		Usage:            usage,
		Model:            originalModel,
		BillingModel:     upstreamModel,
		UpstreamModel:    upstreamModel,
		UpstreamEndpoint: nativeOpenAIEmbeddingsEndpoint,
		Duration:         time.Since(startedAt),
	}, nil
}

func (s *OpenAIGatewayService) buildNativeOpenAIEmbeddingsRequest(
	ctx context.Context,
	exchange gatewaytransport.Exchange,
	account *Account,
	body []byte,
	token string,
) (*http.Request, error) {
	baseURL := strings.TrimSpace(account.GetOpenAIBaseURL())
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	validatedURL, err := s.validateUpstreamBaseURL(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid OpenAI Embeddings base_url: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, buildOpenAIEmbeddingsURL(validatedURL), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build OpenAI Embeddings request: %w", err)
	}
	req = req.WithContext(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAI))
	headers, err := s.buildOpenAIAuthenticationHeaders(ctx, account, token)
	if err != nil {
		return nil, fmt.Errorf("build OpenAI Embeddings authentication headers: %w", err)
	}
	req.Header = headers.Clone()
	for key, values := range exchange.Request().Header {
		if !openaiCCRawAllowedHeaders[strings.ToLower(key)] {
			continue
		}
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	if customUA := strings.TrimSpace(account.GetOpenAIUserAgent()); customUA != "" {
		req.Header.Set("User-Agent", customUA)
	}
	account.ApplyHeaderOverrides(req.Header)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	return req, nil
}

func (s *OpenAIGatewayService) handleNativeOpenAIEmbeddingsError(exchange gatewaytransport.Exchange, account *Account, resp *http.Response) error {
	body, err := readUpstreamResponseBodyExchange(resp.Body, s.cfg, exchange, nil)
	if err != nil {
		return nativeOpenAIAccountFailoverError(http.StatusBadGateway, GatewayFailureStageInference, openAIEmbeddingsTransportUnavailableReason, err)
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
	MarkResponseCommitted(exchange.Values())
	return errors.Join(
		fmt.Errorf("OpenAI Embeddings upstream error: %d", resp.StatusCode),
		exchange.WriteData(resp.StatusCode, resp.Header.Get("Content-Type"), body),
	)
}

type nativeOpenAIEmbeddingsTokenDetails struct {
	CachedTokens        *int `json:"cached_tokens"`
	CacheCreationTokens *int `json:"cache_creation_tokens"`
	CacheWriteTokens    *int `json:"cache_write_tokens"`
	ImageTokens         *int `json:"image_tokens"`
}

type nativeOpenAIEmbeddingsUsage struct {
	PromptTokens        *int                                `json:"prompt_tokens"`
	InputTokens         *int                                `json:"input_tokens"`
	TotalTokens         *int                                `json:"total_tokens"`
	PromptTokensDetails *nativeOpenAIEmbeddingsTokenDetails `json:"prompt_tokens_details"`
	InputTokensDetails  *nativeOpenAIEmbeddingsTokenDetails `json:"input_tokens_details"`
}

func extractNativeOpenAIEmbeddingsUsage(body []byte) (OpenAIUsage, error) {
	var envelope struct {
		Usage *nativeOpenAIEmbeddingsUsage `json:"usage"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return OpenAIUsage{}, fmt.Errorf("parse OpenAI Embeddings response: %w", err)
	}
	if envelope.Usage == nil {
		return OpenAIUsage{}, errors.New("OpenAI Embeddings response omitted usage")
	}
	inputTokens, found, err := exactNativeOpenAIEmbeddingsCount("input tokens", envelope.Usage.PromptTokens, envelope.Usage.InputTokens)
	if err != nil {
		return OpenAIUsage{}, err
	}
	if !found {
		return OpenAIUsage{}, errors.New("OpenAI Embeddings usage omitted prompt_tokens or input_tokens")
	}
	totalTokens, found, err := exactNativeOpenAIEmbeddingsCount("total tokens", envelope.Usage.TotalTokens)
	if err != nil {
		return OpenAIUsage{}, err
	}
	if !found {
		return OpenAIUsage{}, errors.New("OpenAI Embeddings usage omitted total_tokens")
	}
	if totalTokens != inputTokens {
		return OpenAIUsage{}, fmt.Errorf("OpenAI Embeddings total_tokens %d does not equal input token count %d", totalTokens, inputTokens)
	}

	var prompt, input *nativeOpenAIEmbeddingsTokenDetails
	prompt = envelope.Usage.PromptTokensDetails
	input = envelope.Usage.InputTokensDetails
	detailPointers := func(selectField func(*nativeOpenAIEmbeddingsTokenDetails) *int) []*int {
		values := make([]*int, 0, 2)
		if prompt != nil {
			values = append(values, selectField(prompt))
		}
		if input != nil {
			values = append(values, selectField(input))
		}
		return values
	}
	cacheRead, _, err := exactNativeOpenAIEmbeddingsCount("cached input tokens", detailPointers(func(details *nativeOpenAIEmbeddingsTokenDetails) *int { return details.CachedTokens })...)
	if err != nil {
		return OpenAIUsage{}, err
	}
	cacheCreationValues := detailPointers(func(details *nativeOpenAIEmbeddingsTokenDetails) *int { return details.CacheCreationTokens })
	cacheCreationValues = append(cacheCreationValues, detailPointers(func(details *nativeOpenAIEmbeddingsTokenDetails) *int { return details.CacheWriteTokens })...)
	cacheCreation, _, err := exactNativeOpenAIEmbeddingsCount("cache creation input tokens", cacheCreationValues...)
	if err != nil {
		return OpenAIUsage{}, err
	}
	imageInput, _, err := exactNativeOpenAIEmbeddingsCount("image input tokens", detailPointers(func(details *nativeOpenAIEmbeddingsTokenDetails) *int { return details.ImageTokens })...)
	if err != nil {
		return OpenAIUsage{}, err
	}
	for _, detail := range []struct {
		label string
		count int
	}{
		{label: "cached input tokens", count: cacheRead},
		{label: "cache creation input tokens", count: cacheCreation},
		{label: "image input tokens", count: imageInput},
	} {
		if detail.count > inputTokens {
			return OpenAIUsage{}, fmt.Errorf("OpenAI Embeddings %s %d exceeds input token count %d", detail.label, detail.count, inputTokens)
		}
	}
	return OpenAIUsage{
		InputTokens:              inputTokens,
		ImageInputTokens:         imageInput,
		CacheReadInputTokens:     cacheRead,
		CacheCreationInputTokens: cacheCreation,
	}, nil
}

func exactNativeOpenAIEmbeddingsCount(label string, values ...*int) (int, bool, error) {
	result := 0
	found := false
	for _, value := range values {
		if value == nil {
			continue
		}
		if *value < 0 {
			return 0, false, fmt.Errorf("OpenAI Embeddings %s cannot be negative", label)
		}
		if found && *value != result {
			return 0, false, fmt.Errorf("OpenAI Embeddings %s fields are contradictory", label)
		}
		result = *value
		found = true
	}
	return result, found, nil
}
