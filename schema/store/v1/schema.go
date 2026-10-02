// Package storev1 carries the CodeBridge runtime store schema v1 as a
// language-neutral artifact (schema.sql) plus the Go binding the daemon embeds.
//
// schema.sql is authoritative for the version it describes: the file and the
// Version constant change together, and internal/runtime applies the DDL inside
// one migration transaction. Go and Swift consumers read the same file, so no
// binding is generated from Go structs (docs/v2/provider-contracts.md §7).
//
// The package name is storev1 rather than v1 so call sites read as
// storev1.SQL(); the directory name carries the schema version.
package storev1

import _ "embed"

// Version is the store schema version described by schema.sql.
const Version = 1

//go:embed schema.sql
var schema string

// SQL returns the schema v1 DDL.
func SQL() string { return schema }
