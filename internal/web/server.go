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
	"time"

	"guildlink/companion/internal/api"
	"guildlink/companion/internal/config"
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
	mux.HandleFunc("POST /sync", w.syncNow)
	mux.HandleFunc("POST /test", w.testConnection)
	mux.HandleFunc("GET /updates", w.updateStatus)
	mux.HandleFunc("POST /updates/check", w.updateCheck)
	mux.HandleFunc("POST /updates/apply", w.updateApply)
	w.srv = &http.Server{Handler: w.guard(mux), ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = w.srv.Serve(ln) }()
	return w, nil
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
}

type flavorChoice struct {
	Name    string
	Checked bool
}

func (w *Server) render(rw http.ResponseWriter, saved bool, saveErr string) {
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
	_ = page.Execute(rw, pageData{
		Settings: s, TokenSet: token != "", CSRF: w.csrf, Version: w.version,
		Detected: wow.DetectInstalls(), ConfigPath: w.store.Path(), Saved: saved, SaveError: saveErr,
		AutoUpdate: !s.DisableAutoUpdate, Flavors: flavors,
	})
}

func (w *Server) index(rw http.ResponseWriter, r *http.Request) {
	w.render(rw, r.URL.Query().Has("saved"), "")
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
	s.WowDir = r.FormValue("wowDir")
	s.WriteSeed = r.FormValue("writeSeed") == "on"
	s.OpenOnLaunch = r.FormValue("openOnLaunch") == "on"
	s.DisableAutoUpdate = r.FormValue("autoUpdate") != "on"
	// None ticked means "pick automatically".
	s.AddonFlavors = r.Form["addonFlavor"]

	if s.WowDir != "" {
		if info, err := os.Stat(s.WowDir); err != nil || !info.IsDir() {
			w.render(rw, false, "That World of Warcraft folder does not exist.")
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
		w.render(rw, false, "Could not save settings: "+err.Error())
		return
	}
	w.syncer.SyncNow()
	w.updater.CheckNow(false)
	http.Redirect(rw, r, "/?saved", http.StatusSeeOther)
}

func (w *Server) syncNow(rw http.ResponseWriter, _ *http.Request) {
	w.syncer.SyncNow()
	rw.WriteHeader(http.StatusNoContent)
}

func (w *Server) testConnection(rw http.ResponseWriter, r *http.Request) {
	s := w.store.Get()
	rw.Header().Set("Content-Type", "application/json")
	if s.ServerURL == "" || s.Token == "" {
		_ = json.NewEncoder(rw).Encode(map[string]string{"error": "Save the server address and token first."})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	me, err := api.New(s.ServerURL, s.Token, w.version).Me(ctx)
	if err != nil {
		msg := err.Error()
		var apiErr *api.Error
		if errors.As(err, &apiErr) && apiErr.Status == http.StatusUnauthorized {
			msg = "The server rejected the token. Run /link regenerate:True in Discord for a new one."
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
