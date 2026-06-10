package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/capy-base/terraform-provider-capydb/internal/capydb"
)

var (
	_ resource.Resource              = (*webhookEndpointResource)(nil)
	_ resource.ResourceWithConfigure = (*webhookEndpointResource)(nil)
)

// NewWebhookEndpointResource creates the capydb_webhook_endpoint resource.
func NewWebhookEndpointResource() resource.Resource {
	return &webhookEndpointResource{}
}

type webhookEndpointResource struct {
	client *capydb.Client
}

type webhookEndpointModel struct {
	ID             types.String `tfsdk:"id"`
	OrganizationID types.String `tfsdk:"organization_id"`
	URL            types.String `tfsdk:"url"`
	Description    types.String `tfsdk:"description"`
	EventTypes     types.List   `tfsdk:"event_types"`
	Active         types.Bool   `tfsdk:"active"`
	SecretVersion  types.Int64  `tfsdk:"secret_version"`
	SigningSecret  types.String `tfsdk:"signing_secret"`
}

func (m *webhookEndpointModel) fill(ctx context.Context, endpoint capydb.WebhookEndpoint) diag.Diagnostics {
	m.ID = types.StringValue(endpoint.ID)
	m.OrganizationID = types.StringValue(endpoint.OrganizationID)
	m.URL = types.StringValue(endpoint.URL)
	m.Description = types.StringValue(endpoint.Description)
	m.Active = types.BoolValue(endpoint.IsActive)
	eventTypes, diags := types.ListValueFrom(ctx, types.StringType, endpoint.EventTypes)
	if diags.HasError() {
		return diags
	}
	m.EventTypes = eventTypes
	return nil
}

// signingSecretPlanModifier keeps the prior signing secret in the plan unless
// secret_version changes, in which case the secret becomes unknown so the
// rotation result can be stored.
type signingSecretPlanModifier struct{}

func (signingSecretPlanModifier) Description(context.Context) string {
	return "Keeps the signing secret from state unless secret_version changes."
}

func (signingSecretPlanModifier) MarkdownDescription(context.Context) string {
	return "Keeps the signing secret from state unless `secret_version` changes."
}

func (signingSecretPlanModifier) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if req.StateValue.IsNull() {
		return
	}

	var planVersion, stateVersion types.Int64
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, req.Path.ParentPath().AtName("secret_version"), &planVersion)...)
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, req.Path.ParentPath().AtName("secret_version"), &stateVersion)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if planVersion.Equal(stateVersion) {
		resp.PlanValue = req.StateValue
		return
	}
	// secret_version changed: the apply will rotate the secret, so the planned
	// value must be unknown (Terraform proposes the prior state for computed
	// attributes, which would make the rotated value an inconsistent result).
	resp.PlanValue = types.StringUnknown()
}

func (r *webhookEndpointResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_webhook_endpoint"
}

func (r *webhookEndpointResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "An outbound CapyDB webhook endpoint for the organization. The signing secret is returned " +
			"by the API exactly once at creation (and again on rotation) and is stored in state as the sensitive " +
			"`signing_secret` attribute; drift on the secret itself cannot be detected. Bump `secret_version` to " +
			"rotate the secret — the previous secret stops validating immediately.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Webhook endpoint id.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"organization_id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Organization the endpoint belongs to. Defaults to the authenticated principal's organization.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"url": schema.StringAttribute{
				Required:    true,
				Description: "HTTPS receiver URL deliveries are POSTed to.",
			},
			"description": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Free-form endpoint description.",
			},
			"event_types": schema.ListAttribute{
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
				Description: "Event types to deliver. An empty (or omitted) list subscribes to all events.",
			},
			"active": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
				Description: "Whether deliveries are enabled. Defaults to `true`.",
			},
			"secret_version": schema.Int64Attribute{
				Optional: true,
				Computed: true,
				Default:  int64default.StaticInt64(1),
				Description: "Rotation trigger for the signing secret. Bump this value (e.g. 1 -> 2) to rotate " +
					"the secret in place; the value itself is never sent to the API.",
			},
			"signing_secret": schema.StringAttribute{
				Computed:  true,
				Sensitive: true,
				Description: "HMAC-SHA256 signing secret used for the `X-CapyDB-Signature` header. Returned " +
					"exactly once at creation/rotation and stored in state; out-of-band rotation cannot be detected.",
				PlanModifiers: []planmodifier.String{
					signingSecretPlanModifier{},
				},
			},
		},
	}
}

func (r *webhookEndpointResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = resourceClient(req, resp)
}

