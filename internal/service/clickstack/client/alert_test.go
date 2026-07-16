package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"
)

func TestCreateAlert(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v2/alerts" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if body["source"] != "saved_search" {
			t.Errorf("expected source saved_search, got %v", body["source"])
		}
		if body["savedSearchId"] != "ss1" {
			t.Errorf("expected savedSearchId ss1, got %v", body["savedSearchId"])
		}
		ch, ok := body["channel"].(map[string]any)
		if !ok || ch["type"] != "webhook" || ch["webhookId"] != "wh1" {
			t.Errorf("unexpected channel: %v", body["channel"])
		}
		if body["thresholdType"] != "above" || body["interval"] != "5m" {
			t.Errorf("unexpected threshold/interval: %v", body)
		}
		// Non-range alert omits thresholdMax.
		if _, ok := body["thresholdMax"]; ok {
			t.Errorf("expected thresholdMax omitted, got %v", body["thresholdMax"])
		}
		// Unset optional pointers are omitted.
		if _, ok := body["groupBy"]; ok {
			t.Errorf("expected groupBy omitted")
		}
		// Transient server fields are never sent.
		for _, k := range []string{"state", "silenced", "executionErrors"} {
			if _, ok := body[k]; ok {
				t.Errorf("did not expect %s in request body", k)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"id":"al1","source":"saved_search","savedSearchId":"ss1","interval":"5m","threshold":100,"thresholdType":"above","channel":{"type":"webhook","webhookId":"wh1"},"state":"OK"}}`)
	})

	al, err := c.CreateAlert(context.Background(), Alert{
		Channel:       AlertChannel{Type: AlertChannelWebhook, WebhookID: "wh1"},
		Interval:      "5m",
		Threshold:     100,
		ThresholdType: "above",
		SavedSearchID: "ss1",
	})
	if err != nil {
		t.Fatalf("CreateAlert: %v", err)
	}
	if al.ID != "al1" {
		t.Errorf("expected id al1, got %q", al.ID)
	}
}

func TestCreateAlert_RangeSendsThresholdMax(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if body["thresholdType"] != "between" {
			t.Errorf("expected between, got %v", body["thresholdType"])
		}
		if body["thresholdMax"] != float64(200) {
			t.Errorf("expected thresholdMax 200, got %v", body["thresholdMax"])
		}
		if body["groupBy"] != "service" {
			t.Errorf("expected groupBy service, got %v", body["groupBy"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"id":"al2"}}`)
	})

	max := 200.0
	groupBy := "service"
	_, err := c.CreateAlert(context.Background(), Alert{
		Channel:       AlertChannel{Type: AlertChannelWebhook, WebhookID: "wh1"},
		Interval:      "1h",
		Threshold:     100,
		ThresholdType: "between",
		ThresholdMax:  &max,
		SavedSearchID: "ss1",
		GroupBy:       &groupBy,
	})
	if err != nil {
		t.Fatalf("CreateAlert range: %v", err)
	}
}

func TestGetAlert_NotFound(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	_, err := c.GetAlert(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestUpdateAlert(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/api/v2/alerts/al1" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		// KTD8: update never sends the server-managed transient fields.
		for _, k := range []string{"state", "silenced", "executionErrors"} {
			if _, ok := body[k]; ok {
				t.Errorf("did not expect %s in update body", k)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"id":"al1","threshold":50}}`)
	})

	al, err := c.UpdateAlert(context.Background(), "al1", Alert{
		Channel:       AlertChannel{Type: AlertChannelWebhook, WebhookID: "wh1"},
		Interval:      "5m",
		Threshold:     50,
		ThresholdType: "above",
		SavedSearchID: "ss1",
	})
	if err != nil {
		t.Fatalf("UpdateAlert: %v", err)
	}
	if al.Threshold != 50 {
		t.Errorf("expected threshold 50, got %v", al.Threshold)
	}
}

func TestDeleteAlert_NotFound(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	// A cascade-deleted alert (its saved search was removed) is a no-op for the
	// caller; the resource treats ErrNotFound as success.
	err := c.DeleteAlert(context.Background(), "al1")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}
