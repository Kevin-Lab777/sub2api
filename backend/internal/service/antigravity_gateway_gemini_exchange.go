package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/gatewaytransport"
	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
)

// ValidateTechnicalRuntime verifies only the dependencies needed by the
// framework-independent Antigravity provider path.
func (s *AntigravityGatewayService) ValidateTechnicalRuntime() error {
	if s == nil {
		return errors.New("antigravity gateway service is nil")
	}
	missing := make([]string, 0, 2)
	if s.tokenProvider == nil {
		missing = append(missing, "Antigravity token provider")
	}
	if s.httpUpstream == nil {
		missing = append(missing, "upstream HTTP transport")
	}
	if len(missing) > 0 {
		return fmt.Errorf("technical Antigravity dependencies are incomplete: %s", strings.Join(missing, ", "))
	}
	return nil
}

// ForwardGeminiExchange forwards one native Gemini request through an
// Antigravity account. It performs exactly one provider request. Account
// retry/failover remains the dispatcher's responsibility.
func (s *AntigravityGatewayService) ForwardGeminiExchange(
	ctx context.Context,
	exchange gatewaytransport.Exchange,
	account *Account,
	originalModel string,
	action string,
	stream bool,
	body []byte,
) (*ForwardResult, error) {
	if err := s.ValidateTechnicalRuntime(); err != nil {
		return nil, err
	}
	if exchange == nil || exchange.Request() == nil || exchange.Response() == nil {
		return nil, errors.New("antigravity Gemini exchange is required")
	}
	if account == nil {
		return nil, errors.New("antigravity account is required")
	}
	if account.Platform != PlatformAntigravity {
		return nil, fmt.Errorf("antigravity Gemini forwarding requires an Antigravity account, got %q", account.Platform)
	}
	if account.Type != AccountTypeOAuth && account.Type != AccountTypeUpstream {
		return nil, fmt.Errorf("unsupported Antigravity account type %q", account.Type)
	}
	if !account.IsMixedSchedulingEnabled() {
		return nil, errors.New("Antigravity account is not enabled for mixed scheduling")
	}
	if strings.TrimSpace(originalModel) == "" {
		return nil, errors.New("Antigravity Gemini model is required")
	}
	switch action {
	case "generateContent", "streamGenerateContent":
		// Antigravity exposes generation only through streamGenerateContent.
	case "countTokens":
		return nil, errors.New("Antigravity does not expose an exact Gemini countTokens operation")
	default:
		return nil, fmt.Errorf("unsupported Antigravity Gemini action %q", action)
	}
	if len(body) == 0 {
		return nil, errors.New("Antigravity Gemini request body is empty")
	}

	startedAt := time.Now()
	mappedModel := mapAntigravityModel(account, originalModel)
	if mappedModel == "" {
		return nil, fmt.Errorf("model %q has no Antigravity mapping", originalModel)
	}
	projectID := strings.TrimSpace(account.GetCredential("project_id"))
	if projectID == "" {
		return nil, errors.New("Antigravity project_id is required")
	}
	accessToken, err := s.tokenProvider.GetAccessToken(ctx, account)
	if err != nil {
		return nil, errors.Join(
			&UpstreamFailoverError{StatusCode: http.StatusBadGateway},
			fmt.Errorf("get Antigravity access token: %w", err),
		)
	}
	providerBody, err := s.buildStrictAntigravityGeminiRequest(projectID, mappedModel, body)
	if err != nil {
		return nil, err
	}
	baseURL := resolveAntigravityForwardBaseURL()
	if baseURL == "" {
		return nil, errors.New("no Antigravity forward base URL configured")
	}
	upstreamReq, err := antigravity.NewAPIRequestWithURL(ctx, baseURL, "streamGenerateContent", accessToken, providerBody)
	if err != nil {
		return nil, fmt.Errorf("build Antigravity Gemini request: %w", err)
	}

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		return nil, errors.Join(
			&UpstreamFailoverError{
				StatusCode:             http.StatusBadGateway,
				RetryableOnSameAccount: account.IsPoolMode() && account.IsPoolModeRetryableStatus(http.StatusBadGateway),
			},
			fmt.Errorf("Antigravity Gemini transport: %w", err),
		)
	}
	if resp == nil || resp.Body == nil {
		return nil, errors.Join(
			&UpstreamFailoverError{
				StatusCode:             http.StatusBadGateway,
				RetryableOnSameAccount: account.IsPoolMode() && account.IsPoolModeRetryableStatus(http.StatusBadGateway),
			},
			errors.New("Antigravity Gemini upstream returned an empty response"),
		)
	}
	defer func() { _ = resp.Body.Close() }()

	requestID := strings.TrimSpace(resp.Header.Get("x-request-id"))
	if requestID == "" {
		requestID = strings.TrimSpace(resp.Header.Get("x-goog-request-id"))
	}
	if requestID != "" {
		exchange.SetResponseHeader("x-request-id", requestID)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		respBody, readErr := readUpstreamResponseBodyLimited(resp.Body, s.antigravityGeminiResponseLimit())
		if readErr != nil {
			return nil, fmt.Errorf("read Antigravity Gemini error response: %w", readErr)
		}
		downstreamBody, unwrapErr := unwrapAntigravityGeminiResponseIfPresent(respBody)
		if unwrapErr != nil {
			return nil, fmt.Errorf("decode Antigravity Gemini error response: %w", unwrapErr)
		}
		if s.shouldFailoverUpstreamError(resp.StatusCode) {
			return nil, &UpstreamFailoverError{
				StatusCode:             resp.StatusCode,
				ResponseBody:           downstreamBody,
				ResponseHeaders:        resp.Header.Clone(),
				RetryableOnSameAccount: account.IsPoolMode() && account.IsPoolModeRetryableStatus(resp.StatusCode),
			}
		}
		contentType := resp.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "application/json"
		}
		MarkResponseCommitted(exchange.Values())
		if err := exchange.WriteData(resp.StatusCode, contentType, downstreamBody); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("Antigravity Gemini upstream error: %d", resp.StatusCode)
	}

	stats, err := s.forwardStrictAntigravityGeminiResponse(exchange, resp, startedAt, stream)
	if err != nil {
		return nil, err
	}
	usage := ClaudeUsage{}
	if stats.usage != nil {
		usage = *stats.usage
	}
	return &ForwardResult{
		RequestID:     requestID,
		Usage:         usage,
		Model:         originalModel,
		UpstreamModel: mappedModel,
		Stream:        stream,
		Duration:      time.Since(startedAt),
		FirstTokenMs:  stats.firstTokenMs,
		ImageCount:    stats.imageCount,
	}, nil
}

