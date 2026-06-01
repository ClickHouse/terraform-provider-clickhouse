package api

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestRedactSensitiveBody(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "empty body unchanged",
			input: "",
			want:  "",
		},
		{
			name:  "non-sensitive payload unchanged",
			input: `{"name":"foo","region":"us-east-1"}`,
			want:  `{"name":"foo","region":"us-east-1"}`,
		},
		{
			name:  "top-level password redacted",
			input: `{"password":"plaintext-secret"}`,
			want:  `{"password":"REDACTED"}`,
		},
		{
			name:  "top-level newPassword redacted",
			input: `{"newPassword":"plaintext-secret"}`,
			want:  `{"newPassword":"REDACTED"}`,
		},
		{
			name:  "top-level newPasswordHash redacted",
			input: `{"newPasswordHash":"deadbeef"}`,
			want:  `{"newPasswordHash":"REDACTED"}`,
		},
		{
			name:  "top-level newDoubleSha1Hash redacted",
			input: `{"newDoubleSha1Hash":"deadbeef"}`,
			want:  `{"newDoubleSha1Hash":"REDACTED"}`,
		},
		{
			name:  "top-level password_wo redacted",
			input: `{"password_wo":"plaintext-secret"}`,
			want:  `{"password_wo":"REDACTED"}`,
		},
		{
			name:  "top-level tokenSecret redacted",
			input: `{"tokenSecret":"plaintext-secret"}`,
			want:  `{"tokenSecret":"REDACTED"}`,
		},
		{
			name:  "top-level connectionString redacted (URI with embedded password)",
			input: `{"connectionString":"postgresql://default:Secret123@host:5432/db?channel_binding=require"}`,
			want:  `{"connectionString":"REDACTED"}`,
		},
		{
			name:  "snake_case connection_string redacted",
			input: `{"connection_string":"postgresql://u:p@h/d"}`,
			want:  `{"connection_string":"REDACTED"}`,
		},
		{
			name:  "Postgres create response: password and connectionString both redacted",
			input: `{"result":{"id":"pg-1","password":"Secret123","connectionString":"postgresql://default:Secret123@host:5432/db"}}`,
			want:  `{"result":{"id":"pg-1","password":"REDACTED","connectionString":"REDACTED"}}`,
		},
		{
			name:  "secrets container redacted to scalar",
			input: `{"secrets":{"username":"u","password":"p"}}`,
			want:  `{"secrets":"REDACTED"}`,
		},
		{
			name:  "credentials container redacted to scalar",
			input: `{"credentials":{"accessKeyId":"AKIA","accessKeySecret":"shh"}}`,
			want:  `{"credentials":"REDACTED"}`,
		},
		{
			name:  "sensitive key nested under result envelope",
			input: `{"result":{"id":"abc","password":"plaintext"}}`,
			want:  `{"result":{"id":"abc","password":"REDACTED"}}`,
		},
		{
			name:  "array of objects: each element's sensitive key redacted",
			input: `{"items":[{"id":"a","password":"p1"},{"id":"b","password":"p2"}]}`,
			want:  `{"items":[{"id":"a","password":"REDACTED"},{"id":"b","password":"REDACTED"}]}`,
		},
		{
			name:  "mixed sensitive and non-sensitive at same level",
			input: `{"name":"foo","password":"p","region":"us-east-1"}`,
			want:  `{"name":"foo","password":"REDACTED","region":"us-east-1"}`,
		},
		{
			name:  "non-string sensitive value still redacted",
			input: `{"password":null}`,
			want:  `{"password":"REDACTED"}`,
		},
		{
			name:  "non-sensitive bool passes through",
			input: `{"enabled":true,"disabled":false}`,
			want:  `{"enabled":true,"disabled":false}`,
		},
		{
			name:  "non-sensitive number passes through",
			input: `{"count":42,"ratio":1.5}`,
			want:  `{"count":42,"ratio":1.5}`,
		},
		{
			name:  "non-sensitive null passes through",
			input: `{"optional":null}`,
			want:  `{"optional":null}`,
		},
		{
			name:  "malformed JSON returns placeholder",
			input: `{"password":`,
			want:  `"<unparseable, redacted>"`,
		},
		{
			name:  "non-object JSON returned as-is when no sensitive content",
			input: `["a","b","c"]`,
			want:  `["a","b","c"]`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := redactSensitiveBody([]byte(tc.input))

			if tc.input == "" {
				if string(got) != "" {
					t.Errorf("expected empty output for empty input; got %q", string(got))
				}
				return
			}

			// Compare as decoded JSON to be robust against map key ordering.
			var gotAny, wantAny interface{}
			if err := json.Unmarshal(got, &gotAny); err != nil {
				t.Fatalf("redacted output not valid JSON: %v (got=%q)", err, string(got))
			}
			if err := json.Unmarshal([]byte(tc.want), &wantAny); err != nil {
				t.Fatalf("want literal not valid JSON: %v (want=%q)", err, tc.want)
			}
			if diff := cmp.Diff(wantAny, gotAny); diff != "" {
				t.Errorf("redactSensitiveBody() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestFormatLogBody(t *testing.T) {
	t.Run("empty body produces empty string", func(t *testing.T) {
		if got := formatLogBody(nil); got != "" {
			t.Errorf("expected empty string for nil input; got %q", got)
		}
		if got := formatLogBody([]byte{}); got != "" {
			t.Errorf("expected empty string for empty input; got %q", got)
		}
	})

	t.Run("sensitive value is redacted before formatting", func(t *testing.T) {
		got := formatLogBody([]byte(`{"password":"super-secret"}`))
		if got == "" {
			t.Fatal("expected non-empty formatted output")
		}
		if !jsonContainsPath(t, got, "password", "REDACTED") {
			t.Errorf("expected redacted password in formatted output; got: %s", got)
		}
		if strings.Contains(got, "super-secret") {
			t.Errorf("plaintext password leaked into log output: %s", got)
		}
	})

	t.Run("malformed JSON yields placeholder, not raw bytes", func(t *testing.T) {
		raw := `{"password":"super-secret"`
		got := formatLogBody([]byte(raw))
		if got == "" {
			t.Fatal("expected non-empty output for malformed JSON")
		}
		if strings.Contains(got, "super-secret") {
			t.Errorf("plaintext password leaked through malformed-JSON path: %s", got)
		}
	})

	t.Run("nested response envelope is redacted and pretty-printed", func(t *testing.T) {
		input := `{"result":{"id":"abc","password":"plain"}}`
		got := formatLogBody([]byte(input))
		if strings.Contains(got, "plain") {
			t.Errorf("plaintext password leaked: %s", got)
		}
		// Pretty-printing inserts newlines and indentation.
		if got == input {
			t.Errorf("expected pretty-printed output; got original: %s", got)
		}
	})

	t.Run("connection string with embedded password does not leak", func(t *testing.T) {
		// Realistic Postgres create response shape. Synthetic test secret;
		// the whole point of this test is to verify it gets REDACTED before
		// it can reach a log sink.
		secret := "Hunter2-Aa1!"
		input := `{"result":{"id":"pg-1","password":"` + secret + `","connectionString":"postgresql://default:` + secret + `@host:5432/db?channel_binding=require"}}` //nolint:gosec // synthetic test fixture
		got := formatLogBody([]byte(input))
		if strings.Contains(got, secret) {
			t.Errorf("plaintext secret leaked through connectionString: %s", got)
		}
	})
}

// jsonContainsPath returns true if the JSON document contains the (key, value)
// pair anywhere in its tree.
func jsonContainsPath(t *testing.T, doc string, key, value string) bool {
	t.Helper()
	var v interface{}
	if err := json.Unmarshal([]byte(doc), &v); err != nil {
		t.Fatalf("doc not valid JSON: %v", err)
	}
	return walkFind(v, func(k string, val interface{}) bool {
		s, ok := val.(string)
		return ok && k == key && s == value
	})
}

func walkFind(v interface{}, predicate func(k string, val interface{}) bool) bool {
	switch t := v.(type) {
	case map[string]interface{}:
		for k, child := range t {
			if predicate(k, child) {
				return true
			}
			if walkFind(child, predicate) {
				return true
			}
		}
	case []interface{}:
		for _, child := range t {
			if walkFind(child, predicate) {
				return true
			}
		}
	}
	return false
}
