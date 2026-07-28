package clickstack

import "github.com/hashicorp/terraform-plugin-framework/diag"

// addNotConfiguredError reports the shared "ClickStack not configured" error
// emitted by every clickhouse_clickstack_* resource and data source whose
// Configure runs without a ClickStack client. kind is "resource" or
// "data source".
func addNotConfiguredError(diags *diag.Diagnostics, kind string) {
	diags.AddError("ClickStack not configured",
		"This "+kind+" requires ClickStack credentials. For self-hosted ClickStack, set clickstack_endpoint and "+
			"clickstack_api_key on the provider (or the CLICKSTACK_ENDPOINT / CLICKSTACK_API_KEY environment variables). "+
			"For ClickStack on ClickHouse Cloud, set clickstack_service_id (or CLICKSTACK_SERVICE_ID) together with "+
			"the ClickHouse Cloud credentials (organization_id, token_key, token_secret).")
}
