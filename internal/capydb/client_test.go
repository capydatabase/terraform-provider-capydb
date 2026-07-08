package capydb

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func newTestClient(t *testing.T, handler http.Handler) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	client, err := NewClient(server.URL, "capy_live_test", "test",
		WithRetryBackoff([]time.Duration{0, time.Millisecond, time.Millisecond}),
		WithJobPollInterval(time.Millisecond),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return client
}

func TestNewClientDefaultsBaseURL(t *testing.T) {
	client, err := NewClient("", "key", "1.0.0")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if client.doer.BaseURL != DefaultBaseURL {
		t.Fatalf("baseURL = %q, want %q", client.doer.BaseURL, DefaultBaseURL)
	}
	if client.doer.UserAgent != "terraform-provider-capydb/1.0.0" {
		t.Fatalf("userAgent = %q", client.doer.UserAgent)
	}
}

func TestNewClientRejectsInvalidURL(t *testing.T) {
	if _, err := NewClient("not a url", "key", "test"); err == nil {
		t.Fatal("expected error for invalid url")
	}
}

func TestCreateProjectSendsAuthAndBody(t *testing.T) {
	var gotAuth, gotMethod, gotPath string
	var gotBody map[string]any

	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotMethod = r.Method
		gotPath = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"project":{"id":"prj_1","name":"app","slug":"app","state":"provisioning"},"job":{"id":"job_1","type":"instance.create","state":"pending"}}`))
	}))

	project, job, err := client.CreateProject(context.Background(), CreateProjectRequest{
		Name:        "app",
		Environment: "production",
	})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if gotAuth != "Bearer capy_live_test" {
		t.Errorf("Authorization = %q", gotAuth)
	}
	if gotMethod != http.MethodPost || gotPath != "/v1/projects" {
		t.Errorf("request = %s %s", gotMethod, gotPath)
	}
	if gotBody["name"] != "app" || gotBody["environment"] != "production" {
		t.Errorf("body = %v", gotBody)
	}
	if _, ok := gotBody["region"]; ok {
		t.Errorf("empty region should be omitted, body = %v", gotBody)
	}
	if project.ID != "prj_1" || job.ID != "job_1" {
		t.Errorf("project = %+v job = %+v", project, job)
	}
}

func TestGetRetriesOn5xx(t *testing.T) {
	var calls atomic.Int32
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 3 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(`{"project":{"id":"prj_1","name":"app","slug":"app","state":"ready"}}`))
	}))

	project, err := client.GetProject(context.Background(), "prj_1")
	if err != nil {
		t.Fatalf("GetProject after retries: %v", err)
	}
	if project.State != "ready" {
		t.Errorf("state = %q", project.State)
	}
	if got := calls.Load(); got != 3 {
		t.Errorf("calls = %d, want 3", got)
	}
}

func TestGetDoesNotRetryOn4xx(t *testing.T) {
	var calls atomic.Int32
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"project not found"}`))
	}))

	_, err := client.GetProject(context.Background(), "prj_missing")
	if !IsNotFound(err) {
		t.Fatalf("expected not-found APIError, got %v", err)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Message != "project not found" {
		t.Errorf("error = %v", err)
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("calls = %d, want 1 (no retry on 4xx)", got)
	}
}

