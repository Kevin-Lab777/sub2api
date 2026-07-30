package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	openAIImagesSubscriptionCredentialUnavailableReason GatewayFailureReason = "openai_images_subscription_credential_unavailable"
	openAIImagesSubscriptionTransportUnavailableReason  GatewayFailureReason = "openai_images_subscription_transport_unavailable"
)

type nativeOpenAIImagesSubscriptionRequest struct {
	endpoint          string
	originalModel     string
	upstreamModel     string
	prompt            string
	stream            bool
	n                 int
	size              string
	responseFormat    string
	quality           string
	background        string
	outputFormat      string
	moderation        string
	inputFidelity     string
	style             string
	outputCompression *int
	partialImages     *int
	inputImageURLs    []string
	maskImageURL      string
	uploads           []OpenAIImagesUpload
	maskUpload        *OpenAIImagesUpload
	responsesBody     []byte
}

// ValidateOpenAIImagesSubscriptionRequest reports whether an Images request
// can be represented by the subscription account's Responses transport
// without dropping a client field or inventing a remote image URL.
func ValidateOpenAIImagesSubscriptionRequest(req *http.Request, body []byte) error {
	_, err := parseStrictNativeOpenAIImagesSubscriptionRequest(req, body)
	return err
}

func parseStrictNativeOpenAIImagesSubscriptionRequest(req *http.Request, body []byte) (*nativeOpenAIImagesSubscriptionRequest, error) {
	if req == nil || req.URL == nil || req.Method != http.MethodPost ||
		(req.URL.Path != openAIImagesGenerationsEndpoint && req.URL.Path != openAIImagesEditsEndpoint) {
		return nil, errors.New("OpenAI subscription Images endpoint mismatch")
	}
	contentType := req.Header.Get("Content-Type")
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, fmt.Errorf("parse OpenAI subscription Images Content-Type: %w", err)
	}
	parsed := &nativeOpenAIImagesSubscriptionRequest{endpoint: req.URL.Path, n: 1}
	switch strings.ToLower(mediaType) {
	case "application/json":
		if err := parseStrictNativeOpenAIImagesSubscriptionJSON(body, parsed); err != nil {
			return nil, err
		}
	case "multipart/form-data":
		if req.URL.Path != openAIImagesEditsEndpoint {
			return nil, errors.New("OpenAI subscription image generation requires application/json")
		}
		if err := parseStrictNativeOpenAIImagesSubscriptionMultipart(body, contentType, parsed); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("OpenAI subscription Images request has unsupported Content-Type %q", contentType)
	}
	if parsed.originalModel == "" || parsed.originalModel != strings.TrimSpace(parsed.originalModel) {
		return nil, errors.New("OpenAI subscription Images model must be an exact non-empty string")
	}
	if strings.TrimSpace(parsed.prompt) == "" {
		return nil, errors.New("OpenAI subscription Images prompt must be a non-empty string")
	}
	if parsed.n <= 0 {
		return nil, errors.New("OpenAI subscription Images n must be a positive integer")
	}
	if parsed.responseFormat != "" && parsed.responseFormat != "b64_json" {
		return nil, errors.New("OpenAI subscription Images adapter cannot represent response_format=url without fabricating a remote URL")
	}
	if parsed.stream && parsed.n != 1 {
		return nil, errors.New("OpenAI subscription Images streaming cannot split aggregate usage across multiple images")
	}
	if parsed.endpoint == openAIImagesEditsEndpoint && len(parsed.inputImageURLs) == 0 && len(parsed.uploads) == 0 {
		return nil, errors.New("OpenAI subscription image edit requires at least one image input")
	}
	return parsed, nil
}

func parseStrictNativeOpenAIImagesSubscriptionJSON(body []byte, parsed *nativeOpenAIImagesSubscriptionRequest) error {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(body, &root); err != nil || root == nil {
		return errors.New("OpenAI subscription Images request must be a JSON object")
	}
	allowed := map[string]struct{}{
		"model": {}, "prompt": {}, "stream": {}, "n": {}, "size": {},
		"response_format": {}, "quality": {}, "background": {}, "output_format": {},
		"moderation": {}, "style": {}, "output_compression": {}, "partial_images": {},
	}
	if parsed.endpoint == openAIImagesEditsEndpoint {
		allowed["images"] = struct{}{}
		allowed["mask"] = struct{}{}
		allowed["input_fidelity"] = struct{}{}
	}
	if err := rejectUnknownJSONObjectFields(root, allowed, "OpenAI subscription Images request"); err != nil {
		return err
	}
	model, err := requiredExactJSONString(root, "model", "OpenAI subscription Images")
	if err != nil {
		return err
	}
	prompt, err := requiredNonBlankJSONString(root, "prompt", "OpenAI subscription Images")
	if err != nil {
		return err
	}
	parsed.originalModel = model
	parsed.prompt = prompt
	if raw, ok := root["stream"]; ok {
		var value any
		if err := json.Unmarshal(raw, &value); err != nil {
			return errors.New("OpenAI subscription Images stream must be a boolean")
		}
		var valid bool
		parsed.stream, valid = value.(bool)
		if !valid {
			return errors.New("OpenAI subscription Images stream must be a boolean")
		}
	}
	if raw, ok := root["n"]; ok {
		if err := json.Unmarshal(raw, &parsed.n); err != nil || parsed.n <= 0 {
			return errors.New("OpenAI subscription Images n must be a positive integer")
		}
	}
	for _, field := range []struct {
		name string
		dst  *string
	}{
		{name: "size", dst: &parsed.size},
		{name: "response_format", dst: &parsed.responseFormat},
		{name: "quality", dst: &parsed.quality},
		{name: "background", dst: &parsed.background},
		{name: "output_format", dst: &parsed.outputFormat},
		{name: "moderation", dst: &parsed.moderation},
		{name: "input_fidelity", dst: &parsed.inputFidelity},
		{name: "style", dst: &parsed.style},
	} {
		value, found, err := optionalExactJSONString(root, field.name, "OpenAI subscription Images")
		if err != nil {
			return err
		}
		if found {
			*field.dst = value
		}
	}
	if raw, ok := root["output_compression"]; ok {
		var value int
		if err := json.Unmarshal(raw, &value); err != nil || value < 0 || value > 100 {
			return errors.New("OpenAI subscription Images output_compression must be an integer from 0 through 100")
		}
		parsed.outputCompression = &value
	}
	if raw, ok := root["partial_images"]; ok {
		var value int
		if err := json.Unmarshal(raw, &value); err != nil || value < 0 || value > 3 {
			return errors.New("OpenAI subscription Images partial_images must be an integer from 0 through 3")
		}
		parsed.partialImages = &value
	}
	if parsed.endpoint == openAIImagesEditsEndpoint {
		if err := parseStrictNativeOpenAIImagesSubscriptionJSONInputs(root, parsed); err != nil {
			return err
		}
	}
	return nil
}

