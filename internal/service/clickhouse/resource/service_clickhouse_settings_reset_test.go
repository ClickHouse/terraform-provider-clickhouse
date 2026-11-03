package resource

import (
	"context"
	"errors"
	"testing"

	"github.com/gojuno/minimock/v3"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
)

// The API answers 404 "Unknown setting" for a name it no longer accepts; the other settings must still be reset.
func TestServiceClickhouseSettingsResource_Delete_UnknownSettingDoesNotSkipOthers(t *testing.T) {
	ctx := context.Background()
	r, sch := clickhouseSettingsResourceWithSchema(t)

	mc := minimock.NewController(t)
	mock := api.NewClientMock(mc)
	mock.DeleteServiceClickhouseSettingMock.
		When(ctx, "svc-1", "a_removed_setting").
		Then(errors.New(`status: 404, body: {"error":"NOT_FOUND: Unknown setting: a_removed_setting","status":404}`))
	mock.DeleteServiceClickhouseSettingMock.
		When(ctx, "svc-1", "max_query_size").
		Then(nil)
	r.client = mock

	state := clickhouseSettingsRaw(ctx, sch, "svc-1", map[string]string{
		"a_removed_setting": "1",
		"max_query_size":    "262144",
	})
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: sch, Raw: state}}
	r.Delete(ctx, resource.DeleteRequest{State: tfsdk.State{Schema: sch, Raw: state}}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Delete diags: %v", resp.Diagnostics)
	}
}
