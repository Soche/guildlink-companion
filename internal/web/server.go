// Package web serves the companion's settings and status page on localhost.
package web

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"html/template"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"time"

	"guildlink/companion/internal/api"
	"guildlink/companion/internal/autostart"
	"guildlink/companion/internal/config"
	"guildlink/companion/internal/i18n"
	"guildlink/companion/internal/syncer"
	"guildlink/companion/internal/update"
	"guildlink/companion/internal/wow"
)

// DefaultPort is tried first so the page's address stays the same between runs.
const DefaultPort = 47631

//go:embed index.html
var indexHTML string

var page = template.Must(template.New("index").Parse(indexHTML))

type Server struct {
	store   *config.Store
	syncer  *syncer.Syncer
	updater *update.Updater
	version string
	csrf    string
	port    int
	srv     *http.Server
	// The language the browser last asked for, for the tray while no
	// language has been picked on the page.
	browserLang atomic.Value
}

// Listen binds to localhost only; nothing here is reachable from the network.
func Listen(store *config.Store, s *syncer.Syncer, u *update.Updater, version string, port int) (*Server, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		ln, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return nil, err
		}
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	w := &Server{store: store, syncer: s, updater: u, version: version, csrf: hex.EncodeToString(b), port: ln.Addr().(*net.TCPAddr).Port}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", w.index)
	mux.HandleFunc("GET /status", w.status)
	mux.HandleFunc("POST /settings", w.saveSettings)
	mux.HandleFunc("POST /language", w.setLanguage)
	mux.HandleFunc("POST /sync", w.syncNow)
	mux.HandleFunc("POST /test", w.testConnection)
	mux.HandleFunc("GET /updates", w.updateStatus)
	mux.HandleFunc("POST /updates/check", w.updateCheck)
	mux.HandleFunc("POST /updates/apply", w.updateApply)
	w.srv = &http.Server{Handler: w.guard(mux), ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = w.srv.Serve(ln) }()
	return w, nil
}

// lang is the language to show: the one picked with the flags, else the
// browser's, else English.
func (w *Server) lang(r *http.Request) string {
	if r != nil {
		if l := i18n.FromAcceptLanguage(r.Header.Get("Accept-Language")); l != "" {
			w.browserLang.Store(l)
		}
	}
	return w.Lang()
}

// Lang is the language for what isn't the page, like the tray menu.
func (w *Server) Lang() string {
	if l := w.store.Get().Language; i18n.Supported(l) {
		return l
	}
	if l, ok := w.browserLang.Load().(string); ok && l != "" {
		return l
	}
	return i18n.Default
}

func (w *Server) URL() string { return "http://127.0.0.1:" + strconv.Itoa(w.port) + "/" }

func (w *Server) Close(ctx context.Context) error { return w.srv.Shutdown(ctx) }

// guard rejects requests that did not come from our own page: a wrong Host
// header (DNS rebinding) or a POST without the per-run CSRF token.
func (w *Server) guard(next http.Handler) http.Handler {
	hosts := map[string]bool{"127.0.0.1:" + strconv.Itoa(w.port): true, "localhost:" + strconv.Itoa(w.port): true}
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if !hosts[r.Host] {
			http.Error(rw, "forbidden", http.StatusForbidden)
			return
		}
		if r.Method == http.MethodPost {
			token := r.Header.Get("X-CSRF-Token")
			if token == "" {
				token = r.FormValue("csrf")
			}
			if subtle.ConstantTimeCompare([]byte(token), []byte(w.csrf)) != 1 {
				http.Error(rw, "forbidden", http.StatusForbidden)
				return
			}
		}
		rw.Header().Set("X-Frame-Options", "DENY")
		rw.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'unsafe-inline'; script-src 'unsafe-inline'; frame-ancestors 'none'")
		next.ServeHTTP(rw, r)
	})
}

type pageData struct {
	Lang      string
	T         map[string]string // texts for the template and the script
	TokenHint template.HTML
	Languages []langLink

	Settings   config.Settings
	TokenSet   bool
	CSRF       string
	Version    string
	Detected   []string
	ConfigPath string
	Saved      bool
	SaveError  string
	AutoUpdate bool
	Flavors    []flavorChoice
	// Start at login: whether it's on, and whether this build can set it
	// (a development build runs from a temporary folder).
	StartAtLogin    bool
	CanStartAtLogin bool
}

type langLink struct {
	Code, Name string
	Flag       template.HTML
	Current    bool
}

type flavorChoice struct {
	Name    string
	Checked bool
}

