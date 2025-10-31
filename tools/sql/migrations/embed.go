package migrations

import "embed"

//go:embed *.sql
//go:embed *.sum
var Content embed.FS
