package common

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnmarshalUseNumber(t *testing.T) {
	input := []byte(`{"values":[9007199254740993,1.234567890123456789,-0,1.2300e+10],"text":"ok"}`)
	var decoded interface{}
	require.NoError(t, UnmarshalUseNumber(input, &decoded))
	object := decoded.(map[string]interface{})
	values := object["values"].([]interface{})
	assert.Equal(t, []interface{}{json.Number("9007199254740993"), json.Number("1.234567890123456789"), json.Number("-0"), json.Number("1.2300e+10")}, values)
	roundTrip, err := Marshal(decoded)
	require.NoError(t, err)
	assert.JSONEq(t, `{"text":"ok","values":[9007199254740993,1.234567890123456789,-0,1.2300e+10]}`, string(roundTrip))
	assert.Contains(t, string(roundTrip), `"values":[9007199254740993,1.234567890123456789,-0,1.2300e+10]`)

	var ordinary interface{}
	require.NoError(t, Unmarshal([]byte(`1`), &ordinary))
	assert.IsType(t, float64(0), ordinary)
}

func TestUnmarshalUseNumberRejectsTrailingAndMalformedJSON(t *testing.T) {
	for _, input := range []string{`1 2`, `{"x":1} trailing`, `{"x":`, ``} {
		t.Run(input, func(t *testing.T) {
			decoded := interface{}("unchanged")
			require.Error(t, UnmarshalUseNumber([]byte(input), &decoded))
			assert.Equal(t, "unchanged", decoded)
		})
	}
}
