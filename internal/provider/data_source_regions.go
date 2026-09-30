package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/capydatabase/terraform-provider-capydb/internal/capydb"
)

var (
	_ datasource.DataSource              = (*regionsDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*regionsDataSource)(nil)
)

// NewRegionsDataSource creates the capydb_regions data source.
func NewRegionsDataSource() datasource.DataSource {
	return &regionsDataSource{}
}

type regionsDataSource struct {
	client *capydb.Client
}

type regionModel struct {
	Slug types.String `tfsdk:"slug"`
}

type regionDetailModel struct {
	ID          types.String `tfsdk:"id"`
	DisplayName types.String `tfsdk:"display_name"`
	Location    types.String `tfsdk:"location"`
}

type regionsModel struct {
	Regions       []regionModel       `tfsdk:"regions"`
	RegionDetails []regionDetailModel `tfsdk:"region_details"`
}

func (d *regionsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_regions"
}

func (d *regionsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Lists the available CapyDB placement regions projects can be created in.",
		Attributes: map[string]schema.Attribute{
			"regions": schema.ListNestedAttribute{
				Computed:    true,
				Description: "Available placement regions.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"slug": schema.StringAttribute{
							Computed:    true,
							Description: "Region id (for example `eu-north-1`), the value `capydb_project.region` takes.",
						},
					},
				},
			},
			"region_details": schema.ListNestedAttribute{
				Computed:    true,
				Description: "The same regions, in the same order, with their display labels.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Computed:    true,
							Description: "Region id (for example `eu-north-1`), the value `capydb_project.region` takes.",
						},
						"display_name": schema.StringAttribute{Computed: true, Description: "Display label, for example `EU North`."},
						"location":     schema.StringAttribute{Computed: true, Description: "Where the region's nodes run, for example `Helsinki, Finland`."},
					},
				},
			},
		},
	}
}

func (d *regionsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = dataSourceClient(req, resp)
}

func (d *regionsDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	regions, err := d.client.ListRegions(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error listing CapyDB regions", err.Error())
		return
	}

	state := regionsModel{
		Regions:       make([]regionModel, 0, len(regions.Regions)),
		RegionDetails: make([]regionDetailModel, 0, len(regions.RegionDetails)),
	}
	for _, id := range regions.Regions {
		state.Regions = append(state.Regions, regionModel{Slug: types.StringValue(id)})
	}
	for _, detail := range regions.RegionDetails {
		state.RegionDetails = append(state.RegionDetails, regionDetailModel{
			ID:          types.StringValue(detail.ID),
			DisplayName: types.StringValue(detail.DisplayName),
			Location:    types.StringValue(detail.Location),
		})
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