func parseStrictNativeOpenAIImagesSubscriptionJSONInputs(root map[string]json.RawMessage, parsed *nativeOpenAIImagesSubscriptionRequest) error {
	var images []json.RawMessage
	if err := json.Unmarshal(root["images"], &images); err != nil || len(images) == 0 {
		return errors.New("OpenAI subscription Images images must be a non-empty array")
	}
	for index, raw := range images {
		var image map[string]json.RawMessage
		if err := json.Unmarshal(raw, &image); err != nil || image == nil {
			return fmt.Errorf("OpenAI subscription Images image %d must be an object", index)
		}
		if err := rejectUnknownJSONObjectFields(image, map[string]struct{}{"image_url": {}}, fmt.Sprintf("OpenAI subscription Images image %d", index)); err != nil {
			return err
		}
		url, err := requiredExactJSONString(image, "image_url", fmt.Sprintf("OpenAI subscription Images image %d", index))
		if err != nil {
			return err
		}
		if nativeOpenAIChatEmptyBase64DataURI(url) {
			return fmt.Errorf("OpenAI subscription Images image %d contains empty base64 data", index)
		}
		parsed.inputImageURLs = append(parsed.inputImageURLs, url)
	}
	if raw, ok := root["mask"]; ok {
		var mask map[string]json.RawMessage
		if err := json.Unmarshal(raw, &mask); err != nil || mask == nil {
			return errors.New("OpenAI subscription Images mask must be an object")
		}
		if err := rejectUnknownJSONObjectFields(mask, map[string]struct{}{"image_url": {}}, "OpenAI subscription Images mask"); err != nil {
			return err
		}
		url, err := requiredExactJSONString(mask, "image_url", "OpenAI subscription Images mask")
		if err != nil {
			return err
		}
		if nativeOpenAIChatEmptyBase64DataURI(url) {
			return errors.New("OpenAI subscription Images mask contains empty base64 data")
		}
		parsed.maskImageURL = url
	}
	return nil
}

func parseStrictNativeOpenAIImagesSubscriptionMultipart(body []byte, contentType string, parsed *nativeOpenAIImagesSubscriptionRequest) error {
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return fmt.Errorf("parse OpenAI subscription Images multipart Content-Type: %w", err)
	}
	boundary := params["boundary"]
	if boundary == "" || boundary != strings.TrimSpace(boundary) {
		return errors.New("OpenAI subscription Images multipart boundary must be exact and non-empty")
	}
	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	scalars := make(map[string]string)
	allowedScalars := map[string]struct{}{
		"model": {}, "prompt": {}, "stream": {}, "n": {}, "size": {},
		"response_format": {}, "quality": {}, "background": {}, "output_format": {},
		"moderation": {}, "input_fidelity": {}, "style": {}, "output_compression": {},
		"partial_images": {},
	}
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read OpenAI subscription Images multipart body: %w", err)
		}
		name := part.FormName()
		filename := part.FileName()
		data, readErr := io.ReadAll(part)
		_ = part.Close()
		if readErr != nil {
			return fmt.Errorf("read OpenAI subscription Images multipart field %q: %w", name, readErr)
		}
		if filename != "" {
			if len(data) == 0 {
				return fmt.Errorf("OpenAI subscription Images upload %q is empty", name)
			}
			mediaType, mediaParams, parseErr := mime.ParseMediaType(part.Header.Get("Content-Type"))
			if parseErr != nil || len(mediaParams) != 0 || !strings.HasPrefix(strings.ToLower(mediaType), "image/") {
				return fmt.Errorf("OpenAI subscription Images upload %q requires an exact image Content-Type", name)
			}
			upload := OpenAIImagesUpload{FieldName: name, FileName: filename, ContentType: mediaType, Data: data}
			switch name {
			case "image", "image[]":
				parsed.uploads = append(parsed.uploads, upload)
			case "mask":
				if parsed.maskUpload != nil {
					return errors.New("OpenAI subscription Images multipart mask must not be duplicated")
				}
				parsed.maskUpload = &upload
			default:
				return fmt.Errorf("OpenAI subscription Images multipart contains unsupported file field %q", name)
			}
			continue
		}
		if _, ok := allowedScalars[name]; !ok {
			return fmt.Errorf("OpenAI subscription Images multipart contains unsupported field %q", name)
		}
		if _, duplicate := scalars[name]; duplicate {
			return fmt.Errorf("OpenAI subscription Images multipart field %q must not be duplicated", name)
		}
		scalars[name] = string(data)
	}
	model, ok := scalars["model"]
	if !ok || model == "" || model != strings.TrimSpace(model) {
		return errors.New("OpenAI subscription Images multipart model must be exact and non-empty")
	}
	prompt, ok := scalars["prompt"]
	if !ok || strings.TrimSpace(prompt) == "" {
		return errors.New("OpenAI subscription Images multipart prompt must be non-empty")
	}
	parsed.originalModel = model
	parsed.prompt = prompt
	if value, ok := scalars["stream"]; ok {
		if value != "true" && value != "false" {
			return errors.New("OpenAI subscription Images multipart stream must be a boolean")
		}
		parsed.stream = value == "true"
	}
	if value, ok := scalars["n"]; ok {
		parsed.n, err = strconv.Atoi(value)
		if err != nil || parsed.n <= 0 {
			return errors.New("OpenAI subscription Images multipart n must be a positive integer")
		}
	}
	for name, dst := range map[string]*string{
		"size": &parsed.size, "response_format": &parsed.responseFormat,
		"quality": &parsed.quality, "background": &parsed.background,
		"output_format": &parsed.outputFormat, "moderation": &parsed.moderation,
		"input_fidelity": &parsed.inputFidelity, "style": &parsed.style,
	} {
		if value, ok := scalars[name]; ok {
			if value == "" || value != strings.TrimSpace(value) {
				return fmt.Errorf("OpenAI subscription Images multipart %s must be exact and non-empty", name)
			}
			*dst = value
		}
	}
	if value, ok := scalars["output_compression"]; ok {
		parsedValue, parseErr := strconv.Atoi(value)
		if parseErr != nil || parsedValue < 0 || parsedValue > 100 {
			return errors.New("OpenAI subscription Images multipart output_compression must be an integer from 0 through 100")
		}
		parsed.outputCompression = &parsedValue
	}
	if value, ok := scalars["partial_images"]; ok {
		parsedValue, parseErr := strconv.Atoi(value)
		if parseErr != nil || parsedValue < 0 || parsedValue > 3 {
			return errors.New("OpenAI subscription Images multipart partial_images must be an integer from 0 through 3")
		}
		parsed.partialImages = &parsedValue
	}
	return nil
}

