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
			["Jaina-"] = { ["name"] = "Jaina", ["realm"] = "", ["region"] = "EU", ["level"] = 12, ["class"] = "MAGE", ["updatedAt"] = 1000,
				["professions"] = {
					[197] = { ["name"] = "Tailoring", ["rank"] = 40, ["maxRank"] = 75 },
					[171] = { ["name"] = "Alchemy", ["rank"] = 5, ["scannedAt"] = 999,
						["recipes"] = { [2330] = { ["name"] = "Minor Healing Potion", ["itemID"] = 118, ["classID"] = 0, ["subclassID"] = 1, ["category"] = "Potions",
							["reagents"] = { { ["itemID"] = 2447, ["count"] = 1, ["name"] = "Peacebloom" }, { ["itemID"] = 765, ["count"] = 2 } } } } },
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
	want := `{"addonSchema":1,"discordId":"123456789012345678","characters":[{"name":"Jaina","realm":"","region":"EU","class":"MAGE","className":null,"race":null,"faction":null,"level":12,"guild":null,"guildRank":null,"guildRankIndex":null,"updatedAt":1000,"professions":[{"skillLineId":171,"name":"Alchemy","rank":5,"maxRank":null,"scannedAt":999,"recipes":[{"recipeId":2330,"name":"Minor Healing Potion","itemId":118,"classId":0,"subclassId":1,"equipLoc":null,"enchant":false,"category":"Potions","reagents":[{"itemId":2447,"count":1,"name":"Peacebloom"},{"itemId":765,"count":2,"name":null}]}]},{"skillLineId":197,"name":"Tailoring","rank":40,"maxRank":75,"scannedAt":null,"recipes":null}]}],"instances":[{"instanceId":2050,"name":"Hyjal Summit","kind":"raid","maxPlayers":20}],"guildRosters":[]}`
	if string(got) != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

func TestMissingGlobal(t *testing.T) {
	if _, err := FromSavedVariables(map[string]any{}); err == nil {
		t.Error("expected an error")
	}
}

func TestGuildRosters(t *testing.T) {
	vars, err := luasv.Parse(`GuildLinkDB = {
		["characters"] = { ["Soche-PvE"] = { ["name"] = "Soche Lightbringer", ["guild"] = "Nobility", ["guildRank"] = "Officer", ["guildRankIndex"] = 1 } },
		["guildRosters"] = {
			["Nobility"] = { ["guild"] = "Nobility", ["region"] = "EU", ["scannedAt"] = 500,
				["ranks"] = { [0] = "Guild Master", [1] = "Officer" },
				["members"] = { { ["name"] = "Soche-Lightbringer", ["rankIndex"] = 1 }, { ["name"] = "Boss-Person", ["rankIndex"] = 0 } } },
		},
	}`)
	if err != nil {
		t.Fatal(err)
	}
	p, err := FromSavedVariables(vars)
	if err != nil {
		t.Fatal(err)
	}
	if got := *p.Characters[0].GuildRankIndex; got != 1 {
		t.Errorf("rank index = %d", got)
	}
	got, _ := json.Marshal(p.GuildRosters)
	want := `[{"guild":"Nobility","region":"EU","scannedAt":500,"ranks":[{"index":0,"name":"Guild Master"},{"index":1,"name":"Officer"}],"members":[{"name":"Soche-Lightbringer","rankIndex":1},{"name":"Boss-Person","rankIndex":0}]}]`
	if string(got) != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}
