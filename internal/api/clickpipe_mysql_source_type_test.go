package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsClickPipeMySQLMariaDBSourceType_CloudSQLMySQL(t *testing.T) {
	assert.False(t, IsClickPipeMySQLMariaDBSourceType(ClickPipeMySQLSourceTypeCloudSQLMySQL))
}
