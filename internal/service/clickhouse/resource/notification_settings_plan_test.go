package resource

import (
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
)

func TestExpandNotificationRules(t *testing.T) {
	tests := []struct {
		name         string
		rules        []notificationRule
		channels     []string
		want         map[notificationSetting]string
		wantConflict *notificationSetting
	}{
		{
			name: "a rule crosses its notifications, resources and channels",
			rules: []notificationRule{
				{
					Effect:        api.NotificationEffectEnabled,
					Notifications: []string{"service_backup_failed", "service_metrics_too_many_parts"},
					Resources:     []string{"svc-1", "svc-2"},
				},
			},
			channels: []string{"ui", "email"},
			want: map[notificationSetting]string{
				{Notification: "service_backup_failed", Resource: "svc-1", Channel: "ui"}:             api.NotificationEffectEnabled,
				{Notification: "service_backup_failed", Resource: "svc-1", Channel: "email"}:          api.NotificationEffectEnabled,
				{Notification: "service_backup_failed", Resource: "svc-2", Channel: "ui"}:             api.NotificationEffectEnabled,
				{Notification: "service_backup_failed", Resource: "svc-2", Channel: "email"}:          api.NotificationEffectEnabled,
				{Notification: "service_metrics_too_many_parts", Resource: "svc-1", Channel: "ui"}:    api.NotificationEffectEnabled,
				{Notification: "service_metrics_too_many_parts", Resource: "svc-1", Channel: "email"}: api.NotificationEffectEnabled,
				{Notification: "service_metrics_too_many_parts", Resource: "svc-2", Channel: "ui"}:    api.NotificationEffectEnabled,
				{Notification: "service_metrics_too_many_parts", Resource: "svc-2", Channel: "email"}: api.NotificationEffectEnabled,
			},
		},
		{
			name:     "no channels addresses nothing",
			rules:    []notificationRule{{Effect: api.NotificationEffectEnabled, Notifications: []string{"service_backup_failed"}, Resources: []string{"svc-1"}}},
			channels: nil,
			want:     map[notificationSetting]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, conflict := expandNotificationRules(tt.rules, tt.channels)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("expanded settings mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tt.wantConflict, conflict); diff != "" {
				t.Errorf("conflict mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestNotificationSettingsRequest(t *testing.T) {
	enabledBackups := notificationRule{
		Effect:        api.NotificationEffectEnabled,
		Notifications: []string{"service_backup_failed"},
		Resources:     []string{"svc-1"},
	}
	disabledParts := notificationRule{
		Effect:        api.NotificationEffectDisabled,
		Notifications: []string{"service_metrics_too_many_parts"},
		Resources:     []string{"svc-1"},
	}

	tests := []struct {
		name                           string
		desired, prior                 []notificationRule
		desiredChannels, priorChannels []string
		want                           []api.NotificationSettingRule
	}{
		{
			name:            "a create sends only what is declared",
			desired:         []notificationRule{enabledBackups},
			desiredChannels: []string{"ui", "email"},
			want: []api.NotificationSettingRule{
				{
					Effect:        api.NotificationEffectEnabled,
					Notifications: []string{"service_backup_failed"},
					Resources:     []string{"svc-1"},
					Channels:      []string{"email", "ui"},
				},
			},
		},
		{
			name:            "a rule leaving the configuration is handed back to the default",
			desired:         []notificationRule{enabledBackups},
			prior:           []notificationRule{enabledBackups, disabledParts},
			desiredChannels: []string{"ui"},
			priorChannels:   []string{"ui"},
			want: []api.NotificationSettingRule{
				{
					Effect:        api.NotificationEffectEnabled,
					Notifications: []string{"service_backup_failed"},
					Resources:     []string{"svc-1"},
					Channels:      []string{"ui"},
				},
				{
					Effect:        api.NotificationEffectInherit,
					Notifications: []string{"service_metrics_too_many_parts"},
					Resources:     []string{"svc-1"},
					Channels:      []string{"ui"},
				},
			},
		},
		{
			name:            "narrowing the channels inherits only the channel dropped",
			desired:         []notificationRule{enabledBackups},
			prior:           []notificationRule{enabledBackups},
			desiredChannels: []string{"ui"},
			priorChannels:   []string{"ui", "email"},
			want: []api.NotificationSettingRule{
				{
					Effect:        api.NotificationEffectEnabled,
					Notifications: []string{"service_backup_failed"},
					Resources:     []string{"svc-1"},
					Channels:      []string{"ui"},
				},
				{
					Effect:        api.NotificationEffectInherit,
					Notifications: []string{"service_backup_failed"},
					Resources:     []string{"svc-1"},
					Channels:      []string{"email"},
				},
			},
		},
		{
			name:          "a destroy inherits everything that was declared",
			prior:         []notificationRule{enabledBackups},
			priorChannels: []string{"email", "ui"},
			want: []api.NotificationSettingRule{
				{
					Effect:        api.NotificationEffectInherit,
					Notifications: []string{"service_backup_failed"},
					Resources:     []string{"svc-1"},
					Channels:      []string{"email", "ui"},
				},
			},
		},
		{
			name:            "resources dropped from one rule collapse into a single inherit rule",
			desired:         []notificationRule{{Effect: api.NotificationEffectEnabled, Notifications: []string{"service_backup_failed"}, Resources: []string{"svc-1"}}},
			prior:           []notificationRule{{Effect: api.NotificationEffectEnabled, Notifications: []string{"service_backup_failed"}, Resources: []string{"svc-1", "svc-2", "svc-3"}}},
			desiredChannels: []string{"ui"},
			priorChannels:   []string{"ui"},
			want: []api.NotificationSettingRule{
				{
					Effect:        api.NotificationEffectEnabled,
					Notifications: []string{"service_backup_failed"},
					Resources:     []string{"svc-1"},
					Channels:      []string{"ui"},
				},
				{
					Effect:        api.NotificationEffectInherit,
					Notifications: []string{"service_backup_failed"},
					Resources:     []string{"svc-2", "svc-3"},
					Channels:      []string{"ui"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := notificationSettingsRequest(tt.desired, tt.prior, tt.desiredChannels, tt.priorChannels)
			if err != nil {
				t.Fatalf("notificationSettingsRequest: %v", err)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("request mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// A setting both the prior state and the configuration declare must appear once:
// the API rejects a request naming the same notification, resource and channel
// twice, so an inherit for something still declared would fail the whole apply.
func TestNotificationSettingsRequestNamesNoSettingTwice(t *testing.T) {
	rule := notificationRule{
		Effect:        api.NotificationEffectEnabled,
		Notifications: []string{"service_backup_failed"},
		Resources:     []string{"svc-1", "svc-2"},
	}

	got, err := notificationSettingsRequest(
		[]notificationRule{rule},
		[]notificationRule{{Effect: api.NotificationEffectDisabled, Notifications: rule.Notifications, Resources: []string{"svc-1", "svc-2", "svc-3"}}},
		[]string{"ui", "email"},
		[]string{"ui", "email"},
	)
	if err != nil {
		t.Fatalf("notificationSettingsRequest: %v", err)
	}

	seen := make(map[notificationSetting]int)
	for _, sent := range got {
		for _, notification := range sent.Notifications {
			for _, resource := range sent.Resources {
				for _, channel := range sent.Channels {
					seen[notificationSetting{Notification: notification, Resource: resource, Channel: channel}]++
				}
			}
		}
	}

	for setting, count := range seen {
		if count != 1 {
			t.Errorf("setting %+v named %d times, want 1", setting, count)
		}
	}
	if len(seen) != 6 {
		t.Errorf("request addresses %d settings, want 6 (2 declared + 1 abandoned resource, on 2 channels)", len(seen))
	}
}

func TestNotificationSettingsRequestRejectsAnOversizedRequest(t *testing.T) {
	tests := []struct {
		name     string
		desired  []notificationRule
		channels []string
		wantIn   string
	}{
		{
			name:     "more rules than one request carries",
			desired:  manyRules(api.NotificationSettingsMaxRules + 1),
			channels: []string{"ui"},
			wantIn:   fmt.Sprintf("more than the %d the API accepts", api.NotificationSettingsMaxRules),
		},
		{
			name: "more settings than one request changes",
			desired: []notificationRule{{
				Effect:        api.NotificationEffectEnabled,
				Notifications: []string{"service_backup_failed"},
				Resources:     manyResources(api.NotificationSettingsMaxWrites + 1),
			}},
			channels: []string{"ui"},
			wantIn:   fmt.Sprintf("more than the %d one request may change", api.NotificationSettingsMaxWrites),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := notificationSettingsRequest(tt.desired, nil, tt.channels, nil)
			if err == nil {
				t.Fatalf("notificationSettingsRequest succeeded, want an error naming the cap")
			}
			if !strings.Contains(err.Error(), tt.wantIn) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), tt.wantIn)
			}
		})
	}
}

// The provider builds the inherit rules itself, so a resource well inside the cap
// on its declared rules must not be pushed over it by how those removals are
// built. A rule covers a cross product, so one notification over many resources
// and many notifications over one resource both have to collapse — otherwise a
// resource that applied could not be destroyed.
func TestRemovalsCollapseIntoAsFewRulesAsTheRequestAllows(t *testing.T) {
	tests := []struct {
		name          string
		prior         []notificationRule
		wantResources int
		wantTypes     int
	}{
		{
			name: "one notification over more resources than the rule cap",
			prior: []notificationRule{{
				Effect:        api.NotificationEffectEnabled,
				Notifications: []string{"service_backup_failed"},
				Resources:     manyResources(api.NotificationSettingsMaxRules + 50),
			}},
			wantResources: api.NotificationSettingsMaxRules + 50,
			wantTypes:     1,
		},
		{
			name: "more notifications than the rule cap over one resource",
			prior: []notificationRule{{
				Effect:        api.NotificationEffectEnabled,
				Notifications: manyNotifications(api.NotificationSettingsMaxRules + 50),
				Resources:     []string{"svc-1"},
			}},
			wantResources: 1,
			wantTypes:     api.NotificationSettingsMaxRules + 50,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := notificationSettingsRequest(nil, tt.prior, nil, []string{"ui", "email"})
			if err != nil {
				t.Fatalf("notificationSettingsRequest: %v", err)
			}
			if len(got) != 1 {
				t.Fatalf("got %d rules, want 1 covering the whole removal", len(got))
			}
			if len(got[0].Resources) != tt.wantResources {
				t.Errorf("rule covers %d resources, want %d", len(got[0].Resources), tt.wantResources)
			}
			if len(got[0].Notifications) != tt.wantTypes {
				t.Errorf("rule covers %d notification types, want %d", len(got[0].Notifications), tt.wantTypes)
			}
		})
	}
}

// Two rules naming the same setting cannot both be sent, because the API rejects
// a request that names one twice. The provider says which setting they fight
// over rather than letting the API answer with an opaque 400.
func TestNotificationSettingsRequestRejectsRulesThatDisagree(t *testing.T) {
	desired := []notificationRule{
		{
			Effect:        api.NotificationEffectEnabled,
			Notifications: []string{"service_backup_failed"},
			Resources:     []string{"svc-1", "svc-2"},
		},
		{
			Effect:        api.NotificationEffectDisabled,
			Notifications: []string{"service_backup_failed"},
			Resources:     []string{"svc-2"},
		},
	}

	_, err := notificationSettingsRequest(desired, nil, []string{"ui"}, nil)
	if err == nil {
		t.Fatal("notificationSettingsRequest succeeded, want an error naming the setting two rules disagree about")
	}
	for _, want := range []string{"service_backup_failed", "svc-2", "ui"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to name %q", err.Error(), want)
		}
	}
}

// Two rules that ask for the same effect on an overlapping resource are a
// reasonable thing to write, and must still produce a request naming each
// setting once.
func TestNotificationSettingsRequestMergesRulesThatAgree(t *testing.T) {
	desired := []notificationRule{
		{
			Effect:        api.NotificationEffectEnabled,
			Notifications: []string{"service_backup_failed"},
			Resources:     []string{"svc-1", "svc-2"},
		},
		{
			Effect:        api.NotificationEffectEnabled,
			Notifications: []string{"service_backup_failed"},
			Resources:     []string{"svc-2", "svc-3"},
		},
	}

	got, err := notificationSettingsRequest(desired, nil, []string{"ui"}, nil)
	if err != nil {
		t.Fatalf("notificationSettingsRequest: %v", err)
	}
	want := []api.NotificationSettingRule{{
		Effect:        api.NotificationEffectEnabled,
		Notifications: []string{"service_backup_failed"},
		Resources:     []string{"svc-1", "svc-2", "svc-3"},
		Channels:      []string{"ui"},
	}}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("request mismatch (-want +got):\n%s", diff)
	}
}

// The cap is what the API counts: the settings the request addresses, not the
// combinations the configuration spells out.
func TestNotificationSettingsRequestCountsSettingsOnceWhenRulesOverlap(t *testing.T) {
	desired := []notificationRule{
		{
			Effect:        api.NotificationEffectEnabled,
			Notifications: []string{"service_backup_failed"},
			Resources:     manyResources(api.NotificationSettingsMaxWrites),
		},
		{
			Effect:        api.NotificationEffectEnabled,
			Notifications: []string{"service_backup_failed"},
			Resources:     []string{"svc-0"},
		},
	}

	got, err := notificationSettingsRequest(desired, nil, []string{"ui"}, nil)
	if err != nil {
		t.Fatalf("notificationSettingsRequest: %v", err)
	}
	addressed := 0
	for _, rule := range got {
		addressed += len(rule.Notifications) * len(rule.Resources) * len(rule.Channels)
	}
	if addressed != api.NotificationSettingsMaxWrites {
		t.Errorf("request addresses %d settings, want %d", addressed, api.NotificationSettingsMaxWrites)
	}
}

func TestNotificationRuleHolds(t *testing.T) {
	channels := []string{"ui", "email"}
	rule := notificationRule{
		Effect:        api.NotificationEffectDisabled,
		Notifications: []string{"service_backup_failed"},
		Resources:     []string{"svc-1"},
	}

	tests := []struct {
		name   string
		rule   notificationRule
		stored []api.NotificationSetting
		want   bool
	}{
		{
			name: "holds when every setting it names agrees",
			rule: rule,
			stored: []api.NotificationSetting{
				{Type: "service_backup_failed", ResourceID: "svc-1", ChannelType: "ui", Effect: api.NotificationEffectDisabled},
				{Type: "service_backup_failed", ResourceID: "svc-1", ChannelType: "email", Effect: api.NotificationEffectDisabled},
			},
			want: true,
		},
		{
			name: "does not hold when one channel was flipped",
			rule: rule,
			stored: []api.NotificationSetting{
				{Type: "service_backup_failed", ResourceID: "svc-1", ChannelType: "ui", Effect: api.NotificationEffectDisabled},
				{Type: "service_backup_failed", ResourceID: "svc-1", ChannelType: "email", Effect: api.NotificationEffectEnabled},
			},
			want: false,
		},
		{
			name: "does not hold when a setting was removed",
			rule: rule,
			stored: []api.NotificationSetting{
				{Type: "service_backup_failed", ResourceID: "svc-1", ChannelType: "ui", Effect: api.NotificationEffectDisabled},
			},
			want: false,
		},
		{
			name:   "an inherit rule holds while nothing it names is stored",
			rule:   notificationRule{Effect: api.NotificationEffectInherit, Notifications: rule.Notifications, Resources: rule.Resources},
			stored: nil,
			want:   true,
		},
		{
			name: "an inherit rule does not hold once something it names is stored",
			rule: notificationRule{Effect: api.NotificationEffectInherit, Notifications: rule.Notifications, Resources: rule.Resources},
			stored: []api.NotificationSetting{
				{Type: "service_backup_failed", ResourceID: "svc-1", ChannelType: "ui", Effect: api.NotificationEffectEnabled},
			},
			want: false,
		},
		{
			name: "settings the rule does not name are not consulted",
			rule: rule,
			stored: []api.NotificationSetting{
				{Type: "service_backup_failed", ResourceID: "svc-1", ChannelType: "ui", Effect: api.NotificationEffectDisabled},
				{Type: "service_backup_failed", ResourceID: "svc-1", ChannelType: "email", Effect: api.NotificationEffectDisabled},
				{Type: "service_backup_failed", ResourceID: "svc-9", ChannelType: "ui", Effect: api.NotificationEffectEnabled},
				{Type: "billing_payment_failed", ResourceID: "org-1", ChannelType: "email", Effect: api.NotificationEffectEnabled},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := notificationRuleHolds(tt.rule, channels, storedNotificationSettings(tt.stored))
			if got != tt.want {
				t.Errorf("notificationRuleHolds = %v, want %v", got, tt.want)
			}
		})
	}
}

// manyRules returns rules that cannot be combined: each names its own
// notification type on its own resource, so the request needs one rule per rule
// declared.
func manyRules(n int) []notificationRule {
	rules := make([]notificationRule, n)
	for i := range rules {
		rules[i] = notificationRule{
			Effect:        api.NotificationEffectEnabled,
			Notifications: []string{fmt.Sprintf("notification_%d", i)},
			Resources:     []string{fmt.Sprintf("svc-%d", i)},
		}
	}
	return rules
}

func manyNotifications(n int) []string {
	notifications := make([]string, n)
	for i := range notifications {
		notifications[i] = fmt.Sprintf("notification_%d", i)
	}
	return notifications
}

func manyResources(n int) []string {
	resources := make([]string, n)
	for i := range resources {
		resources[i] = fmt.Sprintf("svc-%d", i)
	}
	return resources
}
