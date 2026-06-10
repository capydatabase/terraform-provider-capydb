package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/capy-base/terraform-provider-capydb/internal/capydb"
)

var (
	_ datasource.DataSource              = (*projectConnectionDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*projectConnectionDataSource)(nil)
)

// NewProjectConnectionDataSource creates the capydb_project_connection data
// source.
func NewProjectConnectionDataSource() datasource.DataSource {
	return &projectConnectionDataSource{}
}

type projectConnectionDataSource struct {
	client *capydb.Client
}

type projectConnectionModel struct {
	ProjectID types.String `tfsdk:"project_id"`
	PooledURL types.String `tfsdk:"pooled_url"`
	DirectURL types.String `tfsdk:"direct_url"`
	Username  types.String `tfsdk:"username"`
}

func (d *projectConnectionDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_project_connection"
}

func (d *projectConnectionDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Fetches a project's connection strings with the current credentials embedded. " +
			"Requires an API key with the `credentials:read` scope. The URLs are sensitive and end up in " +
			"Terraform state — treat the state file accordingly.",
		Attributes: map[string]schema.Attribute{
			"project_id": schema.StringAttribute{
				Required:    true,
				Description: "Project to fetch connection strings for.",
			},
			"pooled_url": schema.StringAttribute{
				Computed:    true,
				Sensitive:   true,
				Description: "Pooled (PgBouncer) connection URL — the default for applications.",
			},
			"direct_url": schema.StringAttribute{
				Computed:    true,
				Sensitive:   true,
				Description: "Direct Postgres connection URL for migrations and long-lived sessions.",
			},
			"username": schema.StringAttribute{
				Computed:    true,
				Description: "Database role the URLs authenticate as.",
			},
		},
	}
}

func (d *projectConnectionDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = dataSourceClient(req, resp)
}

func (d *projectConnectionDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config projectConnectionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	connections, err := d.client.GetProjectConnections(ctx, config.ProjectID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading CapyDB project connections", err.Error())
		return
	}

	config.PooledURL = types.StringValue(connections.PooledURL)
	config.DirectURL = types.StringValue(connections.DirectURL)
	config.Username = types.StringValue(connections.Username)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
