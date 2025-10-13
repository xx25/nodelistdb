package storage

// Date-related SQL query methods

// IsProcessedSQL returns SQL for checking if a nodelist date is already processed
func (qb *QueryBuilder) IsProcessedSQL() string {
	return "SELECT COUNT(*) FROM nodes WHERE nodelist_date = ? LIMIT 1"
}

// LatestDateSQL returns SQL for getting the latest nodelist date
func (qb *QueryBuilder) LatestDateSQL() string {
	return "SELECT MAX(nodelist_date) FROM nodes"
}

// AvailableDatesSQL returns SQL for getting all available nodelist dates
func (qb *QueryBuilder) AvailableDatesSQL() string {
	return "SELECT DISTINCT nodelist_date FROM nodes ORDER BY nodelist_date DESC"
}

// ExactDateExistsSQL returns SQL for checking if an exact date exists
func (qb *QueryBuilder) ExactDateExistsSQL() string {
	return "SELECT COUNT(*) FROM nodes WHERE nodelist_date = ?"
}

// NearestDateBeforeSQL returns SQL for finding the closest date before a given date
func (qb *QueryBuilder) NearestDateBeforeSQL() string {
	return `SELECT MAX(nodelist_date)
		FROM nodes
		WHERE nodelist_date < ?`
}

// NearestDateAfterSQL returns SQL for finding the closest date after a given date
func (qb *QueryBuilder) NearestDateAfterSQL() string {
	return `SELECT MIN(nodelist_date)
		FROM nodes
		WHERE nodelist_date > ?`
}

// ConsecutiveNodelistCheckSQL returns SQL for checking gaps between dates
func (qb *QueryBuilder) ConsecutiveNodelistCheckSQL() string {
	return "SELECT COUNT(DISTINCT nodelist_date) FROM nodes WHERE nodelist_date > ? AND nodelist_date < ?"
}

// NextNodelistDateSQL returns SQL for finding the next nodelist date after a given date
func (qb *QueryBuilder) NextNodelistDateSQL() string {
	return "SELECT MIN(nodelist_date) FROM nodes WHERE nodelist_date > ?"
}
