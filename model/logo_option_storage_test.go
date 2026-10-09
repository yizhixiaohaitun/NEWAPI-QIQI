package model

import (
	"strings"
	"sync"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func TestLogoOptionDataURLStorageCompatibility(t *testing.T) {
	value := "data:image/png;base64," + strings.Repeat("A", 349500)
	db, err := gorm.Open(sqlite.Open("file:logo-option-storage?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Option{}))
	require.NoError(t, db.Create(&Option{Key: "Logo", Value: value}).Error)

	var stored Option
	require.NoError(t, db.First(&stored, "key = ?", "Logo").Error)
	require.Equal(t, value, stored.Value)
}

func TestLogoOptionUsesUnboundedTextOnServerDatabases(t *testing.T) {
	parsed, err := schema.Parse(&Option{}, &sync.Map{}, schema.NamingStrategy{})
	require.NoError(t, err)
	valueField := parsed.LookUpField("Value")
	require.NotNil(t, valueField)
	require.Equal(t, "longtext", mysql.New(mysql.Config{}).DataTypeOf(valueField))
	require.Equal(t, "text", postgres.New(postgres.Config{}).DataTypeOf(valueField))
}
