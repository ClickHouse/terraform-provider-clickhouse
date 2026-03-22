package api

import (
	"encoding/json"
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
