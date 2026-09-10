package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/capydatabase/terraform-provider-capydb/internal/capydb"
)

var (
	_ resource.Resource                = (*kvStoreResource)(nil)
	_ resource.ResourceWithConfigure   = (*kvStoreResource)(nil)
	_ resource.ResourceWithImportState = (*kvStoreResource)(nil)
)

// NewKVStoreResource creates the capydb_kv_store resource.
func NewKVStoreResource() resource.Resource {
	return &kvStoreResource{}
}

type kvStoreResource struct {
	client *capydb.Client
}

type kvStoreModel struct {
	ID              types.String `tfsdk:"id"`
	ProjectID       types.String `tfsdk:"project_id"`
	State           types.String `tfsdk:"state"`
	MaxMemoryMB     types.Int64  `tfsdk:"maxmemory_mb"`
	MaxMemoryPolicy types.String `tfsdk:"maxmemory_policy"`
	Persistence     types.String `tfsdk:"persistence"`
	RestURL         types.String `tfsdk:"rest_url"`
	RestToken       types.String `tfsdk:"rest_token"`
	RedisURL        types.String `tfsdk:"redis_url"`
}

func (r *kvStoreResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_kv_store"
}

func (r *kvStoreResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A CapyDB K/V store (CapyDB Knight/Valkyrie): a managed key-value and rate-limiting " +
			"service running in its own KV cell, beside the project's database rather than inside it. A project " +
			"has at most one, and may have one with or without ever using its database.\n\n" +
			"The store is sized from the organization's plan; there is no size attribute to set. The plaintext " +
			"token is returned by the API exactly once at creation and is stored in state as the sensitive " +
			"`rest_token` - the API never returns it again, so it is empty after an import and drift on the " +
			"secret itself cannot be detected.\n\n" +
			"A K/V store keeps only a periodic snapshot: no backups, no point-in-time recovery, and keys with a " +
			"TTL are evicted once it is full. Destroying this resource deletes the store and its data.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "K/V store id.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"project_id": schema.StringAttribute{
				Required:    true,
				Description: "Project the store belongs to. Changing it forces a replacement.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"state": schema.StringAttribute{
				Computed: true,
				Description: "Lifecycle state: `provisioning`, `running`, `error` or `destroying`. Creation is " +
					"asynchronous, so a freshly created store reads as `provisioning`.",
			},
			"maxmemory_mb": schema.Int64Attribute{
				Computed: true,
				Description: "Storable capacity in MB, derived from the organization's plan. This is what the " +
					"store will hold - the KV cell's own memory ceiling is larger so a snapshot fork has " +
					"headroom, and that difference is not usable capacity.",
			},
			"maxmemory_policy": schema.StringAttribute{
				Computed: true,
				Description: "Eviction policy, managed by CapyDB. `volatile-lru`: only keys carrying a TTL are " +
					"evictable, so a store filled with non-expiring keys rejects writes rather than discarding data.",
			},
			"persistence": schema.StringAttribute{
				Computed:    true,
				Description: "Durability mode, managed by CapyDB. `rdb` is a periodic snapshot; there are no backups.",
			},
			"rest_url": schema.StringAttribute{
				Computed:    true,
				Description: "Upstash-compatible REST endpoint. Set it as `CAPYDB_KV_REST_URL`.",
			},
			"rest_token": schema.StringAttribute{
				Computed:  true,
				Sensitive: true,
				Description: "The plaintext K/V token (format `capy_kv_...`). Returned exactly once at creation " +
					"and stored in state; set it as `CAPYDB_KV_REST_TOKEN`. Empty on an imported resource, because " +
					"the control plane keeps only its hash. Rotation is not a Terraform operation - use the " +
					"dashboard or `capydb kv rotate-token`, and take the new token from that output: a refresh " +
					"cannot recover it and deliberately preserves the value already in state, so a rotation " +
					"elsewhere leaves this attribute stale with no drift to detect.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"redis_url": schema.StringAttribute{
				Computed:  true,
				Sensitive: true,
				Description: "RESP endpoint for ordinary Redis(R) OSS clients. Carries the token as its password " +
					"at creation; after an import it has no password, because the token is not recoverable. Like " +
					"`rest_token` it is preserved across refreshes rather than rebuilt, so it goes stale with the " +
					"token when the token is rotated elsewhere.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *kvStoreResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = resourceClient(req, resp)
}

func (r *kvStoreResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan kvStoreModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	store, _, err := r.client.CreateKVStore(ctx, plan.ProjectID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error creating CapyDB K/V store", err.Error())
		return
	}

	applyKVStore(&plan, store)
	// The one response that carries the plaintext. Persist it now or it is gone.
	plan.RestToken = types.StringValue(firstNonEmptyString(store.Token, store.Credentials.RestToken))
	plan.RedisURL = types.StringValue(store.Credentials.RedisURL)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *kvStoreResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state kvStoreModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	store, err := r.client.GetKVStore(ctx, state.ProjectID.ValueString())
	if err != nil {
		if capydb.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading CapyDB K/V store", err.Error())
		return
	}

	applyKVStore(&state, store)

	// The endpoints come from the credentials call, which never returns the
	// secret. rest_token in state is left untouched: it cannot be refreshed, so
	// overwriting it here would destroy the only copy the practitioner has.
	credentials, err := r.client.GetKVCredentials(ctx, state.ProjectID.ValueString())
	if err == nil {
		state.RestURL = types.StringValue(credentials.RestURL)
		if state.RedisURL.IsNull() || state.RedisURL.IsUnknown() {
			state.RedisURL = types.StringValue(credentials.RedisURL)
		}
	} else if !capydb.IsNotFound(err) {
		resp.Diagnostics.AddError("Error reading CapyDB K/V credentials", err.Error())
		return
	}

	if state.RestToken.IsNull() || state.RestToken.IsUnknown() {
		state.RestToken = types.StringValue("")
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *kvStoreResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	// project_id is the only configurable attribute and it carries
	// RequiresReplace, so the framework never plans an in-place update.
	resp.Diagnostics.AddError(
		"CapyDB K/V stores cannot be updated in place",
		"The only configurable attribute of capydb_kv_store forces a replacement; this update path should be "+
			"unreachable. Please report this as a provider bug.",
	)
}

func (r *kvStoreResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state kvStoreModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if _, err := r.client.DeleteKVStore(ctx, state.ProjectID.ValueString()); err != nil && !capydb.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting CapyDB K/V store", err.Error())
	}
}

// ImportState takes the PROJECT id, not the store id: a project has at most one
// store and every K/V endpoint is addressed by project, so the project id is the
// only identifier that can be used to fetch one.
func (r *kvStoreResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("project_id"), req, resp)
}

func applyKVStore(model *kvStoreModel, store capydb.KVStore) {
	model.ID = types.StringValue(store.ID)
	model.ProjectID = types.StringValue(store.ProjectID)
	model.State = types.StringValue(store.State)
	model.MaxMemoryMB = types.Int64Value(int64(store.MaxMemoryMB))
	model.MaxMemoryPolicy = types.StringValue(store.MaxMemoryPolicy)
	model.Persistence = types.StringValue(store.Persistence)
	if store.Credentials.RestURL != "" {
		model.RestURL = types.StringValue(store.Credentials.RestURL)
	}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