func (r *webhookEndpointResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan webhookEndpointModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	orgID, err := resolveOrganizationID(ctx, r.client, plan.OrganizationID)
	if err != nil {
		resp.Diagnostics.AddError("Error resolving CapyDB organization", err.Error())
		return
	}

	var eventTypes []string
	if !plan.EventTypes.IsNull() && !plan.EventTypes.IsUnknown() {
		resp.Diagnostics.Append(plan.EventTypes.ElementsAs(ctx, &eventTypes, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	created, err := r.client.CreateWebhookEndpoint(ctx, orgID, capydb.CreateWebhookEndpointRequest{
		URL:         plan.URL.ValueString(),
		Description: plan.Description.ValueString(),
		EventTypes:  eventTypes,
	})
	if err != nil {
		resp.Diagnostics.AddError("Error creating CapyDB webhook endpoint", err.Error())
		return
	}

	endpoint := created.Endpoint

	// The create endpoint has no active flag; disable post-creation when the
	// practitioner asked for an inactive endpoint.
	if !plan.Active.ValueBool() {
		isActive := false
		endpoint, err = r.client.UpdateWebhookEndpoint(ctx, orgID, endpoint.ID, capydb.UpdateWebhookEndpointRequest{
			IsActive: &isActive,
		})
		if err != nil {
			resp.Diagnostics.AddError("Error disabling CapyDB webhook endpoint after creation", err.Error())
			return
		}
	}

	resp.Diagnostics.Append(plan.fill(ctx, endpoint)...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.SigningSecret = types.StringValue(created.PlaintextSecret)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *webhookEndpointResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state webhookEndpointModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	endpoint, err := r.client.GetWebhookEndpoint(ctx, state.OrganizationID.ValueString(), state.ID.ValueString())
	if err != nil {
		if capydb.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading CapyDB webhook endpoint", err.Error())
		return
	}

	resp.Diagnostics.Append(state.fill(ctx, endpoint)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// signing_secret and secret_version are kept from prior state: the API
	// never returns the secret after creation/rotation.
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *webhookEndpointResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state webhookEndpointModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	orgID := state.OrganizationID.ValueString()
	endpointID := state.ID.ValueString()

	update := capydb.UpdateWebhookEndpointRequest{}
	hasFieldUpdate := false
	if !plan.URL.Equal(state.URL) {
		urlValue := plan.URL.ValueString()
		update.URL = &urlValue
		hasFieldUpdate = true
	}
	if !plan.Description.Equal(state.Description) && !plan.Description.IsUnknown() {
		description := plan.Description.ValueString()
		update.Description = &description
		hasFieldUpdate = true
	}
	if !plan.EventTypes.Equal(state.EventTypes) && !plan.EventTypes.IsUnknown() && !plan.EventTypes.IsNull() {
		var eventTypes []string
		resp.Diagnostics.Append(plan.EventTypes.ElementsAs(ctx, &eventTypes, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if eventTypes == nil {
			eventTypes = []string{}
		}
		update.EventTypes = &eventTypes
		hasFieldUpdate = true
	}
	if !plan.Active.Equal(state.Active) {
		isActive := plan.Active.ValueBool()
		update.IsActive = &isActive
		hasFieldUpdate = true
	}

	endpoint, err := r.client.GetWebhookEndpoint(ctx, orgID, endpointID)
	if err != nil {
		resp.Diagnostics.AddError("Error reading CapyDB webhook endpoint during update", err.Error())
		return
	}

	if hasFieldUpdate {
		endpoint, err = r.client.UpdateWebhookEndpoint(ctx, orgID, endpointID, update)
		if err != nil {
			resp.Diagnostics.AddError("Error updating CapyDB webhook endpoint", err.Error())
			return
		}
	}

	signingSecret := state.SigningSecret
	if !plan.SecretVersion.Equal(state.SecretVersion) {
		tflog.Info(ctx, "rotating capydb webhook endpoint signing secret", map[string]any{
			"endpoint_id": endpointID,
		})
		rotated, err := r.client.RotateWebhookEndpointSecret(ctx, orgID, endpointID)
		if err != nil {
			resp.Diagnostics.AddError("Error rotating CapyDB webhook endpoint signing secret", err.Error())
			return
		}
		endpoint = rotated.Endpoint
		signingSecret = types.StringValue(rotated.PlaintextSecret)
	}

	resp.Diagnostics.Append(plan.fill(ctx, endpoint)...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.SigningSecret = signingSecret
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *webhookEndpointResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state webhookEndpointModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteWebhookEndpoint(ctx, state.OrganizationID.ValueString(), state.ID.ValueString())
	if err != nil && !capydb.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting CapyDB webhook endpoint", err.Error())
	}
}
