// Package calendar writes the guild's upcoming Discord events into the
// addon's Events.lua, which the addon reads at login to offer adding them to
// the game's calendar. Addons can't make network requests, and the game only
// reads addon files at login or /reload, so this is how they get there.
package calendar

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"guildlink/companion/internal/api"
	"guildlink/companion/internal/wow"
)

const fileName = "Events.lua"

// Render turns events into the Lua file the addon loads.
func Render(events []api.Event) []byte {
	var b bytes.Buffer
	b.WriteString("-- Written by the GuildLink companion app: upcoming events from Discord. Do not edit.\n")
	b.WriteString("GuildLinkEvents = {\n")
	for _, e := range events {
		fmt.Fprintf(&b, "  { id = %d, title = %s, type = %s, startsAt = %d", e.ID, quote(e.Title), quote(e.Type), e.StartsAt)
		if e.Description != nil && *e.Description != "" {
			fmt.Fprintf(&b, ", description = %s", quote(*e.Description))
		}
		if e.Instance != nil && *e.Instance != "" {
			fmt.Fprintf(&b, ", instance = %s", quote(*e.Instance))
		}
		if e.Signup != nil {
			fmt.Fprintf(&b, ", signup = %s, signupStatus = %s", quote(e.Signup.Role), quote(e.Signup.Status))
		}
		b.WriteString(" },\n")
	}
	b.WriteString("}\n")
	return b.Bytes()
}

// quote makes a Lua 5.1 string literal; anything but printable ASCII and
// UTF-8 text is written as a decimal escape.
func quote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '"' || c == '\\':
			b.WriteByte('\\')
			b.WriteByte(c)
		case c == '\n':
			b.WriteString(`\n`)
		case c < 0x20 || c == 0x7f:
			fmt.Fprintf(&b, `\%03d`, c)
		default:
			b.WriteByte(c)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// Write puts content in Events.lua of every flavor that has the addon.
// Returns the flavors whose file changed.
func Write(installDir string, content []byte) ([]string, error) {
	var changed []string
	for _, flavor := range wow.Flavors(installDir) {
		dir := filepath.Join(wow.AddOnsDir(installDir, flavor), wow.AddonName)
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			continue
		}
		path := filepath.Join(dir, fileName)
		if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, content) {
			continue
		}
		tmp := path + ".tmp"
		if err := os.WriteFile(tmp, content, 0o644); err != nil {
			return changed, err
		}
		if err := os.Rename(tmp, path); err != nil {
			return changed, err
		}
		changed = append(changed, flavor)
	}
	return changed, nil
}
