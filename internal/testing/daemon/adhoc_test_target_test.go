package daemon

import "testing"

// TestSingleNode used to file an ad-hoc `host:port` test under 2:5001/5001,
// which is not a placeholder but a live node — thousands of nodelist rows, in
// today's issue, tested every cycle. A stranger's handshake landed in its
// history as a well-formed row on a real address, indistinguishable downstream
// from a genuine result. Zone 0 is not a valid FTN zone, so the replacement
// cannot name anyone.
func TestAdHocAddressCannotNameARealNode(t *testing.T) {
	if adHocZone != 0 {
		t.Errorf("ad-hoc zone is %d; only zone 0 is guaranteed absent from every FTN nodelist, "+
			"and any real zone risks landing on a live node as 2:5001/5001 did", adHocZone)
	}
	// node != 0 is the filter most reports apply before any nodelist gate, so a
	// node number of 0 keeps ad-hoc rows out even of a report that forgets one.
	if adHocNode != 0 {
		t.Errorf("ad-hoc node is %d, want 0 so the `node != 0` filter excludes it", adHocNode)
	}
}

// The three tables that have to agree about a protocol: what flag announces it,
// what port to fall back to, and — in test_executor.go — whether to test it at
// all. They were three separate switch statements, which is how -test-proto
// vmodem came to probe port 3141 on a node announcing only IBN:24554.
func TestProtocolTablesAgree(t *testing.T) {
	for protocol := range protocolFlags {
		if _, ok := protocolDefaultPorts[protocol]; !ok {
			t.Errorf("protocol %q has an announcement flag but no default port; "+
				"TestSingleNode would reject it as unsupported", protocol)
		}
	}
	for protocol := range protocolDefaultPorts {
		if _, ok := protocolFlags[protocol]; !ok {
			t.Errorf("protocol %q has a default port but no announcement flag; "+
				"a hand-run would skip the announced-port lookup and the not-announced warning", protocol)
		}
	}
}
