package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/capy-base/terraform-provider-capydb/internal/capydb"
)

// --- framework plumbing helpers -------------------------------------------

// resourceSchema returns the schema a resource exposes.
func resourceSchema(t *testing.T, r resource.Resource) rschema.Schema {
	t.Helper()
	var resp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	return resp.Schema
}

// dataSourceSchema returns the schema a data source exposes.
func dataSourceSchema(t *testing.T, d datasource.DataSource) dschema.Schema {
	t.Helper()
	var resp datasource.SchemaResponse
	d.Schema(context.Background(), datasource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	return resp.Schema
}

// objectValue builds a tftypes object for the given schema type, defaulting
// every attribute not present in attrs to null.
func objectValue(t *testing.T, schemaType tftypes.Type, attrs map[string]tftypes.Value) tftypes.Value {
	t.Helper()
	objectType, ok := schemaType.(tftypes.Object)
	if !ok {
		t.Fatalf("schema type %v is not an object", schemaType)
	}
	values := map[string]tftypes.Value{}
	for name, attrType := range objectType.AttributeTypes {
		if value, present := attrs[name]; present {
			values[name] = value
		} else {
			values[name] = tftypes.NewValue(attrType, nil)
		}
	}
	return tftypes.NewValue(objectType, values)
}

func str(value string) tftypes.Value { return tftypes.NewValue(tftypes.String, value) }
func num(value int64) tftypes.Value  { return tftypes.NewValue(tftypes.Number, value) }
func boolV(value bool) tftypes.Value { return tftypes.NewValue(tftypes.Bool, value) }
func strList(values ...string) tftypes.Value {
	elements := make([]tftypes.Value, 0, len(values))
	for _, value := range values {
		elements = append(elements, str(value))
	}
	return tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, elements)
}

func emptyResourceState(t *testing.T, s rschema.Schema) tfsdk.State {
	t.Helper()
	return tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(context.Background()), nil)}
}

func configureResource(t *testing.T, r resource.Resource, client *capydb.Client) {
	t.Helper()
	configurable, ok := r.(resource.ResourceWithConfigure)
	if !ok {
		t.Fatal("resource does not implement Configure")
	}
	var resp resource.ConfigureResponse
	configurable.Configure(context.Background(), resource.ConfigureRequest{ProviderData: client}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("configure diagnostics: %v", resp.Diagnostics)
	}
}

func configureDataSource(t *testing.T, d datasource.DataSource, client *capydb.Client) {
	t.Helper()
	configurable, ok := d.(datasource.DataSourceWithConfigure)
	if !ok {
		t.Fatal("data source does not implement Configure")
	}
	var resp datasource.ConfigureResponse
	configurable.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: client}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("configure diagnostics: %v", resp.Diagnostics)
	}
}

func requireNoDiags(t *testing.T, stage string, diags diag.Diagnostics) {
	t.Helper()
	if diags.HasError() {
		t.Fatalf("%s diagnostics: %v", stage, diags)
	}
}

func stateString(t *testing.T, state tfsdk.State, name string) string {
	t.Helper()
	var value *string
	diags := state.GetAttribute(context.Background(), path.Root(name), &value)
	requireNoDiags(t, "get "+name, diags)
	if value == nil {
		return ""
	}
	return *value
}

func stateBool(t *testing.T, state tfsdk.State, name string) bool {
	t.Helper()
	var value *bool
	diags := state.GetAttribute(context.Background(), path.Root(name), &value)
	requireNoDiags(t, "get "+name, diags)
	return value != nil && *value
}

func stateInt(t *testing.T, state tfsdk.State, name string) int64 {
	t.Helper()
	var value *int64
	diags := state.GetAttribute(context.Background(), path.Root(name), &value)
	requireNoDiags(t, "get "+name, diags)
	if value == nil {
		return 0
	}
	return *value
}

// --- capydb_project --------------------------------------------------------

