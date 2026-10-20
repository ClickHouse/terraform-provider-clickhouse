package resource

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/gojuno/minimock/v3"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/stretchr/testify/require"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/postgres/resource/models"
)

func ipAccessList(rules ...api.PostgresIpAccess) types.List {
	values := make([]attr.Value, 0, len(rules))
	for _, rule := range rules {
		values = append(values, models.IPAccessList{
			Source: types.StringValue(rule.Source), PortRange: types.StringValue(rule.PortRange),
			Description: types.StringValue(rule.Description),
		}.ObjectValue())
	}
	return types.ListValueMust(models.IPAccessList{}.ObjectType(), values)
}

func TestValidatePostgresIPAccess(t *testing.T) {
	for _, tc := range []struct {
		source, port string
		valid        bool
	}{
		{"192.0.2.1", "5432", true},
		{"192.0.2.17/24", "05432..06432", true},
		{"0.0.0.0/0", "1..65535", true},
		{"192.0.2.1/32", "5432..5432", true},
		{"::/0", "5432", false},
		{"::ffff:192.0.2.1", "5432", false},
		{"192.0.2.1/33", "5432", false},
		{"256.0.0.1", "5432", false},
		{"example.com", "5432", false},
		{"", "5432", false},
		{"192.0.2.1", "0", false},
		{"192.0.2.1", "65536", false},
		{"192.0.2.1", "6432..5432", false},
		{"192.0.2.1", "5432-6432", false},
		{"192.0.2.1", "..5432", false},
		{"192.0.2.1", "5432..", false},
		{"192.0.2.1", "1..2..3", false},
		{"192.0.2.1", "+5432", false},
		{"192.0.2.1", " 5432", false},
	} {
		t.Run(tc.source+"/"+tc.port, func(t *testing.T) {
			diags := validatePostgresIPAccess(ipAccessList(api.PostgresIpAccess{Source: tc.source, PortRange: tc.port}))
			require.Equal(t, !tc.valid, diags.HasError(), "%v", diags)
		})
	}
	t.Run("normalized duplicates", func(t *testing.T) {
		rules := ipAccessList(
			api.PostgresIpAccess{Source: "192.0.2.1", PortRange: "05432"},
			api.PostgresIpAccess{Source: "192.0.2.1/32", PortRange: "5432..5432"},
		)
		require.True(t, validatePostgresIPAccess(rules).HasError())
	})
	t.Run("same source different ports", func(t *testing.T) {
		require.False(t, validatePostgresIPAccess(ipAccessList(
			api.PostgresIpAccess{Source: "192.0.2.1", PortRange: "5432"},
			api.PostgresIpAccess{Source: "192.0.2.1", PortRange: "6432"},
		)).HasError())
	})
	elementType := models.IPAccessList{}.ObjectType()
	for _, list := range []types.List{
		types.ListNull(elementType), types.ListUnknown(elementType), ipAccessList(),
		types.ListValueMust(elementType, []attr.Value{types.ObjectUnknown(elementType.AttrTypes)}),
		types.ListValueMust(elementType, []attr.Value{models.IPAccessList{
			Source: types.StringUnknown(), PortRange: types.StringValue("5432"), Description: types.StringUnknown(),
		}.ObjectValue()}),
	} {
		require.False(t, validatePostgresIPAccess(list).HasError(), "%v", list)
	}
	require.True(t, validatePostgresIPAccess(types.ListValueMust(elementType, []attr.Value{
		types.ObjectNull(elementType.AttrTypes),
	})).HasError())
}

