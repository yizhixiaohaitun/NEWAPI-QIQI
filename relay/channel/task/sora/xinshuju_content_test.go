package sora

import (
	"io"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/require"
)

func TestNativeXinshujuContentIsPreserved(t *testing.T) {
	body := `{"model":"seedance-2.0","content":[{"type":"text","text":"keep actions"},{"type":"video_url","video_url":{"url":"https://example.com/ref.mp4"},"role":"reference_video"},{"type":"audio_url","audio_url":{"url":"https://example.com/ref.mp3"},"role":"reference_audio"}],"generate_audio":true,"ratio":"16:9","duration":8,"watermark":false,"resolution":"480p"}`
	context, info := newJSONTaskContextForModel(t, body, "47:seedance-2.0")
	info.ChannelSetting.VideoUpstreamProtocol = dto.VideoUpstreamProtocolXinshujuContent
	reader, err := (&TaskAdaptor{}).BuildRequestBody(context, info)
	require.NoError(t, err)
	forwarded, err := io.ReadAll(reader)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, common.Unmarshal(forwarded, &payload))
	content, ok := payload["content"].([]any)
	require.True(t, ok)
	t.Logf("forwarded=%s", forwarded)
	require.Len(t, content, 3)
	textItem := content[0].(map[string]any)
	videoItem := content[1].(map[string]any)
	audioItem := content[2].(map[string]any)
	require.Equal(t, "keep actions", textItem["text"])
	require.Equal(t, "reference_video", videoItem["role"])
	require.Equal(t, "reference_audio", audioItem["role"])
}
