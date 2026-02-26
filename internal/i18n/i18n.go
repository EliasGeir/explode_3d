package i18n

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"strings"
)

//go:embed locales/*.json
var localesFS embed.FS

const (
	LangIT      = "it"
	LangEN      = "en"
	DefaultLang = LangIT
)

type contextKey struct{}

// translations maps lang -> key -> translated string.
var translations map[string]map[string]string

// Load reads the embedded JSON locale files. Call once at startup.
func Load() {
	translations = make(map[string]map[string]string)
	for _, lang := range []string{LangIT, LangEN} {
		data, err := localesFS.ReadFile("locales/" + lang + ".json")
		if err != nil {
			log.Fatalf("i18n: cannot read %s.json: %v", lang, err)
		}
		var raw map[string]any
		if err := json.Unmarshal(data, &raw); err != nil {
			log.Fatalf("i18n: cannot parse %s.json: %v", lang, err)
		}
		flat := make(map[string]string)
		flatten("", raw, flat)
		translations[lang] = flat
	}
	log.Printf("i18n: loaded %d IT keys, %d EN keys", len(translations[LangIT]), len(translations[LangEN]))
}

// flatten recursively flattens nested JSON into dot-notation keys.
func flatten(prefix string, m map[string]any, out map[string]string) {
	for k, v := range m {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}
		switch val := v.(type) {
		case string:
			out[key] = val
		case map[string]any:
			flatten(key, val, out)
		}
	}
}

// WithLocale stores the locale in the context.
func WithLocale(ctx context.Context, lang string) context.Context {
	return context.WithValue(ctx, contextKey{}, lang)
}

// GetLocale returns the locale from the context, or DefaultLang.
func GetLocale(ctx context.Context) string {
	if lang, ok := ctx.Value(contextKey{}).(string); ok && lang != "" {
		return lang
	}
	return DefaultLang
}

// T translates a key using the locale from ctx.
// Optional args are used with fmt.Sprintf if the translated string contains %s/%d etc.
// Fallback chain: current lang -> EN -> IT -> return key itself.
func T(ctx context.Context, key string, args ...any) string {
	lang := GetLocale(ctx)

	// Try current language
	if m, ok := translations[lang]; ok {
		if s, ok := m[key]; ok {
			return format(s, args)
		}
	}
	// Fallback to IT (default)
	if lang != LangIT {
		if m, ok := translations[LangIT]; ok {
			if s, ok := m[key]; ok {
				return format(s, args)
			}
		}
	}
	// Fallback to EN
	if lang != LangEN {
		if m, ok := translations[LangEN]; ok {
			if s, ok := m[key]; ok {
				return format(s, args)
			}
		}
	}
	return key
}

func format(s string, args []any) string {
	if len(args) == 0 || !strings.Contains(s, "%") {
		return s
	}
	return fmt.Sprintf(s, args...)
}
