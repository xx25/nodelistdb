package storage

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/nodelistdb/internal/pingtrace"
)

// The ping_tests / ping_replies tables are ReplacingMergeTree versioned by
// updated_at: every write is a full row that supersedes the one with the
// same key, and every read goes through FINAL. Both tables are tiny (a
// few hundred rows a fortnight), so FINAL costs nothing measurable and
// spares the daemon any read-modify-write dance.
//
// Both INSERTs are batch-shaped (no VALUES placeholders) for the reason
// emailDomainCheckInsertSQL documents: the Exec form renders time.Time as
// toDateTime('<unix>'), which the server's Values fast path rejects before
// recovering, bumping an error counter on every write.

const pingTestInsertSQL = `INSERT INTO ping_tests
	(domain, zone, net, node, address, mode, sent_time, token, msgid, fidomail_message_id,
	 first_hop, route_source, status, dispatched_time, reply_time, rtt_seconds,
	 reply_message_id, reply_msgid, reply_from_name, reply_from_addr, robot_pid, robot_tearline,
	 out_hops, out_hop_times, out_hop_software, out_vias_raw,
	 back_hops, back_hop_times, back_hop_software, back_vias_raw,
	 trace_count, error, updated_at)`

const pingReplyInsertSQL = `INSERT INTO ping_replies
	(fidomail_message_id, kind, ping_domain, ping_zone, ping_net, ping_node, ping_sent_time, ping_msgid,
	 msgid, reply_id, from_name, from_addr, to_name, subject, body, date, received_at, pid, tearline,
	 vias, hops, hop_times, hop_software, updated_at)`

const pingTestSelectColumns = `domain, zone, net, node, address, mode, sent_time, token, msgid, fidomail_message_id,
	first_hop, route_source, status, dispatched_time, reply_time, rtt_seconds,
	reply_message_id, reply_msgid, reply_from_name, reply_from_addr, robot_pid, robot_tearline,
	out_hops, out_hop_times, out_hop_software, out_vias_raw,
	back_hops, back_hop_times, back_hop_software, back_vias_raw,
	trace_count, error, updated_at`

// zeroTime is what a DateTime DEFAULT toDateTime(0) column reads back as.
var zeroTime = time.Unix(0, 0).UTC()

// chTime renders a Go time for a non-nullable DateTime column: the zero
// time becomes the 1970 epoch the schema documents as "not yet".
func chTime(t time.Time) time.Time {
	if t.IsZero() {
		return zeroTime
	}
	return t.UTC()
}

// fromCHTime is the inverse: the epoch reads back as the zero time.
func fromCHTime(t time.Time) time.Time {
	if t.IsZero() || !t.After(zeroTime) {
		return time.Time{}
	}
	return t
}

func splitHops(hops []pingtrace.Hop) (addrs []string, times []time.Time, software []string, raw []string) {
	addrs = make([]string, 0, len(hops))
	times = make([]time.Time, 0, len(hops))
	software = make([]string, 0, len(hops))
	raw = make([]string, 0, len(hops))
	for _, h := range hops {
		addrs = append(addrs, h.Address)
		times = append(times, chTime(h.Time))
		software = append(software, h.Software)
		raw = append(raw, h.Raw)
	}
	return
}

func joinHops(addrs []string, times []time.Time, software, raw []string) []pingtrace.Hop {
	hops := make([]pingtrace.Hop, 0, len(addrs))
	for i := range addrs {
		h := pingtrace.Hop{Address: addrs[i]}
		if i < len(times) {
			h.Time = fromCHTime(times[i])
		}
		if i < len(software) {
			h.Software = software[i]
		}
		if i < len(raw) {
			h.Raw = raw[i]
			h.TimeIsUTC = pingtrace.TimeIsUTCInVia(raw[i])
		}
		hops = append(hops, h)
	}
	return hops
}

