package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestGetChannelExcludingChoosesHighestRemainingPathCompatiblePriority(t *testing.T) {
	oldDB := DB
	t.Cleanup(func() { DB = oldDB })

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Ability{}, &Channel{}))
	DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	initCol()

	p10, p5, p0, weight := int64(10), int64(5), int64(0), uint(1)
	incompatible := Channel{Id: 462, Type: constant.ChannelTypeAdvancedCustom, Group: "fixed", Models: "test-model", Status: common.ChannelStatusEnabled, Priority: &p10, Weight: &weight}
	incompatible.SetOtherSettings(dto.ChannelOtherSettings{AdvancedCustom: &dto.AdvancedCustomConfig{Routes: []dto.AdvancedCustomRoute{{IncomingPath: "/v1/responses", UpstreamPath: "/v1/responses"}}}})
	require.NoError(t, db.Create(&[]Channel{
		{Id: 461, Group: "fixed", Models: "test-model", Status: common.ChannelStatusEnabled, Priority: &p10, Weight: &weight},
		incompatible,
		{Id: 464, Group: "fixed", Models: "test-model", Status: common.ChannelStatusEnabled, Priority: &p5, Weight: &weight},
		{Id: 465, Group: "fixed", Models: "test-model", Status: common.ChannelStatusEnabled, Priority: &p0, Weight: &weight},
	}).Error)
	require.NoError(t, db.Create(&[]Ability{
		{Group: "fixed", Model: "test-model", ChannelId: 461, Enabled: true, Priority: &p10, Weight: 1},
		{Group: "fixed", Model: "test-model", ChannelId: 462, Enabled: true, Priority: &p10, Weight: 1},
		{Group: "fixed", Model: "test-model", ChannelId: 464, Enabled: true, Priority: &p5, Weight: 1},
		{Group: "fixed", Model: "test-model", ChannelId: 465, Enabled: true, Priority: &p0, Weight: 1},
	}).Error)

	channel, err := GetChannelExcluding("fixed", "test-model", 1, "/v1/messages", map[int]bool{461: true})
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 464, channel.Id, "an incompatible remaining high-tier route must not hide a compatible lower tier")
}
