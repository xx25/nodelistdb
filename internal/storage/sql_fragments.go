package storage

// SQL text shared across query builders, kept together so a rule about how a
// column must be read is written down once.

// internetConfigSelectSQL is how `nodes.internet_config` must be read back.
//
// The column is a native ClickHouse JSON, and its values nest Array(JSON(...))
// under dynamic paths (e.g. protocols.IFT). Once a document's inner paths spill
// into the JSON shared-data structure, clickhouse-go v2.40.1 cannot decode the
// nested SharedVariant: it misreads the length prefix and either fails the query
// or calls makeslice with a garbage length, which panics. That panic happens on
// the driver's own read goroutine, so no handler recover() can contain it -- it
// takes the whole process down (see the 2026-07-24 outage).
//
// Rendering server-side with toString() sidesteps the native JSON decoder
// entirely and hands the client a plain String. Node.InternetConfig is a
// json.RawMessage, so the string form is exactly what the result parser wants.
// Do NOT "optimize" this back to a bare `internet_config`.
const internetConfigSelectSQL = `toString(internet_config) AS internet_config`
