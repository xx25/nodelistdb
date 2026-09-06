package storage

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/nodelistdb/internal/database"
	"github.com/nodelistdb/internal/pingtrace"
)

// PingTraceOperations reads the FTS-4010 PING/TRACE measurement the
// testdaemon writes (ping_tests / ping_replies) for the web and API.
//
// Both tables are ReplacingMergeTree versioned by updated_at, so every read
// goes through FINAL. They hold at most a few hundred rows a fortnight, so
// the summary simply loads the window into memory and folds it in Go: the
// TRACE evaluation needs to walk each ping's hop list, which is no query
// shape ClickHouse would make clearer.
type PingTraceOperations struct {
	db database.DatabaseInterface
	mu sync.RWMutex
}

// NewPingTraceOperations creates the read layer.
func NewPingTraceOperations(db database.DatabaseInterface) *PingTraceOperations {
	return &PingTraceOperations{db: db}
}

// PingNodeSummary is one node's standing in the report: its flags, its
// latest ping per mode, and -- for a TRACE node -- what the transit
// evidence says.
type PingNodeSummary struct {
	Domain     string `json:"domain"`
	Zone       int    `json:"zone"`
	Net        int    `json:"net"`
	Node       int    `json:"node"`
	Address    string `json:"address"`
	SystemName string `json:"system_name"`
	SysopName  string `json:"sysop_name"`
	HasPing    bool   `json:"has_ping"`
	HasTrace   bool   `json:"has_trace"`

	// Latest is the most recent routed ping; LatestDirect the most recent
	// DIR ping (nil when that mode is not in use).
	Latest       *pingtrace.Ping `json:"latest,omitempty"`
	LatestDirect *pingtrace.Ping `json:"latest_direct,omitempty"`
	// Pings / Pongs count the window, all modes.
	Pings int `json:"pings"`
	Pongs int `json:"pongs"`

	// TraceVerdict is confirmed | silent | unobserved for a TRACE node
	// ("" otherwise). TraceSeen counts pings whose quoted outbound path
	// crossed this node as a transit hop; TraceNotices how many of those
	// earned a notice from it.
	TraceVerdict string `json:"trace_verdict,omitempty"`
	TraceSeen    int    `json:"trace_seen"`
	TraceNotices int    `json:"trace_notices"`
}

// TracerStat is a node that sent transit notices in the window, whether
// or not it flies TRACE.
type TracerStat struct {
	Address string `json:"address"`
	// Domain is the network the pings it answered belong to: without a
	// ?domain= filter the same address can appear once per network.
	Domain  string `json:"domain,omitempty"`
	Notices int    `json:"notices"`
	Flagged bool   `json:"flagged"`
}

// PingTraceSummary is the whole report for one network.
type PingTraceSummary struct {
	Domain string `json:"domain"`
	Days   int    `json:"days"`

	Nodes []PingNodeSummary `json:"nodes"`

	PingNodes int `json:"ping_nodes"`
	// Answered / Pending / Timeouts / Failed / Bounced classify the latest
	// routed ping of each PING node; NeverPinged is how many have none yet.
	Answered    int `json:"answered"`
	Pending     int `json:"pending"`
	Timeouts    int `json:"timeouts"`
	Failed      int `json:"failed"`
	Bounced     int `json:"bounced"`
	NeverPinged int `json:"never_pinged"`

	MedianRTTSeconds uint32 `json:"median_rtt_seconds"`
	MedianHops       int    `json:"median_hops"`

	TraceNodes      int `json:"trace_nodes"`
	TraceConfirmed  int `json:"trace_confirmed"`
	TraceSilent     int `json:"trace_silent"`
	TraceUnobserved int `json:"trace_unobserved"`

	// Tracers lists every node that sent a transit notice in the window.
	Tracers []TracerStat `json:"tracers"`
	// Robots counts pong senders by PID (software), most common first.
	Robots []RobotStat `json:"robots"`
}

// RobotStat is one PING robot implementation and how many nodes run it.
type RobotStat struct {
	Software string `json:"software"`
	Nodes    int    `json:"nodes"`
}

// PingReplyRow is one stored reply with its parsed hop list.
type PingReplyRow struct {
	pingtrace.Reply
	Hops []pingtrace.Hop `json:"hops"`
}

