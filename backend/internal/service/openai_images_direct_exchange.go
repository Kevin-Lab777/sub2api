package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"mime"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/gatewaytransport"
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
)

const (
	openAIImagesDirectCredentialUnavailableReason GatewayFailureReason = "openai_images_direct_credential_unavailable"
	openAIImagesDirectTransportUnavailableReason  GatewayFailureReason = "openai_images_direct_transport_unavailable"
)

// OpenAIImagesRuntimeRequest contains only the protocol fields needed for
// technical scheduling and transport selection. The original body remains the
// source of truth forwarded upstream.
type OpenAIImagesRuntimeRequest struct {
	Endpoint    string
	ContentType string
	Model       string
	Stream      bool
	N           int
	Multipart   bool
}

// ParseOpenAIImagesRuntimeRequest validates routing fields without applying
// legacy defaults, request repair, or customer image policy.
func ParseOpenAIImagesRuntimeRequest(req *http.Request, body []byte) (*OpenAIImagesRuntimeRequest, error) {
	if req == nil || req.URL == nil || req.Method != http.MethodPost ||
		(req.URL.Path != openAIImagesGenerationsEndpoint && req.URL.Path != openAIImagesEditsEndpoint) {
		return nil, errors.New("native OpenAI Images endpoint mismatch")
	}
	contentType := strings.TrimSpace(req.Header.Get("Content-Type"))
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, fmt.Errorf("parse OpenAI Images Content-Type: %w", err)
	}
	parsed := &OpenAIImagesRuntimeRequest{
		Endpoint:    req.URL.Path,
		ContentType: contentType,
		N:           1,
	}
	switch strings.ToLower(mediaType) {
	case "application/json":
		if err := parseNativeOpenAIImagesJSONRouting(body, parsed); err != nil {
			return nil, err
		}
	case "multipart/form-data":
		if req.URL.Path != openAIImagesEditsEndpoint {
			return nil, errors.New("OpenAI image generation requires application/json")
		}
		parsed.Multipart = true
		if err := parseNativeOpenAIImagesMultipartRouting(body, contentType, parsed); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("OpenAI Images request has unsupported Content-Type %q", contentType)
	}
	if parsed.Model == "" || parsed.Model != strings.TrimSpace(parsed.Model) {
		return nil, errors.New("OpenAI Images model must be an exact non-empty string")
	}
	if parsed.N <= 0 {
		return nil, errors.New("OpenAI Images n must be a positive integer")
	}
	return parsed, nil
}

func parseNativeOpenAIImagesJSONRouting(body []byte, parsed *OpenAIImagesRuntimeRequest) error {
	if len(body) == 0 || !json.Valid(body) {
		return errors.New("OpenAI Images request must be valid JSON")
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(body, &object); err != nil || object == nil {
		return errors.New("OpenAI Images request must be a JSON object")
	}
	if err := json.Unmarshal(object["model"], &parsed.Model); err != nil {
		return errors.New("OpenAI Images model must be a string")
	}
	if raw, ok := object["stream"]; ok {
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return errors.New("OpenAI Images stream must be a boolean")
		}
		if err := json.Unmarshal(raw, &parsed.Stream); err != nil {
			return errors.New("OpenAI Images stream must be a boolean")
		}
	}
	if raw, ok := object["n"]; ok {
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return errors.New("OpenAI Images n must be an integer")
		}
		if err := json.Unmarshal(raw, &parsed.N); err != nil {
			return errors.New("OpenAI Images n must be an integer")
		}
	}
	return nil
}

