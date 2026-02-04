// Package main provides tests for nodelist source and scheduling.
package main

import (
	"context"
	"testing"
	"time"
)

// Test ScheduleNodes emits one job per node
func TestScheduleNodes_SingleJobPerNode(t *testing.T) {
	nodes := []NodeTarget{
		{
			Phone:      "79001234567",
			Zone:       2,
			Net:        5001,
			Node:       100,
			SystemName: "Test BBS 1",
			SysopName:  "Sysop One",
			Location:   "Moscow",
			IsCM:       true,
			Flags:      []string{"CM"}, // CM flag makes node callable 24/7
		},
		{
			Phone:      "79007654321",
			Zone:       2,
			Net:        5020,
			Node:       200,
			SystemName: "Test BBS 2",
			SysopName:  "Sysop Two",
			Location:   "SPB",
			IsCM:       true,
			Flags:      []string{"CM"}, // CM flag makes node callable 24/7
		},
	}

	operators := []OperatorConfig{
		{Name: "Primary", Prefix: "01"},
		{Name: "Secondary", Prefix: "02"},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	log := NewTestLogger(LoggingConfig{})
	jobsChan := ScheduleNodes(ctx, nodes, operators, log)

	var jobs []phoneJob
	for job := range jobsChan {
		jobs = append(jobs, job)
	}

	// Should have one job per node
	if len(jobs) != len(nodes) {
		t.Errorf("ScheduleNodes() emitted %d jobs, want %d", len(jobs), len(nodes))
	}

	// Each job should have full operators list
	for i, job := range jobs {
		if len(job.operators) != len(operators) {
			t.Errorf("job[%d].operators len = %d, want %d", i, len(job.operators), len(operators))
		}

		// Verify operators match
		for j, op := range job.operators {
			if op != operators[j] {
				t.Errorf("job[%d].operators[%d] = %v, want %v", i, j, op, operators[j])
			}
		}

		// Verify node info is populated
		if job.phone != nodes[i].Phone {
			t.Errorf("job[%d].phone = %q, want %q", i, job.phone, nodes[i].Phone)
		}
		if job.nodeAddress != nodes[i].Address() {
			t.Errorf("job[%d].nodeAddress = %q, want %q", i, job.nodeAddress, nodes[i].Address())
		}
		if job.testNum != i+1 {
			t.Errorf("job[%d].testNum = %d, want %d", i, job.testNum, i+1)
		}
	}
}

// Test ScheduleNodes with empty operators
func TestScheduleNodes_EmptyOperators(t *testing.T) {
	nodes := []NodeTarget{
		{
			Phone:      "79001234567",
			Zone:       2,
			Net:        5001,
			Node:       100,
			SystemName: "Test BBS",
			IsCM:       true,
			Flags:      []string{"CM"}, // CM flag makes node callable 24/7
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	log := NewTestLogger(LoggingConfig{})
	jobsChan := ScheduleNodes(ctx, nodes, nil, log)

	var jobs []phoneJob
	for job := range jobsChan {
		jobs = append(jobs, job)
	}

	if len(jobs) != 1 {
		t.Fatalf("ScheduleNodes() emitted %d jobs, want 1", len(jobs))
	}

	// Job should have empty operators slice
	if len(jobs[0].operators) != 0 {
		t.Errorf("job.operators = %v, want nil or empty", jobs[0].operators)
	}

	// Node info should still be populated
	if jobs[0].phone != nodes[0].Phone {
		t.Errorf("job.phone = %q, want %q", jobs[0].phone, nodes[0].Phone)
	}
}

// Test ScheduleNodes with empty nodes
func TestScheduleNodes_EmptyNodes(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	log := NewTestLogger(LoggingConfig{})
	jobsChan := ScheduleNodes(ctx, []NodeTarget{}, nil, log)

	var jobs []phoneJob
	for job := range jobsChan {
		jobs = append(jobs, job)
	}

	if len(jobs) != 0 {
		t.Errorf("ScheduleNodes() emitted %d jobs for empty nodes, want 0", len(jobs))
	}
}

// Test ScheduleNodes context cancellation
func TestScheduleNodes_Cancellation(t *testing.T) {
	// Create many nodes - CM nodes are immediately callable
	nodes := make([]NodeTarget, 10)
	for i := range nodes {
		nodes[i] = NodeTarget{
			Phone:      "7900123456" + string(rune('0'+i%10)),
			Zone:       2,
			Net:        5001,
			Node:       i,
			SystemName: "Test BBS",
			IsCM:       true,
			Flags:      []string{"CM"}, // CM flag makes node callable 24/7
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // Ensure cancel is always called
	log := NewTestLogger(LoggingConfig{})

	// Use a smaller buffer (1) to force blocking and test cancellation
	// But ScheduleNodes creates its own channel with buffer 100, so we need
	// to test that the channel closes after context cancellation
	jobsChan := ScheduleNodes(ctx, nodes, nil, log)

	// Read one job then cancel immediately
	<-jobsChan
	cancel()

	// Verify channel eventually closes (doesn't block forever)
	done := make(chan struct{})
	go func() {
		for range jobsChan {
			// Drain remaining jobs
		}
		close(done)
	}()

	select {
	case <-done:
		// Channel closed as expected
	case <-time.After(5 * time.Second):
		t.Error("ScheduleNodes() channel did not close after context cancellation")
	}
}

// Test NodeTarget.Address method
func TestNodeTarget_Address(t *testing.T) {
	tests := []struct {
		name string
		node NodeTarget
		want string
	}{
		{
			name: "standard address",
			node: NodeTarget{Zone: 2, Net: 5001, Node: 100},
			want: "2:5001/100",
		},
		{
			name: "zone 1",
			node: NodeTarget{Zone: 1, Net: 123, Node: 456},
			want: "1:123/456",
		},
		{
			name: "zero node",
			node: NodeTarget{Zone: 2, Net: 5020, Node: 0},
			want: "2:5020/0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.node.Address(); got != tt.want {
				t.Errorf("Address() = %q, want %q", got, tt.want)
			}
		})
	}
}

// Test ParseNodeAddress function
func TestParseNodeAddress(t *testing.T) {
	tests := []struct {
		name     string
		addr     string
		wantZone int
		wantNet  int
		wantNode int
		wantErr  bool
	}{
		{
			name:     "standard address",
			addr:     "2:5001/100",
			wantZone: 2,
			wantNet:  5001,
			wantNode: 100,
			wantErr:  false,
		},
		{
			name:     "zone 1",
			addr:     "1:123/456",
			wantZone: 1,
			wantNet:  123,
			wantNode: 456,
			wantErr:  false,
		},
		{
			name:    "point address rejected",
			addr:    "2:5001/100.1",
			wantErr: true,
		},
		{
			name:    "invalid format - no colon",
			addr:    "2-5001/100",
			wantErr: true,
		},
		{
			name:    "invalid format - no slash",
			addr:    "2:5001-100",
			wantErr: true,
		},
		{
			name:    "invalid zone",
			addr:    "abc:5001/100",
			wantErr: true,
		},
		{
			name:    "invalid net",
			addr:    "2:abc/100",
			wantErr: true,
		},
		{
			name:    "invalid node",
			addr:    "2:5001/abc",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			zone, net, node, err := ParseNodeAddress(tt.addr)

			if tt.wantErr {
				if err == nil {
					t.Error("ParseNodeAddress() error = nil, want error")
				}
				return
			}

			if err != nil {
				t.Errorf("ParseNodeAddress() error = %v", err)
				return
			}

			if zone != tt.wantZone {
				t.Errorf("zone = %d, want %d", zone, tt.wantZone)
			}
			if net != tt.wantNet {
				t.Errorf("net = %d, want %d", net, tt.wantNet)
			}
			if node != tt.wantNode {
				t.Errorf("node = %d, want %d", node, tt.wantNode)
			}
		})
	}
}

// Test UniquePhones function
func TestUniquePhones(t *testing.T) {
	tests := []struct {
		name  string
		nodes []NodeTarget
		want  []string
	}{
		{
			name:  "empty list",
			nodes: nil,
			want:  nil,
		},
		{
			name: "single node",
			nodes: []NodeTarget{
				{Phone: "79001234567"},
			},
			want: []string{"79001234567"},
		},
		{
			name: "unique phones",
			nodes: []NodeTarget{
				{Phone: "79001234567"},
				{Phone: "79007654321"},
			},
			want: []string{"79001234567", "79007654321"},
		},
		{
			name: "duplicate phones",
			nodes: []NodeTarget{
				{Phone: "79001234567"},
				{Phone: "79001234567"},
				{Phone: "79007654321"},
			},
			want: []string{"79001234567", "79007654321"},
		},
		{
			name: "preserves order",
			nodes: []NodeTarget{
				{Phone: "333"},
				{Phone: "111"},
				{Phone: "222"},
				{Phone: "111"},
			},
			want: []string{"333", "111", "222"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := UniquePhones(tt.nodes)

			if len(got) != len(tt.want) {
				t.Errorf("UniquePhones() len = %d, want %d\ngot: %v", len(got), len(tt.want), got)
				return
			}

			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("UniquePhones()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// Test BuildNodeLookupByPhone function
func TestBuildNodeLookupByPhone(t *testing.T) {
	nodes := []NodeTarget{
		{Phone: "79001234567", SystemName: "BBS1"},
		{Phone: "79007654321", SystemName: "BBS2"},
		{Phone: "79001234567", SystemName: "BBS1-Dup"}, // Duplicate phone
	}

	lookup := BuildNodeLookupByPhone(nodes)

	// Should have 2 unique phones
	if len(lookup) != 2 {
		t.Errorf("BuildNodeLookupByPhone() len = %d, want 2", len(lookup))
	}

	// First occurrence wins
	if n, ok := lookup["79001234567"]; !ok {
		t.Error("lookup missing 79001234567")
	} else if n.SystemName != "BBS1" {
		t.Errorf("lookup[79001234567].SystemName = %q, want %q", n.SystemName, "BBS1")
	}

	if n, ok := lookup["79007654321"]; !ok {
		t.Error("lookup missing 79007654321")
	} else if n.SystemName != "BBS2" {
		t.Errorf("lookup[79007654321].SystemName = %q, want %q", n.SystemName, "BBS2")
	}

	// Non-existent phone
	if _, ok := lookup["nonexistent"]; ok {
		t.Error("lookup found non-existent phone")
	}
}

// Test phoneJob structure
func TestPhoneJob_Fields(t *testing.T) {
	operators := []OperatorConfig{
		{Name: "Primary", Prefix: "01"},
		{Name: "Secondary", Prefix: "02"},
	}

	job := phoneJob{
		phone:          "79001234567",
		operators:      operators,
		testNum:        1,
		nodeAddress:    "2:5001/100",
		nodeSystemName: "Test BBS",
		nodeLocation:   "Moscow",
		nodeSysop:      "Test Sysop",
	}

	// Verify operators field
	if len(job.operators) != 2 {
		t.Errorf("job.operators len = %d, want 2", len(job.operators))
	}

	// Verify node info
	if job.nodeAddress != "2:5001/100" {
		t.Errorf("job.nodeAddress = %q, want %q", job.nodeAddress, "2:5001/100")
	}
	if job.nodeSystemName != "Test BBS" {
		t.Errorf("job.nodeSystemName = %q, want %q", job.nodeSystemName, "Test BBS")
	}

	// Backward compatibility: legacy single-operator fields
	legacyJob := phoneJob{
		phone:          "79001234567",
		operatorName:   "Legacy",
		operatorPrefix: "99",
		testNum:        1,
	}

	if legacyJob.operatorName != "Legacy" {
		t.Errorf("legacyJob.operatorName = %q, want %q", legacyJob.operatorName, "Legacy")
	}
	if legacyJob.operatorPrefix != "99" {
		t.Errorf("legacyJob.operatorPrefix = %q, want %q", legacyJob.operatorPrefix, "99")
	}
}