func TestPlanPostgresIPAccess(t *testing.T) {
	for _, list := range []types.List{
		types.ListNull(models.IPAccessList{}.ObjectType()),
		types.ListUnknown(models.IPAccessList{}.ObjectType()),
	} {
		rules, diags := planPostgresIPAccess(context.Background(), list)
		require.False(t, diags.HasError())
		require.Nil(t, rules)
	}
	rules, diags := planPostgresIPAccess(context.Background(), ipAccessList())
	require.False(t, diags.HasError())
	require.NotNil(t, rules)
	require.Empty(t, *rules)
	unresolved := models.IPAccessList{
		Source: types.StringUnknown(), PortRange: types.StringValue("5432"), Description: types.StringValue(""),
	}
	_, diags = planPostgresIPAccess(context.Background(), types.ListValueMust(unresolved.ObjectType(), []attr.Value{unresolved.ObjectValue()}))
	require.True(t, diags.HasError())
}

func TestDiffPostgresIPAccess(t *testing.T) {
	current := []api.PostgresIpAccess{
		{Id: "opaque-rule-a", Source: "192.0.2.1/32", PortRange: "5432", Description: "old"},
		{Id: "opaque-rule-b", Source: "192.0.2.1/32", PortRange: "6432", Description: "pooler"},
	}
	t.Run("edit clear description remove and add", func(t *testing.T) {
		got, err := diffPostgresIPAccess(current, []api.PostgresIpAccess{
			{Source: "192.0.2.1", PortRange: "05432..05432", Description: ""},
			{Source: "198.51.100.0/24", PortRange: "5432", Description: "new"},
		})
		require.NoError(t, err)
		require.Equal(t, &api.PostgresIpAccessUpdate{
			Add: []api.PostgresIpAccess{
				{Id: "opaque-rule-a", Source: "192.0.2.1/32", PortRange: "5432", Description: ""},
				{Source: "198.51.100.0/24", PortRange: "5432", Description: "new"},
			},
			Remove: []api.PostgresIpAccessRuleReference{{Id: "opaque-rule-b"}},
		}, got)
	})
	t.Run("reordering and equivalent spelling do not patch", func(t *testing.T) {
		got, err := diffPostgresIPAccess(current, []api.PostgresIpAccess{
			{Source: "192.0.2.1", PortRange: "06432", Description: " pooler "},
			{Source: "192.0.2.1", PortRange: "5432..5432", Description: "old"},
		})
		require.NoError(t, err)
		require.Nil(t, got)
	})
	t.Run("empty clears all IPv4 rules", func(t *testing.T) {
		got, err := diffPostgresIPAccess(current, nil)
		require.NoError(t, err)
		require.Empty(t, got.Add)
		require.Equal(t, []api.PostgresIpAccessRuleReference{{Id: "opaque-rule-a"}, {Id: "opaque-rule-b"}}, got.Remove)
	})
	t.Run("remove out of band duplicate", func(t *testing.T) {
		duplicate := current[0]
		duplicate.Id = "out-of-band"
		got, err := diffPostgresIPAccess(append(current, duplicate), current)
		require.NoError(t, err)
		require.Empty(t, got.Add)
		require.Equal(t, []api.PostgresIpAccessRuleReference{{Id: "out-of-band"}}, got.Remove)
	})
	t.Run("invalid server response", func(t *testing.T) {
		for _, rule := range []api.PostgresIpAccess{
			{Source: "192.0.2.1", PortRange: "5432"},
			{Id: "x", Source: "::/0", PortRange: "5432"},
			{Id: "x", Source: "192.0.2.1", PortRange: "invalid"},
		} {
			_, err := diffPostgresIPAccess([]api.PostgresIpAccess{rule}, nil)
			require.Error(t, err)
		}
	})
}

