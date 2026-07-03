package runner

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// sdkModulePath is the import path of the embedded SDK. The generated module
// requires it and replaces it with the materialised SDK directory, so user and
// built-in tests import the SDK by its real path with no rewriting.
const sdkModulePath = "github.com/stroppy-io/stroppy/next"

// pseudoVersion is the placeholder version for the replaced SDK require. Its
// value is irrelevant — a local replace directive is never checksum-verified —
// but a require line must name some version.
const pseudoVersion = "v0.0.0-00010101000000-000000000000"

// genGoMod derives the temp module's go.mod from the SDK's own go.mod: it renames
// the module to usertest, keeps the go directive and the SDK's require block
// verbatim (so versions and go.sum never drift), then appends the SDK self-require
// and a replace pointing at the materialised SDK directory. Copying the require
// block verbatim is what lets the SDK's go.sum be reused unchanged.
func genGoMod(sdkGoMod []byte, sdkDir string) []byte {
	var b strings.Builder
	for line := range strings.SplitSeq(strings.TrimRight(string(sdkGoMod), "\n"), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "module ") {
			b.WriteString("module usertest\n")
			continue
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	fmt.Fprintf(&b, "\nrequire %s %s\n", sdkModulePath, pseudoVersion)
	fmt.Fprintf(&b, "\nreplace %s => %s\n", sdkModulePath, sdkDir)
	return []byte(b.String())
}

// buildHash is the cache key for a build: a function of the SDK content (via
// sdkHash) and the test's build sources (names + bytes). Editing any build file,
// or changing the embedded SDK, yields a new key and thus a new build directory,
// while an unchanged (SDK, source) pair always maps to the same directory.
func buildHash(sdkHash string, files map[string][]byte) string {
	h := sha256.New()
	h.Write([]byte(sdkHash))
	writeLen(h, len(files))
	for _, name := range sortedNames(files) {
		h.Write([]byte(name))
		writeLen(h, len(files[name]))
		h.Write(files[name])
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// hashFS hashes an ordered set of (path, content) pairs — the content identity
// of the embedded SDK tree.
func hashFS(paths []string, read func(string) ([]byte, error)) (string, error) {
	sort.Strings(paths)
	h := sha256.New()
	for _, p := range paths {
		b, err := read(p)
		if err != nil {
			return "", err
		}
		h.Write([]byte(p))
		writeLen(h, len(b))
		h.Write(b)
	}
	return hex.EncodeToString(h.Sum(nil))[:16], nil
}

// writeLen mixes a length into the hash so concatenation is unambiguous.
func writeLen(h interface{ Write([]byte) (int, error) }, n int) {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], uint64(n))
	_, _ = h.Write(buf[:])
}
