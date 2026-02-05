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
					ORDER BY test_time DESC, modem_connect_speed DESC, modem_response_ms ASC
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