func (s *AntigravityGatewayService) buildStrictAntigravityGeminiRequest(projectID, mappedModel string, body []byte) ([]byte, error) {
	request, err := decodeStrictJSONObject(body)
	if err != nil {
		return nil, fmt.Errorf("decode Antigravity Gemini request: %w", err)
	}
	identityPart := map[string]any{"text": antigravity.GetDefaultIdentityPatch()}
	if current, ok := request["systemInstruction"]; ok {
		systemInstruction, ok := current.(map[string]any)
		if !ok {
			return nil, errors.New("Gemini systemInstruction must be an object")
		}
		partsValue, exists := systemInstruction["parts"]
		if !exists {
			systemInstruction["parts"] = []any{identityPart}
		} else {
			parts, ok := partsValue.([]any)
			if !ok {
				return nil, errors.New("Gemini systemInstruction.parts must be an array")
			}
			systemInstruction["parts"] = append([]any{identityPart}, parts...)
		}
	} else {
		request["systemInstruction"] = map[string]any{"parts": []any{identityPart}}
	}
	requestBody, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("encode Antigravity Gemini request: %w", err)
	}
	wrapped, err := s.wrapV1InternalRequest(projectID, mappedModel, requestBody)
	if err != nil {
		return nil, fmt.Errorf("wrap Antigravity Gemini request: %w", err)
	}
	return wrapped, nil
}

type strictAntigravityGeminiStats struct {
	usage        *ClaudeUsage
	firstTokenMs *int
	imageCount   int
}

