package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// The control plane no longer accepts the retired hel1 name, so a config that
// still says it must fail at plan time and name the id to write instead of
// reaching the API.
func TestProjectRegionRefusesRetiredName(t *testing.T) {
	s := resourceSchema(t, NewProjectResource())
	attribute, ok := s.Attributes["region"].(rschema.StringAttribute)
	if !ok {
		t.Fatal("region is not a string attribute")
	}
	validate := func(value string) validator.StringResponse {
		var resp validator.StringResponse
		for _, v := range attribute.Validators {
			v.ValidateString(context.Background(), validator.StringRequest{
				Path:        path.Root("region"),
				ConfigValue: types.StringValue(value),
			}, &resp)
		}
		return resp
	}
	for _, retired := range []string{"hel1", "HEL1"} {
		resp := validate(retired)
		if !resp.Diagnostics.HasError() {
			t.Fatalf("region %q must be refused at plan time", retired)
		}
		if detail := resp.Diagnostics.Errors()[0].Detail(); !strings.Contains(detail, `"eu-north-1"`) {
			t.Errorf("error for %q does not name eu-north-1: %s", retired, detail)
		}
	}
	for _, valid := range []string{"eu-north-1", "us-east-1"} {
		if resp := validate(valid); resp.Diagnostics.HasError() {
			t.Errorf("region %q refused: %v", valid, resp.Diagnostics)
		}
	}
}

// State written while the alias was accepted can still hold hel1. Refresh
// replaces it with the id the API reports, and a plan made without refresh
// must not replace the database over the old name.
func TestProjectStateHoldingRetiredRegionNameIsNotReplaced(t *testing.T) {
	ctx := context.Background()
	client, _ := newTestClient(t)
	r := NewProjectResource()
	configureResource(t, r, client)
	s := resourceSchema(t, r)

	planRaw := objectValue(t, s.Type().TerraformType(ctx), map[string]tftypes.Value{
		"name":        str("legacy"),
		"environment": str("non_production"),
		"region":      str("eu-north-1"),
	})
	created := resource.CreateResponse{State: emptyResourceState(t, s)}
	r.Create(ctx, resource.CreateRequest{
		Plan:   tfsdk.Plan{Schema: s, Raw: planRaw},
		Config: tfsdk.Config{Schema: s, Raw: planRaw},
	}, &created)
	requireNoDiags(t, "create", created.Diagnostics)
	legacy := created.State
	requireNoDiags(t, "set legacy region", legacy.SetAttribute(ctx, path.Root("region"), "hel1"))

	readResp := resource.ReadResponse{State: legacy}
	r.Read(ctx, resource.ReadRequest{State: legacy}, &readResp)
	requireNoDiags(t, "read", readResp.Diagnostics)
	if got := stateString(t, readResp.State, "region"); got != "eu-north-1" {
		t.Errorf("region after refresh = %q, want eu-north-1", got)
	}

	cases := []struct {
		state, planned string
		want           bool
	}{
		{"hel1", "eu-north-1", false},
		{"HEL1", "eu-north-1", false},
		{"eu-north-1", "eu-north-1", false},
		{"eu-north-1", "us-east-1", true},
		{"hel1", "us-east-1", true},
	}
	for _, tc := range cases {
		req := planmodifier.StringRequest{
			StateValue: types.StringValue(tc.state),
			PlanValue:  types.StringValue(tc.planned),
		}
		var resp stringplanmodifier.RequiresReplaceIfFuncResponse
		regionChangeRequiresReplace(ctx, req, &resp)
		if resp.RequiresReplace != tc.want {
			t.Errorf("%s -> %s: RequiresReplace = %v, want %v", tc.state, tc.planned, resp.RequiresReplace, tc.want)
		}
	}
	// Region omitted from the config while another attribute changes: the plan
	// is unknown until UseStateForUnknown runs, which is not a region move.
	var unknownResp stringplanmodifier.RequiresReplaceIfFuncResponse
	regionChangeRequiresReplace(ctx, planmodifier.StringRequest{
		StateValue: types.StringValue("hel1"),
		PlanValue:  types.StringUnknown(),
	}, &unknownResp)
	if unknownResp.RequiresReplace {
		t.Error("an unknown planned region must not replace the database")
	}
}

// The mock mirrors the control plane, which refuses hel1 since the alias was
// removed.
func TestProjectCreateWithRetiredRegionNameIsRefused(t *testing.T) {
	ctx := context.Background()
	client, _ := newTestClient(t)
	r := NewProjectResource()
	configureResource(t, r, client)
	s := resourceSchema(t, r)
	planRaw := objectValue(t, s.Type().TerraformType(ctx), map[string]tftypes.Value{
		"name":        str("aliased"),
		"environment": str("non_production"),
		"region":      str("hel1"),
	})
	resp := resource.CreateResponse{State: emptyResourceState(t, s)}
	r.Create(ctx, resource.CreateRequest{
		Plan:   tfsdk.Plan{Schema: s, Raw: planRaw},
		Config: tfsdk.Config{Schema: s, Raw: planRaw},
	}, &resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("the control plane refuses hel1; create must fail")
	}
}
