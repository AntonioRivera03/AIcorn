// Package bin embeds build artifacts the Go binaries shell out to at runtime.
// md-convert.cjs is produced by `cd app && npm run build:md-convert` (or
// `make build-md-convert`) and is gitignored, so a build fails loudly here
// rather than silently shipping without it.
package bin

import _ "embed"

//go:embed md-convert.cjs
var MdConvert []byte
