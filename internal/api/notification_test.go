package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

const memberSettingsPath = "/organizations/" + testOrgID + "/members/user-1/notificationSettings"

func TestListMemberNotificationSettingsFollowsTheCursor(t *testing.T) {
	firstPage := []NotificationSetting{
		{Type: "service_backup_failed", ResourceType: "service", ResourceID: "svc-1", ChannelType: "email", Effect: NotificationEffectDisabled},
	}
	secondPage := []NotificationSetting{
		{Type: "service_backup_failed", ResourceType: "service", ResourceID: "svc-2", ChannelType: "ui", Effect: NotificationEffectEnabled},
	}

	var cursors []string
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != memberSettingsPath {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		cursor := r.URL.Query().Get("cursor")
		cursors = append(cursors, cursor)

		page := notificationSettingsPage{Result: secondPage}
		if cursor == "" {
			next := "page-2"
			page = notificationSettingsPage{Result: firstPage, NextCursor: &next}
		}
		if err := json.NewEncoder(w).Encode(page); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	})

	settings, err := client.ListMemberNotificationSettings(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("ListMemberNotificationSettings: %v", err)
	}

	if diff := cmp.Diff(append(append([]NotificationSetting{}, firstPage...), secondPage...), settings); diff != "" {
		t.Errorf("settings mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"", "page-2"}, cursors); diff != "" {
		t.Errorf("requested cursors mismatch (-want +got):\n%s", diff)
	}
}

// A cursor that comes back a second time would page forever, so the client stops
// and says so rather than collecting the same settings until the context ends.
func TestListMemberNotificationSettingsStopsOnARepeatedCursor(t *testing.T) {
	requests := 0
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		next := "page-2"
		page := notificationSettingsPage{
			Result: []NotificationSetting{
				{Type: "service_backup_failed", ResourceType: "service", ResourceID: "svc-1", ChannelType: "email", Effect: NotificationEffectDisabled},
			},
			NextCursor: &next,
		}
		if err := json.NewEncoder(w).Encode(page); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	})

	_, err := client.ListMemberNotificationSettings(context.Background(), "user-1")
	if err == nil {
		t.Fatal("ListMemberNotificationSettings succeeded, want an error naming the repeated cursor")
	}
	if !strings.Contains(err.Error(), "page-2") {
		t.Errorf("error = %q, want it to name the repeated cursor", err.Error())
	}
	if requests != 2 {
		t.Errorf("made %d requests, want 2: the first page and the one that repeats its cursor", requests)
	}
}

func TestUpdateMemberNotificationSettingsSendsRules(t *testing.T) {
	rules := []NotificationSettingRule{
		{
			Effect:        NotificationEffectDisabled,
			Notifications: []string{"service_backup_failed"},
			Resources:     []string{"svc-1"},
			Channels:      []string{"email", "ui"},
		},
	}
	written := []NotificationSetting{
		{Type: "service_backup_failed", ResourceType: "service", ResourceID: "svc-1", ChannelType: "email", Effect: NotificationEffectDisabled},
	}

	var got notificationSettingsUpdateRequest
	var method string
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		if r.URL.Path != memberSettingsPath {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		response := ResponseWithResult[notificationSettingsUpdateResponse]{
			Result: notificationSettingsUpdateResponse{Settings: written},
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	})

	settings, err := client.UpdateMemberNotificationSettings(context.Background(), "user-1", rules)
	if err != nil {
		t.Fatalf("UpdateMemberNotificationSettings: %v", err)
	}

	if method != http.MethodPatch {
		t.Errorf("method = %q, want PATCH", method)
	}
	if diff := cmp.Diff(rules, got.Rules); diff != "" {
		t.Errorf("sent rules mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(written, settings); diff != "" {
		t.Errorf("returned settings mismatch (-want +got):\n%s", diff)
	}
}

// A rule naming no channel must leave the field out rather than send an empty
// array, which the API would read as "no channels" instead of "every channel".
func TestUpdateMemberNotificationSettingsOmitsEmptyChannels(t *testing.T) {
	var body map[string]any
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if err := json.NewEncoder(w).Encode(ResponseWithResult[notificationSettingsUpdateResponse]{}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	})

	_, err := client.UpdateMemberNotificationSettings(context.Background(), "user-1", []NotificationSettingRule{
		{Effect: NotificationEffectInherit, Notifications: []string{"service_backup_failed"}, Resources: []string{"svc-1"}},
	})
	if err != nil {
		t.Fatalf("UpdateMemberNotificationSettings: %v", err)
	}

	rule := body["rules"].([]any)[0].(map[string]any)
	if _, present := rule["channels"]; present {
		t.Errorf("rule carries channels %v, want the field omitted", rule["channels"])
	}
}

func TestListNotificationTypes(t *testing.T) {
	want := []NotificationType{
		{
			Name:           "service_backup_failed",
			Title:          "Backup failed",
			Description:    "A backup of the service did not complete.",
			Category:       "service",
			Severity:       "warning",
			ResourceType:   "service",
			IsConfigurable: true,
			IsAvailable:    true,
		},
	}

	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/organizations/"+testOrgID+"/notificationTypes" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if err := json.NewEncoder(w).Encode(ResponseWithResult[[]NotificationType]{Result: want}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	})

	got, err := client.ListNotificationTypes(context.Background())
	if err != nil {
		t.Fatalf("ListNotificationTypes: %v", err)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("notification types mismatch (-want +got):\n%s", diff)
	}
}
