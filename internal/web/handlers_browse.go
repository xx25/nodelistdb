package web

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/nodelistdb/internal/database"
	"github.com/nodelistdb/internal/storage"
	"github.com/nodelistdb/internal/version"
)

// browseData is the unified template payload for every level of the FidoNet
// hierarchy browser (/browse). Only the slice matching Level is populated.
type browseData struct {
	Title          string
	ActivePage     string
	Level          string // "zones" | "regions" | "nets" | "nodes"
	Version        string
	Error          string
	AvailableDates []time.Time
	SelectedDate   string // raw ?date= value, for the date <select>
	ActualDate     string // resolved nodelist date (YYYY-MM-DD)
	DateAdjusted   bool   // true if the requested date was snapped to a nearby one
	DateQuery      string // "" or "?date=...&domain=..." suffix carried on nav links

	// Multi-network support.
	Domain   string              // selected FTN network (always set)
	Networks []storage.DomainInfo // all networks, for the selector on the top level

	// Breadcrumb context.
	Zone       int
	Region     int
	RegionName string
	Net        int
	NetName    string
	HasRegion  bool // false for the "no region" bucket (region 0)

	// Rows. Only the slice for the current Level is populated.
	Zones   []storage.BrowseZone
	Regions []storage.BrowseRegion
	Nets    []storage.BrowseNet
	Nodes   []database.Node
}

// resolveBrowseDate reads the optional ?date= query parameter and returns the
// nearest available nodelist date within the selected network. With no
// parameter it returns the network's latest date.
func (s *Server) resolveBrowseDate(r *http.Request, domain string) (actual time.Time, raw string, adjusted bool, err error) {
	raw = r.URL.Query().Get("date")
	if raw == "" {
		actual, err = s.storage.GetLatestStatsDate(domain)
		return actual, raw, false, err
	}
	parsed, perr := time.Parse("2006-01-02", raw)
	if perr != nil {
		actual, err = s.storage.GetLatestStatsDate(domain)
		return actual, raw, true, err
	}
	actual, err = s.storage.GetNearestAvailableDate(parsed, domain)
	if err != nil {
		return actual, raw, false, err
	}
	return actual, raw, !actual.Equal(parsed), nil
}

// newBrowseData builds the common scaffolding (date, nav state) shared by every
// browse page.
func (s *Server) newBrowseData(r *http.Request, level, title string) (*browseData, time.Time, bool) {
	data := &browseData{
		Title:      title,
		ActivePage: "browse",
		Level:      level,
		Version:    version.GetVersionInfo(),
		Domain:     requestDomain(r),
	}
	data.AvailableDates, _ = s.storage.GetAvailableDates(data.Domain)

	actualDate, raw, adjusted, err := s.resolveBrowseDate(r, data.Domain)
	data.SelectedDate = raw
	if err != nil {
		data.Error = "Failed to determine nodelist date: " + err.Error()
		return data, time.Time{}, false
	}
	data.ActualDate = actualDate.Format("2006-01-02")
	data.DateAdjusted = adjusted

	// Carry the selected date and non-default network on every nav link
	var params []string
	if raw != "" {
		params = append(params, "date="+data.ActualDate)
	}
	if data.Domain != database.DefaultDomain {
		params = append(params, "domain="+data.Domain)
	}
	if len(params) > 0 {
		data.DateQuery = "?" + strings.Join(params, "&")
	}
	return data, actualDate, true
}

// pathSegments returns the path components that follow the given prefix.
func pathSegments(path, prefix string) []string {
	rest := strings.Trim(strings.TrimPrefix(path, prefix), "/")
	if rest == "" {
		return nil
	}
	return strings.Split(rest, "/")
}

