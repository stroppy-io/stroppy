//go:build !pgnoop_embed

package pgnoop

// embeddedBinary is nil for plain builds; CI release builds compile with
// -tags pgnoop_embed to carry the server binary inside the stroppy binary.
var embeddedBinary []byte
