//go:build alpha

package resource

// Snapshot of validator constants from cp-common protocol definitions.
//
// Source of truth (read at Phase 2 implementation time):
//
//	control-plane/packages/cp-common/src/protocol/postgres/ManagedPostgres.ts
//
// When the server adds new values, bump this list in a follow-up patch
// release. Users on the prior provider release will see a plan-time
// validation error against the new value; documented as a known limitation
// in descriptions/postgres_service.md.

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

// Note: an earlier alpha pinned a `postgresSizes` snapshot of the 82 VM_SPECS
// keys at ManagedPostgres.ts:203 as a client-side OneOf validator on `size`.
// Removed in PR review — the maintenance burden (provider patch needed on
// every new AWS instance family) outweighed the plan-time error benefit for
// what is the most-frequently-changed attribute. The schema now matches the
// region attribute: pass through to the server, which rejects invalid sizes
// with HTTP 400 at apply time.

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

// Lifecycle timeouts (seconds).
// Used as defaults for the timeouts {} block. Generous Create/Update
// budgets cover slow regions and HA-flip resizes; Delete is faster on
// dev (Phase 0 observed <15s) but kept conservative for prod.
const (
	postgresDefaultCreateTimeoutSeconds = 30 * 60 // 30 minutes
	postgresDefaultUpdateTimeoutSeconds = 30 * 60 // 30 minutes
	postgresDefaultDeleteTimeoutSeconds = 10 * 60 // 10 minutes
)
