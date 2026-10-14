package relay

import (
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel/vertex"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

// This guard belongs after disabled fields/ParamOverride and outside the raw
// passthrough branches. Claude Messages gets the thinking/sampling collection;
// OpenAI proxy protocols get only the confirmed 4.7 sampling rule.
func normalizeClaudeOutboundJSON(body []byte, info *relaycommon.RelayInfo, request any) ([]byte, error) {
	if !info.IsRequestCompatibilityEnabled() {
		return body, nil
	}
	model := info.UpstreamModelName
	switch req := request.(type) {
	case *dto.ClaudeRequest:
		model = req.Model
		// AWS uses RelayInfo for InvokeModel's ModelId, not the JSON model.
		// Keep the normalized conversion model and transport model aligned.
		if dto.ClaudeRequiresAdaptiveThinking(info.UpstreamModelName) {
			info.UpstreamModelName = model
		}
	case *vertex.VertexAIClaudeRequest:
		// Vertex removes model from its payload and selects it in the URL.
		reqModel := &dto.ClaudeRequest{Model: model, Thinking: req.Thinking, OutputConfig: req.OutputConfig}
		if err := dto.NormalizeClaudeAdaptiveThinking(reqModel, ""); err != nil {
			return nil, err
		}
		model = reqModel.Model
		if dto.ClaudeRequiresAdaptiveThinking(info.UpstreamModelName) {
			info.UpstreamModelName = model
		}
	case *dto.GeneralOpenAIRequest, dto.GeneralOpenAIRequest, *dto.OpenAIResponsesRequest, dto.OpenAIResponsesRequest:
		// OpenAI-protocol proxy channels keep their protocol; only the confirmed
		// 4.7 sampling rule applies, never inject Claude thinking/output_config.
		return dto.NormalizeClaudeSamplingJSON(body, model)
	default:
		return body, nil
	}
	return dto.NormalizeClaudeAdaptiveJSON(body, model)
}
