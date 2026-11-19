package resource

// Snapshot of cp-common validator values. Bump in a patch release when the
// server adds new entries.

var postgresCloudProviders = []string{
	"aws",
	"gcp",
}

var postgresVersions = []string{
	"19",
	"18",
	"17",
}

var postgresHaTypes = []string{
	"none",
	"async",
	"sync",
}

// PostgresStandbyCounts lists the standby counts each ha_type accepts, the
// server default first.
var PostgresStandbyCounts = map[string][]int64{
	"none":  {0},
	"async": {1},
	"sync":  {2, 1},
}

const (
	postgresInstanceNameMin = 1
	postgresInstanceNameMax = 50
)

// postgresDefaultPort: server doesn't expose a per-instance port today.
const postgresDefaultPort int64 = 5432

// Lifecycle timeout budgets (seconds).
const (
	postgresDefaultCreateTimeoutSeconds = 30 * 60 // 30m
	postgresDefaultUpdateTimeoutSeconds = 30 * 60 // 30m
)
