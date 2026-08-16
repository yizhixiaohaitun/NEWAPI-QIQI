package relay

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

const (
	seedanceAssetUploadPath = "/asset/seedance2/assetUpload"
	seedanceAssetDetailPath = "/asset/seedance2/assetDetail"
)

type seedanceAssetUploadRequest struct {
	Model     string `json:"model"`
	AssetType string `json:"asset_type"`
	Type      string `json:"type"`
	URL       string `json:"url"`
	Name      string `json:"name,omitempty"`
}

type seedanceAssetUpstreamRequest struct {
	AssetType string `json:"assetType"`
	URL       string `json:"url"`
	Name      string `json:"name,omitempty"`
}

type seedanceAssetDetailRequest struct {
	AssetID string `json:"assetId"`
}

type seedanceAssetUpstreamResponse struct {
	AssetID      string  `json:"assetId"`
	AssetType    string  `json:"assetType"`
	URL          string  `json:"url"`
	Status       string  `json:"status"`
	ErrorMessage *string `json:"errorMessage"`
	Name         string  `json:"name"`
}

type seedanceAssetResponse struct {
	ID        string  `json:"id"`
	Object    string  `json:"object"`
	URI       string  `json:"uri"`
	AssetType string  `json:"asset_type"`
	URL       string  `json:"url"`
	Status    string  `json:"status"`
	Name      string  `json:"name,omitempty"`
	Error     *string `json:"error"`
}

func normalizeSeedanceAssetType(value string) (string, string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "image":
		return "Image", "image", nil
	case "video":
		return "Video", "video", nil
	case "audio":
		return "Audio", "audio", nil
	default:
		return "", "", fmt.Errorf("asset_type must be image, video, or audio")
	}
}

func validateSeedanceAssetURL(value string) error {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(value))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("url must be an absolute HTTP or HTTPS URL accessible by the upstream service")
	}
	return nil
}

func relaySeedanceAssetRequest(info *relaycommon.RelayInfo, path string, payload any) (*http.Response, error) {
	body, err := common.Marshal(payload)
	if err != nil {
		return nil, err
	}
	requestURL := strings.TrimRight(info.ChannelBaseUrl, "/") + path
	req, err := http.NewRequestWithContext(info.UpstreamContext, http.MethodPost, requestURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+info.ApiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	client, err := service.GetHttpClientWithProxy(info.ChannelSetting.Proxy)
	if err != nil {
		return nil, err
	}
	return client.Do(req)
}

func parseSeedanceAssetResponse(resp *http.Response) (*seedanceAssetResponse, *dto.TaskError) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusBadGateway)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, service.TaskErrorWrapper(fmt.Errorf("upstream returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body))), "bad_response_status_code", resp.StatusCode)
	}
	var upstream seedanceAssetUpstreamResponse
	if err := common.Unmarshal(body, &upstream); err != nil {
		return nil, service.TaskErrorWrapper(fmt.Errorf("invalid upstream asset response: %w", err), "invalid_response", http.StatusBadGateway)
	}
	if strings.TrimSpace(upstream.AssetID) == "" {
		return nil, service.TaskErrorWrapper(fmt.Errorf("upstream asset response is missing assetId"), "invalid_response", http.StatusBadGateway)
	}
	_, normalizedType, err := normalizeSeedanceAssetType(upstream.AssetType)
	if err != nil {
		normalizedType = strings.ToLower(strings.TrimSpace(upstream.AssetType))
	}
	return &seedanceAssetResponse{
		ID:        upstream.AssetID,
		Object:    "video.asset",
		URI:       "assetId://" + upstream.AssetID,
		AssetType: normalizedType,
		URL:       upstream.URL,
		Status:    strings.ToLower(strings.TrimSpace(upstream.Status)),
		Name:      upstream.Name,
		Error:     upstream.ErrorMessage,
	}, nil
}

// RelaySeedanceAsset translates the OpenAI-style /v1/video/assets contract to
// the provider's Seedance assetUpload and assetDetail JSON endpoints.
func RelaySeedanceAsset(c *gin.Context, info *relaycommon.RelayInfo) (*seedanceAssetResponse, *dto.TaskError) {
	info.InitChannelMeta(c)
	if strings.TrimSpace(info.ChannelBaseUrl) == "" {
		return nil, service.TaskErrorWrapperLocal(fmt.Errorf("channel base URL is required for Seedance assets"), "invalid_channel_config", http.StatusInternalServerError)
	}

	var resp *http.Response
	var err error
	if c.Request.Method == http.MethodPost {
		var input seedanceAssetUploadRequest
		if err := common.UnmarshalBodyReusable(c, &input); err != nil {
			return nil, service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
		}
		requestedType := input.AssetType
		if strings.TrimSpace(requestedType) == "" {
			requestedType = input.Type
		}
		upstreamType, _, typeErr := normalizeSeedanceAssetType(requestedType)
		if typeErr != nil {
			return nil, service.TaskErrorWrapperLocal(typeErr, "invalid_request", http.StatusBadRequest)
		}
		input.URL = strings.TrimSpace(input.URL)
		if err := validateSeedanceAssetURL(input.URL); err != nil {
			return nil, service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
		}
		resp, err = relaySeedanceAssetRequest(info, seedanceAssetUploadPath, seedanceAssetUpstreamRequest{
			AssetType: upstreamType,
			URL:       input.URL,
			Name:      input.Name,
		})
	} else if c.Request.Method == http.MethodGet {
		assetID := strings.TrimSpace(c.Param("asset_id"))
		assetID = strings.TrimPrefix(assetID, "assetId://")
		if assetID == "" {
			return nil, service.TaskErrorWrapperLocal(fmt.Errorf("asset_id is required"), "invalid_request", http.StatusBadRequest)
		}
		resp, err = relaySeedanceAssetRequest(info, seedanceAssetDetailPath, seedanceAssetDetailRequest{AssetID: assetID})
	} else {
		return nil, service.TaskErrorWrapperLocal(fmt.Errorf("method not allowed"), "method_not_allowed", http.StatusMethodNotAllowed)
	}
	if err != nil {
		return nil, service.TaskErrorWrapper(err, "do_request_failed", http.StatusBadGateway)
	}
	defer resp.Body.Close()
	return parseSeedanceAssetResponse(resp)
}