func requiredExactJSONString(object map[string]json.RawMessage, field, label string) (string, error) {
	value, found, err := optionalExactJSONString(object, field, label)
	if err != nil {
		return "", err
	}
	if !found {
		return "", fmt.Errorf("%s %s is required", label, field)
	}
	return value, nil
}

func requiredNonBlankJSONString(object map[string]json.RawMessage, field, label string) (string, error) {
	raw, found := object[field]
	if !found {
		return "", fmt.Errorf("%s %s is required", label, field)
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("%s %s must be a non-empty string", label, field)
	}
	return value, nil
}

func optionalExactJSONString(object map[string]json.RawMessage, field, label string) (string, bool, error) {
	raw, found := object[field]
	if !found {
		return "", false, nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil || value == "" || value != strings.TrimSpace(value) {
		return "", false, fmt.Errorf("%s %s must be an exact non-empty string", label, field)
	}
	return value, true, nil
}

func buildNativeOpenAIImagesSubscriptionRequest(account *Account, req *http.Request, body []byte) (*nativeOpenAIImagesSubscriptionRequest, error) {
	parsed, err := parseStrictNativeOpenAIImagesSubscriptionRequest(req, body)
	if err != nil {
		return nil, err
	}
	parsed.upstreamModel, err = resolveExactOpenAIAccountModel(account, parsed.originalModel)
	if err != nil {
		return nil, err
	}
	if !IsGPTImageGenerationModel(parsed.upstreamModel) {
		return nil, fmt.Errorf("OpenAI subscription Images requires a GPT image model, got %q", parsed.upstreamModel)
	}
	content := []map[string]any{{"type": "input_text", "text": parsed.prompt}}
	for _, imageURL := range parsed.inputImageURLs {
		content = append(content, map[string]any{"type": "input_image", "image_url": imageURL})
	}
	for _, upload := range parsed.uploads {
		dataURL, err := strictNativeOpenAIImagesUploadDataURL(upload)
		if err != nil {
			return nil, err
		}
		content = append(content, map[string]any{"type": "input_image", "image_url": dataURL})
	}
	tool := map[string]any{
		"type":   "image_generation",
		"action": "generate",
		"model":  parsed.upstreamModel,
	}
	if parsed.endpoint == openAIImagesEditsEndpoint {
		tool["action"] = "edit"
	}
	if parsed.n > 1 {
		tool["n"] = parsed.n
	}
	for name, value := range map[string]string{
		"size": parsed.size, "quality": parsed.quality, "background": parsed.background,
		"output_format": parsed.outputFormat, "moderation": parsed.moderation,
		"input_fidelity": parsed.inputFidelity, "style": parsed.style,
	} {
		if value != "" {
			tool[name] = value
		}
	}
	if parsed.outputCompression != nil {
		tool["output_compression"] = *parsed.outputCompression
	}
	if parsed.partialImages != nil {
		tool["partial_images"] = *parsed.partialImages
	}
	maskURL := parsed.maskImageURL
	if parsed.maskUpload != nil {
		maskURL, err = strictNativeOpenAIImagesUploadDataURL(*parsed.maskUpload)
		if err != nil {
			return nil, err
		}
	}
	if maskURL != "" {
		tool["input_image_mask"] = map[string]any{"image_url": maskURL}
	}
	responses := map[string]any{
		"input":       []any{map[string]any{"type": "message", "role": "user", "content": content}},
		"model":       openAIImagesResponsesMainModel,
		"store":       false,
		"stream":      true,
		"tool_choice": map[string]any{"type": "image_generation"},
		"tools":       []any{tool},
	}
	parsed.responsesBody, err = json.Marshal(responses)
	if err != nil {
		return nil, fmt.Errorf("marshal OpenAI subscription Images Responses request: %w", err)
	}
	return parsed, nil
}

func strictNativeOpenAIImagesUploadDataURL(upload OpenAIImagesUpload) (string, error) {
	if len(upload.Data) == 0 {
		return "", fmt.Errorf("OpenAI subscription Images upload %q is empty", upload.FieldName)
	}
	mediaType, params, err := mime.ParseMediaType(upload.ContentType)
	if err != nil || len(params) != 0 || !strings.HasPrefix(strings.ToLower(mediaType), "image/") {
		return "", fmt.Errorf("OpenAI subscription Images upload %q requires an exact image Content-Type", upload.FieldName)
	}
	return "data:" + mediaType + ";base64," + base64.StdEncoding.EncodeToString(upload.Data), nil
}

type nativeOpenAIImagesSubscriptionSemanticError struct {
	eventType string
}

func (e *nativeOpenAIImagesSubscriptionSemanticError) Error() string {
	return fmt.Sprintf("OpenAI subscription Images upstream terminated with %s", e.eventType)
}

// ForwardImagesSubscriptionExchange converts one public Images request into a
// real subscription-account Responses image-tool call and converts only
// authoritative Responses lifecycle data back to the Images protocol.
func (s *OpenAIGatewayService) ForwardImagesSubscriptionExchange(
	ctx context.Context,
	exchange gatewaytransport.Exchange,
	account *Account,
	body []byte,
) (*OpenAIForwardResult, error) {
	if exchange == nil || exchange.Request() == nil || exchange.Response() == nil {
		return nil, errors.New("OpenAI subscription Images exchange is required")
	}
	if account == nil || account.Platform != PlatformOpenAI || account.Type != AccountTypeOAuth {
		return nil, errors.New("OpenAI subscription Images adapter requires an OAuth account")
	}
	request, err := buildNativeOpenAIImagesSubscriptionRequest(account, exchange.Request(), body)
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
			openAIImagesSubscriptionCredentialUnavailableReason,
			fmt.Errorf("get OpenAI subscription Images credential: %w", err),
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
			openAIImagesSubscriptionTransportUnavailableReason,
			fmt.Errorf("send OpenAI subscription Images request: %w", err),
		)
	}
	if resp == nil || resp.Body == nil {
		return nil, nativeOpenAIAccountFailoverError(
			http.StatusBadGateway,
			GatewayFailureStageInference,
			openAIImagesSubscriptionTransportUnavailableReason,
			errors.New("OpenAI subscription Images upstream returned an empty HTTP response"),
		)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < http.StatusOK {
		return nil, fmt.Errorf("OpenAI subscription Images upstream returned invalid final status %d", resp.StatusCode)
	}
	if resp.StatusCode >= http.StatusMultipleChoices {
		return nil, s.handleNativeOpenAIResponsesError(exchange, account, resp)
	}
	if !account.IsShadow() {
		if snapshot := ParseCodexRateLimitHeaders(resp.Header); snapshot != nil {
			s.updateCodexUsageSnapshot(ctx, account.ID, snapshot)
		}
	}
	kind, err := classifyNativeOpenAIResponsesContentType(resp.Header.Get("Content-Type"))
	if err != nil || kind != nativeOpenAIResponsesContentSSE {
		if err == nil {
			err = errors.New("OpenAI subscription Images adapter requires an SSE Responses upstream")
		}
		return nil, nativeOpenAIAccountFailoverError(http.StatusBadGateway, GatewayFailureStageInference, openAIImagesSubscriptionTransportUnavailableReason, err)
	}
	if request.stream {
		return s.forwardNativeOpenAIImagesSubscriptionStream(exchange, resp, request, startedAt)
	}
	return s.forwardNativeOpenAIImagesSubscriptionJSON(exchange, resp, request, startedAt)
}

