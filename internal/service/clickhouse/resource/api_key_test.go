package resource

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/gojuno/minimock/v3"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickhouse/resource/models"
)

func TestApiKeyResource_syncApiKeyState(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		state       models.ApiKeyResourceModel
		response    *api.ApiKey
		responseErr error
		wantErr     bool
		wantState   models.ApiKeyResourceModel
	}{
		{
			name: "maps fields and preserves secret",
			state: models.ApiKeyResourceModel{
				ID:        types.StringValue("key-1"),
				KeySecret: types.StringValue("preserved-secret"),
			},
			response: &api.ApiKey{
				ID:        "key-1",
				Name:      "monitoring",
				State:     "enabled",
				KeySuffix: "wxyz",
				ExpireAt:  "2027-01-01T00:00:00Z",
			},
			wantState: models.ApiKeyResourceModel{
				ID:           types.StringValue("key-1"),
				KeyID:        types.StringValue("key-1"),
				Name:         types.StringValue("monitoring"),
				State:        types.StringValue("enabled"),
				ExpireAt:     types.StringValue("2027-01-01T00:00:00Z"),
				KeySuffix:    types.StringValue("wxyz"),
				KeySecret:    types.StringValue("preserved-secret"),
				IpAccessList: types.ListNull(models.IPAccessList{}.ObjectType()),
			},
		},
		{
			// After `terraform import` the state carries only the ID (passthrough).
			// Read hydrates the rest; key_secret has no prior value and the API
			// never returns it, so it stays empty — the documented import limitation.
			name: "import with no prior secret leaves key_secret empty",
			state: models.ApiKeyResourceModel{
				ID:        types.StringValue("key-2"),
				KeySecret: types.StringNull(),
			},
			response: &api.ApiKey{
				ID:        "key-2",
				Name:      "imported",
				State:     "enabled",
				KeySuffix: "abcd",
			},
			wantState: models.ApiKeyResourceModel{
				ID:           types.StringValue("key-2"),
				KeyID:        types.StringValue("key-2"),
				Name:         types.StringValue("imported"),
				State:        types.StringValue("enabled"),
				ExpireAt:     types.StringNull(),
				KeySuffix:    types.StringValue("abcd"),
				KeySecret:    types.StringNull(),
				IpAccessList: types.ListNull(models.IPAccessList{}.ObjectType()),
			},
		},
		{
			name:        "propagates api error",
			state:       models.ApiKeyResourceModel{ID: types.StringValue("key-1")},
			responseErr: fmt.Errorf("status: 500, body: internal error"),
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := minimock.NewController(t)

			apiClientMock := api.NewClientMock(mc).
				GetApiKeyMock.
				Expect(ctx, tt.state.ID.ValueString()).
				Return(tt.response, tt.responseErr)

			r := &ApiKeyResource{client: apiClientMock}

			_, err := r.syncApiKeyState(ctx, &tt.state)

			if (err != nil) != tt.wantErr {
				t.Errorf("%s error mismatch:\ngot  = %v\nwant error = %v", tt.name, err, tt.wantErr)
			}
			if !tt.wantErr && !reflect.DeepEqual(tt.state, tt.wantState) {
				t.Errorf("%s state mismatch:\ngot  = %+v\nwant = %+v", tt.name, tt.state, tt.wantState)
			}
		})
	}
}

func TestApiKeyResource_ipAccessListFromPlan(t *testing.T) {
	ctx := context.Background()
	objType := models.IPAccessList{}.ObjectType()

	entry := models.IPAccessList{
		Source:      types.StringValue("10.0.0.0/8"),
		Description: types.StringValue("vpc"),
	}
	list, d := types.ListValue(objType, []attr.Value{entry.ObjectValue()})
	if d.HasError() {
		t.Fatalf("build list: %v", d)
	}

	got, diags := ipAccessListFromPlan(ctx, list)
	if diags.HasError() {
		t.Fatalf("ipAccessListFromPlan: %v", diags)
	}
	want := []api.IpAccessListEntry{{Source: "10.0.0.0/8", Description: "vpc"}}
	if got == nil || !reflect.DeepEqual(*got, want) {
		t.Fatalf("got %+v want %+v", got, want)
	}

	// null list -> non-nil empty slice, so the request sends "ipAccessList":[]
	// and the server clears any prior entries.
	gotEmpty, diags := ipAccessListFromPlan(ctx, types.ListNull(objType))
	if diags.HasError() {
		t.Fatalf("null case: %v", diags)
	}
	if gotEmpty == nil || len(*gotEmpty) != 0 {
		t.Fatalf("expected non-nil empty slice for null list, got %+v", gotEmpty)
	}
}

// TestApiKeyResource_clearPathMarshalsExplicitly locks the clear-path fix:
// clearing expire_at/ip_access must marshal to explicit JSON so the
// server clears prior values instead of retaining them (omitempty would
// drop the fields and cause "inconsistent result after apply").
func TestApiKeyResource_clearPathMarshalsExplicitly(t *testing.T) {
	ctx := context.Background()

	plan := models.ApiKeyResourceModel{
		Name:         types.StringValue("monitoring"),
		ExpireAt:     types.StringNull(),
		IpAccessList: types.ListNull(models.IPAccessList{}.ObjectType()),
	}

	req, diags := planToUpdateRequest(ctx, plan)
	if diags.HasError() {
		t.Fatalf("planToUpdateRequest: %v", diags)
	}

	got, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	body := string(got)
	if !strings.Contains(body, `"expireAt":null`) {
		t.Fatalf("expected explicit \"expireAt\":null, got %s", body)
	}
	if !strings.Contains(body, `"ipAccessList":[]`) {
		t.Fatalf("expected explicit \"ipAccessList\":[], got %s", body)
	}
}
