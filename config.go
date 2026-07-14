package main

import (
	"encoding/json"
	"log/slog"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

type pluginConfig struct {
	FallbackHours int
	PersistState  bool
	StateFile     string
	LogMatches    bool
}

type configYAML struct {
	FallbackHours int    `yaml:"fallback_hours"`
	PersistState  *bool  `yaml:"persist_state"`
	StateFile     string `yaml:"state_file"`
	LogMatches    *bool  `yaml:"log_matches"`
}

type lifecycleRequest struct {
	SchemaVersion uint32 `json:"schema_version"`
	ConfigYAML    []byte `json:"config_yaml"`
}

var currentConfig atomic.Value

func init() {
	currentConfig.Store(defaultPluginConfig())
}

func defaultPluginConfig() pluginConfig {
	return pluginConfig{
		FallbackHours: 24,
		PersistState:  true,
		LogMatches:    true,
	}
}

func decodeConfig(raw []byte) (pluginConfig, error) {
	cfg := defaultPluginConfig()
	if len(raw) == 0 {
		return cfg, nil
	}

	decoded := configYAML{}
	for _, line := range strings.Split(string(raw), "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
		switch key {
		case "fallback_hours":
			decoded.FallbackHours, _ = strconv.Atoi(value)
		case "persist_state":
			if parsed, err := strconv.ParseBool(value); err == nil {
				decoded.PersistState = &parsed
			}
		case "state_file":
			decoded.StateFile = value
		case "log_matches":
			if parsed, err := strconv.ParseBool(value); err == nil {
				decoded.LogMatches = &parsed
			}
		}
	}
	if decoded.FallbackHours >= 1 && decoded.FallbackHours <= 168 {
		cfg.FallbackHours = decoded.FallbackHours
	}
	if decoded.PersistState != nil {
		cfg.PersistState = *decoded.PersistState
	}
	cfg.StateFile = strings.TrimSpace(decoded.StateFile)
	if decoded.LogMatches != nil {
		cfg.LogMatches = *decoded.LogMatches
	}
	return cfg, nil
}

func configure(raw []byte) error {
	var req lifecycleRequest
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &req); err != nil {
			return err
		}
	}
	cfg, err := decodeConfig(req.ConfigYAML)
	if err != nil {
		return err
	}
	currentConfig.Store(cfg)
	if cfg.PersistState && cfg.StateFile != "" {
		if err := activeStore.Load(cfg.StateFile, time.Now()); err != nil {
			slog.Warn("grok-autoban: failed to load state", "error", err)
		}
	}
	// Free-usage 429 is owned by CPA Manager Plus. Drop legacy quota bans so this
	// plugin only tracks 401/403 permanent disables going forward.
	if purged := purgeLegacyQuotaBans(); purged > 0 {
		slog.Info("grok-autoban: purged legacy free-usage-exhausted bans", "count", purged)
		if cfg.PersistState && cfg.StateFile != "" {
			if err := activeStore.Save(cfg.StateFile); err != nil {
				slog.Warn("grok-autoban: failed to save state after purging legacy bans", "error", err)
			}
		}
	}
	return nil
}

func purgeLegacyQuotaBans() int {
	items := activeStore.List(time.Time{})
	purged := 0
	now := time.Now()
	for _, entry := range items {
		if entry.ErrorCode != exhaustedErrorCode {
			continue
		}
		// Best-effort re-enable so Manager Plus can own future 429 cooldowns.
		if err := enableAuthInCPA(entry.AuthID, ""); err != nil {
			// Startup may race the management API. Keep a past ResetAt so the next
			// scheduler pick retries enableAuthInCPA via the expired-ban path.
			slog.Warn("grok-autoban: failed to re-enable legacy quota ban; will retry on schedule", "auth_id", entry.AuthID, "error", err)
			entry.ResetAt = now.Add(-time.Second)
			entry.ResetSource = "legacy_quota_handoff"
			activeStore.Set(entry)
			continue
		}
		if activeStore.Delete(entry.AuthID) {
			purged++
		}
	}
	return purged
}

func loadedConfig() pluginConfig {
	if cfg, ok := currentConfig.Load().(pluginConfig); ok {
		return cfg
	}
	return defaultPluginConfig()
}
