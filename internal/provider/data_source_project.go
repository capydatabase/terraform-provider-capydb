package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/capy-base/terraform-provider-capydb/internal/capydb"
)

var (
	_ datasource.DataSource                     = (*projectDataSource)(nil)
	_ datasource.DataSourceWithConfigure        = (*projectDataSource)(nil)
	_ datasource.DataSourceWithConfigValidators = (*projectDataSource)(nil)
)

// NewProjectDataSource creates the capydb_project data source.
func NewProjectDataSource() datasource.DataSource {
	return &projectDataSource{}
}

type projectDataSource struct {
	client *capydb.Client
}

type projectDataSourceModel struct {
	ID             types.String `tfsdk:"id"`
	Slug           types.String `tfsdk:"slug"`
	Name           types.String `tfsdk:"name"`
	ClusterID      types.String `tfsdk:"cluster_id"`
	Environment    types.String `tfsdk:"environment"`
	Plan           types.String `tfsdk:"plan"`
	Region         types.String `tfsdk:"region"`
	State          types.String `tfsdk:"state"`
	OrganizationID types.String `tfsdk:"organization_id"`
	DatabaseName   types.String `tfsdk:"database_name"`
}

func (m *projectDataSourceModel) fill(project capydb.Project) {
	m.ID = types.StringValue(project.ID)
	m.Slug = types.StringValue(project.Slug)
	m.Name = types.StringValue(project.Name)
	m.ClusterID = types.StringValue(project.ClusterID)
	m.Environment = types.StringValue(project.Environment)
	m.Plan = types.StringValue(project.Plan)
	m.Region = types.StringValue(project.Region)
	m.State = types.StringValue(project.State)
	m.OrganizationID = types.StringValue(project.OrganizationID)
	m.DatabaseName = types.StringValue(project.DatabaseName)
}

func (d *projectDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_project"
}

func (d *projectDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Looks up a CapyDB project by `id` or by `slug` (exactly one must be set).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Project id. Exactly one of `id` or `slug` must be set.",
			},
			"slug": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Project slug. Exactly one of `id` or `slug` must be set.",
			},
			"name":            schema.StringAttribute{Computed: true, Description: "Project name."},
			"cluster_id":      schema.StringAttribute{Computed: true, Description: "Cluster the project lives on."},
			"environment":     schema.StringAttribute{Computed: true, Description: "Environment label."},
			"plan":            schema.StringAttribute{Computed: true, Description: "Billing-derived project plan."},
			"region":          schema.StringAttribute{Computed: true, Description: "Region the project's cluster lives in."},
			"state":           schema.StringAttribute{Computed: true, Description: "Lifecycle state."},
			"organization_id": schema.StringAttribute{Computed: true, Description: "Owning organization id."},
			"database_name":   schema.StringAttribute{Computed: true, Description: "Underlying Postgres database name."},
		},
	}
}

func (d *projectDataSource) ConfigValidators(_ context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(
			path.MatchRoot("id"),
			path.MatchRoot("slug"),
		),
	}
}

func (d *projectDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = dataSourceClient(req, resp)
}

func (d *projectDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config projectDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var project capydb.Project
	switch {
	case !config.ID.IsNull():
		found, err := d.client.GetProject(ctx, config.ID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Error reading CapyDB project", err.Error())
			return
		}
		project = found
	default:
		slug := config.Slug.ValueString()
		projects, err := d.client.ListProjects(ctx)
		if err != nil {
			resp.Diagnostics.AddError("Error listing CapyDB projects", err.Error())
			return
		}
		found := false
		for _, candidate := range projects {
			if candidate.Slug == slug {
				project = candidate
				found = true
				break
			}
		}
		if !found {
			resp.Diagnostics.AddError(
				"CapyDB project not found",
				fmt.Sprintf("No project with slug %q exists in the organization.", slug),
			)
			return
		}
	}

	config.fill(project)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