func TestSyncPostgresIPAccess(t *testing.T) {
	ctx := context.Background()
	prior := ipAccessList(
		api.PostgresIpAccess{Source: "192.0.2.17/24", PortRange: "05432..05432", Description: " office "},
		api.PostgresIpAccess{Source: "198.51.100.1", PortRange: "6432", Description: "pooler"},
	)
	remote := []api.PostgresIpAccess{
		{Id: "b", Source: "198.51.100.1/32", PortRange: "6432", Description: "pooler"},
		{Id: "a", Source: "192.0.2.0/24", PortRange: "5432", Description: "office"},
	}
	got, diags := syncPostgresIPAccess(ctx, remote, prior)
	require.False(t, diags.HasError(), "%v", diags)
	require.True(t, prior.Equal(got), "%v", got)

	// Changed descriptions, new entries and removed entries must surface as drift.
	remote[1].Description = "changed outside Terraform"
	got, diags = syncPostgresIPAccess(ctx, remote[1:], prior)
	require.False(t, diags.HasError())
	require.True(t, got.Equal(ipAccessList(api.PostgresIpAccess{
		Source: "192.0.2.17/24", PortRange: "05432..05432", Description: "changed outside Terraform",
	})))
	got, diags = syncPostgresIPAccess(ctx, remote, ipAccessList())
	require.False(t, diags.HasError())
	require.Len(t, got.Elements(), 2)
	got, diags = syncPostgresIPAccess(ctx, []api.PostgresIpAccess{}, prior)
	require.False(t, diags.HasError())
	require.True(t, got.Equal(ipAccessList()))
	_, diags = syncPostgresIPAccess(ctx, nil, prior)
	require.True(t, diags.HasError())
	got, diags = syncPostgresIPAccess(ctx, nil, types.ListNull(models.IPAccessList{}.ObjectType()))
	require.False(t, diags.HasError())
	require.True(t, got.IsNull())
	got, diags = syncPostgresIPAccess(ctx, remote, types.ListUnknown(models.IPAccessList{}.ObjectType()))
	require.False(t, diags.HasError())
	require.True(t, got.Equal(ipAccessList(remote...)))
}

func TestPostgresIPAccessRetryUsesFreshIDs(t *testing.T) {
	ctx := context.Background()
	client := api.NewClientMock(minimock.NewController(t))
	r := &PostgresServiceResource{client: client}
	desired := ipAccessList(api.PostgresIpAccess{Source: "192.0.2.1", PortRange: "5432", Description: "desired"})
	reads := 0
	client.GetPostgresMock.Set(func(context.Context, string) (*api.Postgres, error) {
		reads++
		if reads == 1 {
			return &api.Postgres{IpAccessList: []api.PostgresIpAccess{
				{Id: "remove-me", Source: "0.0.0.0/0", PortRange: "5432"},
			}}, nil
		}
		return &api.Postgres{IpAccessList: []api.PostgresIpAccess{
			{Id: "created-before-error", Source: "192.0.2.1/32", PortRange: "5432", Description: "desired"},
		}}, nil
	})
	client.UpdatePostgresMock.Set(func(_ context.Context, id string, body api.PostgresUpdate) (*api.Postgres, error) {
		require.Equal(t, "pg-1", id)
		require.Nil(t, body.Tags)
		require.Empty(t, body.Size)
		require.Empty(t, body.HaType)
		require.Equal(t, []api.PostgresIpAccessRuleReference{{Id: "remove-me"}}, body.IpAccessList.Remove)
		require.Empty(t, body.IpAccessList.Add[0].Id)
		return nil, errors.New("response lost after applying changes")
	})
	require.NoError(t, r.updatePostgresIPAccess(ctx, "pg-1", desired))
	require.Equal(t, 2, reads)
	require.Equal(t, uint64(1), client.UpdatePostgresAfterCounter())
}

