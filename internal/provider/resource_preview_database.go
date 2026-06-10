package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/capy-base/terraform-provider-capydb/internal/capydb"
)

var (
	_ resource.Resource              = (*previewDatabaseResource)(nil)
	_ resource.ResourceWithConfigure = (*previewDatabaseResource)(nil)
)

// NewPreviewDatabaseResource creates the capydb_preview_database resource.
func NewPreviewDatabaseResource() resource.Resource {
	return &previewDatabaseResource{}
}

type previewDatabaseResource struct {
	client *capydb.Client
}

type previewDatabaseModel struct {
	ID           types.String   `tfsdk:"id"`
	ProjectID    types.String   `tfsdk:"project_id"`
	Name         types.String   `tfsdk:"name"`
	Mode         types.String   `tfsdk:"mode"`
	TTLHours     types.Int64    `tfsdk:"ttl_hours"`
	State        types.String   `tfsdk:"state"`
	DatabaseName types.String   `tfsdk:"database_name"`
	TTLExpiresAt types.String   `tfsdk:"ttl_expires_at"`
	Timeouts     timeouts.Value `tfsdk:"timeouts"`
}

func (m *previewDatabaseModel) fill(preview capydb.PreviewDatabase) {
	m.ID = types.StringValue(preview.ID)
	m.ProjectID = types.StringValue(preview.ProjectID)
	m.Name = types.StringValue(preview.Name)
	m.Mode = types.StringValue(preview.Mode)
	m.State = types.StringValue(preview.State)
	m.DatabaseName = types.StringValue(preview.DatabaseName)
	m.TTLExpiresAt = types.StringValue(preview.TTLExpiresAt.UTC().Format(time.RFC3339))
}

// ttlExpiresAtPlanModifier marks ttl_expires_at unknown whenever ttl_hours
// changes. Terraform proposes the prior state for computed attributes, but an
// in-place TTL extension moves the expiry, so leaving the old timestamp in the
// plan would make the applied value an inconsistent result.
type ttlExpiresAtPlanModifier struct{}

func (ttlExpiresAtPlanModifier) Description(context.Context) string {
	return "Marks ttl_expires_at unknown when ttl_hours changes."
}

func (ttlExpiresAtPlanModifier) MarkdownDescription(context.Context) string {
	return "Marks `ttl_expires_at` unknown when `ttl_hours` changes."
}

func (ttlExpiresAtPlanModifier) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if req.StateValue.IsNull() {
		return
	}

	var planTTL, stateTTL types.Int64
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, req.Path.ParentPath().AtName("ttl_hours"), &planTTL)...)
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, req.Path.ParentPath().AtName("ttl_hours"), &stateTTL)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !planTTL.Equal(stateTTL) {
		resp.PlanValue = types.StringUnknown()
	}
}

func (r *previewDatabaseResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_preview_database"
}

func (r *previewDatabaseResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A disposable CapyDB preview/branch database for a project. Creation and deletion run as " +
			"asynchronous jobs which this resource waits on. Previews expire after their TTL; increasing " +
			"`ttl_hours` extends the expiry via the extend endpoint, decreasing it forces a replacement.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Preview database id.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"project_id": schema.StringAttribute{
				Required:    true,
				Description: "Project the preview belongs to. Changing it forces a replacement.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Preview name. Omit to let CapyDB generate one. Changing it forces a replacement.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"mode": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Description: "Data source mode: `clone` (copy of the production database) or `empty`. " +
					"Changing it forces a replacement.",
				Validators: []validator.String{
					stringvalidator.OneOf("clone", "empty"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"ttl_hours": schema.Int64Attribute{
				Required: true,
				Description: "Time to live in hours. Increasing it extends the preview's expiry by the " +
					"difference; decreasing it forces a replacement.",
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
				},
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplaceIf(
						func(_ context.Context, req planmodifier.Int64Request, resp *int64planmodifier.RequiresReplaceIfFuncResponse) {
							if !req.StateValue.IsNull() && !req.PlanValue.IsNull() &&
								req.PlanValue.ValueInt64() < req.StateValue.ValueInt64() {
								resp.RequiresReplace = true
							}
						},
						"Decreasing ttl_hours forces a replacement; the API only supports extending TTLs.",
						"Decreasing `ttl_hours` forces a replacement; the API only supports extending TTLs.",
					),
				},
			},
			"state": schema.StringAttribute{
				Computed:    true,
				Description: "Lifecycle state of the preview database.",
			},
			"database_name": schema.StringAttribute{
				Computed:    true,
				Description: "Name of the underlying Postgres database.",
			},
			"ttl_expires_at": schema.StringAttribute{
				Computed:    true,
				Description: "RFC 3339 timestamp the preview expires at.",
				PlanModifiers: []planmodifier.String{
					ttlExpiresAtPlanModifier{},
				},
			},
			"timeouts": timeouts.Attributes(ctx, timeouts.Opts{
				Create:            true,
				Delete:            true,
				CreateDescription: "Bound on waiting for the asynchronous create job. Defaults to 20m.",
				DeleteDescription: "Bound on waiting for the asynchronous deletion job. Defaults to 20m.",
			}),
		},
	}
}

