package api

import (
	"encoding/json"
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
		if jsonContainsValueAnywhere(t, got, "super-secret") {
			t.Errorf("plaintext password leaked into log output: %s", got)
		}
	})

	t.Run("malformed JSON yields placeholder, not raw bytes", func(t *testing.T) {
		raw := `{"password":"super-secret"`
		got := formatLogBody([]byte(raw))
		if got == "" {
			t.Fatal("expected non-empty output for malformed JSON")
		}
		if jsonContainsValueAnywhere(t, got, "super-secret") {
			t.Errorf("plaintext password leaked through malformed-JSON path: %s", got)
		}
	})

	t.Run("nested response envelope is redacted and pretty-printed", func(t *testing.T) {
		input := `{"result":{"id":"abc","password":"plain"}}`
		got := formatLogBody([]byte(input))
		if jsonContainsValueAnywhere(t, got, "plain") {
			t.Errorf("plaintext password leaked: %s", got)
		}
		// Pretty-printing inserts newlines and indentation.
		if got == input {
			t.Errorf("expected pretty-printed output; got original: %s", got)
		}
	})
}

// jsonContainsPath returns true iff the JSON document contains the (key, value)
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

// jsonContainsValueAnywhere returns true iff the substring appears in the
// serialized form of the document. Useful for sanity checks ("did the secret
// leak through any path").
func jsonContainsValueAnywhere(t *testing.T, doc string, needle string) bool {
	t.Helper()
	return jsonStringContains(doc, needle)
}

func jsonStringContains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
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
