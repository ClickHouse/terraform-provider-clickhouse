package resource

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gojuno/minimock/v3"
	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service"
)

// errNotFound is what the client returns for a 404, which api.IsNotFound matches
// on the message rather than on a sentinel.
var errNotFound = errors.New("status: 404, body: member not found")

// notificationRuleValue builds a rules element the way Terraform stores one.
func notificationRuleValue(t *testing.T, effect string, notifications, resources []string) attr.Value {
	t.Helper()

	obj, diags := types.ObjectValue(notificationSettingsRuleAttrTypes, map[string]attr.Value{
		"effect":        types.StringValue(effect),
		"notifications": strSetValue(notifications...),
		"resources":     strSetValue(resources...),
	})
	if diags.HasError() {
		t.Fatalf("building rule: %v", diags.Errors())
	}
	return obj
}

func notificationRulesValue(t *testing.T, rules ...attr.Value) types.Set {
	t.Helper()

	set, diags := types.SetValue(notificationSettingsRuleObjectType(), rules)
	if diags.HasError() {
		t.Fatalf("building rules: %v", diags.Errors())
	}
	return set
}

// readNotificationSettings drives the resource's Read against a client that
// answers with the given stored settings, and returns the rules left in state.
func readNotificationSettings(t *testing.T, state NotificationSettingsModel, stored []api.NotificationSetting) (types.Set, *resource.ReadResponse) {
	t.Helper()
	ctx := context.Background()

	client := api.NewClientMock(minimock.NewController(t)).
		ListMemberNotificationSettingsMock.
		Expect(ctx, state.UserID.ValueString()).
		Return(stored, nil)

	r := &NotificationSettingsResource{client: client}

	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("building resource schema: %v", schemaResp.Diagnostics.Errors())
	}

	priorState := tfsdk.State{Schema: schemaResp.Schema}
	if diags := priorState.Set(ctx, state); diags.HasError() {
		t.Fatalf("seeding state: %v", diags.Errors())
	}

	resp := &resource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema, Raw: priorState.Raw}}
	r.Read(ctx, resource.ReadRequest{State: priorState}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", resp.Diagnostics.Errors())
	}

	var got NotificationSettingsModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("reading state back: %v", diags.Errors())
	}
	return got.Rules, resp
}

// Drift is reported per rule: a rule the stored settings no longer agree with is
// recorded at the effect they report, so the next plan re-applies that one rule,
// and the rules that still hold keep the value Terraform planned.
func TestNotificationSettingsReadRecordsOnlyTheDriftedRule(t *testing.T) {
	held := notificationRuleValue(t, api.NotificationEffectEnabled, []string{"service_backup_failed"}, []string{"svc-1"})
	drifted := notificationRuleValue(t, api.NotificationEffectDisabled, []string{"service_metrics_too_many_parts"}, []string{"svc-1"})

	state := NotificationSettingsModel{
		ID:       types.StringValue("user-1"),
		UserID:   types.StringValue("user-1"),
		Channels: strSetValue("email"),
		Rules:    notificationRulesValue(t, held, drifted),
	}

	gotRules, _ := readNotificationSettings(t, state, []api.NotificationSetting{
		{Type: "service_backup_failed", ResourceType: "service", ResourceID: "svc-1", ChannelType: "email", Effect: api.NotificationEffectEnabled},
		// Someone turned this back on in the console.
		{Type: "service_metrics_too_many_parts", ResourceType: "service", ResourceID: "svc-1", ChannelType: "email", Effect: api.NotificationEffectEnabled},
	})

	want := notificationRulesValue(t, held,
		notificationRuleValue(t, api.NotificationEffectEnabled, []string{"service_metrics_too_many_parts"}, []string{"svc-1"}))
	if diff := cmp.Diff(want, gotRules); diff != "" {
		t.Errorf("rules left in state mismatch (-want +got):\n%s", diff)
	}
}

