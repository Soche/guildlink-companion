// Package wow finds World of Warcraft installs and the GuildLink addon's files in them.
package wow

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strings"
	"time"
)

const (
	AddonName = "GuildLink"
	svName    = AddonName + ".lua"
	seedName  = "Seed.lua"
)

// candidateDirs lists usual install locations; globs are allowed.
func candidateDirs() []string {
	home, _ := os.UserHomeDir()
	const x86 = "Program Files (x86)"
	switch runtime.GOOS {
	case "windows":
		var dirs []string
		for _, drive := range []string{"C:", "D:", "E:", "F:"} {
			root := drive + `\`
			dirs = append(dirs,
				filepath.Join(root, x86, "World of Warcraft"),
				filepath.Join(root, "Program Files", "World of Warcraft"),
				filepath.Join(root, "World of Warcraft"),
				filepath.Join(root, "Games", "World of Warcraft"),
			)
		}
		return dirs
	case "darwin":
		return []string{"/Applications/World of Warcraft", filepath.Join(home, "Applications", "World of Warcraft")}
	default:
		// Wine prefixes from Lutris, plain Wine, Steam/Proton and Bottles.
		inPrefix := func(prefix string) string { return filepath.Join(prefix, "drive_c", x86, "World of Warcraft") }
		return []string{
			inPrefix(filepath.Join(home, "Games", "world-of-warcraft")),
			inPrefix(filepath.Join(home, "Games", "battlenet")),
			inPrefix(filepath.Join(home, "Games", "*")),
			inPrefix(filepath.Join(home, ".wine")),
			inPrefix(filepath.Join(home, ".local/share/Steam/steamapps/compatdata/*/pfx")),
			inPrefix(filepath.Join(home, ".local/share/bottles/bottles/*")),
			inPrefix(filepath.Join(home, ".var/app/com.usebottles.bottles/data/bottles/bottles/*")),
		}
	}
}

// DetectInstalls returns existing WoW install folders in the usual places.
func DetectInstalls() []string {
	seen := map[string]bool{}
	var found []string
	for _, pattern := range candidateDirs() {
		matches, _ := filepath.Glob(pattern)
		for _, m := range matches {
			if info, err := os.Stat(m); err == nil && info.IsDir() && !seen[m] {
				seen[m] = true
				found = append(found, m)
			}
		}
	}
	return found
}

// SavedVariables is one GuildLink.lua file, for one WoW account in one game flavor.
type SavedVariables struct {
	Path    string    `json:"path"`
	Flavor  string    `json:"flavor"`  // e.g. "_forever_"
	Account string    `json:"account"` // WTF account folder name
	ModTime time.Time `json:"modTime"`
	Size    int64     `json:"size"`

	flavorDir string
}

// FindSavedVariables lists GuildLink SavedVariables files under every game
// flavor (_retail_, _forever_, ...) of an install, a retired beta's included.
func FindSavedVariables(installDir string) ([]SavedVariables, error) {
	matches, err := filepath.Glob(filepath.Join(installDir, "*", "WTF", "Account", "*", "SavedVariables", svName))
	if err != nil {
		return nil, err
	}
	var out []SavedVariables
	for _, m := range matches {
		info, err := os.Stat(m)
		if err != nil {
			continue
		}
		accountDir := filepath.Dir(filepath.Dir(m))
		flavorDir := filepath.Dir(filepath.Dir(filepath.Dir(accountDir)))
		out = append(out, SavedVariables{
			Path:      m,
			Flavor:    filepath.Base(flavorDir),
			Account:   filepath.Base(accountDir),
			ModTime:   info.ModTime(),
			Size:      info.Size(),
			flavorDir: flavorDir,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// AddonInstalled reports whether the flavor has the GuildLink addon folder.
func (sv SavedVariables) AddonInstalled() bool {
	info, err := os.Stat(filepath.Join(sv.flavorDir, "Interface", "AddOns", AddonName))
	return err == nil && info.IsDir()
}

// WriteSeed copies the saved data into the addon's Seed.lua so the next
// session starts with it. The Forever client (at least during the beta)
// writes SavedVariables on logout but never loads them, and Seed.lua runs as
// addon code instead.
// Returns whether the file changed.
func (sv SavedVariables) WriteSeed(content []byte) (bool, error) {
	if !sv.AddonInstalled() {
		return false, nil
	}
	path := filepath.Join(sv.flavorDir, "Interface", "AddOns", AddonName, seedName)
	header := []byte("-- Written by the GuildLink companion app from " + sv.Account + "'s saved data. Do not edit.\n")
	data := append(header, content...)
	if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, data) {
		return false, nil
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return false, err
	}
	return true, os.Rename(tmp, path)
}

// The Forever release client's folder, and the beta's.
const (
	ForeverFlavor = "_forever_"
	BetaFlavor    = "_classic_beta_"
)

// Retired reports whether a flavor folder is the ended Forever beta: the
// beta's folder once the release client is installed next to it. Its data
// isn't uploaded and the addon isn't kept updated there.
func Retired(installDir, flavor string) bool {
	if flavor != BetaFlavor {
		return false
	}
	info, err := os.Stat(filepath.Join(installDir, ForeverFlavor))
	return err == nil && info.IsDir()
}

// NeedsSeed reports whether a flavor has the addon installed but neither
// saved data nor a filled-in Seed.lua yet (the addon ships an empty one).
func NeedsSeed(installDir, flavor string) bool {
	addonDir := filepath.Join(AddOnsDir(installDir, flavor), AddonName)
	if info, err := os.Stat(addonDir); err != nil || !info.IsDir() {
		return false
	}
	if seed, err := os.ReadFile(filepath.Join(addonDir, seedName)); err == nil && len(bytes.TrimSpace(seed)) > 0 {
		return false
	}
	saved, _ := filepath.Glob(filepath.Join(installDir, flavor, "WTF", "Account", "*", "SavedVariables", svName))
	return len(saved) == 0
}

var discordIDPattern = regexp.MustCompile(`^[0-9]{17,20}$`)

// SeedDiscordID starts a flavor's addon data with just a Discord user ID,
// so you don't have to enter it again in a new game client. Only done while
// the flavor has no saved data and no Seed.lua of its own. Returns whether
// it wrote the file.
func SeedDiscordID(installDir, flavor, discordID string) (bool, error) {
	if !discordIDPattern.MatchString(discordID) || !NeedsSeed(installDir, flavor) {
		return false, nil
	}
	path := filepath.Join(AddOnsDir(installDir, flavor), AddonName, seedName)
	data := "-- Written by the GuildLink companion app: your Discord ID, carried over from another game client. Do not edit.\n" +
		"GuildLinkDB = { discordId = \"" + discordID + "\" }\n"
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(data), 0o644); err != nil {
		return false, err
	}
	return true, os.Rename(tmp, path)
}

// Flavors lists the game flavor folders of an install (_retail_,
// _forever_, ...), leaving out a retired beta folder.
func Flavors(installDir string) []string {
	entries, err := os.ReadDir(installDir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() && len(n) > 2 && strings.HasPrefix(n, "_") && strings.HasSuffix(n, "_") && !Retired(installDir, n) {
			out = append(out, n)
		}
	}
	return out
}

// AddOnsDir is where a flavor's addons live.
func AddOnsDir(installDir, flavor string) string {
	return filepath.Join(installDir, flavor, "Interface", "AddOns")
}

// DefaultAddonFlavors picks which flavors get the addon when the user hasn't
// chosen: ones that already have it, else Forever-looking ones (the addon
// only loads in the Forever client), else the only flavor installed. The
// Forever release client always gets it once installed.
func DefaultAddonFlavors(installDir string) []string {
	flavors := Flavors(installDir)
	var have, forever []string
	for _, f := range flavors {
		if info, err := os.Stat(filepath.Join(AddOnsDir(installDir, f), AddonName)); err == nil && info.IsDir() {
			have = append(have, f)
		}
		if l := strings.ToLower(f); strings.Contains(l, "forever") || strings.Contains(l, "classic_beta") {
			forever = append(forever, f)
		}
	}
	switch {
	case len(have) > 0:
		if !slices.Contains(have, ForeverFlavor) && slices.Contains(flavors, ForeverFlavor) {
			have = append(have, ForeverFlavor)
		}
		return have
	case len(forever) > 0:
		return forever
	case len(flavors) == 1:
		return flavors
	}
	return nil
}

// FlavorKey identifies the game flavor folder an SV file belongs to; files
// with the same key share one Seed.lua.
func (sv SavedVariables) FlavorKey() string { return sv.flavorDir }
