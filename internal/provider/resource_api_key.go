package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/capy-base/terraform-provider-capydb/internal/capydb"
)

var (
	_ resource.Resource              = (*apiKeyResource)(nil)
	_ resource.ResourceWithConfigure = (*apiKeyResource)(nil)
)

// NewAPIKeyResource creates the capydb_api_key resource.
func NewAPIKeyResource() resource.Resource {
	return &apiKeyResource{}
}

type apiKeyResource struct {
	client *capydb.Client
}

type apiKeyModel struct {
	ID             types.String `tfsdk:"id"`
	OrganizationID types.String `tfsdk:"organization_id"`
	Name           types.String `tfsdk:"name"`
	Scopes         types.List   `tfsdk:"scopes"`
	ProjectID      types.String `tfsdk:"project_id"`
	Token          types.String `tfsdk:"token"`
	KeyPrefix      types.String `tfsdk:"key_prefix"`
	IsActive       types.Bool   `tfsdk:"is_active"`
}

func (r *apiKeyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_api_key"
}

func (r *apiKeyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A CapyDB organization API key. The plaintext key is returned by the API exactly once at " +
			"creation and is stored in Terraform state as the sensitive `token` attribute - the API never returns " +
			"it again, so drift on the secret itself cannot be detected (only metadata such as revocation is). " +
			"All configurable attributes force a replacement; deleting the resource revokes the key.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "API key id.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"organization_id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Organization the key belongs to. Defaults to the authenticated principal's organization.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Human-readable key name. Changing it forces a replacement.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"scopes": schema.ListAttribute{
				Required:    true,
				ElementType: types.StringType,
				Description: "Scopes granted to the key, e.g. `projects:read`, `projects:write`, " +
					"`credentials:read`, `backups:read`, `backups:write`, `jobs:read`. Changing it forces a replacement.",
				Validators: []validator.List{
					listvalidator.SizeAtLeast(1),
				},
				PlanModifiers: []planmodifier.List{
					listplanmodifier.RequiresReplace(),
				},
			},
			"project_id": schema.StringAttribute{
				Optional: true,
				Description: "Restricts the key to a single project. Project-scoped keys cannot touch sibling " +
					"projects, manage other keys, or mutate the organization. Changing it forces a replacement.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"token": schema.StringAttribute{
				Computed:  true,
				Sensitive: true,
				Description: "The plaintext API key (format `capy_live_...`). Returned exactly once at creation " +
					"and stored in state; rotation of the secret outside Terraform cannot be detected.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"key_prefix": schema.StringAttribute{
				Computed:    true,
				Description: "Non-secret key prefix used to identify the key.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"is_active": schema.BoolAttribute{
				Computed:    true,
				Description: "Whether the key is active (not revoked or expired).",
			},
		},
	}
}

func (r *apiKeyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = resourceClient(req, resp)
}

// resolveOrganizationID returns the configured org id, falling back to the
// authenticated principal's organization via /v1/me.
func resolveOrganizationID(ctx context.Context, client *capydb.Client, configured types.String) (string, error) {
	if !configured.IsNull() && !configured.IsUnknown() && configured.ValueString() != "" {
		return configured.ValueString(), nil
	}
	viewer, err := client.GetViewer(ctx)
	if err != nil {
		return "", err
	}
	if viewer.Organization == nil {
		return "", &capydb.APIError{
			Message:    "the authenticated principal is not bound to an organization; set organization_id explicitly",
			StatusCode: 0,
		}
	}
	return viewer.Organization.ID, nil
}

func (r *apiKeyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan apiKeyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	orgID, err := resolveOrganizationID(ctx, r.client, plan.OrganizationID)
	if err != nil {
		resp.Diagnostics.AddError("Error resolving CapyDB organization", err.Error())
		return
	}

	var scopes []string
	resp.Diagnostics.Append(plan.Scopes.ElementsAs(ctx, &scopes, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.CreateAPIKey(ctx, orgID, capydb.CreateAPIKeyRequest{
		Name:      plan.Name.ValueString(),
		Scopes:    scopes,
		ProjectID: plan.ProjectID.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error creating CapyDB API key", err.Error())
		return
	}

	plan.ID = types.StringValue(created.APIKey.ID)
	plan.OrganizationID = types.StringValue(created.APIKey.OrganizationID)
	plan.Token = types.StringValue(created.PlaintextKey)
	plan.KeyPrefix = types.StringValue(created.APIKey.KeyPrefix)
	plan.IsActive = types.BoolValue(created.APIKey.IsActive)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *apiKeyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state apiKeyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	key, err := r.client.GetAPIKey(ctx, state.OrganizationID.ValueString(), state.ID.ValueString())
	if err != nil {
		if capydb.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading CapyDB API key", err.Error())
		return
	}

	// A revoked key is gone for all practical purposes - drop it from state
	// so Terraform plans a recreation.
	if !key.IsActive {
		resp.State.RemoveResource(ctx)
		return
	}

	state.Name = types.StringValue(key.Name)
	state.KeyPrefix = types.StringValue(key.KeyPrefix)
	state.IsActive = types.BoolValue(key.IsActive)
	if key.ProjectID != "" {
		state.ProjectID = types.StringValue(key.ProjectID)
	}
	scopes, diags := types.ListValueFrom(ctx, types.StringType, key.Scopes)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	state.Scopes = scopes
	// The plaintext token in state is left untouched: the API never returns
	// it again, so it cannot be refreshed or drift-checked.
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *apiKeyResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Every configurable attribute carries RequiresReplace, so the framework
	// never plans an in-place update for this resource.
	resp.Diagnostics.AddError(
		"CapyDB API keys cannot be updated in place",
		"All capydb_api_key attributes force a replacement; this update path should be unreachable. "+
			"Please report this as a provider bug.",
	)
}

func (r *apiKeyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state apiKeyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.RevokeAPIKey(ctx, state.OrganizationID.ValueString(), state.ID.ValueString())
	if err != nil && !capydb.IsNotFound(err) {
		resp.Diagnostics.AddError("Error revoking CapyDB API key", err.Error())
	}
}
