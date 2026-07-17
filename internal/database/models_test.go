package database

import (
	"testing"
	"time"
)

func TestComputeFtsIdDomainSuffix(t *testing.T) {
	date := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)

	fido := Node{Zone: 2, Net: 5001, Node: 100, NodelistDate: date}
	fido.ComputeFtsId()
	if fido.FtsId != "2:5001/100@2026-07-10#0" {
		t.Errorf("fidonet fts_id = %q, want legacy format without domain suffix", fido.FtsId)
	}

	explicit := Node{Zone: 2, Net: 5001, Node: 100, NodelistDate: date, Domain: DefaultDomain}
	explicit.ComputeFtsId()
	if explicit.FtsId != fido.FtsId {
		t.Errorf("explicit fidonet fts_id = %q, must equal default %q", explicit.FtsId, fido.FtsId)
	}

	fsx := Node{Zone: 21, Net: 1, Node: 100, NodelistDate: date, Domain: "fsxnet", ConflictSequence: 1}
	fsx.ComputeFtsId()
	if fsx.FtsId != "21:1/100@2026-07-10#1@fsxnet" {
		t.Errorf("fsxnet fts_id = %q, want domain-suffixed format", fsx.FtsId)
	}
}