const pingTestReadColumns = `domain, zone, net, node, address, mode, sent_time, token, msgid, fidomail_message_id,
	first_hop, route_source, status, dispatched_time, reply_time, rtt_seconds,
	reply_message_id, reply_msgid, reply_from_name, reply_from_addr, robot_pid, robot_tearline,
	out_hops, out_hop_times, out_hop_software, out_vias_raw,
	back_hops, back_hop_times, back_hop_software, back_vias_raw,
	trace_count, error, updated_at`

var pingEpoch = time.Unix(0, 0).UTC()

func pingTime(t time.Time) time.Time {
	if !t.After(pingEpoch) {
		return time.Time{}
	}
	return t
}

func pingHops(addrs []string, times []time.Time, software, raw []string) []pingtrace.Hop {
	return pingtrace.HopsFromColumns(addrs, times, software, raw, pingTime)
}

// replyHops reconstructs the path a reply quoted. ping_replies stores each
// hop's parsed fields but not its raw Via line, so the raw text -- and with
// it the FTS-4009 "UTC" marker -- comes back only by re-parsing the stored
// body, which is the same input the daemon parsed on arrival. The stored
// columns stay the fallback for any row whose body no longer yields the
// same path.
func replyHops(body string, addrs []string, times []time.Time, soft []string) []pingtrace.Hop {
	if hops := pingtrace.ExtractPath(body); len(hops) == len(addrs) {
		return hops
	}
	return pingHops(addrs, times, soft, nil)
}

func scanPingRow(rows *sql.Rows) (pingtrace.Ping, error) {
	var (
		p                                          pingtrace.Ping
		zone, net, node                            uint16
		sentTime, dispatched, replyTime, updatedAt time.Time
		outHops, outSoft, outRaw                   []string
		backHops, backSoft, backRaw                []string
		outTimes, backTimes                        []time.Time
	)
	if err := rows.Scan(
		&p.Domain, &zone, &net, &node, &p.Address, &p.Mode, &sentTime, &p.Token, &p.MSGID, &p.FidomailMessageID,
		&p.FirstHop, &p.RouteSource, &p.Status, &dispatched, &replyTime, &p.RTTSeconds,
		&p.ReplyMessageID, &p.ReplyMSGID, &p.ReplyFromName, &p.ReplyFromAddr, &p.RobotPID, &p.RobotTearline,
		&outHops, &outTimes, &outSoft, &outRaw,
		&backHops, &backTimes, &backSoft, &backRaw,
		&p.TraceCount, &p.Error, &updatedAt,
	); err != nil {
		return p, fmt.Errorf("scan ping: %w", err)
	}
	p.Zone, p.Net, p.Node = int(zone), int(net), int(node)
	p.SentTime = sentTime
	p.DispatchedTime = pingTime(dispatched)
	p.ReplyTime = pingTime(replyTime)
	p.UpdatedAt = updatedAt
	p.OutHops = pingHops(outHops, outTimes, outSoft, outRaw)
	p.BackHops = pingHops(backHops, backTimes, backSoft, backRaw)
	return p, nil
}

