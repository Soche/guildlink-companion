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
	// What the crafted item is, for browsing by category (nil from addons before 0.3.0).
	ClassID    *int64  `json:"classId"`
	SubclassID *int64  `json:"subclassId"`
	EquipLoc   *string `json:"equipLoc"`
	Enchant    bool    `json:"enchant"`
	Category   *string `json:"category"` // the profession window's category name
	// Required materials (nil from addons before 0.4.0).
	Reagents []Reagent `json:"reagents,omitempty"`
}

type Reagent struct {
	ItemID int64   `json:"itemId"`
	Count  int64   `json:"count"`
	Name   *string `json:"name"` // nil when the game hadn't loaded the item's name yet
}

// reagentsOf reads the addon's positional reagent list, in order.
func reagentsOf(r luasv.Table) []Reagent {
	list, ok := r.Table("reagents")
	if !ok {
		return nil
	}
	var out []Reagent
	for i := 1; ; i++ {
		e, ok := list.Table(strconv.Itoa(i))
		if !ok {
			return out
		}
		id, ok := e.Int("itemID")
		if !ok || id <= 0 {
			continue
		}
		count, ok := e.Int("count")
		if !ok || count < 1 {
			count = 1
		}
		out = append(out, Reagent{ItemID: id, Count: count, Name: str(e, "name")})
	}
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
	Name           string       `json:"name"`
	Realm          string       `json:"realm"`
	Region         *string      `json:"region"` // "EU", "US", ... (nil from addons before 0.5.0)
	Class          *string      `json:"class"`
	ClassName      *string      `json:"className"`
	Race           *string      `json:"race"`
	Faction        *string      `json:"faction"`
	Level          *int64       `json:"level"`
	Guild          *string      `json:"guild"`
	GuildRank      *string      `json:"guildRank"`
	GuildRankIndex *int64       `json:"guildRankIndex"` // 0 = Guild Master
	UpdatedAt      *int64       `json:"updatedAt"`
	Professions    []Profession `json:"professions"`
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
	// In-game guild rosters the account's characters saw (addon 0.6.0+).
	GuildRosters []GuildRoster `json:"guildRosters"`
	// Guild bank tabs the account's characters saw at the vault (addon 0.7.0+).
	GuildBanks []GuildBank `json:"guildBanks"`
}

type GuildBank struct {
	Guild     string    `json:"guild"`
	Region    *string   `json:"region"`
	ScannedAt int64     `json:"scannedAt"`
	Money     *int64    `json:"money"` // copper
	NumTabs   *int64    `json:"numTabs"`
	Tabs      []BankTab `json:"tabs"`
	// Which ranks can view which tabs (addon 0.7.0+).
	Access []TabAccess `json:"access"`
}

// TabAccess is one rank's view permission on one tab, either from the
// guild's settings (read by the Guild Master) or observed by a member.
type TabAccess struct {
	Rank    int64  `json:"rank"`
	Tab     int64  `json:"tab"`
	CanView bool   `json:"canView"`
	At      int64  `json:"at"`
	Source  string `json:"source"` // "settings" or "observed"
}

type BankTab struct {
	Index     int64      `json:"index"`
	Name      string     `json:"name"`
	ScannedAt int64      `json:"scannedAt"`
	Items     []BankItem `json:"items"`
}

type BankItem struct {
	Slot    int64   `json:"slot"`
	ItemID  int64   `json:"itemId"`
	Name    *string `json:"name"`
	Count   int64   `json:"count"`
	Quality *int64  `json:"quality"`
}

type GuildRoster struct {
	Guild     string         `json:"guild"`
	Region    *string        `json:"region"`
	ScannedAt int64          `json:"scannedAt"`
	Ranks     []GuildRank    `json:"ranks"`
	Members   []RosterMember `json:"members"`
}

type GuildRank struct {
	Index int64  `json:"index"`
	Name  string `json:"name"`
}

type RosterMember struct {
	Name      string `json:"name"` // as the game writes it, e.g. "Name-Realm"
	RankIndex int64  `json:"rankIndex"`
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
	p := &Payload{Characters: []Character{}, Instances: []Instance{}, GuildRosters: []GuildRoster{}, GuildBanks: []GuildBank{}}
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
			Name: name, Realm: realm, Region: str(ct, "region"),
			Class: str(ct, "class"), ClassName: str(ct, "className"),
			Race: str(ct, "race"), Faction: str(ct, "faction"),
			Level: num(ct, "level"), Guild: str(ct, "guild"), GuildRank: str(ct, "guildRank"), GuildRankIndex: num(ct, "guildRankIndex"),
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
					enchant, _ := r["enchant"].(bool)
					prof.Recipes = append(prof.Recipes, Recipe{
						RecipeID: id, Name: rname, ItemID: num(r, "itemID"),
						ClassID: num(r, "classID"), SubclassID: num(r, "subclassID"), EquipLoc: str(r, "equipLoc"),
						Enchant: enchant, Category: str(r, "category"), Reagents: reagentsOf(r),
					})
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
	p.GuildRosters = rostersOf(db)
	p.GuildBanks = banksOf(db)
	return p, nil
}

