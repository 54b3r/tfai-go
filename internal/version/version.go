// Package version holds build-time version information for the tfai binary.
// The variables in this package are populated at build time via -ldflags:
//
//	go build -ldflags="-X github.com/54b3r/tfai-go/internal/version.Version=v1.2.3 \
//	                    -X github.com/54b3r/tfai-go/internal/version.Commit=abc1234 \
//	                    -X github.com/54b3r/tfai-go/internal/version.BuildDate=2025-01-01"
//
// When built without ldflags (e.g. `go run`), the values fall back to
// human-readable defaults so the binary is always usable.
package version

// Version is the semantic version of the binary (e.g. "v1.2.3").
// Set at build time via -ldflags. Defaults to "dev" for local builds.
var Version = "dev"

// Commit is the short git SHA of the commit the binary was built from.
// Set at build time via -ldflags. Defaults to "unknown".
var Commit = "unknown"

// BuildDate is the UTC date the binary was built (RFC3339 format).
// Set at build time via -ldflags. Defaults to "unknown".
var BuildDate = "unknown"
