package clickstack

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickstack/client"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/utils"
)

const (
	appResourceName = "clickhouse_clickstack_app"

	isSharedAttr  = "is_shared"
	codeAttr      = "code"
	purposeAttr   = "purpose"
	versionIDAttr = "version_id"

	// Limits enforced by the API, quoted in the attribute descriptions. They are
	// not checked locally: the API is the source of truth, and its validate
	// endpoint reports a bad value at plan time for apps being created or
	// changed.
	appNameMaxLength    = 60
	appCodeMaxLength    = 500000
	appPurposeMaxLength = 2000

	// appValidateName stands in for a name that is not yet known at plan time,
	// so the code can still be compiled by /validate.
	appValidateName = "terraform-validate"

	privateAppWarningSummary = "Private app is tied to this API key" //nolint:gosec // G101 false positive: a warning title, not a credential

	// privateCanChangeIsShared is the private state key holding the API's
	// canChangeIsShared from the last read, as JSON true/false. Absent when no
	// read has recorded it yet, or the response lacked the field.
	privateCanChangeIsShared = "can_change_is_shared"
)

// privateGetter and privateSetter are the parts of the framework's private
// state used here. Its concrete type lives in an internal framework package,
// so naming these lets unit tests substitute a fake.
type privateGetter interface {
	GetKey(ctx context.Context, key string) ([]byte, diag.Diagnostics)
}

type privateSetter interface {
	SetKey(ctx context.Context, key string, value []byte) diag.Diagnostics
}

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                   = (*appResource)(nil)
	_ resource.ResourceWithConfigure      = (*appResource)(nil)
	_ resource.ResourceWithImportState    = (*appResource)(nil)
	_ resource.ResourceWithValidateConfig = (*appResource)(nil)
	_ resource.ResourceWithModifyPlan     = (*appResource)(nil)
)

// NewAppResource is a helper to register the resource with the provider.
func NewAppResource() resource.Resource {
	return &appResource{}
}

// appResource manages a ClickStack app: a single-file React (TSX) application that
// ClickStack renders in a sandbox on top of the team's data.
type appResource struct {
	client *client.Client
}

type appResourceModel struct {
	ID        types.String `tfsdk:"id"`
	Team      types.String `tfsdk:"team"`
	Name      types.String `tfsdk:"name"`
	IsShared  types.Bool   `tfsdk:"is_shared"`
	Code      types.String `tfsdk:"code"`
	Purpose   types.String `tfsdk:"purpose"`
	VersionID types.String `tfsdk:"version_id"`
}

func (r *appResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_clickstack_app"
}

func (r *appResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a ClickStack app: a single-file React (TSX) application that ClickStack renders in a " +
			"sandbox on top of the team's data. Every change to `code` or `purpose` saves a new version, " +
			"kept in the app's version history; edits made in the app builder show up as drift and are " +
			"overwritten on the next apply. Requires Apps to be enabled for the team.\n\n" +
			"~> **Note:** This resource is in beta and its behavior may change in future provider versions. " +
			"It is built on the ClickStack Apps API, which is in preview.",
		Attributes: map[string]schema.Attribute{
			idAttr: schema.StringAttribute{
				Computed:      true,
				Description:   "Identifier of the app.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			teamAttr: schema.StringAttribute{
				Optional: true,
				Description: "Team ID to manage this app under (`x-hdx-team`). Self-hosted only. " +
					"Changing this forces the app to be replaced.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			nameAttr: schema.StringAttribute{
				Required:    true,
				Description: fmt.Sprintf("App name, 1 to %d characters, without leading or trailing whitespace.", appNameMaxLength),
			},
			isSharedAttr: schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(true),
				Description: "Whether the whole team can see and edit the app. Defaults to `true`. When `false`, only " +
					"the app's creator — the user the API key belongs to — can see it. Only the creator can change " +
					"this, so a plan that flips it with a different API key fails. On ClickHouse Cloud an API key " +
					"is not a user anyone signs in as, so a private app is visible to no one in the UI.",
			},
			codeAttr: schema.StringAttribute{
				Required: true,
				Description: fmt.Sprintf("The app's TSX source, up to %d characters. It must default-export a React "+
					"component and import only modules the app runtime provides (such as `@hdx/app`). Typically "+
					"read with `file()`. To pass in values such as source IDs, put a placeholder string in the "+
					"code and swap it with `replace()`, as in the example; this keeps the file valid TSX, "+
					"unlike `templatefile()`, which clashes with JavaScript's `${...}` syntax.", appCodeMaxLength),
			},
			purposeAttr: schema.StringAttribute{
				Optional: true,
				Computed: true,
				Default:  stringdefault.StaticString(""),
				Description: fmt.Sprintf("What the app is for, who it helps, and how to use it, up to %d characters. "+
					"Shown on the app's About page and used as context when the app is edited in the app builder. "+
					"Defaults to empty.", appPurposeMaxLength),
			},
			versionIDAttr: schema.StringAttribute{
				Computed: true,
				Description: "ID of the app's current version. Changes whenever `code` or `purpose` changes, " +
					"so it can be used to trigger dependent resources.",
			},
		},
	}
}