func TestProjectResourceCRUD(t *testing.T) {
	ctx := context.Background()
	client, mock := newTestClient(t)

	r := NewProjectResource()
	configureResource(t, r, client)
	s := resourceSchema(t, r)
	schemaType := s.Type().TerraformType(ctx)

	// Create.
	planRaw := objectValue(t, schemaType, map[string]tftypes.Value{
		"name":        str("my app"),
		"environment": str("non_production"),
	})
	createResp := resource.CreateResponse{State: emptyResourceState(t, s)}
	r.Create(ctx, resource.CreateRequest{
		Plan:   tfsdk.Plan{Schema: s, Raw: planRaw},
		Config: tfsdk.Config{Schema: s, Raw: planRaw},
	}, &createResp)
	requireNoDiags(t, "create", createResp.Diagnostics)

	projectID := stateString(t, createResp.State, "id")
	if projectID == "" {
		t.Fatal("project id not set after create")
	}
	if got := stateString(t, createResp.State, "environment"); got != "non_production" {
		t.Errorf("environment = %q", got)
	}
	if got := stateString(t, createResp.State, "plan"); got != "launch" {
		t.Errorf("plan = %q (must be server-derived)", got)
	}
	if got := stateString(t, createResp.State, "slug"); got != "my-app" {
		t.Errorf("slug = %q", got)
	}
	mock.mu.Lock()
	provisionPolls := 0
	for jobID, jobType := range mock.jobStates {
		if jobType == "instance.create" {
			provisionPolls += mock.jobPolls[jobID]
		}
	}
	mock.mu.Unlock()
	if provisionPolls < 2 {
		t.Errorf("provision job polled %d times, want >= 2 (job wait not exercised)", provisionPolls)
	}

	// Read.
	readResp := resource.ReadResponse{State: createResp.State}
	r.Read(ctx, resource.ReadRequest{State: createResp.State}, &readResp)
	requireNoDiags(t, "read", readResp.Diagnostics)
	if got := stateString(t, readResp.State, "name"); got != "my app" {
		t.Errorf("name = %q", got)
	}

	// Update (environment is the only in-place updatable attribute).
	updatePlanRaw := objectValue(t, schemaType, map[string]tftypes.Value{
		"id":          str(projectID),
		"name":        str("my app"),
		"environment": str("production"),
	})
	updateResp := resource.UpdateResponse{State: readResp.State}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: s, Raw: updatePlanRaw},
		State: readResp.State,
	}, &updateResp)
	requireNoDiags(t, "update", updateResp.Diagnostics)
	if got := stateString(t, updateResp.State, "environment"); got != "production" {
		t.Errorf("environment after update = %q", got)
	}
	mock.mu.Lock()
	patched := append([]string(nil), mock.patchedEnvs...)
	mock.mu.Unlock()
	if len(patched) != 1 || patched[0] != "production" {
		t.Errorf("PATCH environment calls = %v", patched)
	}

	// Delete.
	deleteResp := resource.DeleteResponse{State: updateResp.State}
	r.Delete(ctx, resource.DeleteRequest{State: updateResp.State}, &deleteResp)
	requireNoDiags(t, "delete", deleteResp.Diagnostics)
	mock.mu.Lock()
	remaining := len(mock.projects)
	mock.mu.Unlock()
	if remaining != 0 {
		t.Errorf("projects remaining after delete = %d", remaining)
	}

	// Read after deletion removes the resource from state.
	goneResp := resource.ReadResponse{State: updateResp.State}
	r.Read(ctx, resource.ReadRequest{State: updateResp.State}, &goneResp)
	requireNoDiags(t, "read-after-delete", goneResp.Diagnostics)
	if !goneResp.State.Raw.IsNull() {
		t.Error("state should be removed after the project disappears")
	}
}

// --- capydb_preview_database ------------------------------------------------

