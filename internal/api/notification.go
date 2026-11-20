package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// Effects a stored notification setting can report. The update endpoint accepts a
// third value, NotificationEffectInherit, which deletes the setting instead of
// storing a state, so a read never reports it.
const (
	NotificationEffectEnabled  = "enabled"
	NotificationEffectDisabled = "disabled"
	NotificationEffectInherit  = "inherit"
)

// NotificationRuleEffects are the effects an update rule may ask for.
var NotificationRuleEffects = []string{
	NotificationEffectEnabled,
	NotificationEffectDisabled,
	NotificationEffectInherit,
}

// Largest request the update endpoint accepts: rules in the body, and settings
// the rules expand to. A rule expands to one setting per notification type,
// resource and channel it names, so a short body can address many.
const (
	NotificationSettingsMaxRules  = 100
	NotificationSettingsMaxWrites = 1000
)

// NotificationSetting is one stored setting: whether a member receives one
// notification type, about one resource, on one channel.
type NotificationSetting struct {
	Type         string `json:"type"`
	ResourceType string `json:"resourceType"`
	ResourceID   string `json:"resourceId"`
	ChannelType  string `json:"channelType"`
	Effect       string `json:"effect"`
}

// NotificationSettingRule applies one effect to every combination of the
// notification types, resources and channels it names. An empty Channels is
// omitted from the request, which the API reads as every channel a member can
// hold; callers that care name the channels explicitly.
type NotificationSettingRule struct {
	Effect        string   `json:"effect"`
	Notifications []string `json:"notifications"`
	Resources     []string `json:"resources"`
	Channels      []string `json:"channels,omitempty"`
}

// NotificationType describes one entry of the notification registry. Name is a
// stable identifier, never renamed or reused; the set of names grows over time.
type NotificationType struct {
	Name           string `json:"name"`
	Title          string `json:"title"`
	Description    string `json:"description"`
	Category       string `json:"category"`
	Severity       string `json:"severity"`
	ResourceType   string `json:"resourceType"`
	IsConfigurable bool   `json:"isConfigurable"`
	IsAvailable    bool   `json:"isAvailable"`
}

type notificationSettingsUpdateRequest struct {
	Rules []NotificationSettingRule `json:"rules"`
}

type notificationSettingsUpdateResponse struct {
	Settings []NotificationSetting `json:"settings"`
}

func (c *ClientImpl) memberNotificationSettingsPath(userID string) string {
	return c.getOrgPath(fmt.Sprintf("/members/%s/notificationSettings", userID))
}

// ListMemberNotificationSettings returns every setting a member has explicitly
// chosen, following the cursor until the server reports no further page. An
// empty result means the member follows the platform default everywhere, not
// that their notifications are off.
func (c *ClientImpl) ListMemberNotificationSettings(ctx context.Context, userID string) ([]NotificationSetting, error) {
	return listAll[NotificationSetting](ctx, c, c.memberNotificationSettingsPath(userID), nil)
}

// UpdateMemberNotificationSettings applies rules to a member's settings and
// returns the settings they address afterwards. The request either applies
// completely or changes nothing, and a rule asking for
// NotificationEffectInherit removes its settings, so those are absent from the
// response.
func (c *ClientImpl) UpdateMemberNotificationSettings(ctx context.Context, userID string, rules []NotificationSettingRule) ([]NotificationSetting, error) {
	rb, err := json.Marshal(notificationSettingsUpdateRequest{Rules: rules})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPatch, c.memberNotificationSettingsPath(userID), bytes.NewReader(rb))
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	response := ResponseWithResult[notificationSettingsUpdateResponse]{}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("unmarshal notification settings update: %w", err)
	}

	return response.Result.Settings, nil
}

// ListNotificationTypes returns the notification registry for the organization.
// Types the organization is not entitled to are omitted; a type the caller may
// not use is returned with IsAvailable false rather than dropped.
func (c *ClientImpl) ListNotificationTypes(ctx context.Context) ([]NotificationType, error) {
	req, err := http.NewRequest(http.MethodGet, c.getOrgPath("/notificationTypes"), nil)
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	response := ResponseWithResult[[]NotificationType]{}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("unmarshal notification types: %w", err)
	}

	return response.Result, nil
}
