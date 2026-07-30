package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/gatewaytransport"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
)

var errNativeOpenAIChatTerminal = errors.New("native OpenAI Chat terminal event")

type nativeOpenAIChatSemanticError struct {
	eventType string
}

func (e *nativeOpenAIChatSemanticError) Error() string {
	return fmt.Sprintf("OpenAI subscription upstream terminated with %s", e.eventType)
}

type nativeOpenAIChatSubscriptionRequest struct {
	originalModel      string
	upstreamModel      string
	clientStream       bool
	clientIncludeUsage bool
	responsesBody      []byte
}

// ValidateOpenAIChatCompletionsSubscriptionRequest reports whether a Chat
// request can be represented by the subscription account's Responses
// transport without dropping or inventing fields.
func ValidateOpenAIChatCompletionsSubscriptionRequest(body []byte) error {
	_, _, err := parseStrictNativeOpenAIChatRequest(body)
	return err
}

func (s *OpenAIGatewayService) forwardChatCompletionsSubscriptionExchange(
	ctx context.Context,
	exchange gatewaytransport.Exchange,
	account *Account,
	body []byte,
) (*OpenAIForwardResult, error) {
	if exchange == nil || exchange.Request() == nil || exchange.Response() == nil || exchange.Request().URL == nil ||
		exchange.Request().Method != http.MethodPost || exchange.Request().URL.Path != nativeOpenAIChatCompletionsEndpoint {
		return nil, errors.New("native OpenAI subscription Chat Completions endpoint mismatch")
	}
	if account == nil || account.Platform != PlatformOpenAI || account.Type != AccountTypeOAuth {
		return nil, errors.New("OpenAI Chat Completions subscription adapter requires an OAuth account")
	}
	request, err := buildNativeOpenAIChatSubscriptionRequest(account, body)
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
			fmt.Errorf("get OpenAI subscription credential: %w", err),
		)
	}
	upstreamReq, err := s.buildNativeOpenAIResponsesRequest(ctx, exchange, account, request.responsesBody, token, true, false)
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
			fmt.Errorf("send OpenAI subscription Chat Completions request: %w", err),
		)
	}
	if resp == nil || resp.Body == nil {
		return nil, nativeOpenAIAccountFailoverError(
			http.StatusBadGateway,
			GatewayFailureStageInference,
			openAIChatTransportUnavailableReason,
			errors.New("OpenAI subscription Chat Completions upstream returned an empty HTTP response"),
		)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < http.StatusOK {
		return nil, fmt.Errorf("OpenAI subscription upstream returned invalid final status %d", resp.StatusCode)
	}
	if resp.StatusCode >= http.StatusMultipleChoices {
		return nil, s.handleNativeOpenAIChatCompletionsError(exchange, account, request.upstreamModel, resp)
	}
	contentKind, err := classifyNativeOpenAIResponsesContentType(resp.Header.Get("Content-Type"))
	if err != nil || contentKind != nativeOpenAIResponsesContentSSE {
		if err == nil {
			err = errors.New("OpenAI subscription Chat Completions adapter requires an SSE Responses upstream")
		}
		return nil, nativeOpenAIAccountFailoverError(http.StatusBadGateway, GatewayFailureStageInference, openAIChatTransportUnavailableReason, err)
	}
	if request.clientStream {
		return s.forwardNativeOpenAIChatSubscriptionStream(exchange, resp, request, startedAt)
	}
	return s.forwardNativeOpenAIChatSubscriptionJSON(exchange, resp, request, startedAt)
}

func buildNativeOpenAIChatSubscriptionRequest(account *Account, body []byte) (*nativeOpenAIChatSubscriptionRequest, error) {
	originalModel, clientStream, err := ParseOpenAIChatCompletionsRequest(body)
	if err != nil {
		return nil, err
	}
	upstreamModel, err := resolveExactOpenAIAccountModel(account, originalModel)
	if err != nil {
		return nil, err
	}
	chatRequest, includeUsage, err := parseStrictNativeOpenAIChatRequest(body)
	if err != nil {
		return nil, err
	}
	chatRequest.Model = upstreamModel
	responsesRequest, err := apicompat.ChatCompletionsToResponses(chatRequest)
	if err != nil {
		return nil, fmt.Errorf("convert strict OpenAI Chat Completions request to Responses: %w", err)
	}
	responsesRequest.Stream = true
	// Chat Completions has no encrypted-reasoning field. Do not ask the
	// Responses upstream to return state the adapter cannot represent.
	responsesRequest.Include = nil
	responsesBody, err := json.Marshal(responsesRequest)
	if err != nil {
		return nil, fmt.Errorf("marshal OpenAI subscription Responses request: %w", err)
	}
	return &nativeOpenAIChatSubscriptionRequest{
		originalModel:      originalModel,
		upstreamModel:      upstreamModel,
		clientStream:       clientStream,
		clientIncludeUsage: includeUsage,
		responsesBody:      responsesBody,
	}, nil
}