func (r *appResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	providerData, ok := req.ProviderData.(*service.ProviderData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("expected *service.ProviderData, got: %T. This is a bug in the provider.", req.ProviderData),
		)
		return
	}
	if providerData.ClickStack == nil {
		addNotConfiguredError(&resp.Diagnostics, "resource")
		return
	}
	r.client = providerData.ClickStack
}

// ModifyPlan fails a sharing change the caller isn't allowed to make, and has
// the API validate an app that is being created or changed.
//
// version_id is left unknown on every update, even when code and purpose match
// state. The API saves a new version when the sent code or purpose differs from
// the server's current one, and state can be stale (a saved plan, or
// -refresh=false, after an edit in the app builder); promising the state's
// version_id would then fail the apply with an inconsistent result.
func (r *appResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return // destroy
	}
	var plan, state appResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	hasState := !req.State.Raw.IsNull()
	if hasState {
		resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	}
	if resp.Diagnostics.HasError() {
		return
	}
	// resp.RequiresReplace is already filled in by the attribute plan
	// modifiers (a team change) when ModifyPlan runs.
	replaced := len(resp.RequiresReplace) > 0
	if hasState {
		canChange, diags := storedCanChangeIsShared(ctx, req.Private)
		resp.Diagnostics.Append(diags...)
		resp.Diagnostics.Append(sharingChangeDiagnostics(plan, state, canChange, replaced)...)
	}
	// r.client is nil when the provider isn't configured yet, as during
	// terraform validate.
	if r.client != nil && needsAPIValidation(plan, state, hasState, replaced) {
		resp.Diagnostics.Append(r.validateWithAPI(ctx, plan)...)
	}
}

// needsAPIValidation reports whether a plan sends the API anything it hasn't
// already accepted: a create, a replacement, or a change to a field the
// validate endpoint checks. Validating compiles the code on the server, so an
// unchanged app is not sent; Terraform plans every resource on every run.
func needsAPIValidation(plan, state appResourceModel, hasState, replaced bool) bool {
	return !hasState || replaced ||
		!plan.Code.Equal(state.Code) || !plan.Purpose.Equal(state.Purpose) ||
		!plan.Name.Equal(state.Name) || !plan.IsShared.Equal(state.IsShared)
}

// validateWithAPI has the API validate the planned body and compile its code,
// so a compile error fails the plan instead of the apply. It is skipped when
// the code, purpose or sharing isn't known yet.
func (r *appResource) validateWithAPI(ctx context.Context, plan appResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics
	input, ok := plan.validateInput()
	if !ok {
		return diags
	}
	res, err := r.client.WithTeam(plan.Team.ValueString()).ValidateApp(ctx, input)
	if err != nil {
		if errors.Is(err, client.ErrValidateUnsupported) {
			diags.AddAttributeWarning(path.Root(codeAttr), "App validation skipped",
				"The ClickStack API does not expose /api/v2/apps/validate; the app will be validated on apply.")
			return diags
		}
		// A warning so a transient outage, a key without read permission or
		// Apps being off does not block plan; apply reports the real error.
		tflog.Warn(ctx, "app validation endpoint returned an error; deferring validation to apply: "+err.Error())
		diags.AddAttributeWarning(path.Root(codeAttr), "App validation unavailable",
			"Could not validate the app: "+err.Error())
		return diags
	}
	return appValidateDiagnostics(res, known(plan.Name))
}

