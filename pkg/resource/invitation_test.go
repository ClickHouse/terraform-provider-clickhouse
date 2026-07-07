package resource

import (
	"context"
	"fmt"
	"testing"

	"github.com/gojuno/minimock/v3"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/api"
)

func TestInvitationRoleSet(t *testing.T) {
	tests := []struct {
		name     string
		ids      []string
		keepNull bool
		want     types.Set
	}{
		{
			name: "builds a set from role IDs (order-independent)",
			ids:  []string{"role-1", "role-2"},
			want: strSetValue("role-1", "role-2"),
		},
		{
			name:     "empty with keepNull returns a null set",
			ids:      []string{},
			keepNull: true,
			want:     types.SetNull(types.StringType),
		},
		{
			name:     "empty without keepNull returns an empty set",
			ids:      []string{},
			keepNull: false,
			want:     strSetValue(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, diags := invitationRoleSet(tt.ids, tt.keepNull)
			if diags.HasError() {
				t.Fatalf("%s unexpected diagnostics: %v", tt.name, diags)
			}
			if !got.Equal(tt.want) {
				t.Errorf("%s set does not match:\ngot  = %v\nwant = %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestInvitationResource_findMemberByEmail(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name       string
		email      string
		members    []api.Member
		membersErr error
		wantUserID string
		wantNil    bool
		wantErr    bool
	}{
		{
			name:  "matches member by email",
			email: "alice@example.com",
			members: []api.Member{
				{UserID: "u-1", Email: "bob@example.com"},
				{UserID: "u-2", Email: "alice@example.com"},
			},
			wantUserID: "u-2",
		},
		{
			name:  "returns nil when no member matches",
			email: "nobody@example.com",
			members: []api.Member{
				{UserID: "u-1", Email: "bob@example.com"},
			},
			wantNil: true,
		},
		{
			name:       "propagates list error",
			email:      "alice@example.com",
			membersErr: fmt.Errorf("status: 500, body: internal error"),
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := minimock.NewController(t)
			mockClient := api.NewClientMock(mc).
				ListMembersMock.
				Expect(ctx).
				Return(tt.members, tt.membersErr)

			r := &InvitationResource{client: mockClient}

			member, err := r.findMemberByEmail(ctx, tt.email)
			if (err != nil) != tt.wantErr {
				t.Fatalf("%s error mismatch:\ngot  = %v\nwant error = %v", tt.name, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if tt.wantNil {
				if member != nil {
					t.Errorf("%s expected nil member, got %v", tt.name, member)
				}
				return
			}
			if member == nil || member.UserID != tt.wantUserID {
				t.Errorf("%s member mismatch:\ngot  = %v\nwant userID = %s", tt.name, member, tt.wantUserID)
			}
		})
	}
}
