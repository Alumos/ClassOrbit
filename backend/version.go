package main

import "fmt"

// These values default to development identifiers and are replaced through
// -ldflags in release and Docker builds.
var (
	appVersion  = "dev"
	buildCommit = "unknown"
)

type buildInfo struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
}

func currentBuildInfo() buildInfo {
	return buildInfo{Version: appVersion, Commit: buildCommit}
}

func buildVersionText() string {
	return fmt.Sprintf("ClassOrbit %s (%s)", appVersion, buildCommit)
}