type nativeOpenAIImagesSubscriptionMetadata struct {
	Model        string
	OutputFormat string
	Size         string
	Background   string
	Quality      string
}

type nativeOpenAIImagesSubscriptionImage struct {
	Result        string
	RevisedPrompt string
	Metadata      nativeOpenAIImagesSubscriptionMetadata
}

type nativeOpenAIImagesSubscriptionOutputEvent struct {
	name string
	body []byte
}

type nativeOpenAIImagesSubscriptionState struct {
	request          *nativeOpenAIImagesSubscriptionRequest
	created          bool
	createdAt        int64
	metadata         nativeOpenAIImagesSubscriptionMetadata
	terminal         bool
	usage            OpenAIUsage
	usageRaw         json.RawMessage
	images           []nativeOpenAIImagesSubscriptionImage
	imageOutputSizes []string
}

func (s *nativeOpenAIImagesSubscriptionState) observe(data []byte) ([]nativeOpenAIImagesSubscriptionOutputEvent, error) {
	if s == nil || s.request == nil {
		return nil, errors.New("OpenAI subscription Images state is incomplete")
	}
	if !json.Valid(data) {
		return nil, errors.New("OpenAI subscription Images upstream emitted non-JSON SSE data")
	}
	var envelope struct {
		Type     string          `json:"type"`
		Response json.RawMessage `json:"response"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil || envelope.Type == "" {
		return nil, errors.New("OpenAI subscription Images upstream emitted an invalid Responses event")
	}
	if s.terminal {
		return nil, errors.New("OpenAI subscription Images upstream emitted data after the terminal response")
	}
	switch envelope.Type {
	case "response.created":
		createdAt, metadata, err := parseNativeOpenAIImagesSubscriptionLifecycle(envelope.Response)
		if err != nil {
			return nil, err
		}
		if s.created {
			return nil, errors.New("OpenAI subscription Images upstream emitted multiple response.created events")
		}
		s.created = true
		s.createdAt = createdAt
		s.metadata = metadata
		return nil, nil
	case "response.image_generation_call.partial_image":
		if !s.request.stream {
			return nil, nil
		}
		if !s.created {
			return nil, errors.New("OpenAI subscription Images partial image preceded response.created")
		}
		event, err := s.convertPartial(data)
		if err != nil {
			return nil, err
		}
		return []nativeOpenAIImagesSubscriptionOutputEvent{event}, nil
	case "response.completed":
		images, createdAt, metadata, usage, usageRaw, sizes, err := parseNativeOpenAIImagesSubscriptionTerminal(envelope.Response, s.request)
		if err != nil {
			return nil, err
		}
		if s.created && createdAt != s.createdAt {
			return nil, errors.New("OpenAI subscription Images terminal created_at differs from response.created")
		}
		if s.created {
			if err := requireCompatibleNativeOpenAIImagesMetadata(s.metadata, metadata); err != nil {
				return nil, err
			}
			metadata = mergeNativeOpenAIImagesSubscriptionMetadata(metadata, s.metadata)
			for index := range images {
				if err := requireCompatibleNativeOpenAIImagesMetadata(images[index].Metadata, metadata); err != nil {
					return nil, err
				}
				images[index].Metadata = mergeNativeOpenAIImagesSubscriptionMetadata(images[index].Metadata, metadata)
			}
			sizes = sizes[:0]
			for _, image := range images {
				if image.Metadata.Size != "" {
					sizes = append(sizes, image.Metadata.Size)
				}
			}
		}
		if s.request.stream && len(images) != s.request.n {
			return nil, fmt.Errorf("OpenAI subscription Images upstream returned %d images for n=%d", len(images), s.request.n)
		}
		s.createdAt = createdAt
		s.metadata = metadata
		s.terminal = true
		s.usage = usage
		s.usageRaw = append(json.RawMessage(nil), usageRaw...)
		s.images = images
		s.imageOutputSizes = sizes
		if !s.request.stream {
			return nil, nil
		}
		events := make([]nativeOpenAIImagesSubscriptionOutputEvent, 0, len(images))
		for _, image := range images {
			event, err := buildNativeOpenAIImagesSubscriptionCompletedEvent(s.request, image, createdAt, usageRaw)
			if err != nil {
				return nil, err
			}
			events = append(events, event)
		}
		return events, nil
	case "response.failed", "response.incomplete", "response.cancelled", "response.canceled", "error":
		return nil, &nativeOpenAIImagesSubscriptionSemanticError{eventType: envelope.Type}
	default:
		// Responses lifecycle events without a public Images representation are
		// deliberately observed but not rewritten. A real response.completed event
		// remains mandatory, so an unknown terminal cannot become success.
		return nil, nil
	}
}

func parseNativeOpenAIImagesSubscriptionLifecycle(raw json.RawMessage) (int64, nativeOpenAIImagesSubscriptionMetadata, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return 0, nativeOpenAIImagesSubscriptionMetadata{}, errors.New("OpenAI subscription Images response.created omitted a response object")
	}
	var response struct {
		CreatedAt int64             `json:"created_at"`
		Tools     []json.RawMessage `json:"tools"`
	}
	if err := json.Unmarshal(raw, &response); err != nil || response.CreatedAt <= 0 {
		return 0, nativeOpenAIImagesSubscriptionMetadata{}, errors.New("OpenAI subscription Images response.created requires a positive created_at")
	}
	metadata, err := parseNativeOpenAIImagesSubscriptionToolMetadata(response.Tools)
	if err != nil {
		return 0, nativeOpenAIImagesSubscriptionMetadata{}, err
	}
	return response.CreatedAt, metadata, nil
}

func parseNativeOpenAIImagesSubscriptionToolMetadata(tools []json.RawMessage) (nativeOpenAIImagesSubscriptionMetadata, error) {
	var metadata nativeOpenAIImagesSubscriptionMetadata
	found := 0
	for _, raw := range tools {
		var tool struct {
			Type         string `json:"type"`
			Model        string `json:"model"`
			OutputFormat string `json:"output_format"`
			Size         string `json:"size"`
			Background   string `json:"background"`
			Quality      string `json:"quality"`
		}
		if err := json.Unmarshal(raw, &tool); err != nil {
			return nativeOpenAIImagesSubscriptionMetadata{}, errors.New("OpenAI subscription Images response contains an invalid tool")
		}
		if tool.Type != "image_generation" {
			continue
		}
		found++
		metadata = nativeOpenAIImagesSubscriptionMetadata{
			Model: tool.Model, OutputFormat: tool.OutputFormat, Size: tool.Size,
			Background: tool.Background, Quality: tool.Quality,
		}
	}
	if found != 1 {
		return nativeOpenAIImagesSubscriptionMetadata{}, errors.New("OpenAI subscription Images response must contain exactly one image_generation tool")
	}
	for name, value := range map[string]string{
		"model": metadata.Model, "output_format": metadata.OutputFormat, "size": metadata.Size,
		"background": metadata.Background, "quality": metadata.Quality,
	} {
		if value != "" && value != strings.TrimSpace(value) {
			return nativeOpenAIImagesSubscriptionMetadata{}, fmt.Errorf("OpenAI subscription Images tool %s is inexact", name)
		}
	}
	if metadata.Model == "" {
		return nativeOpenAIImagesSubscriptionMetadata{}, errors.New("OpenAI subscription Images tool omitted model")
	}
	return metadata, nil
}

func requireCompatibleNativeOpenAIImagesMetadata(first, terminal nativeOpenAIImagesSubscriptionMetadata) error {
	for _, field := range []struct {
		name     string
		first    string
		terminal string
	}{
		{name: "model", first: first.Model, terminal: terminal.Model},
		{name: "output_format", first: first.OutputFormat, terminal: terminal.OutputFormat},
		{name: "size", first: first.Size, terminal: terminal.Size},
		{name: "background", first: first.Background, terminal: terminal.Background},
		{name: "quality", first: first.Quality, terminal: terminal.Quality},
	} {
		if field.first != "" && field.terminal != "" && field.first != field.terminal {
			return fmt.Errorf("OpenAI subscription Images %s changed between response.created and response.completed", field.name)
		}
	}
	return nil
}

func mergeNativeOpenAIImagesSubscriptionMetadata(primary, secondary nativeOpenAIImagesSubscriptionMetadata) nativeOpenAIImagesSubscriptionMetadata {
	if primary.Model == "" {
		primary.Model = secondary.Model
	}
	if primary.OutputFormat == "" {
		primary.OutputFormat = secondary.OutputFormat
	}
	if primary.Size == "" {
		primary.Size = secondary.Size
	}
	if primary.Background == "" {
		primary.Background = secondary.Background
	}
	if primary.Quality == "" {
		primary.Quality = secondary.Quality
	}
	return primary
}

func parseNativeOpenAIImagesSubscriptionTerminal(
	raw json.RawMessage,
	request *nativeOpenAIImagesSubscriptionRequest,
) ([]nativeOpenAIImagesSubscriptionImage, int64, nativeOpenAIImagesSubscriptionMetadata, OpenAIUsage, json.RawMessage, []string, error) {
	if request == nil || len(raw) == 0 || string(raw) == "null" {
		return nil, 0, nativeOpenAIImagesSubscriptionMetadata{}, OpenAIUsage{}, nil, nil, errors.New("OpenAI subscription Images terminal event omitted a response object")
	}
	var response struct {
		Status    string            `json:"status"`
		CreatedAt int64             `json:"created_at"`
		Tools     []json.RawMessage `json:"tools"`
		Output    []json.RawMessage `json:"output"`
		Usage     json.RawMessage   `json:"usage"`
		ToolUsage struct {
			ImageGen json.RawMessage `json:"image_gen"`
		} `json:"tool_usage"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, 0, nativeOpenAIImagesSubscriptionMetadata{}, OpenAIUsage{}, nil, nil, fmt.Errorf("parse OpenAI subscription Images terminal response: %w", err)
	}
	if response.Status != "completed" || response.CreatedAt <= 0 {
		return nil, 0, nativeOpenAIImagesSubscriptionMetadata{}, OpenAIUsage{}, nil, nil, errors.New("OpenAI subscription Images terminal response requires status=completed and a positive created_at")
	}
	metadata, err := parseNativeOpenAIImagesSubscriptionToolMetadata(response.Tools)
	if err != nil {
		return nil, 0, nativeOpenAIImagesSubscriptionMetadata{}, OpenAIUsage{}, nil, nil, err
	}
	if metadata.Model != request.upstreamModel {
		return nil, 0, nativeOpenAIImagesSubscriptionMetadata{}, OpenAIUsage{}, nil, nil, fmt.Errorf("OpenAI subscription Images tool model mismatch: expected %q, got %q", request.upstreamModel, metadata.Model)
	}
	images := make([]nativeOpenAIImagesSubscriptionImage, 0, len(response.Output))
	for index, rawOutput := range response.Output {
		var header struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(rawOutput, &header); err != nil || header.Type == "" {
			return nil, 0, nativeOpenAIImagesSubscriptionMetadata{}, OpenAIUsage{}, nil, nil, fmt.Errorf("OpenAI subscription Images output %d is invalid", index)
		}
		switch header.Type {
		case "reasoning":
			continue
		case "message":
			return nil, 0, nativeOpenAIImagesSubscriptionMetadata{}, OpenAIUsage{}, nil, nil, &nativeOpenAIImagesSubscriptionSemanticError{eventType: "response.completed without image output"}
		case "image_generation_call":
			image, err := parseNativeOpenAIImagesSubscriptionImage(rawOutput, metadata, index)
			if err != nil {
				return nil, 0, nativeOpenAIImagesSubscriptionMetadata{}, OpenAIUsage{}, nil, nil, err
			}
			images = append(images, image)
		default:
			return nil, 0, nativeOpenAIImagesSubscriptionMetadata{}, OpenAIUsage{}, nil, nil, fmt.Errorf("OpenAI subscription Images output %d has unsupported type %q", index, header.Type)
		}
	}
	if len(images) == 0 {
		return nil, 0, nativeOpenAIImagesSubscriptionMetadata{}, OpenAIUsage{}, nil, nil, &nativeOpenAIImagesSubscriptionSemanticError{eventType: "response.completed without image output"}
	}
	usage, usageRaw, err := parseNativeOpenAIImagesSubscriptionUsage(response.ToolUsage.ImageGen, response.Usage, request.endpoint == openAIImagesEditsEndpoint, len(images))
	if err != nil {
		return nil, 0, nativeOpenAIImagesSubscriptionMetadata{}, OpenAIUsage{}, nil, nil, err
	}
	sizes := make([]string, 0, len(images))
	for _, image := range images {
		if image.Metadata.Size != "" {
			sizes = append(sizes, image.Metadata.Size)
		}
	}
	return images, response.CreatedAt, metadata, usage, usageRaw, sizes, nil
}

