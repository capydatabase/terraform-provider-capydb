// Package capydb is a typed, context-aware HTTP client for the CapyDB
// control-plane API. It is consumed by the Terraform provider resources and
// data sources.
//
// Conventions mirrored from the control plane:
//   - All endpoints are JSON over HTTPS with bearer (org API key) auth.
//   - Mutating lifecycle operations (project provision/delete, preview
//     create/delete) return asynchronous job rows that must be polled until
//     they reach a terminal state.
//   - List endpoints may serialize empty Go slices as JSON null — every list
//     decode is normalized to a non-nil slice.
package capydb

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultBaseURL is the public CapyDB API bridge.
const DefaultBaseURL = "https://capydb.dev/api/capydb"

const defaultHTTPTimeout = 30 * time.Second

// APIError is a non-2xx control-plane response.
type APIError struct {
	Message    string
	StatusCode int
}

func (e *APIError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("capydb api request failed with status %d", e.StatusCode)
	}
	return fmt.Sprintf("capydb api request failed with status %d: %s", e.StatusCode, e.Message)
}

// IsNotFound reports whether err is an APIError with HTTP status 404.
func IsNotFound(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound
}

// Client talks to the CapyDB control plane.
type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
	userAgent  string
	// retryBackoff holds the delay before each idempotent GET attempt. The
	// first entry is the delay before the initial attempt (zero), so the
	// slice length equals the total number of attempts. Non-GET requests are
	// never retried.
	retryBackoff []time.Duration
	// jobPollInterval is the delay between job state polls in WaitForJob.
	jobPollInterval time.Duration
}

// Option customizes a Client.
type Option func(*Client)

// WithHTTPClient replaces the underlying *http.Client.
func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) { c.httpClient = httpClient }
}

// WithRetryBackoff replaces the GET retry schedule. The slice length is the
// total number of attempts; each entry is the delay before that attempt.
func WithRetryBackoff(backoff []time.Duration) Option {
	return func(c *Client) { c.retryBackoff = backoff }
}

// WithJobPollInterval sets the delay between job polls in WaitForJob.
func WithJobPollInterval(interval time.Duration) Option {
	return func(c *Client) { c.jobPollInterval = interval }
}

// NewClient builds an API client. version is the provider build version used
// for the User-Agent header.
func NewClient(baseURL, apiKey, version string, opts ...Option) (*Client, error) {
	trimmedBase := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if trimmedBase == "" {
		trimmedBase = DefaultBaseURL
	}
	if _, err := url.ParseRequestURI(trimmedBase); err != nil {
		return nil, fmt.Errorf("invalid api url %q: %w", baseURL, err)
	}

	versionToken := "dev"
	if fields := strings.Fields(strings.TrimSpace(version)); len(fields) > 0 {
		versionToken = fields[0]
	}

	client := &Client{
		apiKey:          strings.TrimSpace(apiKey),
		baseURL:         trimmedBase,
		httpClient:      &http.Client{Timeout: defaultHTTPTimeout},
		userAgent:       "terraform-provider-capydb/" + versionToken,
		retryBackoff:    []time.Duration{0, time.Second, 3 * time.Second},
		jobPollInterval: 5 * time.Second,
	}
	for _, opt := range opts {
		opt(client)
	}
	return client, nil
}

// Job is an asynchronous control-plane job. Poll until State is "completed"
// or "failed".
type Job struct {
	ID                string `json:"id"`
	Type              string `json:"type"`
	State             string `json:"state"`
	Error             string `json:"error,omitempty"`
	ProjectID         string `json:"project_id,omitempty"`
	PreviewDatabaseID string `json:"preview_database_id,omitempty"`
	OrganizationID    string `json:"organization_id,omitempty"`
	Attempts          int    `json:"attempts"`
	MaxAttempts       int    `json:"max_attempts"`
}

// Terminal job states.
const (
	JobStateCompleted = "completed"
	JobStateFailed    = "failed"
)

// JobFailedError is returned when an awaited job reaches the failed state.
type JobFailedError struct {
	Job Job
}

func (e *JobFailedError) Error() string {
	if e.Job.Error == "" {
		return fmt.Sprintf("job %s (%s) failed", e.Job.ID, e.Job.Type)
	}
	return fmt.Sprintf("job %s (%s) failed: %s", e.Job.ID, e.Job.Type, e.Job.Error)
}

