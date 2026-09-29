package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// A configuration still naming the deprecated hel1 alias must keep it in state:
// the API reports eu-north-1, and writing that back would make the create result
// differ from the plan and plan a replacement on every later run.
func TestProjectResourceKeepsDeprecatedRegionAlias(t *testing.T) {
	ctx := context.Background()
	client, _ := newTestClient(t)

	r := NewProjectResource()
	configureResource(t, r, client)
	s := resourceSchema(t, r)
	schemaType := s.Type().TerraformType(ctx)

	planRaw := objectValue(t, schemaType, map[string]tftypes.Value{
		"name":        str("aliased"),
		"environment": str("non_production"),
		"region":      str("hel1"),
	})
	createResp := resource.CreateResponse{State: emptyResourceState(t, s)}
	r.Create(ctx, resource.CreateRequest{
		Plan:   tfsdk.Plan{Schema: s, Raw: planRaw},
		Config: tfsdk.Config{Schema: s, Raw: planRaw},
	}, &createResp)
	requireNoDiags(t, "create", createResp.Diagnostics)
	if got := stateString(t, createResp.State, "region"); got != "hel1" {
		t.Errorf("region after create = %q, want the configured hel1", got)
	}

	readResp := resource.ReadResponse{State: createResp.State}
	r.Read(ctx, resource.ReadRequest{State: createResp.State}, &readResp)
	requireNoDiags(t, "read", readResp.Diagnostics)
	if got := stateString(t, readResp.State, "region"); got != "hel1" {
		t.Errorf("region after read = %q, want the configured hel1", got)
	}
}

func TestRegionChangeRequiresReplace(t *testing.T) {
	cases := []struct {
		state, planned string
		want           bool
	}{
		{"hel1", "eu-north-1", false},
		{"eu-north-1", "hel1", false},
		{"HEL1", "eu-north-1", false},
		{"eu-north-1", "us-east-1", true},
		{"hel1", "us-east-1", true},
	}
	for _, tc := range cases {
		req := planmodifier.StringRequest{
			StateValue: types.StringValue(tc.state),
			PlanValue:  types.StringValue(tc.planned),
		}
		var resp stringplanmodifier.RequiresReplaceIfFuncResponse
		regionChangeRequiresReplace(context.Background(), req, &resp)
		if resp.RequiresReplace != tc.want {
			t.Errorf("%s -> %s: RequiresReplace = %v, want %v", tc.state, tc.planned, resp.RequiresReplace, tc.want)
		}
	}
}