func (s *AntigravityGatewayService) forwardStrictAntigravityGeminiResponse(
	exchange gatewaytransport.Exchange,
	resp *http.Response,
	startedAt time.Time,
	stream bool,
) (*strictAntigravityGeminiStats, error) {
	if stream {
		return s.streamStrictAntigravityGeminiResponse(exchange, resp, startedAt)
	}
	return s.collectStrictAntigravityGeminiResponse(exchange, resp, startedAt)
}

func (s *AntigravityGatewayService) streamStrictAntigravityGeminiResponse(
	exchange gatewaytransport.Exchange,
	resp *http.Response,
	startedAt time.Time,
) (*strictAntigravityGeminiStats, error) {
	writer := exchange.Response()
	if !writer.SupportsFlush() {
		return nil, errors.New("Antigravity Gemini streaming is not supported by the response writer")
	}
	exchange.SetResponseHeader("Content-Type", "text/event-stream; charset=utf-8")
	exchange.SetResponseHeader("Cache-Control", "no-cache")
	exchange.SetResponseHeader("X-Accel-Buffering", "no")

	stats := &strictAntigravityGeminiStats{}
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), s.antigravityGeminiMaxLineSize())
	written := false
	for scanner.Scan() {
		line := scanner.Bytes()
		trimmed := bytes.TrimRight(line, "\r")
		if !bytes.HasPrefix(trimmed, []byte("data:")) {
			if len(bytes.TrimSpace(trimmed)) == 0 {
				continue
			}
			if !written {
				writer.WriteHeader(resp.StatusCode)
				written = true
			}
			if _, err := writer.Write(append(append([]byte(nil), line...), '\n')); err != nil {
				return nil, err
			}
			if err := writer.Flush(); err != nil {
				return nil, err
			}
			continue
		}

		payload := bytes.TrimSpace(bytes.TrimPrefix(trimmed, []byte("data:")))
		if len(payload) == 0 {
			continue
		}
		if bytes.Equal(payload, []byte("[DONE]")) {
			if stats.firstTokenMs == nil {
				return nil, errors.New("Antigravity Gemini stream terminated before a response event")
			}
			if !written {
				writer.WriteHeader(resp.StatusCode)
				written = true
			}
			if _, err := fmt.Fprintf(writer, "data: %s\n\n", payload); err != nil {
				return nil, err
			}
			if err := writer.Flush(); err != nil {
				return nil, err
			}
			continue
		}

		responseBody, err := unwrapStrictAntigravityGeminiResponse(payload)
		if err != nil {
			return nil, fmt.Errorf("decode Antigravity Gemini stream event: %w", err)
		}
		stats.observe(responseBody, startedAt)
		if !written {
			writer.WriteHeader(resp.StatusCode)
			written = true
		}
		if _, err := fmt.Fprintf(writer, "data: %s\n\n", responseBody); err != nil {
			return nil, err
		}
		if err := writer.Flush(); err != nil {
			return nil, err
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read Antigravity Gemini stream: %w", err)
	}
	if !written {
		return nil, errors.New("Antigravity Gemini upstream returned no stream events")
	}
	return stats, nil
}

func (s *AntigravityGatewayService) collectStrictAntigravityGeminiResponse(
	exchange gatewaytransport.Exchange,
	resp *http.Response,
	startedAt time.Time,
) (*strictAntigravityGeminiStats, error) {
	stats := &strictAntigravityGeminiStats{}
	accumulator := newGeminiResponseAccumulator()
	maxBytes := s.antigravityGeminiResponseLimit()
	maxLineSize := s.antigravityGeminiMaxLineSize()
	if maxBytes < int64(maxLineSize) {
		maxLineSize = int(maxBytes) + 1
	}
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), maxLineSize)
	var readBytes int64
	for scanner.Scan() {
		line := scanner.Bytes()
		readBytes += int64(len(line)) + 1
		if readBytes > maxBytes {
			return nil, fmt.Errorf("%w: limit=%d", ErrUpstreamResponseBodyTooLarge, maxBytes)
		}
		trimmed := bytes.TrimRight(line, "\r")
		if !bytes.HasPrefix(trimmed, []byte("data:")) {
			continue
		}
		payload := bytes.TrimSpace(bytes.TrimPrefix(trimmed, []byte("data:")))
		if len(payload) == 0 || bytes.Equal(payload, []byte("[DONE]")) {
			continue
		}
		responseBody, err := unwrapStrictAntigravityGeminiResponse(payload)
		if err != nil {
			return nil, fmt.Errorf("decode Antigravity Gemini stream event: %w", err)
		}
		stats.observe(responseBody, startedAt)
		if err := accumulator.Add(responseBody); err != nil {
			return nil, fmt.Errorf("accumulate Antigravity Gemini stream event: %w", err)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read Antigravity Gemini stream: %w", err)
	}
	responseBody, err := accumulator.Marshal()
	if err != nil {
		return nil, err
	}
	if err := exchange.WriteData(resp.StatusCode, "application/json", responseBody); err != nil {
		return nil, err
	}
	return stats, nil
}

