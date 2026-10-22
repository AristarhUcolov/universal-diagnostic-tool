// Package i18n holds the small localisation helpers shared by the data
// tables (parameter names, fault descriptions) and backend messages.
// The user interface keeps its own dictionaries; this package only
// covers text that originates in Go code or embedded data.
package i18n

import "sort"

// Languages lists the languages the program ships with.
var Languages = []string{"en", "ru"}

// Default is the fallback language.
const Default = "en"

// Text is one string translated into several languages, keyed by a
// two-letter code ("en", "ru").
type Text map[string]string

// T builds a Text from English and Russian strings.
func T(en, ru string) Text {
	t := Text{}
	if en != "" {
		t["en"] = en
	}
	if ru != "" {
		t["ru"] = ru
	}
	return t
}

// Get returns the text in lang, falling back to English, then Russian,
// then any language that is present.
func (t Text) Get(lang string) string {
	if len(t) == 0 {
		return ""
	}
	if s, ok := t[lang]; ok && s != "" {
		return s
	}
	if s, ok := t["en"]; ok && s != "" {
		return s
	}
	if s, ok := t["ru"]; ok && s != "" {
		return s
	}
	keys := make([]string, 0, len(t))
	for k := range t {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if t[k] != "" {
			return t[k]
		}
	}
	return ""
}

// Empty reports whether no language has a non-empty value.
func (t Text) Empty() bool { return t.Get(Default) == "" }

// Normalize maps a locale such as "ru-RU" or "en_US.UTF-8" to one of
// the supported languages.
func Normalize(locale string) string {
	if len(locale) >= 2 {
		code := locale[:2]
		for _, l := range Languages {
			if l == code {
				return l
			}
		}
		// Ukrainian and Belarusian speakers usually read Russian
		// technical documentation; prefer it over English.
		if code == "uk" || code == "be" || code == "kk" {
			return "ru"
		}
	}
	return Default
}
