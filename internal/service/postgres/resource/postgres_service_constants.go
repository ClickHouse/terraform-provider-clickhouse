package resource

// Snapshot of cp-common validator values. Bump in a patch release when the
// server adds new entries.

var postgresCloudProviders = []string{
	"aws",
}

var postgresVersions = []string{
	"18",
	"17",
}

var postgresHaTypes = []string{
	"none",
	"async",
	"sync",
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
