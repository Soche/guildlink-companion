package i18n

import (
	"regexp"
	"testing"
)

func TestEveryLanguageHasEveryText(t *testing.T) {
	placeholders := regexp.MustCompile(`\{\w+\}`)
	for _, l := range Languages {
		if !Supported(l.Code) {
			t.Fatalf("no texts for %s", l.Code)
		}
	}
	for code, texts := range catalog {
		for key, en := range catalog[Default] {
			s, ok := texts[key]
			if !ok {
				t.Errorf("%s is missing %q", code, key)
				continue
			}
			if a, b := placeholders.FindAllString(en, -1), placeholders.FindAllString(s, -1); len(a) != len(b) {
				t.Errorf("%s %q has placeholders %v, English has %v", code, key, b, a)
			}
		}
		for key := range texts {
			if _, ok := catalog[Default][key]; !ok {
				t.Errorf("%s has %q, which English doesn't", code, key)
			}
		}
	}
}

func TestT(t *testing.T) {
	if got := T("sv", "lastUpload", "when", "12:00"); got != "Senaste uppladdning 12:00" {
		t.Errorf("got %q", got)
	}
	if got := T("xx", "save"); got != "Save" {
		t.Errorf("unknown language: %q", got)
	}
}

func TestFromAcceptLanguage(t *testing.T) {
	for header, want := range map[string]string{
		"sv-SE,sv;q=0.9,en-US;q=0.8": "sv",
		"en-GB,en;q=0.9,sv;q=0.8":    "en",
		"de-DE,de;q=0.9":             "",
		"":                           "",
		"de,sv;q=0.5":                "sv",
	} {
		if got := FromAcceptLanguage(header); got != want {
			t.Errorf("%q: got %q, want %q", header, got, want)
		}
	}
}
