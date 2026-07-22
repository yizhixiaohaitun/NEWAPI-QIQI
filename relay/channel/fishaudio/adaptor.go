package fishaudio

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

const (
	ChannelName    = "Fish Audio"
	defaultBaseURL = "https://api.fish.audio"
)

type Adaptor struct{}

type ttsRequest struct {
	Text        string `json:"text"`
	ReferenceID string `json:"reference_id,omitempty"`
	Format      string `json:"format,omitempty"`
}

func (a *Adaptor) Init(*relaycommon.RelayInfo) {}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	baseURL := strings.TrimRight(info.ChannelBaseUrl, "/")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return baseURL + "/v1/tts", nil
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, header *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, header)
	header.Set("Content-Type", "application/json")
	header.Set("Authorization", "Bearer "+info.ApiKey)
	header.Set("model", info.UpstreamModelName)
	return nil
}

func (a *Adaptor) ConvertAudioRequest(_ *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	if info.RelayMode != relayconstant.RelayModeAudioSpeech {
		return nil, errors.New("Fish Audio only supports speech synthesis")
	}
	if strings.TrimSpace(request.Input) == "" {
		return nil, errors.New("input must not be empty")
	}
	body, err := common.Marshal(ttsRequest{
		Text: request.Input, ReferenceID: request.Voice, Format: request.ResponseFormat,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal Fish Audio request: %w", err)
	}
	return bytes.NewReader(body), nil
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, body io.Reader) (any, error) {
	url, err := a.GetRequestURL(info)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, url, body)
	if err != nil {
		return nil, fmt.Errorf("create Fish Audio request: %w", err)
	}
	if err = a.SetupRequestHeader(c, &req.Header, info); err != nil {
		return nil, err
	}
	return channel.DoRequest(c, req, info)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (any, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(errors.New("Fish Audio returned an empty response"), types.ErrorCodeEmptyResponse, http.StatusBadGateway)
	}

	reader := bufio.NewReader(resp.Body)
	if _, err := reader.Peek(1); err != nil {
		_ = resp.Body.Close()
		if errors.Is(err, io.EOF) {
			return nil, types.NewOpenAIError(errors.New("Fish Audio returned empty audio"), types.ErrorCodeEmptyResponse, http.StatusBadGateway)
		}
		return nil, types.NewOpenAIError(fmt.Errorf("read Fish Audio response: %w", err), types.ErrorCodeReadResponseBodyFailed, http.StatusBadGateway)
	}
	resp.Body = struct {
		io.Reader
		io.Closer
	}{Reader: reader, Closer: resp.Body}

	contentType := safeContentType(resp.Header.Get("Content-Type"), responseFormat(info))
	resp.Header = make(http.Header)
	resp.Header.Set("Content-Type", contentType)
	return openai.OpenaiTTSHandler(c, resp, info), nil
}

func safeContentType(value, format string) string {
	mediaType, _, err := mime.ParseMediaType(value)
	if err == nil && (strings.HasPrefix(strings.ToLower(mediaType), "audio/") || strings.EqualFold(mediaType, "application/octet-stream")) {
		return mediaType
	}
	switch strings.ToLower(format) {
	case "wav":
		return "audio/wav"
	case "flac":
		return "audio/flac"
	case "opus":
		return "audio/ogg"
	case "pcm":
		return "application/octet-stream"
	default:
		return "audio/mpeg"
	}
}

func responseFormat(info *relaycommon.RelayInfo) string {
	if request, ok := info.Request.(*dto.AudioRequest); ok {
		return request.ResponseFormat
	}
	return ""
}

func (a *Adaptor) GetModelList() []string { return nil }
func (a *Adaptor) GetChannelName() string { return ChannelName }

func (a *Adaptor) ConvertOpenAIRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeneralOpenAIRequest) (any, error) {
	return nil, errors.New("Fish Audio only supports speech synthesis")
}
func (a *Adaptor) ConvertRerankRequest(*gin.Context, int, dto.RerankRequest) (any, error) {
	return nil, errors.New("Fish Audio does not support rerank")
}
func (a *Adaptor) ConvertEmbeddingRequest(*gin.Context, *relaycommon.RelayInfo, dto.EmbeddingRequest) (any, error) {
	return nil, errors.New("Fish Audio does not support embeddings")
}
func (a *Adaptor) ConvertImageRequest(*gin.Context, *relaycommon.RelayInfo, dto.ImageRequest) (any, error) {
	return nil, errors.New("Fish Audio does not support images")
}
func (a *Adaptor) ConvertOpenAIResponsesRequest(*gin.Context, *relaycommon.RelayInfo, dto.OpenAIResponsesRequest) (any, error) {
	return nil, errors.New("Fish Audio does not support responses")
}
func (a *Adaptor) ConvertClaudeRequest(*gin.Context, *relaycommon.RelayInfo, *dto.ClaudeRequest) (any, error) {
	return nil, errors.New("Fish Audio does not support Claude requests")
}
func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	return nil, errors.New("Fish Audio does not support Gemini requests")
}
