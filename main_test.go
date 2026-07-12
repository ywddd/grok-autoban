package main

import (
	"encoding/json"
	"testing"
)

func TestPluginRegistration(t *testing.T) {
	raw, err := handleMethod("plugin.register", []byte(`{"schema_version":1,"config_yaml":"ZmFsbGJhY2tfaG91cnM6IDI0Cg=="}`))
	if err != nil {
		t.Fatalf("handleMethod() error = %v", err)
	}

	var env struct {
		OK     bool            `json:"ok"`
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	if !env.OK {
		t.Fatal("registration envelope ok = false")
	}

	var got struct {
		SchemaVersion uint32 `json:"schema_version"`
		Metadata      struct {
			Name         string `json:"Name"`
			Version      string `json:"Version"`
			ConfigFields []struct {
				Name string `json:"Name"`
			} `json:"ConfigFields"`
		} `json:"metadata"`
		Capabilities struct {
			UsagePlugin   bool `json:"usage_plugin"`
			Scheduler     bool `json:"scheduler"`
			ManagementAPI bool `json:"management_api"`
		} `json:"capabilities"`
	}
	if err := json.Unmarshal(env.Result, &got); err != nil {
		t.Fatalf("decode registration: %v", err)
	}

	if got.SchemaVersion != 1 {
		t.Fatalf("schema version = %d, want 1", got.SchemaVersion)
	}
	if got.Metadata.Name != "Grok 429 Auto Ban" {
		t.Fatalf("name = %q", got.Metadata.Name)
	}
	if got.Metadata.Version != "0.1.2" {
		t.Fatalf("version = %q", got.Metadata.Version)
	}
	if !got.Capabilities.UsagePlugin || !got.Capabilities.Scheduler || !got.Capabilities.ManagementAPI {
		t.Fatalf("capabilities = %#v", got.Capabilities)
	}

	wantFields := map[string]bool{
		"fallback_hours": false,
		"persist_state":  false,
		"state_file":     false,
		"log_matches":    false,
	}
	for _, field := range got.Metadata.ConfigFields {
		if _, ok := wantFields[field.Name]; ok {
			wantFields[field.Name] = true
		}
	}
	for name, found := range wantFields {
		if !found {
			t.Errorf("missing config field %q", name)
		}
	}
}
