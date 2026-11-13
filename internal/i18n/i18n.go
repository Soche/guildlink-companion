// Package i18n has the companion's texts in English and Swedish, for the
// settings page (its template and its script) and the tray menu.
package i18n

import (
	_ "embed"
	"encoding/json"
	"strconv"
	"strings"
)

//go:embed strings.json
var raw []byte

var catalog map[string]map[string]string

func init() {
	if err := json.Unmarshal(raw, &catalog); err != nil {
		panic("i18n: " + err.Error())
	}
}

// Default is used when nothing else says which language to use.
const Default = "en"

// Language is one the page can be shown in, with the flag the toggle uses.
type Language struct {
	Code string
	Name string
	Flag string // SVG
}

// Languages, in the order the toggle shows them.
var Languages = []Language{
	{"sv", "Svenska", `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 10" width="24" height="15" aria-hidden="true"><rect width="16" height="10" fill="#006aa7"/><rect x="5" width="2" height="10" fill="#fecc02"/><rect y="4" width="16" height="2" fill="#fecc02"/></svg>`},
	{"en", "English", `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 60 30" width="24" height="12" aria-hidden="true"><clipPath id="gb-s"><path d="M0,0 v30 h60 v-30 z"/></clipPath><clipPath id="gb-t"><path d="M30,15 h30 v15 z v15 h-30 z h-30 v-15 z v-15 h30 z"/></clipPath><g clip-path="url(#gb-s)"><path d="M0,0 v30 h60 v-30 z" fill="#012169"/><path d="M0,0 L60,30 M60,0 L0,30" stroke="#fff" stroke-width="6"/><path d="M0,0 L60,30 M60,0 L0,30" clip-path="url(#gb-t)" stroke="#c8102e" stroke-width="4"/><path d="M30,0 v30 M0,15 h60" stroke="#fff" stroke-width="10"/><path d="M30,0 v30 M0,15 h60" stroke="#c8102e" stroke-width="6"/></g></svg>`},
}

// Supported reports whether there are texts for lang.
func Supported(lang string) bool {
	_, ok := catalog[lang]
	return ok
}

// Strings is every text in lang, falling back to English.
func Strings(lang string) map[string]string {
	if !Supported(lang) {
		lang = Default
	}
	return catalog[lang]
}

// T is one text, with {name} placeholders filled from name, value pairs.
func T(lang, key string, args ...string) string {
	s, ok := Strings(lang)[key]
	if !ok {
		s = catalog[Default][key]
	}
	for i := 0; i+1 < len(args); i += 2 {
		s = strings.ReplaceAll(s, "{"+args[i]+"}", args[i+1])
	}
	return s
}

// FromAcceptLanguage picks the browser's preferred language among those
// supported, or "" when it prefers none of them.
func FromAcceptLanguage(header string) string {
	best, bestQ := "", -1.0
	for _, part := range strings.Split(header, ",") {
		fields := strings.Split(strings.TrimSpace(part), ";")
		lang := strings.ToLower(strings.SplitN(strings.TrimSpace(fields[0]), "-", 2)[0])
		if !Supported(lang) {
			continue
		}
		q := 1.0
		for _, f := range fields[1:] {
			if v, ok := strings.CutPrefix(strings.TrimSpace(f), "q="); ok {
				if n, err := strconv.ParseFloat(v, 64); err == nil {
					q = n
				}
			}
		}
		if q > bestQ {
			best, bestQ = lang, q
		}
	}
	return best
}
