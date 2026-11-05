package addon

import (
	"sort"
	"strconv"

	"guildlink/companion/internal/luasv"
)

// Raid is one raid as one character saw it: who was in the group, what was
// looted (rare and better) and the boss encounters (addon 0.10.0+).
type Raid struct {
	InstanceID int64           `json:"instanceId"`
	Name       string          `json:"name"`
	MaxPlayers *int64          `json:"maxPlayers"`
	Region     *string         `json:"region"`
	StartedAt  int64           `json:"startedAt"`
	EndedAt    int64           `json:"endedAt"`
	Members    []RaidMember    `json:"members"`
	Loot       []RaidLoot      `json:"loot"`
	Encounters []RaidEncounter `json:"encounters"`
}

type RaidMember struct {
	Name      string  `json:"name"`
	Class     *string `json:"class"`
	FirstSeen int64   `json:"firstSeen"`
	LastSeen  int64   `json:"lastSeen"`
}

type RaidLoot struct {
	At       int64   `json:"at"`
	Name     string  `json:"name"` // who got it
	ItemID   int64   `json:"itemId"`
	ItemName *string `json:"itemName"`
	Quality  *int64  `json:"quality"`
	Count    int64   `json:"count"`
}

type RaidEncounter struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Success bool   `json:"success"`
	Size    *int64 `json:"size"`
	At      int64  `json:"at"`
}

// list reads a Lua array (keys "1", "2", ...) in order.
func list(parent luasv.Table, key string) []luasv.Table {
	t, ok := parent.Table(key)
	if !ok {
		return nil
	}
	var out []luasv.Table
	for i := 1; ; i++ {
		e, ok := t.Table(strconv.Itoa(i))
		if !ok {
			return out
		}
		out = append(out, e)
	}
}

func raidsOf(db luasv.Table) []Raid {
	out := []Raid{}
	raids, _ := db.Table("raids")
	for _, key := range sortedKeys(raids) {
		rt, ok := raids.Table(key)
		if !ok {
			continue
		}
		id, okID := rt.Int("instanceID")
		name, _ := rt.String("name")
		started, okStart := rt.Int("startedAt")
		ended, okEnd := rt.Int("endedAt")
		if !okID || id <= 0 || name == "" || !okStart || !okEnd {
			continue
		}
		r := Raid{InstanceID: id, Name: name, MaxPlayers: num(rt, "maxPlayers"), Region: str(rt, "region"),
			StartedAt: started, EndedAt: ended, Members: []RaidMember{}, Loot: []RaidLoot{}, Encounters: []RaidEncounter{}}
		members, _ := rt.Table("members")
		for _, mk := range sortedKeys(members) {
			m, ok := members.Table(mk)
			first, ok1 := m.Int("firstSeen")
			last, ok2 := m.Int("lastSeen")
			if ok && ok1 && ok2 && mk != "" {
				r.Members = append(r.Members, RaidMember{Name: mk, Class: str(m, "class"), FirstSeen: first, LastSeen: last})
			}
		}
		for _, l := range list(rt, "loot") {
			at, ok1 := l.Int("at")
			who, _ := l.String("name")
			item, ok2 := l.Int("itemID")
			if !ok1 || !ok2 || who == "" || item <= 0 {
				continue
			}
			count, ok := l.Int("count")
			if !ok || count < 1 {
				count = 1
			}
			r.Loot = append(r.Loot, RaidLoot{At: at, Name: who, ItemID: item, ItemName: str(l, "itemName"), Quality: num(l, "quality"), Count: count})
		}
		for _, e := range list(rt, "encounters") {
			eid, ok1 := e.Int("id")
			at, ok2 := e.Int("at")
			ename, _ := e.String("name")
			if !ok1 || !ok2 || ename == "" {
				continue
			}
			success, _ := e["success"].(bool)
			r.Encounters = append(r.Encounters, RaidEncounter{ID: eid, Name: ename, Success: success, Size: num(e, "size"), At: at})
		}
		out = append(out, r)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].StartedAt < out[j].StartedAt })
	return out
}

// BankLog is one tab's log as the game showed it; Ago is hours before ScannedAt.
type BankLog struct {
	Tab       int64          `json:"tab"`
	ScannedAt int64          `json:"scannedAt"`
	Entries   []BankLogEntry `json:"entries"`
}

type BankLogEntry struct {
	Type     string  `json:"type"` // deposit, withdraw, move, ...
	Name     *string `json:"name"`
	ItemID   *int64  `json:"itemId"`
	ItemName *string `json:"itemName"`
	Count    *int64  `json:"count"`
	Tab1     *int64  `json:"tab1"`
	Tab2     *int64  `json:"tab2"`
	Ago      int64   `json:"ago"`
}

type MoneyLog struct {
	ScannedAt int64           `json:"scannedAt"`
	Entries   []MoneyLogEntry `json:"entries"`
}

type MoneyLogEntry struct {
	Type   string  `json:"type"` // deposit, withdraw, repair, buyTab, ...
	Name   *string `json:"name"`
	Amount int64   `json:"amount"` // copper
	Ago    int64   `json:"ago"`
}

func bankLogsOf(bank luasv.Table) []BankLog {
	out := []BankLog{}
	logs, _ := bank.Table("log")
	for _, k := range sortedKeys(logs) {
		lt, ok := logs.Table(k)
		tab, err := strconv.ParseInt(k, 10, 64)
		scanned, okScan := lt.Int("scannedAt")
		if !ok || err != nil || !okScan {
			continue
		}
		l := BankLog{Tab: tab, ScannedAt: scanned, Entries: []BankLogEntry{}}
		for _, e := range list(lt, "entries") {
			kind, _ := e.String("type")
			ago, okAgo := e.Int("ago")
			if kind == "" || !okAgo {
				continue
			}
			l.Entries = append(l.Entries, BankLogEntry{Type: kind, Name: str(e, "name"), ItemID: num(e, "itemID"), ItemName: str(e, "itemName"),
				Count: num(e, "count"), Tab1: num(e, "tab1"), Tab2: num(e, "tab2"), Ago: ago})
		}
		out = append(out, l)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Tab < out[j].Tab })
	return out
}

func moneyLogOf(bank luasv.Table) *MoneyLog {
	mt, ok := bank.Table("moneyLog")
	if !ok {
		return nil
	}
	scanned, ok := mt.Int("scannedAt")
	if !ok {
		return nil
	}
	m := &MoneyLog{ScannedAt: scanned, Entries: []MoneyLogEntry{}}
	for _, e := range list(mt, "entries") {
		kind, _ := e.String("type")
		amount, okAmount := e.Int("amount")
		ago, okAgo := e.Int("ago")
		if kind != "" && okAmount && okAgo {
			m.Entries = append(m.Entries, MoneyLogEntry{Type: kind, Name: str(e, "name"), Amount: amount, Ago: ago})
		}
	}
	return m
}