func parseNativeOpenAIImagesMultipartRouting(body []byte, contentType string, parsed *OpenAIImagesRuntimeRequest) error {
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return fmt.Errorf("parse OpenAI Images multipart Content-Type: %w", err)
	}
	boundary := strings.TrimSpace(params["boundary"])
	if boundary == "" {
		return errors.New("OpenAI Images multipart boundary is required")
	}
	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	modelFields := 0
	streamFields := 0
	nFields := 0
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read OpenAI Images multipart body: %w", err)
		}
		name := strings.TrimSpace(part.FormName())
		if part.FileName() != "" || (name != "model" && name != "stream" && name != "n") {
			_, copyErr := io.Copy(io.Discard, part)
			_ = part.Close()
			if copyErr != nil {
				return fmt.Errorf("read OpenAI Images multipart part %q: %w", name, copyErr)
			}
			continue
		}
		valueBytes, readErr := io.ReadAll(part)
		_ = part.Close()
		if readErr != nil {
			return fmt.Errorf("read OpenAI Images multipart field %q: %w", name, readErr)
		}
		value := string(valueBytes)
		switch name {
		case "model":
			modelFields++
			parsed.Model = value
		case "stream":
			streamFields++
			stream, parseErr := strconv.ParseBool(value)
			if parseErr != nil {
				return errors.New("OpenAI Images multipart stream must be a boolean")
			}
			parsed.Stream = stream
		case "n":
			nFields++
			n, parseErr := strconv.Atoi(value)
			if parseErr != nil {
				return errors.New("OpenAI Images multipart n must be an integer")
			}
			parsed.N = n
		}
	}
	if modelFields != 1 {
		return errors.New("OpenAI Images multipart request must contain exactly one model field")
	}
	if streamFields > 1 || nFields > 1 {
		return errors.New("OpenAI Images multipart routing fields must not be duplicated")
	}
	return nil
}

// ForwardImagesDirectExchange forwards the public Images API directly through
// one API-key account. It performs no Responses conversion or asset recovery.
func (s *OpenAIGatewayService) ForwardImagesDirectExchange(
	ctx context.Context,
	exchange gatewaytransport.Exchange,
	account *Account,
	body []byte,
) (*OpenAIForwardResult, error) {
	if exchange == nil || exchange.Request() == nil || exchange.Response() == nil {
		return nil, errors.New("OpenAI Images exchange is required")
	}
	if account == nil || account.Platform != PlatformOpenAI || account.Type != AccountTypeAPIKey {
		return nil, errors.New("native direct OpenAI Images requires an OpenAI API-key account")
	}
	parsed, err := ParseOpenAIImagesRuntimeRequest(exchange.Request(), body)
	if err != nil {
		return nil, err
	}
	upstreamModel, err := resolveExactOpenAIAccountModel(account, parsed.Model)
	if err != nil {
		return nil, err
	}
	upstreamBody, upstreamContentType, err := rewriteOpenAIImagesModel(body, parsed.ContentType, upstreamModel)
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
			openAIImagesDirectCredentialUnavailableReason,
			fmt.Errorf("get OpenAI Images credential: %w", err),
		)
	}
	upstreamReq, err := s.buildNativeOpenAIImagesDirectRequest(ctx, exchange, account, parsed, upstreamBody, upstreamContentType, token)
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
			openAIImagesDirectTransportUnavailableReason,
			fmt.Errorf("send OpenAI Images request: %w", err),
		)
	}
	if resp == nil || resp.Body == nil {
		return nil, nativeOpenAIAccountFailoverError(
			http.StatusBadGateway,
			GatewayFailureStageInference,
			openAIImagesDirectTransportUnavailableReason,
			errors.New("OpenAI Images upstream returned an empty HTTP response"),
		)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < http.StatusOK {
		return nil, fmt.Errorf("OpenAI Images upstream returned invalid final status %d", resp.StatusCode)
	}
	if resp.StatusCode >= http.StatusMultipleChoices {
		return nil, s.handleNativeOpenAIImagesDirectError(exchange, account, resp)
	}
	if parsed.Stream {
		return s.forwardNativeOpenAIImagesDirectStream(exchange, resp, parsed, upstreamModel, startedAt)
	}
	return s.forwardNativeOpenAIImagesDirectJSON(exchange, resp, parsed, upstreamModel, startedAt)
}

