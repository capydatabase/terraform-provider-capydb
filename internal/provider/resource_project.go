package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/capy-base/terraform-provider-capydb/internal/capydb"
)

// defaultJobTimeout bounds async provision/delete job waits unless overridden
// via the timeouts block.
const defaultJobTimeout = 20 * time.Minute

var (
	_ resource.Resource                = (*projectResource)(nil)
	_ resource.ResourceWithConfigure   = (*projectResource)(nil)
	_ resource.ResourceWithImportState = (*projectResource)(nil)
)

// NewProjectResource creates the capydb_project resource.
func NewProjectResource() resource.Resource {
	return &projectResource{}
}

type projectResource struct {
	client *capydb.Client
}

type projectModel struct {
	ID             types.String   `tfsdk:"id"`
	Name           types.String   `tfsdk:"name"`
	ClusterID      types.String   `tfsdk:"cluster_id"`
	Environment    types.String   `tfsdk:"environment"`
	Slug           types.String   `tfsdk:"slug"`
	Plan           types.String   `tfsdk:"plan"`
	Region         types.String   `tfsdk:"region"`
	State          types.String   `tfsdk:"state"`
	OrganizationID types.String   `tfsdk:"organization_id"`
	DatabaseName   types.String   `tfsdk:"database_name"`
	Timeouts       timeouts.Value `tfsdk:"timeouts"`
}

func (m *projectModel) fill(project capydb.Project) {
	m.ID = types.StringValue(project.ID)
	m.Name = types.StringValue(project.Name)
	m.ClusterID = types.StringValue(project.ClusterID)
	m.Environment = types.StringValue(project.Environment)
	m.Slug = types.StringValue(project.Slug)
	m.Plan = types.StringValue(project.Plan)
	m.Region = types.StringValue(project.Region)
	m.State = types.StringValue(project.State)
	m.OrganizationID = types.StringValue(project.OrganizationID)
	m.DatabaseName = types.StringValue(project.DatabaseName)
}

func (r *projectResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_project"
}

func (r *projectResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A CapyDB project (a managed logical Postgres database). Provisioning and deletion run as " +
			"asynchronous jobs which this resource waits on. The project plan is derived from the organization's " +
			"billing state and cannot be configured here.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Project id.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required: true,
				Description: "Project name. The CapyDB API does not support renaming projects, so changing the " +
					"name forces a replacement.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"cluster_id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Cluster to place the project on. Omit to let CapyDB pick. Changing it forces a replacement.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"environment": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Environment label, either `production` or `nonproduction`. Updatable in place.",
				Validators: []validator.String{
					stringvalidator.OneOf("production", "nonproduction"),
				},
			},
			"slug": schema.StringAttribute{
				Computed:    true,
				Description: "URL-safe project slug derived from the name.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"plan": schema.StringAttribute{
				Computed: true,
				Description: "Project plan (e.g. `free`, `launch`, `scale`). Derived from the organization's " +
					"billing state; never configurable.",
			},
			"region": schema.StringAttribute{
				Computed:    true,
				Description: "Region the project's cluster lives in.",
			},
			"state": schema.StringAttribute{
				Computed:    true,
				Description: "Lifecycle state of the project.",
			},
			"organization_id": schema.StringAttribute{
				Computed:    true,
				Description: "Owning organization id.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"database_name": schema.StringAttribute{
				Computed:    true,
				Description: "Name of the underlying Postgres database.",
			},
			"timeouts": timeouts.Attributes(ctx, timeouts.Opts{
				Create:            true,
				Delete:            true,
				CreateDescription: "Bound on waiting for the asynchronous provision job. Defaults to 20m.",
				DeleteDescription: "Bound on waiting for the asynchronous deletion job. Defaults to 20m.",
			}),
		},
	}
}

func (r *projectResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = resourceClient(req, resp)
}

func (r *projectResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan projectModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	createTimeout, diags := plan.Timeouts.Create(ctx, defaultJobTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	request := capydb.CreateProjectRequest{
		Name:        plan.Name.ValueString(),
		ClusterID:   plan.ClusterID.ValueString(),
		Environment: plan.Environment.ValueString(),
	}

	project, job, err := r.client.CreateProject(ctx, request)
	if err != nil {
		resp.Diagnostics.AddError("Error creating CapyDB project", err.Error())
		return
	}

	tflog.Info(ctx, "created capydb project, waiting for provision job", map[string]any{
		"project_id": project.ID,
		"job_id":     job.ID,
	})

	// Persist the id immediately so a failed wait still leaves a trackable
	// resource in state instead of an orphaned project.
	plan.fill(project)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	waitCtx, cancel := context.WithTimeout(ctx, createTimeout)
	defer cancel()
	if _, err := r.client.WaitForJob(waitCtx, job.ID); err != nil {
		resp.Diagnostics.AddError(
			"CapyDB project provisioning did not complete",
			fmt.Sprintf("Project %s was created but the provision job did not complete: %s. "+
				"The project remains in state; re-run terraform apply or inspect the job in the dashboard.", project.ID, err),
		)
		return
	}

	provisioned, err := r.client.GetProject(ctx, project.ID)
	if err != nil {
		resp.Diagnostics.AddError("Error reading CapyDB project after provisioning", err.Error())
		return
	}

	plan.fill(provisioned)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *projectResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state projectModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	project, err := r.client.GetProject(ctx, state.ID.ValueString())
	if err != nil {
		if capydb.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading CapyDB project", err.Error())
		return
	}

	state.fill(project)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *projectResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state projectModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Environment is the only in-place updatable attribute (PATCH
	// /v1/projects/{id} only accepts `environment`).
	if !plan.Environment.IsNull() && !plan.Environment.IsUnknown() &&
		plan.Environment.ValueString() != state.Environment.ValueString() {
		environment := plan.Environment.ValueString()
		project, err := r.client.UpdateProject(ctx, state.ID.ValueString(), capydb.UpdateProjectRequest{
			Environment: &environment,
		})
		if err != nil {
			resp.Diagnostics.AddError("Error updating CapyDB project", err.Error())
			return
		}
		plan.fill(project)
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
		return
	}

	project, err := r.client.GetProject(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading CapyDB project during update", err.Error())
		return
	}
	plan.fill(project)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *projectResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state projectModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	deleteTimeout, diags := state.Timeouts.Delete(ctx, defaultJobTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	job, err := r.client.DeleteProject(ctx, state.ID.ValueString())
	if err != nil {
		if capydb.IsNotFound(err) {
			return
		}
		resp.Diagnostics.AddError("Error deleting CapyDB project", err.Error())
		return
	}

	waitCtx, cancel := context.WithTimeout(ctx, deleteTimeout)
	defer cancel()
	if _, err := r.client.WaitForJob(waitCtx, job.ID); err != nil {
		resp.Diagnostics.AddError(
			"CapyDB project deletion did not complete",
			fmt.Sprintf("The deletion job for project %s did not complete: %s", state.ID.ValueString(), err),
		)
	}
}

func (r *projectResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
