package gcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/impersonate"
	"google.golang.org/api/option"
)

const cloudScope = "https://www.googleapis.com/auth/cloud-platform"

func TokenSource(ctx context.Context, impersonateSA string) (oauth2.TokenSource, error) {
	if path := os.Getenv("DEVHUB_GCP_KEY_FILE"); path != "" {
		return nil, fmt.Errorf("devhub does not accept service account key files")
	}
	creds, err := google.FindDefaultCredentials(ctx, cloudScope)
	if err != nil {
		return nil, fmt.Errorf("application default credentials: %w (run: gcloud auth application-default login)", err)
	}
	if impersonateSA == "" {
		return creds.TokenSource, nil
	}
	ts, err := impersonate.CredentialsTokenSource(ctx, impersonate.CredentialsConfig{
		TargetPrincipal: impersonateSA,
		Scopes:          []string{cloudScope},
	})
	if err != nil {
		return nil, fmt.Errorf("impersonate %s: %w", impersonateSA, err)
	}
	return ts, nil
}

func HTTPClient(ctx context.Context, impersonateSA string) (*http.Client, error) {
	ts, err := TokenSource(ctx, impersonateSA)
	if err != nil {
		return nil, err
	}
	return oauth2.NewClient(ctx, ts), nil
}

func ClientOptions(ctx context.Context, impersonateSA string) ([]option.ClientOption, error) {
	ts, err := TokenSource(ctx, impersonateSA)
	if err != nil {
		return nil, err
	}
	return []option.ClientOption{option.WithTokenSource(ts)}, nil
}

type adcAuth struct {
	impersonate string
}

func (a *adcAuth) Check(ctx context.Context) (AuthInfo, error) {
	creds, err := google.FindDefaultCredentials(ctx, cloudScope)
	if err != nil {
		return AuthInfo{}, fmt.Errorf("ADC: %w (run: gcloud auth application-default login)", err)
	}
	info := AuthInfo{Email: adcEmail(creds.JSON)}
	if a.impersonate != "" {
		info.Email = a.impersonate + " (impersonated)"
	}
	return info, nil
}

func (a *adcAuth) Reachable(ctx context.Context, projectID string) bool {
	hc, err := HTTPClient(ctx, a.impersonate)
	if err != nil {
		return false
	}
	u := fmt.Sprintf("https://cloudresourcemanager.googleapis.com/v1/projects/%s", projectID)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	resp, err := hc.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode < 400
}

func adcEmail(raw []byte) string {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return "adc"
	}
	if v, ok := m["client_email"].(string); ok && v != "" {
		return v
	}
	if v, ok := m["quota_project_id"].(string); ok {
		return v
	}
	if strings.Contains(string(raw), "authorized_user") {
		return "authorized_user (gcloud ADC)"
	}
	return "adc"
}
