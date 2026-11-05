package calendar

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"guildlink/companion/internal/api"
	"guildlink/companion/internal/luasv"
)

func TestRenderIsReadableLua(t *testing.T) {
	desc := "Bring \"flasks\"\nand \\ repair\x01"
	inst := "Molten Core"
	e := api.Event{ID: 7, Title: "MC — tisdag", Description: &desc, Type: "raid", StartsAt: 1_800_000_000, Instance: &inst}
	e.Signup = &struct {
		Role   string `json:"role"`
		Status string `json:"status"`
	}{"healer", "bench"}
	vars, err := luasv.Parse(string(Render([]api.Event{e, {ID: 8, Title: "Social", Type: "event", StartsAt: 1}})))
	if err != nil {
		t.Fatal(err)
	}
	list, ok := vars["GuildLinkEvents"].(luasv.Table)
	if !ok || len(list) != 2 {
		t.Fatalf("GuildLinkEvents = %#v", vars["GuildLinkEvents"])
	}
	first, _ := list.Table("1")
	for key, want := range map[string]string{"title": "MC — tisdag", "description": desc, "instance": inst, "signup": "healer", "signupStatus": "bench", "type": "raid"} {
		if got, _ := first.String(key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
	if n, _ := first.Int("startsAt"); n != 1_800_000_000 {
		t.Errorf("startsAt = %d", n)
	}
	second, _ := list.Table("2")
	if _, ok := second.String("description"); ok {
		t.Error("an event without a description got one")
	}
}

func TestWriteOnlyWhereTheAddonIs(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "_forever_", "Interface", "AddOns", "GuildLink"), 0o755)
	os.MkdirAll(filepath.Join(dir, "_retail_", "Interface", "AddOns"), 0o755)
	content := Render(nil)
	changed, err := Write(dir, content)
	if err != nil || !slices.Equal(changed, []string{"_forever_"}) {
		t.Fatalf("changed %v, err %v", changed, err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "_forever_", "Interface", "AddOns", "GuildLink", "Events.lua")); string(got) != string(content) {
		t.Errorf("Events.lua = %q", got)
	}
	if changed, _ := Write(dir, content); len(changed) != 0 {
		t.Errorf("rewrote an unchanged file: %v", changed)
	}
}
