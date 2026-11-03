package autostart

import (
	"runtime"
	"strings"
	"testing"
)

func TestDesktopEntryQuotesTheCommand(t *testing.T) {
	got := desktopEntry(`/home/me/Games/my "wow" $stuff/100%/guildlink-companion`)
	want := `Exec="/home/me/Games/my \"wow\" \$stuff/100%%/guildlink-companion" "-no-browser"`
	if !strings.Contains(got, want+"\n") {
		t.Errorf("desktop entry:\n%s\nwant line %s", got, want)
	}
	for _, line := range []string{"[Desktop Entry]", "Type=Application", "Terminal=false"} {
		if !strings.Contains(got, line+"\n") {
			t.Errorf("missing %q", line)
		}
	}
}

func TestLaunchAgentRunsTheBinaryAtLogin(t *testing.T) {
	got := launchAgent("top.lassi.guildlink.companion", "/Users/me/Apps & Tools/guildlink-companion-darwin-arm64")
	for _, want := range []string{
		"<string>/Users/me/Apps &amp; Tools/guildlink-companion-darwin-arm64</string>",
		"<string>-no-browser</string>",
		"<key>RunAtLoad</key>\n\t<true/>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("plist lacks %q:\n%s", want, got)
		}
	}
}

func TestWindowsCommandQuotesThePath(t *testing.T) {
	if got := windowsCommand(`C:\Users\Me\Downloads\guildlink-companion-windows-amd64.exe`); got != `"C:\Users\Me\Downloads\guildlink-companion-windows-amd64.exe" -no-browser` {
		t.Errorf("got %s", got)
	}
}

func TestEnableDisableRoundTrip(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("would change the real registry")
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
	if on, _ := Enabled(); on {
		t.Fatal("enabled before Enable")
	}
	if err := Enable("/opt/guildlink-companion"); err != nil {
		t.Fatal(err)
	}
	if on, err := Enabled(); !on || err != nil {
		t.Fatalf("Enabled = %v, %v", on, err)
	}
	if err := Disable(); err != nil {
		t.Fatal(err)
	}
	if err := Disable(); err != nil {
		t.Fatal("second Disable:", err)
	}
	if on, _ := Enabled(); on {
		t.Error("still enabled")
	}
}
