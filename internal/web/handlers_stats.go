package web

import (
	"errors"
	"net/http"
	"time"

	"github.com/nodelistdb/internal/database"
	"github.com/nodelistdb/internal/storage"
	"github.com/nodelistdb/internal/version"
)

// statsPageData is the whole payload stats.html reads. It is one named struct
// rather than a literal per exit path because the template dereferences every
// field unconditionally: an error path that builds its own literal and forgets
// one aborts the render mid-document, which used to reach the client as a
// truncated page behind a 200.
type statsPageData struct {
	Title          string
	ActivePage     string
	Version        string
	Domain         string
	Error          error
	NoData         bool
	Stats          *database.NetworkStats
	AvailableDates []time.Time
	SelectedDate   string
	ActualDate     string
	DateAdjusted   bool
	NodeHistory    []storage.NodeCountByDate
	PointStats     *storage.PointStats
}

// StatsHandler renders the network statistics page for one nodelist date.
func (s *Server) StatsHandler(w http.ResponseWriter, r *http.Request) {
	domain := requestDomain(r)
	data := statsPageData{
		Title:          "Network Statistics",
		ActivePage:     "stats",
		Version:        version.GetVersionInfo(),
		Domain:         domain,
		NoData:         true,
		AvailableDates: []time.Time{},
	}

	availableDates, err := s.storage.GetAvailableDates(r.Context(), domain)
	if err != nil {
		display, handled := storageFailure("Stats: available dates", "Failed to get available dates: "+err.Error(), err)
		if handled {
			return
		}
		data.Error = display
		s.renderStatus(w, "stats", data, statusFor(data.Error))
		return
	}
	data.AvailableDates = availableDates

	// Same date resolution the browse pages use: no ?date= means the network's
	// latest, an unparseable one falls back to it, and a valid one snaps to the
	// nearest nodelist that exists.
	actualDate, rawDate, adjusted, err := s.resolveBrowseDate(r, domain)
	data.SelectedDate = rawDate
	if err != nil {
		// resolveBrowseDate wraps with %w, so the cause survives the trip up.
		if clientGone("Stats: date resolution", err) {
			return
		}
		data.Error = err
		s.renderStatus(w, "stats", data, statusFor(data.Error))
		return
	}
	data.ActualDate = actualDate.Format("2006-01-02")
	data.DateAdjusted = adjusted

	data.Stats, data.Error = s.storage.GetStats(r.Context(), actualDate, domain)
	if data.Error != nil {
		display, handled := storageFailure("Stats: network statistics", data.Error.Error(), data.Error)
		if handled {
			return
		}
		data.Error = display
	}
	data.NoData = data.Stats == nil || data.Stats.TotalNodes == 0
	if data.NoData && data.Error == nil {
		data.Error = errors.New("No nodelist data available. Please import nodelist files first.")
	}

	// The trend chart and the pointlist companion are both best-effort:
	// neither is worth failing the page over.
	data.NodeHistory, _ = s.storage.GetNodeCountHistory(r.Context(), domain)

	// A zero TotalPoints hides the pointlist tile. For the current view (no
	// explicit ?date=) anchor at the newest imported pointlist - the pointlist
	// feed can lag far behind the daily nodelist and an as-of-today snapshot
	// would go dark. Explicit historical dates stay strictly as-of.
	var pointAsOf *time.Time
	if rawDate != "" {
		pointAsOf = &actualDate
	}
	data.PointStats, _ = s.storage.GetPointStats(r.Context(), domain, pointAsOf)

	s.renderStatus(w, "stats", data, statusFor(data.Error))
}
