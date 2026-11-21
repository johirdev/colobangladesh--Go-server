// Package buildinfo exposes build metadata injected at compile time:
//
//	go build -ldflags "-X cholobangladesh-server/pkg/buildinfo.Version=1.2.0 \
//	  -X cholobangladesh-server/pkg/buildinfo.Commit=$(git rev-parse --short HEAD) \
//	  -X cholobangladesh-server/pkg/buildinfo.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
package buildinfo

var (
	Version   = "1.0.0-dev"
	Commit    = "unknown"
	BuildTime = "unknown"
)
