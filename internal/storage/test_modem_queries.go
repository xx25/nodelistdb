package storage

import (
	"fmt"
	"sync"

	"github.com/nodelistdb/internal/database"
)

// ModemQueryOperations handles modem-specific test result queries
type ModemQueryOperations struct {
	db database.DatabaseInterface
	mu sync.RWMutex
}

// NewModemQueryOperations creates a new modem query operations instance
func NewModemQueryOperations(db database.DatabaseInterface) *ModemQueryOperations {
	return &ModemQueryOperations{db: db}
}

// GetModemAccessibleNodes returns nodes that have been successfully reached via modem tests
// within the specified time range. Returns the latest successful modem test per node.
func (mq *ModemQueryOperations) GetModemAccessibleNodes(limit int, days int, includeZeroNodes bool) ([]ModemAccessibleNode, error) {
	mq.mu.RLock()
	defer mq.mu.RUnlock()

	if limit <= 0 {
		limit = DefaultSearchLimit
	}
	if limit > MaxPSTNSearchLimit {
		limit = MaxPSTNSearchLimit
	}

	conn := mq.db.Conn()

	nodeFilter := ""
	if !includeZeroNodes {
		nodeFilter = "AND node != 0"
	}

	query := fmt.Sprintf(`
		WITH ranked_modem_tests AS (
			SELECT
				zone, net, node, address, test_time,
				modem_phone_dialed, modem_connect_speed, modem_protocol,
				modem_system_name, modem_mailer_info, modem_operator_name,
				modem_connect_string, modem_response_ms, modem_address_valid,
				modem_remote_location, modem_remote_sysop,
				modem_tx_speed, modem_rx_speed, modem_modulation, test_source,
				row_number() OVER (
					PARTITION BY zone, net, node
					ORDER BY test_time DESC, modem_tx_speed DESC, modem_connect_speed DESC, modem_response_ms ASC
				) as rn
			FROM node_test_results
			WHERE test_time >= now() - INTERVAL ? DAY
				AND modem_tested = true
				AND modem_success = true
				%s
		)
		SELECT
			zone, net, node, address, test_time,
			modem_phone_dialed, modem_connect_speed, modem_protocol,
			modem_system_name, modem_mailer_info, modem_operator_name,
			modem_connect_string, modem_response_ms, modem_address_valid,
			modem_remote_location, modem_remote_sysop,
			modem_tx_speed, modem_rx_speed, modem_modulation, test_source
		FROM ranked_modem_tests
		WHERE rn = 1
		ORDER BY test_time DESC
		LIMIT ?`, nodeFilter)

	rows, err := conn.Query(query, days, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query modem accessible nodes: %w", err)
	}
	defer rows.Close()

	var results []ModemAccessibleNode
	for rows.Next() {
		var n ModemAccessibleNode
		err := rows.Scan(
			&n.Zone, &n.Net, &n.Node, &n.Address, &n.TestTime,
			&n.ModemPhoneDialed, &n.ModemConnectSpeed, &n.ModemProtocol,
			&n.ModemSystemName, &n.ModemMailerInfo, &n.ModemOperatorName,
			&n.ModemConnectString, &n.ModemResponseMs, &n.ModemAddressValid,
			&n.ModemRemoteLocation, &n.ModemRemoteSysop,
			&n.ModemTxSpeed, &n.ModemRxSpeed, &n.ModemModulation, &n.TestSource,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan modem accessible node row: %w", err)
		}
		results = append(results, n)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating modem accessible rows: %w", err)
	}

	return results, nil
}

// GetDetailedModemTestResult returns a single detailed modem test result for a specific node and test time
func (mq *ModemQueryOperations) GetDetailedModemTestResult(zone, net, node int, testTime string) (*ModemTestDetail, error) {
	mq.mu.RLock()
	defer mq.mu.RUnlock()

	conn := mq.db.Conn()

	query := `
		SELECT
			zone, net, node, address, test_time, test_source,
			modem_connect_speed, modem_protocol, modem_phone_dialed,
			modem_ring_count, modem_carrier_time_ms, modem_connect_string, modem_response_ms,
			modem_system_name, modem_mailer_info, modem_addresses, modem_address_valid,
			modem_response_type, modem_remote_location, modem_remote_sysop, modem_error,
			modem_operator_name, modem_operator_prefix, modem_dial_time_ms, modem_emsi_time_ms,
			modem_tx_speed, modem_rx_speed, modem_compression, modem_modulation,
			modem_line_quality, modem_snr, modem_rx_level, modem_tx_power,
			modem_round_trip_delay, modem_local_retrains, modem_remote_retrains,
			modem_termination_reason, modem_stats_notes, modem_line_stats,
			modem_cdr_session_id, modem_cdr_codec, modem_cdr_rtp_jitter_ms, modem_cdr_rtp_delay_ms,
			modem_cdr_packet_loss, modem_cdr_remote_packet_loss,
			modem_cdr_local_mos, modem_cdr_remote_mos,
			modem_cdr_local_r_factor, modem_cdr_remote_r_factor,
			modem_cdr_term_reason, modem_cdr_term_category,
			modem_ast_disposition, modem_ast_peer, modem_ast_duration, modem_ast_billsec,
			modem_ast_hangup_cause, modem_ast_hangup_source, modem_ast_early_media,
			modem_caller_id, modem_used, modem_match_reason
		FROM node_test_results
		WHERE zone = ? AND net = ? AND node = ?
			AND test_time = parseDateTimeBestEffort(?)
			AND modem_tested = true
		ORDER BY modem_tx_speed DESC, modem_connect_speed DESC, modem_response_ms ASC
		LIMIT 1`

	row := conn.QueryRow(query, zone, net, node, testTime)

	var d ModemTestDetail
	err := row.Scan(
		&d.Zone, &d.Net, &d.Node, &d.Address, &d.TestTime, &d.TestSource,
		&d.ConnectSpeed, &d.Protocol, &d.PhoneDialed,
		&d.RingCount, &d.CarrierTimeMs, &d.ConnectString, &d.ResponseMs,
		&d.SystemName, &d.MailerInfo, &d.Addresses, &d.AddressValid,
		&d.ResponseType, &d.RemoteLocation, &d.RemoteSysop, &d.Error,
		&d.OperatorName, &d.OperatorPrefix, &d.DialTimeMs, &d.EmsiTimeMs,
		&d.TxSpeed, &d.RxSpeed, &d.Compression, &d.Modulation,
		&d.LineQuality, &d.SNR, &d.RxLevel, &d.TxPower,
		&d.RoundTripDelay, &d.LocalRetrains, &d.RemoteRetrains,
		&d.TerminationReason, &d.StatsNotes, &d.RawLineStats,
		&d.CdrSessionId, &d.CdrCodec, &d.CdrRtpJitterMs, &d.CdrRtpDelayMs,
		&d.CdrPacketLoss, &d.CdrRemotePacketLoss,
		&d.CdrLocalMos, &d.CdrRemoteMos,
		&d.CdrLocalRFactor, &d.CdrRemoteRFactor,
		&d.CdrTermReason, &d.CdrTermCategory,
		&d.AstDisposition, &d.AstPeer, &d.AstDuration, &d.AstBillsec,
		&d.AstHangupCause, &d.AstHangupSource, &d.AstEarlyMedia,
		&d.CallerID, &d.ModemUsed, &d.MatchReason,
	)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to scan modem test detail: %w", err)
	}

	return &d, nil
}
