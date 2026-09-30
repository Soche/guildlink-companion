// Package syncer watches the addon's SavedVariables files and uploads them
// to the bot whenever the game writes a new version.
package syncer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"sort"
	"sync"
	"time"

	"guildlink/companion/internal/addon"
	"guildlink/companion/internal/api"
	"guildlink/companion/internal/config"
	"guildlink/companion/internal/luasv"
	"guildlink/companion/internal/wow"
)

const (
	pollInterval = 5 * time.Second
	retryDelay   = time.Minute
)

type FileStatus struct {
	wow.SavedVariables
	Characters  int             `json:"characters"`
	Recipes     int             `json:"recipes"`
	DiscordID   string          `json:"discordId"`
	LastSync    time.Time       `json:"lastSync"`
	LastError   string          `json:"lastError"`
	Result      *api.SyncResult `json:"result"`
	SeedWritten time.Time       `json:"seedWritten"`

	uploadedHash string
	retryAt      time.Time
}

type Status struct {
	Problem   string       `json:"problem"`
	Files     []FileStatus `json:"files"`
	LastCheck time.Time    `json:"lastCheck"`
}

type Syncer struct {
	store   *config.Store
	version string
	trigger chan bool

	mu      sync.Mutex
	files   map[string]*FileStatus
	problem string
	checked time.Time
}

func New(store *config.Store, version string) *Syncer {
	return &Syncer{store: store, version: version, trigger: make(chan bool, 1), files: map[string]*FileStatus{}}
}

// Run polls until ctx ends. Polling instead of file events works the same on
// every OS and inside Wine prefixes.
func (s *Syncer) Run(ctx context.Context) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	s.check(ctx, false)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.check(ctx, false)
		case force := <-s.trigger:
			s.check(ctx, force)
		}
	}
}

// SyncNow re-uploads every file, even unchanged ones.
func (s *Syncer) SyncNow() {
	select {
	case s.trigger <- true:
	default:
	}
}

func (s *Syncer) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := Status{Problem: s.problem, LastCheck: s.checked}
	for _, f := range s.files {
		st.Files = append(st.Files, *f)
	}
	sort.Slice(st.Files, func(i, j int) bool { return st.Files[i].Path < st.Files[j].Path })
	return st
}

func (s *Syncer) setProblem(p string) {
	s.mu.Lock()
	s.problem = p
	s.checked = time.Now()
	s.mu.Unlock()
}

func (s *Syncer) check(ctx context.Context, force bool) {
	settings := s.store.Get()
	if settings.WowDir == "" {
		s.setProblem("Choose your World of Warcraft folder in the settings below.")
		return
	}
	found, err := wow.FindSavedVariables(settings.WowDir)
	if err != nil || len(found) == 0 {
		s.setProblem(fmt.Sprintf("No GuildLink saved data under %s yet. Install the addon, log in to a character, then log out or type /reload.", settings.WowDir))
		s.mu.Lock()
		s.files = map[string]*FileStatus{}
		s.mu.Unlock()
		return
	}
	switch {
	case settings.ServerURL == "" || settings.Token == "":
		s.setProblem("Add the server address and token from the bot's /link command.")
	default:
		s.setProblem("")
	}

	s.mu.Lock()
	current := map[string]*FileStatus{}
	for _, sv := range found {
		f := s.files[sv.Path]
		if f == nil {
			f = &FileStatus{}
		}
		changed := f.ModTime != sv.ModTime || f.Size != sv.Size
		f.SavedVariables = sv
		if changed {
			f.uploadedHash = ""
			f.retryAt = time.Time{}
		}
		current[sv.Path] = f
	}
	s.files = current
	s.mu.Unlock()

	newest := map[string]*FileStatus{}
	for _, f := range current {
		if n := newest[f.FlavorKey()]; n == nil || f.ModTime.After(n.ModTime) {
			newest[f.FlavorKey()] = f
		}
	}

	for _, f := range current {
		content, err := os.ReadFile(f.Path)
		if err != nil {
			s.update(f, func() { f.LastError = err.Error() })
			continue
		}
		if settings.WriteSeed && newest[f.FlavorKey()] == f {
			if wrote, err := f.WriteSeed(content); err != nil {
				log.Printf("writing Seed.lua for %s: %v", f.Flavor, err)
			} else if wrote {
				s.update(f, func() { f.SeedWritten = time.Now() })
			}
		}
		s.upload(ctx, settings, f, content, force)
	}
}

func (s *Syncer) update(f *FileStatus, fn func()) {
	s.mu.Lock()
	fn()
	s.mu.Unlock()
}

func (s *Syncer) upload(ctx context.Context, settings config.Settings, f *FileStatus, content []byte, force bool) {
	sum := sha256.Sum256(content)
	hash := hex.EncodeToString(sum[:])
	s.mu.Lock()
	skip := !force && (f.uploadedHash == hash || time.Now().Before(f.retryAt))
	s.mu.Unlock()
	if skip {
		return
	}

	vars, err := luasv.Parse(string(content))
	if err != nil {
		s.update(f, func() { f.LastError = "could not read the file: " + err.Error() })
		return
	}
	payload, err := addon.FromSavedVariables(vars)
	if err != nil {
		s.update(f, func() { f.LastError = err.Error() })
		return
	}
	recipes := 0
	for _, c := range payload.Characters {
		for _, p := range c.Professions {
			recipes += len(p.Recipes)
		}
	}
	s.update(f, func() {
		f.Characters = len(payload.Characters)
		f.Recipes = recipes
		f.DiscordID = ""
		if payload.DiscordID != nil {
			f.DiscordID = *payload.DiscordID
		}
	})
	if settings.ServerURL == "" || settings.Token == "" {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	res, err := api.New(settings.ServerURL, settings.Token, s.version).Sync(ctx, payload)
	s.update(f, func() {
		if err != nil {
			f.LastError = err.Error()
			f.retryAt = time.Now().Add(retryDelay)
			return
		}
		f.LastError = ""
		f.Result = res
		f.LastSync = time.Now()
		f.uploadedHash = hash
	})
	if err != nil {
		log.Printf("upload of %s failed: %v", f.Path, err)
	} else {
		log.Printf("uploaded %s: %d characters, %d recipes stored", f.Path, res.Accepted, res.RecipesStored)
	}
}