func TestPreviewDatabaseResourceCRUD(t *testing.T) {
	ctx := context.Background()
	client, mock := newTestClient(t)

	// Seed a parent project directly through the client.
	project, _, err := client.CreateProject(ctx, capydb.CreateProjectRequest{Name: "parent"})
	if err != nil {
		t.Fatalf("seed project: %v", err)
	}

	r := NewPreviewDatabaseResource()
	configureResource(t, r, client)
	s := resourceSchema(t, r)
	schemaType := s.Type().TerraformType(ctx)

	// Create.
	planRaw := objectValue(t, schemaType, map[string]tftypes.Value{
		"project_id": str(project.ID),
		"name":       str("pr-42"),
		"mode":       str("clone"),
		"ttl_hours":  num(24),
	})
	createResp := resource.CreateResponse{State: emptyResourceState(t, s)}
	r.Create(ctx, resource.CreateRequest{
		Plan:   tfsdk.Plan{Schema: s, Raw: planRaw},
		Config: tfsdk.Config{Schema: s, Raw: planRaw},
	}, &createResp)
	requireNoDiags(t, "create", createResp.Diagnostics)

	previewID := stateString(t, createResp.State, "id")
	if previewID == "" {
		t.Fatal("preview id not set after create")
	}
	if got := stateString(t, createResp.State, "mode"); got != "clone" {
		t.Errorf("mode = %q", got)
	}
	if got := stateString(t, createResp.State, "ttl_expires_at"); got != "2026-06-11T00:00:00Z" {
		t.Errorf("ttl_expires_at = %q", got)
	}

	// Read.
	readResp := resource.ReadResponse{State: createResp.State}
	r.Read(ctx, resource.ReadRequest{State: createResp.State}, &readResp)
	requireNoDiags(t, "read", readResp.Diagnostics)
	if got := stateInt(t, readResp.State, "ttl_hours"); got != 24 {
		t.Errorf("ttl_hours = %d", got)
	}

	// Update: raising ttl_hours 24 -> 48 must call the extend endpoint with
	// the full new TTL (the API treats ttl_hours as absolute, expiry = now + N).
	updatePlanRaw := objectValue(t, schemaType, map[string]tftypes.Value{
		"id":         str(previewID),
		"project_id": str(project.ID),
		"name":       str("pr-42"),
		"mode":       str("clone"),
		"ttl_hours":  num(48),
	})
	updateResp := resource.UpdateResponse{State: readResp.State}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: s, Raw: updatePlanRaw},
		State: readResp.State,
	}, &updateResp)
	requireNoDiags(t, "update", updateResp.Diagnostics)

	mock.mu.Lock()
	extendCalls := append([]int64(nil), mock.extendCalls...)
	mock.mu.Unlock()
	if len(extendCalls) != 1 || extendCalls[0] != 48 {
		t.Errorf("extend calls = %v, want one call with the full ttl 48", extendCalls)
	}
	if got := stateString(t, updateResp.State, "ttl_expires_at"); got != "2026-06-12T00:00:00Z" {
		t.Errorf("ttl_expires_at after extend = %q", got)
	}
	if got := stateInt(t, updateResp.State, "ttl_hours"); got != 48 {
		t.Errorf("ttl_hours after extend = %d", got)
	}

	// Delete.
	deleteResp := resource.DeleteResponse{State: updateResp.State}
	r.Delete(ctx, resource.DeleteRequest{State: updateResp.State}, &deleteResp)
	requireNoDiags(t, "delete", deleteResp.Diagnostics)
	mock.mu.Lock()
	remaining := len(mock.previews)
	mock.mu.Unlock()
	if remaining != 0 {
		t.Errorf("previews remaining after delete = %d", remaining)
	}
}

// --- capydb_api_key ----------------------------------------------------------

