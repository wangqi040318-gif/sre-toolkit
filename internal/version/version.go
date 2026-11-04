// Package version holds build metadata injected with -ldflags.
package version

import "fmt"

// Values replaced by the Makefile. Defaults keep `go run` useful.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// String is the human-readable build line.
func String() string {
	return fmt.Sprintf("%s (commit %s, built %s)", Version, Commit, Date)
}

// UserAgent identifies this client to the Kubernetes API server.
func UserAgent() string {
	return "sre-toolkit/" + Version
}
