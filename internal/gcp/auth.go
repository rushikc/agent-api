package gcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

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
	email, quota := ParseADC(creds.JSON)
	if a.impersonate != "" {
		email = a.impersonate
	} else if email == "" {
		email = fetchUserEmail(ctx, creds.TokenSource)
	}
	return AuthInfo{Email: email, QuotaProject: quota}, nil
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

var (
	userinfoURL    = "https://www.googleapis.com/oauth2/v2/userinfo"
	userinfoClient = http.DefaultClient
)

func ParseADC(raw []byte) (email, quota string) {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return "", ""
	}
	if v, ok := m["quota_project_id"].(string); ok {
		quota = v
	}
	if v, ok := m["client_email"].(string); ok && v != "" {
		return v, quota
	}
	return "", quota
}

func adcEmail(raw []byte) string {
	email, _ := ParseADC(raw)
	return email
}

func fetchUserEmail(ctx context.Context, ts oauth2.TokenSource) string {
	if ts == nil {
		return ""
	}
	tok, err := ts.Token()
	if err != nil || tok == nil || tok.AccessToken == "" {
		return ""
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, userinfoURL, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	resp, err := userinfoClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return ""
	}
	var body struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return ""
	}
	return body.Email
}