func (s *OpenAIGatewayService) buildNativeOpenAIImagesDirectRequest(
	ctx context.Context,
	exchange gatewaytransport.Exchange,
	account *Account,
	parsed *OpenAIImagesRuntimeRequest,
	body []byte,
	contentType string,
	token string,
) (*http.Request, error) {
	baseURL := strings.TrimSpace(account.GetOpenAIBaseURL())
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	validatedURL, err := s.validateUpstreamBaseURL(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid OpenAI Images base_url: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, buildOpenAIImagesURL(validatedURL, parsed.Endpoint), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build OpenAI Images request: %w", err)
	}
	req = req.WithContext(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAI))
	headers, err := s.buildOpenAIAuthenticationHeaders(ctx, account, token)
	if err != nil {
		return nil, fmt.Errorf("build OpenAI Images authentication headers: %w", err)
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
	req.Header.Set("Content-Type", contentType)
	if parsed.Stream {
		req.Header.Set("Accept", "text/event-stream")
	} else {
		req.Header.Set("Accept", "application/json")
	}
	return req, nil
}

func (s *OpenAIGatewayService) handleNativeOpenAIImagesDirectError(exchange gatewaytransport.Exchange, account *Account, resp *http.Response) error {
	body, err := readUpstreamResponseBodyExchange(resp.Body, s.cfg, exchange, nil)
	if err != nil {
		return nativeOpenAIAccountFailoverError(http.StatusBadGateway, GatewayFailureStageInference, openAIImagesDirectTransportUnavailableReason, err)
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
		fmt.Errorf("OpenAI Images upstream error: %d", resp.StatusCode),
		exchange.WriteData(resp.StatusCode, resp.Header.Get("Content-Type"), body),
	)
}

func (s *OpenAIGatewayService) forwardNativeOpenAIImagesDirectJSON(
	exchange gatewaytransport.Exchange,
	resp *http.Response,
	parsed *OpenAIImagesRuntimeRequest,
	upstreamModel string,
	startedAt time.Time,
) (*OpenAIForwardResult, error) {
	body, readErr := readUpstreamResponseBodyExchange(resp.Body, s.cfg, exchange, nil)
	if readErr != nil {
		return nil, readErr
	}
	responseheaders.WriteFilteredHeaders(exchange.Response().Header(), resp.Header, s.responseHeaderFilter)
	write := func() error { return exchange.WriteData(resp.StatusCode, resp.Header.Get("Content-Type"), body) }
	kind, mediaErr := classifyNativeOpenAIResponsesContentType(resp.Header.Get("Content-Type"))
	if mediaErr != nil || kind != nativeOpenAIResponsesContentJSON {
		if mediaErr == nil {
			mediaErr = errors.New("OpenAI Images non-streaming upstream response must be JSON")
		}
		return nil, errors.Join(mediaErr, write())
	}
	usage, imageCount, imageOutputSizes, validationErr := validateNativeOpenAIImagesJSONResponse(body, upstreamModel)
	if validationErr != nil {
		return nil, errors.Join(validationErr, write())
	}
	result := nativeOpenAIImagesDirectResult(resp, parsed, upstreamModel, usage, imageCount, imageOutputSizes, false, startedAt, nil)
	if imageCount != parsed.N {
		return result, errors.Join(
			fmt.Errorf("OpenAI Images upstream returned %d images for n=%d", imageCount, parsed.N),
			write(),
		)
	}
	return result, write()
}

type nativeOpenAIImagesUsageWire struct {
	InputTokens  *int `json:"input_tokens"`
	OutputTokens *int `json:"output_tokens"`
	TotalTokens  *int `json:"total_tokens"`
	InputDetails *struct {
		TextTokens  *int `json:"text_tokens"`
		ImageTokens *int `json:"image_tokens"`
	} `json:"input_tokens_details"`
}

