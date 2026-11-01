package main

// Set by scripts/build.mjs using Go linker flags.
var version = "dev"
var commit = "unknown"

type BuildInfo struct {
	Product string `json:"product"`
	Version string `json:"version"`
	Commit  string `json:"commit"`
}

func buildInfo() BuildInfo {
	return BuildInfo{Product: "Maker Track Studio", Version: version, Commit: commit}
}

func buildTitle() string {
	return "Maker Track Studio — " + version
}

// GetVersion exposes only the display version to the desktop interface.
func (a *App) GetVersion() string {
	return version
}