// GetPingCandidates lists the nodes on the latest nodelist of each given
// domain that fly PING or TRACE. Unlike GetNodesWithInternet it does not
// require any internet flag: a netmail PING is routed, so a PSTN-only node
// is as pingable as any other.
func (s *ClickHouseStorage) GetPingCandidates(ctx context.Context, domains []string) ([]pingtrace.Candidate, error) {
	if len(domains) == 0 {
		return nil, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(domains)), ",")
	args := make([]interface{}, 0, len(domains))
	for _, d := range domains {
		args = append(args, d)
	}
	query := fmt.Sprintf(`
		SELECT domain, zone, net, node, system_name, sysop_name,
		       has(flags, 'PING') AS has_ping,
		       has(flags, 'TRACE') AS has_trace,
		       JSONHas(toString(internet_config), 'protocols', 'IBN') AS has_ibn
		FROM nodes
		WHERE domain IN (%s)
		  AND (domain, nodelist_date) IN (SELECT domain, MAX(nodelist_date) FROM nodes WHERE domain IN (%s) GROUP BY domain)
		  AND conflict_sequence = 0
		  AND node_type NOT IN ('Down', 'Hold')
		  AND (has(flags, 'PING') OR has(flags, 'TRACE'))
		ORDER BY domain, zone, net, node`, placeholders, placeholders)
	args = append(args, args...)

	rows, err := s.conn.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query ping candidates: %w", err)
	}
	defer rows.Close()

	var out []pingtrace.Candidate
	for rows.Next() {
		// nodes.zone/net/node are Int32 in the production schema; the
		// native driver refuses a width conversion.
		var (
			c                         pingtrace.Candidate
			zone, net, node           int32
			hasPing, hasTrace, hasIBN uint8
		)
		if err := rows.Scan(&c.Domain, &zone, &net, &node, &c.SystemName, &c.SysopName, &hasPing, &hasTrace, &hasIBN); err != nil {
			return nil, fmt.Errorf("scan ping candidate: %w", err)
		}
		c.Zone, c.Net, c.Node = int(zone), int(net), int(node)
		c.Address = fmt.Sprintf("%d:%d/%d", zone, net, node)
		c.HasPing, c.HasTrace, c.HasIBN = hasPing != 0, hasTrace != 0, hasIBN != 0
		out = append(out, c)
	}
	return out, rows.Err()
}

// GetLatestPingTimes returns, per "address@domain|mode", the most recent
// sent_time of any ping, whatever its outcome. It is what decides whether a
// node is due.
func (s *ClickHouseStorage) GetLatestPingTimes(ctx context.Context) (map[string]time.Time, error) {
	rows, err := s.conn.Query(ctx, `
		SELECT domain, address, mode, max(sent_time)
		FROM ping_tests
		GROUP BY domain, address, mode`)
	if err != nil {
		return nil, fmt.Errorf("query latest ping times: %w", err)
	}
	defer rows.Close()

	out := make(map[string]time.Time)
	for rows.Next() {
		var domain, address, mode string
		var t time.Time
		if err := rows.Scan(&domain, &address, &mode, &t); err != nil {
			return nil, fmt.Errorf("scan latest ping time: %w", err)
		}
		out[pingtrace.DueKey(address, domain, mode)] = t
	}
	return out, rows.Err()
}

