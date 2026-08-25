package settings

import "testing"

// TestUILanguageValidation pins the asymmetry between the two language fields:
// Language must always name a supported locale, while UILanguage additionally
// accepts "" — the "follow the agent reply language" state every pre-i18n
// installation starts in.
func TestUILanguageValidation(t *testing.T) {
	cases := []struct {
		name  string
		patch Patch
		ok    bool
	}{
		{"ui empty is the follow state", Patch{UILanguage: strptr("")}, true},
		{"ui en", Patch{UILanguage: strptr("en")}, true},
		{"ui tr", Patch{UILanguage: strptr("tr")}, true},
		{"ui unknown rejected", Patch{UILanguage: strptr("de")}, false},
		{"agent language may not be empty", Patch{Language: strptr("")}, false},
	}
	for _, c := range cases {
		if err := Validate(c.patch); (err == nil) != c.ok {
			t.Errorf("%s: Validate err=%v, want ok=%v", c.name, err, c.ok)
		}
	}
}

// TestNormalizeUILanguage verifies a hand-edited bad UI locale degrades to the
// follow state instead of wedging the UI on a catalog that does not exist.
func TestNormalizeUILanguage(t *testing.T) {
	if got := normalize(Settings{UILanguage: "klingon"}).UILanguage; got != "" {
		t.Errorf("unknown uiLanguage not coerced to follow-state: %q", got)
	}
	if got := normalize(Settings{UILanguage: "en"}).UILanguage; got != "en" {
		t.Errorf("valid uiLanguage was dropped: %q", got)
	}
}

// TestEffectiveUILanguage covers the resolution order: explicit UI choice wins,
// otherwise the agent reply language, otherwise the default.
func TestEffectiveUILanguage(t *testing.T) {
	cases := []struct {
		in   Settings
		want string
	}{
		{Settings{Language: "tr", UILanguage: "en"}, "en"},
		{Settings{Language: "en", UILanguage: ""}, "en"},
		{Settings{Language: "tr", UILanguage: ""}, "tr"},
		{Settings{}, DefaultLanguage},
		{Settings{Language: "xx", UILanguage: "xx"}, DefaultLanguage},
	}
	for _, c := range cases {
		if got := EffectiveUILanguage(c.in); got != c.want {
			t.Errorf("EffectiveUILanguage(%+v) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestLanguageDisplayNameCoversEverySupportedLocale keeps the prompt-facing name
// table in step with SupportedLanguages: adding a locale without a display name
// would silently drop the "reply in <language>" directive from the system prompt.
func TestLanguageDisplayNameCoversEverySupportedLocale(t *testing.T) {
	for _, code := range SupportedLanguages {
		if LanguageDisplayName(code) == "" {
			t.Errorf("supported locale %q has no display name", code)
		}
	}
	if LanguageDisplayName("de") != "" {
		t.Error("unknown locale should have no display name")
	}
}
