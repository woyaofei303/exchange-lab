// Package migrations embeds the versioned PostgreSQL schema used by the API
// and the standalone migration command.
package migrations

import "embed"

// CurrentVersion is the schema version required by the current API binary.
const CurrentVersion int64 = 16

// FS contains SQL migration files at the package root.
//
//go:embed *.sql
var FS embed.FS