func banksOf(db luasv.Table) []GuildBank {
	out := []GuildBank{}
	banks, _ := db.Table("guildBanks")
	for _, key := range sortedKeys(banks) {
		bt, ok := banks.Table(key)
		guild, _ := bt.String("guild")
		scanned, okScan := bt.Int("scannedAt")
		if !ok || guild == "" || !okScan {
			continue
		}
		b := GuildBank{Guild: guild, Region: str(bt, "region"), ScannedAt: scanned, Money: num(bt, "money"), NumTabs: num(bt, "numTabs"), Tabs: []BankTab{}}
		tabs, _ := bt.Table("tabs")
		for _, tk := range sortedKeys(tabs) {
			tt, ok := tabs.Table(tk)
			idx, err := strconv.ParseInt(tk, 10, 64)
			tabScanned, okTab := tt.Int("scannedAt")
			if !ok || err != nil || !okTab {
				continue
			}
			name, _ := tt.String("name")
			tab := BankTab{Index: idx, Name: name, ScannedAt: tabScanned, Items: []BankItem{}}
			items, _ := tt.Table("items")
			for i := 1; ; i++ {
				it, ok := items.Table(strconv.Itoa(i))
				if !ok {
					break
				}
				id, okID := it.Int("itemID")
				if !okID || id <= 0 {
					continue
				}
				slot, _ := it.Int("slot")
				count, okCount := it.Int("count")
				if !okCount || count < 1 {
					count = 1
				}
				tab.Items = append(tab.Items, BankItem{Slot: slot, ItemID: id, Name: str(it, "name"), Count: count, Quality: num(it, "quality")})
			}
			b.Tabs = append(b.Tabs, tab)
		}
		sort.Slice(b.Tabs, func(i, j int) bool { return b.Tabs[i].Index < b.Tabs[j].Index })
		b.Access = append(accessOf(bt, "observed"), accessOf(bt, "permissions")...)
		out = append(out, b)
	}
	return out
}

func rostersOf(db luasv.Table) []GuildRoster {
	out := []GuildRoster{}
	rosters, _ := db.Table("guildRosters")
	for _, key := range sortedKeys(rosters) {
		rt, ok := rosters.Table(key)
		guild, _ := rt.String("guild")
		scanned, okScan := rt.Int("scannedAt")
		if !ok || guild == "" || !okScan {
			continue
		}
		r := GuildRoster{Guild: guild, Region: str(rt, "region"), ScannedAt: scanned, Ranks: []GuildRank{}, Members: []RosterMember{}}
		ranks, _ := rt.Table("ranks")
		for _, k := range sortedKeys(ranks) {
			idx, err := strconv.ParseInt(k, 10, 64)
			name, ok := ranks.String(k)
			if err == nil && ok {
				r.Ranks = append(r.Ranks, GuildRank{Index: idx, Name: name})
			}
		}
		sort.Slice(r.Ranks, func(i, j int) bool { return r.Ranks[i].Index < r.Ranks[j].Index })
		members, _ := rt.Table("members")
		for i := 1; ; i++ {
			m, ok := members.Table(strconv.Itoa(i))
			if !ok {
				break
			}
			name, _ := m.String("name")
			rank, okRank := m.Int("rankIndex")
			if name != "" && okRank {
				r.Members = append(r.Members, RosterMember{Name: name, RankIndex: rank})
			}
		}
		out = append(out, r)
	}
	return out
}

// accessOf flattens the addon's per-rank tab views. "observed" is keyed
// rank -> {at, tabs}; "permissions" is {at, ranks: rank -> tabs}.
func accessOf(bank luasv.Table, key string) []TabAccess {
	out := []TabAccess{}
	src, ok := bank.Table(key)
	if !ok {
		return out
	}
	source := "observed"
	ranks := src
	sharedAt, _ := src.Int("at")
	if key == "permissions" {
		source = "settings"
		ranks, _ = src.Table("ranks")
	}
	for _, rk := range sortedKeys(ranks) {
		rank, err := strconv.ParseInt(rk, 10, 64)
		entry, ok := ranks.Table(rk)
		if err != nil || !ok {
			continue
		}
		at := sharedAt
		tabs := entry
		if source == "observed" {
			at, _ = entry.Int("at")
			tabs, _ = entry.Table("tabs")
		}
		for _, tk := range sortedKeys(tabs) {
			tab, err := strconv.ParseInt(tk, 10, 64)
			canView, isBool := tabs[tk].(bool)
			if err == nil && isBool && at > 0 {
				out = append(out, TabAccess{Rank: rank, Tab: tab, CanView: canView, At: at, Source: source})
			}
		}
	}
	return out
}
