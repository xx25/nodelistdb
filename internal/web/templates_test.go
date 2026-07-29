package web

import (
	"html/template"
	"io/fs"
	"path"
	"strings"
	"testing"
)

// TestEveryPageTemplateLoadsWithItsChrome pins the loader's contract: one
// template per page file, each carrying the shared chrome and every partial.
//
// The previous loader kept the partial list in a hand-maintained slice and
// only warned when one failed to parse, so a partial could go missing from a
// page and the page would still render - just without that cell. It also
// re-parsed the whole chrome once per page, and the per-page object had to be
// built in one specific order or it rendered blank.
func TestEveryPageTemplateLoadsWithItsChrome(t *testing.T) {
	s := &Server{templates: make(map[string]*template.Template), templatesFS: TemplatesFS}
	if err := s.loadTemplates(); err != nil {
		t.Fatalf("loading templates: %v", err)
	}

	pages, err := fs.Glob(TemplatesFS, "templates/*.html")
	if err != nil {
		t.Fatal(err)
	}
	var want []string
	for _, p := range pages {
		name := strings.TrimSuffix(path.Base(p), ".html")
		if name == "base" || name == "nav" || name == "footer" {
			continue
		}
		want = append(want, name)
	}
	if len(s.templates) != len(want) {
		t.Errorf("loaded %d templates, want %d (one per page file)", len(s.templates), len(want))
	}

	partials, err := fs.Glob(TemplatesFS, "templates/partials/*.html")
	if err != nil {
		t.Fatal(err)
	}

	for _, name := range want {
		tmpl, ok := s.templates[name]
		if !ok {
			t.Errorf("page template %q was not loaded", name)
			continue
		}
		// The page's own body must be the {{template "base" .}} call, not an
		// empty string - the failure mode of parsing the page as an associated
		// template instead of onto the clone is a silently blank page.
		if body := tmpl.Tree; body == nil || strings.TrimSpace(body.Root.String()) == "" {
			t.Errorf("template %q has an empty body; it would render a blank page", name)
		}
		for _, chrome := range []string{"base", "nav", "footer"} {
			if tmpl.Lookup(chrome) == nil {
				t.Errorf("template %q is missing the %q block", name, chrome)
			}
		}
		for _, p := range partials {
			partial := strings.TrimSuffix(path.Base(p), ".html")
			if tmpl.Lookup(partial) == nil {
				t.Errorf("template %q is missing the %q partial", name, partial)
			}
		}
	}
}

// TestLoadTemplatesReportsFailure covers the error return that replaced
// log.Fatalf: a Server pointed at a filesystem with no templates must say so
// rather than exit the process or come up half-loaded.
func TestLoadTemplatesReportsFailure(t *testing.T) {
	s := &Server{templates: make(map[string]*template.Template), templatesFS: StaticFS}
	if err := s.loadTemplates(); err == nil {
		t.Error("loadTemplates succeeded against a filesystem with no templates")
	}
}