func TestPostgresIPAccessDescriptionWhitespace(t *testing.T) {
	for _, tc := range []struct {
		name, configured, stored string
	}{
		{"ASCII and NUL", "\x00 \t\r\n\v\foffice\x00 \t\r\n\v\f", "office"},
		{"non-breaking spaces", " \u00a0office\u00a0 ", "\u00a0office\u00a0"},
		{"em spaces", "\t\u2003office\u2003\n", "\u2003office\u2003"},
		{"only whitespace", "\x00 \t\r\n\v\f", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			desired := api.PostgresIpAccess{Source: "192.0.2.1/32", PortRange: "5432", Description: tc.configured}
			remote := desired
			remote.Id = "existing"
			remote.Description = tc.stored
			patch, err := diffPostgresIPAccess([]api.PostgresIpAccess{remote}, []api.PostgresIpAccess{desired})
			require.NoError(t, err)
			require.Nil(t, patch, "an unchanged description must not be patched")
			prior := ipAccessList(desired)
			got, diags := syncPostgresIPAccess(context.Background(), []api.PostgresIpAccess{remote}, prior)
			require.False(t, diags.HasError())
			require.True(t, prior.Equal(got), "preserve configured description: %v", got)
		})
	}
}

// Use the real HTTP client: a client mock cannot detect automatic PATCH replay.
func TestPostgresIPAccessHTTPRecovery(t *testing.T) {
	for _, tc := range []struct {
		name           string
		applied        int
		status         int
		patches, reads int
		wantError      bool
	}{
		{"before mutation", 0, 503, 2, 2, false},
		{"after removal", 1, 503, 2, 2, false},
		{"after first add", 2, 503, 2, 2, false},
		{"after full mutation", 3, 503, 1, 2, false},
		{"lost connection", 3, 0, 1, 2, false},
		{"truncated response", 3, 200, 1, 2, false},
		{"bad request", 0, 400, 1, 1, true},
		{"forbidden", 0, 403, 1, 1, true},
		{"canceled", 0, -1, 1, 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			desired := []api.PostgresIpAccess{
				{Source: "192.0.2.1/32", PortRange: "5432", Description: "Postgres"},
				{Source: "192.0.2.1/32", PortRange: "6432", Description: "PgBouncer"},
			}
			current := []api.PostgresIpAccess{{Id: "old", Source: "0.0.0.0/0", PortRange: "5432"}}
			var mu sync.Mutex
			patches, reads, nextID := 0, 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				switch req.Method {
				case http.MethodGet:
					reads++
				case http.MethodPatch:
					patches++
					var body api.PostgresUpdate
					require.NoError(t, json.NewDecoder(req.Body).Decode(&body))
					require.NotNil(t, body.IpAccessList)
					require.Nil(t, body.Tags)
					if patches == 1 && tc.status == -1 {
						cancel()
						return
					}
					if patches == 1 && tc.applied == 0 {
						http.Error(w, "request failed", tc.status)
						return
					}
					for _, remove := range body.IpAccessList.Remove {
						index := slices.IndexFunc(current, func(rule api.PostgresIpAccess) bool { return rule.Id == remove.Id })
						if index < 0 {
							http.Error(w, "unknown rule ID", http.StatusBadRequest)
							return
						}
						current = slices.Delete(current, index, index+1)
					}
					for i, add := range body.IpAccessList.Add {
						if patches == 1 && i >= tc.applied-1 {
							break
						}
						if slices.ContainsFunc(current, func(rule api.PostgresIpAccess) bool {
							return rule.Source == add.Source && rule.PortRange == add.PortRange
						}) {
							http.Error(w, "duplicate rule", http.StatusBadRequest)
							return
						}
						nextID++
						add.Id = fmt.Sprintf("created-%d", nextID)
						current = append(current, add)
					}
					if patches == 1 {
						switch tc.status {
						case 0:
							conn, _, err := w.(http.Hijacker).Hijack()
							require.NoError(t, err)
							require.NoError(t, conn.Close())
						case http.StatusOK:
							w.Header().Set("Content-Length", "100")
							_, err := w.Write([]byte("{"))
							require.NoError(t, err)
						default:
							http.Error(w, "response lost", tc.status)
						}
						return
					}
				default:
					t.Errorf("unexpected method %s", req.Method)
				}
				require.NoError(t, json.NewEncoder(w).Encode(api.ResponseWithResult[api.Postgres]{
					Result: api.Postgres{Id: "pg-1", IpAccessList: current},
				}))
			}))
			defer server.Close()
			client, err := api.NewClient(api.ClientConfig{
				ApiURL: server.URL, OrganizationID: "org", TokenKey: "key", TokenSecret: "secret",
			})
			require.NoError(t, err)
			r := &PostgresServiceResource{client: client}
			err = r.updatePostgresIPAccess(ctx, "pg-1", ipAccessList(desired...))
			require.Equal(t, tc.wantError, err != nil, "%v", err)
			if tc.status == -1 {
				require.ErrorIs(t, err, context.Canceled)
			}
			mu.Lock()
			defer mu.Unlock()
			require.Equal(t, tc.patches, patches)
			require.Equal(t, tc.reads, reads)
			if !tc.wantError {
				patch, err := diffPostgresIPAccess(current, desired)
				require.NoError(t, err)
				require.Nil(t, patch)
			}
		})
	}
}

