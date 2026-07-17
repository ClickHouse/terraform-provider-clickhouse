package clickstack

import (
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickstack/client"
)

func TestWebhookResource_Metadata(t *testing.T) {
	t.Parallel()
	r := NewWebhookResource()
	resp := &fwresource.MetadataResponse{}
	r.Metadata(context.Background(), fwresource.MetadataRequest{ProviderTypeName: "clickhouse"}, resp)
	if resp.TypeName != "clickhouse_clickstack_webhook" {
		t.Errorf("expected clickhouse_clickstack_webhook, got %q", resp.TypeName)
	}
}

func TestWebhookResource_Schema(t *testing.T) {
	t.Parallel()
	r := NewWebhookResource()
	resp := &fwresource.SchemaResponse{}
	r.Schema(context.Background(), fwresource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %s", resp.Diagnostics)
	}
	// Secret maps must be write-only; url/body sensitive.
	for _, attr := range []string{"headers", "query_params"} {
		a, ok := resp.Schema.Attributes[attr]
		if !ok {
			t.Fatalf("missing attribute %q", attr)
		}
		if !a.IsWriteOnly() {
			t.Errorf("%q must be write-only", attr)
		}
	}
	if !resp.Schema.Attributes["url"].IsSensitive() {
		t.Error("url must be sensitive")
	}
}

func mkStringMap(kv map[string]string) types.Map {
	elems := make(map[string]attr.Value, len(kv))
	for k, v := range kv {
		elems[k] = types.StringValue(v)
	}
	return types.MapValueMust(types.StringType, elems)
}

func TestWebhookResource_Validate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		model   webhookResourceModel
		wantErr bool
	}{
		{
			name:  "valid generic with headers",
			model: webhookResourceModel{Service: types.StringValue("generic"), Headers: mkStringMap(map[string]string{"A": "b"})},
		},
		{
			name:  "valid slack minimal",
			model: webhookResourceModel{Service: types.StringValue("slack"), Headers: types.MapNull(types.StringType), QueryParams: types.MapNull(types.StringType), Body: types.StringNull()},
		},
		{
			name:    "invalid service",
			model:   webhookResourceModel{Service: types.StringValue("teams")},
			wantErr: true,
		},
		{
			name:    "slack rejects headers",
			model:   webhookResourceModel{Service: types.StringValue("slack"), Headers: mkStringMap(map[string]string{"A": "b"})},
			wantErr: true,
		},
		{
			name:    "slack rejects query_params",
			model:   webhookResourceModel{Service: types.StringValue("slack"), QueryParams: mkStringMap(map[string]string{"a": "b"})},
			wantErr: true,
		},
		{
			name:    "slack rejects body",
			model:   webhookResourceModel{Service: types.StringValue("slack"), Body: types.StringValue("x")},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			diags := tc.model.validate()
			if diags.HasError() != tc.wantErr {
				t.Fatalf("HasError()=%v, want %v: %s", diags.HasError(), tc.wantErr, diags)
			}
		})
	}
}

func TestWebhookResource_ToClient_WriteOnlyFromConfig(t *testing.T) {
	t.Parallel()

	// Plan carries the normal fields; write-only secrets live only in config.
	plan := &webhookResourceModel{
		Service:     types.StringValue("generic"),
		Name:        types.StringValue("pd"),
		URL:         types.StringValue("https://example.com"),
		Headers:     types.MapNull(types.StringType),
		QueryParams: types.MapNull(types.StringType),
	}
	config := &webhookResourceModel{
		Headers:     mkStringMap(map[string]string{"Authorization": "Bearer x"}),
		QueryParams: types.MapNull(types.StringType),
	}

	wh, diags := plan.toClient(context.Background(), config)
	if diags.HasError() {
		t.Fatalf("toClient: %s", diags)
	}
	if wh.Name != "pd" || wh.URL != "https://example.com" {
		t.Errorf("normal fields not from plan: %+v", wh)
	}
	if wh.Headers["Authorization"] != "Bearer x" {
		t.Errorf("write-only headers not sourced from config: %v", wh.Headers)
	}
}

