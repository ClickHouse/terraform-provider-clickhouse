package api

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestPgConfigMap_UnmarshalsStringValues(t *testing.T) {
	input := []byte(`{"max_connections":"200","work_mem":"64MB"}`)
	var got PgConfigMap
	if err := json.Unmarshal(input, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	want := PgConfigMap{"max_connections": "200", "work_mem": "64MB"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("PgConfigMap mismatch (-want +got):\n%s", diff)
	}
}

func TestPgConfigMap_UnmarshalsNumericValuesAsStrings(t *testing.T) {
	// Server's wire shape is {[key: string]: string | number}. If another
	// client (UI, raw API) wrote a numeric value, we must accept it on read
	// and coerce to string.
	input := []byte(`{"max_connections":200,"work_mem":64,"random_page_cost":1.1}`)
	var got PgConfigMap
	if err := json.Unmarshal(input, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	want := PgConfigMap{
		"max_connections":  "200",
		"work_mem":         "64",
		"random_page_cost": "1.1",
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("PgConfigMap mismatch (-want +got):\n%s", diff)
	}
}

func TestPgConfigMap_UnmarshalsMixedStringAndNumeric(t *testing.T) {
	input := []byte(`{"max_connections":"200","work_mem":64}`)
	var got PgConfigMap
	if err := json.Unmarshal(input, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["max_connections"] != "200" {
		t.Errorf("max_connections = %q; want 200", got["max_connections"])
	}
	if got["work_mem"] != "64" {
		t.Errorf("work_mem = %q; want 64", got["work_mem"])
	}
}

func TestPgConfigMap_UnmarshalsEmptyObject(t *testing.T) {
	var got PgConfigMap
	if err := json.Unmarshal([]byte(`{}`), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty map; got %v", got)
	}
}

func TestPgConfigMap_RejectsBoolValue(t *testing.T) {
	// Bools / nulls would surprise downstream code that expects string. Surface
	// it as an error instead of silently swallowing.
	var got PgConfigMap
	err := json.Unmarshal([]byte(`{"foo":true}`), &got)
	if err == nil {
		t.Errorf("expected error for bool value; got nil (map=%v)", got)
	}
}

func TestPgConfigMap_MarshalsAsPlainStringMap(t *testing.T) {
	m := PgConfigMap{"max_connections": "200"}
	body, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(body) != `{"max_connections":"200"}` {
		t.Errorf("got %q; want plain string map", string(body))
	}
}

func TestPgConfigMap_MarshalsEmptyMapAsEmptyObject(t *testing.T) {
	m := PgConfigMap{}
	body, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(body) != `{}` {
		t.Errorf("got %q; want {}", string(body))
	}
}

func TestPostgresUpdate_DoesNotIncludeNameField(t *testing.T) {
	// Server's PostgresInstancePatchRequestV1 has no `name` field; sending
	// one would silently no-op. Guard against accidental addition.
	body, err := json.Marshal(PostgresUpdate{Size: "r6gd.large"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(body), "name") {
		t.Errorf("PostgresUpdate must not marshal name; got %s", body)
	}
	if !strings.Contains(string(body), `"size":"r6gd.large"`) {
		t.Errorf("PostgresUpdate must include size; got %s", body)
	}
}

func TestPostgresUpdate_OmitsUnsetFields(t *testing.T) {
	body, err := json.Marshal(PostgresUpdate{HaType: "async"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(body), "size") {
		t.Errorf("PostgresUpdate must omit unset size; got %s", body)
	}
	if strings.Contains(string(body), "tags") {
		t.Errorf("PostgresUpdate must omit unset tags; got %s", body)
	}
	if !strings.Contains(string(body), `"haType":"async"`) {
		t.Errorf("PostgresUpdate must include haType; got %s", body)
	}
}

func TestPostgres_OmitsAbsentableFields(t *testing.T) {
	// Hostname, ConnectionString, Username, Password are *string so a nil
	// value gets omitted from outgoing JSON.
	body, err := json.Marshal(Postgres{Id: "x", Name: "n", Provider: "aws", Region: "us-east-1"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, field := range []string{"hostname", "connectionString", "username", "password"} {
		if strings.Contains(string(body), field) {
			t.Errorf("expected %q absent from output; got %s", field, body)
		}
	}
}

func TestPostgresCreate_OmitsEmptyConfigMaps(t *testing.T) {
	body, err := json.Marshal(PostgresCreate{
		Name:     "p",
		Provider: "aws",
		Region:   "us-east-1",
		Size:     "r6gd.large",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(body), "pgConfig") {
		t.Errorf("expected pgConfig absent when nil; got %s", body)
	}
	if strings.Contains(string(body), "pgBouncerConfig") {
		t.Errorf("expected pgBouncerConfig absent when nil; got %s", body)
	}
}

func TestPostgresStateCommandRequest_LowercaseTag(t *testing.T) {
	body, err := json.Marshal(PostgresStateCommandRequest{Command: PostgresCommandRestart})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(body) != `{"command":"restart"}` {
		t.Errorf("got %s; want lowercase command field with restart value", body)
	}
}

func TestPostgresConfig_RoundTripsMixedValueTypes(t *testing.T) {
	// Verify the full PostgresConfig response can be unmarshaled from a wire
	// payload with mixed string and numeric values.
	input := []byte(`{"pgConfig":{"max_connections":200,"work_mem":"64MB"},"pgBouncerConfig":{"default_pool_size":10}}`)
	var got PostgresConfig
	if err := json.Unmarshal(input, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.PgConfig["max_connections"] != "200" {
		t.Errorf("PgConfig[max_connections] = %q; want 200", got.PgConfig["max_connections"])
	}
	if got.PgConfig["work_mem"] != "64MB" {
		t.Errorf("PgConfig[work_mem] = %q; want 64MB", got.PgConfig["work_mem"])
	}
	if got.PgBouncerConfig["default_pool_size"] != "10" {
		t.Errorf("PgBouncerConfig[default_pool_size] = %q; want 10", got.PgBouncerConfig["default_pool_size"])
	}
}

func TestPostgresConfigUpdateResponse_OptionalMessage(t *testing.T) {
	withMsg := []byte(`{"pgConfig":{},"pgBouncerConfig":{},"message":"restart required"}`)
	var gotWith PostgresConfigUpdateResponse
	if err := json.Unmarshal(withMsg, &gotWith); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if gotWith.Message == nil || *gotWith.Message != "restart required" {
		t.Errorf("Message = %v; want pointer to 'restart required'", gotWith.Message)
	}

	withoutMsg := []byte(`{"pgConfig":{},"pgBouncerConfig":{}}`)
	var gotWithout PostgresConfigUpdateResponse
	if err := json.Unmarshal(withoutMsg, &gotWithout); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if gotWithout.Message != nil {
		t.Errorf("Message = %v; want nil when absent from server", *gotWithout.Message)
	}
}

func TestPostgresState_ConstantsMatchWireValues(t *testing.T) {
	// Sanity check: constants must match the verbatim server-side enum.
	// If the server changes a value (e.g., "running" → "active"), this test
	// will continue to pass (it only checks our side). The contract surface
	// here is "the constants we ship match what we believe the server emits."
	cases := []struct {
		constant string
		expected string
	}{
		{PostgresStateCreating, "creating"},
		{PostgresStateRestarting, "restarting"},
		{PostgresStateRunning, "running"},
		{PostgresStateReplayingWal, "replaying_wal"},
		{PostgresStateRestoringBackup, "restoring_backup"},
		{PostgresStateFinalizingRestore, "finalizing_restore"},
		{PostgresStateUnavailable, "unavailable"},
		{PostgresStateDeleting, "deleting"},
	}
	for _, tc := range cases {
		if tc.constant != tc.expected {
			t.Errorf("state constant = %q; want %q", tc.constant, tc.expected)
		}
	}
}
