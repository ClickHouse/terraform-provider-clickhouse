package resource

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClickPipeResource_MySQLSourceTypeAcceptsCloudSQLMySQL(t *testing.T) {
	schemaResponse := &resource.SchemaResponse{}
	(&ClickPipeResource{}).Schema(t.Context(), resource.SchemaRequest{}, schemaResponse)
	require.False(t, schemaResponse.Diagnostics.HasError())

	source := schemaResponse.Schema.Attributes["source"].(resourceschema.SingleNestedAttribute)
	mysql := source.Attributes["mysql"].(resourceschema.SingleNestedAttribute)
	sourceType := mysql.Attributes["type"].(resourceschema.StringAttribute)

	for _, v := range sourceType.Validators {
		resp := &validator.StringResponse{}
		v.ValidateString(t.Context(), validator.StringRequest{ConfigValue: types.StringValue("cloudsqlmysql")}, resp)
		assert.False(t, resp.Diagnostics.HasError(), "%v", resp.Diagnostics)
	}
}