func TestAPIKeyResourceCRUD(t *testing.T) {
	ctx := context.Background()
	client, mock := newTestClient(t)

	r := NewAPIKeyResource()
	configureResource(t, r, client)
	s := resourceSchema(t, r)
	schemaType := s.Type().TerraformType(ctx)

	// Create without organization_id: it must be resolved from /v1/me.
	planRaw := objectValue(t, schemaType, map[string]tftypes.Value{
		"name":   str("ci key"),
		"scopes": strList("projects:read", "credentials:read"),
	})
	createResp := resource.CreateResponse{State: emptyResourceState(t, s)}
	r.Create(ctx, resource.CreateRequest{
		Plan:   tfsdk.Plan{Schema: s, Raw: planRaw},
		Config: tfsdk.Config{Schema: s, Raw: planRaw},
	}, &createResp)
	requireNoDiags(t, "create", createResp.Diagnostics)

	keyID := stateString(t, createResp.State, "id")
	if keyID == "" {
		t.Fatal("key id not set after create")
	}
	if got := stateString(t, createResp.State, "organization_id"); got != "org_1" {
		t.Errorf("organization_id = %q, want resolved org_1", got)
	}
	token := stateString(t, createResp.State, "token")
	if token != "capy_live_secret_"+keyID {
		t.Errorf("token = %q (plaintext key must be captured at create)", token)
	}
	if got := stateString(t, createResp.State, "key_prefix"); got != "capy_live_ab12" {
		t.Errorf("key_prefix = %q", got)
	}

	// Read keeps the token from prior state (the API never re-returns it).
	readResp := resource.ReadResponse{State: createResp.State}
	r.Read(ctx, resource.ReadRequest{State: createResp.State}, &readResp)
	requireNoDiags(t, "read", readResp.Diagnostics)
	if got := stateString(t, readResp.State, "token"); got != token {
		t.Errorf("token after read = %q, want preserved %q", got, token)
	}
	if !stateBool(t, readResp.State, "is_active") {
		t.Error("is_active should be true")
	}

	// Delete revokes the key.
	deleteResp := resource.DeleteResponse{State: readResp.State}
	r.Delete(ctx, resource.DeleteRequest{State: readResp.State}, &deleteResp)
	requireNoDiags(t, "delete", deleteResp.Diagnostics)
	mock.mu.Lock()
	revoked := !mock.apiKeys[keyID].IsActive
	mock.mu.Unlock()
	if !revoked {
		t.Error("key should be revoked after delete")
	}

	// Read after revocation drops the resource from state.
	goneResp := resource.ReadResponse{State: readResp.State}
	r.Read(ctx, resource.ReadRequest{State: readResp.State}, &goneResp)
	requireNoDiags(t, "read-after-revoke", goneResp.Diagnostics)
	if !goneResp.State.Raw.IsNull() {
		t.Error("state should be removed for a revoked key")
	}
}

// --- capydb_webhook_endpoint -------------------------------------------------

