package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/capydatabase/terraform-provider-capydb/internal/capydb"
)

var (
	_ datasource.DataSource              = (*postgresVersionsDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*postgresVersionsDataSource)(nil)
)

// NewPostgresVersionsDataSource creates the capydb_postgres_versions data source.
func NewPostgresVersionsDataSource() datasource.DataSource {
	return &postgresVersionsDataSource{}
}

type postgresVersionsDataSource struct {
	client *capydb.Client
}

type postgresVersionModel struct {
	Version         types.String `tfsdk:"version"`
	Channel         types.String `tfsdk:"channel"`
	Default         types.Bool   `tfsdk:"default"`
	ProductionReady types.Bool   `tfsdk:"production_ready"`
}

type postgresVersionsModel struct {
	Versions       []postgresVersionModel `tfsdk:"versions"`
	DefaultVersion types.String           `tfsdk:"default_version"`
}

func (d *postgresVersionsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_postgres_versions"
}

func (d *postgresVersionsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Lists the Postgres major versions a new CapyDB database can be created on, oldest first.",
		Attributes: map[string]schema.Attribute{
			"versions": schema.ListNestedAttribute{
				Computed:    true,
				Description: "Postgres majors open for new databases, oldest first.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"version": schema.StringAttribute{
							Computed:    true,
							Description: "Major version (for example `17`), the value `capydb_project.postgres_version` takes.",
						},
						"channel": schema.StringAttribute{
							Computed:    true,
							Description: "Release channel: `previous`, `stable`, `current` or `beta`.",
						},
						"default": schema.BoolAttribute{
							Computed:    true,
							Description: "Whether a new database gets this major when `postgres_version` is not set.",
						},
						"production_ready": schema.BoolAttribute{
							Computed:    true,
							Description: "Whether this major is fit for production data. False for a `beta` major, an upstream PostgreSQL beta offered for evaluation only.",
						},
					},
				},
			},
			"default_version": schema.StringAttribute{
				Computed:    true,
				Description: "The `version` of the entry marked `default`. Null if no entry is marked.",
			},
		},
	}
}

func (d *postgresVersionsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = dataSourceClient(req, resp)
}

func (d *postgresVersionsDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	versions, err := d.client.ListPostgresVersions(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error listing CapyDB Postgres versions", err.Error())
		return
	}

	state := postgresVersionsModel{
		Versions:       make([]postgresVersionModel, 0, len(versions.Versions)),
		DefaultVersion: types.StringNull(),
	}
	for _, v := range versions.Versions {
		state.Versions = append(state.Versions, postgresVersionModel{
			Version:         types.StringValue(v.Version),
			Channel:         types.StringValue(v.Channel),
			Default:         types.BoolValue(v.Default),
			ProductionReady: types.BoolValue(v.ProductionReady),
		})
		if v.Default {
			state.DefaultVersion = types.StringValue(v.Version)
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
