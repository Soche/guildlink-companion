package wow

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func mkdirs(t *testing.T, dirs ...string) {
	t.Helper()
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func TestBetaRetiredOnceReleaseClientInstalled(t *testing.T) {
	root := t.TempDir()
	mkdirs(t, filepath.Join(AddOnsDir(root, BetaFlavor), AddonName))
	if Retired(root, BetaFlavor) {
		t.Error("beta retired without a release client")
	}
	if got := DefaultAddonFlavors(root); !slices.Equal(got, []string{BetaFlavor}) {
		t.Errorf("before release: %v", got)
	}

	mkdirs(t, filepath.Join(root, ForeverFlavor), filepath.Join(root, "_retail_"))
	if !Retired(root, BetaFlavor) || Retired(root, ForeverFlavor) {
		t.Error("Retired wrong after release")
	}
	if got := Flavors(root); !slices.Equal(got, []string{"_forever_", "_retail_"}) {
		t.Errorf("Flavors = %v", got)
	}
	if got := DefaultAddonFlavors(root); !slices.Equal(got, []string{ForeverFlavor}) {
		t.Errorf("after release: %v", got)
	}

	// The addon also kept somewhere else: the release client is added.
	mkdirs(t, filepath.Join(AddOnsDir(root, "_retail_"), AddonName))
	if got := DefaultAddonFlavors(root); !slices.Equal(got, []string{"_retail_", ForeverFlavor}) {
		t.Errorf("with retail addon: %v", got)
	}
}

func TestSeedDiscordIDRejectsBadIDs(t *testing.T) {
	root := t.TempDir()
	mkdirs(t, filepath.Join(AddOnsDir(root, ForeverFlavor), AddonName))
	for _, id := range []string{"", "123", `1234567890123456789" } os.exit() --`} {
		if wrote, _ := SeedDiscordID(root, ForeverFlavor, id); wrote {
			t.Errorf("wrote seed for %q", id)
		}
	}
	if wrote, err := SeedDiscordID(root, ForeverFlavor, "123456789012345678"); !wrote || err != nil {
		t.Errorf("valid ID: %v %v", wrote, err)
	}
}

func TestExpandPath(t *testing.T) {
	home, _ := os.UserHomeDir()
	for in, want := range map[string]string{
		"~/Games/Heroic/":         filepath.Join(home, "Games", "Heroic"),
		` "~/World of Warcraft" `: filepath.Join(home, "World of Warcraft"),
		"~":                       home,
		"/opt/wow/":               "/opt/wow",
		"~other/wow":              "~other/wow",
		"":                        "",
	} {
		if got := ExpandPath(in); got != want {
			t.Errorf("ExpandPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHeroicPrefixesFromSettings(t *testing.T) {
	home := t.TempDir()
	cfg := filepath.Join(home, ".var", "app", "com.heroicgameslauncher.hgl", "config", "heroic", "GamesConfig")
	mkdirs(t, cfg)
	game := `{"abc123": {"winePrefix": "/data/Prefixes/Battle.net", "wineVersion": {"bin": "/x"}}, "version": "v0"}`
	if err := os.WriteFile(filepath.Join(cfg, "abc123.json"), []byte(game), 0o644); err != nil {
		t.Fatal(err)
	}
	got := heroicPrefixes(home)
	if !slices.Contains(got, "/data/Prefixes/Battle.net") {
		t.Errorf("heroicPrefixes = %v", got)
	}
}

func TestDetectsHeroicBattleNetPrefix(t *testing.T) {
	home := t.TempDir()
	wow := filepath.Join(home, "Games", "Heroic", "Prefixes", "default", "Battle.net", "drive_c", "Program Files (x86)", "World of Warcraft")
	elsewhere := filepath.Join(home, "wine", "bnet", "drive_c", "Games", "World of Warcraft")
	mkdirs(t, filepath.Join(wow, "_forever_"), elsewhere, filepath.Join(home, ".config", "heroic", "GamesConfig"))
	settings := `{"bnet": {"winePrefix": "` + filepath.Join(home, "wine", "bnet") + `"}}`
	if err := os.WriteFile(filepath.Join(home, ".config", "heroic", "GamesConfig", "bnet.json"), []byte(settings), 0o644); err != nil {
		t.Fatal(err)
	}
	got := detectIn(wineDirs(home))
	if !slices.Contains(got, wow) || !slices.Contains(got, elsewhere) {
		t.Errorf("detected %v", got)
	}
}