func parseNativeOpenAIImagesUsage(raw json.RawMessage, required bool) (OpenAIUsage, error) {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		if required {
			return OpenAIUsage{}, errors.New("OpenAI Images response omitted exact usage")
		}
		return OpenAIUsage{}, nil
	}
	var wire nativeOpenAIImagesUsageWire
	if err := json.Unmarshal(raw, &wire); err != nil {
		return OpenAIUsage{}, fmt.Errorf("parse OpenAI Images usage: %w", err)
	}
	if wire.InputTokens == nil || wire.OutputTokens == nil || wire.TotalTokens == nil || wire.InputDetails == nil ||
		wire.InputDetails.TextTokens == nil || wire.InputDetails.ImageTokens == nil {
		return OpenAIUsage{}, errors.New("OpenAI Images usage omitted required token fields")
	}
	values := []*int{wire.InputTokens, wire.OutputTokens, wire.TotalTokens, wire.InputDetails.TextTokens, wire.InputDetails.ImageTokens}
	for _, value := range values {
		if *value < 0 {
			return OpenAIUsage{}, errors.New("OpenAI Images usage contains a negative token count")
		}
	}
	if *wire.InputDetails.TextTokens+*wire.InputDetails.ImageTokens != *wire.InputTokens {
		return OpenAIUsage{}, errors.New("OpenAI Images input token details do not equal input_tokens")
	}
	if *wire.InputTokens+*wire.OutputTokens != *wire.TotalTokens {
		return OpenAIUsage{}, errors.New("OpenAI Images total_tokens does not equal input_tokens plus output_tokens")
	}
	return OpenAIUsage{
		InputTokens:       *wire.InputTokens,
		ImageInputTokens:  *wire.InputDetails.ImageTokens,
		OutputTokens:      *wire.OutputTokens,
		ImageOutputTokens: *wire.OutputTokens,
	}, nil
}

func validateNativeOpenAIImagesJSONResponse(body []byte, upstreamModel string) (OpenAIUsage, int, []string, error) {
	var root struct {
		Data  []json.RawMessage `json:"data"`
		Usage json.RawMessage   `json:"usage"`
	}
	if err := json.Unmarshal(body, &root); err != nil {
		return OpenAIUsage{}, 0, nil, fmt.Errorf("parse OpenAI Images response: %w", err)
	}
	if len(root.Data) == 0 {
		return OpenAIUsage{}, 0, nil, errors.New("OpenAI Images response contained no image data")
	}
	imageOutputSizes := make([]string, 0, len(root.Data))
	for index, raw := range root.Data {
		var item struct {
			B64JSON string `json:"b64_json"`
			URL     string `json:"url"`
			Size    string `json:"size"`
		}
		if err := json.Unmarshal(raw, &item); err != nil || (strings.TrimSpace(item.B64JSON) == "" && strings.TrimSpace(item.URL) == "") {
			return OpenAIUsage{}, 0, nil, fmt.Errorf("OpenAI Images response data item %d omitted image content", index)
		}
		if item.Size != "" {
			if item.Size != strings.TrimSpace(item.Size) {
				return OpenAIUsage{}, 0, nil, fmt.Errorf("OpenAI Images response data item %d contains an inexact size", index)
			}
			imageOutputSizes = append(imageOutputSizes, item.Size)
		}
	}
	usage, err := parseNativeOpenAIImagesUsage(root.Usage, nativeOpenAIImagesRequiresUsage(upstreamModel))
	if err != nil {
		return OpenAIUsage{}, 0, nil, err
	}
	return usage, len(root.Data), imageOutputSizes, nil
}

func nativeOpenAIImagesRequiresUsage(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	return strings.HasPrefix(model, "gpt-image-") || model == "chatgpt-image-latest"
}

type nativeOpenAIImagesSSEEvent struct {
	raw  []byte
	name string
	data []byte
}

func readNativeOpenAIImagesSSEEvent(reader *bufio.Reader, maxLineSize int) (*nativeOpenAIImagesSSEEvent, error) {
	var raw bytes.Buffer
	dataLines := make([]string, 0, 1)
	eventName := ""
	for {
		line, err := reader.ReadSlice('\n')
		if errors.Is(err, bufio.ErrBufferFull) || len(line) > maxLineSize {
			return nil, errors.New("OpenAI Images SSE line exceeds configured limit")
		}
		if len(line) > 0 {
			_, _ = raw.Write(line)
			trimmed := strings.TrimRight(string(line), "\r\n")
			if trimmed == "" {
				return &nativeOpenAIImagesSSEEvent{raw: raw.Bytes(), name: eventName, data: []byte(strings.Join(dataLines, "\n"))}, nil
			}
			if strings.HasPrefix(trimmed, "event:") {
				eventName = strings.TrimSpace(strings.TrimPrefix(trimmed, "event:"))
			}
			if strings.HasPrefix(trimmed, "data:") {
				value := strings.TrimPrefix(trimmed, "data:")
				if strings.HasPrefix(value, " ") {
					value = value[1:]
				}
				dataLines = append(dataLines, value)
			}
		}
		if err == io.EOF {
			if raw.Len() == 0 {
				return nil, io.EOF
			}
			return &nativeOpenAIImagesSSEEvent{raw: raw.Bytes(), name: eventName, data: []byte(strings.Join(dataLines, "\n"))}, nil
		}
		if err != nil {
			return nil, err
		}
	}
}