func TestPostNotRetriedOn5xx(t *testing.T) {
	var calls atomic.Int32
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"boom"}`))
	}))

	_, _, err := client.CreateProject(context.Background(), CreateProjectRequest{Name: "app"})
	if err == nil {
		t.Fatal("expected error")
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("calls = %d, want 1 (non-GET must not retry)", got)
	}
}

func TestListEndpointsNormalizeNull(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/projects":
			_, _ = w.Write([]byte(`{"projects":null}`))
		case "/v1/projects/prj_1/preview-databases":
			_, _ = w.Write([]byte(`{"preview_databases":null}`))
		case "/v1/organizations/org_1/api-keys":
			_, _ = w.Write([]byte(`{"api_keys":[{"id":"key_1","name":"k","key_prefix":"capy_live_ab","scopes":null,"is_active":true}]}`))
		case "/v1/organizations/org_1/webhook-endpoints":
			_, _ = w.Write([]byte(`{"webhook_endpoints":[{"id":"whe_1","url":"https://example.com","event_types":null,"is_active":true}]}`))
		default:
			http.NotFound(w, r)
		}
	}))

	ctx := context.Background()

	projects, err := client.ListProjects(ctx)
	if err != nil || projects == nil || len(projects) != 0 {
		t.Errorf("ListProjects = %v, %v; want non-nil empty slice", projects, err)
	}
	previews, err := client.ListPreviewDatabases(ctx, "prj_1")
	if err != nil || previews == nil || len(previews) != 0 {
		t.Errorf("ListPreviewDatabases = %v, %v; want non-nil empty slice", previews, err)
	}
	keys, err := client.ListAPIKeys(ctx, "org_1")
	if err != nil || len(keys) != 1 || keys[0].Scopes == nil {
		t.Errorf("ListAPIKeys = %v, %v; want one key with non-nil scopes", keys, err)
	}
	endpoints, err := client.ListWebhookEndpoints(ctx, "org_1")
	if err != nil || len(endpoints) != 1 || endpoints[0].EventTypes == nil {
		t.Errorf("ListWebhookEndpoints = %v, %v; want one endpoint with non-nil event_types", endpoints, err)
	}
}

func TestListRegions(t *testing.T) {
	var gotPath string
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"regions":["us-east","eu-central"]}`))
	}))

	regions, err := client.ListRegions(context.Background())
	if err != nil {
		t.Fatalf("ListRegions: %v", err)
	}
	if gotPath != "/v1/regions" {
		t.Errorf("path = %q, want /v1/regions", gotPath)
	}
	if len(regions) != 2 || regions[0].Slug != "us-east" || regions[1].Slug != "eu-central" {
		t.Errorf("regions = %+v", regions)
	}
}

func TestListRegionsNormalizesNull(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"regions":null}`))
	}))

	regions, err := client.ListRegions(context.Background())
	if err != nil || regions == nil || len(regions) != 0 {
		t.Errorf("ListRegions = %v, %v; want non-nil empty slice", regions, err)
	}
}

func TestWaitForJobPollsUntilCompleted(t *testing.T) {
	var polls atomic.Int32
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		state := "running"
		if polls.Add(1) >= 3 {
			state = "completed"
		}
		_, _ = w.Write([]byte(`{"job":{"id":"job_1","type":"instance.create","state":"` + state + `"}}`))
	}))

	job, err := client.WaitForJob(context.Background(), "job_1")
	if err != nil {
		t.Fatalf("WaitForJob: %v", err)
	}
	if job.State != JobStateCompleted {
		t.Errorf("state = %q", job.State)
	}
	if got := polls.Load(); got != 3 {
		t.Errorf("polls = %d, want 3", got)
	}
}

func TestWaitForJobFailed(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"job":{"id":"job_1","type":"instance.create","state":"failed","error":"disk full"}}`))
	}))

	_, err := client.WaitForJob(context.Background(), "job_1")
	var failed *JobFailedError
	if !errors.As(err, &failed) {
		t.Fatalf("expected JobFailedError, got %v", err)
	}
	if failed.Job.Error != "disk full" {
		t.Errorf("job error = %q", failed.Job.Error)
	}
}

func TestWaitForJobHonorsContextDeadline(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"job":{"id":"job_1","type":"instance.create","state":"running"}}`))
	}))

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()

	_, err := client.WaitForJob(ctx, "job_1")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
}

func TestGetPreviewDatabaseNotFoundIn404(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"preview_databases":[{"id":"pvw_other","project_id":"prj_1","name":"x","mode":"clone","state":"ready","database_name":"db","role_name":"r","ttl_expires_at":"2026-06-11T00:00:00Z","created_at":"2026-06-10T00:00:00Z","updated_at":"2026-06-10T00:00:00Z"}]}`))
	}))

	_, err := client.GetPreviewDatabase(context.Background(), "prj_1", "pvw_missing")
	if !IsNotFound(err) {
		t.Fatalf("expected not-found error, got %v", err)
	}
}