// State is also what says which settings this resource wrote, so a drifted rule
// has to survive the read: dropping it would leave its settings stored when the
// configuration later stops declaring it.
func TestNotificationSettingsDriftKeepsTheRuleRemovable(t *testing.T) {
	declared := notificationRuleValue(t, api.NotificationEffectEnabled, []string{"service_backup_failed"}, []string{"svc-1"})

	state := NotificationSettingsModel{
		ID:       types.StringValue("user-1"),
		UserID:   types.StringValue("user-1"),
		Channels: strSetValue("email"),
		Rules:    notificationRulesValue(t, declared),
	}

	// Someone turned the one declared setting off in the console.
	afterRead, _ := readNotificationSettings(t, state, []api.NotificationSetting{
		{Type: "service_backup_failed", ResourceType: "service", ResourceID: "svc-1", ChannelType: "email", Effect: api.NotificationEffectDisabled},
	})

	state.Rules = afterRead
	prior, priorChannels, diags := readNotificationSettingsModel(context.Background(), state)
	if diags.HasError() {
		t.Fatalf("reading the refreshed state back: %v", diags.Errors())
	}

	// The rule is gone from the configuration now: the request has to hand the
	// setting back to the platform default.
	got, err := notificationSettingsRequest(nil, prior, nil, priorChannels)
	if err != nil {
		t.Fatalf("notificationSettingsRequest: %v", err)
	}
	want := []api.NotificationSettingRule{{
		Effect:        api.NotificationEffectInherit,
		Notifications: []string{"service_backup_failed"},
		Resources:     []string{"svc-1"},
		Channels:      []string{"email"},
	}}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("request mismatch (-want +got):\n%s", diff)
	}
}

// Declared-only reconciliation: settings the configuration never named are not
// drift, however they were set, so every declared rule survives the read.
func TestNotificationSettingsReadIgnoresUndeclaredSettings(t *testing.T) {
	declared := notificationRuleValue(t, api.NotificationEffectEnabled, []string{"service_backup_failed"}, []string{"svc-1"})

	state := NotificationSettingsModel{
		ID:       types.StringValue("user-1"),
		UserID:   types.StringValue("user-1"),
		Channels: strSetValue("email"),
		Rules:    notificationRulesValue(t, declared),
	}

	gotRules, _ := readNotificationSettings(t, state, []api.NotificationSetting{
		{Type: "service_backup_failed", ResourceType: "service", ResourceID: "svc-1", ChannelType: "email", Effect: api.NotificationEffectEnabled},
		// Set in the console for a service and a type this resource never declared.
		{Type: "service_metrics_too_many_parts", ResourceType: "service", ResourceID: "svc-9", ChannelType: "ui", Effect: api.NotificationEffectDisabled},
		{Type: "billing_payment_failed", ResourceType: "organization", ResourceID: "org-1", ChannelType: "email", Effect: api.NotificationEffectDisabled},
	})

	if diff := cmp.Diff(state.Rules, gotRules); diff != "" {
		t.Errorf("rules left in state mismatch (-want +got):\n%s", diff)
	}
}

// An imported resource declares nothing yet, so there is nothing to check it
// against and the read must not call the API at all.
func TestNotificationSettingsReadAfterImportKeepsNullRules(t *testing.T) {
	ctx := context.Background()

	// A mock with no expectations fails the test if Read reaches the API.
	client := api.NewClientMock(minimock.NewController(t))
	r := &NotificationSettingsResource{client: client}

	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("building resource schema: %v", schemaResp.Diagnostics.Errors())
	}

	imported := NotificationSettingsModel{
		ID:       types.StringValue("user-1"),
		UserID:   types.StringValue("user-1"),
		Channels: types.SetNull(types.StringType),
		Rules:    types.SetNull(notificationSettingsRuleObjectType()),
	}
	priorState := tfsdk.State{Schema: schemaResp.Schema}
	if diags := priorState.Set(ctx, imported); diags.HasError() {
		t.Fatalf("seeding state: %v", diags.Errors())
	}

	resp := &resource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema, Raw: priorState.Raw}}
	r.Read(ctx, resource.ReadRequest{State: priorState}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", resp.Diagnostics.Errors())
	}

	var got NotificationSettingsModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("reading state back: %v", diags.Errors())
	}
	if !got.Rules.IsNull() {
		t.Errorf("rules = %v, want null so the first plan applies what the configuration declares", got.Rules)
	}
	if got.UserID.ValueString() != "user-1" {
		t.Errorf("user_id = %q, want it preserved", got.UserID.ValueString())
	}
}