// GetNodePings returns a node's pings, newest first.
func (po *PingTraceOperations) GetNodePings(ctx context.Context, domain string, zone, net, node int, limit int) ([]pingtrace.Ping, error) {
	po.mu.RLock()
	defer po.mu.RUnlock()
	if limit <= 0 {
		limit = 100
	}
	query := fmt.Sprintf(`SELECT %s FROM ping_tests FINAL
		WHERE zone = ? AND net = ? AND node = ? %s
		ORDER BY sent_time DESC
		LIMIT ?`, pingTestReadColumns, domainFilterSQL(domain, ""))
	rows, err := po.db.Conn().QueryContext(ctx, query, zone, net, node, limit)
	if err != nil {
		return nil, fmt.Errorf("query node pings: %w", err)
	}
	defer rows.Close()
	var out []pingtrace.Ping
	for rows.Next() {
		p, err := scanPingRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// GetNodePingReplies returns every stored reply linked to the node's
// pings -- pongs, trace notices, bounces -- newest first.
func (po *PingTraceOperations) GetNodePingReplies(ctx context.Context, domain string, zone, net, node int, limit int) ([]PingReplyRow, error) {
	po.mu.RLock()
	defer po.mu.RUnlock()
	if limit <= 0 {
		limit = 200
	}
	query := fmt.Sprintf(`SELECT fidomail_message_id, kind, ping_domain, ping_zone, ping_net, ping_node, ping_sent_time, ping_msgid,
			msgid, reply_id, from_name, from_addr, to_name, subject, body, date, received_at, pid, tearline,
			vias, hops, hop_times, hop_software, updated_at
		FROM ping_replies FINAL
		WHERE ping_zone = ? AND ping_net = ? AND ping_node = ? %s
		ORDER BY received_at DESC
		LIMIT ?`, domainFilterSQL(domain, "ping_"))
	rows, err := po.db.Conn().QueryContext(ctx, query, zone, net, node, limit)
	if err != nil {
		return nil, fmt.Errorf("query node ping replies: %w", err)
	}
	defer rows.Close()
	var out []PingReplyRow
	for rows.Next() {
		r, err := scanReplyRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func scanReplyRow(rows *sql.Rows) (PingReplyRow, error) {
	var (
		r                        PingReplyRow
		pz, pn, pnode            uint16
		pingSent, date, received time.Time
		updated                  time.Time
		hops, soft               []string
		times                    []time.Time
	)
	if err := rows.Scan(
		&r.FidomailMessageID, &r.Kind, &r.PingDomain, &pz, &pn, &pnode, &pingSent, &r.PingMSGID,
		&r.MSGID, &r.ReplyID, &r.FromName, &r.FromAddr, &r.ToName, &r.Subject, &r.Body, &date, &received, &r.PID, &r.Tearline,
		&r.Vias, &hops, &times, &soft, &updated,
	); err != nil {
		return r, fmt.Errorf("scan ping reply: %w", err)
	}
	r.PingZone, r.PingNet, r.PingNode = int(pz), int(pn), int(pnode)
	r.PingSentTime = pingTime(pingSent)
	r.Date = pingTime(date)
	r.ReceivedAt = pingTime(received)
	r.UpdatedAt = updated
	r.Hops = replyHops(r.Body, hops, times, soft)
	return r, nil
}

type traceNotice struct {
	from         string
	pingDomain   string
	pingAddress  string
	pingSentTime time.Time
}

// GetPingTraceSummary builds the report over the last `days` days (0 = 90).
func (po *PingTraceOperations) GetPingTraceSummary(ctx context.Context, domain string, days int) (*PingTraceSummary, error) {
	po.mu.RLock()
	defer po.mu.RUnlock()
	if days <= 0 {
		days = 90
	}
	conn := po.db.Conn()
	since := time.Now().Add(-time.Duration(days) * 24 * time.Hour).UTC()

	// 1. The flagged population on the current nodelist.
	nodesQuery := fmt.Sprintf(`
		SELECT domain, zone, net, node, system_name, sysop_name,
		       has(flags, 'PING') AS has_ping, has(flags, 'TRACE') AS has_trace
		FROM nodes
		WHERE (domain, nodelist_date) IN (SELECT domain, MAX(nodelist_date) FROM nodes WHERE 1 = 1 %s GROUP BY domain)
		  %s
		  AND conflict_sequence = 0
		  AND node_type NOT IN ('Down', 'Hold')
		  AND (has(flags, 'PING') OR has(flags, 'TRACE'))
		ORDER BY zone, net, node`, domainFilterSQL(domain, ""), domainFilterSQL(domain, ""))
	rows, err := conn.QueryContext(ctx, nodesQuery)
	if err != nil {
		return nil, fmt.Errorf("query flagged nodes: %w", err)
	}
	summary := &PingTraceSummary{Domain: domain, Days: days}
	index := map[string]int{}
	for rows.Next() {
		var (
			n                 PingNodeSummary
			zone, net, node   uint16
			hasPing, hasTrace uint8
		)
		if err := rows.Scan(&n.Domain, &zone, &net, &node, &n.SystemName, &n.SysopName, &hasPing, &hasTrace); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan flagged node: %w", err)
		}
		n.Zone, n.Net, n.Node = int(zone), int(net), int(node)
		n.Address = fmt.Sprintf("%d:%d/%d", zone, net, node)
		n.HasPing, n.HasTrace = hasPing != 0, hasTrace != 0
		index[n.Address+"@"+n.Domain] = len(summary.Nodes)
		summary.Nodes = append(summary.Nodes, n)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// 2. Every ping in the window.
	pingQuery := fmt.Sprintf(`SELECT %s FROM ping_tests FINAL
		WHERE sent_time >= ? %s
		ORDER BY sent_time DESC`, pingTestReadColumns, domainFilterSQL(domain, ""))
	prow, err := conn.QueryContext(ctx, pingQuery, since)
	if err != nil {
		return nil, fmt.Errorf("query pings: %w", err)
	}
	var pings []pingtrace.Ping
	for prow.Next() {
		p, err := scanPingRow(prow)
		if err != nil {
			prow.Close()
			return nil, err
		}
		pings = append(pings, p)
	}
	prow.Close()
	if err := prow.Err(); err != nil {
		return nil, err
	}

	// 3. Every trace notice in the window, keyed to its ping.
	traceQuery := fmt.Sprintf(`SELECT from_addr, ping_domain, ping_zone, ping_net, ping_node, ping_sent_time
		FROM ping_replies FINAL
		WHERE kind = 'trace' AND received_at >= ? %s`, domainFilterSQL(domain, "ping_"))
	trow, err := conn.QueryContext(ctx, traceQuery, since)
	if err != nil {
		return nil, fmt.Errorf("query trace notices: %w", err)
	}
	var notices []traceNotice
	for trow.Next() {
		var (
			n         traceNotice
			z, nn, nd uint16
			pingSent  time.Time
		)
		if err := trow.Scan(&n.from, &n.pingDomain, &z, &nn, &nd, &pingSent); err != nil {
			trow.Close()
			return nil, fmt.Errorf("scan trace notice: %w", err)
		}
		n.from = pingtrace.Node3D(n.from)
		n.pingAddress = fmt.Sprintf("%d:%d/%d", z, nn, nd)
		n.pingSentTime = pingSent
		notices = append(notices, n)
	}
	trow.Close()
	if err := trow.Err(); err != nil {
		return nil, err
	}

	foldPingTraceSummary(summary, index, pings, notices)
	return summary, nil
}

// nodeKey identifies one node in one FTN network, the only safe key for
// evidence gathered across networks.
type nodeKey struct {
	address string
	domain  string
}

// foldPingTraceSummary is the pure aggregation over the loaded window,
// split out so it can be tested without ClickHouse.
func foldPingTraceSummary(summary *PingTraceSummary, index map[string]int, pings []pingtrace.Ping, notices []traceNotice) {
	// Notices indexed by (ping key, transit node).
	noticeKey := func(pingDomain, pingAddress string, sent time.Time, from string) string {
		return pingDomain + "|" + pingAddress + "|" + sent.UTC().Format(time.RFC3339) + "|" + from
	}
	noticed := map[string]bool{}
	// Transit evidence is keyed by node AND network: zone:net/node numbers
	// are reused across FTN domains, and this summary is built with no
	// domain filter whenever /api/analytics/pingtrace is called without
	// ?domain=, so a bare address would pool one network's paths with
	// another's.
	tracers := map[nodeKey]int{}
	for _, n := range notices {
		noticed[noticeKey(n.pingDomain, n.pingAddress, n.pingSentTime, n.from)] = true
		tracers[nodeKey{n.from, n.pingDomain}]++
	}

	// Per-node latest ping per mode, counts, and transit evidence.
	seen := map[nodeKey]int{}   // transit node -> pings that crossed it
	earned := map[nodeKey]int{} // transit node -> of those, with a notice
	robots := map[string]map[string]bool{}
	var rtts []uint32
	var hopCounts []int
	for i := range pings {
		p := &pings[i]
		key := p.Address + "@" + p.Domain
		if idx, ok := index[key]; ok {
			n := &summary.Nodes[idx]
			n.Pings++
			if p.Status == pingtrace.StatusPong {
				n.Pongs++
			}
			switch p.Mode {
			case pingtrace.ModeDirect:
				if n.LatestDirect == nil {
					n.LatestDirect = p
				}
			default:
				if n.Latest == nil {
					n.Latest = p
				}
			}
		}
		if p.Status == pingtrace.StatusPong {
			rtts = append(rtts, p.RTTSeconds)
			if len(p.OutHops) > 0 {
				hopCounts = append(hopCounts, len(p.OutHops))
			}
			if p.RobotPID != "" {
				if robots[p.RobotPID] == nil {
					robots[p.RobotPID] = map[string]bool{}
				}
				robots[p.RobotPID][key] = true
			}
		}
		// Transit hops: every node on the quoted path that is neither
		// endpoint. The endpoints are named, never positional -- the
		// destination is p.Address and the origin is whoever authored the
		// MSGID (us). Trimming the first and last hop instead would drop
		// a real transit node at each end, because a sending system does
		// not normally stamp its own Via and a robot quotes the path as
		// it stood before its own toss stamp was added.
		origin := pingtrace.OriginAddress(p.MSGID)
		crossed := map[string]bool{}
		for _, h := range p.OutHops {
			addr := pingtrace.Node3D(h.Address)
			if addr == "" || addr == p.Address || addr == origin || crossed[addr] {
				continue
			}
			crossed[addr] = true
			k := nodeKey{addr, p.Domain}
			seen[k]++
			if noticed[noticeKey(p.Domain, p.Address, p.SentTime, addr)] {
				earned[k]++
			}
		}
	}

	for i := range summary.Nodes {
		n := &summary.Nodes[i]
		if n.HasPing {
			summary.PingNodes++
			switch {
			case n.Latest == nil:
				summary.NeverPinged++
			case n.Latest.Status == pingtrace.StatusPong:
				summary.Answered++
			case n.Latest.Status == pingtrace.StatusTimeout:
				summary.Timeouts++
			case n.Latest.Status == pingtrace.StatusFailed:
				summary.Failed++
			case n.Latest.Status == pingtrace.StatusNDR:
				summary.Bounced++
			default:
				summary.Pending++
			}
		}
		if n.HasTrace {
			summary.TraceNodes++
			n.TraceSeen = seen[nodeKey{n.Address, n.Domain}]
			n.TraceNotices = earned[nodeKey{n.Address, n.Domain}]
			switch {
			case n.TraceSeen == 0:
				n.TraceVerdict = "unobserved"
				summary.TraceUnobserved++
			case n.TraceNotices > 0:
				n.TraceVerdict = "confirmed"
				summary.TraceConfirmed++
			default:
				n.TraceVerdict = "silent"
				summary.TraceSilent++
			}
		}
	}

	if len(rtts) > 0 {
		sort.Slice(rtts, func(i, j int) bool { return rtts[i] < rtts[j] })
		summary.MedianRTTSeconds = rtts[len(rtts)/2]
	}
	if len(hopCounts) > 0 {
		sort.Ints(hopCounts)
		summary.MedianHops = hopCounts[len(hopCounts)/2]
	}
	for k, count := range tracers {
		flagged := false
		if idx, ok := index[k.address+"@"+k.domain]; ok {
			flagged = summary.Nodes[idx].HasTrace
		}
		summary.Tracers = append(summary.Tracers, TracerStat{Address: k.address, Domain: k.domain, Notices: count, Flagged: flagged})
	}
	sort.Slice(summary.Tracers, func(i, j int) bool {
		if summary.Tracers[i].Notices != summary.Tracers[j].Notices {
			return summary.Tracers[i].Notices > summary.Tracers[j].Notices
		}
		if summary.Tracers[i].Address != summary.Tracers[j].Address {
			return summary.Tracers[i].Address < summary.Tracers[j].Address
		}
		return summary.Tracers[i].Domain < summary.Tracers[j].Domain
	})
	for sw, nodes := range robots {
		summary.Robots = append(summary.Robots, RobotStat{Software: sw, Nodes: len(nodes)})
	}
	sort.Slice(summary.Robots, func(i, j int) bool {
		if summary.Robots[i].Nodes != summary.Robots[j].Nodes {
			return summary.Robots[i].Nodes > summary.Robots[j].Nodes
		}
		return summary.Robots[i].Software < summary.Robots[j].Software
	})
}
