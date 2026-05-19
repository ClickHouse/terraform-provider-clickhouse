package api

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestPostgresInstanceUpdate_TagsOmittedWhenNil(t *testing.T) {
	update := PostgresInstanceUpdate{
		Size: "m6gd.large",
	}
	data, err := json.Marshal(update)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"tags"`) {
		t.Errorf("expected tags to be omitted from JSON when nil, got: %s", string(data))
	}
}

func TestPostgresInstanceUpdate_TagsIncludedWhenSet(t *testing.T) {
	tags := []Tag{{Key: "env", Value: "prod"}}
	update := PostgresInstanceUpdate{
		Size: "m6gd.large",
		Tags: &tags,
	}
	data, err := json.Marshal(update)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"tags"`) {
		t.Errorf("expected tags to be present in JSON when set, got: %s", string(data))
	}
}

func TestPostgresInstanceUpdate_EmptyTagsSentAsEmptyArray(t *testing.T) {
	tags := []Tag{}
	update := PostgresInstanceUpdate{
		Tags: &tags,
	}
	data, err := json.Marshal(update)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"tags":[]`) {
		t.Errorf("expected empty tags array in JSON, got: %s", string(data))
	}
}

func TestDescribePostgresAPIError_AddsFeatureFlagHintOn403(t *testing.T) {
	err := errors.New("status: 403, body: forbidden")

	description := DescribePostgresAPIError(err)

	if !strings.Contains(description, "FT_ORG_MANAGED_POSTGRES_SERVICES") {
		t.Fatalf("expected feature flag hint in description, got: %s", description)
	}
}

func TestDescribePostgresAPIError_LeavesOtherErrorsUnchanged(t *testing.T) {
	err := errors.New("status: 404, body: not found")

	description := DescribePostgresAPIError(err)

	if description != err.Error() {
		t.Fatalf("expected original error string, got: %s", description)
	}
}
