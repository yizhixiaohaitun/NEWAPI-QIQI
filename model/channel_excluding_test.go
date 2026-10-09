package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestGetChannelExcludingUsesHighestRemainingPriority(t *testing.T) {
	oldDB := DB
	t.Cleanup(func() { DB = oldDB })

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Ability{}, &Channel{}))
	DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	initCol()

	p10, p5, p0, weight := int64(10), int64(5), int64(0), uint(1)
	require.NoError(t, db.Create(&[]Channel{
		{Id: 461, Group: "fixed", Models: "test-model", Status: common.ChannelStatusEnabled, Priority: &p10, Weight: &weight},
		{Id: 464, Group: "fixed", Models: "test-model", Status: common.ChannelStatusEnabled, Priority: &p5, Weight: &weight},
		{Id: 465, Group: "fixed", Models: "test-model", Status: common.ChannelStatusEnabled, Priority: &p0, Weight: &weight},
	}).Error)
	require.NoError(t, db.Create(&[]Ability{
		{Group: "fixed", Model: "test-model", ChannelId: 461, Enabled: true, Priority: &p10, Weight: 1},
		{Group: "fixed", Model: "test-model", ChannelId: 464, Enabled: true, Priority: &p5, Weight: 1},
		{Group: "fixed", Model: "test-model", ChannelId: 465, Enabled: true, Priority: &p0, Weight: 1},
	}).Error)

	channel, err := GetChannelExcluding("fixed", "test-model", 1, "", map[int]bool{461: true})
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 464, channel.Id, "DB selection must restart at the highest still-unfailed priority")

	channel, err = GetChannelExcluding("fixed", "test-model", 2, "", map[int]bool{461: true, 464: true})
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 465, channel.Id)

	channel, err = GetChannelExcluding("fixed", "test-model", 3, "", map[int]bool{461: true, 464: true, 465: true})
	require.NoError(t, err)
	assert.Nil(t, channel)
}
