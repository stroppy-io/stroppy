// Package next embeds the SDK source tree and the built-in test sources into the
// stroppy2 CLI binary. The CLI materialises these into a throwaway Go module and
// builds it with the toolchain on PATH; embedding the source (rather than
// depending on a published module) means the CLI and the SDK it builds against
// can never skew, and builds work offline.
//
// This file lives at the module root because a //go:embed pattern cannot escape
// its own directory (no "../"): only a file here can name the SDK package
// directories as embeddable children. It declares no runtime API — just the two
// embedded filesystems and a version string — so it does not extend the frozen
// SDK surface.
package next

import "embed"

// Version identifies the CLI and the SDK it carries. Dev builds are unversioned.
const Version = "dev"

// SDK holds the engine packages the CLI materialises as a replace target for a
// user (or built-in) test module: the importable SDK plus its go.mod/go.sum. It
// excludes tests/ and cmd/ — those are not part of the SDK a test imports.
//
//go:embed go.mod go.sum bench dag driver metrics rng mem sqlfile
var SDK embed.FS

// Builtins holds the source of the compiled-in tests, addressed by name under
// tests/<name>/. They are built through the same temp-module pipeline as user
// tests (uniform path), and `stroppy2 eject` writes them out for forking.
//
//go:embed tests/simple tests/tpcc
var Builtins embed.FS