type nativeOpenAIImagesStreamState struct {
	expectedPartialType   string
	expectedCompletedType string
	requiresUsage         bool
	usage                 OpenAIUsage
	completed             int
	imageOutputSizes      []string
	done                  bool
}

func (s *nativeOpenAIImagesStreamState) process(event *nativeOpenAIImagesSSEEvent) error {
	if event == nil || len(event.data) == 0 {
		return nil
	}
	if bytes.Equal(bytes.TrimSpace(event.data), []byte("[DONE]")) {
		if s.completed == 0 {
			return errors.New("OpenAI Images stream ended before a completed event")
		}
		s.done = true
		return nil
	}
	if s.done {
		return errors.New("OpenAI Images upstream emitted data after [DONE]")
	}
	if !json.Valid(event.data) {
		return errors.New("OpenAI Images upstream emitted non-JSON SSE data")
	}
	var envelope struct {
		Type    string          `json:"type"`
		B64JSON string          `json:"b64_json"`
		Size    string          `json:"size"`
		Usage   json.RawMessage `json:"usage"`
	}
	if err := json.Unmarshal(event.data, &envelope); err != nil || envelope.Type == "" {
		return errors.New("OpenAI Images upstream emitted an invalid SSE event")
	}
	if event.name != "" && event.name != envelope.Type {
		return fmt.Errorf("OpenAI Images SSE event name %q does not match payload type %q", event.name, envelope.Type)
	}
	switch envelope.Type {
	case s.expectedPartialType:
		if strings.TrimSpace(envelope.B64JSON) == "" {
			return errors.New("OpenAI Images partial event omitted b64_json")
		}
		return nil
	case s.expectedCompletedType:
		if strings.TrimSpace(envelope.B64JSON) == "" {
			return errors.New("OpenAI Images completed event omitted b64_json")
		}
		usage, err := parseNativeOpenAIImagesUsage(envelope.Usage, s.requiresUsage)
		if err != nil {
			return err
		}
		if err := addNativeOpenAIImagesUsage(&s.usage, usage); err != nil {
			return err
		}
		if envelope.Size != "" {
			if envelope.Size != strings.TrimSpace(envelope.Size) {
				return errors.New("OpenAI Images completed event contains an inexact size")
			}
			s.imageOutputSizes = append(s.imageOutputSizes, envelope.Size)
		}
		s.completed++
		return nil
	case "error":
		return errors.New("OpenAI Images upstream emitted an error event")
	default:
		return fmt.Errorf("OpenAI Images upstream emitted unsupported event %q", envelope.Type)
	}
}

func addNativeOpenAIImagesUsage(dst *OpenAIUsage, next OpenAIUsage) error {
	fields := []struct {
		name string
		dst  *int
		next int
	}{
		{name: "input_tokens", dst: &dst.InputTokens, next: next.InputTokens},
		{name: "image_input_tokens", dst: &dst.ImageInputTokens, next: next.ImageInputTokens},
		{name: "output_tokens", dst: &dst.OutputTokens, next: next.OutputTokens},
		{name: "image_output_tokens", dst: &dst.ImageOutputTokens, next: next.ImageOutputTokens},
	}
	for _, field := range fields {
		if field.next > math.MaxInt-*field.dst {
			return fmt.Errorf("OpenAI Images %s overflow", field.name)
		}
		*field.dst += field.next
	}
	return nil
}

