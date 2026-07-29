package storage

// Date-related SQL queries. All of them are domain-scoped: bind the domain
// twice for the `(? = '' OR domain = ?)` filter; an empty domain matches all
// networks.

// optionalDomainSQL is the reusable optional-domain predicate.
// Bind the domain value twice; empty string disables the filter.
const optionalDomainSQL = "(? = '' OR domain = ?)"

// IsProcessedSQL returns SQL for checking if a nodelist date is already
// processed within one network. Binds: domain, nodelist_date.
func (qb *QueryBuilder) IsProcessedSQL() string {
	return "SELECT COUNT(*) FROM nodes WHERE domain = ? AND nodelist_date = ? LIMIT 1"
}

// LatestDateSQL returns SQL for getting the latest nodelist date.
// Binds: domain, domain.
func (qb *QueryBuilder) LatestDateSQL() string {
	return "SELECT MAX(nodelist_date) FROM nodes WHERE " + optionalDomainSQL
}

// AvailableDatesSQL returns SQL for getting all available nodelist dates.
// Binds: domain, domain.
func (qb *QueryBuilder) AvailableDatesSQL() string {
	return "SELECT DISTINCT nodelist_date FROM nodes WHERE " + optionalDomainSQL + " ORDER BY nodelist_date DESC"
}

// ExactDateExistsSQL returns SQL for checking if an exact date exists.
// Binds: date, domain, domain.
func (qb *QueryBuilder) ExactDateExistsSQL() string {
	return "SELECT COUNT(*) FROM nodes WHERE nodelist_date = ? AND " + optionalDomainSQL
}

// NearestDateBeforeSQL returns SQL for finding the closest date before a given
// date. Binds: date, domain, domain.
func (qb *QueryBuilder) NearestDateBeforeSQL() string {
	return `SELECT MAX(nodelist_date)
		FROM nodes
		WHERE nodelist_date < ? AND ` + optionalDomainSQL
}

// NearestDateAfterSQL returns SQL for finding the closest date after a given
// date. Binds: date, domain, domain.
func (qb *QueryBuilder) NearestDateAfterSQL() string {
	return `SELECT MIN(nodelist_date)
		FROM nodes
		WHERE nodelist_date > ? AND ` + optionalDomainSQL
}

// ConsecutiveNodelistCheckSQL returns SQL for checking gaps between dates.
// Binds: date, date, domain, domain.
func (qb *QueryBuilder) ConsecutiveNodelistCheckSQL() string {
	return "SELECT COUNT(DISTINCT nodelist_date) FROM nodes WHERE nodelist_date > ? AND nodelist_date < ? AND " + optionalDomainSQL
}

// NextNodelistDateSQL returns SQL for finding the next nodelist date after a
// given date. Binds: date, domain, domain.
func (qb *QueryBuilder) NextNodelistDateSQL() string {
	return "SELECT MIN(nodelist_date) FROM nodes WHERE nodelist_date > ? AND " + optionalDomainSQL
}
