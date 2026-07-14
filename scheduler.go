package main

import (
	"encoding/json"
	"log/slog"
	"time"

	"grok-autoban/cpasdk/pluginabi"
	"grok-autoban/cpasdk/pluginapi"
)

var activeStore = newBanStore()

func handleUsage(raw []byte) ([]byte, error) {
	var record pluginapi.UsageRecord
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &record); err != nil {
			return okEnvelope(map[string]any{})
		}
	}
	if _, err := handleUsageRecord(record, loadedConfig(), time.Now()); err != nil {
		return nil, err
	}
	return okEnvelope(map[string]any{})
}

func handleUsageRecord(record pluginapi.UsageRecord, cfg pluginConfig, now time.Time) (banEntry, error) {
	entry, ok := detectBan(record, cfg, now)
	if !ok {
		return banEntry{}, nil
	}
	activeStore.Set(entry)
	if errDisable := disableAuthInCPA(entry.AuthID); errDisable != nil {
		slog.Warn("grok-autoban: failed to disable auth in CPA", "auth_id", entry.AuthID, "error", errDisable)
	}
	if cfg.PersistState && cfg.StateFile != "" {
		if err := activeStore.Save(cfg.StateFile); err != nil {
			slog.Warn("grok-autoban: failed to save state", "error", err)
		}
	}
	return entry, nil
}

func pickCandidate(req pluginapi.SchedulerPickRequest, store *banStore, now time.Time) pluginapi.SchedulerPickResponse {
	for _, authID := range store.Expired(now) {
		if errEnable := enableAuthInCPA(authID, ""); errEnable != nil {
			slog.Warn("grok-autoban: failed to re-enable expired auth in CPA", "auth_id", authID, "error", errEnable)
			continue
		}
		store.Delete(authID)
	}
	available := make([]pluginapi.SchedulerAuthCandidate, 0, len(req.Candidates))
	for _, candidate := range req.Candidates {
		if normalizeProvider(candidate.Provider) == "xai" {
			if _, banned := store.Get(candidate.ID); banned {
				continue
			}
		}
		available = append(available, candidate)
	}
	if len(available) == 0 {
		return pluginapi.SchedulerPickResponse{Handled: false}
	}
	if len(available) == len(req.Candidates) {
		return pluginapi.SchedulerPickResponse{
			Handled:         true,
			DelegateBuiltin: pluginapi.SchedulerBuiltinRoundRobin,
		}
	}
	chosen := available[0]
	for _, candidate := range available[1:] {
		if candidate.Priority > chosen.Priority {
			chosen = candidate
		}
	}
	return pluginapi.SchedulerPickResponse{Handled: true, AuthID: chosen.ID}
}

func handleSchedulerPick(raw []byte) ([]byte, error) {
	var req pluginapi.SchedulerPickRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, err
	}
	return okEnvelope(pickCandidate(req, activeStore, time.Now()))
}

func handlePluginMethod(method string, request []byte) ([]byte, error) {
	switch method {
	case pluginabi.MethodUsageHandle:
		return handleUsage(request)
	case pluginabi.MethodSchedulerPick:
		return handleSchedulerPick(request)
	default:
		return nil, nil
	}
}
