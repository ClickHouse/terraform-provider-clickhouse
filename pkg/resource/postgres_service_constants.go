//go:build alpha

package resource

// Snapshot of validator constants from cp-common protocol definitions
// (control-plane/packages/cp-common/src/protocol/postgres/ManagedPostgres.ts).
// When the server adds new values, bump this list in a follow-up patch
// release; users on the prior provider release see a plan-time validation
// error against the new value until they upgrade.

// postgresCloudProviders mirrors CLOUD_PROVIDERS at ManagedPostgres.ts:90.
// Server today is AWS-only; GCP/Azure marked "coming soon" in source.
var postgresCloudProviders = []string{
	"aws",
}

// postgresVersions mirrors PG_VERSIONS at ManagedPostgres.ts:185.
// Major versions only; server picks the patch release.
var postgresVersions = []string{
	"18",
	"17",
	"16",
}

// postgresHaTypes mirrors POSTGRES_HA_TYPES at ManagedPostgres.ts:84.
var postgresHaTypes = []string{
	"none",
	"async",
	"sync",
}

// postgresInstanceNameMin / postgresInstanceNameMax mirror
// MIN_INSTANCE_NAME_LENGTH / MAX_INSTANCE_NAME_LENGTH at ValidationUtils.ts:354-355.
const (
	postgresInstanceNameMin = 1
	postgresInstanceNameMax = 50
)

// postgresReservedTagPrefix is the chc_ prefix the server reserves for
// system-managed tags (ManagedPostgresV1.ts:222). User-supplied tags whose
// key starts with this prefix are rejected at plan time; tags returned
// from the server with this prefix are filtered out during Read.
const postgresReservedTagPrefix = "chc_"

// postgresDefaultPort is the listening port the server uses today.
// Hardcoded into computed state because the server does not expose a
// per-instance port in PostgresInstanceV1. Reassess when/if the server
// gains a port field.
const postgresDefaultPort int64 = 5432

// Lifecycle timeout budgets (seconds). Generous Create/Update budgets
// cover slow regions and HA-flip resizes; Delete returns fast in practice
// but the budget stays wide for safety.
const (
	postgresDefaultCreateTimeoutSeconds = 30 * 60 // 30 minutes
	postgresDefaultUpdateTimeoutSeconds = 30 * 60 // 30 minutes
	postgresDefaultDeleteTimeoutSeconds = 10 * 60 // 10 minutes
)
