package resource

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
)

func mysqlSourceTypeAttribute(t *testing.T) resourceschema.StringAttribute {
	t.Helper()

	schemaResponse := &resource.SchemaResponse{}
	(&ClickPipeResource{}).Schema(t.Context(), resource.SchemaRequest{}, schemaResponse)
	require.False(t, schemaResponse.Diagnostics.HasError())

	source, ok := schemaResponse.Schema.Attributes["source"].(resourceschema.SingleNestedAttribute)
	require.True(t, ok)
	mysql, ok := source.Attributes["mysql"].(resourceschema.SingleNestedAttribute)
	require.True(t, ok)
	sourceType, ok := mysql.Attributes["type"].(resourceschema.StringAttribute)
	require.True(t, ok)
	return sourceType
}

func validateMySQLSourceType(ctx context.Context, validators []validator.String, value string) diag.Diagnostics {
	var diags diag.Diagnostics
	for _, v := range validators {
		resp := &validator.StringResponse{}
		v.ValidateString(ctx, validator.StringRequest{ConfigValue: types.StringValue(value)}, resp)
		diags.Append(resp.Diagnostics...)
	}
	return diags
}

func TestClickPipeResource_MySQLSourceTypeSchema(t *testing.T) {
	sourceType := mysqlSourceTypeAttribute(t)

	assert.True(t, sourceType.Optional)
	assert.True(t, sourceType.Computed)
	assert.Contains(t, sourceType.MarkdownDescription, "`cloudsqlmysql`")

	t.Run("accepts every MySQL source type", func(t *testing.T) {
		for _, name := range api.ClickPipeMySQLSourceTypes {
			diags := validateMySQLSourceType(t.Context(), sourceType.Validators, name)
			assert.False(t, diags.HasError(), "%s should be accepted: %v", name, diags)
		}
	})

	t.Run("rejects other source types", func(t *testing.T) {
		for _, name := range []string{"cloudsqlpostgres", "postgres", "cloudsql", ""} {
			diags := validateMySQLSourceType(t.Context(), sourceType.Validators, name)
			assert.True(t, diags.HasError(), "%q should be rejected", name)
		}
	})
}

func TestExtractSourceFromPlan_MySQL_SourceType(t *testing.T) {
	r := &ClickPipeResource{}

	for _, sourceType := range api.ClickPipeMySQLSourceTypes {
		t.Run(sourceType, func(t *testing.T) {
			plan := buildMySQLPlanWithType(sourceType, types.Int64Null())

			diagnostics := diag.Diagnostics{}
			source := r.extractSourceFromPlan(t.Context(), &diagnostics, plan, nil, false)

			assert.False(t, diagnostics.HasError(), "expected no errors, got: %v", diagnostics.Errors())
			require.NotNil(t, source.MySQL)
			assert.Equal(t, sourceType, source.MySQL.Type)
		})
	}
}
