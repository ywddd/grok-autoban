package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"grok-autoban/cpasdk/pluginapi"
)

const (
	exhaustedErrorCode        = "subscription:free-usage-exhausted"
	permissionDeniedErrorCode = "permission-denied"
	unauthorizedErrorCode     = "unauthorized"
)

type banEntry struct {
	AuthID      string    `json:"auth_id"`
	Provider    string    `json:"provider"`
	ErrorCode   string    `json:"error_code"`
	BannedAt    time.Time `json:"banned_at"`
	ResetAt     time.Time `json:"reset_at"`
	ResetSource string    `json:"reset_source"`
	TraceID     string    `json:"trace_id,omitempty"`
}

func detectBan(record pluginapi.UsageRecord, cfg pluginConfig, now time.Time) (banEntry, bool) {
	_ = cfg
	provider := normalizeProvider(record.Provider)
	if provider != "xai" || !record.Failed {
		return banEntry{}, false
	}

	authID := strings.TrimSpace(record.AuthID)
	if authID == "" {
		return banEntry{}, false
	}

	status := record.Failure.StatusCode
	errorCode, hasCode := parseErrorCode(record.Failure.Body)
	if status == http.StatusUnauthorized {
		if !hasCode {
			errorCode = unauthorizedErrorCode
		}
	} else if !hasCode {
		return banEntry{}, false
	}

	resetAt, resetSource, ok := resolveBanWindow(status, errorCode, now)
	if !ok {
		return banEntry{}, false
	}

	return banEntry{
		AuthID:      authID,
		Provider:    provider,
		ErrorCode:   errorCode,
		BannedAt:    now,
		ResetAt:     resetAt,
		ResetSource: resetSource,
		TraceID:     firstHeader(record.ResponseHeaders, "X-Request-Id"),
	}, true
}

func resolveBanWindow(status int, errorCode string, now time.Time) (time.Time, string, bool) {
	switch {
	case status == http.StatusForbidden && errorCode == permissionDeniedErrorCode:
		// Permission issues are not temporary quota windows. Keep the account out of
		// the pool until an operator unbans it manually.
		return now.AddDate(100, 0, 0), "manual_unban", true
	case status == http.StatusUnauthorized:
		// Auth failures usually mean expired or invalid credentials. Keep the account
		// out of the pool until an operator unbans it manually.
		return now.AddDate(100, 0, 0), "manual_unban", true
	default:
		// 429 free-usage-exhausted is left to CPA Manager Plus provider quota cooldown.
		return time.Time{}, "", false
	}
}

func normalizeProvider(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "xai", "x-ai", "grok":
		return "xai"
	default:
		return strings.ToLower(strings.TrimSpace(provider))
	}
}

func parseErrorCode(body string) (string, bool) {
	var payload struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		return "", false
	}
	payload.Code = strings.TrimSpace(payload.Code)
	return payload.Code, payload.Code != ""
}

func firstHeader(headers http.Header, name string) string {
	return strings.TrimSpace(headers.Get(name))
}