func (s *strictAntigravityGeminiStats) observe(responseBody []byte, startedAt time.Time) {
	if usage := extractGeminiUsage(responseBody); usage != nil {
		s.usage = usage
	}
	if s.firstTokenMs == nil {
		elapsed := int(time.Since(startedAt).Milliseconds())
		s.firstTokenMs = &elapsed
	}
	s.imageCount += countGeminiInlineImages(responseBody)
}

func countGeminiInlineImages(responseBody []byte) int {
	response, err := decodeStrictJSONObject(responseBody)
	if err != nil {
		return 0
	}
	candidates, _ := response["candidates"].([]any)
	count := 0
	for _, candidateValue := range candidates {
		candidate, _ := candidateValue.(map[string]any)
		content, _ := candidate["content"].(map[string]any)
		parts, _ := content["parts"].([]any)
		for _, partValue := range parts {
			part, _ := partValue.(map[string]any)
			inlineData, _ := part["inlineData"].(map[string]any)
			mimeType, _ := inlineData["mimeType"].(string)
			if strings.HasPrefix(strings.ToLower(strings.TrimSpace(mimeType)), "image/") {
				count++
			}
		}
	}
	return count
}

func (s *AntigravityGatewayService) antigravityGeminiMaxLineSize() int {
	if s != nil && s.settingService != nil && s.settingService.cfg != nil && s.settingService.cfg.Gateway.MaxLineSize > 0 {
		return s.settingService.cfg.Gateway.MaxLineSize
	}
	return defaultMaxLineSize
}

func (s *AntigravityGatewayService) antigravityGeminiResponseLimit() int64 {
	if s != nil && s.settingService != nil {
		return resolveUpstreamResponseReadLimit(s.settingService.cfg)
	}
	return resolveUpstreamResponseReadLimit(nil)
}

func unwrapStrictAntigravityGeminiResponse(envelopeBody []byte) ([]byte, error) {
	var envelope map[string]json.RawMessage
	if err := decodeStrictJSONDocument(envelopeBody, &envelope); err != nil {
		return nil, err
	}
	response, ok := envelope["response"]
	if !ok || len(bytes.TrimSpace(response)) == 0 || bytes.Equal(bytes.TrimSpace(response), []byte("null")) {
		return nil, errors.New("Antigravity v1internal response is missing response")
	}
	var object map[string]json.RawMessage
	if err := decodeStrictJSONDocument(response, &object); err != nil {
		return nil, errors.New("Antigravity v1internal response must contain a JSON object")
	}
	return append([]byte(nil), response...), nil
}

func unwrapAntigravityGeminiResponseIfPresent(body []byte) ([]byte, error) {
	var envelope map[string]json.RawMessage
	if err := decodeStrictJSONDocument(body, &envelope); err != nil {
		return body, nil
	}
	if _, ok := envelope["response"]; !ok {
		return body, nil
	}
	return unwrapStrictAntigravityGeminiResponse(body)
}

func decodeStrictJSONObject(data []byte) (map[string]any, error) {
	var object map[string]any
	if err := decodeStrictJSONDocument(data, &object); err != nil {
		return nil, err
	}
	if object == nil {
		return nil, errors.New("JSON object is required")
	}
	return object, nil
}