func parseStrictNativeOpenAIChatRequest(body []byte) (*apicompat.ChatCompletionsRequest, bool, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(body, &root); err != nil || root == nil {
		return nil, false, errors.New("OpenAI Chat Completions request must be a JSON object")
	}
	allowed := map[string]struct{}{
		"model": {}, "messages": {}, "max_tokens": {}, "max_completion_tokens": {},
		"stream": {}, "stream_options": {}, "tools": {}, "parallel_tool_calls": {},
		"tool_choice": {}, "reasoning_effort": {}, "service_tier": {},
	}
	if err := rejectUnknownJSONObjectFields(root, allowed, "OpenAI subscription Chat Completions request"); err != nil {
		return nil, false, err
	}
	messagesRaw, ok := root["messages"]
	if !ok {
		return nil, false, errors.New("OpenAI subscription Chat Completions messages are required")
	}
	var messages []json.RawMessage
	if err := json.Unmarshal(messagesRaw, &messages); err != nil || len(messages) == 0 {
		return nil, false, errors.New("OpenAI subscription Chat Completions messages must be a non-empty array")
	}
	for index, raw := range messages {
		if err := validateStrictNativeOpenAIChatMessage(raw, index); err != nil {
			return nil, false, err
		}
	}
	if _, hasMax := root["max_tokens"]; hasMax {
		if _, hasCompletionMax := root["max_completion_tokens"]; hasCompletionMax {
			return nil, false, errors.New("OpenAI subscription Chat Completions request cannot set both max_tokens and max_completion_tokens")
		}
	}
	for _, field := range []string{"max_tokens", "max_completion_tokens"} {
		if raw, ok := root[field]; ok {
			var limit int
			if err := json.Unmarshal(raw, &limit); err != nil || limit < 128 {
				return nil, false, fmt.Errorf("OpenAI subscription Chat Completions %s must be an integer of at least 128", field)
			}
		}
	}
	if raw, ok := root["parallel_tool_calls"]; ok {
		var value any
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil, false, errors.New("OpenAI subscription Chat Completions parallel_tool_calls must be a boolean")
		}
		if _, ok := value.(bool); !ok {
			return nil, false, errors.New("OpenAI subscription Chat Completions parallel_tool_calls must be a boolean")
		}
	}
	if raw, ok := root["reasoning_effort"]; ok {
		var effort string
		if err := json.Unmarshal(raw, &effort); err != nil ||
			(effort != "low" && effort != "medium" && effort != "high" && effort != "xhigh") {
			return nil, false, errors.New("OpenAI subscription Chat Completions reasoning_effort must be low, medium, high, or xhigh")
		}
	}
	if raw, ok := root["service_tier"]; ok {
		var tier string
		if err := json.Unmarshal(raw, &tier); err != nil || tier == "" || tier != strings.TrimSpace(tier) {
			return nil, false, errors.New("OpenAI subscription Chat Completions service_tier must be an exact non-empty string")
		}
	}
	if raw, ok := root["tools"]; ok {
		if err := validateStrictNativeOpenAIChatTools(raw); err != nil {
			return nil, false, err
		}
	}
	if raw, ok := root["tool_choice"]; ok {
		var choice string
		if err := json.Unmarshal(raw, &choice); err != nil || (choice != "none" && choice != "auto" && choice != "required") {
			return nil, false, errors.New("OpenAI subscription Chat Completions tool_choice must be none, auto, or required")
		}
	}
	includeUsage := false
	stream := false
	if raw, ok := root["stream"]; ok {
		var value any
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil, false, errors.New("OpenAI subscription Chat Completions stream must be a boolean")
		}
		var ok bool
		stream, ok = value.(bool)
		if !ok {
			return nil, false, errors.New("OpenAI subscription Chat Completions stream must be a boolean")
		}
	}
	if raw, ok := root["stream_options"]; ok {
		if !stream {
			return nil, false, errors.New("OpenAI subscription Chat Completions stream_options requires stream=true")
		}
		var options map[string]json.RawMessage
		if err := json.Unmarshal(raw, &options); err != nil || options == nil {
			return nil, false, errors.New("OpenAI subscription Chat Completions stream_options must be an object")
		}
		if err := rejectUnknownJSONObjectFields(options, map[string]struct{}{"include_usage": {}}, "OpenAI subscription Chat Completions stream_options"); err != nil {
			return nil, false, err
		}
		if includeRaw, ok := options["include_usage"]; ok {
			var value any
			if err := json.Unmarshal(includeRaw, &value); err != nil {
				return nil, false, errors.New("OpenAI subscription Chat Completions include_usage must be a boolean")
			}
			var ok bool
			includeUsage, ok = value.(bool)
			if !ok {
				return nil, false, errors.New("OpenAI subscription Chat Completions include_usage must be a boolean")
			}
		}
	}
	var request apicompat.ChatCompletionsRequest
	if err := json.Unmarshal(body, &request); err != nil {
		return nil, false, fmt.Errorf("parse strict OpenAI Chat Completions request: %w", err)
	}
	return &request, includeUsage, nil
}