func (r *previewDatabaseResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = resourceClient(req, resp)
}

func (r *previewDatabaseResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan previewDatabaseModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	createTimeout, diags := plan.Timeouts.Create(ctx, defaultJobTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	projectID := plan.ProjectID.ValueString()
	preview, job, err := r.client.CreatePreviewDatabase(ctx, projectID, capydb.CreatePreviewRequest{
		Name:     plan.Name.ValueString(),
		Mode:     plan.Mode.ValueString(),
		TTLHours: plan.TTLHours.ValueInt64(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error creating CapyDB preview database", err.Error())
		return
	}

	tflog.Info(ctx, "created capydb preview database, waiting for create job", map[string]any{
		"preview_id": preview.ID,
		"job_id":     job.ID,
	})

	// Persist the id immediately so a failed wait still leaves a trackable
	// resource in state instead of an orphaned preview.
	plan.fill(preview)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	waitCtx, cancel := context.WithTimeout(ctx, createTimeout)
	defer cancel()
	if _, err := r.client.WaitForJob(waitCtx, job.ID); err != nil {
		resp.Diagnostics.AddError(
			"CapyDB preview database creation did not complete",
			fmt.Sprintf("Preview %s was created but the create job did not complete: %s", preview.ID, err),
		)
		return
	}

	created, err := r.client.GetPreviewDatabase(ctx, projectID, preview.ID)
	if err != nil {
		resp.Diagnostics.AddError("Error reading CapyDB preview database after creation", err.Error())
		return
	}

	plan.fill(created)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *previewDatabaseResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state previewDatabaseModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	preview, err := r.client.GetPreviewDatabase(ctx, state.ProjectID.ValueString(), state.ID.ValueString())
	if err != nil {
		if capydb.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading CapyDB preview database", err.Error())
		return
	}

	state.fill(preview)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *previewDatabaseResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state previewDatabaseModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// ttl_hours is the only in-place updatable attribute. The API extends
	// TTLs by a relative number of hours, so send the configured delta.
	delta := plan.TTLHours.ValueInt64() - state.TTLHours.ValueInt64()
	if delta > 0 {
		preview, err := r.client.ExtendPreviewDatabase(ctx, state.ID.ValueString(), delta)
		if err != nil {
			resp.Diagnostics.AddError("Error extending CapyDB preview database TTL", err.Error())
			return
		}
		plan.fill(preview)
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
		return
	}

	preview, err := r.client.GetPreviewDatabase(ctx, state.ProjectID.ValueString(), state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading CapyDB preview database during update", err.Error())
		return
	}
	plan.fill(preview)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *previewDatabaseResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state previewDatabaseModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	deleteTimeout, diags := state.Timeouts.Delete(ctx, defaultJobTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	job, err := r.client.DeletePreviewDatabase(ctx, state.ID.ValueString())
	if err != nil {
		if capydb.IsNotFound(err) {
			return
		}
		resp.Diagnostics.AddError("Error deleting CapyDB preview database", err.Error())
		return
	}

	waitCtx, cancel := context.WithTimeout(ctx, deleteTimeout)
	defer cancel()
	if _, err := r.client.WaitForJob(waitCtx, job.ID); err != nil {
		resp.Diagnostics.AddError(
			"CapyDB preview database deletion did not complete",
			fmt.Sprintf("The deletion job for preview %s did not complete: %s", state.ID.ValueString(), err),
		)
	}
}
