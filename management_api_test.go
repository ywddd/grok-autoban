package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestDisableAuthInCPAUsesManagementStatusAPI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Fatalf("method = %s", r.Method)
		}
		if r.URL.Path != "/v0/management/auth-files/status" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-management-password" {
			t.Fatalf("authorization = %q", got)
		}
		var body struct {
			Name     string `json:"name"`
			Disabled bool   `json:"disabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Name != "xai-test.json" || !body.Disabled {
			t.Fatalf("body = %#v", body)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","disabled":true}`))
	}))
	defer server.Close()

	oldBaseURL := cpaManagementBaseURL
	oldDo := cpaManagementDo
	oldPassword := os.Getenv("MANAGEMENT_PASSWORD")
	defer func() {
		cpaManagementBaseURL = oldBaseURL
		cpaManagementDo = oldDo
		_ = os.Setenv("MANAGEMENT_PASSWORD", oldPassword)
	}()

	cpaManagementBaseURL = server.URL
	cpaManagementDo = server.Client().Do
	_ = os.Setenv("MANAGEMENT_PASSWORD", "test-management-password")

	if err := disableAuthInCPA("xai-test.json"); err != nil {
		t.Fatal(err)
	}
}

func TestEnableAuthInCPAUsesPagePassword(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer page-password" {
			t.Fatalf("authorization = %q", got)
		}
		var body struct {
			Name     string `json:"name"`
			Disabled bool   `json:"disabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Name != "xai-test.json" || body.Disabled {
			t.Fatalf("body = %#v", body)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","disabled":false}`))
	}))
	defer server.Close()

	oldBaseURL := cpaManagementBaseURL
	oldDo := cpaManagementDo
	oldPassword := os.Getenv("MANAGEMENT_PASSWORD")
	defer func() {
		cpaManagementBaseURL = oldBaseURL
		cpaManagementDo = oldDo
		_ = os.Setenv("MANAGEMENT_PASSWORD", oldPassword)
	}()
	cpaManagementBaseURL = server.URL
	cpaManagementDo = server.Client().Do
	_ = os.Unsetenv("MANAGEMENT_PASSWORD")
	_ = os.Unsetenv("CPA_MANAGEMENT_KEY")

	if err := enableAuthInCPA("xai-test.json", "page-password"); err != nil {
		t.Fatal(err)
	}
}

func TestResolveManagementPasswordPrefersRequestBearer(t *testing.T) {
	oldPassword := os.Getenv("MANAGEMENT_PASSWORD")
	defer func() { _ = os.Setenv("MANAGEMENT_PASSWORD", oldPassword) }()
	_ = os.Setenv("MANAGEMENT_PASSWORD", "env-password")

	headers := http.Header{"Authorization": []string{"Bearer page-password"}}
	if got := resolveManagementPassword(headers); got != "page-password" {
		t.Fatalf("password = %q, want page-password", got)
	}
	if got := resolveManagementPassword(nil); got != "env-password" {
		t.Fatalf("env password = %q, want env-password", got)
	}
}

func TestEnableAuthInCPAAllowMissingTreats404AsMissing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"auth file not found"}`))
	}))
	defer server.Close()

	oldBaseURL := cpaManagementBaseURL
	oldDo := cpaManagementDo
	defer func() {
		cpaManagementBaseURL = oldBaseURL
		cpaManagementDo = oldDo
	}()
	cpaManagementBaseURL = server.URL
	cpaManagementDo = server.Client().Do

	enabled, err := enableAuthInCPAAllowMissing("gone.json", "page-password")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if enabled {
		t.Fatal("expected enabled=false for missing auth file")
	}
}

func TestEnableAuthInCPAAllowMissingPropagatesOtherErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"boom"}`))
	}))
	defer server.Close()

	oldBaseURL := cpaManagementBaseURL
	oldDo := cpaManagementDo
	defer func() {
		cpaManagementBaseURL = oldBaseURL
		cpaManagementDo = oldDo
	}()
	cpaManagementBaseURL = server.URL
	cpaManagementDo = server.Client().Do

	enabled, err := enableAuthInCPAAllowMissing("x.json", "page-password")
	if err == nil {
		t.Fatal("expected error")
	}
	if enabled {
		t.Fatal("expected enabled=false")
	}
}

