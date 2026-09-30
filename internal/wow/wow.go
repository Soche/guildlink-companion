// Package wow finds World of Warcraft installs and the GuildLink addon's files in them.
package wow

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"sort"
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
	Flavor  string    `json:"flavor"`  // e.g. "_classic_beta_"
	Account string    `json:"account"` // WTF account folder name
	ModTime time.Time `json:"modTime"`
	Size    int64     `json:"size"`

	flavorDir string
}

// FindSavedVariables lists GuildLink SavedVariables files under every game
// flavor (_retail_, _classic_beta_, ...) of an install.
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
// session starts with it. The Forever beta client writes SavedVariables on
// logout but never loads them, and Seed.lua runs as addon code instead.
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

// FlavorKey identifies the game flavor folder an SV file belongs to; files
// with the same key share one Seed.lua.
func (sv SavedVariables) FlavorKey() string { return sv.flavorDir }
