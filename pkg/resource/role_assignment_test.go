//go:build alpha

package resource

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/api"
	"github.com/gojuno/minimock/v3"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestRoleAssignmentResource_syncAssignmentState(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		state       RoleAssignmentModel
		response    *api.RBACRole
		responseErr error
		wantErr     bool
		wantState   RoleAssignmentModel
	}{
		{
			name: "splits actors into user IDs and API key IDs",
			state: RoleAssignmentModel{
				ID:        types.StringValue("role-1"),
				RoleID:    types.StringValue("role-1"),
				UserIDs:   types.SetNull(types.StringType),
				APIKeyIDs: types.SetNull(types.StringType),
			},
			response: &api.RBACRole{
				ID:     "role-1",
				Actors: []string{"user/user-1", "apiKey/key-1"},
			},
			wantState: RoleAssignmentModel{
				ID:        types.StringValue("role-1"),
				RoleID:    types.StringValue("role-1"),
				UserIDs:   strSetValue("user-1"),
				APIKeyIDs: strSetValue("key-1"),
			},
		},
		{
			name: "preserves null for empty actors when state had null",
			state: RoleAssignmentModel{
				ID:        types.StringValue("role-1"),
				RoleID:    types.StringValue("role-1"),
				UserIDs:   types.SetNull(types.StringType),
				APIKeyIDs: types.SetNull(types.StringType),
			},
			response: &api.RBACRole{
				ID:     "role-1",
				Actors: []string{},
			},
			wantState: RoleAssignmentModel{
				ID:        types.StringValue("role-1"),
				RoleID:    types.StringValue("role-1"),
				UserIDs:   types.SetNull(types.StringType),
				APIKeyIDs: types.SetNull(types.StringType),
			},
		},
		{
			name:        "propagates API error",
			state:       RoleAssignmentModel{RoleID: types.StringValue("role-1")},
			responseErr: fmt.Errorf("status: 500, body: internal error"),
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := minimock.NewController(t)

			apiClientMock := api.NewClientMock(mc).
				GetRoleMock.
				Expect(ctx, tt.state.RoleID.ValueString()).
				Return(tt.response, tt.responseErr)

			r := &RoleAssignmentResource{client: apiClientMock}

			_, err := r.syncAssignmentState(ctx, &tt.state)

			if (err != nil) != tt.wantErr {
				t.Errorf("%s error does not match:\ngot  = %v\nwant error = %v", tt.name, err, tt.wantErr)
			}

			if !tt.wantErr && !reflect.DeepEqual(tt.state, tt.wantState) {
				t.Errorf("%s state does not match:\ngot  = %v\nwant = %v", tt.name, tt.state, tt.wantState)
			}
		})
	}
}

func TestParseActors(t *testing.T) {
	tests := []struct {
		name         string
		actors       []string
		wantUsers    []string
		wantAPIKeys  []string
		wantWarnings int
	}{
		{
			name:        "mixed actors are split correctly",
			actors:      []string{"user/user-1", "apiKey/key-1", "user/user-2"},
			wantUsers:   []string{"user-1", "user-2"},
			wantAPIKeys: []string{"key-1"},
		},
		{
			name:         "actor with no separator emits warning and is ignored",
			actors:       []string{"malformed"},
			wantUsers:    []string{},
			wantAPIKeys:  []string{},
			wantWarnings: 1,
		},
		{
			name:         "actor with unknown type emits warning and is ignored",
			actors:       []string{"service/svc-1"},
			wantUsers:    []string{},
			wantAPIKeys:  []string{},
			wantWarnings: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			users, apiKeys, diags := parseActors(tt.actors)

			if !reflect.DeepEqual(users, tt.wantUsers) {
				t.Errorf("%s user IDs do not match:\ngot  = %v\nwant = %v", tt.name, users, tt.wantUsers)
			}

			if !reflect.DeepEqual(apiKeys, tt.wantAPIKeys) {
				t.Errorf("%s API key IDs do not match:\ngot  = %v\nwant = %v", tt.name, apiKeys, tt.wantAPIKeys)
			}

			if diags.HasError() {
				t.Errorf("%s parseActors should never produce errors, got: %v", tt.name, diags)
			}

			if len(diags) != tt.wantWarnings {
				t.Errorf("%s unexpected warning count:\ngot  = %d\nwant = %d", tt.name, len(diags), tt.wantWarnings)
			}
		})
	}
}

// strSetValue builds a types.Set of strings for use in tests.
func strSetValue(strs ...string) types.Set {
	values := make([]attr.Value, len(strs))
	for i, s := range strs {
		values[i] = types.StringValue(s)
	}
	s, _ := types.SetValue(types.StringType, values)
	return s
}
