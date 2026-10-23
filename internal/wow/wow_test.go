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
