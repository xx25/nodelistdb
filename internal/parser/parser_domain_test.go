package parser

import (
	"os"
	"testing"
)

func TestParserStampsDomain(t *testing.T) {
	const sample = "../../FSXNET.191"
	if _, err := os.Stat(sample); err != nil {
		t.Skipf("sample nodelist %s not present", sample)
	}

	p := New(false)
	p.SetDomain("fsxnet")

	result, err := p.ParseFileWithCRC(sample)
	if err != nil {
		t.Fatalf("ParseFileWithCRC: %v", err)
	}
	if len(result.Nodes) == 0 {
		t.Fatal("no nodes parsed from FSXNET.191")
	}
	for _, n := range result.Nodes[:5] {
		if n.Domain != "fsxnet" {
			t.Fatalf("node %d:%d/%d domain = %q, want fsxnet", n.Zone, n.Net, n.Node, n.Domain)
		}
	}
	if result.NodelistDate.IsZero() {
		t.Error("nodelist date not extracted from FSXNET.191")
	}
	t.Logf("parsed %d fsxnet nodes, date %s (day %d)", len(result.Nodes), result.NodelistDate.Format("2006-01-02"), result.DayNumber)
}
