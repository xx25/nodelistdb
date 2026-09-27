package web

import (
	"net/http"
	"sort"
	"strings"

	"github.com/nodelistdb/internal/logging"
	"github.com/nodelistdb/internal/storage"
	"github.com/nodelistdb/internal/version"
)

// EmailFlagReference describes one email flag for the on-page reference table.
type EmailFlagReference struct {
	Flag string
	// Standard is true for the five flags FTS-5001 defines. EMA and EVY are
	// observed in real nodelists but appear in no FTSC document.
	Standard bool
	// ReceiptRequired marks the flags FTS-5001 requires to answer with a
	// receipt within 24 hours.
	ReceiptRequired bool
	Description     string
	// Count is how many nodes on the current nodelist advertise it.
	Count int
	// Color is the chart series colour, so the reference table and the chart
	// agree on identity.
	Color string
}

// emailFlagReferences is the static per-flag documentation, in presentation
// order. Descriptions paraphrase FTS-5001 rev 4 section "Email Flags".
var emailFlagReferences = []EmailFlagReference{
	{
		Flag:        "IEM",
		Standard:    true,
		Description: "Unspecified mail tunnelling method, or the default email address for the other email flags. On its own it names no wire format.",
	},
	{
		Flag:        "IMI",
		Standard:    true,
		Description: "MIME encoding of mail bundles. The encoding is named; the message profile is not, so interoperability needs a peer agreement.",
	},
	{
		Flag:            "ITX",
		Standard:        true,
		ReceiptRequired: true,
		Description:     "TransX email tunnelling with receipts enabled. No complete FTSC wire specification exists for TransX.",
	},
	{
		Flag:            "ISE",
		Standard:        true,
		ReceiptRequired: true,
		Description:     "SEAT (FTS-1025), the only method here with a complete FTSC wire specification. Should accompany IUC and/or IMI.",
	},
	{
		Flag:        "IUC",
		Standard:    true,
		Description: "UUencoding of mail bundles. As with IMI, the encoding is named but the message profile is not.",
	},
	{
		Flag:        "EMA",
		Description: "Email transport, method unspecified. Not defined by any FTSC document; recorded because it occurs in real nodelists.",
	},
	{
		Flag:        "EVY",
		Description: "Voyager-compatible email transport. Not defined by any FTSC document; recorded because it occurs in real nodelists.",
	},
}

// emailSeriesColors is the validated categorical palette, assigned to flags in
// fixed order so a flag keeps its colour no matter which flags are present.
var emailSeriesColors = map[string]string{
	"IEM": "#2a78d6",
	"IMI": "#eb6834",
	"ITX": "#1baf7a",
	"ISE": "#eda100",
	"IUC": "#e87ba4",
	"EMA": "#008300",
	"EVY": "#4a3aa7",
}

// EmailSummaryStats aggregates the current nodelist's email capabilities.
type EmailSummaryStats struct {
	TotalNodes int
	// EmailOnly counts nodes with no other published transport.
	EmailOnly int
	// Resolved counts nodes for which at least one address could be worked out.
	Resolved   int
	Unresolved int
	// ReceiptCapable counts nodes advertising ITX or ISE.
	ReceiptCapable int
	// NonStandardOnly counts nodes whose only email flags are EMA/EVY.
	NonStandardOnly int
	Malformed       int
	DistinctDomains int
	TotalAddresses  int

	FlagReferences []EmailFlagReference
	DomainCounts   []EmailDomainCount
}

// EmailDomainCount is one mail domain and how many addresses use it.
type EmailDomainCount struct {
	Domain string
	Count  int
}

func computeEmailStats(nodes []storage.EmailCapableNode) EmailSummaryStats {
	stats := EmailSummaryStats{TotalNodes: len(nodes)}

	flagCounts := make(map[string]int)
	domainCounts := make(map[string]int)
	addresses := make(map[string]bool)

	for _, n := range nodes {
		if n.EmailOnly {
			stats.EmailOnly++
		}
		if n.Resolved {
			stats.Resolved++
		} else {
			stats.Unresolved++
		}
		if n.ReceiptCapable {
			stats.ReceiptCapable++
		}
		if n.HasNonStandardFlag && !n.HasStandardMethod {
			stats.NonStandardOnly++
		}
		if n.HasMalformed {
			stats.Malformed++
		}
		for _, flag := range n.FlagNames {
			flagCounts[flag]++
		}
		for _, addr := range n.Addresses {
			addresses[strings.ToLower(addr)] = true
			if at := strings.LastIndex(addr, "@"); at > 0 && at < len(addr)-1 {
				domainCounts[strings.ToLower(addr[at+1:])]++
			}
		}
	}

	stats.TotalAddresses = len(addresses)
	stats.DistinctDomains = len(domainCounts)

	stats.FlagReferences = make([]EmailFlagReference, 0, len(emailFlagReferences))
	for _, ref := range emailFlagReferences {
		ref.Count = flagCounts[ref.Flag]
		ref.Color = emailSeriesColors[ref.Flag]
		stats.FlagReferences = append(stats.FlagReferences, ref)
	}

	for domain, count := range domainCounts {
		stats.DomainCounts = append(stats.DomainCounts, EmailDomainCount{Domain: domain, Count: count})
	}
	// Every domain is listed, not a top-N slice. The list is bounded by the
	// number of distinct published addresses -- a few dozen in practice -- and
	// a truncated list is worse than none here: the whole point of the section
	// is to show where FidoNet's email traffic would actually land.
	sort.Slice(stats.DomainCounts, func(i, j int) bool {
		if stats.DomainCounts[i].Count != stats.DomainCounts[j].Count {
			return stats.DomainCounts[i].Count > stats.DomainCounts[j].Count
		}
		return stats.DomainCounts[i].Domain < stats.DomainCounts[j].Domain
	})

	return stats
}

