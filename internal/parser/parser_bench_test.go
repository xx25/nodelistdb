package parser

import (
	"fmt"
	"os"
	"strconv"
	"testing"
	
	"nodelistdb/internal/database"
)

// createTestNodelistData creates a sample nodelist content for benchmarking
func createTestNodelistData() string {
	return `;A FidoNet Nodelist for Friday, January 15, 2024 -- Day number 15 : 12345
;S 
Zone,2,North_America,Somewhere_USA,User,-Unpublished-,300,CM,INA:fidonet.example.org,IBN:24554
Host,234,Test_Host,Test_Location,Sysop_Name,1-234-567-8900,9600,CM,INA:host.example.org,IBN:24554
Hub,56,Test_Hub,Hub_Location,Hub_Sysop,1-234-567-8901,9600,CM,INA:hub.example.org,IBN:24554,ITN:23
,1,Test_Node1,Node_Location1,Node_Sysop1,1-234-567-8902,9600,CM,INA:node1.example.org,IBN:24554
,2,Test_Node2,Node_Location2,Node_Sysop2,1-234-567-8903,9600,CM,INA:node2.example.org,IBN:24554
,3,Test_Node3,Node_Location3,Node_Sysop3,1-234-567-8904,9600,CM,INA:node3.example.org,IBN:24554
,4,Test_Node4,Node_Location4,Node_Sysop4,1-234-567-8905,2400,CM,V32B,V42B,XA
,5,Test_Node5,Node_Location5,Node_Sysop5,1-234-567-8906,14400,CM,V32B,V42B,ZYX
,6,Test_Node6,Node_Location6,Node_Sysop6,1-234-567-8907,28800,CM,V34,V42B,XA
,7,Test_Node7,Node_Location7,Node_Sysop7,1-234-567-8908,33600,CM,V34,V42B,INA:node7.example.org
Hub,78,Another_Hub,Hub_Location2,Hub_Sysop2,1-234-567-8909,9600,CM,INA:hub2.example.org,IBN:24554
,10,Big_Node,Big_Location,Big_Sysop,1-234-567-8910,9600,CM,INA:big.example.org,IBN:24554,ITN:23
,11,Fast_Node,Fast_Location,Fast_Sysop,1-234-567-8911,57600,CM,V90,V42B,INA:fast.example.org
,12,Modern_Node,Modern_Location,Modern_Sysop,1-234-567-8912,9600,CM,INA:modern.example.org,IBN:24554,IFT:21
Pvt,999,Private_Node,Private_Location,Private_Sysop,-Unlisted-,9600,,XA
Down,888,Down_Node,Down_Location,Down_Sysop,1-234-567-8913,9600,,XA
Hold,777,Hold_Node,Hold_Location,Hold_Sysop,1-234-567-8914,9600,,XA
`
}

// createLargeNodelistData creates a larger nodelist for more realistic benchmarking
func createLargeNodelistData(nodeCount int) string {
	header := `;A FidoNet Nodelist for Friday, January 15, 2024 -- Day number 15 : 12345
;S 
Zone,1,North_America,Somewhere_USA,User,-Unpublished-,300,CM,INA:fidonet.example.org,IBN:24554
Host,234,Test_Host,Test_Location,Sysop_Name,1-234-567-8900,9600,CM,INA:host.example.org,IBN:24554
`

	var data string = header
	for i := 1; i <= nodeCount; i++ {
		// Vary the node data to make it realistic
		nodeNum := strconv.Itoa(i)
		phone := fmt.Sprintf("1-555-000-%04d", 1000+i)
		var nodeLine string
		switch i % 5 {
		case 0:
			nodeLine = `,` + nodeNum + `,Node` + nodeNum + `,Location` + nodeNum + `,Sysop` + nodeNum + `,` + phone + `,9600,CM,INA:node` + nodeNum + `.example.org,IBN:24554`
		case 1:
			nodeLine = `,` + nodeNum + `,Node` + nodeNum + `,Location` + nodeNum + `,Sysop` + nodeNum + `,` + phone + `,14400,CM,V32B,V42B,XA`
		case 2:
			nodeLine = `,` + nodeNum + `,Node` + nodeNum + `,Location` + nodeNum + `,Sysop` + nodeNum + `,` + phone + `,28800,CM,V34,V42B,INA:node` + nodeNum + `.example.org,IBN:24554,ITN:23`
		case 3:
			nodeLine = `,` + nodeNum + `,Node` + nodeNum + `,Location` + nodeNum + `,Sysop` + nodeNum + `,` + phone + `,33600,CM,V90,V42B,INA:node` + nodeNum + `.example.org,IFT:21`
		case 4:
			nodeLine = `Pvt,` + nodeNum + `,Private` + nodeNum + `,PrivLoc` + nodeNum + `,PrivSysop` + nodeNum + `,-Unlisted-,9600,,XA`
		}
		data += nodeLine + "\n"
	}
	return data
}

// createTestFile creates a temporary test file with nodelist data
func createTestFile(t *testing.B, content string) string {
	tmpFile, err := os.CreateTemp("", "bench_nodelist_*.txt")
	if err != nil {
		t.Fatal(err)
	}
	
	if _, err := tmpFile.WriteString(content); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		t.Fatal(err)
	}
	tmpFile.Close()
	
	// Clean up file after benchmark
	t.Cleanup(func() { os.Remove(tmpFile.Name()) })
	
	return tmpFile.Name()
}