func decodeStrictJSONDocument(data []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values are not allowed")
		}
		return err
	}
	return nil
}

type geminiResponseAccumulator struct {
	top            map[string]any
	candidates     map[int]*geminiCandidateAccumulator
	candidateOrder []int
	seen           bool
}

type geminiCandidateAccumulator struct {
	fields      map[string]any
	content     map[string]any
	parts       []any
	seenContent bool
	seenParts   bool
}

func newGeminiResponseAccumulator() *geminiResponseAccumulator {
	return &geminiResponseAccumulator{candidates: make(map[int]*geminiCandidateAccumulator)}
}

func (a *geminiResponseAccumulator) Add(responseBody []byte) error {
	response, err := decodeStrictJSONObject(responseBody)
	if err != nil {
		return err
	}
	a.seen = true
	if a.top == nil {
		a.top = make(map[string]any)
	}
	for key, value := range response {
		if key != "candidates" {
			a.top[key] = value
		}
	}
	candidateValues, exists := response["candidates"]
	if !exists {
		return nil
	}
	candidates, ok := candidateValues.([]any)
	if !ok {
		return errors.New("Gemini candidates must be an array")
	}
	for position, candidateValue := range candidates {
		candidate, ok := candidateValue.(map[string]any)
		if !ok {
			return errors.New("Gemini candidate must be an object")
		}
		index := position
		if rawIndex, exists := candidate["index"]; exists {
			parsedIndex, ok := strictJSONInt(rawIndex)
			if !ok || parsedIndex < 0 {
				return errors.New("Gemini candidate index must be a non-negative integer")
			}
			index = parsedIndex
		}
		accumulated, exists := a.candidates[index]
		if !exists {
			accumulated = &geminiCandidateAccumulator{fields: make(map[string]any), content: make(map[string]any)}
			a.candidates[index] = accumulated
			a.candidateOrder = append(a.candidateOrder, index)
		}
		for key, value := range candidate {
			switch key {
			case "content":
				content, ok := value.(map[string]any)
				if !ok {
					return errors.New("Gemini candidate content must be an object")
				}
				accumulated.seenContent = true
				for contentKey, contentValue := range content {
					if contentKey == "parts" {
						parts, ok := contentValue.([]any)
						if !ok {
							return errors.New("Gemini candidate content parts must be an array")
						}
						accumulated.seenParts = true
						accumulated.parts = append(accumulated.parts, parts...)
						continue
					}
					accumulated.content[contentKey] = contentValue
				}
			default:
				accumulated.fields[key] = value
			}
		}
	}
	return nil
}

func (a *geminiResponseAccumulator) Marshal() ([]byte, error) {
	if a == nil || !a.seen {
		return nil, errors.New("Antigravity Gemini upstream returned no response events")
	}
	result := make(map[string]any, len(a.top)+1)
	for key, value := range a.top {
		result[key] = value
	}
	if len(a.candidates) > 0 {
		candidates := make([]any, 0, len(a.candidateOrder))
		for _, index := range a.candidateOrder {
			accumulated := a.candidates[index]
			candidate := make(map[string]any, len(accumulated.fields)+1)
			for key, value := range accumulated.fields {
				candidate[key] = value
			}
			if accumulated.seenContent {
				content := make(map[string]any, len(accumulated.content)+1)
				for key, value := range accumulated.content {
					content[key] = value
				}
				if accumulated.seenParts {
					content["parts"] = accumulated.parts
				}
				candidate["content"] = content
			}
			candidates = append(candidates, candidate)
		}
		result["candidates"] = candidates
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("encode aggregated Gemini response: %w", err)
	}
	return encoded, nil
}

func strictJSONInt(value any) (int, bool) {
	switch typed := value.(type) {
	case json.Number:
		integer, err := typed.Int64()
		if err != nil || int64(int(integer)) != integer {
			return 0, false
		}
		return int(integer), true
	case int:
		return typed, true
	case int64:
		if int64(int(typed)) != typed {
			return 0, false
		}
		return int(typed), true
	default:
		return 0, false
	}
}