func (s *OpenAIGatewayService) forwardNativeOpenAIImagesDirectStream(
	exchange gatewaytransport.Exchange,
	resp *http.Response,
	parsed *OpenAIImagesRuntimeRequest,
	upstreamModel string,
	startedAt time.Time,
) (*OpenAIForwardResult, error) {
	kind, err := classifyNativeOpenAIResponsesContentType(resp.Header.Get("Content-Type"))
	if err != nil || kind != nativeOpenAIResponsesContentSSE {
		if err == nil {
			err = errors.New("OpenAI Images streaming upstream response must be SSE")
		}
		return nil, err
	}
	writer := exchange.Response()
	if !writer.SupportsFlush() {
		return nil, errors.New("OpenAI Images streaming requires flush support")
	}
	prefix := "image_generation"
	if parsed.Endpoint == openAIImagesEditsEndpoint {
		prefix = "image_edit"
	}
	state := &nativeOpenAIImagesStreamState{
		expectedPartialType:   prefix + ".partial_image",
		expectedCompletedType: prefix + ".completed",
		requiresUsage:         nativeOpenAIImagesRequiresUsage(upstreamModel),
	}
	maxLineSize := defaultMaxLineSize
	if s.cfg != nil && s.cfg.Gateway.MaxLineSize > 0 {
		maxLineSize = s.cfg.Gateway.MaxLineSize
	}
	reader := bufio.NewReaderSize(resp.Body, maxLineSize+1)
	var firstTokenMs *int
	resultWithObservedUsage := func() *OpenAIForwardResult {
		if state.completed == 0 {
			return nil
		}
		return nativeOpenAIImagesDirectResult(resp, parsed, upstreamModel, state.usage, state.completed, state.imageOutputSizes, true, startedAt, firstTokenMs)
	}
	for {
		event, readErr := readNativeOpenAIImagesSSEEvent(reader, maxLineSize)
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return resultWithObservedUsage(), readErr
		}
		if !writer.Written() {
			responseheaders.WriteFilteredHeaders(writer.Header(), resp.Header, s.responseHeaderFilter)
			writer.Header().Set("Content-Type", "text/event-stream")
			writer.Header().Set("Cache-Control", "no-cache")
			writer.Header().Set("X-Accel-Buffering", "no")
			writer.WriteHeader(resp.StatusCode)
		}
		if firstTokenMs == nil {
			elapsed := int(time.Since(startedAt).Milliseconds())
			firstTokenMs = &elapsed
		}
		processErr := state.process(event)
		if _, writeErr := writer.Write(event.raw); writeErr != nil {
			return resultWithObservedUsage(), writeErr
		}
		if flushErr := writer.Flush(); flushErr != nil {
			return resultWithObservedUsage(), flushErr
		}
		if processErr != nil {
			return resultWithObservedUsage(), processErr
		}
	}
	if state.completed == 0 {
		return nil, errors.New("OpenAI Images stream ended without a completed image")
	}
	result := resultWithObservedUsage()
	if state.completed != parsed.N {
		return result, fmt.Errorf("OpenAI Images upstream completed %d images for n=%d", state.completed, parsed.N)
	}
	return result, nil
}

func nativeOpenAIImagesDirectResult(
	resp *http.Response,
	parsed *OpenAIImagesRuntimeRequest,
	upstreamModel string,
	usage OpenAIUsage,
	imageCount int,
	imageOutputSizes []string,
	stream bool,
	startedAt time.Time,
	firstTokenMs *int,
) *OpenAIForwardResult {
	return &OpenAIForwardResult{
		RequestID:        firstNonEmptyString(resp.Header.Get("X-Request-Id"), resp.Header.Get("Request-Id")),
		Usage:            usage,
		Model:            parsed.Model,
		BillingModel:     upstreamModel,
		UpstreamModel:    upstreamModel,
		UpstreamEndpoint: parsed.Endpoint,
		Stream:           stream,
		Duration:         time.Since(startedAt),
		FirstTokenMs:     firstTokenMs,
		ImageCount:       imageCount,
		ImageOutputSizes: append([]string(nil), imageOutputSizes...),
	}
}
