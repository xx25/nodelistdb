// Package main defines the common sink abstraction for test result persistence.
package main

// resultsWriter is a sink for completed test records (CSV file, SQL database,
// NodelistDB API). Disabled sinks are simply never constructed; a resultSinks
// slice only ever holds live writers.
type resultsWriter interface {
	Name() string
	WriteRecord(rec *TestRecord) error
	Close() error
}

// resultSinks fans one test record out to every configured sink.
type resultSinks []resultsWriter

// writeAll writes rec to every sink, logging failures instead of propagating
// them so one sink's outage never costs the others the record.
func (s resultSinks) writeAll(rec *TestRecord, log *TestLogger) {
	for _, w := range s {
		if err := w.WriteRecord(rec); err != nil {
			log.Error("Failed to write %s record: %v", w.Name(), err)
		}
	}
}
