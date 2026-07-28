package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newCloudTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c, err := NewCloud(srv.URL, "org1", "svc1", "key1", "secret1", srv.Client())
	if err != nil {
		t.Fatalf("NewCloud: %v", err)
	}
	return c
}

// TestCloudRequestShape guards the Cloud OpenAPI request contract: the
// org/service-scoped path without the /api/v2 prefix, HTTP basic auth instead
// of a Bearer key, and no x-hdx-team header (teams are Cloud services).
func TestCloudRequestShape(t *testing.T) {
	t.Parallel()

	c := newCloudTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/organizations/org1/services/svc1/clickstack/sources" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		user, pass, ok := r.BasicAuth()
		if !ok || user != "key1" || pass != "secret1" {
			t.Errorf("expected basic auth key1:secret1, got %q:%q (ok=%v)", user, pass, ok)
		}
		if h := r.Header.Get("x-hdx-team"); h != "" {
			t.Errorf("unexpected x-hdx-team header %q", h)
		}
		if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer") {
			t.Errorf("unexpected bearer Authorization %q", h)
		}
		_, _ = io.WriteString(w, `{"status":200,"requestId":"r1","result":[{"id":"s1","name":"Metrics","kind":"metric"}]}`)
	})

	sources, err := c.ListSources(context.Background())
	if err != nil {
		t.Fatalf("ListSources: %v", err)
	}
	if len(sources) != 1 || sources[0].ID != "s1" {
		t.Errorf("unexpected sources: %+v", sources)
	}
}

// TestCloudTeamRejected guards that team scoping, which the Cloud API cannot
// honor, fails loudly instead of being silently dropped.
func TestCloudTeamRejected(t *testing.T) {
	t.Parallel()

	c := newCloudTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request %s %s: team-scoped calls must not reach the server", r.Method, r.URL.Path)
	})

	if _, err := c.WithTeam("team1").ListSources(context.Background()); !errors.Is(err, ErrCloudUnsupported) {
		t.Errorf("expected ErrCloudUnsupported for team-scoped cloud call, got %v", err)
	}
}

// TestNewCloudValidation mirrors the New() constructor checks for the cloud
// constructor: scheme validation and the org/service-scoped endpoint shape.
func TestNewCloudValidation(t *testing.T) {
	t.Parallel()

	if _, err := NewCloud("ftp://api.example.com", "org1", "svc1", "k", "s", nil); err == nil {
		t.Error("expected error for non-http scheme")
	}
	if _, err := NewCloud("://bad", "org1", "svc1", "k", "s", nil); err == nil {
		t.Error("expected error for malformed url")
	}
	c, err := NewCloud("https://api.example.com/v1/", "org1", "svc1", "k", "s", nil)
	if err != nil {
		t.Fatalf("NewCloud: %v", err)
	}
	want := "https://api.example.com/v1/organizations/org1/services/svc1/clickstack"
	if c.endpoint != want {
		t.Errorf("endpoint = %q, want %q", c.endpoint, want)
	}
}

