package clickstack

import (
	"context"
	"io"
	"net/http"
	"testing"

	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickstack/client"
)

func TestSavedSearchResource_Metadata(t *testing.T) {
	t.Parallel()
	r := NewSavedSearchResource()
	resp := &fwresource.MetadataResponse{}
	r.Metadata(context.Background(), fwresource.MetadataRequest{ProviderTypeName: "clickhouse"}, resp)
	if resp.TypeName != "clickhouse_clickstack_saved_search" {
		t.Errorf("expected clickhouse_clickstack_saved_search, got %q", resp.TypeName)
	}
}

func TestSavedSearchResource_Schema(t *testing.T) {
	t.Parallel()
	r := NewSavedSearchResource()
	resp := &fwresource.SchemaResponse{}
	r.Schema(context.Background(), fwresource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %s", resp.Diagnostics)
	}
	for _, attr := range []string{"id", "name", "source_id", "filters", "tags"} {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("expected attribute %q", attr)
		}
	}
}

func TestSavedSearchResource_ToClient_Defaults(t *testing.T) {
	t.Parallel()

	m := &savedSearchResourceModel{
		Name:          types.StringValue("errors"),
		SourceID:      types.StringValue("src1"),
		Select:        types.StringValue(""),
		Where:         types.StringValue(""),
		WhereLanguage: types.StringValue("lucene"),
		OrderBy:       types.StringValue(""),
		Tags:          types.ListNull(types.StringType),
		Filters:       types.StringNull(),
	}
	ss, diags := m.toClient(context.Background())
	if diags.HasError() {
		t.Fatalf("toClient: %s", diags)
	}
	// Full-replace PUT: tags must be a non-nil [] and filters must default to [].
	if ss.Tags == nil || len(ss.Tags) != 0 {
		t.Errorf("expected empty non-nil tags, got %v", ss.Tags)
	}
	if string(ss.Filters) != "[]" {
		t.Errorf("expected filters default [], got %q", string(ss.Filters))
	}
}

func TestSavedSearchResource_ApplyPreservesFilters(t *testing.T) {
	t.Parallel()

	m := &savedSearchResourceModel{}
	ss := &client.SavedSearch{
		ID:       "ss1",
		Name:     "n",
		SourceID: "s",
		Tags:     []string{"a", "b"},
		Filters:  []byte(`[{"type":"sql_ast","operator":"and"}]`),
	}
	if diags := m.applySavedSearch(ss); diags.HasError() {
		t.Fatalf("applySavedSearch: %s", diags)
	}
	if m.Filters.ValueString() != `[{"type":"sql_ast","operator":"and"}]` {
		t.Errorf("filters not preserved verbatim: %q", m.Filters.ValueString())
	}
	if m.Tags.IsNull() || len(m.Tags.Elements()) != 2 {
		t.Errorf("tags not applied: %v", m.Tags)
	}
}

func TestCanonicalizeJSON_Equal(t *testing.T) {
	t.Parallel()
	a, err := canonicalizeJSON(`{"b":1,"a":2}`)
	if err != nil {
		t.Fatalf("canonicalizeJSON: %v", err)
	}
	b, err := canonicalizeJSON(`{ "a": 2, "b": 1 }`)
	if err != nil {
		t.Fatalf("canonicalizeJSON: %v", err)
	}
	if a != b {
		t.Errorf("expected semantically-equal JSON to canonicalize equal: %q vs %q", a, b)
	}
}

func TestSavedSearchResource_Validate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		model   savedSearchResourceModel
		wantErr bool
	}{
		{"valid lucene", savedSearchResourceModel{WhereLanguage: types.StringValue("lucene")}, false},
		{"valid sql", savedSearchResourceModel{WhereLanguage: types.StringValue("sql")}, false},
		{"invalid where_language", savedSearchResourceModel{WhereLanguage: types.StringValue("promql")}, true},
		{"invalid filters json", savedSearchResourceModel{Filters: types.StringValue("{not json")}, true},
		{"valid filters json", savedSearchResourceModel{Filters: types.StringValue(`[{"type":"sql","condition":"x"}]`)}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.model.validate().HasError(); got != tc.wantErr {
				t.Fatalf("HasError()=%v, want %v", got, tc.wantErr)
			}
		})
	}
}