func rejectUnknownJSONObjectFields(object map[string]json.RawMessage, allowed map[string]struct{}, label string) error {
	unknown := make([]string, 0)
	for field := range object {
		if _, ok := allowed[field]; !ok {
			unknown = append(unknown, field)
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	sort.Strings(unknown)
	return fmt.Errorf("%s contains unsupported fields: %s", label, strings.Join(unknown, ", "))
}

func validateStrictNativeOpenAIChatMessage(raw json.RawMessage, index int) error {
	var message map[string]json.RawMessage
	if err := json.Unmarshal(raw, &message); err != nil || message == nil {
		return fmt.Errorf("OpenAI subscription Chat Completions message %d must be an object", index)
	}
	var role string
	if err := json.Unmarshal(message["role"], &role); err != nil || role == "" {
		return fmt.Errorf("OpenAI subscription Chat Completions message %d requires a string role", index)
	}
	allowed := map[string]struct{}{"role": {}, "content": {}}
	switch role {
	case "system":
		if err := validateStrictNativeOpenAIChatStringContent(message["content"], false); err != nil {
			return fmt.Errorf("OpenAI subscription system message %d: %w", index, err)
		}
	case "user":
		if err := validateStrictNativeOpenAIChatUserContent(message["content"]); err != nil {
			return fmt.Errorf("OpenAI subscription user message %d: %w", index, err)
		}
	case "assistant":
		allowed["tool_calls"] = struct{}{}
		contentPresent := false
		if rawContent, ok := message["content"]; ok && strings.TrimSpace(string(rawContent)) != "null" {
			if err := validateStrictNativeOpenAIChatStringContent(rawContent, false); err != nil {
				return fmt.Errorf("OpenAI subscription assistant message %d: %w", index, err)
			}
			var content string
			_ = json.Unmarshal(rawContent, &content)
			contentPresent = content != ""
		}
		toolCallsPresent := false
		if callsRaw, ok := message["tool_calls"]; ok {
			var calls []json.RawMessage
			if err := json.Unmarshal(callsRaw, &calls); err != nil || len(calls) == 0 {
				return fmt.Errorf("OpenAI subscription assistant message %d tool_calls must be a non-empty array", index)
			}
			for callIndex, call := range calls {
				if err := validateStrictNativeOpenAIChatToolCall(call, index, callIndex); err != nil {
					return err
				}
			}
			toolCallsPresent = true
		}
		if !contentPresent && !toolCallsPresent {
			return fmt.Errorf("OpenAI subscription assistant message %d has no representable content", index)
		}
	case "tool":
		allowed["tool_call_id"] = struct{}{}
		if err := validateStrictNativeOpenAIChatStringContent(message["content"], false); err != nil {
			return fmt.Errorf("OpenAI subscription tool message %d: %w", index, err)
		}
		var callID string
		if err := json.Unmarshal(message["tool_call_id"], &callID); err != nil || callID == "" || callID != strings.TrimSpace(callID) {
			return fmt.Errorf("OpenAI subscription tool message %d requires an exact tool_call_id", index)
		}
	default:
		return fmt.Errorf("OpenAI subscription Chat Completions message %d has unsupported role %q", index, role)
	}
	return rejectUnknownJSONObjectFields(message, allowed, fmt.Sprintf("OpenAI subscription Chat Completions message %d", index))
}

func validateStrictNativeOpenAIChatStringContent(raw json.RawMessage, allowEmpty bool) error {
	if len(raw) == 0 {
		return errors.New("content is required")
	}
	var content string
	if err := json.Unmarshal(raw, &content); err != nil {
		return errors.New("content must be a string")
	}
	if !allowEmpty && content == "" {
		return errors.New("content must not be empty")
	}
	return nil
}

func validateStrictNativeOpenAIChatUserContent(raw json.RawMessage) error {
	if err := validateStrictNativeOpenAIChatStringContent(raw, false); err == nil {
		return nil
	}
	var parts []json.RawMessage
	if err := json.Unmarshal(raw, &parts); err != nil || len(parts) == 0 {
		return errors.New("content must be a non-empty string or content-part array")
	}
	for index, rawPart := range parts {
		var part map[string]json.RawMessage
		if err := json.Unmarshal(rawPart, &part); err != nil || part == nil {
			return fmt.Errorf("content part %d must be an object", index)
		}
		var partType string
		if err := json.Unmarshal(part["type"], &partType); err != nil {
			return fmt.Errorf("content part %d requires a string type", index)
		}
		switch partType {
		case "text":
			if err := rejectUnknownJSONObjectFields(part, map[string]struct{}{"type": {}, "text": {}}, fmt.Sprintf("content part %d", index)); err != nil {
				return err
			}
			if err := validateStrictNativeOpenAIChatStringContent(part["text"], false); err != nil {
				return fmt.Errorf("content part %d text: %w", index, err)
			}
		case "image_url":
			if err := rejectUnknownJSONObjectFields(part, map[string]struct{}{"type": {}, "image_url": {}}, fmt.Sprintf("content part %d", index)); err != nil {
				return err
			}
			var image map[string]json.RawMessage
			if err := json.Unmarshal(part["image_url"], &image); err != nil || image == nil {
				return fmt.Errorf("content part %d image_url must be an object", index)
			}
			if err := rejectUnknownJSONObjectFields(image, map[string]struct{}{"url": {}, "detail": {}}, fmt.Sprintf("content part %d image_url", index)); err != nil {
				return err
			}
			var url string
			if err := json.Unmarshal(image["url"], &url); err != nil || url == "" {
				return fmt.Errorf("content part %d image_url requires url", index)
			}
			if nativeOpenAIChatEmptyBase64DataURI(url) {
				return fmt.Errorf("content part %d image_url contains empty base64 data", index)
			}
			if detailRaw, ok := image["detail"]; ok {
				var detail string
				if err := json.Unmarshal(detailRaw, &detail); err != nil || (detail != "auto" && detail != "low" && detail != "high") {
					return fmt.Errorf("content part %d image detail must be auto, low, or high", index)
				}
			}
		default:
			return fmt.Errorf("content part %d has unsupported type %q", index, partType)
		}
	}
	return nil
}

func nativeOpenAIChatEmptyBase64DataURI(value string) bool {
	if !strings.HasPrefix(value, "data:") {
		return false
	}
	comma := strings.Index(value, ",")
	if comma < 0 || !strings.Contains(strings.ToLower(value[:comma]), ";base64") {
		return false
	}
	return strings.TrimSpace(value[comma+1:]) == ""
}

func validateStrictNativeOpenAIChatToolCall(raw json.RawMessage, messageIndex, callIndex int) error {
	var call map[string]json.RawMessage
	if err := json.Unmarshal(raw, &call); err != nil || call == nil {
		return fmt.Errorf("OpenAI subscription assistant message %d tool call %d must be an object", messageIndex, callIndex)
	}
	if err := rejectUnknownJSONObjectFields(call, map[string]struct{}{"id": {}, "type": {}, "function": {}}, fmt.Sprintf("tool call %d", callIndex)); err != nil {
		return err
	}
	var id, callType string
	_ = json.Unmarshal(call["id"], &id)
	_ = json.Unmarshal(call["type"], &callType)
	if id == "" || callType != "function" {
		return fmt.Errorf("OpenAI subscription assistant message %d tool call %d requires id and type=function", messageIndex, callIndex)
	}
	var function map[string]json.RawMessage
	if err := json.Unmarshal(call["function"], &function); err != nil || function == nil {
		return fmt.Errorf("OpenAI subscription assistant message %d tool call %d requires function", messageIndex, callIndex)
	}
	if err := rejectUnknownJSONObjectFields(function, map[string]struct{}{"name": {}, "arguments": {}}, fmt.Sprintf("tool call %d function", callIndex)); err != nil {
		return err
	}
	var name, arguments string
	_ = json.Unmarshal(function["name"], &name)
	_ = json.Unmarshal(function["arguments"], &arguments)
	if name == "" || arguments == "" {
		return fmt.Errorf("OpenAI subscription assistant message %d tool call %d requires function name and arguments", messageIndex, callIndex)
	}
	return nil
}

func validateStrictNativeOpenAIChatTools(raw json.RawMessage) error {
	var tools []json.RawMessage
	if err := json.Unmarshal(raw, &tools); err != nil || len(tools) == 0 {
		return errors.New("OpenAI subscription Chat Completions tools must be a non-empty array")
	}
	for index, rawTool := range tools {
		var tool map[string]json.RawMessage
		if err := json.Unmarshal(rawTool, &tool); err != nil || tool == nil {
			return fmt.Errorf("OpenAI subscription Chat Completions tool %d must be an object", index)
		}
		if err := rejectUnknownJSONObjectFields(tool, map[string]struct{}{"type": {}, "function": {}}, fmt.Sprintf("tool %d", index)); err != nil {
			return err
		}
		var toolType string
		_ = json.Unmarshal(tool["type"], &toolType)
		if toolType != "function" {
			return fmt.Errorf("OpenAI subscription Chat Completions tool %d must have type=function", index)
		}
		var function map[string]json.RawMessage
		if err := json.Unmarshal(tool["function"], &function); err != nil || function == nil {
			return fmt.Errorf("OpenAI subscription Chat Completions tool %d requires function", index)
		}
		if err := rejectUnknownJSONObjectFields(function, map[string]struct{}{"name": {}, "description": {}, "parameters": {}, "strict": {}}, fmt.Sprintf("tool %d function", index)); err != nil {
			return err
		}
		var name string
		_ = json.Unmarshal(function["name"], &name)
		if name == "" {
			return fmt.Errorf("OpenAI subscription Chat Completions tool %d requires function name", index)
		}
		var parameters map[string]json.RawMessage
		if err := json.Unmarshal(function["parameters"], &parameters); err != nil || parameters == nil {
			return fmt.Errorf("OpenAI subscription Chat Completions tool %d parameters must be an object", index)
		}
		if descriptionRaw, ok := function["description"]; ok {
			var description any
			if err := json.Unmarshal(descriptionRaw, &description); err != nil {
				return fmt.Errorf("OpenAI subscription Chat Completions tool %d description must be a string", index)
			}
			if _, ok := description.(string); !ok {
				return fmt.Errorf("OpenAI subscription Chat Completions tool %d description must be a string", index)
			}
		}
		if strictRaw, ok := function["strict"]; ok {
			var strict any
			if err := json.Unmarshal(strictRaw, &strict); err != nil {
				return fmt.Errorf("OpenAI subscription Chat Completions tool %d strict must be a boolean", index)
			}
			if _, ok := strict.(bool); !ok {
				return fmt.Errorf("OpenAI subscription Chat Completions tool %d strict must be a boolean", index)
			}
		}
	}
	return nil
}

func (s *OpenAIGatewayService) forwardNativeOpenAIChatSubscriptionJSON(
	exchange gatewaytransport.Exchange,
	resp *http.Response,
	request *nativeOpenAIChatSubscriptionRequest,
	startedAt time.Time,
) (*OpenAIForwardResult, error) {
	body, err := readUpstreamResponseBodyExchange(resp.Body, s.cfg, exchange, nil)
	if err != nil {
		return nil, nativeOpenAIAccountFailoverError(http.StatusBadGateway, GatewayFailureStageInference, openAIChatTransportUnavailableReason, err)
	}
	terminal, usage, found, err := collectNativeOpenAIChatSubscriptionTerminal(body)
	if err != nil || !found {
		var semanticErr *nativeOpenAIChatSemanticError
		if errors.As(err, &semanticErr) {
			return nil, err
		}
		if err == nil {
			err = errors.New("OpenAI subscription Chat Completions upstream omitted terminal usage")
		}
		return nil, nativeOpenAIAccountFailoverError(http.StatusBadGateway, GatewayFailureStageInference, openAIChatTransportUnavailableReason, err)
	}
	chatResponse, _, webSearchCalls, err := convertStrictNativeOpenAIResponseToChat(terminal, request.originalModel, request.upstreamModel, usage)
	if err != nil {
		return nil, nativeOpenAIAccountFailoverError(http.StatusBadGateway, GatewayFailureStageInference, openAIChatTransportUnavailableReason, err)
	}
	responseheaders.WriteFilteredHeaders(exchange.Response().Header(), resp.Header, s.responseHeaderFilter)
	if err := exchange.WriteData(resp.StatusCode, "application/json", chatResponse); err != nil {
		return nil, err
	}
	return &OpenAIForwardResult{
		RequestID:        resp.Header.Get("X-Request-Id"),
		Usage:            usage,
		Model:            request.originalModel,
		UpstreamModel:    request.upstreamModel,
		UpstreamEndpoint: nativeOpenAIResponsesEndpoint,
		Duration:         time.Since(startedAt),
		WebSearchCalls:   webSearchCalls,
	}, nil
}

func collectNativeOpenAIChatSubscriptionTerminal(stream []byte) ([]byte, OpenAIUsage, bool, error) {
	var terminal []byte
	var usage OpenAIUsage
	found := false
	err := scanNativeOpenAIResponsesSSE(bytes.NewReader(stream), defaultMaxLineSize, nil, func(data []byte) error {
		if !json.Valid(data) {
			return errors.New("OpenAI subscription upstream emitted non-JSON SSE data")
		}
		var envelope struct {
			Type     string          `json:"type"`
			Response json.RawMessage `json:"response"`
		}
		if err := json.Unmarshal(data, &envelope); err != nil || envelope.Type == "" {
			return errors.New("OpenAI subscription upstream emitted an invalid Responses event")
		}
		if terminal != nil {
			return errors.New("OpenAI subscription upstream emitted data after a terminal response")
		}
		switch envelope.Type {
		case "response.failed", "error":
			return &nativeOpenAIChatSemanticError{eventType: envelope.Type}
		case "response.completed", "response.done", "response.incomplete":
			if terminal != nil {
				return errors.New("OpenAI subscription upstream emitted multiple terminal responses")
			}
			if len(envelope.Response) == 0 || string(envelope.Response) == "null" || !json.Valid(envelope.Response) {
				return errors.New("OpenAI subscription terminal event omitted a valid response object")
			}
			var response apicompat.ResponsesResponse
			if err := json.Unmarshal(envelope.Response, &response); err != nil {
				return errors.New("OpenAI subscription terminal event contained an invalid response object")
			}
			if err := validateStrictNativeOpenAIStreamingTerminal(envelope.Type, &response); err != nil {
				return err
			}
			parsed, usageFound, err := extractNativeOpenAIResponsesUsage(data)
			if err != nil {
				return err
			}
			if !usageFound {
				return errors.New("OpenAI subscription terminal event omitted exact usage")
			}
			terminal = append([]byte(nil), envelope.Response...)
			usage = parsed
			found = true
		}
		return nil
	})
	if err != nil {
		return nil, OpenAIUsage{}, false, err
	}
	return terminal, usage, found, nil
}

func convertStrictNativeOpenAIResponseToChat(body []byte, originalModel, upstreamModel string, usage OpenAIUsage) ([]byte, int, int, error) {
	createdAt, response, imageCount, webSearchCalls, err := validateStrictNativeOpenAIResponseForChat(body, upstreamModel)
	if err != nil {
		return nil, 0, 0, err
	}
	chatResponse := apicompat.ResponsesToChatCompletions(response, originalModel)
	chatResponse.ID = response.ID
	chatResponse.Created = createdAt
	chatResponse.Model = originalModel
	chatResponse.Usage = &apicompat.ChatUsage{
		PromptTokens:     usage.InputTokens,
		CompletionTokens: usage.OutputTokens,
		TotalTokens:      usage.InputTokens + usage.OutputTokens,
	}
	if usage.CacheReadInputTokens > 0 || usage.CacheCreationInputTokens > 0 {
		chatResponse.Usage.PromptTokensDetails = &apicompat.ChatTokenDetails{
			CachedTokens:        usage.CacheReadInputTokens,
			CacheCreationTokens: usage.CacheCreationInputTokens,
		}
	}
	encoded, err := json.Marshal(chatResponse)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("marshal strict Chat Completions response: %w", err)
	}
	return encoded, imageCount, webSearchCalls, nil
}

func validateStrictNativeOpenAIResponseForChat(body []byte, upstreamModel string) (int64, *apicompat.ResponsesResponse, int, int, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(body, &root); err != nil || root == nil {
		return 0, nil, 0, 0, errors.New("OpenAI subscription terminal response must be a JSON object")
	}
	var createdAt int64
	if err := json.Unmarshal(root["created_at"], &createdAt); err != nil || createdAt <= 0 {
		return 0, nil, 0, 0, errors.New("OpenAI subscription terminal response requires a positive created_at")
	}
	if err := validateStrictNativeOpenAIResponseWire(body); err != nil {
		return 0, nil, 0, 0, err
	}
	var response apicompat.ResponsesResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return 0, nil, 0, 0, fmt.Errorf("parse OpenAI subscription terminal response: %w", err)
	}
	if response.ID == "" || response.Model == "" {
		return 0, nil, 0, 0, errors.New("OpenAI subscription terminal response requires id and model")
	}
	if response.Model != upstreamModel {
		return 0, nil, 0, 0, fmt.Errorf("OpenAI subscription terminal response model mismatch: expected %q, got %q", upstreamModel, response.Model)
	}
	switch response.Status {
	case "completed":
	case "incomplete":
		if response.IncompleteDetails == nil || (response.IncompleteDetails.Reason != "max_output_tokens" && response.IncompleteDetails.Reason != "content_filter") {
			return 0, nil, 0, 0, errors.New("OpenAI subscription incomplete response has an unsupported reason")
		}
	default:
		return 0, nil, 0, 0, fmt.Errorf("OpenAI subscription response has unsupported status %q", response.Status)
	}
	imageCount, webSearchCalls, err := validateStrictNativeOpenAIResponseOutputs(&response)
	if err != nil {
		return 0, nil, 0, 0, err
	}
	return createdAt, &response, imageCount, webSearchCalls, nil
}