func parseNativeOpenAIImagesSubscriptionImage(raw json.RawMessage, tool nativeOpenAIImagesSubscriptionMetadata, index int) (nativeOpenAIImagesSubscriptionImage, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil || object == nil {
		return nativeOpenAIImagesSubscriptionImage{}, fmt.Errorf("OpenAI subscription Images output %d must be an object", index)
	}
	if err := rejectUnknownJSONObjectFields(object, map[string]struct{}{
		"id": {}, "type": {}, "status": {}, "result": {}, "revised_prompt": {},
		"model": {}, "output_format": {}, "size": {}, "background": {}, "quality": {},
	}, fmt.Sprintf("OpenAI subscription Images output %d", index)); err != nil {
		return nativeOpenAIImagesSubscriptionImage{}, err
	}
	result, err := requiredExactJSONString(object, "result", fmt.Sprintf("OpenAI subscription Images output %d", index))
	if err != nil {
		return nativeOpenAIImagesSubscriptionImage{}, err
	}
	image := nativeOpenAIImagesSubscriptionImage{Result: result, Metadata: tool}
	if rawPrompt, ok := object["revised_prompt"]; ok {
		if err := json.Unmarshal(rawPrompt, &image.RevisedPrompt); err != nil {
			return nativeOpenAIImagesSubscriptionImage{}, fmt.Errorf("OpenAI subscription Images output %d revised_prompt must be a string", index)
		}
	}
	for _, field := range []struct {
		name string
		dst  *string
	}{
		{name: "model", dst: &image.Metadata.Model},
		{name: "output_format", dst: &image.Metadata.OutputFormat},
		{name: "size", dst: &image.Metadata.Size},
		{name: "background", dst: &image.Metadata.Background},
		{name: "quality", dst: &image.Metadata.Quality},
	} {
		value, found, err := optionalExactJSONString(object, field.name, fmt.Sprintf("OpenAI subscription Images output %d", index))
		if err != nil {
			return nativeOpenAIImagesSubscriptionImage{}, err
		}
		if !found {
			continue
		}
		toolValue := *field.dst
		if toolValue != "" && toolValue != value {
			return nativeOpenAIImagesSubscriptionImage{}, fmt.Errorf("OpenAI subscription Images output %d %s conflicts with tool metadata", index, field.name)
		}
		*field.dst = value
	}
	return image, nil
}