// TestCloudAlertLifecycle exercises the alerts allowlist branch end to end:
// create, get, update, delete against the org/service-scoped paths.
func TestCloudAlertLifecycle(t *testing.T) {
	t.Parallel()

	base := "/organizations/org1/services/svc1/clickstack/alerts"
	c := newCloudTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == base,
			r.Method == http.MethodGet && r.URL.Path == base+"/al1",
			r.Method == http.MethodPut && r.URL.Path == base+"/al1":
			_, _ = io.WriteString(w, `{"status":200,"result":{"id":"al1","source":"saved_search","savedSearchId":"ss1","interval":"5m","threshold":10,"thresholdType":"above","channel":{"type":"webhook","webhookId":"wh1"},"scheduleStartAt":null}}`)
		case r.Method == http.MethodDelete && r.URL.Path == base+"/al1":
			_, _ = io.WriteString(w, `{"status":200,"requestId":"r1"}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	})

	ctx := context.Background()
	in := Alert{SavedSearchID: "ss1", Interval: "5m", Threshold: 10, ThresholdType: "above", Channel: AlertChannel{Type: "webhook", WebhookID: "wh1"}}
	created, err := c.CreateAlert(ctx, in)
	if err != nil || created.ID != "al1" {
		t.Fatalf("CreateAlert = %+v, %v", created, err)
	}
	if got, err := c.GetAlert(ctx, "al1"); err != nil || got.SavedSearchID != "ss1" {
		t.Fatalf("GetAlert = %+v, %v", got, err)
	}
	if _, err := c.UpdateAlert(ctx, "al1", in); err != nil {
		t.Fatalf("UpdateAlert: %v", err)
	}
	if err := c.DeleteAlert(ctx, "al1"); err != nil {
		t.Fatalf("DeleteAlert: %v", err)
	}
}

// TestCloudDashboardUpdateDelete covers the by-id PUT/DELETE dashboard paths,
// including a result-less DELETE body through the envelope rewrap.
func TestCloudDashboardUpdateDelete(t *testing.T) {
	t.Parallel()

	base := "/organizations/org1/services/svc1/clickstack/dashboards/d1"
	c := newCloudTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && r.URL.Path == base:
			_, _ = io.WriteString(w, `{"status":200,"result":{"id":"d1","name":"renamed"}}`)
		case r.Method == http.MethodDelete && r.URL.Path == base:
			_, _ = io.WriteString(w, `{"status":200,"requestId":"r1"}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	})

	ctx := context.Background()
	body, err := c.UpdateDashboard(ctx, "d1", json.RawMessage(`{"name":"renamed","tiles":[]}`))
	if err != nil {
		t.Fatalf("UpdateDashboard: %v", err)
	}
	if id, _ := DashboardID(body); id != "d1" {
		t.Errorf("expected dashboard id d1, got %q", id)
	}
	if err := c.DeleteDashboard(ctx, "d1"); err != nil {
		t.Fatalf("DeleteDashboard: %v", err)
	}
}

// TestCloudWebhookListQuery guards that pagination query strings survive the
// cloud path rewrite and allowlist check.
func TestCloudWebhookListQuery(t *testing.T) {
	t.Parallel()

	c := newCloudTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/organizations/org1/services/svc1/clickstack/webhooks" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if r.URL.Query().Get("limit") == "" || r.URL.Query().Get("offset") == "" {
			t.Errorf("expected limit/offset query params, got %q", r.URL.RawQuery)
		}
		_, _ = io.WriteString(w, `{"status":200,"result":[{"id":"wh1","name":"n","service":"generic","url":"https://x"}]}`)
	})

	hooks, err := c.ListWebhooks(context.Background())
	if err != nil || len(hooks) != 1 || hooks[0].ID != "wh1" {
		t.Fatalf("ListWebhooks = %+v, %v", hooks, err)
	}
}

// TestRewrapCloudEnvelope pins the envelope translation edge cases.
func TestRewrapCloudEnvelope(t *testing.T) {
	t.Parallel()

	cases := []struct{ name, in, want string }{
		{"object result", `{"status":200,"result":{"id":"x"}}`, `{"data":{"id":"x"}}`},
		{"array result", `{"status":200,"result":[1,2]}`, `{"data":[1,2]}`},
		{"null result rewraps to null data (zero values downstream)", `{"status":200,"result":null}`, `{"data":null}`},
		{"no result key passes through", `{"status":200,"requestId":"r"}`, `{"status":200,"requestId":"r"}`},
		{"non-json passes through", `<html>`, `<html>`},
		{"top-level array passes through", `[1,2]`, `[1,2]`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := string(rewrapCloudEnvelope([]byte(tc.in))); got != tc.want {
				t.Errorf("rewrapCloudEnvelope(%s) = %s, want %s", tc.in, got, tc.want)
			}
		})
	}
}

