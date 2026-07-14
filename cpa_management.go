package main

import (
	"bytes"
	"encoding/json"
	"errors"
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

func extractBearerToken(headers http.Header) string {
	if headers == nil {
		return ""
	}
	auth := strings.TrimSpace(headers.Get("Authorization"))
	if auth == "" {
		for key, values := range headers {
			if strings.EqualFold(strings.TrimSpace(key), "Authorization") && len(values) > 0 {
				auth = strings.TrimSpace(values[0])
				break
			}
		}
	}
	if auth == "" {
		return ""
	}
	const prefix = "bearer "
	if len(auth) > len(prefix) && strings.EqualFold(auth[:len(prefix)], prefix) {
		return strings.TrimSpace(auth[len(prefix):])
	}
	return auth
}

func resolveManagementPassword(headers http.Header) string {
	if headers != nil {
		if token := extractBearerToken(headers); token != "" {
			return token
		}
		if token := strings.TrimSpace(headers.Get("X-Management-Key")); token != "" {
			return token
		}
		for key, values := range headers {
			if strings.EqualFold(strings.TrimSpace(key), "X-Management-Key") && len(values) > 0 {
				if token := strings.TrimSpace(values[0]); token != "" {
					return token
				}
			}
		}
	}
	return cpaManagementPassword()
}

func disableAuthInCPA(authID string) error {
	return setAuthDisabledInCPA(authID, true, cpaManagementPassword())
}

// errAuthFileNotFound means the credential no longer exists in CPA.
// Unban should still clear the local ban record in this case.
var errAuthFileNotFound = errors.New("auth file not found")

func enableAuthInCPA(authID string, password string) error {
	return setAuthDisabledInCPA(authID, false, password)
}

// enableAuthInCPAAllowMissing re-enables an account. If the auth file is already
// gone from CPA, it returns enabled=false with a nil error so callers can drop
// the local ban record.
func enableAuthInCPAAllowMissing(authID string, password string) (enabled bool, err error) {
	err = enableAuthInCPA(authID, password)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, errAuthFileNotFound) {
		return false, nil
	}
	return false, err
}

func isAuthFileNotFoundResponse(statusCode int, raw []byte) bool {
	if statusCode == http.StatusNotFound {
		return true
	}
	body := strings.ToLower(strings.TrimSpace(string(raw)))
	if body == "" {
		return false
	}
	return strings.Contains(body, "auth file not found") ||
		strings.Contains(body, `"error":"auth file not found"`) ||
		strings.Contains(body, "file not found")
}

func setAuthDisabledInCPA(authID string, disabled bool, password string) error {
	authID = strings.TrimSpace(authID)
	if authID == "" {
		return fmt.Errorf("auth_id is required")
	}
	password = strings.TrimSpace(password)
	if password == "" {
		password = cpaManagementPassword()
	}
	if password == "" {
		return fmt.Errorf("CPA management password is unavailable")
	}

	body, errMarshal := json.Marshal(map[string]any{
		"name":     authID,
		"disabled": disabled,
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
		if isAuthFileNotFoundResponse(resp.StatusCode, raw) {
			return fmt.Errorf("%w: %s", errAuthFileNotFound, strings.TrimSpace(string(raw)))
		}
		return fmt.Errorf("CPA management API returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return nil
}
