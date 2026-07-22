package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/capy-base/terraform-provider-capydb/internal/capydb"
)

var (
	_ datasource.DataSource              = (*organizationDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*organizationDataSource)(nil)
)

// NewOrganizationDataSource creates the capydb_organization data source.
func NewOrganizationDataSource() datasource.DataSource {
	return &organizationDataSource{}
}

type organizationDataSource struct {
	client *capydb.Client
}

type organizationModel struct {
	ID            types.String `tfsdk:"id"`
	Name          types.String `tfsdk:"name"`
	Slug          types.String `tfsdk:"slug"`
	BillingPlan   types.String `tfsdk:"billing_plan"`
	BillingStatus types.String `tfsdk:"billing_status"`
}

func (d *organizationDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_organization"
}

func (d *organizationDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Returns the organization the configured API key belongs to (the authenticated viewer).",
		Attributes: map[string]schema.Attribute{
			"id":             schema.StringAttribute{Computed: true, Description: "Organization id."},
			"name":           schema.StringAttribute{Computed: true, Description: "Organization name."},
			"slug":           schema.StringAttribute{Computed: true, Description: "Organization slug."},
			"billing_plan":   schema.StringAttribute{Computed: true, Description: "Current billing plan (e.g. vibe, ship, business)."},
			"billing_status": schema.StringAttribute{Computed: true, Description: "Billing subscription status."},
		},
	}
}

func (d *organizationDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = dataSourceClient(req, resp)
}

func (d *organizationDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	viewer, err := d.client.GetViewer(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading CapyDB viewer", err.Error())
		return
	}
	if viewer.Organization == nil {
		resp.Diagnostics.AddError(
			"CapyDB principal has no organization",
			"The configured credential is not bound to an organization (e.g. a platform admin token). "+
				"Use an organization API key.",
		)
		return
	}

	state := organizationModel{
		ID:            types.StringValue(viewer.Organization.ID),
		Name:          types.StringValue(viewer.Organization.Name),
		Slug:          types.StringValue(viewer.Organization.Slug),
		BillingPlan:   types.StringValue(viewer.Organization.BillingPlan),
		BillingStatus: types.StringValue(viewer.Organization.BillingStatus),
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