func validateStrictNativeOpenAIResponseWire(body []byte) error {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(body, &root); err != nil || root == nil {
		return errors.New("OpenAI subscription response must be a JSON object")
	}
	var outputs []json.RawMessage
	if err := json.Unmarshal(root["output"], &outputs); err != nil {
		return errors.New("OpenAI subscription response output must be an array")
	}
	for outputIndex, rawOutput := range outputs {
		var output map[string]json.RawMessage
		if err := json.Unmarshal(rawOutput, &output); err != nil || output == nil {
			return fmt.Errorf("OpenAI subscription output %d must be an object", outputIndex)
		}
		var outputType string
		_ = json.Unmarshal(output["type"], &outputType)
		switch outputType {
		case "message":
			if err := rejectUnknownJSONObjectFields(output, map[string]struct{}{
				"id": {}, "type": {}, "status": {}, "role": {}, "content": {},
			}, fmt.Sprintf("OpenAI subscription output %d", outputIndex)); err != nil {
				return err
			}
			var content []json.RawMessage
			if err := json.Unmarshal(output["content"], &content); err != nil {
				return fmt.Errorf("OpenAI subscription output %d content must be an array", outputIndex)
			}
			for partIndex, rawPart := range content {
				var part map[string]json.RawMessage
				if err := json.Unmarshal(rawPart, &part); err != nil || part == nil {
					return fmt.Errorf("OpenAI subscription output %d content %d must be an object", outputIndex, partIndex)
				}
				if err := rejectUnknownJSONObjectFields(part, map[string]struct{}{"type": {}, "text": {}}, fmt.Sprintf("OpenAI subscription output %d content %d", outputIndex, partIndex)); err != nil {
					return err
				}
			}
		case "reasoning":
			if err := rejectUnknownJSONObjectFields(output, map[string]struct{}{
				"id": {}, "type": {}, "status": {}, "summary": {}, "encrypted_content": {},
			}, fmt.Sprintf("OpenAI subscription output %d", outputIndex)); err != nil {
				return err
			}
			if encrypted, ok := output["encrypted_content"]; ok && strings.TrimSpace(string(encrypted)) != `""` && strings.TrimSpace(string(encrypted)) != "null" {
				return errors.New("OpenAI subscription encrypted reasoning cannot be represented by Chat Completions")
			}
			var summaries []json.RawMessage
			if summaryRaw, ok := output["summary"]; ok {
				if err := json.Unmarshal(summaryRaw, &summaries); err != nil {
					return fmt.Errorf("OpenAI subscription output %d reasoning summary must be an array", outputIndex)
				}
			}
			for summaryIndex, rawSummary := range summaries {
				var summary map[string]json.RawMessage
				if err := json.Unmarshal(rawSummary, &summary); err != nil || summary == nil {
					return fmt.Errorf("OpenAI subscription output %d summary %d must be an object", outputIndex, summaryIndex)
				}
				if err := rejectUnknownJSONObjectFields(summary, map[string]struct{}{"type": {}, "text": {}}, fmt.Sprintf("OpenAI subscription output %d summary %d", outputIndex, summaryIndex)); err != nil {
					return err
				}
			}
		case "function_call":
			if err := rejectUnknownJSONObjectFields(output, map[string]struct{}{
				"id": {}, "type": {}, "status": {}, "call_id": {}, "name": {}, "arguments": {},
			}, fmt.Sprintf("OpenAI subscription output %d", outputIndex)); err != nil {
				return err
			}
		case "web_search_call":
			if err := rejectUnknownJSONObjectFields(output, map[string]struct{}{
				"id": {}, "type": {}, "status": {}, "action": {},
			}, fmt.Sprintf("OpenAI subscription output %d", outputIndex)); err != nil {
				return err
			}
		case "image_generation_call":
		default:
			return fmt.Errorf("OpenAI subscription output %d has unsupported type %q", outputIndex, outputType)
		}
	}
	return nil
}

