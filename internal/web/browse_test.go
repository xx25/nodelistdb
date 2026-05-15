package web

import (
	"bytes"
	"strings"
	"testing"

	"github.com/nodelistdb/internal/database"
	"github.com/nodelistdb/internal/storage"
)

// TestBrowseTemplateRenders verifies that the hierarchy browser template loads
// and renders cleanly at every drill-down level.
func TestBrowseTemplateRenders(t *testing.T) {
	// New() runs loadTemplates(), which log.Fatalf's on a broken template.
	s := New(nil, TemplatesFS, StaticFS)

	tmpl := s.templates["browse"]
	if tmpl == nil {
		t.Fatal("browse template was not loaded")
	}

	region := 25
	cases := []struct {
		name string
		data browseData
		want string
	}{
		{
			name: "zones",
			data: browseData{
				Level: "zones", ActualDate: "2026-05-01",
				Zones: []storage.BrowseZone{{Zone: 2, NodeCount: 10, Name: "Zone 2 Coordinator"}},
			},
			want: "/browse/zone/2",
		},
		{
			name: "regions",
			data: browseData{
				Level: "regions", Zone: 2, ActualDate: "2026-05-01",
				Regions: []storage.BrowseRegion{
					{Region: 25, NodeCount: 5, Name: "RC25", Location: "United Kingdom"},
					{Region: 0, NodeCount: 2},
				},
			},
			want: "/browse/region/2/25",
		},
		{
			name: "nets",
			data: browseData{
				Level: "nets", Zone: 2, Region: 25, HasRegion: true, RegionName: "UK", ActualDate: "2026-05-01",
				Nets: []storage.BrowseNet{{Net: 250, NodeCount: 3, Name: "Host", Location: "London"}},
			},
			want: "/browse/net/2/250",
		},
		{
			name: "nodes",
			data: browseData{
				Level: "nodes", Zone: 2, Net: 250, Region: 25, HasRegion: true, ActualDate: "2026-05-01",
				Nodes: []database.Node{{
					Zone: 2, Net: 250, Node: 1, NodeType: "Node",
					SystemName: "Test_BBS", Flags: []string{"CM", "IBN"}, Region: &region,
					RawLine: ",1,Test_BBS,London,John_Doe,-Unpublished-,300,CM,XA,IBN,V34,V90C",
				}},
			},
			want: ",Test_BBS,London,John_Doe,-Unpublished-,300,CM,XA,IBN,V34,V90C",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := tmpl.Execute(&buf, tc.data); err != nil {
				t.Fatalf("execute browse template: %v", err)
			}
			if !strings.Contains(buf.String(), tc.want) {
				t.Errorf("rendered %s level missing %q", tc.name, tc.want)
			}
		})
	}
}