func TestWebhookEndpointResourceCRUD(t *testing.T) {
	ctx := context.Background()
	client, mock := newTestClient(t)

	r := NewWebhookEndpointResource()
	configureResource(t, r, client)
	s := resourceSchema(t, r)
	schemaType := s.Type().TerraformType(ctx)

	// Create. Defaults (active=true, secret_version=1) are set explicitly as
	// the framework's default machinery would during planning.
	planRaw := objectValue(t, schemaType, map[string]tftypes.Value{
		"url":            str("https://hooks.example.com/capydb"),
		"event_types":    strList("instance.createed", "backup.completed"),
		"active":         boolV(true),
		"secret_version": num(1),
	})
	createResp := resource.CreateResponse{State: emptyResourceState(t, s)}
	r.Create(ctx, resource.CreateRequest{
		Plan:   tfsdk.Plan{Schema: s, Raw: planRaw},
		Config: tfsdk.Config{Schema: s, Raw: planRaw},
	}, &createResp)
	requireNoDiags(t, "create", createResp.Diagnostics)

	endpointID := stateString(t, createResp.State, "id")
	if endpointID == "" {
		t.Fatal("endpoint id not set after create")
	}
	secret := stateString(t, createResp.State, "signing_secret")
	if secret != "whsec_initial_"+endpointID {
		t.Errorf("signing_secret = %q", secret)
	}
	if got := stateString(t, createResp.State, "organization_id"); got != "org_1" {
		t.Errorf("organization_id = %q", got)
	}

	// Read keeps the signing secret from prior state.
	readResp := resource.ReadResponse{State: createResp.State}
	r.Read(ctx, resource.ReadRequest{State: createResp.State}, &readResp)
	requireNoDiags(t, "read", readResp.Diagnostics)
	if got := stateString(t, readResp.State, "signing_secret"); got != secret {
		t.Errorf("signing_secret after read = %q, want preserved", got)
	}

	// Update the URL and deactivate without rotating: secret stays.
	updatePlanRaw := objectValue(t, schemaType, map[string]tftypes.Value{
		"id":              str(endpointID),
		"organization_id": str("org_1"),
		"url":             str("https://hooks.example.com/v2"),
		"event_types":     strList("instance.createed", "backup.completed"),
		"active":          boolV(false),
		"secret_version":  num(1),
		"signing_secret":  str(secret),
	})
	updateResp := resource.UpdateResponse{State: readResp.State}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: s, Raw: updatePlanRaw},
		State: readResp.State,
	}, &updateResp)
	requireNoDiags(t, "update", updateResp.Diagnostics)
	if got := stateString(t, updateResp.State, "url"); got != "https://hooks.example.com/v2" {
		t.Errorf("url after update = %q", got)
	}
	if stateBool(t, updateResp.State, "active") {
		t.Error("active should be false after update")
	}
	if got := stateString(t, updateResp.State, "signing_secret"); got != secret {
		t.Errorf("signing_secret changed without rotation: %q", got)
	}
	mock.mu.Lock()
	rotations := mock.secretRotations
	mock.mu.Unlock()
	if rotations != 0 {
		t.Errorf("rotations = %d, want 0", rotations)
	}

	// Bump secret_version: rotates and stores the new secret.
	rotatePlanRaw := objectValue(t, schemaType, map[string]tftypes.Value{
		"id":              str(endpointID),
		"organization_id": str("org_1"),
		"url":             str("https://hooks.example.com/v2"),
		"event_types":     strList("instance.createed", "backup.completed"),
		"active":          boolV(false),
		"secret_version":  num(2),
	})
	rotateResp := resource.UpdateResponse{State: updateResp.State}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: s, Raw: rotatePlanRaw},
		State: updateResp.State,
	}, &rotateResp)
	requireNoDiags(t, "rotate", rotateResp.Diagnostics)
	if got := stateString(t, rotateResp.State, "signing_secret"); got != "whsec_rotated_1" {
		t.Errorf("signing_secret after rotation = %q", got)
	}
	if got := stateInt(t, rotateResp.State, "secret_version"); got != 2 {
		t.Errorf("secret_version = %d", got)
	}
	mock.mu.Lock()
	rotations = mock.secretRotations
	mock.mu.Unlock()
	if rotations != 1 {
		t.Errorf("rotations = %d, want 1", rotations)
	}

	// Delete.
	deleteResp := resource.DeleteResponse{State: rotateResp.State}
	r.Delete(ctx, resource.DeleteRequest{State: rotateResp.State}, &deleteResp)
	requireNoDiags(t, "delete", deleteResp.Diagnostics)
	mock.mu.Lock()
	remaining := len(mock.webhooks)
	mock.mu.Unlock()
	if remaining != 0 {
		t.Errorf("webhook endpoints remaining after delete = %d", remaining)
	}
}

// --- data sources ------------------------------------------------------------

func TestRegionsDataSource(t *testing.T) {
	ctx := context.Background()
	client, _ := newTestClient(t)

	d := NewRegionsDataSource()
	configureDataSource(t, d, client)
	s := dataSourceSchema(t, d)
	schemaType := s.Type().TerraformType(ctx)

	configRaw := objectValue(t, schemaType, nil)
	readResp := datasource.ReadResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(schemaType, nil)}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: s, Raw: configRaw}}, &readResp)
	requireNoDiags(t, "read", readResp.Diagnostics)

	var regions []regionModel
	diags := readResp.State.GetAttribute(ctx, path.Root("regions"), &regions)
	requireNoDiags(t, "get regions", diags)
	if len(regions) != 1 || regions[0].Slug.ValueString() != "us-east" {
		t.Errorf("regions = %v, want one region with slug us-east", regions)
	}
}