// Project is a CapyDB project (a logical Postgres database).
type Project struct {
	ID                string `json:"id"`
	OrganizationID    string `json:"organization_id"`
	PrimaryInstanceID string `json:"primary_instance_id"`
	Environment       string `json:"environment,omitempty"`
	Plan              string `json:"plan,omitempty"`
	Name              string `json:"name"`
	Slug              string `json:"slug"`
	Region            string `json:"region,omitempty"`
	DatabaseName      string `json:"database_name,omitempty"`
	State             string `json:"state"`
	LastError         string `json:"last_error,omitempty"`
	LatestJobID       string `json:"latest_job_id,omitempty"`
}

// CreateProjectRequest creates a project. The plan is derived from the
// organization's billing state and cannot be chosen here. Region pins the
// placement region; omit it to let CapyDB pick.
type CreateProjectRequest struct {
	Name        string `json:"name"`
	Region      string `json:"region,omitempty"`
	Environment string `json:"environment,omitempty"`
	Slug        string `json:"slug,omitempty"`
}

// UpdateProjectRequest updates mutable project metadata. Only the environment
// label is mutable; nil leaves it unchanged.
type UpdateProjectRequest struct {
	Environment *string `json:"environment,omitempty"`
}

// ConnectionInfo carries the credential-embedded connection URLs.
type ConnectionInfo struct {
	DirectURL string `json:"direct_url,omitempty"`
	PooledURL string `json:"pooled_url,omitempty"`
	Username  string `json:"username"`
}

