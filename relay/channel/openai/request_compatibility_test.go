package openai

import (
	"encoding/json"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"testing"
)

func TestRequestCompatibilityOpenRouterClaudeSuffixOnly(t *testing.T) {
	for _, on := range []bool{false, true} {
		for _, name := range []string{"anthropic/claude-opus-4-7-thinking", "deepseek/deepseek-r1-thinking"} {
			t.Run(name+common.Interface2String(on), func(t *testing.T) {
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				common.SetContextKey(c, constant.ContextKeyTokenRequestCompatibilityEnabled, &on)
				info := &relaycommon.RelayInfo{RequestCompatibilityEnabled: &on, ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenRouter, UpstreamModelName: name, ChannelSetting: dto.ChannelSettings{}}, OriginModelName: name}
				req := &dto.GeneralOpenAIRequest{Model: name, Messages: []dto.Message{{Role: "user", Content: "Hi"}}}
				a := Adaptor{}
				a.Init(info)
				_, err := a.ConvertOpenAIRequest(c, info, req)
				require.NoError(t, err)
				if !on && name == "anthropic/claude-opus-4-7-thinking" {
					require.Equal(t, name, req.Model)
					require.Empty(t, req.Reasoning)
				} else {
					require.NotEqual(t, name, req.Model)
					require.JSONEq(t, `{"enabled":true}`, string(req.Reasoning))
				}
			})
		}
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		common.SetContextKey(c, constant.ContextKeyTokenRequestCompatibilityEnabled, &on)
		info := &relaycommon.RelayInfo{RequestCompatibilityEnabled: &on, ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenRouter, UpstreamModelName: "anthropic/claude-opus-4-7"}}
		req := &dto.GeneralOpenAIRequest{Model: info.UpstreamModelName, ReasoningEffort: "high", THINKING: json.RawMessage(`{"type":"enabled","budget_tokens":4096}`)}
		a := Adaptor{}
		a.Init(info)
		_, err := a.ConvertOpenAIRequest(c, info, req)
		require.NoError(t, err)
		require.JSONEq(t, `{"enabled":true,"max_tokens":4096}`, string(req.Reasoning), "explicit reasoning protocol mapping must remain when off")
	}
}

func TestRequestCompatibilityOpenAIClaudeSampling(t *testing.T) {
	for _, on := range []bool{false, true} {
		for _, suffix := range []string{"", "-high", "-thinking"} {
			t.Run(suffix+common.Interface2String(on), func(t *testing.T) {
				name := "claude-opus-4-7" + suffix
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				common.SetContextKey(c, constant.ContextKeyTokenRequestCompatibilityEnabled, &on)
				info := &relaycommon.RelayInfo{RequestCompatibilityEnabled: &on, ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenAI, UpstreamModelName: name}, OriginModelName: name}
				a := Adaptor{}
				a.Init(info)
				r := &dto.GeneralOpenAIRequest{Model: name, Temperature: common.GetPointer(.96), TopP: common.GetPointer(0.0), TopK: common.GetPointer(7), FrequencyPenalty: common.GetPointer(.78), PresencePenalty: common.GetPointer(0.0), Messages: []dto.Message{{Role: "user", Content: "Hi"}}}
				_, err := a.ConvertOpenAIRequest(c, info, r)
				require.NoError(t, err)
				value, err := a.ConvertOpenAIResponsesRequest(c, info, dto.OpenAIResponsesRequest{Model: name, Temperature: common.GetPointer(.96), TopP: common.GetPointer(0.0), Input: json.RawMessage(`"Hi"`)})
				require.NoError(t, err)
				responses := value.(dto.OpenAIResponsesRequest)
				if on {
					require.Nil(t, r.Temperature)
					require.Nil(t, r.TopP)
					require.Nil(t, r.TopK)
					require.Nil(t, responses.Temperature)
					require.Nil(t, responses.TopP)
					require.Equal(t, "claude-opus-4-7", r.Model)
					require.Equal(t, "claude-opus-4-7", responses.Model)
				} else {
					require.Equal(t, .96, *r.Temperature)
					require.Equal(t, 0.0, *r.TopP)
					require.Equal(t, 7, *r.TopK)
					require.Equal(t, .96, *responses.Temperature)
					require.Equal(t, 0.0, *responses.TopP)
					require.Equal(t, name, r.Model)
					require.Equal(t, name, responses.Model)
					require.Nil(t, responses.Reasoning)
				}
				require.Equal(t, .78, *r.FrequencyPenalty)
				require.Equal(t, 0.0, *r.PresencePenalty)
			})
		}
	}
}

func TestRequestCompatibilityResponsesNilRelayInfo(t *testing.T) {
	a := Adaptor{}
	value, err := a.ConvertOpenAIResponsesRequest(nil, nil, dto.OpenAIResponsesRequest{Model: "claude-opus-4-7", Input: json.RawMessage(`"Hi"`), Temperature: common.GetPointer(.96), TopP: common.GetPointer(0.0)})
	require.NoError(t, err)
	r := value.(dto.OpenAIResponsesRequest)
	require.Nil(t, r.Temperature)
	require.Nil(t, r.TopP)
}

func TestRequestCompatibilityResponsesPreservesNonClaudeReasoning(t *testing.T) {
	off := false
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(c, constant.ContextKeyTokenRequestCompatibilityEnabled, &off)
	info := &relaycommon.RelayInfo{RequestCompatibilityEnabled: &off}
	a := Adaptor{}
	value, err := a.ConvertOpenAIResponsesRequest(c, info, dto.OpenAIResponsesRequest{Model: "gpt-5-high", Input: json.RawMessage(`"Hi"`)})
	require.NoError(t, err)
	r := value.(dto.OpenAIResponsesRequest)
	require.Equal(t, "gpt-5", r.Model)
	require.Equal(t, "high", r.Reasoning.Effort)
}
