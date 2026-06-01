package api

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Postgres instance state values. Mirrors ManagedPostgresInstanceStatuses
// in packages/cp-common/src/protocol/postgres/ManagedPostgres.ts:59-68.
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

// PgConfigMap mirrors the server's `pgConfig` / `pgBouncerConfig` shape
// `{[key: string]: string | number}`. Marshals as a plain string map;
// unmarshal accepts mixed string and numeric values and coerces numbers
// to their JSON string form via json.Number.
type PgConfigMap map[string]string

func (m *PgConfigMap) UnmarshalJSON(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var raw map[string]any
	if err := dec.Decode(&raw); err != nil {
		return err
	}
	if *m == nil {
		*m = make(PgConfigMap, len(raw))
	}
	for k, v := range raw {
		switch val := v.(type) {
		case string:
			(*m)[k] = val
		case json.Number:
			(*m)[k] = val.String()
		default:
			return fmt.Errorf("pgConfig key %q: unsupported value type %T", k, v)
		}
	}
	return nil
}

// Postgres mirrors PostgresInstanceV1 (ManagedPostgresV1.ts:60-70).
// storageSize is omitted intentionally — DEPRECATED server-side.
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
	Username         string  `json:"username,omitempty"`
	Password         string  `json:"password,omitempty"`
	Tags             []Tag   `json:"tags,omitempty"`
}

// PostgresListItem is the abbreviated GET /postgres response item. Modeled
// separately from Postgres so callers don't depend on fields the server may
// stop emitting in list endpoints.
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

// PostgresUpdate is the PATCH /postgres/{id} body. Server accepts ONLY
// size / haType / tags; `name` is intentionally absent (no field in the
// server schema). Tags is *[]Tag so callers can distinguish:
//
//	nil       -> field omitted; server leaves existing tags alone
//	&[]Tag{}  -> server clears all tags
//	&[]Tag{…} -> server replaces with these
type PostgresUpdate struct {
	Size   string `json:"size,omitempty"`
	HaType string `json:"haType,omitempty"`
	Tags   *[]Tag `json:"tags,omitempty"`
}

// PostgresRestoreRequest is the POST /postgres/{id}/restoredService body.
type PostgresRestoreRequest struct {
	Name            string      `json:"name"`
	RestoreTarget   string      `json:"restoreTarget"`
	PgConfig        PgConfigMap `json:"pgConfig,omitempty"`
	PgBouncerConfig PgConfigMap `json:"pgBouncerConfig,omitempty"`
	Tags            []Tag       `json:"tags,omitempty"`
}

// PostgresReadReplicaRequest is the POST /postgres/{id}/readReplica body.
type PostgresReadReplicaRequest struct {
	Name            string      `json:"name"`
	PgConfig        PgConfigMap `json:"pgConfig,omitempty"`
	PgBouncerConfig PgConfigMap `json:"pgBouncerConfig,omitempty"`
	Tags            []Tag       `json:"tags,omitempty"`
}

// PostgresPassword is the PATCH /postgres/{id}/password body and response.
// Request: "" → server generates one; set → server adopts. Response: "" means
// the client supplied a value; populated means server-generated.
type PostgresPassword struct {
	Password string `json:"password,omitempty"`
}

// PostgresConfig is the GET /postgres/{id}/config response and POST body.
type PostgresConfig struct {
	PgConfig        PgConfigMap `json:"pgConfig"`
	PgBouncerConfig PgConfigMap `json:"pgBouncerConfig"`
}

// PostgresConfigUpdateResponse is the POST /config response. Message carries
// the server's restart-required hint when applicable.
type PostgresConfigUpdateResponse struct {
	PgConfig        PgConfigMap `json:"pgConfig"`
	PgBouncerConfig PgConfigMap `json:"pgBouncerConfig"`
	Message         *string     `json:"message,omitempty"`
}
