package model

import (
	"container/list"
	"math/big"
	"math/rand"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/setting/operation_setting"
)

const (
	adaptiveWindow         = 20
	adaptiveMinimumSamples = 5
	adaptiveMaxEntries     = 10000
	adaptiveMaxModelBytes  = 255
	adaptiveExpiry         = 24 * time.Hour
)

type adaptiveKey struct {
	channel int
	model   string
}
type adaptiveHealth struct {
	key                    adaptiveKey
	outcomes               [adaptiveWindow]bool
	count, next, successes int
	last                   time.Time
}
type adaptiveStore struct {
	mu      sync.Mutex
	entries map[adaptiveKey]*list.Element
	recent  list.List
}

var adaptiveRouting = adaptiveStore{entries: make(map[adaptiveKey]*list.Element)}

// RecordAdaptiveRoutingOutcome records only attributable upstream outcomes.
// The caller must not report client errors as upstream failures.
func RecordAdaptiveRoutingOutcome(channelID int, modelName string, success bool) {
	if !operation_setting.GetAdaptiveRoutingSetting().Enabled || channelID <= 0 || len(modelName) == 0 || len(modelName) > adaptiveMaxModelBytes {
		return
	}
	adaptiveRouting.record(adaptiveKey{channelID, modelName}, success, time.Now())
}

func (s *adaptiveStore) record(key adaptiveKey, success bool, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.entries[key]
	if e != nil && now.Sub(e.Value.(*adaptiveHealth).last) >= adaptiveExpiry {
		s.recent.Remove(e)
		delete(s.entries, key)
		e = nil
	}
	if e == nil {
		if len(s.entries) >= adaptiveMaxEntries {
			oldest := s.recent.Back()
			delete(s.entries, oldest.Value.(*adaptiveHealth).key)
			s.recent.Remove(oldest)
		}
		e = s.recent.PushFront(&adaptiveHealth{key: key})
		s.entries[key] = e
	}
	h := e.Value.(*adaptiveHealth)
	if h.count == adaptiveWindow {
		if h.outcomes[h.next] {
			h.successes--
		}
	} else {
		h.count++
	}
	h.outcomes[h.next] = success
	if success {
		h.successes++
	}
	h.next = (h.next + 1) % adaptiveWindow
	h.last = now
	s.recent.MoveToFront(e)
}

// A state expires after 24 hours without outcomes; it is then treated as cold.
// Before expiry, time alone never improves a score. Memory pressure evicts the
// least recently sampled state, which is likewise treated as cold.
func (s *adaptiveStore) percent(key adaptiveKey, now time.Time) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.entries[key]
	if e == nil {
		return 100
	}
	h := e.Value.(*adaptiveHealth)
	if now.Sub(h.last) >= adaptiveExpiry {
		delete(s.entries, key)
		s.recent.Remove(e)
		return 100
	}
	if h.count < adaptiveMinimumSamples {
		return 100
	}
	percent := int64(h.successes * 100 / h.count)
	if percent < 5 {
		return 5
	}
	return percent
}

type adaptiveCandidate struct {
	id     int
	weight uint
}

// Uses arbitrary precision so multiplication and summing cannot overflow, even
// for externally supplied maximum weights. A zero manual weight stays zero.
func selectAdaptiveCandidate(candidates []adaptiveCandidate, model string, now time.Time) int {
	return selectAdaptiveCandidateWithDraw(candidates, model, now, func(total *big.Int) *big.Int {
		return new(big.Int).Rand(rand.New(rand.NewSource(rand.Int63())), total)
	})
}

func selectAdaptiveCandidateWithDraw(candidates []adaptiveCandidate, model string, now time.Time, drawForTotal func(*big.Int) *big.Int) int {
	weights := make([]*big.Int, len(candidates))
	total := new(big.Int)
	for i, c := range candidates {
		weights[i] = new(big.Int).SetUint64(uint64(c.weight))
		weights[i].Mul(weights[i], big.NewInt(adaptiveRouting.percent(adaptiveKey{c.id, model}, now)))
		total.Add(total, weights[i])
	}
	if total.Sign() == 0 {
		return 0
	}
	draw := drawForTotal(total)
	for i, w := range weights {
		if draw.Cmp(w) < 0 {
			return candidates[i].id
		}
		draw.Sub(draw, w)
	}
	return 0
}