// GetRecentPings returns every ping sent since the given time, in its
// current state. The poller matches replies against this set, so it must
// include answered pings too: a trace notice can arrive after the pong.
func (s *ClickHouseStorage) GetRecentPings(ctx context.Context, since time.Time) ([]pingtrace.Ping, error) {
	rows, err := s.conn.Query(ctx, `SELECT `+pingTestSelectColumns+`
		FROM ping_tests FINAL
		WHERE sent_time >= ?
		ORDER BY sent_time`, since.UTC())
	if err != nil {
		return nil, fmt.Errorf("query recent pings: %w", err)
	}
	defer rows.Close()

	var out []pingtrace.Ping
	for rows.Next() {
		p, err := scanPing(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

type pingScanner interface {
	Scan(dest ...interface{}) error
}

func scanPing(row pingScanner) (pingtrace.Ping, error) {
	var (
		p                                            pingtrace.Ping
		zone, net, node                              uint16
		sentTime, dispatched, replyTime, updatedAt   time.Time
		outHops, outSoft, outRaw, backHops, backSoft []string
		backRaw                                      []string
		outTimes, backTimes                          []time.Time
	)
	if err := row.Scan(
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
	p.DispatchedTime = fromCHTime(dispatched)
	p.ReplyTime = fromCHTime(replyTime)
	p.UpdatedAt = updatedAt
	p.OutHops = joinHops(outHops, outTimes, outSoft, outRaw)
	p.BackHops = joinHops(backHops, backTimes, backSoft, backRaw)
	return p, nil
}

// StorePing writes the ping's current state, superseding any earlier row
// with the same key.
func (s *ClickHouseStorage) StorePing(ctx context.Context, p pingtrace.Ping) error {
	batch, err := s.conn.PrepareBatch(ctx, pingTestInsertSQL)
	if err != nil {
		return fmt.Errorf("prepare ping batch: %w", err)
	}
	outHops, outTimes, outSoft, outRaw := splitHops(p.OutHops)
	backHops, backTimes, backSoft, backRaw := splitHops(p.BackHops)
	updated := p.UpdatedAt
	if updated.IsZero() {
		updated = time.Now()
	}
	if err := batch.Append(
		p.Domain, uint16(p.Zone), uint16(p.Net), uint16(p.Node), p.Address, p.Mode, chTime(p.SentTime), p.Token, p.MSGID, p.FidomailMessageID,
		p.FirstHop, p.RouteSource, p.Status, chTime(p.DispatchedTime), chTime(p.ReplyTime), p.RTTSeconds,
		p.ReplyMessageID, p.ReplyMSGID, p.ReplyFromName, p.ReplyFromAddr, p.RobotPID, p.RobotTearline,
		outHops, outTimes, outSoft, outRaw,
		backHops, backTimes, backSoft, backRaw,
		p.TraceCount, p.Error, updated.UTC(),
	); err != nil {
		return fmt.Errorf("append ping: %w", err)
	}
	if err := batch.Send(); err != nil {
		return fmt.Errorf("send ping batch: %w", err)
	}
	return nil
}

// StorePingReply records one inbound reply, matched or not.
func (s *ClickHouseStorage) StorePingReply(ctx context.Context, r pingtrace.Reply, hops []pingtrace.Hop) error {
	batch, err := s.conn.PrepareBatch(ctx, pingReplyInsertSQL)
	if err != nil {
		return fmt.Errorf("prepare reply batch: %w", err)
	}
	hopAddrs, hopTimes, hopSoft, _ := splitHops(hops)
	vias := r.Vias
	if vias == nil {
		vias = []string{}
	}
	updated := r.UpdatedAt
	if updated.IsZero() {
		updated = time.Now()
	}
	if err := batch.Append(
		r.FidomailMessageID, r.Kind, r.PingDomain, uint16(r.PingZone), uint16(r.PingNet), uint16(r.PingNode), chTime(r.PingSentTime), r.PingMSGID,
		r.MSGID, r.ReplyID, r.FromName, r.FromAddr, r.ToName, r.Subject, r.Body, chTime(r.Date), chTime(r.ReceivedAt), r.PID, r.Tearline,
		vias, hopAddrs, hopTimes, hopSoft, updated.UTC(),
	); err != nil {
		return fmt.Errorf("append reply: %w", err)
	}
	if err := batch.Send(); err != nil {
		return fmt.Errorf("send reply batch: %w", err)
	}
	return nil
}

// GetKnownReplyIDs returns the fidomail message ids of every reply already
// recorded since the given time, so a re-read of the inbox (after a restart
// loses the in-memory watermark) stores nothing twice.
func (s *ClickHouseStorage) GetKnownReplyIDs(ctx context.Context, since time.Time) (map[uint64]bool, error) {
	rows, err := s.conn.Query(ctx, `SELECT DISTINCT fidomail_message_id FROM ping_replies WHERE received_at >= ?`, since.UTC())
	if err != nil {
		return nil, fmt.Errorf("query known reply ids: %w", err)
	}
	defer rows.Close()
	out := make(map[uint64]bool)
	for rows.Next() {
		var id uint64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan reply id: %w", err)
		}
		out[id] = true
	}
	return out, rows.Err()
}