// A member removed from the organization answers 404, which means the settings
// this resource managed are gone rather than that the apply failed.
func TestNotificationSettingsReadRemovesResourceWhenTheMemberIsGone(t *testing.T) {
	ctx := context.Background()

	client := api.NewClientMock(minimock.NewController(t)).
		ListMemberNotificationSettingsMock.
		Return(nil, errNotFound)

	r := &NotificationSettingsResource{client: client}

	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("building resource schema: %v", schemaResp.Diagnostics.Errors())
	}

	state := NotificationSettingsModel{
		ID:       types.StringValue("user-1"),
		UserID:   types.StringValue("user-1"),
		Channels: strSetValue("email"),
		Rules: notificationRulesValue(t,
			notificationRuleValue(t, api.NotificationEffectEnabled, []string{"service_backup_failed"}, []string{"svc-1"}),
		),
	}
	priorState := tfsdk.State{Schema: schemaResp.Schema}
	if diags := priorState.Set(ctx, state); diags.HasError() {
		t.Fatalf("seeding state: %v", diags.Errors())
	}

	resp := &resource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema, Raw: priorState.Raw}}
	r.Read(ctx, resource.ReadRequest{State: priorState}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Read reported an error instead of dropping the resource: %v", resp.Diagnostics.Errors())
	}
	if !resp.State.Raw.IsNull() {
		t.Errorf("state = %v, want it removed so Terraform plans the resource again", resp.State.Raw)
	}
}

// A member removed from the organization takes their settings with them, so a
// destroy that finds them gone has nothing left to do and must not fail the
// apply that is tearing the configuration down.
func TestNotificationSettingsDeleteToleratesAMemberWhoHasLeft(t *testing.T) {
	ctx := context.Background()

	client := api.NewClientMock(minimock.NewController(t)).
		UpdateMemberNotificationSettingsMock.
		Return(nil, errNotFound)

	r := &NotificationSettingsResource{client: client}

	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("building resource schema: %v", schemaResp.Diagnostics.Errors())
	}

	state := tfsdk.State{Schema: schemaResp.Schema}
	diags := state.Set(ctx, NotificationSettingsModel{
		ID:       types.StringValue("user-1"),
		UserID:   types.StringValue("user-1"),
		Channels: strSetValue("email"),
		Rules: notificationRulesValue(t,
			notificationRuleValue(t, api.NotificationEffectEnabled, []string{"service_backup_failed"}, []string{"svc-1"}),
		),
	})
	if diags.HasError() {
		t.Fatalf("seeding state: %v", diags.Errors())
	}

	resp := &resource.DeleteResponse{State: state}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)

	if resp.Diagnostics.HasError() {
		t.Errorf("Delete reported an error for a member who has left: %v", resp.Diagnostics.Errors())
	}
}

// notificationSettingsSchema builds the resource schema the state and plan
// helpers below are typed against.
func notificationSettingsSchema(t *testing.T) schema.Schema {
	t.Helper()

	resp := &resource.SchemaResponse{}
	(&NotificationSettingsResource{}).Schema(context.Background(), resource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("building resource schema: %v", resp.Diagnostics.Errors())
	}
	return resp.Schema
}

// A create sends one request carrying the declared rules on the declared
// channels. A rule reaching the API without channels would be applied to every
// channel the member can hold, so the channels a rule goes out with are the
// claim worth pinning.
func TestNotificationSettingsCreateSendsTheDeclaredRules(t *testing.T) {
	ctx := context.Background()

	var sentUserID string
	var sent []api.NotificationSettingRule
	client := api.NewClientMock(minimock.NewController(t)).
		UpdateMemberNotificationSettingsMock.
		Set(func(_ context.Context, userID string, rules []api.NotificationSettingRule) ([]api.NotificationSetting, error) {
			sentUserID = userID
			sent = rules
			return nil, nil
		})

	r := &NotificationSettingsResource{client: client}
	resourceSchema := notificationSettingsSchema(t)

	plan := tfsdk.Plan{Schema: resourceSchema}
	if diags := plan.Set(ctx, NotificationSettingsModel{
		UserID:   types.StringValue("user-1"),
		Channels: strSetValue("ui", "email"),
		Rules: notificationRulesValue(t,
			notificationRuleValue(t, api.NotificationEffectDisabled, []string{"service_backup_failed"}, []string{"svc-1"}),
		),
	}); diags.HasError() {
		t.Fatalf("seeding the plan: %v", diags.Errors())
	}

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: resourceSchema}}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", resp.Diagnostics.Errors())
	}

	if sentUserID != "user-1" {
		t.Errorf("wrote settings for %q, want user-1", sentUserID)
	}
	want := []api.NotificationSettingRule{{
		Effect:        api.NotificationEffectDisabled,
		Notifications: []string{"service_backup_failed"},
		Resources:     []string{"svc-1"},
		Channels:      []string{"email", "ui"},
	}}
	if diff := cmp.Diff(want, sent); diff != "" {
		t.Errorf("rules sent mismatch (-want +got):\n%s", diff)
	}

	var state NotificationSettingsModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("reading state back: %v", diags.Errors())
	}
	if state.ID.ValueString() != "user-1" {
		t.Errorf("id = %q, want the user id", state.ID.ValueString())
	}
}