// TestWebhookResource_ApplyKeepsBodyWhenServerOmits proves an incidentio webhook
// (whose body the API accepts but never returns) keeps its configured body rather
// than being nulled — which would otherwise raise "inconsistent result after apply".
func TestWebhookResource_ApplyKeepsBodyWhenServerOmits(t *testing.T) {
	t.Parallel()

	m := &webhookResourceModel{Body: types.StringValue("configured-body")}
	m.applyWebhook(&client.Webhook{ID: "wh1", Service: "incidentio", Name: "n", URL: "u", Body: nil})
	if m.Body.ValueString() != "configured-body" {
		t.Errorf("expected configured body preserved when API omits it, got %q", m.Body.ValueString())
	}

	// Generic service returns body -> it is reconciled from the response.
	body := "server-body"
	m2 := &webhookResourceModel{Body: types.StringValue("old")}
	m2.applyWebhook(&client.Webhook{ID: "wh2", Service: "generic", Name: "n", URL: "u", Body: &body})
	if m2.Body.ValueString() != "server-body" {
		t.Errorf("expected returned body reconciled, got %q", m2.Body.ValueString())
	}
}

func webhookSchema(t *testing.T) rschema.Schema {
	t.Helper()
	resp := &fwresource.SchemaResponse{}
	(&webhookResource{}).Schema(context.Background(), fwresource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema: %s", resp.Diagnostics)
	}
	return resp.Schema
}

func webhookModel(mods func(*webhookResourceModel)) webhookResourceModel {
	m := webhookResourceModel{
		ID: types.StringNull(), Team: types.StringNull(),
		Name: types.StringValue("wh"), Service: types.StringValue("generic"), URL: types.StringValue("u"),
		Description: types.StringNull(),
		Headers:     types.MapNull(types.StringType), QueryParams: types.MapNull(types.StringType),
		HeadersVersion: types.StringNull(), QueryParamsVersion: types.StringNull(),
		Body: types.StringNull(),
	}
	if mods != nil {
		mods(&m)
	}
	return m
}

func TestWebhookResource_CRUD(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	sch := webhookSchema(t)

	t.Run("create maps server id into state", func(t *testing.T) {
		t.Parallel()
		r := &webhookResource{client: dashboardTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, `{"data":{"id":"wh1","service":"generic","name":"wh","url":"u"}}`)
		}))}
		plan := tfsdk.Plan{Schema: sch}
		if d := plan.Set(ctx, webhookModel(nil)); d.HasError() {
			t.Fatalf("plan.Set: %s", d)
		}
		// tfsdk.Config has no Set; reuse the plan's raw value (same config here).
		cfg := tfsdk.Config{Schema: sch, Raw: plan.Raw}
		resp := &fwresource.CreateResponse{State: tfsdk.State{Schema: sch}}
		r.Create(ctx, fwresource.CreateRequest{Plan: plan, Config: cfg}, resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("Create: %s", resp.Diagnostics)
		}
		var got webhookResourceModel
		resp.State.Get(ctx, &got)
		if got.ID.ValueString() != "wh1" {
			t.Errorf("id=%q, want wh1", got.ID.ValueString())
		}
	})

	t.Run("read removes resource when webhook is gone", func(t *testing.T) {
		t.Parallel()
		r := &webhookResource{client: dashboardTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, `{"data":[]}`) // list is empty -> GetWebhook ErrNotFound
		}))}
		state := tfsdk.State{Schema: sch}
		if d := state.Set(ctx, webhookModel(func(m *webhookResourceModel) { m.ID = types.StringValue("wh1") })); d.HasError() {
			t.Fatalf("state.Set: %s", d)
		}
		resp := &fwresource.ReadResponse{State: state}
		r.Read(ctx, fwresource.ReadRequest{State: state}, resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("Read: %s", resp.Diagnostics)
		}
		if !resp.State.Raw.IsNull() {
			t.Error("expected resource removed from state when webhook absent")
		}
	})

	t.Run("delete surfaces a non-404 error as a diagnostic", func(t *testing.T) {
		t.Parallel()
		r := &webhookResource{client: dashboardTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, `{"message":"referenced by an alert"}`, http.StatusConflict)
		}))}
		state := tfsdk.State{Schema: sch}
		if d := state.Set(ctx, webhookModel(func(m *webhookResourceModel) { m.ID = types.StringValue("wh1") })); d.HasError() {
			t.Fatalf("state.Set: %s", d)
		}
		resp := &fwresource.DeleteResponse{State: state}
		r.Delete(ctx, fwresource.DeleteRequest{State: state}, resp)
		if !resp.Diagnostics.HasError() {
			t.Error("expected a diagnostic when delete returns 409")
		}
	})
}