func TestProjectDataSourceBySlugAndID(t *testing.T) {
	ctx := context.Background()
	client, _ := newTestClient(t)

	project, _, err := client.CreateProject(ctx, capydb.CreateProjectRequest{Name: "lookup me"})
	if err != nil {
		t.Fatalf("seed project: %v", err)
	}

	d := NewProjectDataSource()
	configureDataSource(t, d, client)
	s := dataSourceSchema(t, d)
	schemaType := s.Type().TerraformType(ctx)

	// By slug.
	configRaw := objectValue(t, schemaType, map[string]tftypes.Value{"slug": str("lookup-me")})
	readResp := datasource.ReadResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(schemaType, nil)}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: s, Raw: configRaw}}, &readResp)
	requireNoDiags(t, "read by slug", readResp.Diagnostics)
	if got := stateString(t, readResp.State, "id"); got != project.ID {
		t.Errorf("id = %q, want %q", got, project.ID)
	}

	// By id.
	configRaw = objectValue(t, schemaType, map[string]tftypes.Value{"id": str(project.ID)})
	readResp = datasource.ReadResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(schemaType, nil)}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: s, Raw: configRaw}}, &readResp)
	requireNoDiags(t, "read by id", readResp.Diagnostics)
	if got := stateString(t, readResp.State, "slug"); got != "lookup-me" {
		t.Errorf("slug = %q", got)
	}

	// Unknown slug surfaces a clear error.
	configRaw = objectValue(t, schemaType, map[string]tftypes.Value{"slug": str("missing")})
	readResp = datasource.ReadResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(schemaType, nil)}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: s, Raw: configRaw}}, &readResp)
	if !readResp.Diagnostics.HasError() {
		t.Error("expected an error for an unknown slug")
	}
}

func TestProjectConnectionDataSource(t *testing.T) {
	ctx := context.Background()
	client, _ := newTestClient(t)

	project, _, err := client.CreateProject(ctx, capydb.CreateProjectRequest{Name: "conn"})
	if err != nil {
		t.Fatalf("seed project: %v", err)
	}

	d := NewProjectConnectionDataSource()
	configureDataSource(t, d, client)
	s := dataSourceSchema(t, d)
	schemaType := s.Type().TerraformType(ctx)

	configRaw := objectValue(t, schemaType, map[string]tftypes.Value{"project_id": str(project.ID)})
	readResp := datasource.ReadResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(schemaType, nil)}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: s, Raw: configRaw}}, &readResp)
	requireNoDiags(t, "read", readResp.Diagnostics)

	if got := stateString(t, readResp.State, "pooled_url"); got != "postgres://user:pass@host:6432/db_conn" {
		t.Errorf("pooled_url = %q", got)
	}
	if got := stateString(t, readResp.State, "direct_url"); got != "postgres://user:pass@host:5432/db_conn" {
		t.Errorf("direct_url = %q", got)
	}
	if got := stateString(t, readResp.State, "username"); got != "user_"+project.ID {
		t.Errorf("username = %q", got)
	}
}

func TestOrganizationDataSource(t *testing.T) {
	ctx := context.Background()
	client, _ := newTestClient(t)

	d := NewOrganizationDataSource()
	configureDataSource(t, d, client)
	s := dataSourceSchema(t, d)
	schemaType := s.Type().TerraformType(ctx)

	configRaw := objectValue(t, schemaType, nil)
	readResp := datasource.ReadResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(schemaType, nil)}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: s, Raw: configRaw}}, &readResp)
	requireNoDiags(t, "read", readResp.Diagnostics)

	if got := stateString(t, readResp.State, "id"); got != "org_1" {
		t.Errorf("id = %q", got)
	}
	if got := stateString(t, readResp.State, "billing_plan"); got != "launch" {
		t.Errorf("billing_plan = %q", got)
	}
}

// --- provider wiring -----------------------------------------------------------

func TestProviderMetadataAndSchema(t *testing.T) {
	ctx := context.Background()
	p := New("test")()

	var metaResp provider.MetadataResponse
	p.Metadata(ctx, provider.MetadataRequest{}, &metaResp)
	if metaResp.TypeName != "capydb" {
		t.Errorf("type name = %q", metaResp.TypeName)
	}

	var schemaResp provider.SchemaResponse
	p.Schema(ctx, provider.SchemaRequest{}, &schemaResp)
	requireNoDiags(t, "provider schema", schemaResp.Diagnostics)
	if _, ok := schemaResp.Schema.Attributes["api_key"]; !ok {
		t.Error("api_key attribute missing")
	}
	if len(p.Resources(ctx)) != 4 {
		t.Errorf("resources = %d, want 4", len(p.Resources(ctx)))
	}
	if len(p.DataSources(ctx)) != 4 {
		t.Errorf("data sources = %d, want 4", len(p.DataSources(ctx)))
	}
}
