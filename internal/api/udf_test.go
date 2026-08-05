package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestUDFClientUsesExpectedAPIContract(t *testing.T) {
	ctx := context.Background()
	archive := []byte("zip-bytes")

	uploadServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("upload method = %s; want PUT", r.Method)
		}
		if got := r.Header.Get("Content-Type"); got != "application/zip" {
			t.Errorf("upload Content-Type = %q; want application/zip", got)
		}
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("provider credentials leaked to upload URL: %q", got)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read upload body: %v", err)
		}
		if string(body) != string(archive) {
			t.Errorf("upload body = %q; want %q", body, archive)
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(uploadServer.Close)

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok || username != "key" || password != "secret" {
			t.Errorf("API request missing Basic auth")
		}

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/organizations/org-1/udfUploads/url":
			if r.Body != nil {
				body, _ := io.ReadAll(r.Body)
				if len(body) != 0 {
					t.Errorf("upload-session request body = %q; want empty", body)
				}
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, resultEnvelopeJSON(`{"uploadId":"upload-1","uploadUrl":"`+uploadServer.URL+`","expiresAt":"2026-07-21T12:00:00.000Z"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/organizations/org-1/udfs":
			assertUDFWriteBody(t, r, true)
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, resultEnvelopeJSON(udfResponseJSON(1, UDFStatusBuilding)))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/organizations/org-1/udfs/geocode/versions":
			assertUDFWriteBody(t, r, false)
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, resultEnvelopeJSON(udfResponseJSON(2, UDFStatusBuilding)))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/organizations/org-1/udfs/geocode":
			_, _ = io.WriteString(w, resultEnvelopeJSON(udfResponseJSON(2, UDFStatusReady)))
		case r.Method == http.MethodPut && r.URL.Path == "/v1/organizations/org-1/udfs/geocode/attachments/11111111-1111-1111-1111-111111111111":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode attach body: %v", err)
			}
			if body["version"] != float64(2) {
				t.Errorf("attachment version = %#v; want 2", body["version"])
			}
			_, _ = io.WriteString(w, resultEnvelopeJSON(attachmentResponseJSON()))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/organizations/org-1/udfs/geocode/attachments/11111111-1111-1111-1111-111111111111":
			_, _ = io.WriteString(w, resultEnvelopeJSON(attachmentResponseJSON()))
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/organizations/org-1/udfs/geocode/attachments/11111111-1111-1111-1111-111111111111":
			// The public API returns 200 with an empty body.
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/organizations/org-1/udfs/geocode":
			// The public API returns 200 with an empty body.
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(apiServer.Close)

	client, err := NewClient(ClientConfig{
		ApiURL:         apiServer.URL + "/v1",
		OrganizationID: "org-1",
		TokenKey:       "key",
		TokenSecret:    "secret",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	session, err := client.CreateUDFUploadSession(ctx)
	if err != nil {
		t.Fatalf("CreateUDFUploadSession: %v", err)
	}
	if session.UploadID != "upload-1" || session.UploadURL != uploadServer.URL {
		t.Fatalf("session = %+v; unexpected", session)
	}
	if err := client.UploadUDFArchive(ctx, session.UploadURL, archive); err != nil {
		t.Fatalf("UploadUDFArchive: %v", err)
	}

	request := testUDFVersionRequest(session.UploadID)
	created, err := client.CreateUDF(ctx, UDFCreateRequest{
		FunctionName:            "geocode",
		UDFVersionCreateRequest: request,
	})
	if err != nil || created.Version != 1 {
		t.Fatalf("CreateUDF = %+v, %v; want version 1", created, err)
	}
	version, err := client.CreateUDFVersion(ctx, "geocode", request)
	if err != nil || version.Version != 2 {
		t.Fatalf("CreateUDFVersion = %+v, %v; want version 2", version, err)
	}
	got, err := client.GetUDF(ctx, "geocode")
	if err != nil || got.Status != UDFStatusReady || got.PoolSize == nil || *got.PoolSize != 3 {
		t.Fatalf("GetUDF = %+v, %v; unexpected", got, err)
	}

	attachVersion := int64(2)
	attachment, err := client.AttachUDF(ctx, "geocode", "11111111-1111-1111-1111-111111111111", UDFAttachRequest{Version: &attachVersion})
	if err != nil || attachment.ServiceID == "" {
		t.Fatalf("AttachUDF = %+v, %v; unexpected", attachment, err)
	}
	if _, err := client.GetUDFAttachment(ctx, "geocode", attachment.ServiceID); err != nil {
		t.Fatalf("GetUDFAttachment: %v", err)
	}
	if err := client.DetachUDF(ctx, "geocode", attachment.ServiceID); err != nil {
		t.Fatalf("DetachUDF: %v", err)
	}
	if err := client.DeleteUDF(ctx, "geocode"); err != nil {
		t.Fatalf("DeleteUDF: %v", err)
	}
}

func TestUploadUDFArchiveDoesNotExposePresignedURLInErrors(t *testing.T) {
	client, err := NewClient(ClientConfig{
		ApiURL:         "https://api.example.test/v1",
		OrganizationID: "org-1",
		TokenKey:       "key",
		TokenSecret:    "secret",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	sensitiveURL := "://object.example/archive?X-Amz-Signature=secret-value"
	err = client.UploadUDFArchive(context.Background(), sensitiveURL, []byte("archive"))
	if err == nil {
		t.Fatal("UploadUDFArchive returned nil error for malformed URL")
	}
	if strings.Contains(err.Error(), "secret-value") || strings.Contains(err.Error(), sensitiveURL) {
		t.Fatalf("presigned URL leaked through error: %v", err)
	}
}

func TestUploadUDFArchiveErrorDoesNotExposeObjectStorageDetails(t *testing.T) {
	const archive = "archive-secret-bytes"
	var signedURL string
	uploadServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Fatalf("upload method = %s; want PUT", r.Method)
		}
		body := `<?xml version="1.0"?><Error><Message>` + signedURL + ` ` + archive + `</Message></Error>`
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(uploadServer.Close)
	signedURL = uploadServer.URL + "/archive.zip?X-Amz-Signature=secret-value"

	client, err := NewClient(ClientConfig{
		ApiURL:         uploadServer.URL,
		OrganizationID: "org-1",
		TokenKey:       "key",
		TokenSecret:    "secret",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	err = client.UploadUDFArchive(context.Background(), signedURL, []byte(archive))
	if err == nil {
		t.Fatal("UploadUDFArchive returned nil error for a 403 response")
	}
	for _, leaked := range []string{signedURL, "X-Amz-Signature", "secret-value", archive, "<Error>"} {
		if strings.Contains(err.Error(), leaked) {
			t.Fatalf("object-storage detail %q leaked through error: %v", leaked, err)
		}
	}
	if !strings.Contains(err.Error(), "403") {
		t.Fatalf("error = %v; want concise status context", err)
	}
	if !IsForbidden(err) {
		t.Fatalf("IsForbidden(%v) = false; want object-storage 403 classification", err)
	}
}

func TestUDFWritesDoNotReplayAmbiguousFailures(t *testing.T) {
	tests := []struct {
		name string
		path string
		call func(context.Context, *ClientImpl) error
	}{
		{
			name: "server 5xx",
			path: "/organizations/org-1/udfs",
			call: func(ctx context.Context, client *ClientImpl) error {
				_, err := client.CreateUDF(ctx, UDFCreateRequest{
					FunctionName:            "geocode",
					UDFVersionCreateRequest: testUDFVersionRequest("upload-1"),
				})
				return err
			},
		},
		{
			name: "transport failure",
			path: "/organizations/org-1/udfs/geocode/versions",
			call: func(ctx context.Context, client *ClientImpl) error {
				_, err := client.CreateUDFVersion(ctx, "geocode", testUDFVersionRequest("upload-1"))
				return err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.URL.Path != test.path {
					t.Errorf("path = %q; want %q", r.URL.Path, test.path)
				}
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = io.WriteString(w, `{"error":"ambiguous failure"}`)
			})

			if test.name == "transport failure" {
				client.HttpClient.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
					calls++
					return nil, errors.New("connection reset by peer")
				})
			}

			err := test.call(context.Background(), client)
			if err == nil || !IsUDFPublishOutcomeUnknown(err) {
				t.Fatalf("error = %v; want unknown publish outcome", err)
			}
			if test.name == "server 5xx" && !strings.Contains(err.Error(), "status: 500") {
				t.Fatalf("error = %v; want status context", err)
			}
			if calls != 1 {
				t.Fatalf("calls = %d; want exactly one non-idempotent publish request", calls)
			}
		})
	}
}

func TestUDFWriteResponseReadFailureIsUnknownAndNotReplayed(t *testing.T) {
	client, _ := newTestClient(t, func(http.ResponseWriter, *http.Request) {})
	calls := 0
	client.HttpClient.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{
			StatusCode: http.StatusCreated,
			Body:       failingReadCloser{},
			Header:     make(http.Header),
		}, nil
	})

	_, err := client.CreateUDF(context.Background(), UDFCreateRequest{
		FunctionName:            "geocode",
		UDFVersionCreateRequest: testUDFVersionRequest("upload-1"),
	})
	if err == nil || !IsUDFPublishOutcomeUnknown(err) {
		t.Fatalf("error = %v; want unknown publish outcome", err)
	}
	if calls != 1 {
		t.Fatalf("calls = %d; want exactly one request", calls)
	}
}

func TestUDFWriteUnusableSuccessResponseIsUnknownAndNotReplayed(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "malformed JSON", body: `{"result":`},
		{name: "incomplete result", body: `{"result":{"version":1}}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			client, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
				calls++
				w.WriteHeader(http.StatusCreated)
				_, _ = io.WriteString(w, test.body)
			})

			_, err := client.CreateUDF(context.Background(), UDFCreateRequest{
				FunctionName:            "geocode",
				UDFVersionCreateRequest: testUDFVersionRequest("upload-1"),
			})
			if err == nil || !IsUDFPublishOutcomeUnknown(err) {
				t.Fatalf("error = %v; want unknown publish outcome", err)
			}
			if calls != 1 {
				t.Fatalf("calls = %d; want exactly one request", calls)
			}
		})
	}
}