// renderBrowse executes the browse template, mapping render failures to a 500.
func (s *Server) renderBrowse(w http.ResponseWriter, data *browseData) {
	if err := s.templates["browse"].Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// BrowseZonesHandler renders the top level of the hierarchy browser: every zone
// present in the selected nodelist.
func (s *Server) BrowseZonesHandler(w http.ResponseWriter, r *http.Request) {
	data, actualDate, ok := s.newBrowseData(r, "zones", "Browse Nodelist")
	if !ok {
		s.renderBrowse(w, data)
		return
	}

	// The top level also lists all networks so users can switch between them
	data.Networks, _ = s.storage.GetDomains()

	zones, err := s.storage.GetBrowseZones(actualDate, data.Domain)
	if err != nil {
		data.Error = "Failed to load zones: " + err.Error()
		s.renderBrowse(w, data)
		return
	}
	data.Zones = zones
	s.renderBrowse(w, data)
}

// BrowseZoneHandler renders the regions within a single zone.
// Path: /browse/zone/{zone}
func (s *Server) BrowseZoneHandler(w http.ResponseWriter, r *http.Request) {
	data, actualDate, ok := s.newBrowseData(r, "regions", "Browse Nodelist")
	if !ok {
		s.renderBrowse(w, data)
		return
	}

	parts := pathSegments(r.URL.Path, "/browse/zone/")
	if len(parts) < 1 {
		data.Error = "Missing zone number"
		s.renderBrowse(w, data)
		return
	}
	zone, err := strconv.Atoi(parts[0])
	if err != nil {
		data.Error = "Invalid zone number: " + parts[0]
		s.renderBrowse(w, data)
		return
	}
	data.Zone = zone
	data.Title = "Browse Zone " + parts[0]

	regions, err := s.storage.GetBrowseRegions(actualDate, zone, data.Domain)
	if err != nil {
		data.Error = "Failed to load regions: " + err.Error()
		s.renderBrowse(w, data)
		return
	}
	data.Regions = regions
	s.renderBrowse(w, data)
}

// BrowseRegionHandler renders the nets within a single zone+region.
// Path: /browse/region/{zone}/{region} — region 0 is the "no region" bucket.
func (s *Server) BrowseRegionHandler(w http.ResponseWriter, r *http.Request) {
	data, actualDate, ok := s.newBrowseData(r, "nets", "Browse Nodelist")
	if !ok {
		s.renderBrowse(w, data)
		return
	}

	parts := pathSegments(r.URL.Path, "/browse/region/")
	if len(parts) < 2 {
		data.Error = "Expected /browse/region/{zone}/{region}"
		s.renderBrowse(w, data)
		return
	}
	zone, err1 := strconv.Atoi(parts[0])
	region, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		data.Error = "Invalid zone or region number"
		s.renderBrowse(w, data)
		return
	}
	data.Zone = zone
	data.Region = region
	data.HasRegion = region != 0
	data.Title = "Browse Zone " + parts[0] + " Region " + parts[1]

	// Pick up the region-coordinator name for the breadcrumb/heading.
	if data.HasRegion {
		if regions, err := s.storage.GetBrowseRegions(actualDate, zone, data.Domain); err == nil {
			for _, rg := range regions {
				if rg.Region == region {
					data.RegionName = rg.Name
					break
				}
			}
		}
	}

	nets, err := s.storage.GetBrowseNets(actualDate, zone, region, data.Domain)
	if err != nil {
		data.Error = "Failed to load nets: " + err.Error()
		s.renderBrowse(w, data)
		return
	}
	data.Nets = nets
	s.renderBrowse(w, data)
}

// BrowseNetHandler renders every node within a single zone+net for the selected
// nodelist date. Path: /browse/net/{zone}/{net}
func (s *Server) BrowseNetHandler(w http.ResponseWriter, r *http.Request) {
	data, actualDate, ok := s.newBrowseData(r, "nodes", "Browse Nodelist")
	if !ok {
		s.renderBrowse(w, data)
		return
	}

	parts := pathSegments(r.URL.Path, "/browse/net/")
	if len(parts) < 2 {
		data.Error = "Expected /browse/net/{zone}/{net}"
		s.renderBrowse(w, data)
		return
	}
	zone, err1 := strconv.Atoi(parts[0])
	net, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		data.Error = "Invalid zone or net number"
		s.renderBrowse(w, data)
		return
	}
	data.Zone = zone
	data.Net = net
	data.Title = "Browse Net " + parts[0] + ":" + parts[1]

	nodes, err := s.storage.GetBrowseNodes(actualDate, zone, net, data.Domain)
	if err != nil {
		data.Error = "Failed to load nodes: " + err.Error()
		s.renderBrowse(w, data)
		return
	}
	data.Nodes = nodes

	// Derive breadcrumb context (region + host name) from the entries themselves
	// so no extra queries are needed.
	for _, n := range nodes {
		if n.Region != nil && *n.Region != 0 {
			data.Region = *n.Region
			data.HasRegion = true
		}
		if n.NodeType == "Host" && n.SystemName != "" {
			data.NetName = n.SystemName
		}
	}
	s.renderBrowse(w, data)
}
