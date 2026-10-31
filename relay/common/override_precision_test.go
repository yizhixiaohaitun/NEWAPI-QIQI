package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestApplyParamOverridePruneObjectsPreservesGeminiNumbers(t *testing.T) {
	input := []byte(`{
		"contents":[{"role":"model","parts":[
			{"thoughtSignature":"signature-1","functionCall":{"id":"call-1","name":"lookup","args":{"large":9007199254740993,"precise":1.234567890123456789,"nested":[-0,1.2300e+10]}}},
			{"type":"discard","text":"remove me"},
			{"thoughtSignature":"signature-2","functionCall":{"id":"call-2","name":"next","args":{"large":9007199254740993}}}
		]}],
		"generationConfig":{"temperature":0.7},
		"outside":9007199254740993
	}`)

	for _, path := range []string{"", "contents", "contents.0", "contents.0.parts", "outside"} {
		t.Run("path="+path, func(t *testing.T) {
			override := map[string]interface{}{"operations": []interface{}{
				map[string]interface{}{"mode": "prune_objects", "path": path, "value": "discard"},
				map[string]interface{}{"mode": "set", "path": "generationConfig.temperature", "value": 0.25},
			}}
			out, err := ApplyParamOverride(input, override, nil)
			require.NoError(t, err)
			require.True(t, gjson.ValidBytes(out))
			assert.Equal(t, "9007199254740993", gjson.GetBytes(out, "contents.0.parts.0.functionCall.args.large").Raw)
			assert.Equal(t, "1.234567890123456789", gjson.GetBytes(out, "contents.0.parts.0.functionCall.args.precise").Raw)
			assert.Equal(t, "-0", gjson.GetBytes(out, "contents.0.parts.0.functionCall.args.nested.0").Raw)
			assert.Equal(t, "1.2300e+10", gjson.GetBytes(out, "contents.0.parts.0.functionCall.args.nested.1").Raw)
			assert.Equal(t, "9007199254740993", gjson.GetBytes(out, "outside").Raw)
			assert.Equal(t, "signature-1", gjson.GetBytes(out, "contents.0.parts.0.thoughtSignature").String())
			assert.Equal(t, "0.25", gjson.GetBytes(out, "generationConfig.temperature").Raw)
			parts := gjson.GetBytes(out, "contents.0.parts").Array()
			if path == "outside" {
				require.Len(t, parts, 3)
				assert.Equal(t, "discard", parts[1].Get("type").String())
				assert.Equal(t, "signature-2", parts[2].Get("thoughtSignature").String())
			} else {
				require.Len(t, parts, 2)
				assert.Equal(t, "signature-2", parts[1].Get("thoughtSignature").String())
				assert.Equal(t, "call-2", parts[1].Get("functionCall.id").String())
				assert.Equal(t, "9007199254740993", parts[1].Get("functionCall.args.large").Raw)
			}
		})
	}
}

func TestApplyParamOverridePruneObjectsConditionAndNonRecursive(t *testing.T) {
	input := []byte(`{"items":[{"type":"discard","rank":2},{"type":"discard","rank":1,"number":1.234567890123456789,"nested":{"type":"discard","rank":2,"number":9007199254740993}}]}`)
	override := map[string]interface{}{"operations": []interface{}{
		map[string]interface{}{"mode": "prune_objects", "path": "items", "value": map[string]interface{}{
			"where": map[string]interface{}{"type": "discard", "rank": 2}, "recursive": false,
		}},
	}}
	out, err := ApplyParamOverride(input, override, nil)
	require.NoError(t, err)
	items := gjson.GetBytes(out, "items").Array()
	require.Len(t, items, 1)
	assert.Equal(t, "1", items[0].Get("rank").Raw)
	assert.Equal(t, "1.234567890123456789", items[0].Get("number").Raw)
	assert.Equal(t, "discard", items[0].Get("nested.type").String())
	assert.Equal(t, "9007199254740993", items[0].Get("nested.number").Raw)
}
