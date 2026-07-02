// Package mem holds the hot-path memory primitives: Arena, a chunked bump
// allocator for variable-size data, and RowBuf, the columnar struct-of-arrays
// buffer that is the only shape generators fill.
package mem