// BenchmarkParserSmallFile benchmarks parsing a small nodelist file
func BenchmarkParserSmallFile(b *testing.B) {
	content := createTestNodelistData()
	testFile := createTestFile(b, content)
	
	parser := New(false) // verbose=false for benchmarking
	
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		nodes, err := parser.ParseFile(testFile)
		if err != nil {
			b.Fatal(err)
		}
		if len(nodes) == 0 {
			b.Fatal("expected nodes to be parsed")
		}
	}
}

// BenchmarkParserMediumFile benchmarks parsing a medium-sized nodelist file (1000 nodes)
func BenchmarkParserMediumFile(b *testing.B) {
	content := createLargeNodelistData(1000)
	testFile := createTestFile(b, content)
	
	parser := New(false)
	
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		nodes, err := parser.ParseFile(testFile)
		if err != nil {
			b.Fatal(err)
		}
		if len(nodes) == 0 {
			b.Fatal("expected nodes to be parsed")
		}
	}
}

// BenchmarkParserLargeFile benchmarks parsing a large nodelist file (10000 nodes)
func BenchmarkParserLargeFile(b *testing.B) {
	content := createLargeNodelistData(10000)
	testFile := createTestFile(b, content)
	
	parser := New(false)
	
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		nodes, err := parser.ParseFile(testFile)
		if err != nil {
			b.Fatal(err)
		}
		if len(nodes) == 0 {
			b.Fatal("expected nodes to be parsed")
		}
	}
}

// BenchmarkMapReuse benchmarks the map reuse optimization
func BenchmarkMapReuse(b *testing.B) {
	content := createTestNodelistData()
	testFile := createTestFile(b, content)
	
	parser := New(false)
	
	b.Run("WithMapReuse", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()
		
		for i := 0; i < b.N; i++ {
			// The optimized parser with map reuse
			nodes, err := parser.ParseFile(testFile)
			if err != nil {
				b.Fatal(err)
			}
			if len(nodes) == 0 {
				b.Fatal("expected nodes to be parsed")
			}
		}
	})
}

// BenchmarkSlicePreallocation benchmarks slice pre-allocation benefits
func BenchmarkSlicePreallocation(b *testing.B) {
	testSizes := []int{100, 1000, 5000}
	
	for _, size := range testSizes {
		b.Run(fmt.Sprintf("Nodes_%d", size), func(b *testing.B) {
			content := createLargeNodelistData(size)
			testFile := createTestFile(b, content)
			
			parser := New(false)
			
			b.ResetTimer()
			b.ReportAllocs()
			
			for i := 0; i < b.N; i++ {
				nodes, err := parser.ParseFile(testFile)
				if err != nil {
					b.Fatal(err)
				}
				if len(nodes) != size+2 { // +2 for Zone and Host entries
					b.Fatalf("expected %d nodes, got %d", size+2, len(nodes))
				}
			}
		})
	}
}

// BenchmarkMemoryFootprint measures the memory footprint of parsing
func BenchmarkMemoryFootprint(b *testing.B) {
	content := createLargeNodelistData(5000)
	testFile := createTestFile(b, content)
	
	parser := New(false)
	
	b.ResetTimer()
	b.ReportAllocs()
	
	var nodes []database.Node
	for i := 0; i < b.N; i++ {
		var err error
		nodes, err = parser.ParseFile(testFile)
		if err != nil {
			b.Fatal(err)
		}
	}
	
	// Prevent compiler optimization
	_ = nodes
}

// BenchmarkFlagParsing specifically benchmarks flag parsing performance
func BenchmarkFlagParsing(b *testing.B) {
	parser := New(false)
	
	testCases := []struct {
		name  string
		flags string
	}{
		{"Simple", "CM,XA"},
		{"WithModem", "CM,V32B,V42B,XA"},
		{"WithInternet", "CM,INA:node.example.org,IBN:24554,ITN:23"},
		{"Complex", "CM,V34,V42B,INA:node.example.org,IBN:24554,ITN:23,IFT:21,IFC:60179"},
		{"VeryComplex", "CM,V90,V42B,MNP,INA:node.example.org,IBN:24554,ITN:23,IFT:21,IFC:60179,IPX,NEC,ZYX,XA"},
	}
	
	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			b.ResetTimer()
			b.ReportAllocs()
			
			for i := 0; i < b.N; i++ {
				flags, _ := parser.parseFlagsWithConfig(tc.flags)
				if len(flags) == 0 {
					b.Fatal("expected flags to be parsed")
				}
			}
		})
	}
}

// BenchmarkDateExtraction benchmarks date parsing performance
func BenchmarkDateExtraction(b *testing.B) {
	parser := New(false)
	
	testHeaders := []string{
		";A FidoNet Nodelist for Friday, January 15, 2024 -- Day number 15 : 12345",
		";S FidoNet Nodelist for Saturday, March 02, 2024 -- Day 62",
		";A Friday, December 25, 2024 -- Day number 360 : ABCD",
	}
	
	for i, header := range testHeaders {
		b.Run(fmt.Sprintf("Header_%d", i+1), func(b *testing.B) {
			b.ResetTimer()
			b.ReportAllocs()
			
			for j := 0; j < b.N; j++ {
				date, dayNum, err := parser.extractDateFromLine(header)
				if err != nil {
					b.Fatal(err)
				}
				if date.IsZero() || dayNum == 0 {
					b.Fatal("expected valid date and day number")
				}
			}
		})
	}
}