// A change that needs exactly as many rules as the API accepts is sent, not
// rejected: the cap belongs to the API, and the provider refusing one rule early
// would block a configuration that applies.
func TestNotificationSettingsRequestAcceptsTheLargestRequestTheAPITakes(t *testing.T) {
	rules, err := notificationSettingsRequest(manyRules(api.NotificationSettingsMaxRules), nil, []string{"ui"}, nil)
	if err != nil {
		t.Fatalf("notificationSettingsRequest: %v", err)
	}
	if len(rules) != api.NotificationSettingsMaxRules {
		t.Errorf("request carries %d rules, want %d", len(rules), api.NotificationSettingsMaxRules)
	}
}

// The same boundary on the settings a request may change.
func TestNotificationSettingsRequestAcceptsTheMostSettingsOneRequestChanges(t *testing.T) {
	desired := []notificationRule{{
		Effect:        api.NotificationEffectEnabled,
		Notifications: []string{"service_backup_failed"},
		Resources:     manyResources(api.NotificationSettingsMaxWrites),
	}}

	rules, err := notificationSettingsRequest(desired, nil, []string{"ui"}, nil)
	if err != nil {
		t.Fatalf("notificationSettingsRequest: %v", err)
	}
	addressed := 0
	for _, rule := range rules {
		addressed += len(rule.Notifications) * len(rule.Resources) * len(rule.Channels)
	}
	if addressed != api.NotificationSettingsMaxWrites {
		t.Errorf("request addresses %d settings, want %d", addressed, api.NotificationSettingsMaxWrites)
	}
}

func TestNotificationSettingsResourceMetadata(t *testing.T) {
	resp := resource.MetadataResponse{}
	NewNotificationSettingsResource().Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "clickhouse"}, &resp)
	if got, want := resp.TypeName, "clickhouse_notification_settings"; got != want {
		t.Fatalf("type name = %q; want %q", got, want)
	}
}

func TestNotificationSettingsResourceConfigure(t *testing.T) {
	ctx := context.Background()

	// Terraform configures resources before the provider has produced its data,
	// so an absent one is the first of two calls, not a failure.
	t.Run("absent provider data is accepted", func(t *testing.T) {
		r := NewNotificationSettingsResource().(*NotificationSettingsResource)
		resp := resource.ConfigureResponse{}
		r.Configure(ctx, resource.ConfigureRequest{}, &resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("Configure: %v", resp.Diagnostics.Errors())
		}
		if r.client != nil {
			t.Errorf("client = %v, want it left unset", r.client)
		}
	})

	t.Run("unexpected provider data is rejected", func(t *testing.T) {
		r := NewNotificationSettingsResource().(*NotificationSettingsResource)
		resp := resource.ConfigureResponse{}
		r.Configure(ctx, resource.ConfigureRequest{ProviderData: "not provider data"}, &resp)
		if !resp.Diagnostics.HasError() {
			t.Fatal("Configure accepted provider data of the wrong type")
		}
		if got := resp.Diagnostics.Errors()[0].Summary(); got != "Unexpected provider data" {
			t.Errorf("summary = %q, want Unexpected provider data", got)
		}
	})

	t.Run("missing API credentials are rejected", func(t *testing.T) {
		r := NewNotificationSettingsResource().(*NotificationSettingsResource)
		resp := resource.ConfigureResponse{}
		r.Configure(ctx, resource.ConfigureRequest{ProviderData: &service.ProviderData{}}, &resp)
		if !resp.Diagnostics.HasError() {
			t.Fatal("Configure accepted a provider with no ClickHouse Cloud API")
		}
		if got := resp.Diagnostics.Errors()[0].Summary(); got != "ClickHouse Cloud API not configured" {
			t.Errorf("summary = %q, want ClickHouse Cloud API not configured", got)
		}
	})

	t.Run("a configured API is kept", func(t *testing.T) {
		r := NewNotificationSettingsResource().(*NotificationSettingsResource)
		client := api.NewClientMock(minimock.NewController(t))
		resp := resource.ConfigureResponse{}
		r.Configure(ctx, resource.ConfigureRequest{ProviderData: &service.ProviderData{API: client}}, &resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("Configure: %v", resp.Diagnostics.Errors())
		}
		if r.client != client {
			t.Errorf("client = %v, want the API from the provider data", r.client)
		}
	})
}

