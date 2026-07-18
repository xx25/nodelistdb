package web

import (
	"bytes"
	"html/template"
	"strings"
	"testing"

	"github.com/nodelistdb/internal/storage"
)

// renderTestDetail loads the real embedded templates and renders the
// test_detail page for the given result, exactly as the web handler does.
func renderTestDetail(t *testing.T, tr *storage.NodeTestResult) string {
	t.Helper()
	s := &Server{templates: make(map[string]*template.Template), templatesFS: TemplatesFS}
	s.loadTemplates()
	tmpl, ok := s.templates["test_detail"]
	if !ok {
		t.Fatal("test_detail template not loaded")
	}
	data := map[string]interface{}{
		"Title":      "Test Result Details",
		"Version":    "test",
		"ActivePage": "reachability",
		"TestResult": tr,
		"NodeInfo":   nil,
		"Address":    tr.Address,
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		t.Fatalf("test_detail Execute error: %v", err)
	}
	return buf.String()
}

func TestVModemSectionRendersConformant(t *testing.T) {
	out := renderTestDetail(t, &storage.NodeTestResult{
		Address:          "2:423/81",
		VModemTested:     true,
		VModemSuccess:    true,
		VModemConformant: true,
		VModemVariant:    "vmp",
		VModemSoftware:   "VMODEM (Gwinn VMP)",
	})
	for _, want := range []string{"VModem (IVM) Protocol", "vmp", "Genuine VMODEM"} {
		if !strings.Contains(out, want) {
			t.Errorf("conformant render missing %q", want)
		}
	}
}

func TestVModemSectionRendersMismatch(t *testing.T) {
	out := renderTestDetail(t, &storage.NodeTestResult{
		Address:          "3:54/0",
		VModemTested:     true,
		VModemSuccess:    true,
		VModemConformant: false,
		VModemVariant:    "emsi-telnet",
		VModemSoftware:   "Platinum Xpress/WINServer",
		VModemSystemName: "The File Bank BBS",
		VModemAddresses:  []string{"3:54/0@fidonet", "21:1/100@fsxnet"},
	})
	for _, want := range []string{
		"emsi-telnet",
		"Not VMODEM",
		"Platinum Xpress/WINServer",
		"The File Bank BBS",
		"21:1/100@fsxnet",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("mismatch render missing %q", want)
		}
	}
}

// A node that wasn't vmodem-tested must not render the section at all.
func TestVModemSectionHiddenWhenUntested(t *testing.T) {
	out := renderTestDetail(t, &storage.NodeTestResult{Address: "1:1/1", VModemTested: false})
	if strings.Contains(out, "VModem (IVM) Protocol") {
		t.Error("VModem section should be hidden when not tested")
	}
}
