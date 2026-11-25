package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestCreateApp(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v2/apps" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		// isShared and purpose must always be sent, even at their zero values:
		// update is a full replace and requires isShared.
		want := map[string]any{"name": "n", "isShared": false, "code": "c", "purpose": ""}
		for k, v := range want {
			if body[k] != v {
				t.Errorf("body[%q] = %v, want %v", k, body[k], v)
			}
		}
		_, _ = io.WriteString(w, `{"data":{"id":"a1","name":"n","isShared":false,"code":"c","purpose":"","versionId":"v1",
			"manifest":{"purpose":"","requests":[],"queries":[],"runtimeVersion":1,"builtAt":""},
			"createdAt":"2026-01-01T00:00:00.000Z","updatedAt":"2026-01-01T00:00:00.000Z"}}`)
	})

	app, err := c.CreateApp(context.Background(), AppInput{Name: "n", Code: "c"})
	if err != nil {
		t.Fatalf("CreateApp: %v", err)
	}
	if app.ID != "a1" || app.VersionID == nil || *app.VersionID != "v1" || app.Code == nil || *app.Code != "c" {
		t.Errorf("unexpected app: %+v", app)
	}
}

func TestGetApp_NoVersion(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v2/apps/a1" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"data":{"id":"a1","name":"n","isShared":true,"code":null,"purpose":null,"versionId":null,"manifest":null}}`)
	})

	app, err := c.GetApp(context.Background(), "a1")
	if err != nil {
		t.Fatalf("GetApp: %v", err)
	}
	if app.Code != nil || app.Purpose != nil || app.VersionID != nil {
		t.Errorf("expected null code, purpose and versionId, got %+v", app)
	}
}

func TestGetApp_CanChangeIsShared(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		body string
		want *bool
	}{
		{"true", `{"data":{"id":"a1","canChangeIsShared":true}}`, boolPtr(true)},
		{"false", `{"data":{"id":"a1","canChangeIsShared":false}}`, boolPtr(false)},
		{"field missing", `{"data":{"id":"a1"}}`, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, tc.body)
			})
			app, err := c.GetApp(context.Background(), "a1")
			if err != nil {
				t.Fatalf("GetApp: %v", err)
			}
			switch {
			case tc.want == nil && app.CanChangeIsShared != nil:
				t.Errorf("CanChangeIsShared = %v, want nil", *app.CanChangeIsShared)
			case tc.want != nil && (app.CanChangeIsShared == nil || *app.CanChangeIsShared != *tc.want):
				t.Errorf("CanChangeIsShared = %v, want %v", app.CanChangeIsShared, *tc.want)
			}
		})
	}
}

func TestGetApp_NotFound(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"message":"App not found"}`)
	})

	if _, err := c.GetApp(context.Background(), "a1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestUpdateApp(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/api/v2/apps/a1" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var body AppInput
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if body != (AppInput{Name: "n2", IsShared: true, Code: "c2", Purpose: "p"}) {
			t.Errorf("unexpected body: %+v", body)
		}
		_, _ = io.WriteString(w, `{"data":{"id":"a1","name":"n2","isShared":true,"code":"c2","purpose":"p","versionId":"v2"}}`)
	})

	app, err := c.UpdateApp(context.Background(), "a1", AppInput{Name: "n2", IsShared: true, Code: "c2", Purpose: "p"})
	if err != nil {
		t.Fatalf("UpdateApp: %v", err)
	}
	if app.Name != "n2" || *app.VersionID != "v2" {
		t.Errorf("unexpected app: %+v", app)
	}
}

func TestUpdateApp_NotCreator(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"message":"Only the app's creator can change isShared"}`)
	})

	_, err := c.UpdateApp(context.Background(), "a1", AppInput{Name: "n", Code: "c"})
	if err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("expected a non-not-found error, got %v", err)
	}
	if want := "Only the app's creator can change isShared"; !strings.Contains(err.Error(), want) {
		t.Errorf("error %q does not carry the API message %q", err, want)
	}
}

func TestDeleteApp(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/api/v2/apps/a1" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_, _ = io.WriteString(w, `{}`)
	})

	if err := c.DeleteApp(context.Background(), "a1"); err != nil {
		t.Fatalf("DeleteApp: %v", err)
	}
}

func TestValidateApp(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		status    int
		body      string
		wantValid bool
		wantErr   error
		wantAny   bool
	}{
		{name: "valid", body: `{"valid":true,"errors":[],"normalized":{}}`, wantValid: true},
		{name: "invalid", body: `{"valid":false,"errors":[{"path":"code","message":"boom"}],"normalized":null}`},
		{name: "cloud envelope", body: `{"data":{"valid":true,"errors":[]}}`, wantValid: true},
		{name: "endpoint absent", status: http.StatusNotFound, body: `{"message":"x"}`, wantErr: ErrValidateUnsupported},
		{name: "no verdict", body: `{}`, wantAny: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/api/v2/apps/validate" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				if tc.status != 0 {
					w.WriteHeader(tc.status)
				}
				_, _ = io.WriteString(w, tc.body)
			})
			res, err := c.ValidateApp(context.Background(), AppInput{Name: "n", IsShared: true, Code: "c"})
			switch {
			case tc.wantErr != nil:
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("expected %v, got %v", tc.wantErr, err)
				}
			case tc.wantAny:
				if err == nil {
					t.Fatal("expected an error")
				}
			default:
				if err != nil {
					t.Fatalf("ValidateApp: %v", err)
				}
				if res.Valid != tc.wantValid {
					t.Errorf("Valid = %v, want %v", res.Valid, tc.wantValid)
				}
			}
		})
	}
}
