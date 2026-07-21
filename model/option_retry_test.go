package model

import (
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRetryTimes(t *testing.T) {
	overflow := strconv.Itoa(int(^uint(0)>>1)) + "0"
	tests := []struct {
		name    string
		value   string
		want    int
		wantErr bool
	}{
		{name: "zero", value: "0", want: 0},
		{name: "above former limit", value: "11", want: 11},
		{name: "large non-negative integer", value: "1000000", want: 1000000},
		{name: "surrounding whitespace", value: " 12 ", want: 12},
		{name: "negative", value: "-1", wantErr: true},
		{name: "fraction", value: "1.5", wantErr: true},
		{name: "not a number", value: "unlimited", wantErr: true},
		{name: "empty", value: "", wantErr: true},
		{name: "integer overflow", value: overflow, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ParseRetryTimes(test.value)
			if test.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.want, got)
		})
	}
}

func TestUpdateOptionMapRejectsInvalidRetryTimesWithoutChangingRuntime(t *testing.T) {
	originalRetryTimes := common.RetryTimes
	common.OptionMapRWMutex.Lock()
	originalOptionMap := common.OptionMap
	common.OptionMap = map[string]string{"RetryTimes": "11"}
	common.OptionMapRWMutex.Unlock()
	common.RetryTimes = 11
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = originalOptionMap
		common.OptionMapRWMutex.Unlock()
		common.RetryTimes = originalRetryTimes
	})

	require.Error(t, updateOptionMap("RetryTimes", "-1"))
	assert.Equal(t, 11, common.RetryTimes)
	common.OptionMapRWMutex.Lock()
	stored := common.OptionMap["RetryTimes"]
	common.OptionMapRWMutex.Unlock()
	assert.Equal(t, "11", stored)
}
