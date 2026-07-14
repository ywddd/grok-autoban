package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type banStore struct {
	mu   sync.RWMutex
	bans map[string]banEntry
}

func newBanStore() *banStore {
	return &banStore{bans: make(map[string]banEntry)}
}

func (s *banStore) Set(entry banEntry) {
	if entry.AuthID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.bans[entry.AuthID]; ok && existing.ResetAt.After(entry.ResetAt) {
		return
	}
	s.bans[entry.AuthID] = entry
}

func (s *banStore) Get(authID string) (banEntry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry, ok := s.bans[authID]
	return entry, ok
}

func (s *banStore) Delete(authID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.bans[authID]; !ok {
		return false
	}
	delete(s.bans, authID)
	return true
}

func (s *banStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bans = make(map[string]banEntry)
}

func (s *banStore) ClearExpired(now time.Time) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	expired := make([]string, 0)
	for authID, entry := range s.bans {
		if !entry.ResetAt.After(now) {
			expired = append(expired, authID)
			delete(s.bans, authID)
		}
	}
	return expired
}

func (s *banStore) Expired(now time.Time) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	expired := make([]string, 0)
	for authID, entry := range s.bans {
		if !entry.ResetAt.After(now) {
			expired = append(expired, authID)
		}
	}
	return expired
}

func (s *banStore) List(now time.Time) []banEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]banEntry, 0, len(s.bans))
	for _, entry := range s.bans {
		if entry.ResetAt.After(now) {
			out = append(out, entry)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ResetAt.Before(out[j].ResetAt)
	})
	return out
}

func (s *banStore) Save(path string) error {
	if path == "" {
		return nil
	}
	entries := s.List(time.Time{})
	raw, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".grok-autoban-*.tmp")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err := temp.Chmod(0600); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.Write(raw); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempName, path)
}

func (s *banStore) Load(path string, now time.Time) error {
	if path == "" {
		return nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var entries []banEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		s.Clear()
		return err
	}
	s.Clear()
	for _, entry := range entries {
		if entry.AuthID != "" && entry.ResetAt.After(now) {
			s.Set(entry)
		}
	}
	return nil
}
