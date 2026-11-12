package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
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

// planPostgresVersion runs the postgres_version plan modifiers the way
// Terraform does for an update of an existing project: prior state from the
// mock control plane, config and proposed plan from the given overrides.
func planPostgresVersion(t *testing.T, s rschema.Schema, state tfsdk.State, overrides map[string]tftypes.Value) planmodifier.StringResponse {
	t.Helper()
	ctx := context.Background()
	var stateValue types.String
	requireNoDiags(t, "state postgres_version", state.GetAttribute(ctx, path.Root("postgres_version"), &stateValue))

	attrs := map[string]tftypes.Value{}
	for _, name := range []string{"id", "name", "region", "environment"} {
		attrs[name] = str(stateString(t, state, name))
	}
	for name, value := range overrides {
		attrs[name] = value
	}
	raw := objectValue(t, s.Type().TerraformType(ctx), attrs)
	plan := tfsdk.Plan{Schema: s, Raw: raw}
	config := tfsdk.Config{Schema: s, Raw: raw}
	var configValue types.String
	requireNoDiags(t, "config postgres_version", config.GetAttribute(ctx, path.Root("postgres_version"), &configValue))

	attribute, ok := s.Attributes["postgres_version"].(rschema.StringAttribute)
	if !ok {
		t.Fatal("postgres_version is not a string attribute")
	}
	// Terraform proposes unknown for an optional+computed attribute the config omits.
	proposed := configValue
	if configValue.IsNull() {
		proposed = types.StringUnknown()
	}
	resp := planmodifier.StringResponse{PlanValue: proposed}
	for _, modifier := range attribute.PlanModifiers {
		req := planmodifier.StringRequest{
			Path:        path.Root("postgres_version"),
			Config:      config,
			ConfigValue: configValue,
			Plan:        plan,
			PlanValue:   resp.PlanValue,
			State:       state,
			StateValue:  stateValue,
		}
		modifier.PlanModifyString(ctx, req, &resp)
		if resp.Diagnostics.HasError() {
			break
		}
	}
	return resp
}

// A version mismatch on an existing project is never a replacement (FG-3): a
// config ahead of the database asks for `capydb upgrade major`, a config behind
// it (the database was upgraded outside Terraform) asks to align the config.
func TestProjectPostgresVersionMismatchErrorsInsteadOfReplacing(t *testing.T) {
	ctx := context.Background()
	client, _ := newTestClient(t)
	r := NewProjectResource()
	configureResource(t, r, client)
	s := resourceSchema(t, r)

	planRaw := objectValue(t, s.Type().TerraformType(ctx), map[string]tftypes.Value{
		"name":             str("versioned"),
		"environment":      str("non_production"),
		"postgres_version": str("17"),
	})
	created := resource.CreateResponse{State: emptyResourceState(t, s)}
	r.Create(ctx, resource.CreateRequest{
		Plan:   tfsdk.Plan{Schema: s, Raw: planRaw},
		Config: tfsdk.Config{Schema: s, Raw: planRaw},
	}, &created)
	requireNoDiags(t, "create", created.Diagnostics)

	cases := []struct {
		name      string
		overrides map[string]tftypes.Value
		wantError string
		wantPlan  string
	}{
		{"config ahead", map[string]tftypes.Value{"postgres_version": str("18")}, "capydb upgrade major", ""},
		{"config behind", map[string]tftypes.Value{"postgres_version": str("16")}, `"17"`, ""},
		{"config matches", map[string]tftypes.Value{"postgres_version": str("17")}, "", "17"},
		{"config omitted", map[string]tftypes.Value{"postgres_version": tftypes.NewValue(tftypes.String, nil)}, "", "17"},
		// A rename replaces the project anyway, so the new database may take a new major.
		{"replaced by rename", map[string]tftypes.Value{"name": str("renamed"), "postgres_version": str("18")}, "", "18"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := planPostgresVersion(t, s, created.State, tc.overrides)
			if resp.RequiresReplace {
				t.Fatal("a postgres_version difference must never plan a replacement")
			}
			if tc.wantError != "" {
				if !resp.Diagnostics.HasError() {
					t.Fatalf("want a plan error mentioning %s", tc.wantError)
				}
				if detail := resp.Diagnostics.Errors()[0].Detail(); !strings.Contains(detail, tc.wantError) {
					t.Fatalf("error detail %q does not mention %s", detail, tc.wantError)
				}
				return
			}
			requireNoDiags(t, "plan", resp.Diagnostics)
			if got := resp.PlanValue.ValueString(); got != tc.wantPlan {
				t.Errorf("planned postgres_version = %q, want %q", got, tc.wantPlan)
			}
		})
	}
}
