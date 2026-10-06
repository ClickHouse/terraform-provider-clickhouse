package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClickPipeMySQLSourceTypes(t *testing.T) {
	assert.Equal(t, "cloudsqlmysql", ClickPipeMySQLSourceTypeCloudSQLMySQL)
	assert.Contains(t, ClickPipeMySQLSourceTypes, ClickPipeMySQLSourceTypeCloudSQLMySQL)

	seen := map[string]bool{}
	for _, sourceType := range ClickPipeMySQLSourceTypes {
		assert.False(t, seen[sourceType], "%s is listed twice", sourceType)
		seen[sourceType] = true
	}
}

func TestIsClickPipeMySQLMariaDBSourceType(t *testing.T) {
	for _, sourceType := range []string{
		ClickPipeMySQLSourceTypeMariaDB,
		ClickPipeMySQLSourceTypeRDSMariaDB,
	} {
		assert.True(t, IsClickPipeMySQLMariaDBSourceType(sourceType), sourceType)
	}

	for _, sourceType := range []string{
		ClickPipeMySQLSourceTypeMySQL,
		ClickPipeMySQLSourceTypeRDSMySQL,
		ClickPipeMySQLSourceTypeAuroraMySQL,
		ClickPipeMySQLSourceTypeCloudSQLMySQL,
		ClickPipeMySQLSourceTypePlanetScaleVites,
	} {
		assert.False(t, IsClickPipeMySQLMariaDBSourceType(sourceType), sourceType)
	}
}