// sharingChangeDiagnostics errors when the plan changes is_shared but the last
// read said the caller can't, so the plan fails instead of the apply's PUT
// answering 403. A nil canChange (no read has recorded it yet, or the response
// lacked the field) skips the check and leaves it to the server. A replacement skips it too:
// canChange describes the app being deleted, and the new app's creator is the
// caller, who may set is_shared freely.
func sharingChangeDiagnostics(plan, state appResourceModel, canChange *bool, replaced bool) diag.Diagnostics {
	var diags diag.Diagnostics
	if replaced || canChange == nil || *canChange || !known(plan.IsShared) || !known(state.IsShared) ||
		plan.IsShared.Equal(state.IsShared) {
		return diags
	}
	diags.AddAttributeError(path.Root(isSharedAttr), "Cannot change is_shared",
		fmt.Sprintf("Only the app's creator can change is_shared, and this API key's user did not create app %s. "+
			"Run this change with the creator's API key, or set is_shared back to %t.",
			state.ID.ValueString(), state.IsShared.ValueBool()))
	return diags
}

// storedCanChangeIsShared reads the canChangeIsShared recorded by the last
// read, or nil when none was recorded.
func storedCanChangeIsShared(ctx context.Context, priv privateGetter) (*bool, diag.Diagnostics) {
	raw, diags := priv.GetKey(ctx, privateCanChangeIsShared)
	if diags.HasError() || len(raw) == 0 {
		return nil, diags
	}
	var v bool
	if err := json.Unmarshal(raw, &v); err != nil {
		// Unreadable data only disables the plan-time check; the server still
		// enforces the rule on apply.
		tflog.Warn(ctx, "ignoring unreadable "+privateCanChangeIsShared+" private state: "+err.Error())
		return nil, diags
	}
	return &v, diags
}

// recordCanChangeIsShared stores the API's canChangeIsShared for the next
// plan, or clears it when the server didn't send one.
func recordCanChangeIsShared(ctx context.Context, priv privateSetter, app *client.App) diag.Diagnostics {
	if app.CanChangeIsShared == nil {
		return priv.SetKey(ctx, privateCanChangeIsShared, nil)
	}
	return priv.SetKey(ctx, privateCanChangeIsShared, []byte(strconv.FormatBool(*app.CanChangeIsShared)))
}

// ValidateConfig runs the local checks only. Terraform calls it on every plan
// and apply for every resource, whether or not it changed, and without state,
// so the API's validate endpoint (which compiles the code) is called from
// ModifyPlan instead, for apps that are created or changed.
func (r *appResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg appResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(cfg.validate()...)
}

// validate holds the rules the schema validators can't express, as a pure
// function of the model so it can be unit-tested directly.
func (m *appResourceModel) validate() diag.Diagnostics {
	var diags diag.Diagnostics
	// The API rejects a padded name too; checking here as well reports it
	// without a network call, including when /validate can't be reached.
	if known(m.Name) && strings.TrimSpace(m.Name.ValueString()) != m.Name.ValueString() {
		diags.AddAttributeError(path.Root(nameAttr), "Invalid name",
			"name must not have leading or trailing whitespace")
	}
	// The API ties a private app to its creator, the API key's user, and the
	// provider can't tell at plan time which key created an app. Warn on every
	// plan rather than let a key change silently orphan the app.
	if known(m.IsShared) && !m.IsShared.ValueBool() {
		diags.AddAttributeWarning(path.Root(isSharedAttr), privateAppWarningSummary,
			"Only the user this API key belongs to can see this app or change is_shared. If Terraform later "+
				"runs with a different API key (for example after a key rotation), the app reads as missing: "+
				"Terraform forgets it and creates a duplicate, and no one else can see or delete the original. "+
				"On ClickHouse Cloud each API key is its own user that no one can sign in as, so no one can see "+
				"the app in the UI. Set is_shared = true unless the app must stay private to this key.")
	}
	return diags
}

// validateInput is the /validate body for a config, and false when the code,
// purpose or sharing isn't known yet. An unknown name is replaced by a
// placeholder so the code can still be compiled.
func (m *appResourceModel) validateInput() (client.AppInput, bool) {
	if !known(m.Code) || m.Purpose.IsUnknown() || m.IsShared.IsUnknown() {
		return client.AppInput{}, false
	}
	input := m.toClient()
	if !known(m.Name) {
		input.Name = appValidateName
	}
	if m.IsShared.IsNull() {
		input.IsShared = true
	}
	return input, true
}

// appValidateDiagnostics turns a /validate verdict into attribute errors.
// Errors about the name are dropped when the name was a placeholder.
func appValidateDiagnostics(res *client.ValidateResult, nameKnown bool) diag.Diagnostics {
	var diags diag.Diagnostics
	if res.Valid {
		return diags
	}
	reported := false
	for _, e := range res.Errors {
		attr, ok := appAttrForAPIPath(e.Path)
		if attr == nameAttr && !nameKnown {
			continue
		}
		reported = true
		if !ok {
			detail := e.Message
			if e.Path != "" {
				detail = e.Path + ": " + e.Message
			}
			diags.AddError("Invalid app configuration", detail)
			continue
		}
		diags.AddAttributeError(path.Root(attr), "Invalid app configuration", e.Message)
	}
	if !reported && len(res.Errors) == 0 {
		diags.AddError("Invalid app configuration", "the API reported the app as invalid but returned no error details")
	}
	return diags
}

