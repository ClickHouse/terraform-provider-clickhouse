package resource

import (
	"context"
	"encoding/json"
	"math"
	"testing"

	"github.com/gojuno/minimock/v3"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/stretchr/testify/assert"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickhouse/resource/models"
)

// buildMySQLServerIDPlan returns a minimal MySQL CDC plan/state with the given
// server_id value (known, null, or unknown).
func uint32Ptr(v uint32) *uint32 { return &v }

func buildMySQLServerIDPlan(serverID types.Int64) models.ClickPipeResourceModel {
	settingsModel := models.ClickPipeMySQLSettingsModel{
		ReplicationMode: types.StringValue(api.ClickPipeReplicationModeCDC),
	}
	mysqlAttrs := map[string]attr.Value{
		"type":                   types.StringValue("mysql"),
		"host":                   types.StringValue("mysql.example.com"),
		"port":                   types.Int64Value(3306),
		"authentication":         types.StringValue(clickPipeAuthBasic),
		"iam_role":               types.StringNull(),
		"tls_host":               types.StringNull(),
		"ca_certificate":         types.StringNull(),
		"disable_tls":            types.BoolNull(),
		"skip_cert_verification": types.BoolNull(),
		"server_id":              serverID,
		"credentials":            dbSourceCredentials(),
		"settings":               settingsModel.ObjectValue(),
		"table_mappings":         types.SetValueMust(models.ClickPipeMySQLTableMappingModel{}.ObjectType(), []attr.Value{}),
		"ssh_key_resource_id":    types.StringNull(),
	}
	sourceModel := dbSourceModels(
		types.ObjectNull(models.ClickPipePostgresSourceModel{}.ObjectType().AttrTypes),
		types.ObjectValueMust(models.ClickPipeMySQLSourceModel{}.ObjectType().AttrTypes, mysqlAttrs),
	)
	return models.ClickPipeResourceModel{
		ID:        types.StringValue("test-pipe-id"),
		ServiceID: types.StringValue("service-123"),
		Name:      types.StringValue("test-mysql-server-id"),
		Source:    sourceModel.ObjectValue(),
	}
}

func TestExtractSourceFromPlan_MySQL_ServerIDSet(t *testing.T) {
	ctx := context.Background()
	r := &ClickPipeResource{}

	plan := buildMySQLServerIDPlan(types.Int64Value(4242))

	diagnostics := diag.Diagnostics{}
	source := r.extractSourceFromPlan(ctx, &diagnostics, plan, nil, false)

	assert.False(t, diagnostics.HasError(), "expected no errors, got: %v", diagnostics.Errors())
	assert.NotNil(t, source.MySQL)
	assert.NotNil(t, source.MySQL.ServerID)
	assert.Equal(t, uint32(4242), *source.MySQL.ServerID)
}

// server_id is a uint32 on the wire; the upper bound must survive the
// int64 (Terraform) -> uint32 (API) narrowing.
func TestExtractSourceFromPlan_MySQL_ServerIDMaxValue(t *testing.T) {
	ctx := context.Background()
	r := &ClickPipeResource{}

	plan := buildMySQLServerIDPlan(types.Int64Value(math.MaxUint32))

	diagnostics := diag.Diagnostics{}
	source := r.extractSourceFromPlan(ctx, &diagnostics, plan, nil, false)

	assert.False(t, diagnostics.HasError(), "expected no errors, got: %v", diagnostics.Errors())
	assert.NotNil(t, source.MySQL.ServerID)
	assert.Equal(t, uint32(math.MaxUint32), *source.MySQL.ServerID)
}

// Values outside the uint32 range are rejected by the schema validator; the
// conversion path must never emit a wrapped value if one slips through.
func TestExtractSourceFromPlan_MySQL_ServerIDOutOfRangeNotSent(t *testing.T) {
	ctx := context.Background()
	r := &ClickPipeResource{}

	plan := buildMySQLServerIDPlan(types.Int64Value(math.MaxUint32 + 1))

	diagnostics := diag.Diagnostics{}
	source := r.extractSourceFromPlan(ctx, &diagnostics, plan, nil, false)

	assert.False(t, diagnostics.HasError(), "expected no errors, got: %v", diagnostics.Errors())
	assert.Nil(t, source.MySQL.ServerID)
}

func TestExtractSourceFromPlan_MySQL_ServerIDNull(t *testing.T) {
	ctx := context.Background()
	r := &ClickPipeResource{}

	plan := buildMySQLServerIDPlan(types.Int64Null())

	diagnostics := diag.Diagnostics{}
	source := r.extractSourceFromPlan(ctx, &diagnostics, plan, nil, false)

	assert.False(t, diagnostics.HasError(), "expected no errors, got: %v", diagnostics.Errors())
	assert.NotNil(t, source.MySQL)
	assert.Nil(t, source.MySQL.ServerID)
}