func TestUDFClientRejectsIncompleteSuccessfulResponses(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"result":{}}`)
	})

	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "upload session",
			call: func() error {
				_, err := client.CreateUDFUploadSession(context.Background())
				return err
			},
		},
		{
			name: "UDF read",
			call: func() error {
				_, err := client.GetUDF(context.Background(), "geocode")
				return err
			},
		},
		{
			name: "attachment write",
			call: func() error {
				_, err := client.AttachUDF(context.Background(), "geocode", "service-1", UDFAttachRequest{})
				return err
			},
		},
		{
			name: "attachment read",
			call: func() error {
				_, err := client.GetUDFAttachment(context.Background(), "geocode", "service-1")
				return err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.call()
			if err == nil || !strings.Contains(err.Error(), "invalid or incomplete") {
				t.Fatalf("error = %v; want incomplete-response error", err)
			}
		})
	}
}

type failingReadCloser struct{}

func (failingReadCloser) Read([]byte) (int, error) { return 0, errors.New("response body read failed") }

func (failingReadCloser) Close() error { return nil }

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestAttachUDFWakesIdleServiceAndRetries(t *testing.T) {
	const (
		serviceID      = "11111111-1111-1111-1111-111111111111"
		attachmentPath = "/organizations/org-1/udfs/geocode/attachments/" + serviceID
		servicePath    = "/organizations/org-1/services/" + serviceID
	)

	var attachCalls, wakeCalls, stateGetCalls int
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodPut && r.URL.Path == attachmentPath:
			attachCalls++
			var request UDFAttachRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Errorf("decode attachment body (call %d): %v", attachCalls, err)
			}
			if request.Version == nil || *request.Version != 2 {
				t.Errorf("attachment version (call %d) = %v; want 2", attachCalls, request.Version)
			}
			if wakeCalls == 0 {
				w.WriteHeader(http.StatusFailedDependency)
				_, _ = io.WriteString(w, `{"requestId":"x","error":"The service must be running before the UDF can be attached. Current state: idle","code":"SERVICE_IDLE","serviceState":"idle","canWake":true,"status":424}`)
				return
			}
			_, _ = io.WriteString(w, resultEnvelopeJSON(attachmentResponseJSON()))

		case r.Method == http.MethodPatch && r.URL.Path == servicePath+"/state":
			wakeCalls++
			var stateUpdate ServiceStateUpdate
			if err := json.NewDecoder(r.Body).Decode(&stateUpdate); err != nil {
				t.Errorf("decode state update: %v", err)
			}
			if stateUpdate.Command != serviceStateCommandAwake {
				t.Errorf("state command = %q; want awake", stateUpdate.Command)
			}
			_, _ = io.WriteString(w, `{}`)

		case r.Method == http.MethodGet && r.URL.Path == servicePath:
			stateGetCalls++
			_, _ = io.WriteString(w, resultEnvelopeJSON(`{"id":"`+serviceID+`","state":"running"}`))

		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})

	version := int64(2)
	attachment, err := client.AttachUDF(
		context.Background(),
		"geocode",
		serviceID,
		UDFAttachRequest{Version: &version},
	)
	if err != nil {
		t.Fatalf("AttachUDF: %v", err)
	}
	if attachment.Status != UDFAttachmentStatusDeployed {
		t.Errorf("attachment status = %q; want %q", attachment.Status, UDFAttachmentStatusDeployed)
	}
	if attachCalls != 2 {
		t.Errorf("attach calls = %d; want 2", attachCalls)
	}
	if wakeCalls != 1 {
		t.Errorf("wake calls = %d; want 1", wakeCalls)
	}
	if stateGetCalls < 1 {
		t.Errorf("service state polls = %d; want at least 1", stateGetCalls)
	}
}

func TestAttachUDFWaitsForAlreadyAwakingServiceAndRetries(t *testing.T) {
	const (
		serviceID      = "11111111-1111-1111-1111-111111111111"
		attachmentPath = "/organizations/org-1/udfs/geocode/attachments/" + serviceID
		servicePath    = "/organizations/org-1/services/" + serviceID
	)

	var attachCalls, stateGetCalls int
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodPut && r.URL.Path == attachmentPath:
			attachCalls++
			if attachCalls == 1 {
				w.WriteHeader(http.StatusFailedDependency)
				_, _ = io.WriteString(w, `{"error":"The service must be running before the UDF can be attached. Current state: awaking","code":"SERVICE_NOT_RUNNING","serviceState":"awaking","canWake":false,"status":424}`)
				return
			}
			_, _ = io.WriteString(w, resultEnvelopeJSON(attachmentResponseJSON()))

		case r.Method == http.MethodGet && r.URL.Path == servicePath:
			stateGetCalls++
			_, _ = io.WriteString(w, resultEnvelopeJSON(`{"id":"`+serviceID+`","state":"running"}`))

		case r.Method == http.MethodPatch && r.URL.Path == servicePath+"/state":
			t.Error("already-awaking service must not receive another awake command")
			w.WriteHeader(http.StatusInternalServerError)

		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})

	version := int64(2)
	attachment, err := client.AttachUDF(context.Background(), "geocode", serviceID, UDFAttachRequest{Version: &version})
	if err != nil {
		t.Fatalf("AttachUDF: %v", err)
	}
	if attachment.Status != UDFAttachmentStatusDeployed {
		t.Errorf("attachment status = %q; want %q", attachment.Status, UDFAttachmentStatusDeployed)
	}
	if attachCalls != 2 {
		t.Errorf("attach calls = %d; want 2", attachCalls)
	}
	if stateGetCalls < 1 {
		t.Errorf("service state polls = %d; want at least 1", stateGetCalls)
	}
}

func TestAttachUDFContinuesWhenAnotherWakeWon(t *testing.T) {
	const (
		serviceID      = "11111111-1111-1111-1111-111111111111"
		attachmentPath = "/organizations/org-1/udfs/geocode/attachments/" + serviceID
		servicePath    = "/organizations/org-1/services/" + serviceID
	)

	var attachCalls, wakeCalls, stateGetCalls int
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodPut && r.URL.Path == attachmentPath:
			attachCalls++
			if wakeCalls == 0 {
				w.WriteHeader(http.StatusFailedDependency)
				_, _ = io.WriteString(w, `{"requestId":"x","error":"The service must be running before the UDF can be attached. Current state: idle","code":"SERVICE_IDLE","serviceState":"idle","canWake":true,"status":424}`)
				return
			}
			_, _ = io.WriteString(w, resultEnvelopeJSON(attachmentResponseJSON()))

		case r.Method == http.MethodPatch && r.URL.Path == servicePath+"/state":
			wakeCalls++
			// Another attach for the same service already woke it up, so this
			// service's own wake attempt loses the race.
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `Service must be in idle state to awake`)

		case r.Method == http.MethodGet && r.URL.Path == servicePath:
			stateGetCalls++
			state := "running"
			if stateGetCalls == 1 {
				state = "awaking"
			}
			_, _ = io.WriteString(w, resultEnvelopeJSON(`{"id":"`+serviceID+`","state":"`+state+`"}`))

		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})

	version := int64(2)
	attachment, err := client.AttachUDF(context.Background(), "geocode", serviceID, UDFAttachRequest{Version: &version})
	if err != nil {
		t.Fatalf("AttachUDF: %v", err)
	}
	if attachment.Status != UDFAttachmentStatusDeployed {
		t.Errorf("attachment status = %q; want %q", attachment.Status, UDFAttachmentStatusDeployed)
	}
	if attachCalls != 2 {
		t.Errorf("attach calls = %d; want 2", attachCalls)
	}
	if wakeCalls != 1 {
		t.Errorf("wake calls = %d; want 1", wakeCalls)
	}
	if stateGetCalls < 2 {
		t.Errorf("service state polls = %d; want at least 2", stateGetCalls)
	}
}

func TestAttachUDFDoesNotWakeStoppedService(t *testing.T) {
	const (
		serviceID      = "11111111-1111-1111-1111-111111111111"
		attachmentPath = "/organizations/org-1/udfs/geocode/attachments/" + serviceID
		servicePath    = "/organizations/org-1/services/" + serviceID
	)

	var attachCalls int
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodPut && r.URL.Path == attachmentPath:
			attachCalls++
			w.WriteHeader(http.StatusFailedDependency)
			_, _ = io.WriteString(w, `{"error":"The service must be running before the UDF can be attached. Current state: stopped","code":"SERVICE_STOPPED","serviceState":"stopped","canWake":false,"status":424}`)

		case r.URL.Path == servicePath || r.URL.Path == servicePath+"/state":
			t.Errorf("stopped service must not be queried or woken: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)

		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})

	version := int64(2)
	_, err := client.AttachUDF(context.Background(), "geocode", serviceID, UDFAttachRequest{Version: &version})
	if err == nil || !strings.Contains(err.Error(), "status: 424") {
		t.Fatalf("AttachUDF error = %v; want original 424 response", err)
	}
	if attachCalls != 1 {
		t.Errorf("attach calls = %d; want 1", attachCalls)
	}
}

func TestUDFServiceDependencyErrorClassification(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		idle    bool
		awaking bool
	}{
		{
			name: "wakeable idle",
			err:  errors.New(`status: 424, body: {"code":"SERVICE_IDLE","serviceState":"idle","canWake":true}`),
			idle: true,
		},
		{
			name: "idle that cannot be woken",
			err:  errors.New(`status: 424, body: {"code":"SERVICE_IDLE","serviceState":"idle","canWake":false}`),
		},
		{
			name:    "already awaking",
			err:     errors.New(`status: 424, body: {"code":"SERVICE_NOT_RUNNING","serviceState":"awaking","canWake":false}`),
			awaking: true,
		},
		{
			name: "stopped",
			err:  errors.New(`status: 424, body: {"code":"SERVICE_STOPPED","serviceState":"stopped","canWake":false}`),
		},
		{
			name: "ClickPipes message-only idle must not match UDF helpers",
			err:  errors.New(`status: 424, body: {"error":"Current state: idle","status":424}`),
		},
		{name: "nil", err: nil},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isUDFServiceIdle(test.err); got != test.idle {
				t.Errorf("isUDFServiceIdle = %v; want %v", got, test.idle)
			}
			if got := isUDFServiceAwaking(test.err); got != test.awaking {
				t.Errorf("isUDFServiceAwaking = %v; want %v", got, test.awaking)
			}
		})
	}
}

func assertUDFWriteBody(t *testing.T, r *http.Request, create bool) {
	t.Helper()
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		t.Errorf("decode UDF write body: %v", err)
		return
	}
	if create {
		if body["functionName"] != "geocode" {
			t.Errorf("functionName = %#v; want geocode", body["functionName"])
		}
	} else if _, exists := body["functionName"]; exists {
		t.Errorf("version request must not contain functionName: %#v", body)
	}
	if _, exists := body["version"]; exists {
		t.Errorf("server-assigned version must not be sent: %#v", body)
	}
	if body["uploadId"] != "upload-1" {
		t.Errorf("uploadId = %#v; want upload-1", body["uploadId"])
	}
	if body["poolSize"] != float64(3) || body["commandReadTimeout"] != float64(10000) {
		t.Errorf("writable config missing from request: %#v", body)
	}
}

func testUDFVersionRequest(uploadID string) UDFVersionCreateRequest {
	poolSize := int64(3)
	maxExecutionTime := int64(10)
	return UDFVersionCreateRequest{
		UploadID:                uploadID,
		Runtime:                 UDFRuntimePython311,
		Arguments:               []UDFArgument{{Name: "lat", Type: "Float64"}},
		ReturnType:              "String",
		Type:                    UDFTypeExecutablePool,
		PoolSize:                &poolSize,
		CommandReadTimeout:      10000,
		CommandWriteTimeout:     10000,
		MaxCommandExecutionTime: &maxExecutionTime,
		Format:                  "TabSeparated",
		SandboxType:             UDFSandboxTypeBasic,
		SandboxVersion:          UDFSandboxVersionV2,
	}
}

func udfResponseJSON(version int64, status string) string {
	return `{"functionName":"geocode","version":` + jsonNumber(version) + `,"status":"` + status + `","runtime":"python3.11","type":"executable_pool","arguments":[{"name":"lat","type":"Float64"}],"returnType":"String","returnName":null,"poolSize":3,"commandReadTimeout":10000,"commandWriteTimeout":10000,"maxCommandExecutionTime":10,"sendChunkHeader":false,"format":"TabSeparated","sandboxType":"basic","sandboxVersion":"v2","error":null,"createdAt":"2026-07-21T10:00:00.000Z","updatedAt":"2026-07-21T10:00:00.000Z"}`
}

func attachmentResponseJSON() string {
	return `{"functionName":"geocode","serviceId":"11111111-1111-1111-1111-111111111111","status":"deployed","version":2}`
}

func resultEnvelopeJSON(result string) string {
	return `{"status":200,"requestId":"request-1","result":` + result + `}`
}

func jsonNumber(value int64) string {
	return strconv.FormatInt(value, 10)
}
