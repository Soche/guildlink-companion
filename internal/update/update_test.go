package update

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"guildlink/companion/internal/config"
)

func TestNewer(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"v0.5.1", "v0.5.0", true},
		{"v0.10.0", "v0.9.9", true},
		{"v1.0.0", "v0.99.99", true},
		{"v0.5.0", "v0.5.0", false},
		{"v0.4.9", "v0.5.0", false},
		{"0.6.0", "v0.5.0", true},
		{"v0.6.0", "dev", false},
		{"garbage", "v0.1.0", false},
	}
	for _, c := range cases {
		if got := Newer(c.a, c.b); got != c.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestChecksumFor(t *testing.T) {
	sums := []byte("abc123  file-a\nDEF456 *file-b\n")
	if got, ok := checksumFor(sums, "file-b"); !ok || got != "def456" {
		t.Errorf("file-b: %q %v", got, ok)
	}
	if _, ok := checksumFor(sums, "missing"); ok {
		t.Error("found a missing file")
	}
}

// addonZip builds a release zip; extra entries let tests add bad paths.
func addonZip(t *testing.T, version string, extra map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	files := map[string]string{
		"GuildLink/GuildLink.toc": "## Interface: 16001\n## Version: " + strings.TrimPrefix(version, "v") + "\n",
		"GuildLink/Core.lua":      "-- core " + version,
		"GuildLink/Seed.lua":      "-- placeholder",
	}
	for k, v := range extra {
		files[k] = v
	}
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(body))
	}
	zw.Close()
	return buf.Bytes()
}

func TestInstallAddonKeepsSeedAndReplacesFiles(t *testing.T) {
	addons := t.TempDir()
	if err := InstallAddon(addonZip(t, "v0.5.0", nil), addons); err != nil {
		t.Fatal(err)
	}
	if v, ok := InstalledAddonVersion(addons); !ok || v != "v0.5.0" {
		t.Fatalf("installed %q %v", v, ok)
	}
	seed := filepath.Join(addons, "GuildLink", "Seed.lua")
	os.WriteFile(seed, []byte("GuildLinkDB = { mine = true }"), 0o644)
	os.WriteFile(filepath.Join(addons, "GuildLink", "Stale.lua"), []byte("old"), 0o644)

	if err := InstallAddon(addonZip(t, "v0.6.0", nil), addons); err != nil {
		t.Fatal(err)
	}
	if v, _ := InstalledAddonVersion(addons); v != "v0.6.0" {
		t.Errorf("version after update = %q", v)
	}
	if got, _ := os.ReadFile(seed); string(got) != "GuildLinkDB = { mine = true }" {
		t.Errorf("Seed.lua = %q", got)
	}
	if _, err := os.Stat(filepath.Join(addons, "GuildLink", "Stale.lua")); !os.IsNotExist(err) {
		t.Error("files removed from the addon were left behind")
	}
	for _, leftover := range []string{"GuildLink.new", "GuildLink.old"} {
		if _, err := os.Stat(filepath.Join(addons, leftover)); !os.IsNotExist(err) {
			t.Errorf("%s left behind", leftover)
		}
	}
}

func TestInstallAddonRejectsBadZips(t *testing.T) {
	cases := map[string][]byte{
		"path escape":    addonZip(t, "v1.0.0", map[string]string{"GuildLink/../../evil.lua": "x"}),
		"outside folder": addonZip(t, "v1.0.0", map[string]string{"Other/evil.lua": "x"}),
		"not a zip":      []byte("hello"),
	}
	for name, data := range cases {
		addons := t.TempDir()
		if err := InstallAddon(addonZip(t, "v0.5.0", nil), addons); err != nil {
			t.Fatal(err)
		}
		if err := InstallAddon(data, addons); err == nil {
			t.Errorf("%s: installed", name)
		}
		if v, _ := InstalledAddonVersion(addons); v != "v0.5.0" {
			t.Errorf("%s: the working addon was damaged (now %q)", name, v)
		}
	}
}

func TestReplaceFile(t *testing.T) {
	for _, windows := range []bool{false, true} {
		exe := filepath.Join(t.TempDir(), "companion")
		os.WriteFile(exe, []byte("old"), 0o755)
		if err := replaceFile(exe, []byte("new"), windows); err != nil {
			t.Fatal(err)
		}
		if got, _ := os.ReadFile(exe); string(got) != "new" {
			t.Errorf("windows=%v: binary = %q", windows, got)
		}
		info, _ := os.Stat(exe)
		if info.Mode().Perm()&0o100 == 0 {
			t.Errorf("windows=%v: not executable", windows)
		}
	}
}

