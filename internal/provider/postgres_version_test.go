package provider

import (
	"context"
	"testing"

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
}