// Only a missing member lets a read drop the resource. Any other failure is
// surfaced, because dropping state there would make the next apply rewrite
// settings that already exist.
func TestNotificationSettingsReadReportsAFailedList(t *testing.T) {
	ctx := context.Background()

	client := api.NewClientMock(minimock.NewController(t)).
		ListMemberNotificationSettingsMock.
		Return(nil, errors.New("status: 503, body: service unavailable"))

	r := &NotificationSettingsResource{client: client}
	resourceSchema := notificationSettingsSchema(t)

	priorState := tfsdk.State{Schema: resourceSchema}
	if diags := priorState.Set(ctx, NotificationSettingsModel{
		ID:       types.StringValue("user-1"),
		UserID:   types.StringValue("user-1"),
		Channels: strSetValue("email"),
		Rules: notificationRulesValue(t,
			notificationRuleValue(t, api.NotificationEffectEnabled, []string{"service_backup_failed"}, []string{"svc-1"}),
		),
	}); diags.HasError() {
		t.Fatalf("seeding state: %v", diags.Errors())
	}

	resp := &resource.ReadResponse{State: tfsdk.State{Schema: resourceSchema, Raw: priorState.Raw}}
	r.Read(ctx, resource.ReadRequest{State: priorState}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("Read hid the failure instead of reporting it")
	}
	if got := resp.Diagnostics.Errors()[0].Summary(); got != "Error reading notification settings" {
		t.Errorf("summary = %q, want Error reading notification settings", got)
	}
	if got := resp.Diagnostics.Errors()[0].Detail(); !strings.Contains(got, "service unavailable") {
		t.Errorf("detail = %q, want it to carry what the API said", got)
	}
	if resp.State.Raw.IsNull() {
		t.Error("state was removed, want it kept so the settings are not rewritten")
	}
}

// A rejected write fails the apply: the member being gone is the only error a
// write forgives, and only while tearing the resource down.
func TestNotificationSettingsCreateReportsARejectedWrite(t *testing.T) {
	ctx := context.Background()

	client := api.NewClientMock(minimock.NewController(t)).
		UpdateMemberNotificationSettingsMock.
		Return(nil, errors.New("status: 400, body: notification type is not configurable"))

	r := &NotificationSettingsResource{client: client}
	resourceSchema := notificationSettingsSchema(t)

	plan := tfsdk.Plan{Schema: resourceSchema}
	if diags := plan.Set(ctx, NotificationSettingsModel{
		UserID:   types.StringValue("user-1"),
		Channels: strSetValue("email"),
		Rules: notificationRulesValue(t,
			notificationRuleValue(t, api.NotificationEffectDisabled, []string{"service_backup_failed"}, []string{"svc-1"}),
		),
	}); diags.HasError() {
		t.Fatalf("seeding the plan: %v", diags.Errors())
	}

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: resourceSchema}}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("Create reported success for a write the API rejected")
	}
	if got := resp.Diagnostics.Errors()[0].Summary(); got != "Error updating notification settings" {
		t.Errorf("summary = %q, want Error updating notification settings", got)
	}
	if got := resp.Diagnostics.Errors()[0].Detail(); !strings.Contains(got, "not configurable") {
		t.Errorf("detail = %q, want it to carry what the API said", got)
	}
}