func validateStrictNativeOpenAIResponseOutputs(response *apicompat.ResponsesResponse) (int, int, error) {
	if response == nil {
		return 0, 0, errors.New("OpenAI subscription response is nil")
	}
	messageCount := 0
	imageCount := 0
	webSearchCalls := 0
	for index, output := range response.Output {
		switch output.Type {
		case "message":
			messageCount++
			if output.Role != "assistant" {
				return 0, 0, fmt.Errorf("OpenAI subscription output %d message role must be assistant", index)
			}
			for partIndex, part := range output.Content {
				if part.Type != "output_text" {
					return 0, 0, fmt.Errorf("OpenAI subscription output %d content %d has unsupported type %q", index, partIndex, part.Type)
				}
			}
		case "reasoning":
			for summaryIndex, summary := range output.Summary {
				if summary.Type != "summary_text" {
					return 0, 0, fmt.Errorf("OpenAI subscription reasoning summary %d has unsupported type %q", summaryIndex, summary.Type)
				}
			}
		case "function_call":
			if output.CallID == "" || output.Name == "" {
				return 0, 0, fmt.Errorf("OpenAI subscription function call %d requires call_id and name", index)
			}
		case "web_search_call":
			webSearchCalls++
		case "image_generation_call":
			imageCount++
			return 0, 0, errors.New("OpenAI subscription image output cannot be represented by Chat Completions")
		default:
			return 0, 0, fmt.Errorf("OpenAI subscription output %d has unsupported type %q", index, output.Type)
		}
	}
	if messageCount > 1 {
		return 0, 0, errors.New("OpenAI subscription response contains multiple assistant messages")
	}
	return imageCount, webSearchCalls, nil
}

