package clickstack

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickstack/client"
)

// fakeSourceAPI serves one source at /api/v2/sources/t1 and records the body of
// every PUT, which replaces the stored source as the real API does.
type fakeSourceAPI struct {
	t      *testing.T
	stored map[string]any
	puts   []map[string]any
}

func newFakeSourceAPI(t *testing.T, stored map[string]any) (*fakeSourceAPI, *client.Client) {
	t.Helper()
	f := &fakeSourceAPI{t: t, stored: stored}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/sources/t1" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"message":"Source not found"}`)
			return
		}
		switch r.Method {
		case http.MethodGet:
		case http.MethodPut:
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode PUT body: %v", err)
			}
			f.puts = append(f.puts, body)
			f.stored = body
		default:
			t.Errorf("unexpected method %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": f.stored})
	}))
	t.Cleanup(srv.Close)
	c, err := client.New(srv.URL, "test-key", srv.Client())
	if err != nil {
		t.Fatalf("client.New: %v", err)
	}
	return f, c
}

func sourceLinksSchema(t *testing.T) schema.Schema {
	t.Helper()
	resp := &fwresource.SchemaResponse{}
	NewSourceLinksResource().Schema(context.Background(), fwresource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %s", resp.Diagnostics)
	}
	return resp.Schema
}

func linksModel(log, session *string) sourceLinksResourceModel {
	return sourceLinksResourceModel{
		ID:              types.StringValue("t1"),
		Team:            types.StringNull(),
		SourceID:        types.StringValue("t1"),
		LogSourceID:     types.StringPointerValue(log),
		TraceSourceID:   types.StringNull(),
		MetricSourceID:  types.StringNull(),
		SessionSourceID: types.StringPointerValue(session),
	}
}

func toState(t *testing.T, sch schema.Schema, m sourceLinksResourceModel) tfsdk.State {
	t.Helper()
	s := tfsdk.State{Schema: sch}
	if diags := s.Set(context.Background(), &m); diags.HasError() {
		t.Fatalf("set state: %s", diags)
	}
	return s
}

// TestSourceLinksResource_UpdateWritesOnlyManagedLinks guards the ownership
// rule: a link removed from config is cleared, a configured one is written, and
// a link this resource never managed (metricSourceId here) survives the write.
func TestSourceLinksResource_UpdateWritesOnlyManagedLinks(t *testing.T) {
	t.Parallel()

	api, c := newFakeSourceAPI(t, map[string]any{
		"id": "t1", "kind": "trace", "name": "traces",
		"logSourceId": "l1", "sessionSourceId": "s1", "metricSourceId": "m1",
	})
	r := &sourceLinksResource{client: c}
	sch := sourceLinksSchema(t)

	prior := toState(t, sch, linksModel(strPtr("l1"), strPtr("s1")))
	planned := toState(t, sch, linksModel(strPtr("l2"), nil))
	resp := &fwresource.UpdateResponse{State: tfsdk.State{Schema: sch}}
	r.Update(context.Background(), fwresource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: sch, Raw: planned.Raw},
		State: prior,
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Update: %s", resp.Diagnostics)
	}

	want := map[string]any{"id": "t1", "kind": "trace", "name": "traces", "logSourceId": "l2", "metricSourceId": "m1"}
	if len(api.puts) != 1 || !reflect.DeepEqual(api.puts[0], want) {
		t.Fatalf("PUT bodies = %v, want [%v]", api.puts, want)
	}

	var got sourceLinksResourceModel
	resp.Diagnostics.Append(resp.State.Get(context.Background(), &got)...)
	if got.LogSourceID.ValueString() != "l2" || !got.SessionSourceID.IsNull() || !got.MetricSourceID.IsNull() {
		t.Errorf("state after update = %+v", got)
	}
}

// TestSourceLinksResource_ReadReportsOnlyManagedLinks guards that drift on a
// managed link is reported while a link this resource does not manage stays
// null, which would otherwise plan its removal on every run.
func TestSourceLinksResource_ReadReportsOnlyManagedLinks(t *testing.T) {
	t.Parallel()

	_, c := newFakeSourceAPI(t, map[string]any{
		"id": "t1", "kind": "trace", "name": "traces", "logSourceId": "changed", "metricSourceId": "m1",
	})
	r := &sourceLinksResource{client: c}
	sch := sourceLinksSchema(t)

	resp := &fwresource.ReadResponse{State: tfsdk.State{Schema: sch}}
	r.Read(context.Background(), fwresource.ReadRequest{State: toState(t, sch, linksModel(strPtr("l1"), strPtr("s1")))}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read: %s", resp.Diagnostics)
	}

	var got sourceLinksResourceModel
	resp.Diagnostics.Append(resp.State.Get(context.Background(), &got)...)
	if got.LogSourceID.ValueString() != "changed" {
		t.Errorf("log_source_id = %v, want the drifted server value", got.LogSourceID)
	}
	if !got.SessionSourceID.IsNull() {
		t.Errorf("session_source_id = %v, want null: the server no longer holds it", got.SessionSourceID)
	}
	if !got.MetricSourceID.IsNull() {
		t.Errorf("metric_source_id = %v, want null: it is not managed by this resource", got.MetricSourceID)
	}
}

// TestSourceLinksResource_DeleteClearsManagedLinks guards that destroy clears
// only the links this resource set, and treats a deleted source as done.
func TestSourceLinksResource_DeleteClearsManagedLinks(t *testing.T) {
	t.Parallel()

	api, c := newFakeSourceAPI(t, map[string]any{
		"id": "t1", "kind": "trace", "name": "traces", "logSourceId": "l1", "metricSourceId": "m1",
	})
	r := &sourceLinksResource{client: c}
	sch := sourceLinksSchema(t)

	resp := &fwresource.DeleteResponse{}
	r.Delete(context.Background(), fwresource.DeleteRequest{State: toState(t, sch, linksModel(strPtr("l1"), nil))}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Delete: %s", resp.Diagnostics)
	}
	want := map[string]any{"id": "t1", "kind": "trace", "name": "traces", "metricSourceId": "m1"}
	if len(api.puts) != 1 || !reflect.DeepEqual(api.puts[0], want) {
		t.Fatalf("PUT bodies = %v, want [%v]", api.puts, want)
	}

	gone := linksModel(strPtr("l1"), nil)
	gone.SourceID = types.StringValue("deleted")
	resp = &fwresource.DeleteResponse{}
	r.Delete(context.Background(), fwresource.DeleteRequest{State: toState(t, sch, gone)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Delete of a deleted source: %s", resp.Diagnostics)
	}
}
