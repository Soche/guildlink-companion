package luasv

import (
	"reflect"
	"testing"
)

const sample = `
GuildLinkDB = {
["discordId"] = "123456789012345678",
["schema"] = 1,
["characters"] = {
["Thrall-"] = {
["name"] = "Thrall",
["realm"] = "",
["level"] = 20,
["guild"] = "Say \"hi\" \\ \226\152\131",
["professions"] = {
[171] = {
["name"] = "Alchemy",
["rank"] = 50,
["recipes"] = {
[2330] = {
["name"] = "Minor Healing Potion",
["itemID"] = 118,
},
},
},
},
},
},
["list"] = {
"a", -- [1]
"b", -- [2]
},
["neg"] = -1.5e2,
["flag"] = false,
}
--[[ block
comment ]]
Other = nil
`

func TestParse(t *testing.T) {
	got, err := Parse(sample)
	if err != nil {
		t.Fatal(err)
	}
	db := got["GuildLinkDB"].(Table)
	if id, _ := db.String("discordId"); id != "123456789012345678" {
		t.Errorf("discordId = %q", id)
	}
	chars, _ := db.Table("characters")
	thrall, _ := chars.Table("Thrall-")
	if g, _ := thrall.String("guild"); g != "Say \"hi\" \\ ☃" {
		t.Errorf("guild = %q", g)
	}
	profs, _ := thrall.Table("professions")
	alch, _ := profs.Table("171")
	recipes, _ := alch.Table("recipes")
	r, _ := recipes.Table("2330")
	if n, _ := r.Int("itemID"); n != 118 {
		t.Errorf("itemID = %d", n)
	}
	if !reflect.DeepEqual(db["list"], Table{"1": "a", "2": "b"}) {
		t.Errorf("list = %#v", db["list"])
	}
	if db["neg"] != -150.0 || db["flag"] != false {
		t.Errorf("neg=%v flag=%v", db["neg"], db["flag"])
	}
	if _, ok := got["Other"]; !ok || got["Other"] != nil {
		t.Errorf("Other = %#v", got["Other"])
	}
}

func TestParseErrors(t *testing.T) {
	for _, src := range []string{`X = {`, `X = "abc`, `X = {1 2}`, `= 1`, `X = @`} {
		if _, err := Parse(src); err == nil {
			t.Errorf("Parse(%q) succeeded", src)
		}
	}
}