// appAttrForAPIPath maps a /validate error path to the attribute it concerns.
func appAttrForAPIPath(p string) (string, bool) {
	switch p {
	case "name":
		return nameAttr, true
	case "isShared":
		return isSharedAttr, true
	case "code":
		return codeAttr, true
	case "purpose":
		return purposeAttr, true
	}
	return "", false
}

func (r *appResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	utils.BetaWarning(appResourceName, &resp.Diagnostics)
	var plan appResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	app, err := r.client.WithTeam(plan.Team.ValueString()).CreateApp(ctx, plan.toClient())
	if err != nil {
		resp.Diagnostics.AddError("Error Creating App", err.Error())
		return
	}

	// The framework always sets Private; it is nil only in unit tests that
	// call the method directly.
	if resp.Private != nil {
		resp.Diagnostics.Append(recordCanChangeIsShared(ctx, resp.Private, app)...)
	}
	plan.applyApp(app)
	tflog.Trace(ctx, "created app resource")
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *appResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state appResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	app, err := r.client.WithTeam(state.Team.ValueString()).GetApp(ctx, state.ID.ValueString())
	if err != nil {
		if errors.Is(err, client.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error Reading App", err.Error())
		return
	}

	// The framework always sets Private; it is nil only in unit tests that
	// call the method directly.
	if resp.Private != nil {
		resp.Diagnostics.Append(recordCanChangeIsShared(ctx, resp.Private, app)...)
	}
	state.applyApp(app)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *appResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	utils.BetaWarning(appResourceName, &resp.Diagnostics)
	var plan appResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	app, err := r.client.WithTeam(plan.Team.ValueString()).UpdateApp(ctx, plan.ID.ValueString(), plan.toClient())
	if err != nil {
		// Removing state from Update is illegal ("Missing Resource State After
		// Update"); prior state survives and the next refresh converges via Read.
		if errors.Is(err, client.ErrNotFound) {
			resp.Diagnostics.AddError("App No Longer Exists",
				fmt.Sprintf("app %s no longer exists on the server, or the API key can no longer see it: it was "+
					"deleted or made private outside this apply. The next plan recreates it.", plan.ID.ValueString()))
			return
		}
		resp.Diagnostics.AddError("Error Updating App", err.Error())
		return
	}

	// The framework always sets Private; it is nil only in unit tests that
	// call the method directly.
	if resp.Private != nil {
		resp.Diagnostics.Append(recordCanChangeIsShared(ctx, resp.Private, app)...)
	}
	plan.applyApp(app)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *appResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state appResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.WithTeam(state.Team.ValueString()).DeleteApp(ctx, state.ID.ValueString()); err != nil {
		if errors.Is(err, client.ErrNotFound) {
			return
		}
		resp.Diagnostics.AddError("Error Deleting App", err.Error())
	}
}

func (r *appResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	utils.BetaWarning(appResourceName, &resp.Diagnostics)
	if team, id, ok := strings.Cut(req.ID, "/"); ok {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root(teamAttr), team)...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root(idAttr), id)...)
		return
	}
	resource.ImportStatePassthroughID(ctx, path.Root(idAttr), req, resp)
}

// --- conversion helpers ---

// toClient builds the full-replace body. Plan defaults make every field known
// on create and update.
func (m *appResourceModel) toClient() client.AppInput {
	return client.AppInput{
		Name:     m.Name.ValueString(),
		IsShared: m.IsShared.ValueBool(),
		Code:     m.Code.ValueString(),
		Purpose:  m.Purpose.ValueString(),
	}
}

// applyApp records the server's app. An app with no version yet (created in
// the app builder and never saved) has null code, which plans a write of the
// configured code.
func (m *appResourceModel) applyApp(app *client.App) {
	m.ID = types.StringValue(app.ID)
	m.Name = types.StringValue(app.Name)
	m.IsShared = types.BoolValue(app.IsShared)
	m.Code = types.StringPointerValue(app.Code)
	m.Purpose = types.StringValue("")
	if app.Purpose != nil {
		m.Purpose = types.StringValue(*app.Purpose)
	}
	m.VersionID = types.StringPointerValue(app.VersionID)
}
