package syncer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"guildlink/companion/internal/config"
)

const sv = `GuildLinkDB = {
["schema"] = 1,
["characters"] = {
["Thrall-"] = { ["name"] = "Thrall", ["realm"] = "", ["level"] = 20 },
},
}
`

func TestUploadsChangedFilesAndWritesSeed(t *testing.T) {
	root := t.TempDir()
	flavor := filepath.Join(root, "_classic_beta_")
	svDir := filepath.Join(flavor, "WTF", "Account", "ACCOUNT1", "SavedVariables")
	addonDir := filepath.Join(flavor, "Interface", "AddOns", "GuildLink")
	for _, d := range []string{svDir, addonDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	svPath := filepath.Join(svDir, "GuildLink.lua")
	if err := os.WriteFile(svPath, []byte(sv), 0o644); err != nil {
		t.Fatal(err)
	}

	var uploads atomic.Int32
	var lastBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/sync" || r.Header.Get("Authorization") != "Bearer glk_test" {
			http.Error(w, `{"error":"nope"}`, http.StatusUnauthorized)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&lastBody)
		uploads.Add(1)
		_, _ = w.Write([]byte(`{"ok":true,"accepted":1,"skippedStale":0,"rejected":[],"recipesStored":0}`))
	}))
	defer srv.Close()

	store, _ := config.Load(filepath.Join(t.TempDir(), "config.json"))
	if err := store.Save(config.Settings{ServerURL: srv.URL + "/", Token: "glk_test", WowDir: root, WriteSeed: true}); err != nil {
		t.Fatal(err)
	}
	s := New(store, "test")
	ctx := context.Background()

	s.check(ctx, false)
	if uploads.Load() != 1 {
		t.Fatalf("uploads = %d, want 1; status %+v", uploads.Load(), s.Status())
	}
	chars := lastBody["characters"].([]any)
	if chars[0].(map[string]any)["name"] != "Thrall" {
		t.Errorf("uploaded %v", lastBody)
	}
	seed, err := os.ReadFile(filepath.Join(addonDir, "Seed.lua"))
	if err != nil || !strings.Contains(string(seed), `["name"] = "Thrall"`) {
		t.Errorf("Seed.lua = %q, %v", seed, err)
	}

	s.check(ctx, false)
	if uploads.Load() != 1 {
		t.Errorf("unchanged file uploaded again")
	}

	later := time.Now().Add(time.Minute)
	_ = os.WriteFile(svPath, []byte(strings.Replace(sv, "20", "21", 1)), 0o644)
	_ = os.Chtimes(svPath, later, later)
	s.check(ctx, false)
	if uploads.Load() != 2 {
		t.Errorf("changed file not uploaded")
	}

	s.check(ctx, true)
	if uploads.Load() != 3 {
		t.Errorf("forced sync did not upload")
	}

	settings := store.Get()
	settings.Token = "wrong"
	_ = store.Save(settings)
	s.check(ctx, true)
	st := s.Status()
	if len(st.Files) != 1 || !strings.Contains(st.Files[0].LastError, "nope") {
		t.Errorf("expected the server's error to be reported, got %+v", st.Files)
	}
}

func TestRetiredBetaIsNotUploadedAndCarriesDiscordID(t *testing.T) {
	root := t.TempDir()
	betaSV := filepath.Join(root, "_classic_beta_", "WTF", "Account", "ACCOUNT1", "SavedVariables")
	launchAddon := filepath.Join(root, "_forever_", "Interface", "AddOns", "GuildLink")
	for _, d := range []string{betaSV, launchAddon} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	withID := strings.Replace(sv, `["schema"] = 1,`, `["schema"] = 1, ["discordId"] = "123456789012345678",`, 1)
	if err := os.WriteFile(filepath.Join(betaSV, "GuildLink.lua"), []byte(withID), 0o644); err != nil {
		t.Fatal(err)
	}
	seedPath := filepath.Join(launchAddon, "Seed.lua")
	if err := os.WriteFile(seedPath, nil, 0o644); err != nil { // as shipped
		t.Fatal(err)
	}

	var uploads atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uploads.Add(1)
		_, _ = w.Write([]byte(`{"ok":true,"accepted":1}`))
	}))
	defer srv.Close()
	store, _ := config.Load(filepath.Join(t.TempDir(), "config.json"))
	if err := store.Save(config.Settings{ServerURL: srv.URL + "/", Token: "glk_test", WowDir: root, WriteSeed: true}); err != nil {
		t.Fatal(err)
	}
	s := New(store, "test")
	s.check(context.Background(), true)

	if uploads.Load() != 0 {
		t.Errorf("retired beta data uploaded")
	}
	seed, _ := os.ReadFile(seedPath)
	if !strings.Contains(string(seed), `GuildLinkDB = { discordId = "123456789012345678" }`) || strings.Contains(string(seed), "Thrall") {
		t.Errorf("Seed.lua = %q, want only the Discord ID", seed)
	}

	// Once the release client has a seed of its own, it's left alone.
	_ = os.WriteFile(seedPath, []byte("GuildLinkDB = {}\n"), 0o644)
	s.check(context.Background(), true)
	if seed, _ := os.ReadFile(seedPath); string(seed) != "GuildLinkDB = {}\n" {
		t.Errorf("Seed.lua overwritten: %q", seed)
	}
}
