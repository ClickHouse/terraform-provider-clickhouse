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

// postgresSizes mirrors the keys of VM_SPECS at ManagedPostgres.ts:203.
// Snapshotted with `grep -oE "'[a-z0-9]+\.[a-z0-9]+'" ManagedPostgres.ts | sort -u`
// = 82 keys at snapshot time.
var postgresSizes = []string{
	"c6gd.16xlarge", "c6gd.2xlarge", "c6gd.4xlarge", "c6gd.8xlarge", "c6gd.large", "c6gd.xlarge",
	"i7i.12xlarge", "i7i.16xlarge", "i7i.24xlarge", "i7i.2xlarge", "i7i.4xlarge", "i7i.8xlarge",
	"i7i.large", "i7i.xlarge",
	"i7ie.12xlarge", "i7ie.18xlarge", "i7ie.24xlarge", "i7ie.2xlarge", "i7ie.3xlarge",
	"i7ie.6xlarge", "i7ie.large", "i7ie.xlarge",
	"i8g.16xlarge", "i8g.24xlarge", "i8g.2xlarge", "i8g.4xlarge", "i8g.8xlarge", "i8g.large", "i8g.xlarge",
	"i8ge.12xlarge", "i8ge.18xlarge", "i8ge.24xlarge", "i8ge.2xlarge", "i8ge.3xlarge",
	"i8ge.6xlarge", "i8ge.large", "i8ge.xlarge",
	"m6gd.16xlarge", "m6gd.2xlarge", "m6gd.4xlarge", "m6gd.8xlarge", "m6gd.large", "m6gd.xlarge",
	"m6id.16xlarge", "m6id.2xlarge", "m6id.4xlarge", "m6id.8xlarge", "m6id.large", "m6id.xlarge",
	"m8gd.16xlarge", "m8gd.2xlarge", "m8gd.4xlarge", "m8gd.8xlarge", "m8gd.large", "m8gd.xlarge",
	"r6gd.12xlarge", "r6gd.16xlarge", "r6gd.2xlarge", "r6gd.4xlarge", "r6gd.8xlarge",
	"r6gd.large", "r6gd.medium", "r6gd.xlarge",
	"r6id.12xlarge", "r6id.16xlarge", "r6id.24xlarge", "r6id.2xlarge", "r6id.32xlarge",
	"r6id.4xlarge", "r6id.8xlarge", "r6id.large", "r6id.xlarge",
	"r8gd.12xlarge", "r8gd.16xlarge", "r8gd.24xlarge", "r8gd.2xlarge", "r8gd.48xlarge",
	"r8gd.4xlarge", "r8gd.8xlarge", "r8gd.large", "r8gd.medium", "r8gd.xlarge",
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

// Lifecycle timeouts (seconds).
// Used as defaults for the timeouts {} block. Generous Create/Update
// budgets cover slow regions and HA-flip resizes; Delete is faster on
// dev (Phase 0 observed <15s) but kept conservative for prod.
const (
	postgresDefaultCreateTimeoutSeconds = 30 * 60 // 30 minutes
	postgresDefaultUpdateTimeoutSeconds = 30 * 60 // 30 minutes
	postgresDefaultDeleteTimeoutSeconds = 10 * 60 // 10 minutes
)
