package addon

import (
	"encoding/json"
	"testing"

	"guildlink/companion/internal/luasv"
)

func TestFromSavedVariables(t *testing.T) {
	vars, err := luasv.Parse(`GuildLinkDB = {
		["schema"] = 1,
		["discordId"] = "123456789012345678",
		["characters"] = {
			["Jaina-"] = { ["name"] = "Jaina", ["realm"] = "", ["level"] = 12, ["class"] = "MAGE", ["updatedAt"] = 1000,
				["professions"] = {
					[197] = { ["name"] = "Tailoring", ["rank"] = 40, ["maxRank"] = 75 },
					[171] = { ["name"] = "Alchemy", ["rank"] = 5, ["scannedAt"] = 999,
						["recipes"] = { [2330] = { ["name"] = "Minor Healing Potion", ["itemID"] = 118, ["classID"] = 0, ["subclassID"] = 1, ["category"] = "Potions" } } },
				} },
		},
		["instances"] = {
			[2050] = { ["name"] = "Hyjal Summit", ["kind"] = "raid", ["maxPlayers"] = 20, ["seenAt"] = 5 },
			[9] = { ["name"] = "Arena", ["kind"] = "pvp", ["maxPlayers"] = 5 },
		},
	}`)
	if err != nil {
		t.Fatal(err)
	}
	p, err := FromSavedVariables(vars)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := json.Marshal(p)
	want := `{"addonSchema":1,"discordId":"123456789012345678","characters":[{"name":"Jaina","realm":"","class":"MAGE","className":null,"race":null,"faction":null,"level":12,"guild":null,"guildRank":null,"updatedAt":1000,"professions":[{"skillLineId":171,"name":"Alchemy","rank":5,"maxRank":null,"scannedAt":999,"recipes":[{"recipeId":2330,"name":"Minor Healing Potion","itemId":118,"classId":0,"subclassId":1,"equipLoc":null,"enchant":false,"category":"Potions"}]},{"skillLineId":197,"name":"Tailoring","rank":40,"maxRank":75,"scannedAt":null,"recipes":null}]}],"instances":[{"instanceId":2050,"name":"Hyjal Summit","kind":"raid","maxPlayers":20}]}`
	if string(got) != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

func TestMissingGlobal(t *testing.T) {
	if _, err := FromSavedVariables(map[string]any{}); err == nil {
		t.Error("expected an error")
	}
}
