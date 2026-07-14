package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"grok-autoban/cpasdk/pluginapi"
)

func TestManagementRegistration(t *testing.T) {
	reg := managementRegistration()
	if len(reg.Routes) != 3 || len(reg.Resources) != 1 {
		t.Fatalf("registration = %#v", reg)
	}
	if reg.Routes[0].Path != managementRoutePrefix+"/bans" || reg.Routes[1].Path != managementRoutePrefix+"/unban" || reg.Routes[2].Path != managementRoutePrefix+"/unban-all" {
		t.Fatalf("routes = %#v", reg.Routes)
	}
	for _, route := range reg.Routes {
		if route.Handler != nil {
			t.Fatalf("RPC management route %s must not include a local handler", route.Path)
		}
	}
	if reg.Resources[0].Path != "/status" {
		t.Fatalf("resources = %#v", reg.Resources)
	}
	if reg.Resources[0].Handler != nil {
		t.Fatalf("RPC resource route %s must not include a local handler", reg.Resources[0].Path)
	}
}

func withMockCPAEnable(t *testing.T) func() {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	oldBaseURL := cpaManagementBaseURL
	oldDo := cpaManagementDo
	cpaManagementBaseURL = server.URL
	cpaManagementDo = server.Client().Do
	return func() {
		cpaManagementBaseURL = oldBaseURL
		cpaManagementDo = oldDo
		server.Close()
	}
}

func TestManagementListAndUnban(t *testing.T) {
	cleanup := withMockCPAEnable(t)
	defer cleanup()

	oldStore := activeStore
	activeStore = newBanStore()
	defer func() { activeStore = oldStore }()
	activeStore.Set(testEntry("auth-1", time.Now().Add(time.Hour)))

	list := managementRequest(http.MethodGet, "/bans", nil)
	response, err := dispatchManagement(list)
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("list response = %#v, err=%v", response, err)
	}
	if !strings.Contains(string(response.Body), "auth-1") {
		t.Fatalf("list body = %s", response.Body)
	}
	if strings.Contains(string(response.Body), "access_token") {
		t.Fatal("status leaked secret field")
	}

	unban := managementRequest(http.MethodPost, "/unban", []byte(`{"auth_id":"auth-1"}`))
	unban.Headers = http.Header{"Authorization": []string{"Bearer page-password"}}
	response, err = dispatchManagement(unban)
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("unban response = %#v, err=%v body=%s", response, err, string(response.Body))
	}
	if !strings.Contains(string(response.Body), `"enabled":true`) {
		t.Fatalf("unban body missing enabled: %s", response.Body)
	}
	if _, ok := activeStore.Get("auth-1"); ok {
		t.Fatal("auth-1 remains after unban")
	}
}

func TestManagementRejectsMissingAuthIDAndClearsAll(t *testing.T) {
	cleanup := withMockCPAEnable(t)
	defer cleanup()

	oldStore := activeStore
	activeStore = newBanStore()
	defer func() { activeStore = oldStore }()
	activeStore.Set(testEntry("auth-1", time.Now().Add(time.Hour)))
	activeStore.Set(testEntry("auth-2", time.Now().Add(time.Hour)))

	response, err := dispatchManagement(managementRequest(http.MethodPost, "/unban", []byte(`{}`)))
	if err != nil || response.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing auth response = %#v, err=%v", response, err)
	}
	req := managementRequest(http.MethodPost, "/unban-all", []byte(`{}`))
	req.Headers = http.Header{"Authorization": []string{"Bearer page-password"}}
	response, err = dispatchManagement(req)
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("unban all response = %#v, err=%v body=%s", response, err, string(response.Body))
	}
	if len(activeStore.List(time.Now())) != 0 {
		t.Fatal("unban-all did not clear state")
	}
}

func TestManagementResourcePageIsChinese(t *testing.T) {
	response, err := managementStatusPage(pluginapi.ManagementRequest{})
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("page response = %#v, err=%v", response, err)
	}
	body := string(response.Body)
	if !strings.Contains(body, "Grok") || !strings.Contains(body, "/bans") {
		t.Fatalf("page body missing expected text: %s", body)
	}
}

func managementRequest(method, path string, body []byte) pluginapi.ManagementRequest {
	return pluginapi.ManagementRequest{
		Method: method,
		Path:   path,
		Body:   body,
	}
}
