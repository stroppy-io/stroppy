//go:build pgnoop_embed

package pgnoop

import _ "embed"

// embeddedBinary carries the host-matching pg-noop server binary. Release CI
// places it at internal/pgnoop/embedded/pgnoop before building with
// -tags pgnoop_embed.
//
//go:embed embedded/pgnoop
var embeddedBinary []byte
