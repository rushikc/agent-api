package gcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/oauth2"
)

func TestParseADC(t *testing.T) {
	email, quota := ParseADC([]byte(`{"type":"service_account","client_email":"sa@x.iam.gserviceaccount.com","quota_project_id":"q1"}`))
	if email != "sa@x.iam.gserviceaccount.com" || quota != "q1" {
		t.Fatalf("%s %s", email, quota)
	}
	email, quota = ParseADC([]byte(`{"type":"authorized_user","client_id":"x","quota_project_id":"my-quota"}`))
	if email != "" || quota != "my-quota" {
		t.Fatalf("user adc %q %q", email, quota)
	}
	if adcEmail([]byte(`{"client_email":"a@b.com"}`)) != "a@b.com" {
		t.Fatal("adcEmail")
	}
}

func TestFetchUserEmail(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Fatalf("auth %s", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"email":"user@example.com"}`))
	}))
	t.Cleanup(ts.Close)
	oldURL, oldC := userinfoURL, userinfoClient
	t.Cleanup(func() { userinfoURL, userinfoClient = oldURL, oldC })
	userinfoURL = ts.URL
	userinfoClient = ts.Client()
	got := fetchUserEmail(context.Background(), oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "tok"}))
	if got != "user@example.com" {
		t.Fatalf("%q", got)
	}
}
