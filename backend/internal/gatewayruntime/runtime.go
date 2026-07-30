package gatewayruntime

import (
	"errors"
	"fmt"

	"github.com/Wei-Shaw/sub2api/gatewaycore"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/wire"
)

// NewRuntime assembles the protocol-independent technical account gateway.
// Customer identity, billing, and quota accounting stay outside this runtime;
// the caller supplies the usage callback through gatewaycore.Invocation.
func NewRuntime(
	groupRepository service.GroupRepository,
	gatewayService *service.GatewayService,
	openAIGatewayService *service.OpenAIGatewayService,
	geminiService *service.GeminiMessagesCompatService,
	antigravityService *service.AntigravityGatewayService,
	cfg *config.Config,
) (gatewaycore.Runtime, error) {
	if groupRepository == nil {
		return nil, errors.New("gateway runtime group repository is required")
	}
	if cfg == nil {
		return nil, errors.New("gateway runtime config is required")
	}
	if gatewayService == nil || openAIGatewayService == nil || geminiService == nil || antigravityService == nil {
		return nil, errors.New("gateway runtime provider services are incomplete")
	}

	poolResolver := NewPoolResolver(groupRepository)
	anthropicConfig, err := AnthropicDispatcherConfigFromApplication(cfg)
	if err != nil {
		return nil, err
	}
	anthropic, err := NewAnthropicDispatcher(gatewayService, anthropicConfig)
	if err != nil {
		return nil, fmt.Errorf("assemble Anthropic dispatcher: %w", err)
	}

	responsesConfig, err := OpenAIResponsesDispatcherConfigFromApplication(cfg)
	if err != nil {
		return nil, err
	}
	responses, err := NewOpenAIResponsesDispatcher(openAIGatewayService, responsesConfig)
	if err != nil {
		return nil, fmt.Errorf("assemble OpenAI Responses dispatcher: %w", err)
	}
	chatConfig, err := OpenAIChatCompletionsDispatcherConfigFromApplication(cfg)
	if err != nil {
		return nil, err
	}
	chat, err := NewOpenAIChatCompletionsDispatcher(openAIGatewayService, chatConfig)
	if err != nil {
		return nil, fmt.Errorf("assemble OpenAI Chat dispatcher: %w", err)
	}
	embeddingsConfig, err := OpenAIEmbeddingsDispatcherConfigFromApplication(cfg)
	if err != nil {
		return nil, err
	}
	embeddings, err := NewOpenAIEmbeddingsDispatcher(openAIGatewayService, embeddingsConfig)
	if err != nil {
		return nil, fmt.Errorf("assemble OpenAI Embeddings dispatcher: %w", err)
	}
	imagesConfig, err := OpenAIImagesDispatcherConfigFromApplication(cfg)
	if err != nil {
		return nil, err
	}
	images, err := NewOpenAIImagesDispatcher(openAIGatewayService, imagesConfig)
	if err != nil {
		return nil, fmt.Errorf("assemble OpenAI Images dispatcher: %w", err)
	}
	liveConfig, err := OpenAILiveDispatcherConfigFromApplication(cfg)
	if err != nil {
		return nil, err
	}
	live, err := NewOpenAILiveDispatcher(openAIGatewayService, liveConfig)
	if err != nil {
		return nil, fmt.Errorf("assemble OpenAI Live dispatcher: %w", err)
	}
	openAI, err := NewOpenAIDispatcher(OpenAIEndpointDispatchers{
		Responses:       responses,
		ChatCompletions: chat,
		Embeddings:      embeddings,
		Images:          images,
		Live:            live,
	})
	if err != nil {
		return nil, fmt.Errorf("assemble OpenAI dispatcher: %w", err)
	}

	geminiConfig, err := GeminiDispatcherConfigFromApplication(cfg)
	if err != nil {
		return nil, err
	}
	gemini, err := NewGeminiDispatcher(gatewayService, geminiService, antigravityService, geminiConfig)
	if err != nil {
		return nil, fmt.Errorf("assemble Gemini dispatcher: %w", err)
	}

	engine, err := gatewaycore.NewEngine(poolResolver, gatewaycore.Dispatchers{
		Anthropic: anthropic,
		OpenAI:    openAI,
		Gemini:    gemini,
	})
	if err != nil {
		return nil, fmt.Errorf("assemble gateway runtime engine: %w", err)
	}
	return engine, nil
}

// ProviderSet exposes runtime assembly to the application Wire graph.
var ProviderSet = wire.NewSet(NewRuntime)