// PreviewDatabase is a disposable preview/branch database.
type PreviewDatabase struct {
	ID           string    `json:"id"`
	ProjectID    string    `json:"project_id"`
	Name         string    `json:"name"`
	Mode         string    `json:"mode"`
	State        string    `json:"state"`
	DatabaseName string    `json:"database_name"`
	RoleName     string    `json:"role_name"`
	PublicHost   string    `json:"public_host,omitempty"`
	LastError    string    `json:"last_error,omitempty"`
	TTLExpiresAt time.Time `json:"ttl_expires_at"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// CreatePreviewRequest creates a preview database.
type CreatePreviewRequest struct {
	Name     string `json:"name,omitempty"`
	Mode     string `json:"mode,omitempty"`
	TTLHours int64  `json:"ttl_hours,omitempty"`
}

// APIKey is org API key metadata. The plaintext secret is only ever returned
// at creation time.
type APIKey struct {
	ID             string   `json:"id"`
	OrganizationID string   `json:"organization_id"`
	ProjectID      string   `json:"project_id,omitempty"`
	Name           string   `json:"name"`
	KeyPrefix      string   `json:"key_prefix"`
	Scopes         []string `json:"scopes"`
	Source         string   `json:"source"`
	IsActive       bool     `json:"is_active"`
	CreatedAt      string   `json:"created_at"`
	RevokedAt      string   `json:"revoked_at,omitempty"`
}

// CreateAPIKeyRequest creates an org-wide or project-scoped API key.
type CreateAPIKeyRequest struct {
	Name      string   `json:"name"`
	Scopes    []string `json:"scopes"`
	ProjectID string   `json:"project_id,omitempty"`
}

// CreatedAPIKey pairs key metadata with the plaintext secret returned exactly
// once at creation time.
type CreatedAPIKey struct {
	APIKey       APIKey
	PlaintextKey string
}

// WebhookEndpoint is an outbound webhook receiver registration.
type WebhookEndpoint struct {
	ID             string   `json:"id"`
	OrganizationID string   `json:"organization_id"`
	URL            string   `json:"url"`
	Description    string   `json:"description,omitempty"`
	EventTypes     []string `json:"event_types"`
	IsActive       bool     `json:"is_active"`
	CreatedAt      string   `json:"created_at"`
	UpdatedAt      string   `json:"updated_at"`
}

// CreateWebhookEndpointRequest registers a webhook endpoint. Empty EventTypes
// subscribes to all events.
type CreateWebhookEndpointRequest struct {
	URL         string   `json:"url"`
	Description string   `json:"description,omitempty"`
	EventTypes  []string `json:"event_types,omitempty"`
}

// UpdateWebhookEndpointRequest updates a webhook endpoint. Nil fields are
// left unchanged.
type UpdateWebhookEndpointRequest struct {
	URL         *string   `json:"url,omitempty"`
	Description *string   `json:"description,omitempty"`
	EventTypes  *[]string `json:"event_types,omitempty"`
	IsActive    *bool     `json:"is_active,omitempty"`
}

// CreatedWebhookEndpoint pairs an endpoint with the plaintext signing secret
// returned exactly once (at creation or rotation).
type CreatedWebhookEndpoint struct {
	Endpoint        WebhookEndpoint
	PlaintextSecret string
}

// Region is a placement region projects can be created in.
type Region struct {
	Slug string `json:"slug"`
}

// Organization is the org record exposed to its members.
type Organization struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Slug          string `json:"slug"`
	BillingPlan   string `json:"billing_plan"`
	BillingStatus string `json:"billing_status"`
}

// Viewer is the authenticated principal plus its organization (nil when the
// principal is not org-bound, e.g. the platform admin token).
type Viewer struct {
	Organization *Organization `json:"organization"`
}

// GetViewer returns the authenticated principal's organization.
func (c *Client) GetViewer(ctx context.Context) (Viewer, error) {
	var response Viewer
	if err := c.do(ctx, http.MethodGet, "/v1/me", nil, &response); err != nil {
		return Viewer{}, err
	}
	return response, nil
}

// ListRegions lists the placement regions available to the organization.
func (c *Client) ListRegions(ctx context.Context) ([]Region, error) {
	var response struct {
		Regions []string `json:"regions"`
	}
	if err := c.do(ctx, http.MethodGet, "/v1/regions", nil, &response); err != nil {
		return nil, err
	}
	regions := make([]Region, 0, len(response.Regions))
	for _, slug := range response.Regions {
		regions = append(regions, Region{Slug: slug})
	}
	return regions, nil
}

// CreateProject creates a project and returns it plus the asynchronous
// provision job.
func (c *Client) CreateProject(ctx context.Context, request CreateProjectRequest) (Project, Job, error) {
	var response struct {
		Project Project `json:"project"`
		Job     Job     `json:"job"`
	}
	if err := c.do(ctx, http.MethodPost, "/v1/projects", request, &response); err != nil {
		return Project{}, Job{}, err
	}
	return response.Project, response.Job, nil
}

// GetProject returns a single project.
func (c *Client) GetProject(ctx context.Context, projectID string) (Project, error) {
	var response struct {
		Project Project `json:"project"`
	}
	if err := c.do(ctx, http.MethodGet, "/v1/projects/"+url.PathEscape(projectID), nil, &response); err != nil {
		return Project{}, err
	}
	return response.Project, nil
}

// ListProjects lists the organization's projects.
func (c *Client) ListProjects(ctx context.Context) ([]Project, error) {
	var response struct {
		Projects []Project `json:"projects"`
	}
	if err := c.do(ctx, http.MethodGet, "/v1/projects", nil, &response); err != nil {
		return nil, err
	}
	return normalizeList(response.Projects), nil
}

// UpdateProject updates mutable project metadata (currently only the
// environment label).
func (c *Client) UpdateProject(ctx context.Context, projectID string, request UpdateProjectRequest) (Project, error) {
	var response struct {
		Project Project `json:"project"`
	}
	if err := c.do(ctx, http.MethodPatch, "/v1/projects/"+url.PathEscape(projectID), request, &response); err != nil {
		return Project{}, err
	}
	return response.Project, nil
}

// DeleteProject enqueues asynchronous deletion and returns the job.
func (c *Client) DeleteProject(ctx context.Context, projectID string) (Job, error) {
	var response struct {
		Job Job `json:"job"`
	}
	if err := c.do(ctx, http.MethodDelete, "/v1/projects/"+url.PathEscape(projectID), nil, &response); err != nil {
		return Job{}, err
	}
	return response.Job, nil
}

// GetProjectConnections returns the project's pooled and direct connection
// URLs with credentials embedded. Requires the credentials:read scope.
func (c *Client) GetProjectConnections(ctx context.Context, projectID string) (ConnectionInfo, error) {
	var response struct {
		Connections ConnectionInfo `json:"connections"`
	}
	if err := c.do(ctx, http.MethodGet, "/v1/projects/"+url.PathEscape(projectID)+"/connections", nil, &response); err != nil {
		return ConnectionInfo{}, err
	}
	return response.Connections, nil
}

// CreatePreviewDatabase creates a preview database and returns it plus the
// asynchronous create job.
func (c *Client) CreatePreviewDatabase(ctx context.Context, projectID string, request CreatePreviewRequest) (PreviewDatabase, Job, error) {
	var response struct {
		Preview PreviewDatabase `json:"preview"`
		Job     Job             `json:"job"`
	}
	if err := c.do(ctx, http.MethodPost, "/v1/projects/"+url.PathEscape(projectID)+"/preview-databases", request, &response); err != nil {
		return PreviewDatabase{}, Job{}, err
	}
	return response.Preview, response.Job, nil
}

// ListPreviewDatabases lists the project's preview databases.
func (c *Client) ListPreviewDatabases(ctx context.Context, projectID string) ([]PreviewDatabase, error) {
	var response struct {
		PreviewDatabases []PreviewDatabase `json:"preview_databases"`
	}
	if err := c.do(ctx, http.MethodGet, "/v1/projects/"+url.PathEscape(projectID)+"/preview-databases", nil, &response); err != nil {
		return nil, err
	}
	return normalizeList(response.PreviewDatabases), nil
}

// GetPreviewDatabase resolves a single preview database by listing the
// project's previews (the API has no direct preview GET endpoint). A missing
// preview is reported as a 404 APIError so callers can use IsNotFound.
func (c *Client) GetPreviewDatabase(ctx context.Context, projectID, previewID string) (PreviewDatabase, error) {
	previews, err := c.ListPreviewDatabases(ctx, projectID)
	if err != nil {
		return PreviewDatabase{}, err
	}
	for _, preview := range previews {
		if preview.ID == previewID {
			return preview, nil
		}
	}
	return PreviewDatabase{}, &APIError{
		Message:    fmt.Sprintf("preview database %s not found in project %s", previewID, projectID),
		StatusCode: http.StatusNotFound,
	}
}

// DeletePreviewDatabase enqueues asynchronous deletion and returns the job.
func (c *Client) DeletePreviewDatabase(ctx context.Context, previewID string) (Job, error) {
	var response struct {
		Job Job `json:"job"`
	}
	if err := c.do(ctx, http.MethodDelete, "/v1/preview-databases/"+url.PathEscape(previewID), nil, &response); err != nil {
		return Job{}, err
	}
	return response.Job, nil
}

// ExtendPreviewDatabase extends the preview's TTL by ttlHours.
func (c *Client) ExtendPreviewDatabase(ctx context.Context, previewID string, ttlHours int64) (PreviewDatabase, error) {
	var response struct {
		Preview PreviewDatabase `json:"preview"`
	}
	if err := c.do(ctx, http.MethodPost, "/v1/preview-databases/"+url.PathEscape(previewID)+"/extend", map[string]int64{
		"ttl_hours": ttlHours,
	}, &response); err != nil {
		return PreviewDatabase{}, err
	}
	return response.Preview, nil
}

// GetPreviewConnections returns the preview database's connection URLs.
func (c *Client) GetPreviewConnections(ctx context.Context, previewID string) (ConnectionInfo, error) {
	var response struct {
		Connections ConnectionInfo `json:"connections"`
	}
	if err := c.do(ctx, http.MethodGet, "/v1/preview-databases/"+url.PathEscape(previewID)+"/connections", nil, &response); err != nil {
		return ConnectionInfo{}, err
	}
	return response.Connections, nil
}

// CreateAPIKey creates an org-wide or project-scoped API key. The plaintext
// key is returned exactly once.
func (c *Client) CreateAPIKey(ctx context.Context, orgID string, request CreateAPIKeyRequest) (CreatedAPIKey, error) {
	var response struct {
		APIKey       APIKey `json:"api_key"`
		PlaintextKey string `json:"plaintext_api_key"`
	}
	if err := c.do(ctx, http.MethodPost, "/v1/organizations/"+url.PathEscape(orgID)+"/api-keys", request, &response); err != nil {
		return CreatedAPIKey{}, err
	}
	response.APIKey.Scopes = normalizeList(response.APIKey.Scopes)
	return CreatedAPIKey{APIKey: response.APIKey, PlaintextKey: response.PlaintextKey}, nil
}

// ListAPIKeys lists the organization's API keys (metadata only).
func (c *Client) ListAPIKeys(ctx context.Context, orgID string) ([]APIKey, error) {
	var response struct {
		APIKeys []APIKey `json:"api_keys"`
	}
	if err := c.do(ctx, http.MethodGet, "/v1/organizations/"+url.PathEscape(orgID)+"/api-keys", nil, &response); err != nil {
		return nil, err
	}
	keys := normalizeList(response.APIKeys)
	for i := range keys {
		keys[i].Scopes = normalizeList(keys[i].Scopes)
	}
	return keys, nil
}

// GetAPIKey resolves a single API key by listing (the API has no direct key
// GET endpoint). A missing key is reported as a 404 APIError.
func (c *Client) GetAPIKey(ctx context.Context, orgID, keyID string) (APIKey, error) {
	keys, err := c.ListAPIKeys(ctx, orgID)
	if err != nil {
		return APIKey{}, err
	}
	for _, key := range keys {
		if key.ID == keyID {
			return key, nil
		}
	}
	return APIKey{}, &APIError{
		Message:    fmt.Sprintf("api key %s not found in organization %s", keyID, orgID),
		StatusCode: http.StatusNotFound,
	}
}

// RevokeAPIKey revokes an API key immediately.
func (c *Client) RevokeAPIKey(ctx context.Context, orgID, keyID string) error {
	return c.do(ctx, http.MethodDelete, "/v1/organizations/"+url.PathEscape(orgID)+"/api-keys/"+url.PathEscape(keyID), nil, nil)
}

// CreateWebhookEndpoint registers a webhook endpoint. The signing secret is
// returned exactly once.
func (c *Client) CreateWebhookEndpoint(ctx context.Context, orgID string, request CreateWebhookEndpointRequest) (CreatedWebhookEndpoint, error) {
	var response struct {
		Endpoint        WebhookEndpoint `json:"webhook_endpoint"`
		PlaintextSecret string          `json:"plaintext_secret"`
	}
	if err := c.do(ctx, http.MethodPost, "/v1/organizations/"+url.PathEscape(orgID)+"/webhook-endpoints", request, &response); err != nil {
		return CreatedWebhookEndpoint{}, err
	}
	response.Endpoint.EventTypes = normalizeList(response.Endpoint.EventTypes)
	return CreatedWebhookEndpoint{Endpoint: response.Endpoint, PlaintextSecret: response.PlaintextSecret}, nil
}

// ListWebhookEndpoints lists the organization's webhook endpoints.
func (c *Client) ListWebhookEndpoints(ctx context.Context, orgID string) ([]WebhookEndpoint, error) {
	var response struct {
		WebhookEndpoints []WebhookEndpoint `json:"webhook_endpoints"`
	}
	if err := c.do(ctx, http.MethodGet, "/v1/organizations/"+url.PathEscape(orgID)+"/webhook-endpoints", nil, &response); err != nil {
		return nil, err
	}
	endpoints := normalizeList(response.WebhookEndpoints)
	for i := range endpoints {
		endpoints[i].EventTypes = normalizeList(endpoints[i].EventTypes)
	}
	return endpoints, nil
}

// GetWebhookEndpoint resolves a single endpoint by listing (the API has no
// direct endpoint GET). A missing endpoint is reported as a 404 APIError.
func (c *Client) GetWebhookEndpoint(ctx context.Context, orgID, endpointID string) (WebhookEndpoint, error) {
	endpoints, err := c.ListWebhookEndpoints(ctx, orgID)
	if err != nil {
		return WebhookEndpoint{}, err
	}
	for _, endpoint := range endpoints {
		if endpoint.ID == endpointID {
			return endpoint, nil
		}
	}
	return WebhookEndpoint{}, &APIError{
		Message:    fmt.Sprintf("webhook endpoint %s not found in organization %s", endpointID, orgID),
		StatusCode: http.StatusNotFound,
	}
}

// UpdateWebhookEndpoint patches the endpoint; nil request fields are left
// unchanged.
func (c *Client) UpdateWebhookEndpoint(ctx context.Context, orgID, endpointID string, request UpdateWebhookEndpointRequest) (WebhookEndpoint, error) {
	var response struct {
		Endpoint WebhookEndpoint `json:"webhook_endpoint"`
	}
	path := "/v1/organizations/" + url.PathEscape(orgID) + "/webhook-endpoints/" + url.PathEscape(endpointID)
	if err := c.do(ctx, http.MethodPatch, path, request, &response); err != nil {
		return WebhookEndpoint{}, err
	}
	response.Endpoint.EventTypes = normalizeList(response.Endpoint.EventTypes)
	return response.Endpoint, nil
}

// DeleteWebhookEndpoint deletes the endpoint; pending deliveries are dropped.
func (c *Client) DeleteWebhookEndpoint(ctx context.Context, orgID, endpointID string) error {
	path := "/v1/organizations/" + url.PathEscape(orgID) + "/webhook-endpoints/" + url.PathEscape(endpointID)
	return c.do(ctx, http.MethodDelete, path, nil, nil)
}

// RotateWebhookEndpointSecret generates a new signing secret and returns it
// exactly once. The previous secret stops validating immediately.
func (c *Client) RotateWebhookEndpointSecret(ctx context.Context, orgID, endpointID string) (CreatedWebhookEndpoint, error) {
	var response struct {
		Endpoint        WebhookEndpoint `json:"webhook_endpoint"`
		PlaintextSecret string          `json:"plaintext_secret"`
	}
	path := "/v1/organizations/" + url.PathEscape(orgID) + "/webhook-endpoints/" + url.PathEscape(endpointID) + "/rotate-secret"
	if err := c.do(ctx, http.MethodPost, path, nil, &response); err != nil {
		return CreatedWebhookEndpoint{}, err
	}
	response.Endpoint.EventTypes = normalizeList(response.Endpoint.EventTypes)
	return CreatedWebhookEndpoint{Endpoint: response.Endpoint, PlaintextSecret: response.PlaintextSecret}, nil
}

// GetJob returns a single asynchronous job.
func (c *Client) GetJob(ctx context.Context, jobID string) (Job, error) {
	var response struct {
		Job Job `json:"job"`
	}
	if err := c.do(ctx, http.MethodGet, "/v1/jobs/"+url.PathEscape(jobID), nil, &response); err != nil {
		return Job{}, err
	}
	return response.Job, nil
}

// WaitForJob polls jobID until it reaches a terminal state or ctx expires.
// A failed job is returned as a *JobFailedError. Callers bound the wait via
// the context deadline.
func (c *Client) WaitForJob(ctx context.Context, jobID string) (Job, error) {
	for {
		job, err := c.GetJob(ctx, jobID)
		if err != nil {
			return Job{}, fmt.Errorf("poll job %s: %w", jobID, err)
		}
		switch job.State {
		case JobStateCompleted:
			return job, nil
		case JobStateFailed:
			return job, &JobFailedError{Job: job}
		}

		select {
		case <-ctx.Done():
			return job, fmt.Errorf("waiting for job %s (%s, state %s): %w", jobID, job.Type, job.State, ctx.Err())
		case <-time.After(c.jobPollInterval):
		}
	}
}

// normalizeList converts a nil slice (the control plane may serialize empty
// lists as JSON null) into an empty, non-nil slice.
func normalizeList[T any](list []T) []T {
	if list == nil {
		return []T{}
	}
	return list
}

// do executes an API request. Idempotent GET requests are retried on network
// errors and 5xx responses following c.retryBackoff. Non-GET requests are
// never retried.
func (c *Client) do(ctx context.Context, method, path string, payload any, dest any) error {
	var encoded []byte
	if payload != nil {
		var err error
		encoded, err = json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("encode request body: %w", err)
		}
	}

	backoff := []time.Duration{0}
	if method == http.MethodGet && len(c.retryBackoff) > 0 {
		backoff = c.retryBackoff
	}

	var lastErr error
	for attempt, delay := range backoff {
		if delay > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}

		retryable, err := c.doOnce(ctx, method, path, encoded, payload != nil, dest)
		if err == nil {
			return nil
		}
		lastErr = err
		if !retryable || ctx.Err() != nil || attempt == len(backoff)-1 {
			return err
		}
	}
	return lastErr
}

// doOnce performs a single HTTP round trip. The boolean return reports
// whether the failure is retryable (network error or 5xx response).
func (c *Client) doOnce(ctx context.Context, method, path string, encoded []byte, hasBody bool, dest any) (bool, error) {
	var body io.Reader
	if hasBody {
		body = bytes.NewReader(encoded)
	}

	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return false, fmt.Errorf("build request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", c.userAgent)
	if hasBody {
		request.Header.Set("Content-Type", "application/json")
	}
	if c.apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return true, fmt.Errorf("perform request: %w", err)
	}
	defer func() { _ = response.Body.Close() }()

	raw, err := io.ReadAll(response.Body)
	if err != nil {
		return true, fmt.Errorf("read response body: %w", err)
	}

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var payload struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(raw, &payload)
		return response.StatusCode >= 500, &APIError{
			Message:    payload.Error,
			StatusCode: response.StatusCode,
		}
	}

	if dest == nil || len(raw) == 0 {
		return false, nil
	}
	if err := json.Unmarshal(raw, dest); err != nil {
		return false, fmt.Errorf("decode response body: %w", err)
	}
	return false, nil
}
