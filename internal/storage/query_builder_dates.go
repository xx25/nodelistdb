package storage

// DateQueryBuilder handles date-related SQL queries.
// All queries are domain-scoped: bind the domain twice for the
// `(? = '' OR domain = ?)` filter; an empty domain matches all networks.
type DateQueryBuilder struct {
	base *QueryBuilder
}

// optionalDomainSQL is the reusable optional-domain predicate.
// Bind the domain value twice; empty string disables the filter.
const optionalDomainSQL = "(? = '' OR domain = ?)"

// IsProcessed returns SQL for checking if a nodelist date is already processed
// within one network. Binds: domain, nodelist_date.
func (dqb *DateQueryBuilder) IsProcessed() string {
	return "SELECT COUNT(*) FROM nodes WHERE domain = ? AND nodelist_date = ? LIMIT 1"
}

// Latest returns SQL for getting the latest nodelist date. Binds: domain, domain.
func (dqb *DateQueryBuilder) Latest() string {
	return "SELECT MAX(nodelist_date) FROM nodes WHERE " + optionalDomainSQL
}

// Available returns SQL for getting all available nodelist dates. Binds: domain, domain.
func (dqb *DateQueryBuilder) Available() string {
	return "SELECT DISTINCT nodelist_date FROM nodes WHERE " + optionalDomainSQL + " ORDER BY nodelist_date DESC"
}

// Exists returns SQL for checking if an exact date exists. Binds: date, domain, domain.
func (dqb *DateQueryBuilder) Exists() string {
	return "SELECT COUNT(*) FROM nodes WHERE nodelist_date = ? AND " + optionalDomainSQL
}

// NearestBefore returns SQL for finding the closest date before a given date.
// Binds: date, domain, domain.
func (dqb *DateQueryBuilder) NearestBefore() string {
	return `SELECT MAX(nodelist_date)
		FROM nodes
		WHERE nodelist_date < ? AND ` + optionalDomainSQL
}

// NearestAfter returns SQL for finding the closest date after a given date.
// Binds: date, domain, domain.
func (dqb *DateQueryBuilder) NearestAfter() string {
	return `SELECT MIN(nodelist_date)
		FROM nodes
		WHERE nodelist_date > ? AND ` + optionalDomainSQL
}

// ConsecutiveCheck returns SQL for checking gaps between dates.
// Binds: date, date, domain, domain.
func (dqb *DateQueryBuilder) ConsecutiveCheck() string {
	return "SELECT COUNT(DISTINCT nodelist_date) FROM nodes WHERE nodelist_date > ? AND nodelist_date < ? AND " + optionalDomainSQL
}

// NextDate returns SQL for finding the next nodelist date after a given date.
// Binds: date, domain, domain.
func (dqb *DateQueryBuilder) NextDate() string {
	return "SELECT MIN(nodelist_date) FROM nodes WHERE nodelist_date > ? AND " + optionalDomainSQL
}

// LEGACY METHODS - For backward compatibility, will be deprecated
// These maintain the old API while we migrate to the new structure

// IsProcessedSQL returns SQL for checking if a nodelist date is already processed
// Deprecated: Use QueryBuilder.Dates().IsProcessed() instead
func (qb *QueryBuilder) IsProcessedSQL() string {
	return qb.Dates().IsProcessed()
}

// LatestDateSQL returns SQL for getting the latest nodelist date
// Deprecated: Use QueryBuilder.Dates().Latest() instead
func (qb *QueryBuilder) LatestDateSQL() string {
	return qb.Dates().Latest()
}

// AvailableDatesSQL returns SQL for getting all available nodelist dates
// Deprecated: Use QueryBuilder.Dates().Available() instead
func (qb *QueryBuilder) AvailableDatesSQL() string {
	return qb.Dates().Available()
}

// ExactDateExistsSQL returns SQL for checking if an exact date exists
// Deprecated: Use QueryBuilder.Dates().Exists() instead
func (qb *QueryBuilder) ExactDateExistsSQL() string {
	return qb.Dates().Exists()
}

// NearestDateBeforeSQL returns SQL for finding the closest date before a given date
// Deprecated: Use QueryBuilder.Dates().NearestBefore() instead
func (qb *QueryBuilder) NearestDateBeforeSQL() string {
	return qb.Dates().NearestBefore()
}

// NearestDateAfterSQL returns SQL for finding the closest date after a given date
// Deprecated: Use QueryBuilder.Dates().NearestAfter() instead
func (qb *QueryBuilder) NearestDateAfterSQL() string {
	return qb.Dates().NearestAfter()
}

// ConsecutiveNodelistCheckSQL returns SQL for checking gaps between dates
// Deprecated: Use QueryBuilder.Dates().ConsecutiveCheck() instead
func (qb *QueryBuilder) ConsecutiveNodelistCheckSQL() string {
	return qb.Dates().ConsecutiveCheck()
}

// NextNodelistDateSQL returns SQL for finding the next nodelist date after a given date
// Deprecated: Use QueryBuilder.Dates().NextDate() instead
func (qb *QueryBuilder) NextNodelistDateSQL() string {
	return qb.Dates().NextDate()
}
