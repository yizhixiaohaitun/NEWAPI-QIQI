package operation_setting

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestRemoveModelKeywords(t *testing.T) {
	original, enabled := AutomaticRemoveModelKeywordsToString(), AutomaticRemoveModelEnabled()
	t.Cleanup(func() { AutomaticRemoveModelKeywordsFromString(original); SetAutomaticRemoveModelEnabled(enabled) })
	SetAutomaticRemoveModelEnabled(true)
	AutomaticRemoveModelKeywordsFromString("This Model is marked by provider as Legacy")
	require.True(t, ShouldRemoveModel("404: This Model is marked by provider as Legacy and inactive"))
	for _, s := range []string{"404", "Access denied", "quota exceeded"} {
		require.False(t, ShouldRemoveModel(s))
	}
	AutomaticRemoveModelKeywordsFromString("one\ntwo")
	require.True(t, ShouldRemoveModel("TWO"))
	SetAutomaticRemoveModelEnabled(false)
	require.False(t, ShouldRemoveModel("two"))
}
