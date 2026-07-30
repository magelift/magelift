// Package dumpimport loads Magento SQL dumps into a target MySQL schema.
//
// Safety (D-04 / MIGRATE-05): a non-empty target (≥1 BASE TABLE in the schema)
// refuses import unless Options.Yes is true. With Yes, the importer performs a
// full schema-replace then imports so interrupted re-runs converge.
//
// Transport: host `mysql` when on PATH, otherwise
// `docker compose -f .magelift/compose.local.yml exec -T database mysql …`
// using localdev credentials. Set Options.Runner to RunnerKube to pipe SQL via
// kubectl exec into a VPC-adjacent pod (private-IP Cloud SQL). Dumps are piped
// (including .sql.gz); there is no Go SQL parser. Callers map errors onto
// seeddump journal MarkFailed.
package dumpimport
