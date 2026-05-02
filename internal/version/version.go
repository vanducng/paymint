// Package version exposes build-time metadata stamped via -ldflags.
package version

// Build-time metadata, populated via -ldflags. See Makefile.
var (
	// Version is the semver tag (e.g. "v0.1.0") or "dev" for unstamped builds.
	Version = "dev"
	// Commit is the short git SHA at build time.
	Commit = "unknown"
	// Date is the UTC build timestamp in RFC 3339 format.
	Date = "unknown"
)

// String returns "paymint <version> (<commit>, <date>)".
func String() string {
	return "paymint " + Version + " (" + Commit + ", " + Date + ")"
}
