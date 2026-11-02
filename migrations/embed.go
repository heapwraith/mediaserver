// SPDX-License-Identifier: GPL-3.0-or-later

// Package migrations embeds the numbered SQL migration files.
package migrations

import "embed"

// FS holds every NNN_name.sql file of this folder.
//
//go:embed *.sql
var FS embed.FS
