package model

import (
	"container/list"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Check every ticket rather than relying on random samples: equal configured
// weights must move traffic toward B after A fails, preserve probes for A, and
// converge back to the configured split after A completes successful requests.
func TestAdaptiveDistributionFailureAndRecovery(t *testing.T) {
	adaptiveRouting.mu.Lock()
	oldEntries := adaptiveRouting.entries
	oldRecent := adaptiveRouting.recent
	adaptiveRouting.entries = make(map[adaptiveKey]*list.Element)
	adaptiveRouting.recent.Init()
	adaptiveRouting.mu.Unlock()
	t.Cleanup(func() {
		adaptiveRouting.mu.Lock()
		adaptiveRouting.entries, adaptiveRouting.recent = oldEntries, oldRecent
		adaptiveRouting.mu.Unlock()
	})
	now := time.Unix(5000, 0)
	candidates := []adaptiveCandidate{{id: 701, weight: 1}, {id: 702, weight: 1}}
	counts := func() map[int]int {
		result := map[int]int{}
		var total int64
		selectAdaptiveCandidateWithDraw(candidates, "m", now, func(n *big.Int) *big.Int { total = n.Int64(); return big.NewInt(0) })
		require.Positive(t, total)
		for ticket := int64(0); ticket < total; ticket++ {
			id := selectAdaptiveCandidateWithDraw(candidates, "m", now, func(_ *big.Int) *big.Int { return big.NewInt(ticket) })
			result[id]++
		}
		return result
	}
	assert.Equal(t, map[int]int{701: 100, 702: 100}, counts())
	for i := 0; i < 20; i++ {
		adaptiveRouting.record(adaptiveKey{701, "m"}, false, now)
		adaptiveRouting.record(adaptiveKey{702, "m"}, true, now)
	}
	assert.Equal(t, map[int]int{701: 5, 702: 100}, counts(), "unhealthy channel keeps probe tickets, not a guaranteed five percent share")
	for i := 0; i < 10; i++ {
		adaptiveRouting.record(adaptiveKey{701, "m"}, true, now)
	}
	assert.Equal(t, map[int]int{701: 50, 702: 100}, counts())
	for i := 0; i < 10; i++ {
		adaptiveRouting.record(adaptiveKey{701, "m"}, true, now)
	}
	assert.Equal(t, map[int]int{701: 100, 702: 100}, counts())
}