func TestPostgresOriginIPAccessCreate(t *testing.T) {
	for _, origin := range []string{"replica", "restore", "replica-failure", "restore-failure"} {
		t.Run(origin, func(t *testing.T) {
			ctx := context.Background()
			client := api.NewClientMock(minimock.NewController(t))
			r := &PostgresServiceResource{client: client}
			var schemaResp resource.SchemaResponse
			r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
			isReplica := origin == "replica" || origin == "replica-failure"
			fail := origin == "replica-failure" || origin == "restore-failure"
			model := gateModel(!isReplica)
			model.IpAccessList = ipAccessList()
			if isReplica {
				model.ReadReplicaOf = types.StringValue("source")
				client.CreatePostgresReadReplicaMock.Set(func(_ context.Context, id string, _ api.PostgresReadReplicaRequest) (*api.Postgres, error) {
					require.Equal(t, "source", id)
					return &api.Postgres{Id: "allocated"}, nil
				})
			} else {
				model.RestoreToPointInTime = types.ObjectValueMust(model.RestoreToPointInTime.AttributeTypes(ctx), map[string]attr.Value{
					"source_id": types.StringValue("source"), "restore_target": types.StringValue("2026-10-01T00:00:00Z"),
				})
				client.RestorePostgresMock.Set(func(_ context.Context, id string, _ api.PostgresRestoreRequest) (*api.Postgres, error) {
					require.Equal(t, "source", id)
					return &api.Postgres{Id: "allocated"}, nil
				})
			}
			pg := &api.Postgres{
				Id: "allocated", Name: "n", Provider: "aws", Region: "us-east-1",
				Size: "m6gd.large", HaType: "none", PostgresVersion: "18", IsPrimary: !isReplica,
				State:        api.PostgresStateRunning,
				IpAccessList: []api.PostgresIpAccess{{Id: "public", Source: "0.0.0.0/0", PortRange: "5432"}},
			}
			client.GetPostgresMock.Expect(ctx, "allocated").Return(pg, nil)
			var patchErr error
			if fail {
				patchErr = errors.New("status: 400, body: firewall update failed")
			} else {
				client.WaitForPostgresStateMock.Set(func(_ context.Context, id string, _ func(string) bool, _ int) error {
					require.Equal(t, "allocated", id)
					require.Equal(t, uint64(1), client.UpdatePostgresAfterCounter(), "apply access list before waiting")
					pg.IpAccessList = []api.PostgresIpAccess{}
					return nil
				})
				client.GetPostgresConfigMock.Expect(ctx, "allocated").Return(&api.PostgresConfig{}, nil)
			}
			client.UpdatePostgresMock.Expect(ctx, "allocated", api.PostgresUpdate{
				IpAccessList: &api.PostgresIpAccessUpdate{Remove: []api.PostgresIpAccessRuleReference{{Id: "public"}}},
			}).Return(nil, patchErr)
			encoded := tfsdk.State{Schema: schemaResp.Schema}
			require.False(t, encoded.Set(ctx, model).HasError())
			resp := resource.CreateResponse{State: tfsdk.State{
				Schema: schemaResp.Schema, Raw: tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil),
			}}
			r.Create(ctx, resource.CreateRequest{
				Plan:   tfsdk.Plan{Schema: schemaResp.Schema, Raw: encoded.Raw},
				Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: encoded.Raw},
			}, &resp)
			require.Equal(t, fail, resp.Diagnostics.HasError(), "%v", resp.Diagnostics)
			if fail {
				require.Contains(t, resp.Diagnostics.Errors()[0].Detail(), "firewall update failed")
			} else {
				var got models.PostgresServiceResourceModel
				require.False(t, resp.State.Get(ctx, &got).HasError())
				require.True(t, got.IpAccessList.Equal(ipAccessList()))
			}
			var id types.String
			require.False(t, resp.State.GetAttribute(ctx, path.Root("id"), &id).HasError())
			require.Equal(t, "allocated", id.ValueString())
		})
	}
}