// fakeGitHub serves "latest release" JSON and assets for the two repos.
func fakeGitHub(t *testing.T, addonTag, companionTag string, addonZipData []byte, tamper bool) *httptest.Server {
	var srv *httptest.Server
	sum := sha256.Sum256(addonZipData)
	sums := fmt.Sprintf("%s  %s\n", hex.EncodeToString(sum[:]), AddonAssetName(addonTag))
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/" + AddonRepo + "/releases/latest":
			json.NewEncoder(w).Encode(Release{Tag: addonTag, Assets: []Asset{
				{Name: AddonAssetName(addonTag), URL: srv.URL + "/dl/addon.zip"},
				{Name: "SHA256SUMS", URL: srv.URL + "/dl/addon-sums"},
			}})
		case "/repos/" + CompanionRepo + "/releases/latest":
			json.NewEncoder(w).Encode(Release{Tag: companionTag})
		case "/dl/addon.zip":
			data := addonZipData
			if tamper {
				data = append([]byte(nil), data...)
				data[len(data)/2] ^= 0xff
			}
			w.Write(data)
		case "/dl/addon-sums":
			w.Write([]byte(sums))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func wowInstall(t *testing.T) string {
	root := t.TempDir()
	for _, f := range []string{"_retail_", "_classic_beta_"} {
		os.MkdirAll(filepath.Join(root, f, "Interface"), 0o755)
	}
	return root
}

func newUpdater(t *testing.T, wowDir, apiBase string) *Updater {
	store, _ := config.Load(filepath.Join(t.TempDir(), "config.json"))
	store.Save(config.Settings{WowDir: wowDir})
	u := New(store, "v0.5.0", "", func() { t.Error("restart requested; the companion is current") })
	u.gh.APIBase = apiBase
	return u
}

func TestUpdaterInstallsAddonIntoForeverFolderOnly(t *testing.T) {
	root := wowInstall(t)
	srv := fakeGitHub(t, "v0.6.0", "v0.5.0", addonZip(t, "v0.6.0", nil), false)
	u := newUpdater(t, root, srv.URL)
	u.check(context.Background(), true)

	if v, ok := InstalledAddonVersion(filepath.Join(root, "_classic_beta_", "Interface", "AddOns")); !ok || v != "v0.6.0" {
		t.Errorf("_classic_beta_: %q %v", v, ok)
	}
	if _, ok := InstalledAddonVersion(filepath.Join(root, "_retail_", "Interface", "AddOns")); ok {
		t.Error("installed into _retail_ too")
	}
	st := u.Status()
	if st.AddonLatest != "v0.6.0" || st.CompanionLatest != "v0.5.0" || len(st.AddonTargets) != 1 || st.AddonTargets[0].Installed != "v0.6.0" {
		t.Errorf("status %+v", st)
	}
}

func TestUpdaterOnlyReportsWhenNotApplying(t *testing.T) {
	root := wowInstall(t)
	srv := fakeGitHub(t, "v0.6.0", "v0.5.0", addonZip(t, "v0.6.0", nil), false)
	u := newUpdater(t, root, srv.URL)
	u.check(context.Background(), false)
	if _, ok := InstalledAddonVersion(filepath.Join(root, "_classic_beta_", "Interface", "AddOns")); ok {
		t.Error("installed without applying")
	}
}

func TestUpdaterRefusesTamperedDownloads(t *testing.T) {
	root := wowInstall(t)
	srv := fakeGitHub(t, "v0.6.0", "v0.5.0", addonZip(t, "v0.6.0", nil), true)
	u := newUpdater(t, root, srv.URL)
	u.check(context.Background(), true)
	if _, ok := InstalledAddonVersion(filepath.Join(root, "_classic_beta_", "Interface", "AddOns")); ok {
		t.Error("installed a download that failed its checksum")
	}
	if st := u.Status(); len(st.AddonTargets) != 1 || !strings.Contains(st.AddonTargets[0].Error, "checksum") {
		t.Errorf("status %+v", st)
	}
}