// emailAnalyticsPage is the template payload for /analytics/email.
type emailAnalyticsPage struct {
	Title      string
	ActivePage string
	Version    string
	Nodes      []storage.EmailCapableNode
	Stats      EmailSummaryStats
	// AllCount and EmailOnlyCount size both views of the filter, whichever
	// one is showing, so the toggle can label each with its count.
	AllCount       int
	EmailOnlyCount int
	// EmailOnly is true when the report is narrowed to nodes that email is
	// the only way to reach. The summary, flag counts, mail domains and node
	// table follow the filter; the trend does not.
	EmailOnly bool
	// AllURL and EmailOnlyURL are the toggle's links, each keeping the
	// request's other parameters.
	AllURL       string
	EmailOnlyURL string
	// FieldsOnURL and FieldsOffURL toggle the Location/System Name fallback.
	FieldsOnURL      string
	FieldsOffURL     string
	Trend            []storage.EmailFlagTrendPoint
	FlagOrder        []string
	Limit            int
	UseFieldFallback bool
	Error            error
}

// EmailAnalyticsHandler renders the FidoNet-over-email report.
func (s *Server) EmailAnalyticsHandler(w http.ResponseWriter, r *http.Request) {
	limit := parseLimit(r, 5000, 10000)
	domain := requestDomain(r)

	// Recovering an address from the Location or System Name field (FSP-1012
	// section 2.3.4) is a heuristic that FTS-5001 warns about, so it is opt-in
	// per request rather than the default.
	useFieldFallback := r.URL.Query().Get("fields") == "1"
	emailOnly := r.URL.Query().Get("only") == "email"

	var displayError error

	nodes, err := s.storage.GetEmailCapableNodes(r.Context(), limit, useFieldFallback, domain)
	if err != nil {
		var handled bool
		if displayError, handled = storageFailure("Email Analytics", "Failed to fetch email analytics data. Please try again later", err); handled {
			return
		}
		nodes = []storage.EmailCapableNode{}
	}

	trend, err := s.storage.GetEmailFlagTrend(r.Context(), domain)
	if err != nil {
		if clientGone("Email Analytics: flag trend", err) {
			return
		}
		// The trend is supporting context; losing it should not blank the page.
		logging.Errorf("Email Analytics: Error fetching trend: %v", err)
		trend = nil
	}

	allCount := len(nodes)
	shown := nodes
	if emailOnly {
		shown = filterEmailOnly(nodes)
	}

	data := emailAnalyticsPage{
		Title:            "FidoNet over Email",
		ActivePage:       "analytics",
		Version:          version.GetVersionInfo(),
		Nodes:            shown,
		Stats:            computeEmailStats(shown),
		AllCount:         allCount,
		EmailOnlyCount:   countEmailOnly(nodes),
		EmailOnly:        emailOnly,
		AllURL:           emailPageURL(r, "only", ""),
		EmailOnlyURL:     emailPageURL(r, "only", "email"),
		FieldsOnURL:      emailPageURL(r, "fields", "1"),
		FieldsOffURL:     emailPageURL(r, "fields", ""),
		Trend:            trend,
		FlagOrder:        storage.EmailFlagOrder(),
		Limit:            limit,
		UseFieldFallback: useFieldFallback,
		Error:            displayError,
	}

	s.renderStatus(w, "email_analytics", data, statusFor(displayError))
}

// filterEmailOnly keeps the nodes that email is the only way to reach.
func filterEmailOnly(nodes []storage.EmailCapableNode) []storage.EmailCapableNode {
	out := make([]storage.EmailCapableNode, 0, len(nodes))
	for _, n := range nodes {
		if n.EmailOnly {
			out = append(out, n)
		}
	}
	return out
}

func countEmailOnly(nodes []storage.EmailCapableNode) int {
	count := 0
	for _, n := range nodes {
		if n.EmailOnly {
			count++
		}
	}
	return count
}

// emailPageURL is this page's URL with one query parameter set, or removed
// when value is empty, keeping every other parameter (only, fields, limit,
// domain) so the page's toggles never undo one another.
func emailPageURL(r *http.Request, key, value string) string {
	q := r.URL.Query()
	if value == "" {
		q.Del(key)
	} else {
		q.Set(key, value)
	}
	if len(q) == 0 {
		return r.URL.Path
	}
	return r.URL.Path + "?" + q.Encode()
}
