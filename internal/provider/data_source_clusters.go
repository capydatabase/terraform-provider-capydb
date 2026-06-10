package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/capy-base/terraform-provider-capydb/internal/capydb"
)

var (
	_ datasource.DataSource              = (*clustersDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*clustersDataSource)(nil)
)

// NewClustersDataSource creates the capydb_clusters data source.
func NewClustersDataSource() datasource.DataSource {
	return &clustersDataSource{}
}

type clustersDataSource struct {
	client *capydb.Client
}

type clusterModel struct {
	ID                          types.String `tfsdk:"id"`
	Name                        types.String `tfsdk:"name"`
	Provider                    types.String `tfsdk:"provider"`
	Region                      types.String `tfsdk:"region"`
	PublicHost                  types.String `tfsdk:"public_host"`
	DirectPort                  types.Int64  `tfsdk:"direct_port"`
	PooledPort                  types.Int64  `tfsdk:"pooled_port"`
	PostgresVersion             types.String `tfsdk:"postgres_version"`
	Extensions                  types.List   `tfsdk:"extensions"`
	State                       types.String `tfsdk:"state"`
	SupportsPointInTimeRecovery types.Bool   `tfsdk:"supports_point_in_time_recovery"`
}

type clustersModel struct {
	Clusters []clusterModel `tfsdk:"clusters"`
}

func (d *clustersDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_clusters"
}

func (d *clustersDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Lists the active CapyDB clusters (regions) projects can be created on.",
		Attributes: map[string]schema.Attribute{
			"clusters": schema.ListNestedAttribute{
				Computed:    true,
				Description: "Active clusters.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":               schema.StringAttribute{Computed: true, Description: "Cluster id."},
						"name":             schema.StringAttribute{Computed: true, Description: "Cluster name."},
						"provider":         schema.StringAttribute{Computed: true, Description: "Infrastructure provider."},
						"region":           schema.StringAttribute{Computed: true, Description: "Region identifier."},
						"public_host":      schema.StringAttribute{Computed: true, Description: "Public database host."},
						"direct_port":      schema.Int64Attribute{Computed: true, Description: "Direct Postgres port."},
						"pooled_port":      schema.Int64Attribute{Computed: true, Description: "Pooled (PgBouncer) port."},
						"postgres_version": schema.StringAttribute{Computed: true, Description: "Postgres major version."},
						"extensions": schema.ListAttribute{
							Computed:    true,
							ElementType: types.StringType,
							Description: "Postgres extensions installed on the cluster.",
						},
						"state": schema.StringAttribute{Computed: true, Description: "Cluster state."},
						"supports_point_in_time_recovery": schema.BoolAttribute{
							Computed:    true,
							Description: "Whether the cluster supports point-in-time recovery.",
						},
					},
				},
			},
		},
	}
}

func (d *clustersDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = dataSourceClient(req, resp)
}

func (d *clustersDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	clusters, err := d.client.ListClusters(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error listing CapyDB clusters", err.Error())
		return
	}

	state := clustersModel{Clusters: make([]clusterModel, 0, len(clusters))}
	for _, cluster := range clusters {
		extensions, diags := types.ListValueFrom(ctx, types.StringType, cluster.Extensions)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		state.Clusters = append(state.Clusters, clusterModel{
			ID:                          types.StringValue(cluster.ID),
			Name:                        types.StringValue(cluster.Name),
			Provider:                    types.StringValue(cluster.Provider),
			Region:                      types.StringValue(cluster.Region),
			PublicHost:                  types.StringValue(cluster.PublicHost),
			DirectPort:                  types.Int64Value(cluster.DirectPort),
			PooledPort:                  types.Int64Value(cluster.PooledPort),
			PostgresVersion:             types.StringValue(cluster.PostgresVersion),
			Extensions:                  extensions,
			State:                       types.StringValue(cluster.State),
			SupportsPointInTimeRecovery: types.BoolValue(cluster.SupportsPointInTimeRecovery),
		})
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
