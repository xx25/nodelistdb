# i18n (Multi-Language) Support for NodelistDB Web Interface

## Context

The web interface currently has all user-visible text hardcoded in English across ~39 HTML templates, ~6 Go config structs, and ~5 JavaScript files. The goal is to add multi-language support so that:
1. An installation can be forced to a specific language via config
2. Browser language is auto-detected via Accept-Language
3. Users can switch language via a UI control
4. Only the web interface is translated (not REST API responses)

Likely languages: English + Russian (2-3 total).

## Difficulty Assessment: MODERATE-HIGH

**Scale of work:**
- ~200 hardcoded strings in HTML templates (nav, headings, labels, descriptions, empty states)
- ~250 strings in Go analytics config structs (`ProtocolPageConfig`, `IPv6PageConfig`, etc.)
- ~30 strings in JavaScript (datepicker months/days, tooltip labels, chart labels)
- **~480 total translatable strings**

**Favorable factors:**
- Configuration-driven analytics templates already centralize text in config structs -- just swap values for translation keys
- Unified templates (`unified_analytics.html`, `ipv6_analytics_generic.html`) mean fewer files to touch
- Embedded filesystem makes it easy to bundle translation files in the binary
- Only 2-3 languages needed, so no need for heavy i18n frameworks

**Unfavorable factors:**
- High string count requires careful extraction
- Some `PageSubtitle` fields contain HTML markup (`template.HTML`) mixed with translatable text
- Several standalone templates (don't use `base.html`) need individual changes
- JS files need a bridging mechanism for translations
- `dateDuration` FuncMap function returns English duration strings

## Recommended Approach: Lightweight Custom i18n with `golang.org/x/text/language`

No external i18n library needed. Use `golang.org/x/text/language` (already an indirect dependency) for Accept-Language parsing and language matching. Everything else is a simple key-value map.

### File Structure

```
internal/web/
  i18n/
    i18n.go                 -- Translator, Bundle types, loading logic
    translations/
      en.yaml               -- English strings (authoritative source)
      ru.yaml               -- Russian strings
```

Translation files embedded via `//go:embed`, keeping single-binary deployment.

### Translation File Format (flat YAML)

```yaml
# en.yaml
nav.home: "Home"
nav.search: "Search"
nav.statistics: "Statistics"
page.index.title: "FidoNet Nodelist Database"
page.index.subtitle: "Comprehensive FidoNet historical data and analytics"
analytics.binkp.title: "BinkP Enabled Nodes"
analytics.binkp.info: "This report shows nodes tested with BinkP over the last %d days."
js.copied: "Copied!"
datepicker.months: "January,February,March,..."
```

### Core Types

```go
// Translator -- per-language, holds a flat string map
type Translator struct {
    lang    language.Tag
    strings map[string]string
}
func (t *Translator) T(key string) string        // lookup or return key as fallback
func (t *Translator) Tf(key string, args ...any) string  // lookup + fmt.Sprintf

// Bundle -- holds all languages, provides resolution
type Bundle struct {
    translators map[language.Tag]*Translator
    matcher     language.Matcher
    defaultLang language.Tag
    forcedLang  *language.Tag   // non-nil if forced via config
}
func (b *Bundle) Resolve(r *http.Request) *Translator
func (b *Bundle) AvailableLanguages() []LanguageInfo
```

### Language Resolution Priority

1. **Forced language** (config `web.language: "ru"`) -- always wins if set
2. **Cookie `lang`** -- set by language switcher
3. **Query param `?lang=xx`** -- sets cookie + redirects (for shareable links)
4. **Accept-Language header** -- parsed via `language.ParseAcceptLanguage`
5. **Default** (`web.default_language`, defaults to `"en"`)

### Template Integration

Add `t` and `tf` to the template FuncMap at startup. They take a `*Translator` as first arg:

```go
funcMap["t"] = func(tr *i18n.Translator, key string) string { return tr.T(key) }
funcMap["tf"] = func(tr *i18n.Translator, key string, args ...any) string { return tr.Tf(key, args...) }
```

In templates:
```html
<a href="/">{{t .Tr "nav.home"}}</a>
<h1>{{t .Tr "page.index.title"}}</h1>
<p>{{tf .Tr "common.found_n_nodes" .Count .StatsHeading}}</p>
```

Every handler data struct gets a `Tr *i18n.Translator` field. Existing fields remain unchanged.

### Analytics Config Structs

Change from English text to translation keys:

```go
// Before:
config := ProtocolPageConfig{
    PageTitle: "BinkP Enabled Nodes",
    ...
}

// After:
config := ProtocolPageConfig{
    PageTitle: "analytics.binkp.title",  // translation key
    ...
}
```

Templates change from `{{.Config.PageTitle}}` to `{{t .Tr .Config.PageTitle}}`.

`PageSubtitle` changes from `template.HTML` to `string` (translation key). HTML markup moves to the template where it belongs.

### JavaScript i18n

Inline a JSON object in `base.html` (and standalone templates):

```html
<script>
window.i18n = {
    "copied": "{{t .Tr "js.copied"}}",
    "today": "{{t .Tr "datepicker.today"}}",
    ...
};
</script>
```

JS files use `window.i18n.copied` instead of hardcoded `"Copied!"`. Only ~30 strings, <2KB.

### Language Switcher

Small control in nav or footer:

```html
<div class="lang-switcher">
  {{range .Languages}}
  <a href="?lang={{.Code}}" {{if eq .Code $.Lang}}class="active"{{end}}>{{.NativeName}}</a>
  {{end}}
</div>
```

Clicking sets a cookie (`lang=ru`, Max-Age=1 year, SameSite=Lax) and redirects back.

### Config Addition

```yaml
web:
  language: ""            # Force language (empty = auto-detect)
  default_language: "en"  # Fallback when detection fails
```

### Server Struct Change

```go
type Server struct {
    storage     storage.Operations
    templates   map[string]*template.Template
    templatesFS embed.FS
    staticFS    embed.FS
    linksLoader *links.Loader
    i18nBundle  *i18n.Bundle  // NEW
}
```

## Critical Files to Modify

| File | Change |
|------|--------|
| `internal/web/i18n/` (new) | New package: Translator, Bundle, embedded translations |
| `internal/web/i18n/translations/en.yaml` (new) | ~480 English strings extracted from templates+handlers |
| `internal/web/i18n/translations/ru.yaml` (new) | Russian translations |
| `internal/web/templates.go` | Add `t`, `tf` to FuncMap |
| `internal/web/handlers.go` | Add `i18nBundle` to Server, `Tr` to handler data structs |
| `internal/web/handlers_analytics.go` | Change config string values to translation keys, pass `Tr` |
| `internal/web/analytics_config.go` | `PageSubtitle` from `template.HTML` to `string` key |
| `internal/web/templates/nav.html` | `{{t .Tr "nav.home"}}` etc. |
| `internal/web/templates/base.html` | `<html lang="{{.Lang}}">`, `window.i18n` block |
| `internal/web/templates/index.html` | Replace hardcoded English |
| All 39 templates | Replace hardcoded strings with `{{t .Tr "key"}}` |
| `internal/web/static/datepicker.js` | Use `window.i18n` for months, days, labels |
| `internal/web/static/tooltip.js` | Use `window.i18n` for button labels |
| `internal/web/static/app.js` | Use `window.i18n` for alert text |
| `cmd/server/main.go` | Pass web config to Server constructor |
| `internal/config/config.go` (or equivalent) | Add `WebConfig` struct |

## Implementation Phases

### Phase 1: Infrastructure (~4-6h)
- Create `internal/web/i18n/` package
- Extract all English strings to `en.yaml` (the biggest single task)
- Add `WebConfig` to config
- Add `t`, `tf` to template FuncMap
- Add i18n middleware, `Tr` field to all handler structs
- Wire Bundle into Server

At this point: app works identically, all keys resolve to English.

### Phase 2: Migrate nav + base + language switcher (~1-2h)
- `nav.html`, `base.html`, `footer.html`
- Add `<html lang="{{.Lang}}">`
- Add language switcher UI
- Add `window.i18n` JS block

### Phase 3: Migrate all templates (~6-8h)
- Page by page: index, search, stats, analytics, reachability, etc.
- Partials: filters, error display, action buttons, cells
- Mechanical but numerous

### Phase 4: Migrate JavaScript (~1-2h)
- datepicker.js, tooltip.js, app.js, software-charts.js

### Phase 5: Migrate analytics config structs (~3-4h)
- Change ProtocolPageConfig, IPv6PageConfig, etc. values to keys
- Update `processInfoText` to accept Translator
- Change PageSubtitle from template.HTML to string key

### Phase 6: Add Russian translation (~4-6h)
- Create `ru.yaml`, translate all ~480 strings
- Test with config force, cookie, and Accept-Language

**Estimated total: ~20-28 hours of implementation work.**

## Verification Plan

1. `make test` -- existing tests still pass
2. Start server with default config -- English works as before
3. Set `web.language: "ru"` -- all pages render in Russian
4. Remove forced language, visit with browser set to Russian -- auto-detection works
5. Click language switcher -- cookie is set, language changes, persists across pages
6. Visit with `?lang=en` -- overrides cookie, shareable
7. Check JS-heavy pages (datepicker, tooltips, charts) render translated strings
8. Check analytics pages (BinkP, IFCICO, etc.) use translated config text
9. Check edge cases: missing translation key falls back to key string (not crash)
