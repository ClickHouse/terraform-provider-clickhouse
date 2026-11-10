package resource

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
)

// notificationRule is one declared rule, free of Terraform types so the decision
// below can be tested as plain data.
type notificationRule struct {
	Effect        string
	Notifications []string
	Resources     []string
}

// notificationSetting addresses exactly one stored setting. It is the unit the
// API keys rows by, and the unit two rules may not both name.
type notificationSetting struct {
	Notification string
	Resource     string
	Channel      string
}

// notificationSettingsError is a request the provider refuses to send, carrying
// the summary the resource layer reports it under.
type notificationSettingsError struct {
	summary string
	detail  string
}

func (e *notificationSettingsError) Error() string {
	return e.detail
}

// expandNotificationRules returns the settings a rule set addresses on the given
// channels, paired with the effect asked for, and one setting two rules ask
// opposite things of. A single rule carries a single effect, so it cannot
// disagree with itself and expanding one never reports a conflict.
func expandNotificationRules(rules []notificationRule, channels []string) (map[notificationSetting]string, *notificationSetting) {
	effects := make(map[notificationSetting]string)
	var conflict *notificationSetting
	for _, rule := range rules {
		for _, notification := range rule.Notifications {
			for _, resource := range rule.Resources {
				for _, channel := range channels {
					key := notificationSetting{Notification: notification, Resource: resource, Channel: channel}
					taken, seen := effects[key]
					if !seen {
						effects[key] = rule.Effect
						continue
					}
					if taken != rule.Effect && conflict == nil {
						conflict = &key
					}
				}
			}
		}
	}
	return effects, conflict
}

// notificationSettingsRequest builds the single request that moves a member from
// what prior declared to what desired declares. Settings prior addressed and
// desired no longer does are handed back to the platform default, so a rule
// leaving the configuration stops applying rather than lingering at its last
// value.
func notificationSettingsRequest(desired, prior []notificationRule, desiredChannels, priorChannels []string) ([]api.NotificationSettingRule, error) {
	declared, conflict := expandNotificationRules(desired, desiredChannels)
	if conflict != nil {
		return nil, &notificationSettingsError{
			summary: "Conflicting notification rules",
			detail: fmt.Sprintf(
				"Two rules ask for different effects on %q for resource %q on channel %q. A setting may be named by only one rule, because the request that would apply both is rejected whole. Narrow the resources or notification types one of the rules names.",
				conflict.Notification, conflict.Resource, conflict.Channel,
			),
		}
	}

	// Each setting is named once, whichever side of the change it came from: the
	// API rejects a request naming one twice, and a setting still declared must
	// keep the effect the configuration asks for rather than be handed back.
	addressed := maps.Clone(declared)
	abandoned, _ := expandNotificationRules(prior, priorChannels)
	for setting := range abandoned {
		if _, kept := declared[setting]; !kept {
			addressed[setting] = api.NotificationEffectInherit
		}
	}

	rules := groupNotificationRules(addressed)
	if err := checkNotificationSettingsLimits(rules, len(addressed)); err != nil {
		return nil, err
	}
	return rules, nil
}

