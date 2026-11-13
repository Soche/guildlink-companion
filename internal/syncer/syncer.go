// Package syncer watches the addon's SavedVariables files and uploads them
// to the bot whenever the game writes a new version.
package syncer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"guildlink/companion/internal/addon"
	"guildlink/companion/internal/api"
	"guildlink/companion/internal/calendar"
	"guildlink/companion/internal/config"
	"guildlink/companion/internal/i18n"
	"guildlink/companion/internal/luasv"
	"guildlink/companion/internal/wow"
)

const (
	pollInterval = 5 * time.Second
	retryDelay   = time.Minute
	// How often the upcoming events are fetched for the game's calendar.
	calendarInterval = 10 * time.Minute
)

type FileStatus struct {
	wow.SavedVariables
	Characters  int             `json:"characters"`
	Recipes     int             `json:"recipes"`
	Rosters     int             `json:"rosters"` // guild rosters in the file
	Banks       int             `json:"banks"`   // guild bank snapshots in the file
	DiscordID   string          `json:"discordId"`
	LastSync    time.Time       `json:"lastSync"`
	LastError   string          `json:"lastError"`
	Result      *api.SyncResult `json:"result"`
	SeedWritten time.Time       `json:"seedWritten"`

	uploadedHash string
	retryAt      time.Time
}

type Status struct {
	Problem string `json:"problem"`
	// ProblemCode names Problem for the page to show in its language
	// (an i18n key), with ProblemArgs filling its {placeholders}.
	ProblemCode string            `json:"problemCode,omitempty"`
	ProblemArgs map[string]string `json:"problemArgs,omitempty"`
	Files     []FileStatus   `json:"files"`
	LastCheck time.Time      `json:"lastCheck"`
	Calendar  CalendarStatus `json:"calendar"`
}

// CalendarStatus is how the last fetch of events for the game's calendar went.
type CalendarStatus struct {
	Off       bool      `json:"off"`
	Events    int       `json:"events"`
	Fetched   time.Time `json:"fetched"`
	Error     string    `json:"error"`
	ErrorCode string    `json:"errorCode,omitempty"` // an i18n key for Error, when it's one of ours
	WrittenTo []string  `json:"writtenTo"`           // flavors with the addon that got the file
}

type Syncer struct {
	store   *config.Store
	version string
	trigger chan bool

	mu      sync.Mutex
	files   map[string]*FileStatus
	problem     string
	problemCode string
	problemArgs map[string]string
	checked     time.Time
	cal     CalendarStatus
	calNext time.Time
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
	st := Status{Problem: s.problem, ProblemCode: s.problemCode, ProblemArgs: s.problemArgs, LastCheck: s.checked, Calendar: s.cal}
	for _, f := range s.files {
		st.Files = append(st.Files, *f)
	}
	sort.Slice(st.Files, func(i, j int) bool { return st.Files[i].Path < st.Files[j].Path })
	return st
}

// setProblem sets what stops uploads (code is the i18n key, args fill its
// placeholders), or clears it with an empty code.
func (s *Syncer) setProblem(code string, args ...string) {
	s.mu.Lock()
	s.problemCode = code
	s.problem = ""
	s.problemArgs = nil
	if code != "" {
		s.problem = i18n.T(i18n.Default, code, args...)
		s.problemArgs = map[string]string{}
		for i := 0; i+1 < len(args); i += 2 {
			s.problemArgs[args[i]] = args[i+1]
		}
	}
	s.checked = time.Now()
	s.mu.Unlock()
}

func (s *Syncer) check(ctx context.Context, force bool) {
	settings := s.store.Get()
	if settings.WowDir == "" {
		s.setProblem("problemWowDir")
		return
	}
	s.refreshCalendar(ctx, settings, force)
	found, err := wow.FindSavedVariables(settings.WowDir)
	if err != nil || len(found) == 0 {
		s.setProblem("problemNoData", "dir", settings.WowDir)
		s.mu.Lock()
		s.files = map[string]*FileStatus{}
		s.mu.Unlock()
		return
	}
	switch {
	case settings.ServerURL == "" || settings.Token == "":
		s.setProblem("problemNoServer")
	default:
		s.setProblem("")
	}

	// A retired beta's data isn't uploaded, but its Discord ID can start the
	// release client's addon data.
	var active []wow.SavedVariables
	for _, sv := range found {
		if !wow.Retired(settings.WowDir, sv.Flavor) {
			active = append(active, sv)
		} else if settings.WriteSeed {
			s.carryDiscordID(settings.WowDir, sv)
		}
	}
	found = active

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

// refreshCalendar fetches the upcoming events every calendarInterval (or
// when forced) and writes them where the addon reads them at login.
func (s *Syncer) refreshCalendar(ctx context.Context, settings config.Settings, force bool) {
	if settings.DisableCalendar {
		s.mu.Lock()
		s.cal = CalendarStatus{Off: true}
		s.mu.Unlock()
		return
	}
	s.mu.Lock()
	due := force || !time.Now().Before(s.calNext)
	s.mu.Unlock()
	if !due || settings.ServerURL == "" || settings.Token == "" {
		return
	}
	events, err := api.New(settings.ServerURL, settings.Token, s.version).Events(ctx)
	st := CalendarStatus{Fetched: time.Now()}
	next := time.Now().Add(calendarInterval)
	var apiErr *api.Error
	switch {
	case errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound:
		st.Error = i18n.T(i18n.Default, "calBotOld")
		st.ErrorCode = "calBotOld"
	case err != nil:
		st.Error = err.Error()
		next = time.Now().Add(retryDelay)
	default:
		st.Events = len(events)
		written, werr := calendar.Write(settings.WowDir, calendar.Render(events))
		if werr != nil {
			st.Error = "writing Events.lua: " + werr.Error()
			next = time.Now().Add(retryDelay)
		}
		if len(written) > 0 {
			log.Printf("wrote %d upcoming events for %s", len(events), strings.Join(written, ", "))
		}
		for _, f := range wow.Flavors(settings.WowDir) {
			if _, err := os.Stat(filepath.Join(wow.AddOnsDir(settings.WowDir, f), wow.AddonName)); err == nil {
				st.WrittenTo = append(st.WrittenTo, f)
			}
		}
	}
	s.mu.Lock()
	s.cal = st
	s.calNext = next
	s.mu.Unlock()
}

// carryDiscordID seeds the Forever release client's addon with the Discord
// ID from a retired beta's saved data, until that client has data of its own.
func (s *Syncer) carryDiscordID(installDir string, sv wow.SavedVariables) {
	if !wow.NeedsSeed(installDir, wow.ForeverFlavor) {
		return
	}
	content, err := os.ReadFile(sv.Path)
	if err != nil {
		return
	}
	vars, err := luasv.Parse(string(content))
	if err != nil {
		return
	}
	payload, err := addon.FromSavedVariables(vars)
	if err != nil || payload.DiscordID == nil {
		return
	}
	if wrote, err := wow.SeedDiscordID(installDir, wow.ForeverFlavor, *payload.DiscordID); err != nil {
		log.Printf("carrying the Discord ID over to %s: %v", wow.ForeverFlavor, err)
	} else if wrote {
		log.Printf("carried the Discord ID over from %s to %s", sv.Flavor, wow.ForeverFlavor)
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
	if payload.AccountBank != nil {
		// Identifies the WoW account without sending its folder name.
		sum := sha256.Sum256([]byte(strings.ToLower(f.Account)))
		payload.AccountBank.AccountKey = hex.EncodeToString(sum[:8])
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
		f.Rosters = len(payload.GuildRosters)
		f.Banks = len(payload.GuildBanks)
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