type nativeOpenAIImagesSubscriptionUsageWire struct {
	InputTokens  *int `json:"input_tokens"`
	OutputTokens *int `json:"output_tokens"`
	TotalTokens  *int `json:"total_tokens"`
	Images       *int `json:"images"`
	InputDetails *struct {
		ImageTokens *int `json:"image_tokens"`
	} `json:"input_tokens_details"`
	OutputDetails *struct {
		ImageTokens *int `json:"image_tokens"`
	} `json:"output_tokens_details"`
}

type nativeOpenAIImagesSubscriptionResponsesUsageWire struct {
	InputDetails *struct {
		CachedTokens *int `json:"cached_tokens"`
		ImageTokens  *int `json:"image_tokens"`
	} `json:"input_tokens_details"`
}

func parseNativeOpenAIImagesSubscriptionUsage(toolRaw, responsesRaw json.RawMessage, edits bool, imageCount int) (OpenAIUsage, json.RawMessage, error) {
	if len(toolRaw) == 0 || string(toolRaw) == "null" {
		return OpenAIUsage{}, nil, errors.New("OpenAI subscription Images terminal response omitted image_gen usage")
	}
	var wire nativeOpenAIImagesSubscriptionUsageWire
	if err := json.Unmarshal(toolRaw, &wire); err != nil {
		return OpenAIUsage{}, nil, fmt.Errorf("parse OpenAI subscription Images image_gen usage: %w", err)
	}
	if wire.InputTokens == nil || wire.OutputTokens == nil || wire.OutputDetails == nil || wire.OutputDetails.ImageTokens == nil {
		return OpenAIUsage{}, nil, errors.New("OpenAI subscription Images image_gen usage omitted required token fields")
	}
	values := []*int{wire.InputTokens, wire.OutputTokens, wire.TotalTokens, wire.Images, wire.OutputDetails.ImageTokens}
	if wire.InputDetails != nil {
		values = append(values, wire.InputDetails.ImageTokens)
	}
	for _, value := range values {
		if value != nil && *value < 0 {
			return OpenAIUsage{}, nil, errors.New("OpenAI subscription Images usage contains a negative value")
		}
	}
	if *wire.OutputDetails.ImageTokens > *wire.OutputTokens {
		return OpenAIUsage{}, nil, errors.New("OpenAI subscription Images image output tokens exceed output_tokens")
	}
	if wire.TotalTokens != nil && *wire.TotalTokens != *wire.InputTokens+*wire.OutputTokens {
		return OpenAIUsage{}, nil, errors.New("OpenAI subscription Images total_tokens is inconsistent")
	}
	if wire.Images != nil && *wire.Images != imageCount {
		return OpenAIUsage{}, nil, errors.New("OpenAI subscription Images usage image count differs from terminal output")
	}
	var responsesWire nativeOpenAIImagesSubscriptionResponsesUsageWire
	if len(responsesRaw) > 0 && string(responsesRaw) != "null" {
		if err := json.Unmarshal(responsesRaw, &responsesWire); err != nil {
			return OpenAIUsage{}, nil, fmt.Errorf("parse OpenAI subscription Responses usage: %w", err)
		}
	}
	var imageInputTokens *int
	if wire.InputDetails != nil && wire.InputDetails.ImageTokens != nil {
		value := *wire.InputDetails.ImageTokens
		imageInputTokens = &value
	}
	if responsesWire.InputDetails != nil && responsesWire.InputDetails.ImageTokens != nil {
		value := *responsesWire.InputDetails.ImageTokens
		if value < 0 {
			return OpenAIUsage{}, nil, errors.New("OpenAI subscription Responses usage contains negative image input tokens")
		}
		if imageInputTokens != nil && *imageInputTokens != value {
			return OpenAIUsage{}, nil, errors.New("OpenAI subscription Images usage sources disagree on image input tokens")
		}
		imageInputTokens = &value
	}
	if edits && imageInputTokens == nil {
		return OpenAIUsage{}, nil, errors.New("OpenAI subscription image edit usage omitted exact image input tokens")
	}
	if !edits && imageInputTokens != nil && *imageInputTokens != 0 {
		return OpenAIUsage{}, nil, errors.New("OpenAI subscription image generation reported unexpected image input tokens")
	}
	usage := OpenAIUsage{
		InputTokens:       *wire.InputTokens,
		OutputTokens:      *wire.OutputTokens,
		ImageOutputTokens: *wire.OutputDetails.ImageTokens,
	}
	if imageInputTokens != nil {
		if *imageInputTokens > usage.InputTokens {
			return OpenAIUsage{}, nil, errors.New("OpenAI subscription Images image input tokens exceed input_tokens")
		}
		usage.ImageInputTokens = *imageInputTokens
	}
	if responsesWire.InputDetails != nil && responsesWire.InputDetails.CachedTokens != nil {
		if *responsesWire.InputDetails.CachedTokens < 0 {
			return OpenAIUsage{}, nil, errors.New("OpenAI subscription Responses usage contains negative cached tokens")
		}
		usage.CacheReadInputTokens = *responsesWire.InputDetails.CachedTokens
	}
	return usage, append(json.RawMessage(nil), toolRaw...), nil
}

