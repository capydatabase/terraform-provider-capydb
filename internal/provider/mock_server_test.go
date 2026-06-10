package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/capy-base/terraform-provider-capydb/internal/capydb"
)

// mockControlPlane is a stateful in-memory CapyDB control plane for resource
// CRUD tests. Async lifecycle operations apply their side effects immediately
// and hand back a job that reports "running" for the first poll and
// "completed" afterwards, exercising the provider's job-wait loops.
type mockControlPlane struct {
	mu sync.Mutex

	projects  map[string]*capydb.Project
	previews  map[string]*capydb.PreviewDatabase
	apiKeys   map[string]*capydb.APIKey
	webhooks  map[string]*capydb.WebhookEndpoint
	jobPolls  map[string]int
	jobStates map[string]string

	secretRotations int
	extendCalls     []int64
	patchedEnvs     []string

	nextID int
}

func newMockControlPlane() *mockControlPlane {
	return &mockControlPlane{
		projects:  map[string]*capydb.Project{},
		previews:  map[string]*capydb.PreviewDatabase{},
		apiKeys:   map[string]*capydb.APIKey{},
		webhooks:  map[string]*capydb.WebhookEndpoint{},
		jobPolls:  map[string]int{},
		jobStates: map[string]string{},
	}
}

func (m *mockControlPlane) id(prefix string) string {
	m.nextID++
	return fmt.Sprintf("%s_%d", prefix, m.nextID)
}

