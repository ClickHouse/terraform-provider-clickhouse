package datasource

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gojuno/minimock/v3"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service"
)

func TestNotificationTypesDataSourceMetadata(t *testing.T) {
	resp := datasource.MetadataResponse{}
	NewNotificationTypesDataSource().Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "clickhouse"}, &resp)
	if got, want := resp.TypeName, "clickhouse_notification_types"; got != want {
		t.Fatalf("type name = %q; want %q", got, want)
	}
}

func TestNotificationTypesDataSourceSchema(t *testing.T) {
	ctx := context.Background()

	resp := datasource.SchemaResponse{}
	NewNotificationTypesDataSource().Schema(ctx, datasource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema: %v", resp.Diagnostics.Errors())
	}
	if diags := resp.Schema.ValidateImplementation(ctx); diags.HasError() {
		t.Fatalf("invalid schema implementation: %v", diags.Errors())
	}
}

func TestNotificationTypesDataSourceConfigure(t *testing.T) {
	ctx := context.Background()

	// Terraform configures data sources before the provider has produced its
	// data, so an absent one is the first of two calls, not a failure.
	t.Run("absent provider data is accepted", func(t *testing.T) {
		d := NewNotificationTypesDataSource().(*notificationTypesDataSource)
		resp := datasource.ConfigureResponse{}
		d.Configure(ctx, datasource.ConfigureRequest{}, &resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("Configure: %v", resp.Diagnostics.Errors())
		}
		if d.client != nil {
			t.Errorf("client = %v, want it left unset", d.client)
		}
	})

	t.Run("unexpected provider data is rejected", func(t *testing.T) {
		d := NewNotificationTypesDataSource().(*notificationTypesDataSource)
		resp := datasource.ConfigureResponse{}
		d.Configure(ctx, datasource.ConfigureRequest{ProviderData: "not provider data"}, &resp)
		if !resp.Diagnostics.HasError() {
			t.Fatal("Configure accepted provider data of the wrong type")
		}
		if got := resp.Diagnostics.Errors()[0].Summary(); got != "Unexpected provider data" {
			t.Errorf("summary = %q, want Unexpected provider data", got)
		}
	})

	t.Run("missing API credentials are rejected", func(t *testing.T) {
		d := NewNotificationTypesDataSource().(*notificationTypesDataSource)
		resp := datasource.ConfigureResponse{}
		d.Configure(ctx, datasource.ConfigureRequest{ProviderData: &service.ProviderData{}}, &resp)
		if !resp.Diagnostics.HasError() {
			t.Fatal("Configure accepted a provider with no ClickHouse Cloud API")
		}
		if got := resp.Diagnostics.Errors()[0].Summary(); got != "ClickHouse Cloud API not configured" {
			t.Errorf("summary = %q, want ClickHouse Cloud API not configured", got)
		}
	})

	t.Run("a configured API is kept", func(t *testing.T) {
		d := NewNotificationTypesDataSource().(*notificationTypesDataSource)
		client := api.NewClientMock(minimock.NewController(t))
		resp := datasource.ConfigureResponse{}
		d.Configure(ctx, datasource.ConfigureRequest{ProviderData: &service.ProviderData{API: client}}, &resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("Configure: %v", resp.Diagnostics.Errors())
		}
		if d.client != client {
			t.Errorf("client = %v, want the API from the provider data", d.client)
		}
	})
}

// readNotificationTypes drives the data source's Read against a client that
// answers with the given types, and returns the state it produced.
func readNotificationTypes(t *testing.T, listed []api.NotificationType, listErr error) *datasource.ReadResponse {
	t.Helper()
	ctx := context.Background()

	d := NewNotificationTypesDataSource().(*notificationTypesDataSource)
	d.client = api.NewClientMock(minimock.NewController(t)).
		ListNotificationTypesMock.
		Expect(ctx).
		Return(listed, listErr)

	schemaResp := datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("building the schema: %v", schemaResp.Diagnostics.Errors())
	}

	raw := tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), map[string]tftypes.Value{
		"types": tftypes.NewValue(schemaResp.Schema.Attributes["types"].GetType().TerraformType(ctx), nil),
	})
	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema, Raw: raw}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw}}, resp)
	return resp
}

// Every field the API reports reaches state: the data source exists so a
// configuration can read is_configurable before naming a type in a rule.
func TestNotificationTypesDataSourceReadCarriesEveryField(t *testing.T) {
	resp := readNotificationTypes(t, []api.NotificationType{{
		Name:           "service_backup_failed",
		Title:          "Backup failed",
		Description:    "A backup of your service failed",
		Category:       "service",
		Severity:       "error",
		ResourceType:   "service",
		IsConfigurable: true,
		IsAvailable:    false,
	}}, nil)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", resp.Diagnostics.Errors())
	}

	var state notificationTypesDataSourceModel
	if diags := resp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("reading state back: %v", diags.Errors())
	}
	if n := len(state.Types.Elements()); n != 1 {
		t.Fatalf("types length = %d, want 1", n)
	}

	got := state.Types.Elements()[0].(types.Object).Attributes()
	for name, want := range map[string]string{
		"name":          "service_backup_failed",
		"title":         "Backup failed",
		"description":   "A backup of your service failed",
		"category":      "service",
		"severity":      "error",
		"resource_type": "service",
	} {
		if value := got[name].(types.String).ValueString(); value != want {
			t.Errorf("%s = %q, want %q", name, value, want)
		}
	}
	if !got["is_configurable"].(types.Bool).ValueBool() {
		t.Error("is_configurable = false, want true")
	}
	if got["is_available"].(types.Bool).ValueBool() {
		t.Error("is_available = true, want false")
	}
}

// An empty registry is a known empty list, not null: a configuration iterating
// over it must see no types rather than an unknown value.
func TestNotificationTypesDataSourceReadReturnsAKnownEmptyList(t *testing.T) {
	resp := readNotificationTypes(t, nil, nil)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", resp.Diagnostics.Errors())
	}

	var state notificationTypesDataSourceModel
	if diags := resp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("reading state back: %v", diags.Errors())
	}
	if state.Types.IsNull() || state.Types.IsUnknown() || len(state.Types.Elements()) != 0 {
		t.Errorf("types = %v, want a known empty list", state.Types)
	}
}

func TestNotificationTypesDataSourceReadReportsAFailedList(t *testing.T) {
	resp := readNotificationTypes(t, nil, errors.New("status: 403, body: insufficient permissions"))

	if !resp.Diagnostics.HasError() {
		t.Fatal("Read hid the failure instead of reporting it")
	}
	if got := resp.Diagnostics.Errors()[0].Summary(); got != "Error listing notification types" {
		t.Errorf("summary = %q, want Error listing notification types", got)
	}
	if got := resp.Diagnostics.Errors()[0].Detail(); !strings.Contains(got, "insufficient permissions") {
		t.Errorf("detail = %q, want it to carry what the API said", got)
	}
}