func TestPostgresReplicaIPAccessUpdate(t *testing.T) {
	ctx := context.Background()
	client := api.NewClientMock(minimock.NewController(t))
	r := &PostgresServiceResource{client: client}
	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	model := gateModel(false)
	model.ReadReplicaOf = types.StringValue("parent")
	model.Tags = mapTags("team", "data")
	model.IpAccessList = ipAccessList(api.PostgresIpAccess{Source: "0.0.0.0/0", PortRange: "5432"})
	prior := tfsdk.State{Schema: schemaResp.Schema}
	require.False(t, prior.Set(ctx, model).HasError())
	model.IpAccessList = ipAccessList()
	planned := tfsdk.State{Schema: schemaResp.Schema}
	require.False(t, planned.Set(ctx, model).HasError())
	plan := tfsdk.Plan{Schema: schemaResp.Schema, Raw: planned.Raw}
	config := tfsdk.Config{Schema: schemaResp.Schema, Raw: planned.Raw}
	planResp := resource.ModifyPlanResponse{Plan: plan}
	r.ModifyPlan(ctx, resource.ModifyPlanRequest{Plan: plan, Config: config, State: prior}, &planResp)
	require.False(t, planResp.Diagnostics.HasError(), "%v", planResp.Diagnostics)
	require.Empty(t, planResp.RequiresReplace)

	pg := &api.Postgres{
		Id: "pg-1", Name: "n", Provider: "aws", Region: "us-east-1", Size: "m6gd.large",
		PostgresVersion: "18", HaType: "none", State: api.PostgresStateRunning, IsPrimary: false,
		Tags:         []api.Tag{{Key: "team", Value: "data"}},
		IpAccessList: []api.PostgresIpAccess{{Id: "public", Source: "0.0.0.0/0", PortRange: "5432"}},
	}
	client.GetPostgresMock.Expect(ctx, "pg-1").Return(pg, nil)
	client.UpdatePostgresMock.Set(func(_ context.Context, id string, body api.PostgresUpdate) (*api.Postgres, error) {
		require.Equal(t, "pg-1", id)
		require.Equal(t, api.PostgresUpdate{
			IpAccessList: &api.PostgresIpAccessUpdate{Remove: []api.PostgresIpAccessRuleReference{{Id: "public"}}},
		}, body)
		pg.IpAccessList = []api.PostgresIpAccess{}
		return pg, nil
	})
	client.GetPostgresConfigMock.Expect(ctx, "pg-1").Return(&api.PostgresConfig{}, nil)
	resp := resource.UpdateResponse{State: prior}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, Config: config, State: prior}, &resp)
	require.False(t, resp.Diagnostics.HasError(), "%v", resp.Diagnostics)
	var got models.PostgresServiceResourceModel
	require.False(t, resp.State.Get(ctx, &got).HasError())
	require.True(t, got.IpAccessList.Equal(ipAccessList()))
	require.True(t, got.Tags.Equal(model.Tags))
}