// TestCloudRouteNotFound404 guards the 404 body sniff: a JSON 404 is a real
// missing resource (ErrNotFound -> state removal), an HTML "Cannot GET" 404 is
// a routing failure and must NOT read as a deleted resource.
func TestCloudRouteNotFound404(t *testing.T) {
	t.Parallel()

	t.Run("json 404 is ErrNotFound", func(t *testing.T) {
		t.Parallel()
		c := newCloudTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"status":404,"error":"NOT_FOUND: Unknown Error","requestId":"r1"}`)
		})
		if _, err := c.GetDashboard(context.Background(), "gone"); !errors.Is(err, ErrNotFound) {
			t.Errorf("expected ErrNotFound for JSON 404, got %v", err)
		}
	})

	t.Run("html 404 is not ErrNotFound", func(t *testing.T) {
		t.Parallel()
		c := newCloudTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, "<!DOCTYPE html><html><body><pre>Cannot GET /x</pre></body></html>")
		})
		_, err := c.GetDashboard(context.Background(), "d1")
		if err == nil || errors.Is(err, ErrNotFound) {
			t.Errorf("expected non-ErrNotFound error for HTML 404, got %v", err)
		}
	})
}

// TestCloudEnvelopeRewrapSingle guards that a single-object {"result":{...}}
// envelope is translated to {"data":{...}} so the shared decoding works.
func TestCloudEnvelopeRewrapSingle(t *testing.T) {
	t.Parallel()

	c := newCloudTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/organizations/org1/services/svc1/clickstack/dashboards/d1" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"status":200,"requestId":"r1","result":{"id":"d1","name":"n"}}`)
	})

	body, err := c.GetDashboard(context.Background(), "d1")
	if err != nil {
		t.Fatalf("GetDashboard: %v", err)
	}
	id, err := DashboardID(body)
	if err != nil || id != "d1" {
		t.Errorf("expected dashboard id d1, got %q (err=%v)", id, err)
	}
}

// TestCloudGetSourceListsAndFilters guards the GET-by-id fallback: the Cloud
// API only exposes the source list, so GetSource must list and filter.
func TestCloudGetSourceListsAndFilters(t *testing.T) {
	t.Parallel()

	c := newCloudTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/organizations/org1/services/svc1/clickstack/sources" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"status":200,"result":[{"id":"s1","kind":"metric"},{"id":"s2","kind":"trace"}]}`)
	})

	src, err := c.GetSource(context.Background(), "s2")
	if err != nil {
		t.Fatalf("GetSource: %v", err)
	}
	if src.ID != "s2" || src.Kind != "trace" {
		t.Errorf("unexpected source %+v", src)
	}

	if _, err := c.GetSource(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound for missing source, got %v", err)
	}
}

// TestCloudUnroutedPathSurfacesServerError guards that operations the Cloud
// API does not (yet) serve pass through to the server — capability gating is
// deliberately server-side — and that its HTML 404 surfaces as a hard error,
// never as ErrNotFound (which reads mean "resource deleted").
func TestCloudUnroutedPathSurfacesServerError(t *testing.T) {
	t.Parallel()

	c := newCloudTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, "<!DOCTYPE html><html><body><pre>Cannot "+r.Method+" "+r.URL.Path+"</pre></body></html>")
	})

	ctx := context.Background()
	if _, err := c.CreateConnection(ctx, CreateConnectionInput{Name: "n"}); err == nil || errors.Is(err, ErrNotFound) {
		t.Errorf("create connection: expected hard error for unrouted path, got %v", err)
	}
	if err := c.DeleteSource(ctx, "s1"); err == nil || errors.Is(err, ErrNotFound) {
		t.Errorf("delete source: expected hard error (not silent already-deleted), got %v", err)
	}
}

// TestCloudErrorBody guards that Cloud error bodies, which use "error" rather
// than the self-hosted "message" key, are surfaced in the returned error.
func TestCloudErrorBody(t *testing.T) {
	t.Parallel()

	c := newCloudTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"status":400,"error":"invalid dashboard","requestId":"r1"}`)
	})

	_, err := c.CreateDashboard(context.Background(), json.RawMessage(`{"name":"d","tiles":[]}`))
	if err == nil || !strings.Contains(err.Error(), "invalid dashboard") {
		t.Errorf("expected error containing API message, got %v", err)
	}
}

// TestCloudValidateEnvelope guards against the Cloud {"result":...} envelope —
// which do() rewraps to {"data":...} — being decoded as a bare ValidateResult.
// That yields the zero value, reporting every valid dashboard as invalid with
// no error details.
func TestCloudValidateEnvelope(t *testing.T) {
	t.Parallel()

	c := newCloudTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/organizations/org1/services/svc1/clickstack/dashboards/validate" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"status":200,"requestId":"r1","result":{"valid":true,"errors":[],"normalized":{"name":"d"}}}`)
	})

	res, err := c.ValidateDashboard(context.Background(), json.RawMessage(`{"name":"d","tiles":[]}`))
	if err != nil {
		t.Fatalf("ValidateDashboard: %v", err)
	}
	if !res.Valid {
		t.Errorf("expected valid=true, got %+v", res)
	}
}