// server_id is Optional+Computed, so on create it is unknown when not configured.
// An unknown value must not be sent (the control plane assigns one).
func TestExtractSourceFromPlan_MySQL_ServerIDUnknown(t *testing.T) {
	ctx := context.Background()
	r := &ClickPipeResource{}

	plan := buildMySQLServerIDPlan(types.Int64Unknown())

	diagnostics := diag.Diagnostics{}
	source := r.extractSourceFromPlan(ctx, &diagnostics, plan, nil, false)

	assert.False(t, diagnostics.HasError(), "expected no errors, got: %v", diagnostics.Errors())
	assert.NotNil(t, source.MySQL)
	assert.Nil(t, source.MySQL.ServerID)
}

func TestExtractSourceFromPlan_MySQL_ServerIDKeptOnUpdate(t *testing.T) {
	ctx := context.Background()
	r := &ClickPipeResource{}

	plan := buildMySQLServerIDPlan(types.Int64Value(4242))

	diagnostics := diag.Diagnostics{}
	source := r.extractSourceFromPlan(ctx, &diagnostics, plan, nil, true)

	assert.False(t, diagnostics.HasError(), "expected no errors, got: %v", diagnostics.Errors())
	assert.NotNil(t, source.MySQL)
	assert.NotNil(t, source.MySQL.ServerID)
	assert.Equal(t, uint32(4242), *source.MySQL.ServerID)
}

func TestClickPipeMySQLSource_ServerIDJSON(t *testing.T) {
	withServerID, err := json.Marshal(api.ClickPipeMySQLSource{Host: "h", Port: 3306, ServerID: uint32Ptr(4242)})
	assert.NoError(t, err)
	assert.Contains(t, string(withServerID), `"serverId":4242`)

	withoutServerID, err := json.Marshal(api.ClickPipeMySQLSource{Host: "h", Port: 3306})
	assert.NoError(t, err)
	assert.NotContains(t, string(withoutServerID), "serverId")
}

func mysqlAPIResponse(serverID *uint32) *api.ClickPipe {
	return &api.ClickPipe{
		ID:    "test-pipe-id",
		Name:  "test-pipe",
		State: "Running",
		Source: api.ClickPipeSource{
			MySQL: &api.ClickPipeMySQLSource{
				Type:     api.ClickPipeMySQLSourceTypeMySQL,
				Host:     "mysql.example.com",
				Port:     3306,
				ServerID: serverID,
				Settings: &api.ClickPipeMySQLSettings{ReplicationMode: api.ClickPipeReplicationModeCDC},
				Mappings: []api.ClickPipeMySQLTableMapping{{
					SourceSchemaName: "mydb",
					SourceTable:      "users",
					TargetTable:      "users",
				}},
			},
		},
		Destination: api.ClickPipeDestination{Database: "default"},
	}
}

func readMySQLModelFromState(ctx context.Context, t *testing.T, state models.ClickPipeResourceModel) models.ClickPipeMySQLSourceModel {
	var sourceModel models.ClickPipeSourceModel
	diags := state.Source.As(ctx, &sourceModel, basetypes.ObjectAsOptions{})
	assert.False(t, diags.HasError(), "reading source: %v", diags)
	var mysqlModel models.ClickPipeMySQLSourceModel
	diags = sourceModel.MySQL.As(ctx, &mysqlModel, basetypes.ObjectAsOptions{})
	assert.False(t, diags.HasError(), "reading mysql source: %v", diags)
	return mysqlModel
}

func TestClickPipeResource_syncClickPipeState_MySQLServerID(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name          string
		stateServerID types.Int64
		apiServerID   *uint32
		expected      types.Int64
	}{
		{
			name:          "API value wins over state",
			stateServerID: types.Int64Value(1),
			apiServerID:   uint32Ptr(4242),
			expected:      types.Int64Value(4242),
		},
		{
			name:          "auto-assigned value is adopted when state is null",
			stateServerID: types.Int64Null(),
			apiServerID:   uint32Ptr(math.MaxUint32),
			expected:      types.Int64Value(math.MaxUint32),
		},
		{
			name:          "state value preserved when API omits the field",
			stateServerID: types.Int64Value(4242),
			apiServerID:   nil,
			expected:      types.Int64Value(4242),
		},
		{
			name:          "null when neither API nor state has a value",
			stateServerID: types.Int64Null(),
			apiServerID:   nil,
			expected:      types.Int64Null(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := buildMySQLServerIDPlan(tt.stateServerID)

			mc := minimock.NewController(t)
			apiClientMock := api.NewClientMock(mc).
				GetClickPipeMock.
				Expect(ctx, state.ServiceID.ValueString(), state.ID.ValueString()).
				Return(mysqlAPIResponse(tt.apiServerID), nil)

			r := &ClickPipeResource{client: apiClientMock}
			err := r.syncClickPipeState(ctx, &state)
			assert.NoError(t, err)

			mysqlModel := readMySQLModelFromState(ctx, t, state)
			assert.True(t, tt.expected.Equal(mysqlModel.ServerID), "expected server_id %v, got %v", tt.expected, mysqlModel.ServerID)
		})
	}
}
