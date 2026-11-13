package web

import (
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"guildlink/companion/internal/config"
	"guildlink/companion/internal/syncer"
	"guildlink/companion/internal/update"
)

func start(t *testing.T) (*Server, *config.Store) {
	t.Helper()
	store, err := config.Load(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	srv, err := Listen(store, syncer.New(store, "dev"), update.New(store, "dev", "", func() {}), "dev", 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close(t.Context()) })
	return srv, store
}

func get(t *testing.T, srv *Server, acceptLanguage string) string {
	t.Helper()
	req, _ := http.NewRequest("GET", srv.URL(), nil)
	if acceptLanguage != "" {
		req.Header.Set("Accept-Language", acceptLanguage)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return string(b)
}

func TestPageFollowsTheBrowserUntilAFlagIsPicked(t *testing.T) {
	srv, store := start(t)
	if page := get(t, srv, ""); !strings.Contains(page, `<html lang="en">`) || !strings.Contains(page, ">Sync now<") {
		t.Fatal("English by default")
	}
	page := get(t, srv, "sv-SE,sv;q=0.9,en;q=0.8")
	for _, want := range []string{`<html lang="sv">`, ">Synka nu<", ">Spara<", "Skriv <code>/link</code>", `const LANG = "sv"`, `"syncNow":"Synka nu"`} {
		if !strings.Contains(page, want) {
			t.Errorf("Swedish page lacks %q", want)
		}
	}
	if regexp.MustCompile(`\{\{|\{\.`).MatchString(page) {
		t.Error("unfilled template")
	}
	if srv.Lang() != "sv" {
		t.Errorf("tray follows the browser: got %q", srv.Lang())
	}

	// Picking English with the flag sticks, whatever the browser says.
	csrf := regexp.MustCompile(`name="csrf" value="([0-9a-f]+)"`).FindStringSubmatch(page)[1]
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.PostForm(srv.URL()+"language", url.Values{"csrf": {csrf}, "lang": {"en"}})
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusSeeOther || store.Get().Language != "en" {
		t.Fatalf("status %d, language %q", res.StatusCode, store.Get().Language)
	}
	if page := get(t, srv, "sv-SE"); !strings.Contains(page, `<html lang="en">`) {
		t.Error("the picked language should win over the browser's")
	}
	if srv.Lang() != "en" {
		t.Errorf("tray: got %q", srv.Lang())
	}

	// Without the page's token, nothing changes.
	res, _ = client.PostForm(srv.URL()+"language", url.Values{"lang": {"sv"}})
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden || store.Get().Language != "en" {
		t.Errorf("no csrf: status %d, language %q", res.StatusCode, store.Get().Language)
	}
}

func TestServerMessagesAreTranslated(t *testing.T) {
	srv, store := start(t)
	s := store.Get()
	s.Language = "sv"
	_ = store.Save(s)
	page := get(t, srv, "")
	csrf := regexp.MustCompile(`name="csrf" value="([0-9a-f]+)"`).FindStringSubmatch(page)[1]
	req, _ := http.NewRequest("POST", srv.URL()+"test", nil)
	req.Header.Set("X-CSRF-Token", csrf)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if !strings.Contains(string(b), "Spara serveradressen och nyckeln först.") {
		t.Errorf("got %s", b)
	}
}