func (w *Server) render(rw http.ResponseWriter, r *http.Request, saved bool, saveErr string) {
	lang := w.lang(r)
	var langs []langLink
	for _, l := range i18n.Languages {
		langs = append(langs, langLink{Code: l.Code, Name: l.Name, Flag: template.HTML(l.Flag), Current: l.Code == lang})
	}
	s := w.store.Get()
	token := s.Token
	s.Token = ""
	rw.Header().Set("Content-Type", "text/html; charset=utf-8")
	targets := map[string]bool{}
	for _, f := range update.TargetFlavors(s) {
		targets[f] = true
	}
	var flavors []flavorChoice
	if s.WowDir != "" {
		for _, f := range wow.Flavors(s.WowDir) {
			flavors = append(flavors, flavorChoice{Name: f, Checked: targets[f]})
		}
	}
	startAtLogin, _ := autostart.Enabled()
	rw.Header().Set("Content-Language", lang)
	_ = page.Execute(rw, pageData{
		Lang: lang, T: i18n.Strings(lang), TokenHint: template.HTML(i18n.T(lang, "tokenHint")), Languages: langs,
		Settings: s, TokenSet: token != "", CSRF: w.csrf, Version: w.version,
		Detected: wow.DetectInstalls(), ConfigPath: w.store.Path(), Saved: saved, SaveError: saveErr,
		AutoUpdate: !s.DisableAutoUpdate, Flavors: flavors,
		StartAtLogin: startAtLogin, CanStartAtLogin: w.version != "dev",
	})
}

func (w *Server) index(rw http.ResponseWriter, r *http.Request) {
	w.render(rw, r, r.URL.Query().Has("saved"), "")
}

// setLanguage keeps the language picked with the flags.
func (w *Server) setLanguage(rw http.ResponseWriter, r *http.Request) {
	s := w.store.Get()
	if l := r.FormValue("lang"); i18n.Supported(l) {
		s.Language = l
		_ = w.store.Save(s)
	}
	http.Redirect(rw, r, "/", http.StatusSeeOther)
}

func (w *Server) status(rw http.ResponseWriter, _ *http.Request) {
	rw.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(rw).Encode(w.syncer.Status())
}

func (w *Server) saveSettings(rw http.ResponseWriter, r *http.Request) {
	s := w.store.Get()
	s.ServerURL = r.FormValue("serverUrl")
	// An empty token field keeps the saved one; the page never shows it.
	if t := r.FormValue("token"); t != "" {
		s.Token = t
	}
	s.WowDir = wow.ExpandPath(r.FormValue("wowDir"))
	s.WriteSeed = r.FormValue("writeSeed") == "on"
	s.OpenOnLaunch = r.FormValue("openOnLaunch") == "on"
	s.DisableAutoUpdate = r.FormValue("autoUpdate") != "on"
	s.DisableCalendar = r.FormValue("calendar") != "on"
	// None ticked means "pick automatically".
	s.AddonFlavors = r.Form["addonFlavor"]

	if s.WowDir != "" {
		if info, err := os.Stat(s.WowDir); err != nil || !info.IsDir() {
			w.render(rw, r, false, i18n.T(w.lang(r), "errNoFolder"))
			return
		}
		// Accept the flavor folder (e.g. _classic_beta_) or its WTF folder too.
		for range 2 {
			if base := filepath.Base(s.WowDir); len(base) > 1 && (base[0] == '_' || base == "WTF") {
				s.WowDir = filepath.Dir(s.WowDir)
			}
		}
	}
	if err := w.store.Save(s); err != nil {
		w.render(rw, r, false, i18n.T(w.lang(r), "errSave", "err", err.Error()))
		return
	}
	if w.version != "dev" {
		if err := setStartAtLogin(r.FormValue("startAtLogin") == "on"); err != nil {
			w.render(rw, r, false, i18n.T(w.lang(r), "errStartAtLogin", "err", err.Error()))
			return
		}
	}
	w.syncer.SyncNow()
	w.updater.CheckNow(false)
	http.Redirect(rw, r, "/?saved", http.StatusSeeOther)
}

func setStartAtLogin(on bool) error {
	if !on {
		return autostart.Disable()
	}
	exe, err := update.ExecutablePath()
	if err != nil {
		return err
	}
	return autostart.Enable(exe)
}

func (w *Server) syncNow(rw http.ResponseWriter, _ *http.Request) {
	w.syncer.SyncNow()
	rw.WriteHeader(http.StatusNoContent)
}

func (w *Server) testConnection(rw http.ResponseWriter, r *http.Request) {
	s := w.store.Get()
	rw.Header().Set("Content-Type", "application/json")
	if s.ServerURL == "" || s.Token == "" {
		_ = json.NewEncoder(rw).Encode(map[string]string{"error": i18n.T(w.lang(r), "errSaveFirst")})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	me, err := api.New(s.ServerURL, s.Token, w.version).Me(ctx)
	if err != nil {
		msg := err.Error()
		var apiErr *api.Error
		if errors.As(err, &apiErr) && apiErr.Status == http.StatusUnauthorized {
			msg = i18n.T(w.lang(r), "errToken")
		}
		_ = json.NewEncoder(rw).Encode(map[string]string{"error": msg})
		return
	}
	_ = json.NewEncoder(rw).Encode(me)
}

func (w *Server) updateStatus(rw http.ResponseWriter, _ *http.Request) {
	st := w.updater.Status()
	rw.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(rw).Encode(struct {
		update.Status
		Targets []string `json:"targets"`
	}{st, update.TargetFlavors(w.store.Get())})
}

func (w *Server) updateCheck(rw http.ResponseWriter, _ *http.Request) {
	w.updater.CheckNow(false)
	rw.WriteHeader(http.StatusNoContent)
}

func (w *Server) updateApply(rw http.ResponseWriter, _ *http.Request) {
	w.updater.CheckNow(true)
	rw.WriteHeader(http.StatusNoContent)
}