// TestCloudValidateEnvelopeErrors checks the failure direction too: the
// per-error details the API reports must survive the envelope unwrap, so the
// user sees why the dashboard was rejected.
func TestCloudValidateEnvelopeErrors(t *testing.T) {
	t.Parallel()

	c := newCloudTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"status":200,"requestId":"r1","result":{"valid":false,"errors":[{"path":"tiles.0","message":"Could not find source"}]}}`)
	})

	res, err := c.ValidateDashboard(context.Background(), json.RawMessage(`{"name":"d","tiles":[]}`))
	if err != nil {
		t.Fatalf("ValidateDashboard: %v", err)
	}
	if res.Valid {
		t.Fatal("expected valid=false")
	}
	if len(res.Errors) != 1 || res.Errors[0].Message != "Could not find source" || res.Errors[0].Path != "tiles.0" {
		t.Errorf("error details lost through envelope unwrap: %+v", res.Errors)
	}
}

// TestValidateNoVerdictErrors covers the response shapes that carry no
// "valid" field. Each must return an error — ValidateConfig renders an error
// as a "validation unavailable" warning and defers to apply, but a zero
// ValidateResult as a hard "invalid dashboard" plan failure with nothing to
// act on. Failing loudly is the only safe default for a verdict-less body.
func TestValidateNoVerdictErrors(t *testing.T) {
	t.Parallel()

	for name, body := range map[string]string{
		"null result":      `{"status":200,"requestId":"r1","result":null}`,
		"result omitted":   `{"status":200,"requestId":"r1"}`,
		"empty object":     `{}`,
		"envelope of null": `{"data":null}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c := newCloudTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.WriteString(w, body)
			})

			res, err := c.ValidateDashboard(context.Background(), json.RawMessage(`{"name":"d","tiles":[]}`))
			if err == nil {
				t.Fatalf("expected an error for a verdict-less body, got %+v", res)
			}
		})
	}
}

// TestValidateBareResultWithDataField pins that the envelope unwrap keys on a
// missing verdict, not on the presence of a "data" key: a bare result that
// carries its own "data" field must keep its verdict and error details rather
// than being unwrapped into the nested value.
func TestValidateBareResultWithDataField(t *testing.T) {
	t.Parallel()

	c := newSelfHostedValidateClient(t, `{"valid":false,"errors":[{"path":"tiles.0","message":"bad tile"}],"data":{"unrelated":1}}`)

	res, err := c.ValidateDashboard(context.Background(), json.RawMessage(`{"name":"d","tiles":[]}`))
	if err != nil {
		t.Fatalf("ValidateDashboard: %v", err)
	}
	if res.Valid {
		t.Fatal("expected valid=false")
	}
	if len(res.Errors) != 1 || res.Errors[0].Message != "bad tile" {
		t.Errorf("verdict lost to a spurious envelope unwrap: %+v", res.Errors)
	}
}

// newSelfHostedValidateClient returns a self-hosted (non-cloud) client whose
// /validate endpoint replies with body, so tests can exercise the bare
// response shape without do()'s cloud envelope rewrap.
func newSelfHostedValidateClient(t *testing.T, body string) *Client {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)

	c, err := New(srv.URL, "key", srv.Client())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

// TestCloudValidateDegrades guards that a Cloud deployment not serving
// /validate degrades to ErrValidateUnsupported via the 404 path.
func TestCloudValidateDegrades(t *testing.T) {
	t.Parallel()

	c := newCloudTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/organizations/org1/services/svc1/clickstack/dashboards/validate" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusNotFound)
	})

	_, err := c.ValidateDashboard(context.Background(), json.RawMessage(`{"name":"d","tiles":[]}`))
	if !errors.Is(err, ErrValidateUnsupported) {
		t.Errorf("expected ErrValidateUnsupported, got %v", err)
	}
}
