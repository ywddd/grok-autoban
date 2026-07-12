package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

var (
	cpaManagementBaseURL = "http://127.0.0.1:8317"
	cpaManagementDo      = http.DefaultClient.Do
)

func cpaManagementPassword() string {
	if value := strings.TrimSpace(os.Getenv("MANAGEMENT_PASSWORD")); value != "" {
		return value
	}
	return strings.TrimSpace(os.Getenv("CPA_MANAGEMENT_KEY"))
}

func disableAuthInCPA(authID string) error {
	authID = strings.TrimSpace(authID)
	if authID == "" {
		return fmt.Errorf("auth_id is required")
	}
	password := cpaManagementPassword()
	if password == "" {
		return fmt.Errorf("CPA management password is unavailable")
	}

	body, errMarshal := json.Marshal(map[string]any{
		"name":     authID,
		"disabled": true,
	})
	if errMarshal != nil {
		return errMarshal
	}
	req, errRequest := http.NewRequest(
		http.MethodPatch,
		strings.TrimRight(cpaManagementBaseURL, "/")+"/v0/management/auth-files/status",
		bytes.NewReader(body),
	)
	if errRequest != nil {
		return errRequest
	}
	req.Header.Set("Authorization", "Bearer "+password)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, errDo := cpaManagementDo(req)
	if errDo != nil {
		return errDo
	}
	defer resp.Body.Close()
	raw, errRead := io.ReadAll(resp.Body)
	if errRead != nil {
		return errRead
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("CPA management API returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return nil
}
