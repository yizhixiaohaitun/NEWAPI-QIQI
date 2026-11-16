package model

import (
	"container/list"
	"math"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAdaptiveHealthWindowAndExpiry(t *testing.T) {
	s := adaptiveStore{entries: make(map[adaptiveKey]*list.Element)}
	key := adaptiveKey{1, "original"}
	now := time.Unix(1000, 0)
	for i := 0; i < 4; i++ {
		s.record(key, false, now)
	}
	assert.EqualValues(t, 100, s.percent(key, now))
	s.record(key, false, now)
	assert.EqualValues(t, 5, s.percent(key, now))
	assert.EqualValues(t, 100, s.percent(adaptiveKey{1, "other"}, now))
	assert.EqualValues(t, 5, s.percent(key, now.Add(adaptiveExpiry-time.Second)))
	s.record(key, true, now.Add(time.Second))
	assert.EqualValues(t, 16, s.percent(key, now.Add(time.Second)))
	for i := 0; i < adaptiveWindow; i++ {
		s.record(key, true, now.Add(2*time.Second))
	}
	assert.EqualValues(t, 100, s.percent(key, now.Add(2*time.Second)))
	s.record(key, false, now.Add(3*time.Second))
	assert.EqualValues(t, 100, s.percent(key, now.Add(3*time.Second+adaptiveExpiry)))
}

func TestAdaptiveWeightsAndOverflow(t *testing.T) {
	adaptiveRouting.mu.Lock()
	adaptiveRouting.entries = make(map[adaptiveKey]*list.Element)
	adaptiveRouting.recent.Init()
	adaptiveRouting.mu.Unlock()
	now := time.Unix(1000, 0)
	for i := 0; i < 5; i++ {
		adaptiveRouting.record(adaptiveKey{1, "m"}, false, now)
	}
	cs := []adaptiveCandidate{{1, 100}, {2, 100}, {3, 0}}
	draw := func(n int64) func(*big.Int) *big.Int {
		return func(total *big.Int) *big.Int { assert.Equal(t, big.NewInt(10500), total); return big.NewInt(n) }
	}
	assert.Equal(t, 1, selectAdaptiveCandidateWithDraw(cs, "m", now, draw(499)))
	assert.Equal(t, 2, selectAdaptiveCandidateWithDraw(cs, "m", now, draw(500)))
	assert.Zero(t, selectAdaptiveCandidateWithDraw([]adaptiveCandidate{{3, 0}}, "m", now, draw(0)))
	bigCandidates := []adaptiveCandidate{{1, uint(math.MaxUint)}, {2, uint(math.MaxUint)}}
	assert.Equal(t, 2, selectAdaptiveCandidateWithDraw(bigCandidates, "other", now, func(total *big.Int) *big.Int { return new(big.Int).Sub(total, big.NewInt(1)) }))
}

func TestAdaptiveRoutingBothSelectorsAndSwitch(t *testing.T) {
	setting := operation_setting.GetAdaptiveRoutingSetting()
	oldEnabled := setting.Enabled
	oldDB, oldCache := DB, common.MemoryCacheEnabled
	oldGroups, oldChannels, oldConfigs := group2model2channels, channelsIDM, channel2advancedCustomConfig
	t.Cleanup(func() {
		setting.Enabled = oldEnabled
		DB = oldDB
		common.MemoryCacheEnabled = oldCache
		group2model2channels = oldGroups
		channelsIDM = oldChannels
		channel2advancedCustomConfig = oldConfigs
	})
	setting.Enabled = false
	assert.Same(t, setting, config.GlobalConfig.Get("adaptive_routing_setting"))
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Ability{}, &Channel{}))
	DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	initCol()
	p10, p5 := int64(10), int64(5)
	positive, zero := uint(100), uint(0)
	channels := []Channel{{Id: 901, Group: "adaptive", Models: "m", Status: common.ChannelStatusEnabled, Priority: &p10, Weight: &positive}, {Id: 902, Group: "adaptive", Models: "m", Status: common.ChannelStatusEnabled, Priority: &p10, Weight: &zero}, {Id: 903, Group: "adaptive", Models: "m", Status: common.ChannelStatusEnabled, Priority: &p5, Weight: &positive}}
	require.NoError(t, db.Create(&channels).Error)
	for i := range channels {
		require.NoError(t, channels[i].AddAbilities(nil))
	}
	common.MemoryCacheEnabled = true
	InitChannelCache()
	for i := 0; i < 5; i++ {
		RecordAdaptiveRoutingOutcome(901, "m", false)
	}
	assert.EqualValues(t, 100, adaptiveRouting.percent(adaptiveKey{901, "m"}, time.Now()))
	setting.Enabled = true
	for i := 0; i < 5; i++ {
		RecordAdaptiveRoutingOutcome(901, "m", false)
	}
	assert.EqualValues(t, 5, adaptiveRouting.percent(adaptiveKey{901, "m"}, time.Now()))
	for _, cache := range []bool{true, false} {
		common.MemoryCacheEnabled = cache
		c, e := GetRandomSatisfiedChannel("adaptive", "m", 0, "")
		require.NoError(t, e)
		require.NotNil(t, c)
		assert.Equal(t, 901, c.Id)
		c, e = GetRandomSatisfiedChannel("adaptive", "m", 1, "")
		require.NoError(t, e)
		require.NotNil(t, c)
		assert.Equal(t, 903, c.Id)
		c, e = GetRandomSatisfiedChannelExcluding("adaptive", "m", 0, "", map[int]bool{901: true})
		require.NoError(t, e)
		assert.Nil(t, c)
		setting.Enabled = false
		c, e = GetRandomSatisfiedChannelExcluding("adaptive", "m", 0, "", map[int]bool{901: true})
		require.NoError(t, e)
		require.NotNil(t, c)
		assert.Equal(t, 902, c.Id)
		setting.Enabled = true
	}
}

func TestAdaptiveConcurrentRecording(t *testing.T) {
	s := adaptiveStore{entries: make(map[adaptiveKey]*list.Element)}
	now := time.Unix(1000, 0)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				s.record(adaptiveKey{i, "m"}, j%2 == 0, now)
				s.percent(adaptiveKey{i, "m"}, now)
			}
		}(i)
	}
	wg.Wait()
	for i := 0; i < 8; i++ {
		assert.EqualValues(t, 50, s.percent(adaptiveKey{i, "m"}, now))
	}
}

func TestAdaptiveBoundedStore(t *testing.T) {
	s := adaptiveStore{entries: make(map[adaptiveKey]*list.Element)}
	now := time.Unix(1000, 0)
	for i := 0; i < adaptiveMaxEntries; i++ {
		s.record(adaptiveKey{i, "m"}, false, now)
	}
	for i := 0; i < 5; i++ {
		s.record(adaptiveKey{0, "m"}, false, now)
	}
	s.record(adaptiveKey{adaptiveMaxEntries, "m"}, false, now)
	assert.Len(t, s.entries, adaptiveMaxEntries)
	assert.EqualValues(t, 5, s.percent(adaptiveKey{0, "m"}, now))
	_, ok := s.entries[adaptiveKey{1, "m"}]
	assert.False(t, ok)
}
