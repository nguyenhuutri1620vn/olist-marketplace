// Package migrations embeds this service's SQL migrations, applied at startup
// by golang-migrate. Name files NNNNNN_<name>.up.sql / .down.sql.
package migrations

import "embed"

// FS holds every file in this folder. The wildcard keeps the package compiling
// while there are no .sql files yet; golang-migrate ignores non-migration names.
//
//go:embed *
var FS embed.FS