func (m *mockControlPlane) newJob(jobType string) capydb.Job {
	jobID := m.id("job")
	m.jobStates[jobID] = jobType
	return capydb.Job{ID: jobID, Type: jobType, State: "pending"}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func decode[T any](r *http.Request) (T, error) {
	var payload T
	err := json.NewDecoder(r.Body).Decode(&payload)
	return payload, err
}

// handler routes the subset of the CapyDB API the provider uses.
func (m *mockControlPlane) handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /v1/me", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"organization": capydb.Organization{
				ID: "org_1", Name: "Acme", Slug: "acme",
				BillingPlan: "launch", BillingStatus: "active",
			},
			"principal": map[string]any{"auth_source": "api_key", "scopes": []string{}},
		})
	})

	mux.HandleFunc("GET /v1/clusters", func(w http.ResponseWriter, r *http.Request) {
		// Deliberately serialize the list as null to exercise the provider's
		// null-list normalization end to end.
		_, _ = w.Write([]byte(`{"clusters":null}`))
	})

	mux.HandleFunc("GET /v1/jobs/{jobID}", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		jobID := r.PathValue("jobID")
		jobType, ok := m.jobStates[jobID]
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "job not found"})
			return
		}
		m.jobPolls[jobID]++
		state := "running"
		if m.jobPolls[jobID] > 1 {
			state = "completed"
		}
		writeJSON(w, http.StatusOK, map[string]any{"job": capydb.Job{ID: jobID, Type: jobType, State: state}})
	})

	mux.HandleFunc("POST /v1/projects", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		request, err := decode[capydb.CreateProjectRequest](r)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		environment := request.Environment
		if environment == "" {
			environment = "production"
		}
		clusterID := request.ClusterID
		if clusterID == "" {
			clusterID = "cls_auto"
		}
		project := &capydb.Project{
			ID:             m.id("prj"),
			OrganizationID: "org_1",
			ClusterID:      clusterID,
			Environment:    environment,
			Plan:           "launch",
			Name:           request.Name,
			Slug:           strings.ReplaceAll(strings.ToLower(request.Name), " ", "-"),
			Region:         "eu-central",
			DatabaseName:   "db_" + request.Name,
			State:          "ready",
		}
		m.projects[project.ID] = project
		writeJSON(w, http.StatusCreated, map[string]any{"project": project, "job": m.newJob("project.provision")})
	})

	mux.HandleFunc("GET /v1/projects", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		projects := make([]*capydb.Project, 0, len(m.projects))
		for _, project := range m.projects {
			projects = append(projects, project)
		}
		writeJSON(w, http.StatusOK, map[string]any{"projects": projects})
	})

	mux.HandleFunc("GET /v1/projects/{projectID}", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		project, ok := m.projects[r.PathValue("projectID")]
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"project": project})
	})

	mux.HandleFunc("PATCH /v1/projects/{projectID}", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		project, ok := m.projects[r.PathValue("projectID")]
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
			return
		}
		request, err := decode[capydb.UpdateProjectRequest](r)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if request.Environment != nil {
			project.Environment = *request.Environment
			m.patchedEnvs = append(m.patchedEnvs, *request.Environment)
		}
		writeJSON(w, http.StatusOK, map[string]any{"project": project})
	})

	mux.HandleFunc("DELETE /v1/projects/{projectID}", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		projectID := r.PathValue("projectID")
		if _, ok := m.projects[projectID]; !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
			return
		}
		delete(m.projects, projectID)
		writeJSON(w, http.StatusAccepted, map[string]any{"job": m.newJob("project.delete")})
	})

	mux.HandleFunc("GET /v1/projects/{projectID}/connections", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		project, ok := m.projects[r.PathValue("projectID")]
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"connections": capydb.ConnectionInfo{
			DirectURL: "postgres://user:pass@host:5432/" + project.DatabaseName,
			PooledURL: "postgres://user:pass@host:6432/" + project.DatabaseName,
			Username:  "user_" + project.ID,
		}})
	})

	mux.HandleFunc("POST /v1/projects/{projectID}/preview-databases", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		request, err := decode[capydb.CreatePreviewRequest](r)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		mode := request.Mode
		if mode == "" {
			mode = "clone"
		}
		name := request.Name
		if name == "" {
			name = "preview-auto"
		}
		preview := &capydb.PreviewDatabase{
			ID:           m.id("pvw"),
			ProjectID:    r.PathValue("projectID"),
			Name:         name,
			Mode:         mode,
			State:        "ready",
			DatabaseName: "db_" + name,
			RoleName:     "role_" + name,
			TTLExpiresAt: time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC).Add(time.Duration(request.TTLHours) * time.Hour),
			CreatedAt:    time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC),
			UpdatedAt:    time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC),
		}
		m.previews[preview.ID] = preview
		writeJSON(w, http.StatusCreated, map[string]any{"preview": preview, "job": m.newJob("preview.create")})
	})

	mux.HandleFunc("GET /v1/projects/{projectID}/preview-databases", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		previews := make([]*capydb.PreviewDatabase, 0, len(m.previews))
		for _, preview := range m.previews {
			if preview.ProjectID == r.PathValue("projectID") {
				previews = append(previews, preview)
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"preview_databases": previews})
	})

	mux.HandleFunc("DELETE /v1/preview-databases/{previewID}", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		previewID := r.PathValue("previewID")
		if _, ok := m.previews[previewID]; !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "preview not found"})
			return
		}
		delete(m.previews, previewID)
		writeJSON(w, http.StatusAccepted, map[string]any{"job": m.newJob("preview.delete")})
	})

	mux.HandleFunc("POST /v1/preview-databases/{previewID}/extend", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		preview, ok := m.previews[r.PathValue("previewID")]
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "preview not found"})
			return
		}
		request, err := decode[map[string]int64](r)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		hours := request["ttl_hours"]
		m.extendCalls = append(m.extendCalls, hours)
		preview.TTLExpiresAt = preview.TTLExpiresAt.Add(time.Duration(hours) * time.Hour)
		writeJSON(w, http.StatusOK, map[string]any{"preview": preview})
	})

	mux.HandleFunc("POST /v1/organizations/{orgID}/api-keys", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		request, err := decode[capydb.CreateAPIKeyRequest](r)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		key := &capydb.APIKey{
			ID:             m.id("key"),
			OrganizationID: r.PathValue("orgID"),
			ProjectID:      request.ProjectID,
			Name:           request.Name,
			KeyPrefix:      "capy_live_ab12",
			Scopes:         request.Scopes,
			Source:         "api",
			IsActive:       true,
			CreatedAt:      "2026-06-10T00:00:00Z",
		}
		m.apiKeys[key.ID] = key
		writeJSON(w, http.StatusCreated, map[string]any{
			"api_key":           key,
			"plaintext_api_key": "capy_live_secret_" + key.ID,
		})
	})

	mux.HandleFunc("GET /v1/organizations/{orgID}/api-keys", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		keys := make([]*capydb.APIKey, 0, len(m.apiKeys))
		for _, key := range m.apiKeys {
			keys = append(keys, key)
		}
		writeJSON(w, http.StatusOK, map[string]any{"api_keys": keys})
	})

	mux.HandleFunc("DELETE /v1/organizations/{orgID}/api-keys/{keyID}", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		key, ok := m.apiKeys[r.PathValue("keyID")]
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "api key not found"})
			return
		}
		key.IsActive = false
		key.RevokedAt = "2026-06-10T01:00:00Z"
		writeJSON(w, http.StatusOK, map[string]any{"revoked": true})
	})

	mux.HandleFunc("POST /v1/organizations/{orgID}/webhook-endpoints", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		request, err := decode[capydb.CreateWebhookEndpointRequest](r)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		eventTypes := request.EventTypes
		if eventTypes == nil {
			eventTypes = []string{}
		}
		endpoint := &capydb.WebhookEndpoint{
			ID:             m.id("whe"),
			OrganizationID: r.PathValue("orgID"),
			URL:            request.URL,
			Description:    request.Description,
			EventTypes:     eventTypes,
			IsActive:       true,
			CreatedAt:      "2026-06-10T00:00:00Z",
			UpdatedAt:      "2026-06-10T00:00:00Z",
		}
		m.webhooks[endpoint.ID] = endpoint
		writeJSON(w, http.StatusCreated, map[string]any{
			"webhook_endpoint": endpoint,
			"plaintext_secret": "whsec_initial_" + endpoint.ID,
		})
	})

	mux.HandleFunc("GET /v1/organizations/{orgID}/webhook-endpoints", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		endpoints := make([]*capydb.WebhookEndpoint, 0, len(m.webhooks))
		for _, endpoint := range m.webhooks {
			endpoints = append(endpoints, endpoint)
		}
		writeJSON(w, http.StatusOK, map[string]any{"webhook_endpoints": endpoints})
	})

	mux.HandleFunc("PATCH /v1/organizations/{orgID}/webhook-endpoints/{endpointID}", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		endpoint, ok := m.webhooks[r.PathValue("endpointID")]
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "endpoint not found"})
			return
		}
		request, err := decode[capydb.UpdateWebhookEndpointRequest](r)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if request.URL != nil {
			endpoint.URL = *request.URL
		}
		if request.Description != nil {
			endpoint.Description = *request.Description
		}
		if request.EventTypes != nil {
			endpoint.EventTypes = *request.EventTypes
		}
		if request.IsActive != nil {
			endpoint.IsActive = *request.IsActive
		}
		writeJSON(w, http.StatusOK, map[string]any{"webhook_endpoint": endpoint})
	})

	mux.HandleFunc("DELETE /v1/organizations/{orgID}/webhook-endpoints/{endpointID}", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		endpointID := r.PathValue("endpointID")
		if _, ok := m.webhooks[endpointID]; !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "endpoint not found"})
			return
		}
		delete(m.webhooks, endpointID)
		writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
	})

	mux.HandleFunc("POST /v1/organizations/{orgID}/webhook-endpoints/{endpointID}/rotate-secret", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		endpoint, ok := m.webhooks[r.PathValue("endpointID")]
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "endpoint not found"})
			return
		}
		m.secretRotations++
		writeJSON(w, http.StatusOK, map[string]any{
			"webhook_endpoint": endpoint,
			"plaintext_secret": fmt.Sprintf("whsec_rotated_%d", m.secretRotations),
		})
	})

	return mux
}

// newTestClient spins up the mock control plane and a client pointed at it,
// tuned for fast polling.
func newTestClient(t *testing.T) (*capydb.Client, *mockControlPlane) {
	t.Helper()
	mock := newMockControlPlane()
	server := httptest.NewServer(mock.handler())
	t.Cleanup(server.Close)

	client, err := capydb.NewClient(server.URL, "capy_live_test", "test",
		capydb.WithJobPollInterval(time.Millisecond),
		capydb.WithRetryBackoff([]time.Duration{0, time.Millisecond}),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return client, mock
}
