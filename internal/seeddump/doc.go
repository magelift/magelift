// Package seeddump persists seed-dump import status under .magelift/seed-dumps.
//
// Status lives only in the journal (D-03 / ADR 0010): magelift.yaml records the
// dump path via environments.<name>.seedDump; seedDumpStatus is never a YAML field.
package seeddump