// groupNotificationRules covers the given settings with as few rules as the
// request allows: one per effect, channel set and resource set, naming every
// notification type that shares them. A rule applies to every combination it
// names, so grouping by the coverage a type has keeps each rule an exact cover
// of the settings it was built from, and spends the request's rule budget on
// distinct shapes rather than on one type repeated across services.
func groupNotificationRules(effects map[notificationSetting]string) []api.NotificationSettingRule {
	// effect -> notification -> resource -> channels
	byEffect := make(map[string]map[string]map[string]map[string]struct{})
	for setting, effect := range effects {
		notifications, seen := byEffect[effect]
		if !seen {
			notifications = make(map[string]map[string]map[string]struct{})
			byEffect[effect] = notifications
		}
		resources, seen := notifications[setting.Notification]
		if !seen {
			resources = make(map[string]map[string]struct{})
			notifications[setting.Notification] = resources
		}
		channels, seen := resources[setting.Resource]
		if !seen {
			channels = make(map[string]struct{})
			resources[setting.Resource] = channels
		}
		channels[setting.Channel] = struct{}{}
	}

	type group struct {
		effect        string
		channels      []string
		resources     []string
		notifications []string
	}
	groups := make(map[string]*group)
	for effect, byNotification := range byEffect {
		for notification, byResource := range byNotification {
			// Resources reached on the same channels form one cross product; a
			// resource reached on fewer belongs to a rule of its own, so a partial
			// removal stays correct.
			resourcesByChannels := make(map[string][]string)
			channelsByKey := make(map[string][]string)
			for resource, channelSet := range byResource {
				channels := slices.Sorted(maps.Keys(channelSet))
				key := strings.Join(channels, "\x00")
				resourcesByChannels[key] = append(resourcesByChannels[key], resource)
				channelsByKey[key] = channels
			}
			for channelKey, resources := range resourcesByChannels {
				slices.Sort(resources)
				key := strings.Join([]string{effect, channelKey, strings.Join(resources, "\x00")}, "\x01")
				existing, seen := groups[key]
				if !seen {
					groups[key] = &group{
						effect:        effect,
						channels:      channelsByKey[channelKey],
						resources:     resources,
						notifications: []string{notification},
					}
					continue
				}
				existing.notifications = append(existing.notifications, notification)
			}
		}
	}

	rules := make([]api.NotificationSettingRule, 0, len(groups))
	for _, key := range slices.Sorted(maps.Keys(groups)) {
		g := groups[key]
		slices.Sort(g.notifications)
		rules = append(rules, api.NotificationSettingRule{
			Effect:        g.effect,
			Notifications: g.notifications,
			Resources:     g.resources,
			Channels:      g.channels,
		})
	}
	return rules
}

// checkNotificationSettingsLimits reports a request the API would reject for its
// size. Splitting it instead would cost the all-or-nothing guarantee a settings
// write makes, so the configuration is what has to be split.
func checkNotificationSettingsLimits(rules []api.NotificationSettingRule, settings int) error {
	if len(rules) > api.NotificationSettingsMaxRules {
		return &notificationSettingsError{
			summary: "Too many notification rules in one request",
			detail: fmt.Sprintf(
				"This change needs %d rules in one request, more than the %d the API accepts. Split the rules across several clickhouse_notification_settings resources.",
				len(rules), api.NotificationSettingsMaxRules,
			),
		}
	}
	if settings > api.NotificationSettingsMaxWrites {
		return &notificationSettingsError{
			summary: "Too many notification settings in one request",
			detail: fmt.Sprintf(
				"These rules address %d settings, more than the %d one request may change. Name fewer notification types, resources or channels, or split them across several clickhouse_notification_settings resources.",
				settings, api.NotificationSettingsMaxWrites,
			),
		}
	}
	return nil
}

// notificationRuleHolds reports whether the stored settings agree with what a
// rule declares on every setting it addresses. An 'inherit' rule holds while no
// setting it names is stored, since that is what inheriting the default is.
// Settings the rule does not name are not its business and are not consulted.
func notificationRuleHolds(rule notificationRule, channels []string, stored map[notificationSetting]string) bool {
	settings, _ := expandNotificationRules([]notificationRule{rule}, channels)
	for setting := range settings {
		effect, found := stored[setting]
		if rule.Effect == api.NotificationEffectInherit {
			if found {
				return false
			}
			continue
		}
		if !found || effect != rule.Effect {
			return false
		}
	}
	return true
}

// driftedNotificationEffect returns an effect the stored settings report for a
// rule that no longer holds, so state can record the rule as changed rather than
// drop it. A rule whose own settings disagree cannot be written as one rule, so
// the first differing effect stands for the disagreement; either way the effect
// differs from the one declared, which is what makes the next plan re-apply the
// rule.
func driftedNotificationEffect(rule notificationRule, channels []string, stored map[notificationSetting]string) string {
	settings, _ := expandNotificationRules([]notificationRule{rule}, channels)
	observed := make([]string, 0, len(settings))
	for setting := range settings {
		effect, found := stored[setting]
		if !found {
			effect = api.NotificationEffectInherit
		}
		if effect != rule.Effect {
			observed = append(observed, effect)
		}
	}
	if len(observed) == 0 {
		return rule.Effect
	}
	slices.Sort(observed)
	return observed[0]
}

// storedNotificationSettings keys a member's settings by what addresses them, so
// a declared rule can be checked without scanning the list per setting.
func storedNotificationSettings(settings []api.NotificationSetting) map[notificationSetting]string {
	stored := make(map[notificationSetting]string, len(settings))
	for _, setting := range settings {
		stored[notificationSetting{
			Notification: setting.Type,
			Resource:     setting.ResourceID,
			Channel:      setting.ChannelType,
		}] = setting.Effect
	}
	return stored
}
