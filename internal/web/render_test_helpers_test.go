package web

import (
	"regexp"
	"strings"
	"testing"
)

var (
	tagRE   = regexp.MustCompile(`(?is)<(/?)(div|table)\b([^>]*)>`)
	classRE = regexp.MustCompile(`(?i)class="([^"]*)"`)
)

// assertTablesWrapped fails if the rendered page puts a <table> anywhere
// outside a .table-responsive or .table-container div. The page container
// is overflow: hidden, so a bare table wider than the screen is silently cut
// off on a phone rather than scrollable; the wrapper is what makes it
// scroll. The hop table on the PING node page is exempt: it is narrow by
// construction and nested in a card.
func assertTablesWrapped(t *testing.T, html string) {
	t.Helper()
	var stack []bool // one entry per open <div>: true when it is a scroll wrapper
	wrapped := 0
	for _, m := range tagRE.FindAllStringSubmatch(html, -1) {
		closing, tag, attrs := m[1] == "/", strings.ToLower(m[2]), m[3]
		class := ""
		if cm := classRE.FindStringSubmatch(attrs); cm != nil {
			class = " " + cm[1] + " "
		}
		switch {
		case tag == "div" && !closing:
			isWrapper := strings.Contains(class, " table-responsive ") || strings.Contains(class, " table-container ")
			stack = append(stack, isWrapper)
			if isWrapper {
				wrapped++
			}
		case tag == "div" && closing:
			if len(stack) > 0 {
				if stack[len(stack)-1] {
					wrapped--
				}
				stack = stack[:len(stack)-1]
			}
		case tag == "table" && !closing:
			if strings.Contains(class, " hop-table ") {
				continue
			}
			if wrapped == 0 {
				t.Errorf("table outside a scroll wrapper (would be clipped on phones): <table%s>", attrs)
			}
		}
	}
}
