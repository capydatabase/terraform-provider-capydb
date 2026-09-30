package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// The beta major (19) passes the provider's validation so the control plane
// decides whether it is on offer; anything outside the catalog still fails at
// plan time.
func TestProjectPostgresVersionValidator(t *testing.T) {
	s := resourceSchema(t, NewProjectResource())
	attribute, ok := s.Attributes["postgres_version"].(rschema.StringAttribute)
	if !ok {
		t.Fatal("postgres_version is not a string attribute")
	}

	cases := map[string]bool{"16": true, "17": true, "18": true, "19": true, "15": false, "20": false}
	for version, valid := range cases {
		var diagsHaveError bool
		for _, v := range attribute.Validators {
			resp := validator.StringResponse{}
			v.ValidateString(context.Background(), validator.StringRequest{
				Path:        path.Root("postgres_version"),
				ConfigValue: types.StringValue(version),
			}, &resp)
			diagsHaveError = diagsHaveError || resp.Diagnostics.HasError()
		}
		if diagsHaveError == valid {
			t.Errorf("postgres_version %q: validation error = %v, want valid = %v", version, diagsHaveError, valid)
		}
	}
}

func TestProjectResourceCreatesOnBetaMajor(t *testing.T) {
	ctx := context.Background()
	client, mock := newTestClient(t)

	r := NewProjectResource()
	configureResource(t, r, client)
	s := resourceSchema(t, r)
	schemaType := s.Type().TerraformType(ctx)
	planRaw := objectValue(t, schemaType, map[string]tftypes.Value{
		"name":             str("beta app"),
		"environment":      str("non_production"),
		"postgres_version": str("19"),
	})

	// The platform flag is off by default: the API refuses the beta major.
	refused := resource.CreateResponse{State: emptyResourceState(t, s)}
	r.Create(ctx, resource.CreateRequest{
		Plan:   tfsdk.Plan{Schema: s, Raw: planRaw},
		Config: tfsdk.Config{Schema: s, Raw: planRaw},
	}, &refused)
	if !refused.Diagnostics.HasError() {
		t.Fatal("creating on 19 while CapyDB does not offer it must fail")
	}

	mock.mu.Lock()
	mock.postgresBetaEnabled = true
	mock.mu.Unlock()
	created := resource.CreateResponse{State: emptyResourceState(t, s)}
	r.Create(ctx, resource.CreateRequest{
		Plan:   tfsdk.Plan{Schema: s, Raw: planRaw},
		Config: tfsdk.Config{Schema: s, Raw: planRaw},
	}, &created)
	requireNoDiags(t, "create on 19", created.Diagnostics)
	if got := stateString(t, created.State, "postgres_version"); got != "19" {
		t.Errorf("postgres_version = %q, want 19", got)
	}
	if got := stateString(t, created.State, "postgres_channel"); got != "beta" {
		t.Errorf("postgres_channel = %q, want beta", got)
	}
	if got := stateString(t, created.State, "postgres_warning"); got != mockPostgresBetaWarning {
		t.Errorf("postgres_warning = %q, want the beta warning", got)
	}
}

// A GA major reports its channel and no warning (null, not ""), and a refresh
// picks up a channel the API reports differently later - after a major
// upgrade or when a newer major pushes this one to another channel.
func TestProjectPostgresChannelAndWarning(t *testing.T) {
	ctx := context.Background()
	client, mock := newTestClient(t)

	r := NewProjectResource()
	configureResource(t, r, client)
	s := resourceSchema(t, r)
	schemaType := s.Type().TerraformType(ctx)
	planRaw := objectValue(t, schemaType, map[string]tftypes.Value{
		"name":             str("ga app"),
		"environment":      str("non_production"),
		"postgres_version": str("18"),
	})
	created := resource.CreateResponse{State: emptyResourceState(t, s)}
	r.Create(ctx, resource.CreateRequest{
		Plan:   tfsdk.Plan{Schema: s, Raw: planRaw},
		Config: tfsdk.Config{Schema: s, Raw: planRaw},
	}, &created)
	requireNoDiags(t, "create", created.Diagnostics)
	if got := stateString(t, created.State, "postgres_channel"); got != "current" {
		t.Errorf("postgres_channel = %q, want current", got)
	}
	var warning types.String
	requireNoDiags(t, "get postgres_warning", created.State.GetAttribute(ctx, path.Root("postgres_warning"), &warning))
	if !warning.IsNull() {
		t.Errorf("postgres_warning = %v, want null for a production-ready major", warning)
	}

	projectID := stateString(t, created.State, "id")
	mock.mu.Lock()
	mock.projects[projectID].PostgresChannel = "stable"
	mock.mu.Unlock()
	readResp := resource.ReadResponse{State: created.State}
	r.Read(ctx, resource.ReadRequest{State: created.State}, &readResp)
	requireNoDiags(t, "read", readResp.Diagnostics)
	if got := stateString(t, readResp.State, "postgres_channel"); got != "stable" {
		t.Errorf("postgres_channel after refresh = %q, want stable", got)
	}

	// The data source renders the same fields.
	d := NewProjectDataSource()
	configureDataSource(t, d, client)
	ds := dataSourceSchema(t, d)
	dsType := ds.Type().TerraformType(ctx)
	dsResp := datasource.ReadResponse{State: tfsdk.State{Schema: ds, Raw: tftypes.NewValue(dsType, nil)}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: ds, Raw: objectValue(t, dsType, map[string]tftypes.Value{
		"id": str(projectID),
	})}}, &dsResp)
	requireNoDiags(t, "data source read", dsResp.Diagnostics)
	if got := stateString(t, dsResp.State, "postgres_channel"); got != "stable" {
		t.Errorf("data source postgres_channel = %q, want stable", got)
	}
	var dsWarning types.String
	requireNoDiags(t, "get data source postgres_warning", dsResp.State.GetAttribute(ctx, path.Root("postgres_warning"), &dsWarning))
	if !dsWarning.IsNull() {
		t.Errorf("data source postgres_warning = %v, want null", dsWarning)
	}
}
