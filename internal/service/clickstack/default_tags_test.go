package clickstack

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickstack/client"
)

func TestDefaultTags(t *testing.T) {
	t.Parallel()
	if got := withDefaultTags([]string{"a", "tf"}, []string{"tf", "team"}); !slices.Equal(got, []string{"a", "tf", "team"}) {
		t.Errorf("withDefaultTags = %v", got)
	}
	cases := []struct {
		name            string
		server, own, dr []string
		want            []string
	}{
		{"defaults dropped", []string{"a", "tf"}, []string{"a"}, []string{"tf"}, []string{"a"}},
		{"a default the resource lists itself is kept", []string{"tf", "a"}, []string{"tf", "a"}, []string{"tf"}, []string{"tf", "a"}},
		{"a tag added outside Terraform shows as drift", []string{"a", "ui"}, []string{"a"}, []string{"tf"}, []string{"a", "ui"}},
	}
	for _, tc := range cases {
		if got := withoutDefaultTags(tc.server, tc.own, tc.dr); !slices.Equal(got, tc.want) {
			t.Errorf("%s: withoutDefaultTags = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestSavedSearchDefaultTags(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	m := savedSearchModel(func(m *savedSearchResourceModel) {
		m.Tags = types.ListValueMust(types.StringType, nil)
	})
	ss, _ := m.toClient(ctx, []string{"tf"})
	if !slices.Equal(ss.Tags, []string{"tf"}) {
		t.Fatalf("sent tags %v, want the default added", ss.Tags)
	}

	m.TagsAll, _ = tagsSet([]string{"old"})
	m.applySavedSearch(ctx, &client.SavedSearch{Tags: []string{"old", "tf"}}, []string{"tf"})
	if len(m.Tags.Elements()) != 0 {
		t.Errorf("tags = %v, want defaults kept out", m.Tags)
	}
	if want, _ := tagsSet([]string{"old", "tf"}); !m.TagsAll.Equal(want) {
		t.Errorf("tags_all = %v, want the server's tags", m.TagsAll)
	}
}

func TestDashboardDefaultTags(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	sch := dashboardTestSchema(t)

	var sent struct{ Tags []string }
	r := &dashboardResource{defaultTags: []string{"tf"}, client: dashboardTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		b, _ := io.ReadAll(req.Body)
		_ = json.Unmarshal(b, &sent)
		_, _ = w.Write([]byte(`{"data":{"id":"d1","name":"D","tags":["a","tf"],"tiles":[]}}`))
	}))}
	body := `{"name":"D","tags":["a"],"tiles":[]}`
	resp := &fwresource.CreateResponse{State: tfsdk.State{Schema: sch}}
	r.Create(ctx, fwresource.CreateRequest{Plan: tfsdk.Plan{Schema: sch, Raw: dashboardObjectValue(nil, nil, ptr(body), nil)}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	if !slices.Equal(sent.Tags, []string{"a", "tf"}) {
		t.Errorf("sent tags %v, want the default added", sent.Tags)
	}

	// Only the defaults changed: the plan must update the dashboard and let
	// normalized_json change, even though dashboard_json did not.
	state := resp.State
	r.defaultTags = []string{"tf", "team"}
	plan := tfsdk.Plan{Schema: sch, Raw: state.Raw.Copy()}
	mresp := &fwresource.ModifyPlanResponse{Plan: plan}
	r.ModifyPlan(ctx, fwresource.ModifyPlanRequest{State: state, Plan: plan}, mresp)
	var got dashboardResourceModel
	mresp.Plan.Get(ctx, &got)
	if want, _ := tagsSet([]string{"a", "tf", "team"}); !got.TagsAll.Equal(want) {
		t.Errorf("planned tags_all = %v, want %v", got.TagsAll, want)
	}
	if !got.NormalizedJSON.IsUnknown() {
		t.Error("normalized_json stayed pinned on a tags-only update")
	}
}

func TestPlannedTagsAllLimit(t *testing.T) {
	t.Parallel()
	own := make([]string, maxTags)
	for i := range own {
		own[i] = fmt.Sprint(i)
	}
	if _, d := plannedTagsAll(own, nil); d.HasError() {
		t.Errorf("%d tags rejected: %s", maxTags, d)
	}
	if _, d := plannedTagsAll(own, []string{"tf"}); !d.HasError() {
		t.Error("defaults pushing the tags past the API limit must fail at plan time")
	}
}

func TestAlertDefaultTags(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	r := &alertResource{defaultTags: []string{"tf"}}
	if got := r.tagsToSend(nil); got != nil {
		t.Errorf("unset tags sent %v, want nil so the parent's are copied", got)
	}

	m := mkAlert(func(m *alertResourceModel) {
		m.Tags = types.ListValueMust(types.StringType, []attr.Value{types.StringValue("mine")})
	})
	m.applyTags(ctx, &[]string{"mine", "tf"}, r.defaultTags)
	var got []string
	m.Tags.ElementsAs(ctx, &got, false)
	if !slices.Equal(got, []string{"mine"}) {
		t.Errorf("tags = %v, want defaults kept out", got)
	}

	// A tag from another resource's unknown attribute leaves tags_all unknown
	// instead of failing the plan.
	sch := alertSchema(t)
	plan := tfsdk.Plan{Schema: sch}
	plan.Set(ctx, mkAlert(func(m *alertResourceModel) {
		m.Tags = types.ListValueMust(types.StringType, []attr.Value{types.StringUnknown()})
	}))
	resp := &fwresource.ModifyPlanResponse{Plan: plan}
	r.ModifyPlan(ctx, fwresource.ModifyPlanRequest{Plan: plan, State: tfsdk.State{Schema: sch}}, resp)
	var planned alertResourceModel
	resp.Plan.Get(ctx, &planned)
	if resp.Diagnostics.HasError() || !planned.TagsAll.IsUnknown() {
		t.Errorf("tags_all = %v, diags %s; want unknown and no error", planned.TagsAll, resp.Diagnostics)
	}
}

// A tile added in the UI is in state but not in dashboard_json, so a
// defaults-only update drops it and tile_ids must not keep planning it.
func TestDashboardDefaultTagsReplanTileIDs(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	sch := dashboardTestSchema(t)
	prior, _ := tagsSet(nil)
	tileIDs := types.MapValueMust(types.StringType, map[string]attr.Value{
		"T": types.StringValue("t1"), "UI": types.StringValue("ui"),
	})
	state := tfsdk.State{Schema: sch}
	state.Set(ctx, dashboardResourceModel{
		ID: types.StringValue("d1"), Team: types.StringNull(), TagsAll: prior, TileIDs: tileIDs,
		DashboardJSON:  types.StringValue(`{"name":"D","tiles":[{"name":"T"}]}`),
		NormalizedJSON: types.StringValue(`{"id":"d1","name":"D","tiles":[{"id":"t1","name":"T"},{"id":"ui","name":"UI"}]}`),
	})
	plan := tfsdk.Plan{Schema: sch, Raw: state.Raw.Copy()}
	resp := &fwresource.ModifyPlanResponse{Plan: plan}
	(&dashboardResource{defaultTags: []string{"tf"}}).ModifyPlan(ctx, fwresource.ModifyPlanRequest{State: state, Plan: plan}, resp)
	var got dashboardResourceModel
	resp.Plan.Get(ctx, &got)
	want := types.MapValueMust(types.StringType, map[string]attr.Value{"T": types.StringValue("t1")})
	if !got.TileIDs.Equal(want) {
		t.Errorf("planned tile_ids = %v, want %v", got.TileIDs, want)
	}
}

// State written before tags_all existed takes the authored tags, so a tag
// changed in the UI does not plan an update on upgrade.
func TestDashboardReadFillsTagsAllFromAuthored(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	sch := dashboardTestSchema(t)
	r := &dashboardResource{client: dashboardTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"id":"d1","name":"D","tags":["a","ui"],"tiles":[]}}`))
	}))}
	body := `{"name":"D","tags":["a"],"tiles":[]}`
	state := tfsdk.State{Schema: sch, Raw: dashboardObjectValue(ptr("d1"), nil, ptr(body), ptr(body))}
	resp := &fwresource.ReadResponse{State: state}
	r.Read(ctx, fwresource.ReadRequest{State: state}, resp)
	var got dashboardResourceModel
	resp.State.Get(ctx, &got)
	if want, _ := tagsSet([]string{"a"}); !got.TagsAll.Equal(want) {
		t.Errorf("tags_all = %v, want the authored tags; diags %s", got.TagsAll, resp.Diagnostics)
	}
}