func TestExtendPreviewDatabaseSendsHours(t *testing.T) {
	var gotBody map[string]any
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		_, _ = w.Write([]byte(`{"preview":{"id":"pvw_1","project_id":"prj_1","name":"x","mode":"clone","state":"ready","database_name":"db","role_name":"r","ttl_expires_at":"2026-06-12T00:00:00Z","created_at":"2026-06-10T00:00:00Z","updated_at":"2026-06-10T00:00:00Z"}}`))
	}))

	preview, err := client.ExtendPreviewDatabase(context.Background(), "pvw_1", 24)
	if err != nil {
		t.Fatalf("ExtendPreviewDatabase: %v", err)
	}
	if gotBody["ttl_hours"] != float64(24) {
		t.Errorf("body = %v", gotBody)
	}
	if preview.ID != "pvw_1" {
		t.Errorf("preview = %+v", preview)
	}
}

func TestUpdateWebhookEndpointOmitsNilFields(t *testing.T) {
	var raw []byte
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte(`{"webhook_endpoint":{"id":"whe_1","organization_id":"org_1","url":"https://new.example.com","event_types":[],"is_active":true,"created_at":"2026-06-10T00:00:00Z","updated_at":"2026-06-10T00:00:00Z"}}`))
	}))

	urlValue := "https://new.example.com"
	endpoint, err := client.UpdateWebhookEndpoint(context.Background(), "org_1", "whe_1", UpdateWebhookEndpointRequest{
		URL: &urlValue,
	})
	if err != nil {
		t.Fatalf("UpdateWebhookEndpoint: %v", err)
	}

	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if body["url"] != urlValue {
		t.Errorf("url = %v", body["url"])
	}
	for _, field := range []string{"description", "event_types", "is_active"} {
		if _, present := body[field]; present {
			t.Errorf("nil field %q must be omitted; body = %s", field, raw)
		}
	}
	if endpoint.URL != urlValue || endpoint.EventTypes == nil {
		t.Errorf("endpoint = %+v", endpoint)
	}
}

func TestCreateAPIKeyReturnsPlaintextOnce(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"api_key":{"id":"key_1","organization_id":"org_1","name":"ci","key_prefix":"capy_live_ab","scopes":["projects:read"],"source":"api","is_active":true,"created_at":"2026-06-10T00:00:00Z"},"plaintext_api_key":"capy_live_secret"}`))
	}))

	created, err := client.CreateAPIKey(context.Background(), "org_1", CreateAPIKeyRequest{
		Name:   "ci",
		Scopes: []string{"projects:read"},
	})
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	if created.PlaintextKey != "capy_live_secret" || created.APIKey.ID != "key_1" {
		t.Errorf("created = %+v", created)
	}
}

func TestRotateWebhookEndpointSecret(t *testing.T) {
	var gotPath string
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"webhook_endpoint":{"id":"whe_1","organization_id":"org_1","url":"https://example.com","event_types":[],"is_active":true,"created_at":"2026-06-10T00:00:00Z","updated_at":"2026-06-10T00:00:00Z"},"plaintext_secret":"whsec_new"}`))
	}))

	rotated, err := client.RotateWebhookEndpointSecret(context.Background(), "org_1", "whe_1")
	if err != nil {
		t.Fatalf("RotateWebhookEndpointSecret: %v", err)
	}
	if gotPath != "/v1/organizations/org_1/webhook-endpoints/whe_1/rotate-secret" {
		t.Errorf("path = %q", gotPath)
	}
	if rotated.PlaintextSecret != "whsec_new" {
		t.Errorf("secret = %q", rotated.PlaintextSecret)
	}
}
