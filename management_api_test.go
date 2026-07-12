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
