package api

import (
	"encoding/json"
	"fmt"
)

// Postgres instance state values returned by the server.
// Mirrors ManagedPostgresInstanceStatuses in
// packages/cp-common/src/protocol/postgres/ManagedPostgres.ts:59-68.
const (
	PostgresStateCreating          = "creating"
	PostgresStateRestarting        = "restarting"
	PostgresStateRunning           = "running"
	PostgresStateReplayingWal      = "replaying_wal"
	PostgresStateRestoringBackup   = "restoring_backup"
	PostgresStateFinalizingRestore = "finalizing_restore"
	PostgresStateUnavailable       = "unavailable"
	PostgresStateDeleting          = "deleting"
)

// State command values accepted by PATCH /postgres/{id}/state.
// Mirrors ManagedPostgresInstanceCommands in
// packages/cp-common/src/protocol/postgres/ManagedPostgres.ts:55-57.
const (
	PostgresCommandRestart    = "restart"
	PostgresCommandPromote    = "promote"
	PostgresCommandSwitchover = "switchover"
)

// PgConfigMap is the Go-side representation of the server's
// `pgConfig` / `pgBouncerConfig` shape `{[key: string]: string | number}`.
// Outbound: marshals as a plain string map.
// Inbound: accepts mixed string and numeric values; numeric values are
// coerced to their JSON string form (preserves precision via json.Number).
type PgConfigMap map[string]string

func (m *PgConfigMap) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if *m == nil {
		*m = make(PgConfigMap, len(raw))
	}
	for k, v := range raw {
		var s string
		if err := json.Unmarshal(v, &s); err == nil {
			(*m)[k] = s
			continue
		}
		var n json.Number
		if err := json.Unmarshal(v, &n); err == nil {
			(*m)[k] = n.String()
			continue
		}
		return fmt.Errorf("pgConfig key %q: unsupported value type %s", k, string(v))
	}
	return nil
}

// Postgres mirrors PostgresInstanceV1 from the server's OpenAPI definition
// (apps/openapi/src/protocol/v1/ManagedPostgresV1.ts:60-70).
//
// storageSize is deliberately omitted — it is DEPRECATED server-side and the
// resource does not expose it.
type Postgres struct {
	Id               string  `json:"id,omitempty"`
	Name             string  `json:"name"`
	Provider         string  `json:"provider"`
	Region           string  `json:"region"`
	PostgresVersion  string  `json:"postgresVersion,omitempty"`
	Size             string  `json:"size,omitempty"`
	HaType           string  `json:"haType,omitempty"`
	State            string  `json:"state,omitempty"`
	CreatedAt        string  `json:"createdAt,omitempty"`
	IsPrimary        *bool   `json:"isPrimary,omitempty"`
	Hostname         *string `json:"hostname,omitempty"`
	ConnectionString *string `json:"connectionString,omitempty"`
	Username         *string `json:"username,omitempty"`
	Password         *string `json:"password,omitempty"`
	Tags             []Tag   `json:"tags,omitempty"`
}

// PostgresListItem mirrors the abbreviated form returned by GET /postgres.
// The server returns the full instance shape today, but we model the listing
// surface separately so callers don't accidentally depend on fields the server
// may stop emitting in list endpoints.
type PostgresListItem struct {
	Id              string `json:"id"`
	Name            string `json:"name"`
	Provider        string `json:"provider"`
	Region          string `json:"region"`
	PostgresVersion string `json:"postgresVersion,omitempty"`
	Size            string `json:"size,omitempty"`
	HaType          string `json:"haType,omitempty"`
	State           string `json:"state"`
	CreatedAt       string `json:"createdAt"`
	IsPrimary       bool   `json:"isPrimary"`
	Tags            []Tag  `json:"tags,omitempty"`
}

// PostgresCreate is the POST /postgres request body (PostgresInstancePostRequestV1).
type PostgresCreate struct {
	Name            string      `json:"name"`
	Provider        string      `json:"provider"`
	Region          string      `json:"region"`
	Size            string      `json:"size"`
	PostgresVersion string      `json:"postgresVersion,omitempty"`
	HaType          string      `json:"haType,omitempty"`
	Tags            []Tag       `json:"tags,omitempty"`
	PgConfig        PgConfigMap `json:"pgConfig,omitempty"`
	PgBouncerConfig PgConfigMap `json:"pgBouncerConfig,omitempty"`
}

// PostgresUpdate is the PATCH /postgres/{id} request body
// (PostgresInstancePatchRequestV1). The server accepts ONLY these three
// fields; anything else returns 400. In particular, `name` is intentionally
// absent — the server's patch shape has no `name` field.
type PostgresUpdate struct {
	Size   string `json:"size,omitempty"`
	HaType string `json:"haType,omitempty"`
	Tags   []Tag  `json:"tags,omitempty"`
}

// PostgresRestoreRequest is the POST /postgres/{id}/restoredService body
// (PostgresInstanceRestoreRequestV1).
type PostgresRestoreRequest struct {
	Name            string      `json:"name"`
	RestoreTarget   string      `json:"restoreTarget"`
	PgConfig        PgConfigMap `json:"pgConfig,omitempty"`
	PgBouncerConfig PgConfigMap `json:"pgBouncerConfig,omitempty"`
	Tags            []Tag       `json:"tags,omitempty"`
}

// PostgresReadReplicaRequest is the POST /postgres/{id}/readReplica body
// (PostgresInstanceReadReplicaRequestV1).
type PostgresReadReplicaRequest struct {
	Name            string      `json:"name"`
	PgConfig        PgConfigMap `json:"pgConfig,omitempty"`
	PgBouncerConfig PgConfigMap `json:"pgBouncerConfig,omitempty"`
	Tags            []Tag       `json:"tags,omitempty"`
}

// PostgresStateCommandRequest is the PATCH /postgres/{id}/state body
// (PostgresInstanceStateResourceV1). The lowercase JSON tag is mandatory.
type PostgresStateCommandRequest struct {
	Command string `json:"command"`
}

// PostgresPassword is the PATCH /postgres/{id}/password body and response
// (PostgresInstancePasswordResourceV1).
//
// On request: nil / empty → server generates and returns plaintext in response.
// User-supplied → server adopts it and returns an empty body (Password=nil).
//
// On response: nil means user-supplied. Populated means server-generated.
type PostgresPassword struct {
	Password *string `json:"password,omitempty"`
}

// PostgresConfig mirrors postgresInstanceConfigResourceV1 — the body of
// GET / POST / PATCH on /postgres/{id}/config.
type PostgresConfig struct {
	PgConfig        PgConfigMap `json:"pgConfig"`
	PgBouncerConfig PgConfigMap `json:"pgBouncerConfig"`
}

// PostgresConfigUpdateResponse mirrors
// PostgresInstanceUpdateConfigResponseResourceV1 — the response of
// POST / PATCH on /postgres/{id}/config. Message is the server's hint about
// whether a restart is required for changes to take effect; the resource
// surfaces it as a warning diagnostic.
type PostgresConfigUpdateResponse struct {
	PgConfig        PgConfigMap `json:"pgConfig"`
	PgBouncerConfig PgConfigMap `json:"pgBouncerConfig"`
	Message         *string     `json:"message,omitempty"`
}
