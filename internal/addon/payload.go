// Package addon turns the GuildLink addon's SavedVariables into the upload
// format the bot's /api/v1/sync endpoint expects.
package addon

import (
	"fmt"
	"sort"
	"strconv"

	"guildlink/companion/internal/luasv"
)

const Global = "GuildLinkDB"

type Recipe struct {
	RecipeID int64  `json:"recipeId"`
	Name     string `json:"name"`
	ItemID   *int64 `json:"itemId"`
}

type Profession struct {
	SkillLineID int64    `json:"skillLineId"`
	Name        string   `json:"name"`
	Rank        *int64   `json:"rank"`
	MaxRank     *int64   `json:"maxRank"`
	ScannedAt   *int64   `json:"scannedAt"`
	Recipes     []Recipe `json:"recipes"` // nil (JSON null) when never scanned
}

type Character struct {
	Name        string       `json:"name"`
	Realm       string       `json:"realm"`
	Class       *string      `json:"class"`
	ClassName   *string      `json:"className"`
	Race        *string      `json:"race"`
	Faction     *string      `json:"faction"`
	Level       *int64       `json:"level"`
	Guild       *string      `json:"guild"`
	GuildRank   *string      `json:"guildRank"`
	UpdatedAt   *int64       `json:"updatedAt"`
	Professions []Profession `json:"professions"`
}

// Instance is a dungeon or raid a character has been inside.
type Instance struct {
	InstanceID int64  `json:"instanceId"`
	Name       string `json:"name"`
	Kind       string `json:"kind"` // "dungeon" or "raid"
	MaxPlayers int64  `json:"maxPlayers"`
}

type Payload struct {
	AddonSchema int64       `json:"addonSchema"`
	DiscordID   *string     `json:"discordId"`
	Characters  []Character `json:"characters"`
	Instances   []Instance  `json:"instances"`
}

func str(t luasv.Table, k string) *string {
	if v, ok := t.String(k); ok && v != "" {
		return &v
	}
	return nil
}

func num(t luasv.Table, k string) *int64 {
	if v, ok := t.Int(k); ok {
		return &v
	}
	return nil
}

// sortedKeys keeps uploads deterministic so unchanged data hashes the same.
func sortedKeys(t luasv.Table) []string {
	keys := make([]string, 0, len(t))
	for k := range t {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// FromSavedVariables builds the upload from a parsed SavedVariables file.
func FromSavedVariables(vars map[string]any) (*Payload, error) {
	db, ok := vars[Global].(luasv.Table)
	if !ok {
		return nil, fmt.Errorf("%s not found; log in with the addon enabled, then log out or /reload", Global)
	}
	p := &Payload{Characters: []Character{}, Instances: []Instance{}}
	if v, ok := db.Int("schema"); ok {
		p.AddonSchema = v
	}
	p.DiscordID = str(db, "discordId")

	chars, _ := db.Table("characters")
	for _, key := range sortedKeys(chars) {
		ct, ok := chars.Table(key)
		if !ok {
			continue
		}
		name, _ := ct.String("name")
		if name == "" {
			continue
		}
		realm, _ := ct.String("realm")
		c := Character{
			Name: name, Realm: realm,
			Class: str(ct, "class"), ClassName: str(ct, "className"),
			Race: str(ct, "race"), Faction: str(ct, "faction"),
			Level: num(ct, "level"), Guild: str(ct, "guild"), GuildRank: str(ct, "guildRank"),
			UpdatedAt:   num(ct, "updatedAt"),
			Professions: []Profession{},
		}
		profs, _ := ct.Table("professions")
		for _, pk := range sortedKeys(profs) {
			pt, ok := profs.Table(pk)
			skillLine, err := strconv.ParseInt(pk, 10, 64)
			if !ok || err != nil {
				continue
			}
			pname, _ := pt.String("name")
			if pname == "" {
				continue
			}
			prof := Profession{
				SkillLineID: skillLine, Name: pname,
				Rank: num(pt, "rank"), MaxRank: num(pt, "maxRank"), ScannedAt: num(pt, "scannedAt"),
			}
			if rt, ok := pt.Table("recipes"); ok && prof.ScannedAt != nil {
				prof.Recipes = []Recipe{}
				for _, rk := range sortedKeys(rt) {
					r, ok := rt.Table(rk)
					id, err := strconv.ParseInt(rk, 10, 64)
					rname, _ := r.String("name")
					if !ok || err != nil || rname == "" {
						continue
					}
					prof.Recipes = append(prof.Recipes, Recipe{RecipeID: id, Name: rname, ItemID: num(r, "itemID")})
				}
			}
			c.Professions = append(c.Professions, prof)
		}
		p.Characters = append(p.Characters, c)
	}

	insts, _ := db.Table("instances")
	for _, key := range sortedKeys(insts) {
		it, ok := insts.Table(key)
		id, err := strconv.ParseInt(key, 10, 64)
		if !ok || err != nil || id <= 0 {
			continue
		}
		name, _ := it.String("name")
		kind, _ := it.String("kind")
		players, _ := it.Int("maxPlayers")
		if name == "" || (kind != "dungeon" && kind != "raid") || players < 1 || players > 40 {
			continue
		}
		p.Instances = append(p.Instances, Instance{InstanceID: id, Name: name, Kind: kind, MaxPlayers: players})
	}
	return p, nil
}