func (s *nativeOpenAIImagesSubscriptionState) convertPartial(data []byte) (nativeOpenAIImagesSubscriptionOutputEvent, error) {
	var partial struct {
		PartialImageB64   string `json:"partial_image_b64"`
		PartialImageIndex *int   `json:"partial_image_index"`
		OutputFormat      string `json:"output_format"`
		Background        string `json:"background"`
	}
	if err := json.Unmarshal(data, &partial); err != nil || partial.PartialImageB64 == "" || partial.PartialImageB64 != strings.TrimSpace(partial.PartialImageB64) {
		return nativeOpenAIImagesSubscriptionOutputEvent{}, errors.New("OpenAI subscription Images partial event omitted exact partial_image_b64")
	}
	if partial.PartialImageIndex == nil || *partial.PartialImageIndex < 0 {
		return nativeOpenAIImagesSubscriptionOutputEvent{}, errors.New("OpenAI subscription Images partial event omitted a non-negative partial_image_index")
	}
	metadata := s.metadata
	for _, field := range []struct {
		name     string
		value    string
		base     string
		setValue func(string)
	}{
		{name: "output_format", value: partial.OutputFormat, base: metadata.OutputFormat, setValue: func(value string) { metadata.OutputFormat = value }},
		{name: "background", value: partial.Background, base: metadata.Background, setValue: func(value string) { metadata.Background = value }},
	} {
		if field.value == "" {
			continue
		}
		if field.value != strings.TrimSpace(field.value) || (field.base != "" && field.base != field.value) {
			return nativeOpenAIImagesSubscriptionOutputEvent{}, fmt.Errorf("OpenAI subscription Images partial %s conflicts with response.created", field.name)
		}
		field.setValue(field.value)
	}
	payload := map[string]any{
		"type":                openAIImagesSubscriptionStreamPrefix(s.request) + ".partial_image",
		"created_at":          s.createdAt,
		"partial_image_index": *partial.PartialImageIndex,
		"b64_json":            partial.PartialImageB64,
	}
	addNativeOpenAIImagesSubscriptionMetadata(payload, metadata)
	body, err := json.Marshal(payload)
	if err != nil {
		return nativeOpenAIImagesSubscriptionOutputEvent{}, fmt.Errorf("marshal OpenAI subscription Images partial event: %w", err)
	}
	return nativeOpenAIImagesSubscriptionOutputEvent{name: payload["type"].(string), body: body}, nil
}

func buildNativeOpenAIImagesSubscriptionCompletedEvent(request *nativeOpenAIImagesSubscriptionRequest, image nativeOpenAIImagesSubscriptionImage, createdAt int64, usageRaw json.RawMessage) (nativeOpenAIImagesSubscriptionOutputEvent, error) {
	name := openAIImagesSubscriptionStreamPrefix(request) + ".completed"
	payload := map[string]any{
		"type":       name,
		"created_at": createdAt,
		"b64_json":   image.Result,
		"usage":      usageRaw,
	}
	if image.RevisedPrompt != "" {
		payload["revised_prompt"] = image.RevisedPrompt
	}
	addNativeOpenAIImagesSubscriptionMetadata(payload, image.Metadata)
	body, err := json.Marshal(payload)
	if err != nil {
		return nativeOpenAIImagesSubscriptionOutputEvent{}, fmt.Errorf("marshal OpenAI subscription Images completed event: %w", err)
	}
	return nativeOpenAIImagesSubscriptionOutputEvent{name: name, body: body}, nil
}

func addNativeOpenAIImagesSubscriptionMetadata(payload map[string]any, metadata nativeOpenAIImagesSubscriptionMetadata) {
	for name, value := range map[string]string{
		"model": metadata.Model, "output_format": metadata.OutputFormat, "size": metadata.Size,
		"background": metadata.Background, "quality": metadata.Quality,
	} {
		if value != "" {
			payload[name] = value
		}
	}
}

func openAIImagesSubscriptionStreamPrefix(request *nativeOpenAIImagesSubscriptionRequest) string {
	if request != nil && request.endpoint == openAIImagesEditsEndpoint {
		return "image_edit"
	}
	return "image_generation"
}

func (s *OpenAIGatewayService) forwardNativeOpenAIImagesSubscriptionJSON(
	exchange gatewaytransport.Exchange,
	resp *http.Response,
	request *nativeOpenAIImagesSubscriptionRequest,
	startedAt time.Time,
) (*OpenAIForwardResult, error) {
	body, err := readUpstreamResponseBodyExchange(resp.Body, s.cfg, exchange, nil)
	if err != nil {
		return nil, nativeOpenAIAccountFailoverError(http.StatusBadGateway, GatewayFailureStageInference, openAIImagesSubscriptionTransportUnavailableReason, err)
	}
	state := &nativeOpenAIImagesSubscriptionState{request: request}
	maxLineSize := defaultMaxLineSize
	if s.cfg != nil && s.cfg.Gateway.MaxLineSize > 0 {
		maxLineSize = s.cfg.Gateway.MaxLineSize
	}
	err = scanNativeOpenAIImagesSubscriptionSSE(bytes.NewReader(body), maxLineSize, func(data []byte) error {
		_, observeErr := state.observe(data)
		return observeErr
	})
	if err != nil {
		return nil, nativeOpenAIImagesSubscriptionObservedError(err)
	}
	if !state.terminal {
		return nil, nativeOpenAIAccountFailoverError(
			http.StatusBadGateway,
			GatewayFailureStageInference,
			openAIImagesSubscriptionTransportUnavailableReason,
			errors.New("OpenAI subscription Images stream ended without response.completed"),
		)
	}
	responseBody, err := buildNativeOpenAIImagesSubscriptionJSONResponse(state)
	if err != nil {
		return nil, err
	}
	result := nativeOpenAIImagesSubscriptionResult(resp, request, state, startedAt, nil)
	var protocolErr error
	if len(state.images) != request.n {
		protocolErr = fmt.Errorf("OpenAI subscription Images upstream returned %d images for n=%d", len(state.images), request.n)
	}
	responseheaders.WriteFilteredHeaders(exchange.Response().Header(), resp.Header, s.responseHeaderFilter)
	writeErr := exchange.WriteData(resp.StatusCode, "application/json", responseBody)
	return result, errors.Join(protocolErr, writeErr)
}

