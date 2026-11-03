// Package autostart starts the companion when the user logs in: a Run
// registry value on Windows, a LaunchAgent on macOS, and an XDG autostart
// entry on Linux. The entry runs the binary where it is now, with
// -no-browser so logging in doesn't open the settings page.
package autostart

import (
	"os"
	"path/filepath"
	"strings"
)

// Name identifies the entry in each system's list of startup programs.
const Name = "GuildLink Companion"

// Args are what the companion is started with at login.
var Args = []string{"-no-browser"}

// Enabled reports whether the companion starts at login.
func Enabled() (bool, error) { return enabled() }

// Enable makes the companion start at login, running exe (an absolute path).
func Enable(exe string) error { return enable(exe) }

// Disable stops the companion from starting at login. No entry is fine.
func Disable() error { return disable() }

// Refresh points an existing entry at exe, in case the binary was moved.
// It does nothing when autostart is off.
func Refresh(exe string) error {
	on, err := Enabled()
	if err != nil || !on {
		return err
	}
	return Enable(exe)
}

// writeFile writes data to path, creating its folder, through a temporary file.
func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func removeFile(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// launchAgent is the macOS LaunchAgent property list that runs exe at login.
func launchAgent(label, exe string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>` + xmlEscape(label) + `</string>
	<key>ProgramArguments</key>
	<array>
`)
	for _, a := range append([]string{exe}, Args...) {
		b.WriteString("\t\t<string>" + xmlEscape(a) + "</string>\n")
	}
	b.WriteString(`	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>ProcessType</key>
	<string>Interactive</string>
</dict>
</plist>
`)
	return b.String()
}

func xmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;").Replace(s)
}

// desktopEntry is the XDG autostart file that runs exe at login.
func desktopEntry(exe string) string {
	args := []string{desktopQuote(exe)}
	for _, a := range Args {
		args = append(args, desktopQuote(a))
	}
	return "[Desktop Entry]\n" +
		"Type=Application\n" +
		"Name=" + Name + "\n" +
		"Comment=Uploads GuildLink addon data to your guild's Discord bot\n" +
		"Exec=" + strings.Join(args, " ") + "\n" +
		"Terminal=false\n" +
		"X-GNOME-Autostart-enabled=true\n"
}

// desktopQuote quotes an Exec argument as the Desktop Entry spec requires:
// double quotes around it, with ", `, $ and \ escaped, and % doubled.
func desktopQuote(s string) string {
	s = strings.NewReplacer(`\`, `\\`, `"`, `\"`, "`", "\\`", "$", `\$`).Replace(s)
	s = strings.ReplaceAll(s, "%", "%%")
	return `"` + s + `"`
}

// windowsCommand is the Run registry value that starts exe.
func windowsCommand(exe string) string {
	return `"` + exe + `" ` + strings.Join(Args, " ")
}
