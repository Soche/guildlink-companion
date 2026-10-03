// Package config stores the companion's settings in the user's config folder.
package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type Settings struct {
	ServerURL    string `json:"serverUrl"`
	Token        string `json:"token"`
	WowDir       string `json:"wowDir"`
	WriteSeed    bool   `json:"writeSeed"`
	OpenOnLaunch bool   `json:"openOnLaunch"`
	// Updates are on unless turned off, so older settings files keep them on.
	DisableAutoUpdate bool `json:"disableAutoUpdate"`
	// Game flavor folders (e.g. "_classic_beta_") to keep the addon
	// installed in; empty means pick automatically.
	AddonFlavors []string `json:"addonFlavors"`
}

func (s Settings) Ready() bool { return s.ServerURL != "" && s.Token != "" && s.WowDir != "" }

// Store is a settings file guarded for concurrent use.
type Store struct {
	path string
	mu   sync.RWMutex
	s    Settings
}

func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "GuildLink", "config.json"), nil
}

func Load(path string) (*Store, error) {
	st := &Store{path: path, s: Settings{WriteSeed: true, OpenOnLaunch: true}}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return st, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &st.s); err != nil {
		return nil, err
	}
	return st, nil
}

func (st *Store) Get() Settings {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.s
}

func (st *Store) Path() string { return st.path }

// Save normalises and writes the settings. The file holds the API token, so
// it is readable by the current user only.
func (st *Store) Save(s Settings) error {
	s.ServerURL = strings.TrimRight(strings.TrimSpace(s.ServerURL), "/")
	s.Token = strings.TrimSpace(s.Token)
	s.WowDir = strings.TrimSpace(s.WowDir)
	st.mu.Lock()
	defer st.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(st.path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(st.path, data, 0o600); err != nil {
		return err
	}
	st.s = s
	return nil
}
