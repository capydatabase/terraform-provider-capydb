// Package provider implements the Terraform/OpenTofu provider for CapyDB
// (managed Postgres hosting). It is built on terraform-plugin-framework and
// talks to the control plane through internal/capydb.
package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/capy-base/terraform-provider-capydb/internal/capydb"
)

// Environment variable fallbacks for the provider configuration.
const (
	envAPIKey = "CAPYDB_API_KEY"
	envAPIURL = "CAPYDB_API_URL"
)

// Ensure capydbProvider satisfies the framework interfaces.
var _ provider.Provider = (*capydbProvider)(nil)

// capydbProvider implements the CapyDB Terraform provider.
type capydbProvider struct {
	// version is "dev" for local builds and the release version for
	// published binaries.
	version string
}

// New returns a provider factory for the given build version.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &capydbProvider{version: version}
	}
}

type providerModel struct {
	APIKey types.String `tfsdk:"api_key"`
	APIURL types.String `tfsdk:"api_url"`
}

func (p *capydbProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "capydb"
	resp.Version = p.version
}

func (p *capydbProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manage CapyDB managed Postgres projects, preview databases, API keys, and webhook endpoints.",
		Attributes: map[string]schema.Attribute{
			"api_key": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "CapyDB organization API key (format `capy_live_...`). Falls back to the `CAPYDB_API_KEY` environment variable.",
			},
			"api_url": schema.StringAttribute{
				Optional:    true,
				Description: "CapyDB API base URL. Falls back to the `CAPYDB_API_URL` environment variable, then to `" + capydb.DefaultBaseURL + "`.",
			},
		},
	}
}

func (p *capydbProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if config.APIKey.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("api_key"),
			"Unknown CapyDB API key",
			"The provider cannot connect to CapyDB because api_key is derived from a value that is not yet known. "+
				"Set a static value or the CAPYDB_API_KEY environment variable.",
		)
	}
	if config.APIURL.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("api_url"),
			"Unknown CapyDB API URL",
			"The provider cannot connect to CapyDB because api_url is derived from a value that is not yet known. "+
				"Set a static value or the CAPYDB_API_URL environment variable.",
		)
	}
	if resp.Diagnostics.HasError() {
		return
	}

	apiKey := os.Getenv(envAPIKey)
	if !config.APIKey.IsNull() {
		apiKey = config.APIKey.ValueString()
	}
	apiURL := os.Getenv(envAPIURL)
	if !config.APIURL.IsNull() {
		apiURL = config.APIURL.ValueString()
	}
	if apiURL == "" {
		apiURL = capydb.DefaultBaseURL
	}

	if apiKey == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("api_key"),
			"Missing CapyDB API key",
			"No api_key was configured and the CAPYDB_API_KEY environment variable is empty. "+
				"Create an organization API key in the CapyDB dashboard and provide it via the provider block or CAPYDB_API_KEY.",
		)
		return
	}

	client, err := capydb.NewClient(apiURL, apiKey, p.version)
	if err != nil {
		resp.Diagnostics.AddAttributeError(
			path.Root("api_url"),
			"Invalid CapyDB API URL",
			err.Error(),
		)
		return
	}

	tflog.Debug(ctx, "configured capydb client", map[string]any{"api_url": apiURL})

	resp.DataSourceData = client
	resp.ResourceData = client
}

func (p *capydbProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewProjectResource,
		NewPreviewDatabaseResource,
		NewAPIKeyResource,
		NewWebhookEndpointResource,
	}
}

func (p *capydbProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewClustersDataSource,
		NewProjectDataSource,
		NewProjectConnectionDataSource,
		NewOrganizationDataSource,
	}
}

// resourceClient extracts the configured *capydb.Client from resource
// configure request data, reporting a diagnostic on type mismatch.
func resourceClient(req resource.ConfigureRequest, resp *resource.ConfigureResponse) *capydb.Client {
	if req.ProviderData == nil {
		return nil
	}
	client, ok := req.ProviderData.(*capydb.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected resource configure type",
			"Expected *capydb.Client from the provider. This is a bug in the provider.",
		)
		return nil
	}
	return client
}

// dataSourceClient extracts the configured *capydb.Client from data source
// configure request data, reporting a diagnostic on type mismatch.
func dataSourceClient(req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) *capydb.Client {
	if req.ProviderData == nil {
		return nil
	}
	client, ok := req.ProviderData.(*capydb.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected data source configure type",
			"Expected *capydb.Client from the provider. This is a bug in the provider.",
		)
		return nil
	}
	return client
}