func buildNativeOpenAIImagesSubscriptionJSONResponse(state *nativeOpenAIImagesSubscriptionState) ([]byte, error) {
	if state == nil || !state.terminal || state.createdAt <= 0 || len(state.images) == 0 || len(state.usageRaw) == 0 {
		return nil, errors.New("OpenAI subscription Images terminal state is incomplete")
	}
	data := make([]map[string]any, 0, len(state.images))
	for _, image := range state.images {
		item := map[string]any{"b64_json": image.Result}
		if image.RevisedPrompt != "" {
			item["revised_prompt"] = image.RevisedPrompt
		}
		addNativeOpenAIImagesSubscriptionMetadata(item, image.Metadata)
		data = append(data, item)
	}
	payload := map[string]any{
		"created": state.createdAt,
		"data":    data,
		"usage":   state.usageRaw,
	}
	addNativeOpenAIImagesSubscriptionMetadata(payload, state.metadata)
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal OpenAI subscription Images response: %w", err)
	}
	return body, nil
}

func (s *OpenAIGatewayService) forwardNativeOpenAIImagesSubscriptionStream(
	exchange gatewaytransport.Exchange,
	resp *http.Response,
	request *nativeOpenAIImagesSubscriptionRequest,
	startedAt time.Time,
) (*OpenAIForwardResult, error) {
	writer := exchange.Response()
	if !writer.SupportsFlush() {
		return nil, errors.New("OpenAI subscription Images streaming requires flush support")
	}
	state := &nativeOpenAIImagesSubscriptionState{request: request}
	var firstTokenMs *int
	maxLineSize := defaultMaxLineSize
	if s.cfg != nil && s.cfg.Gateway.MaxLineSize > 0 {
		maxLineSize = s.cfg.Gateway.MaxLineSize
	}
	writeEvent := func(event nativeOpenAIImagesSubscriptionOutputEvent) error {
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
		frame := append([]byte("event: "+event.name+"\ndata: "), event.body...)
		frame = append(frame, '\n', '\n')
		if _, err := writer.Write(frame); err != nil {
			return err
		}
		return writer.Flush()
	}
	err := scanNativeOpenAIImagesSubscriptionSSE(resp.Body, maxLineSize, func(data []byte) error {
		events, observeErr := state.observe(data)
		if observeErr != nil {
			return observeErr
		}
		for _, event := range events {
			if err := writeEvent(event); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		if state.terminal {
			return nativeOpenAIImagesSubscriptionResult(resp, request, state, startedAt, firstTokenMs), err
		}
		return nil, nativeOpenAIImagesSubscriptionObservedError(err)
	}
	if !state.terminal {
		return nil, nativeOpenAIAccountFailoverError(
			http.StatusBadGateway,
			GatewayFailureStageInference,
			openAIImagesSubscriptionTransportUnavailableReason,
			errors.New("OpenAI subscription Images stream ended without response.completed"),
		)
	}
	result := nativeOpenAIImagesSubscriptionResult(resp, request, state, startedAt, firstTokenMs)
	if len(state.images) != request.n {
		return result, fmt.Errorf("OpenAI subscription Images upstream returned %d images for n=%d", len(state.images), request.n)
	}
	return result, nil
}

func scanNativeOpenAIImagesSubscriptionSSE(reader io.Reader, maxLineSize int, onData func([]byte) error) error {
	if reader == nil || maxLineSize <= 0 {
		return errors.New("OpenAI subscription Images SSE reader configuration is invalid")
	}
	buffered := bufio.NewReaderSize(reader, maxLineSize+1)
	done := false
	for {
		event, err := readNativeOpenAIImagesSSEEvent(buffered, maxLineSize)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if event == nil || len(event.data) == 0 {
			continue
		}
		data := bytes.TrimSpace(event.data)
		if bytes.Equal(data, []byte("[DONE]")) {
			if done {
				return errors.New("OpenAI subscription Images upstream emitted multiple [DONE] markers")
			}
			done = true
			continue
		}
		if done {
			return errors.New("OpenAI subscription Images upstream emitted data after [DONE]")
		}
		if !json.Valid(data) {
			return errors.New("OpenAI subscription Images upstream emitted non-JSON SSE data")
		}
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(data, &envelope); err != nil || envelope.Type == "" {
			return errors.New("OpenAI subscription Images upstream emitted an invalid Responses event")
		}
		if event.name != "" && event.name != envelope.Type {
			return fmt.Errorf("OpenAI subscription Images SSE event name %q does not match payload type %q", event.name, envelope.Type)
		}
		if onData != nil {
			if err := onData(data); err != nil {
				return err
			}
		}
	}
}

func nativeOpenAIImagesSubscriptionObservedError(err error) error {
	if err == nil {
		return nil
	}
	var semantic *nativeOpenAIImagesSubscriptionSemanticError
	if errors.As(err, &semantic) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return nativeOpenAIAccountFailoverError(
		http.StatusBadGateway,
		GatewayFailureStageInference,
		openAIImagesSubscriptionTransportUnavailableReason,
		err,
	)
}

func nativeOpenAIImagesSubscriptionResult(
	resp *http.Response,
	request *nativeOpenAIImagesSubscriptionRequest,
	state *nativeOpenAIImagesSubscriptionState,
	startedAt time.Time,
	firstTokenMs *int,
) *OpenAIForwardResult {
	if resp == nil || request == nil || state == nil || !state.terminal {
		return nil
	}
	return &OpenAIForwardResult{
		RequestID:        firstNonEmptyString(resp.Header.Get("X-Request-Id"), resp.Header.Get("Request-Id")),
		Usage:            state.usage,
		Model:            request.originalModel,
		BillingModel:     request.upstreamModel,
		UpstreamModel:    request.upstreamModel,
		UpstreamEndpoint: nativeOpenAIResponsesEndpoint,
		Stream:           request.stream,
		ResponseHeaders:  resp.Header.Clone(),
		Duration:         time.Since(startedAt),
		FirstTokenMs:     firstTokenMs,
		ImageCount:       len(state.images),
		ImageInputSize:   request.size,
		ImageOutputSizes: append([]string(nil), state.imageOutputSizes...),
	}
}