// TestSavedSearchResource_ApplyKeepsAuthoredFilters proves the authored filters
// value is the source of truth: applySavedSearch keeps it regardless of what the
// server returns (reordered keys, injected default type, or otherwise
// normalized), so a server transform never yields an "inconsistent result after
// apply". The server value is only adopted when the config left filters unset.
func TestSavedSearchResource_ApplyKeepsAuthoredFilters(t *testing.T) {
	t.Parallel()

	authored := `[{"type":"sql","condition":"x"}]`

	// Server reorders keys -> authored kept.
	m := &savedSearchResourceModel{Filters: types.StringValue(authored)}
	m.applySavedSearch(&client.SavedSearch{ID: "ss1", Filters: []byte(`[{"condition":"x","type":"sql"}]`)})
	if m.Filters.ValueString() != authored {
		t.Errorf("expected authored filters kept on reorder, got %q", m.Filters.ValueString())
	}

	// Server normalizes to a different shape (e.g. injected type) -> still keep the
	// authored value; adopting the server value here would be an inconsistent
	// result after apply against the known planned value.
	m2 := &savedSearchResourceModel{Filters: types.StringValue(`[{"condition":"x"}]`)}
	m2.applySavedSearch(&client.SavedSearch{ID: "ss1", Filters: []byte(`[{"type":"sql","condition":"x"}]`)})
	if m2.Filters.ValueString() != `[{"condition":"x"}]` {
		t.Errorf("expected authored value kept despite server normalization, got %q", m2.Filters.ValueString())
	}

	// Unset (unknown) config -> adopt the server value.
	m3 := &savedSearchResourceModel{Filters: types.StringUnknown()}
	m3.applySavedSearch(&client.SavedSearch{ID: "ss1", Filters: []byte(`[{"type":"sql","condition":"z"}]`)})
	if m3.Filters.ValueString() != `[{"type":"sql","condition":"z"}]` {
		t.Errorf("expected server filters adopted when config unset, got %q", m3.Filters.ValueString())
	}
}

func savedSearchSchema(t *testing.T) rschema.Schema {
	t.Helper()
	resp := &fwresource.SchemaResponse{}
	(&savedSearchResource{}).Schema(context.Background(), fwresource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema: %s", resp.Diagnostics)
	}
	return resp.Schema
}

func savedSearchModel(mods func(*savedSearchResourceModel)) savedSearchResourceModel {
	m := savedSearchResourceModel{
		ID: types.StringNull(), Team: types.StringNull(),
		Name: types.StringValue("n"), SourceID: types.StringValue("src"),
		Select: types.StringValue(""), Where: types.StringValue(""),
		WhereLanguage: types.StringValue("lucene"), OrderBy: types.StringValue(""),
		Tags: types.ListNull(types.StringType), Filters: types.StringNull(),
	}
	if mods != nil {
		mods(&m)
	}
	return m
}

func TestSavedSearchResource_CRUD(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	sch := savedSearchSchema(t)

	t.Run("create maps server id into state", func(t *testing.T) {
		t.Parallel()
		r := &savedSearchResource{client: dashboardTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, `{"data":{"id":"ss1","name":"n","sourceId":"src","whereLanguage":"lucene","tags":[],"filters":[]}}`)
		}))}
		plan := tfsdk.Plan{Schema: sch}
		if d := plan.Set(ctx, savedSearchModel(nil)); d.HasError() {
			t.Fatalf("plan.Set: %s", d)
		}
		resp := &fwresource.CreateResponse{State: tfsdk.State{Schema: sch}}
		r.Create(ctx, fwresource.CreateRequest{Plan: plan}, resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("Create: %s", resp.Diagnostics)
		}
		var got savedSearchResourceModel
		resp.State.Get(ctx, &got)
		if got.ID.ValueString() != "ss1" {
			t.Errorf("id=%q, want ss1", got.ID.ValueString())
		}
	})

	t.Run("read removes resource on 404", func(t *testing.T) {
		t.Parallel()
		r := &savedSearchResource{client: dashboardTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))}
		state := tfsdk.State{Schema: sch}
		if d := state.Set(ctx, savedSearchModel(func(m *savedSearchResourceModel) { m.ID = types.StringValue("ss1") })); d.HasError() {
			t.Fatalf("state.Set: %s", d)
		}
		resp := &fwresource.ReadResponse{State: state}
		r.Read(ctx, fwresource.ReadRequest{State: state}, resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("Read: %s", resp.Diagnostics)
		}
		if !resp.State.Raw.IsNull() {
			t.Error("expected resource removed from state on 404")
		}
	})

	t.Run("create surfaces a generic error as a diagnostic", func(t *testing.T) {
		t.Parallel()
		r := &savedSearchResource{client: dashboardTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, `{"message":"bad source"}`, http.StatusBadRequest)
		}))}
		plan := tfsdk.Plan{Schema: sch}
		if d := plan.Set(ctx, savedSearchModel(nil)); d.HasError() {
			t.Fatalf("plan.Set: %s", d)
		}
		resp := &fwresource.CreateResponse{State: tfsdk.State{Schema: sch}}
		r.Create(ctx, fwresource.CreateRequest{Plan: plan}, resp)
		if !resp.Diagnostics.HasError() {
			t.Error("expected a diagnostic on create error")
		}
	})
}