func validateStrictNativeOpenAIStreamingTerminal(eventType string, response *apicompat.ResponsesResponse) error {
	if response == nil {
		return errors.New("OpenAI subscription terminal response is nil")
	}
	switch eventType {
	case "response.completed", "response.done":
		if response.Status != "completed" {
			return fmt.Errorf("OpenAI subscription %s event has status %q", eventType, response.Status)
		}
	case "response.incomplete":
		if response.Status != "incomplete" || response.IncompleteDetails == nil ||
			(response.IncompleteDetails.Reason != "max_output_tokens" && response.IncompleteDetails.Reason != "content_filter") {
			return errors.New("OpenAI subscription incomplete event has an unsupported status or reason")
		}
	default:
		return fmt.Errorf("unsupported OpenAI subscription terminal event %q", eventType)
	}
	_, _, err := validateStrictNativeOpenAIResponseOutputs(response)
	return err
}

func (s *OpenAIGatewayService) forwardNativeOpenAIChatSubscriptionStream(
	exchange gatewaytransport.Exchange,
	resp *http.Response,
	request *nativeOpenAIChatSubscriptionRequest,
	startedAt time.Time,
) (*OpenAIForwardResult, error) {
	writer := exchange.Response()
	if !writer.SupportsFlush() {
		return nil, errors.New("OpenAI subscription Chat Completions streaming requires flush support")
	}
	state := apicompat.NewResponsesEventToChatState()
	state.IncludeUsage = request.clientIncludeUsage
	created := false
	createdResponseID := ""
	createdAt := int64(0)
	terminal := false
	usage := OpenAIUsage{}
	usageFound := false
	webSearchCalls := 0
	var firstTokenMs *int
	maxLineSize := defaultMaxLineSize
	if s.cfg != nil && s.cfg.Gateway.MaxLineSize > 0 {
		maxLineSize = s.cfg.Gateway.MaxLineSize
	}
	writeChunks := func(chunks []apicompat.ChatCompletionsChunk) error {
		if len(chunks) == 0 {
			return nil
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
		for _, chunk := range chunks {
			line, err := apicompat.ChatChunkToSSE(chunk)
			if err != nil {
				return err
			}
			if _, err := writer.Write([]byte(line)); err != nil {
				return err
			}
		}
		return writer.Flush()
	}
	processData := func(data []byte) error {
		if terminal {
			return errors.New("OpenAI subscription upstream emitted data after terminal response")
		}
		if !json.Valid(data) {
			return errors.New("OpenAI subscription upstream emitted non-JSON SSE data")
		}
		var event apicompat.ResponsesStreamEvent
		if err := json.Unmarshal(data, &event); err != nil || event.Type == "" {
			return errors.New("OpenAI subscription upstream emitted an invalid Responses event")
		}
		switch event.Type {
		case "response.created":
			if created || event.Response == nil || event.Response.ID == "" || event.Response.Model != request.upstreamModel || event.Response.Status != "in_progress" {
				return errors.New("OpenAI subscription response.created event is incomplete or duplicated")
			}
			var envelope struct {
				Response struct {
					CreatedAt int64 `json:"created_at"`
				} `json:"response"`
			}
			if err := json.Unmarshal(data, &envelope); err != nil || envelope.Response.CreatedAt <= 0 {
				return errors.New("OpenAI subscription response.created event omitted created_at")
			}
			createdResponseID = event.Response.ID
			createdAt = envelope.Response.CreatedAt
			state.ID = event.Response.ID
			state.Model = request.originalModel
			state.Created = createdAt
			created = true
		case "response.output_text.delta", "response.output_item.added",
			"response.function_call_arguments.delta", "response.reasoning_summary_text.delta",
			"response.reasoning_text.delta":
			if !created {
				return fmt.Errorf("OpenAI subscription event %s arrived before response.created", event.Type)
			}
			if event.Type == "response.output_item.added" && event.Item != nil {
				switch event.Item.Type {
				case "message", "reasoning", "function_call":
				case "web_search_call":
					webSearchCalls++
				default:
					return fmt.Errorf("OpenAI subscription output item has unsupported type %q", event.Item.Type)
				}
			}
		case "response.in_progress", "response.output_item.done", "response.content_part.added",
			"response.content_part.done", "response.output_text.done",
			"response.function_call_arguments.done", "response.reasoning_summary_part.added",
			"response.reasoning_summary_part.done", "response.reasoning_summary_text.done",
			"response.reasoning_text.done":
			if !created {
				return fmt.Errorf("OpenAI subscription event %s arrived before response.created", event.Type)
			}
			return nil
		case "response.completed", "response.done", "response.incomplete":
			if !created || event.Response == nil {
				return errors.New("OpenAI subscription terminal event is incomplete")
			}
			var terminalEnvelope struct {
				Response json.RawMessage `json:"response"`
			}
			if err := json.Unmarshal(data, &terminalEnvelope); err != nil || len(terminalEnvelope.Response) == 0 {
				return errors.New("OpenAI subscription terminal event omitted response")
			}
			if err := validateStrictNativeOpenAIResponseWire(terminalEnvelope.Response); err != nil {
				return err
			}
			var terminalMetadata struct {
				ID        string `json:"id"`
				CreatedAt int64  `json:"created_at"`
				Model     string `json:"model"`
			}
			if err := json.Unmarshal(terminalEnvelope.Response, &terminalMetadata); err != nil ||
				terminalMetadata.ID != createdResponseID || terminalMetadata.CreatedAt != createdAt || terminalMetadata.Model != request.upstreamModel {
				return errors.New("OpenAI subscription terminal response metadata does not match response.created")
			}
			parsed, found, err := extractNativeOpenAIResponsesUsage(data)
			if err != nil || !found {
				if err == nil {
					err = errors.New("OpenAI subscription terminal event omitted exact usage")
				}
				return err
			}
			if err := validateStrictNativeOpenAIStreamingTerminal(event.Type, event.Response); err != nil {
				return err
			}
			usage = parsed
			usageFound = true
			terminal = true
			state.Usage = &apicompat.ChatUsage{
				PromptTokens:     usage.InputTokens,
				CompletionTokens: usage.OutputTokens,
				TotalTokens:      usage.InputTokens + usage.OutputTokens,
			}
			if usage.CacheReadInputTokens > 0 || usage.CacheCreationInputTokens > 0 {
				state.Usage.PromptTokensDetails = &apicompat.ChatTokenDetails{CachedTokens: usage.CacheReadInputTokens, CacheCreationTokens: usage.CacheCreationInputTokens}
			}
			event.Usage = nil
			event.Response.Usage = nil
		case "response.failed", "error":
			return &nativeOpenAIChatSemanticError{eventType: event.Type}
		default:
			return fmt.Errorf("OpenAI subscription upstream emitted unsupported event %q", event.Type)
		}
		chunks := apicompat.ResponsesEventToChatChunks(&event, state)
		if err := writeChunks(chunks); err != nil {
			return err
		}
		if terminal {
			if _, err := writer.Write([]byte("data: [DONE]\n\n")); err != nil {
				return err
			}
			if err := writer.Flush(); err != nil {
				return err
			}
			return errNativeOpenAIChatTerminal
		}
		return nil
	}
	scanErr := scanNativeOpenAIResponsesSSE(resp.Body, maxLineSize, nil, processData)
	if errors.Is(scanErr, errNativeOpenAIChatTerminal) {
		scanErr = nil
	}
	if scanErr != nil && !writer.Written() {
		var semanticErr *nativeOpenAIChatSemanticError
		if errors.As(scanErr, &semanticErr) {
			return nil, scanErr
		}
		return nil, nativeOpenAIAccountFailoverError(http.StatusBadGateway, GatewayFailureStageInference, openAIChatTransportUnavailableReason, scanErr)
	}
	result := &OpenAIForwardResult{
		RequestID:        resp.Header.Get("X-Request-Id"),
		Usage:            usage,
		Model:            request.originalModel,
		UpstreamModel:    request.upstreamModel,
		UpstreamEndpoint: nativeOpenAIResponsesEndpoint,
		Stream:           true,
		Duration:         time.Since(startedAt),
		FirstTokenMs:     firstTokenMs,
		WebSearchCalls:   webSearchCalls,
	}
	if scanErr != nil {
		return result, scanErr
	}
	if !terminal || !usageFound {
		return result, errors.New("OpenAI subscription upstream ended without a terminal response and exact usage")
	}
	return result, nil
}
