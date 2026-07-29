package web

import (
	"fmt"
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

	// Multi-network support: the selected FTN network (always set; explicit
	// ?domain= wins over the global switcher cookie).
	Domain string

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

	// Pointlist snapshot counts per boss node (nodes level only; empty when
	// the net has no points as of the browsed date).
	PointCounts map[int]uint64
}

// resolveBrowseDate reads the optional ?date= query parameter and returns the
// nearest available nodelist date within the selected network. With no
// parameter it returns the network's latest date.
func (s *Server) resolveBrowseDate(r *http.Request, domain string) (actual time.Time, raw string, adjusted bool, err error) {
	raw = r.URL.Query().Get("date")
	if raw == "" {
		actual, err = s.storage.GetLatestStatsDate(r.Context(), domain)
		if err != nil {
			return actual, raw, false, fmt.Errorf("failed to find latest nodelist date: %w", err)
		}
		return actual, raw, false, nil
	}
	parsed, perr := time.Parse("2006-01-02", raw)
	if perr != nil {
		actual, err = s.storage.GetLatestStatsDate(r.Context(), domain)
		if err != nil {
			return actual, raw, true, fmt.Errorf("invalid date format and failed to get latest date: %w", err)
		}
		return actual, raw, true, nil
	}
	actual, err = s.storage.GetNearestAvailableDate(r.Context(), parsed, domain)
	if err != nil {
		return actual, raw, false, fmt.Errorf("failed to find available date: %w", err)
	}
	return actual, raw, !actual.Equal(parsed), nil
}

// newBrowseData builds the common scaffolding (date, nav state) shared by every
// browse page.
// newBrowseData builds the shared payload for the four browse levels.
//
// ok == false means the handler must not carry on. It comes back two ways: with
// a non-nil data carrying an Error for the caller to render, or with a nil data
// when the request was cancelled and there is nothing left to render to.
func (s *Server) newBrowseData(r *http.Request, level, title string) (*browseData, time.Time, bool) {
	data := &browseData{
		Title:      title,
		ActivePage: "browse",
		Level:      level,
		Version:    version.GetVersionInfo(),
		Domain:     requestDomain(r),
	}
	data.AvailableDates, _ = s.storage.GetAvailableDates(r.Context(), data.Domain)

	actualDate, raw, adjusted, err := s.resolveBrowseDate(r, data.Domain)
	data.SelectedDate = raw
	if err != nil {
		// resolveBrowseDate wraps with %w, so the cause survives the trip up.
		// A nil payload is how "the client is gone, render nothing" is told
		// apart from "render the error page": both are ok == false, and
		// rendering a page for a closed connection is exactly the work the
		// cancellation migration exists to skip.
		if clientGone("Browse date resolution", err) {
			return nil, time.Time{}, false
		}
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
	s.render(w, "browse", data)
}

// BrowseZonesHandler renders the top level of the hierarchy browser: every zone
// present in the selected nodelist.
func (s *Server) BrowseZonesHandler(w http.ResponseWriter, r *http.Request) {
	data, actualDate, ok := s.newBrowseData(r, "zones", "Browse Nodelist")
	if !ok {
		if data != nil {
			s.renderBrowse(w, data)
		}
		return
	}

	zones, err := s.storage.GetBrowseZones(r.Context(), actualDate, data.Domain)
	if err != nil {
		display, handled := storageFailure("Browse zones", "Failed to load zones: "+err.Error(), err)
		if handled {
			return
		}
		data.Error = display.Error()
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
		if data != nil {
			s.renderBrowse(w, data)
		}
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

	regions, err := s.storage.GetBrowseRegions(r.Context(), actualDate, zone, data.Domain)
	if err != nil {
		display, handled := storageFailure("Browse regions", "Failed to load regions: "+err.Error(), err)
		if handled {
			return
		}
		data.Error = display.Error()
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
		if data != nil {
			s.renderBrowse(w, data)
		}
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
		if regions, err := s.storage.GetBrowseRegions(r.Context(), actualDate, zone, data.Domain); err == nil {
			for _, rg := range regions {
				if rg.Region == region {
					data.RegionName = rg.Name
					break
				}
			}
		}
	}

	nets, err := s.storage.GetBrowseNets(r.Context(), actualDate, zone, region, data.Domain)
	if err != nil {
		display, handled := storageFailure("Browse nets", "Failed to load nets: "+err.Error(), err)
		if handled {
			return
		}
		data.Error = display.Error()
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
		if data != nil {
			s.renderBrowse(w, data)
		}
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

	nodes, err := s.storage.GetBrowseNodes(r.Context(), actualDate, zone, net, data.Domain)
	if err != nil {
		display, handled := storageFailure("Browse nodes", "Failed to load nodes: "+err.Error(), err)
		if handled {
			return
		}
		data.Error = display.Error()
		s.renderBrowse(w, data)
		return
	}
	data.Nodes = nodes

	// Pointlist counts (one snapshot GROUP BY per page). The current view
	// (no explicit ?date=) anchors at the newest imported pointlist rather
	// than the nodelist date — the pointlist feed can lag behind the daily
	// nodelist; explicit historical dates stay strictly as-of.
	var pointAsOf *time.Time
	if data.SelectedDate != "" {
		pointAsOf = &actualDate
	}
	data.PointCounts, _ = s.storage.GetPointCountsByNet(r.Context(), data.Domain, zone, net, pointAsOf)

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
